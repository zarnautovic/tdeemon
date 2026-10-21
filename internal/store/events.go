package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
)

// ErrNothingToUndo reports that the user has no change left to undo.
var ErrNothingToUndo = errors.New("nothing to undo")

// errUndoBlocked is what a way back returns, before it writes anything,
// when putting the entity back would clash with what came since (another
// food took the name): UndoLast reports it as a conflict.
var errUndoBlocked = errors.New("undo blocked")

// UndoConflictError reports that the latest change could not be taken
// back: the entity changed since (an import wrote it), and putting it back
// would lose that. The change is passed over from then on, so the next
// undo takes back the one before it.
type UndoConflictError struct {
	Event event.Event
}

func (e *UndoConflictError) Error() string {
	return fmt.Sprintf("%s of %s cannot be undone: it changed since", e.Event.Action, e.Event.Entity)
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
// wrote it since), putting it back would lose that: UndoLast changes
// nothing, returns an *UndoConflictError, and passes over that change
// from then on. With nothing left it returns ErrNothingToUndo.
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
		current, err := stateNow(ctx, tx, userID, e)
		if err != nil {
			return fmt.Errorf("state of %s: %w", e.Entity, err)
		}
		if c, err := stateJSON(current); err != nil {
			return err
		} else if want, _ := stateJSON(e.After); c != want {
			conflict = &UndoConflictError{Event: e}
			_, err := tx.ExecContext(ctx, "UPDATE events SET stale = 1 WHERE user_id = ? AND id = ?", userID.String(), e.ID.String())
			return err
		}
		err = takeBack(ctx, tx, userID, e, now)
		if errors.Is(err, errUndoBlocked) {
			conflict = &UndoConflictError{Event: e}
			_, err := tx.ExecContext(ctx, "UPDATE events SET stale = 1 WHERE user_id = ? AND id = ?", userID.String(), e.ID.String())
			return err
		}
		if err != nil {
			return fmt.Errorf("take back %s of %s: %w", e.Action, e.Entity, err)
		}
		undoID, err := record(ctx, tx, userID, a, event.Undo, e.ID.String(), e.After, e.Before, now)
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

// stateNow returns the state of e's entity now, as e's kind of event
// records it, to compare with the state e left.
func stateNow(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event) (any, error) {
	switch e.Action {
	case event.FoodSave, event.FoodArchive:
		return foodStateNow(ctx, tx, userID, e)
	case event.MealCreate, event.MealReplace, event.MealDelete:
		id, err := uuid.Parse(e.Entity)
		if err != nil {
			return nil, err
		}
		s, err := stateOf(ctx, tx, userID, id)
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return s, err
	case event.DayExclude, event.DayInclude:
		d, err := day.ParseDate(e.Entity)
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
