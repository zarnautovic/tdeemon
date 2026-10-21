package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
)

// ErrNothingToUndo reports that the user has no change left to undo.
var ErrNothingToUndo = errors.New("nothing to undo")

// MaxEvents bounds Events.
const MaxEvents = 100

// record writes the event of a write in its transaction tx. before and
// after are the entity's state, marshalled as JSON (json.RawMessage as it
// is); nil, or a nil pointer, records none.
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
	id := uuid.NewV7()
	_, err = tx.ExecContext(ctx, `INSERT INTO events
		(id, user_id, at, actor, client, model, operation, entity, before_state, after_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), userID.String(), formatTime(now), string(a.Kind), a.Client, a.Model, string(action), entity, b, af)
	return id, err
}

// stateJSON returns v as JSON text, or nil for no state.
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
	rows, err := db.read.QueryContext(ctx, "SELECT "+eventColumns+" FROM events WHERE user_id = ? ORDER BY id DESC LIMIT ?",
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
// not redo. Changes are taken back newest first, so the entity is as the
// change left it, an import aside. With nothing left it returns
// ErrNothingToUndo.
func (db *DB) UndoLast(ctx context.Context, userID uuid.UUID, a event.Actor, now time.Time) (event.Event, error) {
	if err := a.Validate(); err != nil {
		return event.Event{}, fmt.Errorf("invalid actor: %w", err)
	}
	var undone event.Event
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		e, err := scanEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+` FROM events
			WHERE user_id = ? AND actor = 'user' AND operation <> 'undo' AND undone_by IS NULL
			ORDER BY id DESC LIMIT 1`, userID.String()))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNothingToUndo
		}
		if err != nil {
			return err
		}
		if err := takeBack(ctx, tx, userID, e, now); err != nil {
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
	return undone, nil
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
