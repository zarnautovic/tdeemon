package autopilot

import (
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// By is who applied a revision of the plan, as the autopilot's history
// tells them apart.
type By int

// Who applies a revision.
const (
	// ByUser is a revision the user applied: their own change, a coach's
	// proposal they accepted, or a revert, which takes back a step too.
	ByUser By = iota
	// ByAutopilot is a step the autopilot applied by itself.
	ByAutopilot
	// ByAccepted is an autopilot's proposal the user accepted: a target
	// they agreed to, of a step's size.
	ByAccepted
)

// Revision is an applied change of the plan, as the autopilot's history
// reads it.
type Revision struct {
	// Day is the user's day it was applied on.
	Day day.Date
	By  By
	// SetsKcal is true when it changed the weekly average of the target
	// in force, and DeltaKcal by how much, a minus being less.
	SetsKcal  bool
	DeltaKcal float64
	// ChangesPhase is true when it started a phase or changed the type or
	// the rate of one.
	ChangesPhase bool
	// TDEE is the TDEE calibration trusted when it was applied, 0 for
	// none: the base of the drift from a target the user agreed to, or
	// from the first step after one that had none.
	TDEE float64
}

// History is what the autopilot's past says of the next step: its waits,
// and its drift from the last kcal target the user agreed to
// (docs/limits.md, "Autopilot").
type History struct {
	// WaitFrom is the day the wait for a first step started: the
	// phase's start, a change of its type or rate, or a kcal target the
	// user set (a revert included).
	WaitFrom day.Date
	// LastStep is the day of the autopilot's last step since, the zero
	// Date for none: an accepted proposal of its own counts, as one of a
	// step's size.
	LastStep day.Date
	// AgreedOn is the day of the last kcal target the user agreed to:
	// the phase's start, or a revision of theirs that set the kcal.
	AgreedOn day.Date
	// DriftKcal is the net change of the weekly average by the
	// autopilot's steps since, a step up and a step down cancelling.
	DriftKcal float64
	// BaseTDEE is the TDEE the drift is a share of: the one trusted when
	// the user agreed to the target, else the one at the autopilot's
	// first step since, frozen until the next agreed target; 0 until
	// there is one. BaseFrom is the day it was taken.
	BaseTDEE float64
	BaseFrom day.Date
}

// HistoryOf reads the autopilot's history from the revisions of the
// plan, in the order they were applied, in the phase ph, the current one.
// Revisions before the phase started are of another phase: the phase's
// start is an agreed target and starts the wait.
func HistoryOf(revisions []Revision, ph plan.Phase) History {
	h := History{WaitFrom: ph.Start, AgreedOn: ph.Start}
	for _, r := range revisions {
		if r.Day.Before(ph.Start) {
			continue
		}
		switch r.By {
		case ByAutopilot:
			if !r.SetsKcal {
				continue
			}
			h.DriftKcal += r.DeltaKcal
			h.LastStep = r.Day
			if h.BaseTDEE <= 0 && r.TDEE > 0 {
				h.BaseTDEE, h.BaseFrom = r.TDEE, r.Day
			}
		case ByAccepted:
			if r.SetsKcal {
				h = h.agree(r)
				h.LastStep = r.Day
			}
		case ByUser:
			if r.SetsKcal {
				h = h.agree(r)
				h.WaitFrom = r.Day
			}
			if r.ChangesPhase {
				h.WaitFrom = r.Day
			}
		}
	}
	return h
}

// agree makes r the target the user last agreed to: the drift starts
// again at 0, from the TDEE trusted then.
func (h History) agree(r Revision) History {
	h.AgreedOn, h.DriftKcal = r.Day, 0
	h.BaseTDEE, h.BaseFrom = 0, day.Date{}
	if r.TDEE > 0 {
		h.BaseTDEE, h.BaseFrom = r.TDEE, r.Day
	}
	return h
}

// Of returns what the limits read of the history on today
// (limits.Autopilot), with the days logged and the weigh-ins of the last
// 7 days, an excluded day counting as not logged, and the TDEE trusted
// now, which is the base of the drift until there is one: the step
// about to come would be the first.
func (h History) Of(today day.Date, loggedDays, weighIns int, tdee float64) limits.Autopilot {
	a := limits.Autopilot{
		DaysSincePhaseChange: today.DaysSince(h.WaitFrom),
		LoggedDays:           loggedDays, WeighIns: weighIns,
		DriftKcal: h.DriftKcal, DriftBaseTDEE: h.BaseTDEE,
	}
	if !h.LastStep.IsZero() {
		a.DaysSinceStep = new(today.DaysSince(h.LastStep))
	}
	if a.DriftBaseTDEE <= 0 {
		a.DriftBaseTDEE = tdee
	}
	return a
}

// WaitsUntil returns the first day the waits of l let a step come: 14
// days (FirstStepDays) from WaitFrom, and 7 (IntervalDays) from the last
// step.
func (h History) WaitsUntil(l limits.Limits) day.Date {
	d := h.WaitFrom.AddDays(int(l.FirstStepDays))
	if !h.LastStep.IsZero() {
		if next := h.LastStep.AddDays(int(l.IntervalDays)); d.Before(next) {
			d = next
		}
	}
	return d
}
