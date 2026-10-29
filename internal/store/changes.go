package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// MaxRecentChanges bounds RecentChanges.
const MaxRecentChanges = 50

// RecentChange is one of a user's latest changes (RecentChanges): its
// event, what it was of by the names the user's data has, and whether
// undo takes it back next.
type RecentChange struct {
	Event event.Event
	// Name is a meal's description and Items its items' names, as the
	// change left the meal (as it was, for a delete); or a food's name
	// and Brand its brand, as the change left it (as it is now, for one
	// archived or brought back, whose change records no name).
	Name, Brand string
	Items       []string
	// Metric is a measurement's kind.
	Metric metric.Kind
	// Archived tells a food archived from one brought back in use.
	Archived bool
	// Status is a proposal's status after the change.
	Status string
	// Next tells the change undo takes back next (UndoLast).
	Next bool
}

// RecentChanges returns up to limit (MaxRecentChanges at most) of
// userID's latest changes, newest first: those the user made, through
// any client, but the undos, which mark the change each took back
// (Event.UndoneBy). An import's writes are left out: undo leaves them,
// and a device sends them often enough to bury the user's own.
func (db *DB) RecentChanges(ctx context.Context, userID uuid.UUID, limit int) ([]RecentChange, error) {
	if limit <= 0 {
		return nil, nil
	}
	var out []RecentChange
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		next, err := lastToUndo(ctx, tx, userID)
		if err != nil && !errors.Is(err, ErrNothingToUndo) {
			return err
		}
		rows, err := tx.QueryContext(ctx, "SELECT "+eventColumns+` FROM events
			WHERE user_id = ? AND actor = ? AND operation <> ?
			ORDER BY rowid DESC LIMIT ?`,
			userID.String(), string(event.User), string(event.Undo), min(limit, MaxRecentChanges))
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			e, err := scanEvent(rows)
			if err != nil {
				return err
			}
			out = append(out, RecentChange{Event: e, Next: e.ID == next.ID})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range out {
			if err := nameChange(ctx, tx, userID, &out[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("recent changes: %w", err)
	}
	return out, nil
}

// nameChange gives c the names of what its change was of, from its
// states. A state that does not read leaves them out: the change is
// listed all the same, as undo would pass it over.
func nameChange(ctx context.Context, tx *sql.Tx, userID uuid.UUID, c *RecentChange) error {
	e := c.Event
	switch e.Action {
	case event.MealCreate, event.MealReplace, event.MealDelete:
		var s mealState
		if readState(e, &s) {
			c.Name = s.Meal.Description
			for i := range s.Meal.Items {
				c.Items = append(c.Items, s.Meal.Items[i].Name)
			}
		}
	case event.FoodSave:
		var f food.Food
		if readState(e, &f) {
			c.Name, c.Brand = f.Name, f.Brand
		}
	case event.FoodArchive:
		var s archivedState
		if readState(e, &s) {
			c.Archived = s.Archived
		}
		return nameFood(ctx, tx, userID, c)
	case event.MetricSet, event.MetricDelete:
		if _, kind, ok := strings.Cut(e.Entity, "/"); ok {
			c.Metric = metric.Kind(kind)
		}
	case event.ProposalStatus:
		var s proposalState
		if readState(e, &s) {
			c.Status = s.Status
		}
	default:
	}
	return nil
}

// readState decodes into v the state e left, or the one before it for a
// change that left none (a delete), and tells whether it read.
func readState(e event.Event, v any) bool {
	state := e.After
	if state == nil {
		state = e.Before
	}
	return state != nil && decode(state, v) == nil
}

// nameFood gives c, of a food archived or brought back, the food's name
// and brand now: the change records neither. A food gone since (its
// save undone) has none, and so has an entity that is no food's id.
func nameFood(ctx context.Context, tx *sql.Tx, userID uuid.UUID, c *RecentChange) error {
	if id, err := uuid.Parse(c.Event.Entity); err == nil {
		err := tx.QueryRowContext(ctx, "SELECT name, brand FROM foods WHERE user_id = ? AND id = ?",
			userID.String(), id.String()).Scan(&c.Name, &c.Brand)
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}
