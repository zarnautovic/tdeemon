package proposal

import (
	"maps"
	"slices"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// Of is what a check of a proposal is of.
type Of string

// What a check is of.
const (
	OfPhase            Of = "phase"
	OfTarget           Of = "target"
	OfTrainingWeekdays Of = "training_weekdays"
)

// Checked is one check of a proposal's changes against the limits: of the
// phase it sets from Day, of the target in force from Day as it leaves
// it, or of the training weekdays alone, on today; with what
// limits.Validate found.
type Checked struct {
	Of  Of
	Day day.Date
	// Target is the target in force from Day as the changes leave it, nil
	// for a check of a phase or for none; TrainingDays the plan's
	// training weekdays as they leave them.
	Target       *plan.Target
	TrainingDays plan.Weekdays
	limits.Result
}

// Check holds the changes c to the limits (limits.Validate), s being the
// state now, before them, and phases and targets the timelines before
// them, in order. It checks each phase the changes set, as a change of
// the current phase (s.Phase), and the target in force from each day
// they change a target from, against the one in force then before, in
// the phase in force then as they leave it (a new phase starts at the
// trend weight now), with the training weekdays they leave; and, when
// they change the training weekdays, which move a split target's weekly
// average, the target in force today and each to come with them (a
// derived value the change sets, docs/limits.md). A delete of a phase
// sets nothing to check.
// The data and the limits are those of now for a change ahead too: what
// the server knows when the user consents.
func (c Changes) Check(s limits.State, lv limits.Levels, phases []plan.Phase, targets []plan.Target) []Checked {
	training := s.TrainingDays
	var days *plan.Weekdays
	if w := c.TrainingWeekdays; w != nil {
		days, training = &w.After, w.After
	}
	var out []Checked
	for _, ch := range c.Phases {
		if ch.After != nil {
			r := limits.Validate(limits.Change{Phase: ch.After, By: limits.ByPlan}, s, lv)
			out = append(out, Checked{Of: OfPhase, Day: ch.Start, TrainingDays: training, Result: r})
		}
	}
	after := c.TargetTimeline(targets)
	timeline := c.Timeline(phases)
	of := map[day.Date]Of{}
	if days != nil {
		// New training weekdays move the weekly average of the target in
		// force today and of each to come.
		of[s.Today] = OfTrainingWeekdays
		for _, t := range after {
			if s.Today.Before(t.EffectiveFrom) {
				of[t.EffectiveFrom] = OfTrainingWeekdays
			}
		}
	}
	for _, ch := range c.Targets {
		of[ch.From] = OfTarget
	}
	for _, d := range slices.SortedFunc(maps.Keys(of), day.Date.Compare) {
		if ck := checkTarget(of[d], d, inPhase(s, timeline, d), lv, targets, after, days, training); ck.Target != nil {
			out = append(out, ck)
		}
	}
	return out
}

// inPhase returns s in the phase of the timeline phases in force on d:
// the phase's start weight is s's for the phase s is in, else unknown,
// which the limits take as the trend weight now.
func inPhase(s limits.State, phases []plan.Phase, d day.Date) limits.State {
	ph, ok := plan.PhaseOn(phases, d)
	if !ok {
		s.Phase, s.PhaseStartTrendKg = nil, 0
		return s
	}
	if s.Phase == nil || s.Phase.Start != ph.Start || s.Phase.Type != ph.Type {
		s.PhaseStartTrendKg = 0
	}
	s.Phase = &ph
	return s
}

// checkTarget checks the target in force from d in after, with days, the
// training weekdays a change sets (training as they are then), against
// the one in force from d in before; one in force before alone is checked
// for the training weekdays.
func checkTarget(of Of, d day.Date, s limits.State, lv limits.Levels, before, after []plan.Target, days *plan.Weekdays, training plan.Weekdays) Checked {
	ck := Checked{Of: of, Day: d, TrainingDays: training}
	ch := limits.Change{TrainingDays: days, By: limits.ByPlan}
	s.Target = nil
	if was, ok := plan.TargetOn(before, d); ok {
		s.Target = &was
	}
	if now, ok := plan.TargetOn(after, d); ok {
		ck.Target = &now
		if s.Target == nil || *s.Target != now {
			ch.Target = &now
		}
	}
	ck.Result = limits.Validate(ch, s, lv)
	return ck
}

// TargetTimeline returns targets, a timeline in the order the targets
// take effect, as the changes leave it, in the same order; nil for none.
func (c Changes) TargetTimeline(targets []plan.Target) []plan.Target {
	byFrom := map[day.Date]plan.Target{}
	for _, t := range targets {
		byFrom[t.EffectiveFrom] = t
	}
	for _, ch := range c.Targets {
		if ch.After == nil {
			delete(byFrom, ch.From)
		} else {
			byFrom[ch.From] = *ch.After
		}
	}
	var out []plan.Target
	for _, t := range byFrom {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b plan.Target) int { return a.EffectiveFrom.Compare(b.EffectiveFrom) })
	return out
}
