package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"uuid"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
)

// undoLeaves lists the kinds of events UndoLast passes over, besides an
// import's: undos themselves, and the changes of the plan. The profile
// and the user's limits only a web session changes; the phases, the
// targets and the training weekdays must meet the limits as they are now,
// and go back as a revert, which limits.Validate checks; so do the
// proposals of them. The days of workouts only an import writes, and its
// next run replaces them. stateNow and takeBack have no way back for them.
func undoLeaves() []event.Action {
	return []event.Action{
		event.Undo, event.ProfileSet, event.LimitsSet, event.PhaseSet, event.PhaseDelete,
		event.TargetSet, event.TargetDelete, event.WeekdaysSet, event.WorkoutsSet,
		event.ProposalCreate, event.ProposalStatus,
	}
}

// ErrNothingToUndo reports that the user has no change left to undo.
var ErrNothingToUndo = errors.New("nothing to undo")

// ErrUndoID reports the id of a new undo out of form (clientid.Check).
var ErrUndoID = errors.New("invalid undo id")

// Change names a change by what it did and what it changed, as its event
// does: a client's undo of a change it made names it (UndoOf).
type Change struct {
	Action event.Action
	Entity string
}

// NotLatestError reports that the change an undo named is not the latest
// to take back: Latest is, made since through any client, or the named
// one is taken back already.
type NotLatestError struct {
	Latest event.Event
}

func (e *NotLatestError) Error() string {
	return fmt.Sprintf("the latest change is another: %s of %s", e.Latest.Action, e.Latest.Entity)
}

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
	// decode.
	UndoUnreadable UndoReason = "its state cannot be read"
	// UndoStuck: the state before breaks a rule of a table now.
	UndoStuck UndoReason = "it cannot be put back"
	// UndoPassedOver: the same undo, sent before, passed over the change
	// for one of the reasons above.
	UndoPassedOver UndoReason = "this undo passed over it before"
)

// clash is what a way back returns when putting the entity back would
// clash with what came since; what it wrote by then is rolled back to
// the savepoint.
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
// import's writes and undos themselves are skipped, and so are changes of
// the plan: the profile and the limits, which only a web session makes,
// and the phases, the targets and the training weekdays, which must meet
// the limits as they are now and go back as a new change of the plan, a
// revert, which limits.Validate checks), records the undo as
// an event of a's, and returns the event it took back, marked undone.
// Undoing again takes back the change before: undo walks back, it does
// not redo. When the entity is no longer as the change left it (an import
// wrote it, or a later change that undo passed over), putting it back
// would lose that; when putting it back clashes with what came since, or
// the change cannot be read or put back, it cannot be undone. Then
// UndoLast changes nothing, returns an *UndoConflictError, and passes
// over that change from then on. Any other failure (of the database, of
// ctx) is returned as it is and passes over nothing, so a later undo
// tries the same change again. With nothing left it returns
// ErrNothingToUndo.
func (db *DB) UndoLast(ctx context.Context, userID uuid.UUID, a event.Actor, now time.Time) (event.Event, error) {
	e, _, err := db.undo(ctx, userID, a, nil, nil, now)
	return e, err
}

// Undo is UndoLast of an undo under request, the id a client made for
// it, so the same undo sent again is known: the change it settled keeps
// the id, and the undo sent again returns that change, and true, when it
// was taken back, or an *UndoConflictError when it was passed over,
// rather than taking back the change before. A request no undo had must
// be a new id near now (clientid.Check), or ErrUndoID.
func (db *DB) Undo(ctx context.Context, userID uuid.UUID, a event.Actor, request uuid.UUID, now time.Time) (event.Event, bool, error) {
	return db.undo(ctx, userID, a, &request, nil, now)
}

// UndoOf is Undo of the change of, which the client means to take back
// (the web app's undo of a change it just made): it takes it back only
// while it is the latest to take back. When another is, made since
// through any client, it takes back nothing and records nothing, and
// returns a *NotLatestError that names the latest; the same request may
// be sent again.
func (db *DB) UndoOf(ctx context.Context, userID uuid.UUID, a event.Actor, request uuid.UUID, of Change, now time.Time) (event.Event, bool, error) {
	return db.undo(ctx, userID, a, &request, &of, now)
}

// undo is Undo under request, or UndoLast when request is nil; of the
// change of only, when of is not nil.
func (db *DB) undo(ctx context.Context, userID uuid.UUID, a event.Actor, request *uuid.UUID, of *Change, now time.Time) (event.Event, bool, error) {
	if err := a.Validate(); err != nil {
		return event.Event{}, false, fmt.Errorf("invalid actor: %w", err)
	}
	var undone event.Event
	var again, nothing bool
	var conflict *UndoConflictError
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var settled bool
		var err error
		if undone, settled, conflict, err = settledBy(ctx, tx, userID, request, now); err != nil || settled {
			again = settled && conflict == nil
			return err
		}
		undone, conflict, err = undoInTx(ctx, tx, userID, a, of, now)
		if errors.Is(err, ErrNothingToUndo) && request != nil {
			// The same undo sent again finds nothing again, rather than a
			// change made since.
			if err := recordNothing(ctx, tx, userID, a, *request, now); err != nil {
				return err
			}
			nothing = true
			return nil
		}
		if err != nil || request == nil {
			return err
		}
		// The change the undo settled, taken back or passed over.
		id := undone.ID
		if conflict != nil {
			id = conflict.Event.ID
		}
		_, err = tx.ExecContext(ctx, "UPDATE events SET undo_request = ? WHERE user_id = ? AND id = ?",
			request.String(), userID.String(), id.String())
		return err
	})
	return undoAnswer(undone, again, nothing, conflict, err)
}

// undoAnswer is what Undo returns of the outcome of its transaction:
// err, which a failure of the database wraps; the nothing to undo it
// recorded; the conflict it passed over; or the change it took back.
func undoAnswer(undone event.Event, again, nothing bool, conflict *UndoConflictError, err error) (event.Event, bool, error) {
	switch {
	case errors.Is(err, ErrNothingToUndo) || errors.Is(err, ErrUndoID):
		return event.Event{}, false, err
	case err != nil:
		return event.Event{}, false, fmt.Errorf("undo: %w", err)
	case nothing:
		return event.Event{}, false, ErrNothingToUndo
	}
	if conflict != nil {
		return event.Event{}, false, conflict
	}
	return undone, again, nil
}

// lastToUndo returns userID's latest change that undo can take back, or
// ErrNothingToUndo.
func lastToUndo(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (event.Event, error) {
	args := []any{userID.String()}
	for _, a := range undoLeaves() {
		args = append(args, string(a))
	}
	e, err := scanEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+` FROM events
		WHERE user_id = ? AND actor = 'user' AND operation NOT IN (?`+strings.Repeat(", ?", len(args)-2)+`)
			AND undone_by IS NULL AND stale = 0
		ORDER BY rowid DESC LIMIT 1`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return event.Event{}, ErrNothingToUndo
	}
	return e, err
}

// settledBy returns userID's change the undo under request settled, and
// whether there is one: with the conflict of one it passed over. A
// request no undo had must be a new id near now, or ErrUndoID; nil is
// none.
func settledBy(ctx context.Context, tx *sql.Tx, userID uuid.UUID, request *uuid.UUID, now time.Time) (event.Event, bool, *UndoConflictError, error) {
	if request == nil {
		return event.Event{}, false, nil, nil
	}
	e, err := scanEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM events WHERE user_id = ? AND undo_request = ?",
		userID.String(), request.String()))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := clientid.Check("undo", *request, now); err != nil {
			return event.Event{}, false, nil, fmt.Errorf("%w: %w", ErrUndoID, err)
		}
		return event.Event{}, false, nil, nil
	case err != nil:
		return event.Event{}, false, nil, err
	case e.Action == event.Undo:
		return event.Event{}, true, nil, ErrNothingToUndo
	case e.UndoneBy == uuid.Nil():
		return event.Event{}, true, &UndoConflictError{Event: e, Reason: UndoPassedOver}, nil
	}
	return e, true, nil, nil
}

// nothingUndone is the entity of the undo of a request that found nothing
// to take back.
const nothingUndone = "nothing"

// recordNothing records a's undo under request that found nothing to
// take back, so the same undo sent again finds nothing again.
func recordNothing(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, request uuid.UUID, now time.Time) error {
	id, err := record(ctx, tx, userID, a, event.Undo, nothingUndone, nil, nil, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE events SET undo_request = ? WHERE user_id = ? AND id = ?",
		request.String(), userID.String(), id.String())
	return err
}

// undoInTx takes back userID's latest change in tx, as UndoLast does, and
// returns it; or the conflict of one it passed over. When of is not nil,
// the latest must be that change, or it is a *NotLatestError.
func undoInTx(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, of *Change, now time.Time) (event.Event, *UndoConflictError, error) {
	e, err := lastToUndo(ctx, tx, userID)
	if err != nil {
		return event.Event{}, nil, err
	}
	if of != nil && (e.Action != of.Action || e.Entity != of.Entity) {
		return event.Event{}, nil, &NotLatestError{Latest: e}
	}
	after, conflict, err := takeBackChecked(ctx, tx, userID, e, now)
	if err != nil || conflict != nil {
		return event.Event{}, conflict, err
	}
	undoID, err := record(ctx, tx, userID, a, event.Undo, e.ID.String(), e.After, after, now)
	if err != nil {
		return event.Event{}, nil, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE events SET undone_by = ? WHERE user_id = ? AND id = ?",
		undoID.String(), userID.String(), e.ID.String()); err != nil {
		return event.Event{}, nil, err
	}
	e.UndoneBy = undoID
	return e, nil, nil
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

// DaysOf returns the days e's change was of, in order: a meal's day
// before and after it (two for a meal moved to another day), or the day
// of a check-in, an exclusion, a day's type or a measurement. A food and
// the notes are of no day, and neither is a state that does not read.
func DaysOf(e event.Event) []day.Date {
	var ds []day.Date
	switch e.Action {
	case event.MealCreate, event.MealReplace, event.MealDelete:
		for _, state := range []json.RawMessage{e.Before, e.After} {
			var s mealState
			if state != nil && decode(state, &s) == nil && !s.Meal.Day.IsZero() {
				ds = append(ds, s.Meal.Day)
			}
		}
	case event.DayExclude, event.DayInclude, event.DayCheckIn, event.TrainingDay,
		event.MetricsMerge, event.MetricSet, event.MetricDelete:
		dayPart, _, _ := strings.Cut(e.Entity, "/")
		if d, err := entityDay(dayPart); err == nil {
			ds = append(ds, d)
		}
	default:
	}
	slices.SortFunc(ds, day.Date.Compare)
	return slices.Compact(ds)
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
	case event.DayCheckIn:
		d, err := entityDay(e.Entity)
		if err != nil {
			return nil, err
		}
		return checkInOf(ctx, tx, userID, d)
	case event.MetricsMerge, event.MetricSet, event.MetricDelete:
		return metricsStateNow(ctx, tx, userID, e)
	case event.TrainingDay, event.NotesSet:
		return planStateNow(ctx, tx, userID, e)
	case event.ProfileSet, event.LimitsSet, event.PhaseSet, event.PhaseDelete, event.TargetSet, event.TargetDelete,
		event.WeekdaysSet, event.WorkoutsSet, event.ProposalCreate, event.ProposalStatus, event.Undo: // UndoLast never takes them back
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
	case event.DayCheckIn:
		return takeBackCheckIn(ctx, tx, userID, e, now)
	case event.MetricsMerge, event.MetricSet, event.MetricDelete:
		return takeBackMetrics(ctx, tx, userID, e, now)
	case event.TrainingDay, event.NotesSet:
		return takeBackPlan(ctx, tx, userID, e, now)
	case event.ProfileSet, event.LimitsSet, event.PhaseSet, event.PhaseDelete, event.TargetSet, event.TargetDelete,
		event.WeekdaysSet, event.WorkoutsSet, event.ProposalCreate, event.ProposalStatus, event.Undo:
	}
	return fmt.Errorf("no way to take back %q", e.Action)
}
