// Package event defines the record every write leaves: who made it,
// through what, what it did, and the state before and after, so a change
// can be traced and the user's last one undone.
package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/text"
)

// ActorKind is who made a write.
type ActorKind string

// Actor kinds.
const (
	// User is the user, through any client, a model acting for them
	// included: their changes are what undo takes back.
	User ActorKind = "user"
	// Import is a device or an app sending data on its own (Health Auto
	// Export): its writes are recorded, but undo leaves them.
	Import ActorKind = "import"
)

// Limits on an actor's names.
const (
	MaxClientLen = 50
	MaxModelLen  = 100
)

// Actor is who made a write and through what.
type Actor struct {
	Kind ActorKind
	// Client is what the write came through: "mcp", "web", "telegram",
	// "hae".
	Client string
	// Model is the AI model that acted for the user ("claude-opus-5-5"),
	// when one did.
	Model string
}

// Validate reports an actor of an unknown kind, or without a client.
func (a Actor) Validate() error {
	var errs []error
	if a.Kind != User && a.Kind != Import {
		errs = append(errs, fmt.Errorf("actor %q: want user or import", a.Kind))
	}
	errs = append(errs, text.Check("client", a.Client, MaxClientLen, true), text.Check("model", a.Model, MaxModelLen, false))
	return errors.Join(errs...)
}

// Action is what a write did.
type Action string

// Actions.
const (
	FoodSave     Action = "food.save"
	FoodArchive  Action = "food.archive"
	MealCreate   Action = "meal.create"
	MealReplace  Action = "meal.replace"
	MealDelete   Action = "meal.delete"
	DayExclude   Action = "day.exclude"
	DayInclude   Action = "day.include"
	MetricsMerge Action = "metrics.merge"
	MetricSet    Action = "metric.set"
	MetricDelete Action = "metric.delete"
	// Undo took another event back; its Entity is that event's id.
	Undo Action = "undo"
)

// Event is one write, as recorded.
type Event struct {
	ID     uuid.UUID
	At     time.Time
	Actor  Actor
	Action Action
	// Entity names what changed: a food's or a meal's id, a day
	// (YYYY-MM-DD), a day and a metric ("2026-10-21/weight"), or for an
	// undo the event undone.
	Entity string
	// Before and After are the state of the entity, as JSON; nil where
	// there was none (before a create, after a delete).
	Before, After json.RawMessage
	// UndoneBy is the undo that took the event back, or uuid.Nil.
	UndoneBy uuid.UUID
}
