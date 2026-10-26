package proposal

import (
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
// state now, before them, and targets the timeline of targets before
// them, in the order they take effect. It checks each phase the changes
// set, as a change of the current phase (s.Phase), and the target in
// force from each day they change a target from, against the one in
// force then before, with the training weekdays they leave; and, when
// they change the training weekdays and no target from today, the target
// in force today with them. A delete of a phase sets nothing to check.
// The data and the limits are those of now for a change ahead too: what
// the server knows when the user consents.
func (c Changes) Check(s limits.State, lv limits.Levels, targets []plan.Target) []Checked {
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
	var froms []day.Date
	for _, ch := range c.Targets {
		froms = append(froms, ch.From)
	}
	slices.SortFunc(froms, day.Date.Compare)
	if days != nil && !slices.Contains(froms, s.Today) {
		if ck := checkTarget(OfTrainingWeekdays, s.Today, s, lv, targets, after, days, training); ck.Target != nil {
			out = append(out, ck)
		}
	}
	for _, from := range froms {
		if ck := checkTarget(OfTarget, from, s, lv, targets, after, days, training); ck.Target != nil {
			out = append(out, ck)
		}
	}
	return out
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
