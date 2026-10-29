// Package proposal defines a proposed change of the plan: the phases, the
// targets, the training weekdays and the user's limits as a coach or the
// user would leave them, each with its state before, why, and the days it
// was drawn from; and its life, from pending to applied, rejected, expired
// or superseded, and an applied one's revert. A change of the plan is a
// proposal the user consents to; applied ones are the plan's revisions
// (DESIGN.md §8). The limits a change must keep are another package's
// (limits.Validate).
package proposal

import (
	"errors"
	"fmt"
	"slices"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/text"
)

// Status is where a proposal is in its life.
type Status string

// Statuses.
const (
	// Pending waits for the user's consent, until it expires.
	Pending Status = "pending"
	// Applied is a revision of the plan.
	Applied Status = "applied"
	// Rejected is a proposal the user turned down.
	Rejected Status = "rejected"
	// Expired is a proposal left pending for TTL. It is not stored: a
	// pending proposal past its time is expired (StatusAt).
	Expired Status = "expired"
	// Superseded is a pending proposal the plan changed under, or a newer
	// proposal replaced: what it showed holds no more.
	Superseded Status = "superseded"
	// Reverted is an applied proposal a later one took back.
	Reverted Status = "reverted"
)

// TTL is how long a proposal waits for the user's consent.
const TTL = 7 * 24 * time.Hour

// MaxRationaleLen bounds why a proposal is made, in characters.
const MaxRationaleLen = 4000

// ChangedInTheApp is the rationale of the user's own change of the plan
// in the app, which nobody writes: a code, not words in a language, which
// the app says in the user's and a coach reads as it is
// (docs/conventions.md, Languages). The app takes a revision back with
// one of its own, "taken back in the app".
const ChangedInTheApp = "changed in the app"

// PhaseChange is a change of the phase that starts on Start: Before is
// the phase as it is, nil for none, and After as the change leaves it,
// nil for a delete.
type PhaseChange struct {
	Start         day.Date
	Before, After *plan.Phase
}

// TargetChange is a change of the target from day From: Before is the
// target as it is, nil for none, and After as the change leaves it, nil
// for a delete.
type TargetChange struct {
	From          day.Date
	Before, After *plan.Target
}

// WeekdaysChange is a change of the plan's training weekdays.
type WeekdaysChange struct {
	Before, After plan.Weekdays
}

// LimitChange is a change of the user's limit Name (limits.Fields):
// Before is the limit as it is, and After as the change leaves it. Only
// a web session applies it, as only a web session changes the limits
// (DESIGN.md §8).
type LimitChange struct {
	Name          string
	Before, After float64
}

// Changes are what a proposal changes of the plan. The proposer names
// what it leaves (After); the store fills in each Before from the plan
// as it is.
type Changes struct {
	Phases           []PhaseChange
	Targets          []TargetChange
	TrainingWeekdays *WeekdaysChange
	Limits           []LimitChange
}

// Validate reports every rule the changes break, joined into one error,
// today being the user's day, phases the timeline before them, in the
// order the phases start, and user the user's limits before them: each
// change is of the shape of what it changes, changes something, and
// keeps the past as it was. A target takes effect from today on; a
// phase starts from today on, unless it is an edit of the phase that has
// started, which keeps its type and every day it has had up to
// yesterday, and a phase that ended, on its planned end or as the next
// one started, stays as it is. A limit is one of limits.Fields, and the
// limits the changes leave keep their rules (limits.Limits.Validate).
// The timeline the changes leave is the store's to check (Timeline), and
// how the limits they leave stand to the instance's is Check's.
func (c Changes) Validate(today day.Date, phases []plan.Phase, user limits.Limits) error {
	if len(c.Phases) == 0 && len(c.Targets) == 0 && c.TrainingWeekdays == nil && len(c.Limits) == 0 {
		return errors.New("the proposal changes nothing")
	}
	var errs []error
	seen := map[day.Date]bool{}
	for _, ch := range c.Phases {
		if seen[ch.Start] {
			errs = append(errs, fmt.Errorf("the phase from %s twice", ch.Start))
			continue
		}
		seen[ch.Start] = true
		errs = append(errs, ch.validate(today, phases))
	}
	clear(seen)
	for _, ch := range c.Targets {
		if seen[ch.From] {
			errs = append(errs, fmt.Errorf("the target from %s twice", ch.From))
			continue
		}
		seen[ch.From] = true
		errs = append(errs, ch.validate(today))
	}
	if w := c.TrainingWeekdays; w != nil {
		if err := w.After.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("the training weekdays: %w", err))
		} else if w.After == w.Before {
			errs = append(errs, errors.New("the training weekdays are already so"))
		}
	}
	return errors.Join(append(errs, c.validateLimits(user))...)
}

// validateLimits reports the rules the changes of the limits break: a
// limit of no name, one changed twice or to what it is, and limits left
// that break theirs, user being the limits before them.
func (c Changes) validateLimits(user limits.Limits) error {
	if len(c.Limits) == 0 {
		return nil
	}
	var errs []error
	seen := map[string]bool{}
	for _, ch := range c.Limits {
		switch _, ok := limits.FieldNamed(ch.Name); {
		case !ok:
			errs = append(errs, fmt.Errorf("no limit %q", ch.Name))
		case seen[ch.Name]:
			errs = append(errs, fmt.Errorf("the limit %s twice", ch.Name))
		case ch.After == ch.Before:
			errs = append(errs, fmt.Errorf("the limit %s is already so", ch.Name))
		}
		seen[ch.Name] = true
	}
	if errs != nil {
		return errors.Join(errs...)
	}
	if err := c.LimitsOf(user).Validate(); err != nil {
		return fmt.Errorf("the limits it leaves: %w", err)
	}
	return nil
}

// LimitsOf returns the user's limits user as the changes leave them: each
// limit they change as it leaves it. A name of no limit changes nothing.
func (c Changes) LimitsOf(user limits.Limits) limits.Limits {
	for _, ch := range c.Limits {
		if f, ok := limits.FieldNamed(ch.Name); ok {
			f.Set(&user, ch.After)
		}
	}
	return user
}

// Loosened lists the limits the changes loosen, in their order: those
// whose value they leave looser than it was. Applying them takes the
// user's confirmation, as a change of the limits in a web session does.
func (c Changes) Loosened() []string {
	var out []string
	for _, ch := range c.Limits {
		if f, ok := limits.FieldNamed(ch.Name); ok && f.Looser(ch.After, ch.Before) {
			out = append(out, ch.Name)
		}
	}
	return out
}

func (ch PhaseChange) validate(today day.Date, phases []plan.Phase) error {
	b, a := ch.Before, ch.After
	switch {
	case a != nil && a.Validate() != nil:
		return fmt.Errorf("the phase from %s: %w", ch.Start, a.Validate())
	case a != nil && a.Start != ch.Start:
		return fmt.Errorf("the phase from %s starts on %s", ch.Start, a.Start)
	case b == nil && a == nil:
		return fmt.Errorf("no phase from %s to delete", ch.Start)
	case b != nil && a != nil && samePhase(*b, *a):
		return fmt.Errorf("the phase from %s is already so", ch.Start)
	case ch.Start.Before(today):
		return ch.validateStarted(today, phases)
	}
	return nil
}

// validateStarted reports a change of a phase that starts before today
// that changes the past: a new phase, a delete, a phase that ended, a
// type changed, or an end before yesterday.
func (ch PhaseChange) validateStarted(today day.Date, phases []plan.Phase) error {
	b, a := ch.Before, ch.After
	if b == nil {
		return fmt.Errorf("a new phase that starts on %s, before today %s", ch.Start, today)
	}
	switch end := endOf(*b, phases); {
	case !end.IsZero() && end.Before(today):
		return fmt.Errorf("the phase from %s ended on %s: the past stays as it was", ch.Start, end)
	case a == nil:
		return fmt.Errorf("the phase from %s has started: end it instead", ch.Start)
	case a.Type != b.Type:
		return fmt.Errorf("a %s phase from %s for the %s that has started: end it, and start another", a.Type, ch.Start, b.Type)
	case !a.End.IsZero() && a.End.Before(today.AddDays(-1)):
		return fmt.Errorf("the phase from %s ends on %s, before yesterday %s: the days it has had stay in it",
			ch.Start, a.End, today.AddDays(-1))
	}
	return nil
}

func (ch TargetChange) validate(today day.Date) error {
	b, a := ch.Before, ch.After
	switch {
	case a != nil && a.Validate() != nil:
		return fmt.Errorf("the target from %s: %w", ch.From, a.Validate())
	case a != nil && a.EffectiveFrom != ch.From:
		return fmt.Errorf("the target from %s takes effect on %s", ch.From, a.EffectiveFrom)
	case b == nil && a == nil:
		return fmt.Errorf("no target from %s to delete", ch.From)
	case b != nil && a != nil && *b == *a:
		return fmt.Errorf("the target from %s is already so", ch.From)
	case ch.From.Before(today):
		return fmt.Errorf("the target from %s, before today %s: the past keeps its targets", ch.From, today)
	}
	return nil
}

// endOf returns the last day of p on the timeline phases: its planned
// end, or the day before the next phase starts when that comes first;
// the zero Date for a phase open to no end.
func endOf(p plan.Phase, phases []plan.Phase) day.Date {
	end := p.End
	for _, q := range phases {
		if p.Start.Before(q.Start) {
			if next := q.Start.AddDays(-1); end.IsZero() || next.Before(end) {
				end = next
			}
			break
		}
	}
	return end
}

func samePhase(a, b plan.Phase) bool {
	same := func(x, y *float64) bool { return x == nil && y == nil || x != nil && y != nil && *x == *y }
	return a.Type == b.Type && a.Start == b.Start && a.End == b.End && a.RatePctWeek == b.RatePctWeek &&
		same(a.GoalWeight, b.GoalWeight) && same(a.MilestoneWeight, b.MilestoneWeight)
}

// Inverse returns the changes that take c back: each one's state before
// and after swapped.
func (c Changes) Inverse() Changes {
	var out Changes
	for _, ch := range c.Phases {
		out.Phases = append(out.Phases, PhaseChange{Start: ch.Start, Before: ch.After, After: ch.Before})
	}
	for _, ch := range c.Targets {
		out.Targets = append(out.Targets, TargetChange{From: ch.From, Before: ch.After, After: ch.Before})
	}
	if w := c.TrainingWeekdays; w != nil {
		out.TrainingWeekdays = &WeekdaysChange{Before: w.After, After: w.Before}
	}
	for _, ch := range c.Limits {
		out.Limits = append(out.Limits, LimitChange{Name: ch.Name, Before: ch.After, After: ch.Before})
	}
	return out
}

// Timeline returns phases, a timeline in the order the phases start, as
// the changes leave it, in the same order; nil for none.
func (c Changes) Timeline(phases []plan.Phase) []plan.Phase {
	byStart := map[day.Date]plan.Phase{}
	for _, p := range phases {
		byStart[p.Start] = p
	}
	for _, ch := range c.Phases {
		if ch.After == nil {
			delete(byStart, ch.Start)
		} else {
			byStart[ch.Start] = *ch.After
		}
	}
	var out []plan.Phase
	for _, p := range byStart {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b plan.Phase) int { return a.Start.Compare(b.Start) })
	return out
}

// Window is the days a proposal was drawn from, both included.
type Window struct {
	From, To day.Date
}

// Validate reports a window without its days, one that ends before it
// starts, or after today.
func (w Window) Validate(today day.Date) error {
	switch {
	case w.From.IsZero() || w.To.IsZero():
		return errors.New("a window needs its first and last day")
	case w.To.Before(w.From):
		return fmt.Errorf("the window ends on %s, before it starts on %s", w.To, w.From)
	case today.Before(w.To):
		return fmt.Errorf("the window ends on %s, after today %s", w.To, today)
	}
	return nil
}

// ValidateRationale reports a rationale that is missing, too long, or not
// text a person wrote (text.CheckLines).
func ValidateRationale(r string) error {
	return text.CheckLines("rationale", r, MaxRationaleLen, true)
}

// Draft is a proposal as it is made: why, the days it was drawn from, nil
// for none, and the changes, each as it would leave the plan (the store
// fills in each Before).
type Draft struct {
	Rationale string
	Window    *Window
	Changes   Changes
}

// Proposal is a proposed change of the plan, and where it is in its life.
type Proposal struct {
	ID uuid.UUID
	// Status is the status stored; StatusAt tells an expired one.
	Status Status
	// By is who proposed it: the user, through a client, and the model
	// that acted for them, when one did.
	By        event.Actor
	Rationale string
	// Window is the days it was drawn from, nil when none was given.
	Window  *Window
	Changes Changes
	// Reverts is the applied proposal whose changes this one takes back,
	// or uuid.Nil.
	Reverts   uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
	// DecidedAt is when it stopped pending, zero while it is, and
	// DecidedBy who applied it, rejected it, or changed the plan under it;
	// both zero for an expired one.
	DecidedAt time.Time
	DecidedBy event.Actor
	// RevertedBy is the proposal that took a reverted one back, or
	// uuid.Nil.
	RevertedBy uuid.UUID
}

// StatusAt returns p's status at now: a pending proposal past its time is
// expired.
func (p Proposal) StatusAt(now time.Time) Status {
	if p.Status == Pending && !now.Before(p.ExpiresAt) {
		return Expired
	}
	return p.Status
}
