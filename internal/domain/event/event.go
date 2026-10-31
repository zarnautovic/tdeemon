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
	// Autopilot is the server's weekly review: its steps, proposals and
	// reviews are recorded, but undo leaves them; the plan's history
	// takes a step back.
	Autopilot ActorKind = "autopilot"
)

// ClientAutopilot is the client of the autopilot's writes.
const ClientAutopilot = "autopilot"

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
	if a.Kind != User && a.Kind != Import && a.Kind != Autopilot {
		errs = append(errs, fmt.Errorf("actor %q: want user, import or autopilot", a.Kind))
	}
	errs = append(errs, text.Check("client", a.Client, MaxClientLen, true), text.Check("model", a.Model, MaxModelLen, false))
	return errors.Join(errs...)
}

// Action is what a write did.
type Action string

// Actions.
const (
	FoodSave    Action = "food.save"
	FoodArchive Action = "food.archive"
	// RecipeSave saved a recipe whole: its food and its ingredients.
	RecipeSave   Action = "recipe.save"
	MealCreate   Action = "meal.create"
	MealReplace  Action = "meal.replace"
	MealDelete   Action = "meal.delete"
	DayExclude   Action = "day.exclude"
	DayInclude   Action = "day.include"
	MetricsMerge Action = "metrics.merge"
	MetricSet    Action = "metric.set"
	MetricDelete Action = "metric.delete"
	// DayCheckIn changed what the user says of a day at once: the mood,
	// hunger and note, the toggle of its type and its exclusion.
	DayCheckIn Action = "day.checkin"
	// WorkoutsSet replaced the days a source recorded a workout on; undo
	// passes over it, as the source's next export replaces them.
	WorkoutsSet Action = "workouts.set"
	// The changes of the plan, which undo leaves: the profile and the
	// limits only a web session changes, and the phases, the targets and
	// the training weekdays go back as a revert that limits.Validate
	// checks.
	ProfileSet   Action = "profile.set"
	LimitsSet    Action = "limits.set"
	PhaseSet     Action = "phase.set"
	PhaseDelete  Action = "phase.delete"
	TargetSet    Action = "target.set"
	TargetDelete Action = "target.delete"
	TrainingDay  Action = "day.training"
	WeekdaysSet  Action = "plan.weekdays"
	NotesSet     Action = "notes.set"
	// A proposed change of the plan made, and one that stopped pending
	// (applied, rejected, superseded) or was reverted; undo leaves them,
	// as an applied one goes back as a revert.
	ProposalCreate Action = "proposal.create"
	ProposalStatus Action = "proposal.status"
	// AutopilotSet changed the user's settings of the autopilot, and
	// AutopilotReview recorded a weekly review; undo leaves both.
	AutopilotSet    Action = "autopilot.set"
	AutopilotReview Action = "autopilot.review"
	// Undo took another event back; its Entity is that event's id.
	Undo Action = "undo"
)

// Event is one write, as recorded.
type Event struct {
	ID     uuid.UUID
	At     time.Time
	Actor  Actor
	Action Action
	// Entity names what changed: a food's (a recipe's too), a meal's or a
	// proposal's id, a day (YYYY-MM-DD), a day and a metric
	// ("2026-10-21/weight"), a phase by its start or a target by its
	// first day (YYYY-MM-DD), "profile", "limits", "plan" (the training
	// weekdays), "notes", "autopilot" (its settings), a review by its day
	// ("review/2026-11-02") or a source of workouts ("strong"), or for an
	// undo the event undone, or "nothing" for an undo under a client's id
	// that found nothing to take back.
	Entity string
	// Before and After are the state of the entity, as JSON; nil where
	// there was none (before a create, after a delete).
	Before, After json.RawMessage
	// UndoneBy is the undo that took the event back, or uuid.Nil.
	UndoneBy uuid.UUID
}
