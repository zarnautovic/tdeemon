package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
)

// ErrNothingToUndo reports that the user has no change left to undo.
var ErrNothingToUndo = errors.New("nothing to undo")

// UndoReason says why a change cannot be undone, in words for the user
// or the model; a client that translates them tells them apart by the
// constants.
type UndoReason string

// The reasons a change cannot be undone.
const (
	// UndoChanged: the entity is no longer as the change left it (an
	// import wrote it, or a later change that undo passed over).
	UndoChanged UndoReason = "it changed since"
	// UndoNameTaken: another food took the food's name.
	UndoNameTaken UndoReason = "another food has taken its name since"
	// UndoAliasTaken: another food took one of the food's aliases.
	UndoAliasTaken UndoReason = "another food has taken one of its aliases since"
	// UndoItemTaken: another meal took one of the meal's item ids.
	UndoItemTaken UndoReason = "another meal has taken one of its items since"
	// UndoUnreadable: the change's entity or a state of it does not
	// decode, or the entity's state cannot be read.
	UndoUnreadable UndoReason = "its state cannot be read"
	// UndoStuck: the state before breaks a rule of a table now.
	UndoStuck UndoReason = "it cannot be put back"
)

// clash is what a way back returns, before it writes anything, when
// putting the entity back would clash with what came since.
type clash struct{ reason UndoReason }

func (c clash) Error() string { return string(c.reason) }

// errUnreadable marks a change whose own data, its entity or a state of
// it, does not decode.
var errUnreadable = errors.New("the change does not decode")

// UndoConflictError reports that the latest change could not be taken
// back: the entity changed since, putting it back clashes with what came
// since, or the change cannot be read or put back; Reason says which. The
// change is passed over from then on, so the next undo takes back the one
// before it.
type UndoConflictError struct {
	Event  event.Event
	Reason UndoReason
	// Err is what failed, if anything did: for the logs, not for the user.
	// It is not unwrapped, so a conflict does not pass for an error of the
	// write that clashed (ErrMealConflict).
	Err error
}

func (e *UndoConflictError) Error() string {
	return fmt.Sprintf("%s of %s cannot be undone: %s", e.Event.Action, e.Event.Entity, e.Reason)
}

// MaxEvents bounds Events.
const MaxEvents = 100

// record writes the event of a write in its transaction tx. before and
// after are the entity's state, marshalled as JSON (json.RawMessage as it
// is); nil, or a nil pointer, records none. A write that left the state as
// it was (a retry) records nothing, and the id is uuid.Nil.
func record(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, action event.Action, entity string,
	before, after any, now time.Time,
) (uuid.UUID, error) {
	b, err := stateJSON(before)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("event %s: %w", action, err)
	}
	af, err := stateJSON(after)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("event %s: %w", action, err)
	}
	if b == af && action != event.Undo {
		return uuid.Nil(), nil
	}
	id := uuid.NewV7()
	_, err = tx.ExecContext(ctx, `INSERT INTO events
		(id, user_id, at, actor, client, model, operation, entity, before_state, after_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), userID.String(), formatTime(now), string(a.Kind), a.Client, a.Model, string(action), entity, b, af)
	return id, err
}

// stateJSON returns v as JSON text, or nil (an untyped nil, so two "no
// state" compare equal) for no state.
func stateJSON(v any) (any, error) {
	switch s := v.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		if s == nil {
			return nil, nil
		}
		return string(s), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return nil, nil
	}
	return string(b), nil
}

const eventColumns = "id, at, actor, client, model, operation, entity, before_state, after_state, undone_by"

func scanEvent(row interface{ Scan(...any) error }) (event.Event, error) {
	var e event.Event
	var id, at, kind, action string
	var before, after, undoneBy sql.NullString
	if err := row.Scan(&id, &at, &kind, &e.Actor.Client, &e.Actor.Model, &action, &e.Entity, &before, &after, &undoneBy); err != nil {
		return event.Event{}, err
	}
	var err error
	if e.ID, err = uuid.Parse(id); err != nil {
		return event.Event{}, fmt.Errorf("event id %q: %w", id, err)
	}
	if e.At, err = parseTime(at); err != nil {
		return event.Event{}, err
	}
	e.Actor.Kind, e.Action = event.ActorKind(kind), event.Action(action)
	if before.Valid {
		e.Before = json.RawMessage(before.String)
	}
	if after.Valid {
		e.After = json.RawMessage(after.String)
	}
	if undoneBy.Valid {
		if e.UndoneBy, err = uuid.Parse(undoneBy.String); err != nil {
			return event.Event{}, fmt.Errorf("event undone by %q: %w", undoneBy.String, err)
		}
	}
	return e, nil
}

// Events returns up to limit of userID's latest events, newest first.
func (db *DB) Events(ctx context.Context, userID uuid.UUID, limit int) ([]event.Event, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := db.read.QueryContext(ctx, "SELECT "+eventColumns+" FROM events WHERE user_id = ? ORDER BY rowid DESC LIMIT ?",
		userID.String(), min(limit, MaxEvents))
	if err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []event.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("events: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	return out, nil
}

// UndoLast takes back userID's latest change that is not undone yet (an
// import's writes and undos themselves are skipped), records the undo as
// an event of a's, and returns the event it took back, marked undone.
// Undoing again takes back the change before: undo walks back, it does
// not redo. When the entity is no longer as the change left it (an import
// wrote it, or a later change that undo passed over), putting it back
// would lose that; when putting it back
// clashes with what came since, or the change cannot be read or put
// back, it cannot be undone. Then UndoLast changes nothing, returns an
// *UndoConflictError, and passes over that change from then on. A failure
// of the database or of ctx is returned as it is and passes over nothing,
// so a later undo tries the same change again. With nothing left it
// returns ErrNothingToUndo.
func (db *DB) UndoLast(ctx context.Context, userID uuid.UUID, a event.Actor, now time.Time) (event.Event, error) {
	if err := a.Validate(); err != nil {
		return event.Event{}, fmt.Errorf("invalid actor: %w", err)
	}
	var undone event.Event
	var conflict *UndoConflictError
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		e, err := scanEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+` FROM events
			WHERE user_id = ? AND actor = 'user' AND operation <> 'undo' AND undone_by IS NULL AND stale = 0
			ORDER BY rowid DESC LIMIT 1`, userID.String()))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNothingToUndo
		}
		if err != nil {
			return err
		}
		after, c, err := takeBackChecked(ctx, tx, userID, e, now)
		if err != nil || c != nil {
			conflict = c
			return err
		}
		undoID, err := record(ctx, tx, userID, a, event.Undo, e.ID.String(), e.After, after, now)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE events SET undone_by = ? WHERE user_id = ? AND id = ?",
			undoID.String(), userID.String(), e.ID.String()); err != nil {
			return err
		}
		e.UndoneBy = undoID
		undone = e
		return nil
	})
	if errors.Is(err, ErrNothingToUndo) {
		return event.Event{}, err
	}
	if err != nil {
		return event.Event{}, fmt.Errorf("undo: %w", err)
	}
	if conflict != nil {
		return event.Event{}, conflict
	}
	return undone, nil
}

// takeBackChecked puts e's entity back when it is as e left it, and
// returns the state the undo left. When it is not, or the change cannot
// be read or put back, it changes nothing but the event, which it marks
// stale, and returns the conflict: one change that cannot be taken back
// must not keep undo from the ones before it. A failure of the database
// or of ctx is not the change's, and it returns it as an error.
func takeBackChecked(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) (any, *UndoConflictError, error) {
	passOver := func(reason UndoReason, cause error) (any, *UndoConflictError, error) {
		_, err := tx.ExecContext(ctx, "UPDATE events SET stale = 1 WHERE user_id = ? AND id = ?", userID.String(), e.ID.String())
		return nil, &UndoConflictError{Event: e, Reason: reason, Err: cause}, err
	}
	current, err := stateNow(ctx, tx, userID, e)
	if err != nil {
		if !belongsToChange(err) {
			return nil, nil, fmt.Errorf("state of %s: %w", e.Entity, err)
		}
		return passOver(UndoUnreadable, err)
	}
	if c, err := stateJSON(current); err != nil {
		return nil, nil, err
	} else if want, _ := stateJSON(e.After); c != want {
		return passOver(UndoChanged, nil)
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT take_back"); err != nil {
		return nil, nil, err
	}
	if err := takeBack(ctx, tx, userID, e, now); err != nil {
		err = fmt.Errorf("take back %s of %s: %w", e.Action, e.Entity, err)
		if !belongsToChange(err) {
			return nil, nil, err
		}
		if _, rerr := tx.ExecContext(ctx, "ROLLBACK TO take_back"); rerr != nil {
			return nil, nil, errors.Join(err, rerr)
		}
		if _, rerr := tx.ExecContext(ctx, "RELEASE take_back"); rerr != nil {
			return nil, nil, errors.Join(err, rerr)
		}
		reason := UndoStuck
		if c, ok := errors.AsType[clash](err); ok {
			reason = c.reason
		} else if errors.Is(err, errUnreadable) {
			reason = UndoUnreadable
		}
		return passOver(reason, err)
	}
	if _, err := tx.ExecContext(ctx, "RELEASE take_back"); err != nil {
		return nil, nil, err
	}
	// The state the undo left, as read back (a food a meal uses is
	// archived rather than gone).
	after, err := stateNow(ctx, tx, userID, e)
	return after, nil, err
}

// belongsToChange reports whether err, from reading a change's entity or
// putting it back, is the change's: a clash with what came since, a rule
// of a table that its state breaks now, its own data that does not
// decode. Any other error is not: a failure of the database (a disk that
// is full, a table gone, a row that does not read), of the context, or a
// bug, which must not use up the user's changes unseen.
func belongsToChange(err error) bool {
	if _, ok := errors.AsType[clash](err); ok || errors.Is(err, errUnreadable) {
		return true
	}
	se, ok := errors.AsType[*sqlite.Error](err)
	return ok && se.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
}

// decode decodes state, one of a change's, into v.
func decode(state json.RawMessage, v any) error {
	if err := json.Unmarshal(state, v); err != nil {
		return fmt.Errorf("%w: %w", errUnreadable, err)
	}
	return nil
}

// entityID returns the id e's entity names.
func entityID(e event.Event) (uuid.UUID, error) {
	id, err := uuid.Parse(e.Entity)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("%w: entity %q: %w", errUnreadable, e.Entity, err)
	}
	return id, nil
}

// entityDay returns the day s names, an entity or the part of one before
// a metric's kind.
func entityDay(s string) (day.Date, error) {
	d, err := day.ParseDate(s)
	if err != nil {
		return day.Date{}, fmt.Errorf("%w: entity day %q: %w", errUnreadable, s, err)
	}
	return d, nil
}

// stateNow returns the state of e's entity now, as e's kind of event
// records it, to compare with the state e left.
func stateNow(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event) (any, error) {
	switch e.Action {
	case event.FoodSave, event.FoodArchive:
		return foodStateNow(ctx, tx, userID, e)
	case event.MealCreate, event.MealReplace, event.MealDelete:
		id, err := entityID(e)
		if err != nil {
			return nil, err
		}
		s, err := stateOf(ctx, tx, userID, id)
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return s, err
	case event.DayExclude, event.DayInclude:
		d, err := entityDay(e.Entity)
		if err != nil {
			return nil, err
		}
		return exclusionOf(ctx, tx, userID, d)
	case event.MetricsMerge, event.MetricSet, event.MetricDelete:
		return metricsStateNow(ctx, tx, userID, e)
	case event.Undo:
	}
	return nil, fmt.Errorf("no state for %q", e.Action)
}

// takeBack puts e's entity back as it was before e.
func takeBack(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	switch e.Action {
	case event.FoodSave, event.FoodArchive:
		return takeBackFood(ctx, tx, userID, e, now)
	case event.MealCreate, event.MealReplace, event.MealDelete:
		return takeBackMeal(ctx, tx, userID, e, now)
	case event.DayExclude, event.DayInclude:
		return takeBackDay(ctx, tx, userID, e, now)
	case event.MetricsMerge, event.MetricSet, event.MetricDelete:
		return takeBackMetrics(ctx, tx, userID, e, now)
	case event.Undo:
	}
	return fmt.Errorf("no way to take back %q", e.Action)
}
