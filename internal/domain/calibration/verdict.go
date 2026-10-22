package calibration

import (
	"math"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

// Rates: how the measured rate stands to the phase's target rate.
const (
	// RateNone: there is no verdict; Verdict.Reasons say why.
	RateNone = "none"
	// RateOnTarget: within Params.TolerancePctWeek of the target.
	RateOnTarget = "on_target"
	// RateBelow: the weight falls faster, or rises slower, than the
	// target: a cut losing too fast, a bulk gaining too slowly.
	RateBelow = "below"
	// RateAbove: the weight falls slower, or rises faster: a cut losing
	// too slowly, a bulk gaining too fast.
	RateAbove = "above"
)

// Actions: what a verdict proposes.
const (
	ActionNone = "none"
	// ActionHold keeps the kcal: the rate is on target, or a guard holds
	// them.
	ActionHold = "hold"
	// ActionRaiseKcal and ActionLowerKcal move the weekly average toward
	// the target rate.
	ActionRaiseKcal = "raise_kcal"
	ActionLowerKcal = "lower_kcal"
	// ActionRaiseProteinOrKcal: the lean mass falls too fast for a cut
	// or a recomp to cut deeper.
	ActionRaiseProteinOrKcal = "raise_protein_or_kcal"
	// ActionShrinkSurplus: a bulk gains fat with the lean mass flat.
	ActionShrinkSurplus = "shrink_surplus"
	// ActionMaintain: a loss under way crossed a BMI of 18.5, which no
	// step deepens (docs/limits.md); maintenance is proposed.
	ActionMaintain = "maintain"
)

// Guards, which turn or hold a verdict's action, and flags, which the
// summary shows beside it.
const (
	GuardUnderweight = "underweight"
	GuardLeanMass    = "lean_mass"
	GuardFatGain     = "fat_gain"
	// GuardActivity: the last 7 days' steps differ from the days before
	// them by more than Params.StepsChange.
	GuardActivity = "activity"
	// FlagNoSteps: too few days with steps for the activity guard, which
	// is off.
	FlagNoSteps = "no_steps"
	// FlagMaintenanceDrift: the trend weight strayed more than
	// Params.DriftShare from the one the maintenance phase started with.
	FlagMaintenanceDrift = "maintenance_drift"
)

// Reasons there is no verdict.
const (
	ReasonNoPhase     = "no_phase"
	ReasonNoTarget    = "no_target"
	ReasonNoEstimate  = "no_estimate"
	ReasonBeforePhase = "before_phase"
	ReasonNotTrusted  = "not_trusted"
)

// leanFlatKgWeek is the most the lean mass may rise, in kg a week, and be
// flat for the fat-gain guard: a bulk gaining fat without it.
const leanFlatKgWeek = 0.1

// Verdict is calibration's verdict on the current phase (DESIGN.md §7):
// the measured rate against the phase's target, and what to do about it.
// It proposes and applies nothing; a coach turns it into a proposal, and
// the autopilot (P1) into a step.
type Verdict struct {
	// Rate is how the measured rate stands to the target (Rate*), and
	// Reasons why there is none.
	Rate    string
	Reasons []string
	// MeasuredPctWeek and TargetPctWeek are the rates, in % of the trend
	// weight a week, a minus being a loss, and TolerancePctWeek how far
	// apart they may be.
	MeasuredPctWeek, TargetPctWeek, TolerancePctWeek float64
	// Action is what the verdict proposes (Action*), and Guards the
	// guards that turned or held it.
	Action string
	Guards []string
	// GapKcal is the change of the weekly average, in kcal a day, that
	// would bring the measured rate to the target, more than 0 to eat
	// more: what a coach works with. StepKcal is the step toward it that
	// the autopilot's limits allow, the lesser of the user's step_kcal
	// and step_pct of the weekly average, and the one a guard's action
	// takes; 0 when the action keeps the kcal.
	GapKcal, StepKcal float64
	// Flags are what the summary shows beside the verdict (Flag*).
	Flags []string
}

// Judge returns the verdict on the phase ph, whose target is t and
// training weekdays training, from e, the estimate Calibrate gave on the
// day before today. lim are the user's limits, as they hold
// (limits.Effective), for the step, and pr the profile, for the BMI.
//
// A verdict needs a phase with a target and a trusted estimate of its
// own days, not one from before it. Its guards come in order, the first
// that trips taking the action: a loss below a BMI of 18.5, lean mass
// lost on a cut or a recomp, fat gained on a bulk, and a change of
// activity.
func Judge(e Estimate, ph *plan.Phase, t *plan.Target, training plan.Weekdays, pr profile.Profile, lim limits.Limits, p Params) Verdict {
	v := Verdict{Rate: RateNone, Action: ActionNone, Reasons: reasons(e, ph, t)}
	if len(v.Reasons) > 0 {
		return v
	}
	v.MeasuredPctWeek = plan.RatePctWeek(e.KgPerWeek, e.TrendKg)
	v.TargetPctWeek, v.TolerancePctWeek = ph.RatePctWeek, p.TolerancePctWeek
	// The kcal a day that move the rate by the gap: its kg a week at the
	// trend weight, KcalPerKg a kg, over 7 days.
	v.GapKcal = (v.TargetPctWeek - v.MeasuredPctWeek) / 100 * e.TrendKg * plan.KcalPerKg / 7
	step := min(lim.StepKcal, lim.StepPct/100*t.WeeklyAverage(training.Count()).Kcal)
	switch {
	case v.MeasuredPctWeek < v.TargetPctWeek-v.TolerancePctWeek:
		v.Rate, v.Action, v.StepKcal = RateBelow, ActionRaiseKcal, min(v.GapKcal, step)
	case v.MeasuredPctWeek > v.TargetPctWeek+v.TolerancePctWeek:
		v.Rate, v.Action, v.StepKcal = RateAbove, ActionLowerKcal, max(v.GapKcal, -step)
	default:
		v.Rate, v.Action = RateOnTarget, ActionHold
	}
	v.guard(e, ph, pr, step, p)
	v.flag(e, ph, p)
	return v
}

// reasons lists why there is no verdict on ph from e.
func reasons(e Estimate, ph *plan.Phase, t *plan.Target) []string {
	var out []string
	if ph == nil {
		out = append(out, ReasonNoPhase)
	}
	if t == nil {
		out = append(out, ReasonNoTarget)
	}
	switch {
	case !e.Measured:
		out = append(out, ReasonNoEstimate)
	case e.BeforePhase:
		out = append(out, ReasonBeforePhase)
	case !e.Trusted:
		out = append(out, ReasonNotTrusted)
	}
	return out
}

// guard applies the first guard that trips, in their order, to the
// action, a step being step.
func (v *Verdict) guard(e Estimate, ph *plan.Phase, pr profile.Profile, step float64, p Params) {
	b := e.Body
	loss := v.MeasuredPctWeek < 0
	switch {
	case loss && pr.HeightCm > 0 && pr.BMI(e.TrendKg) < limits.UnderweightBMI:
		// Maintenance: the gap to a rate of 0, for a coach to propose; no
		// step deepens the loss, and none is the autopilot's to take.
		v.trip(GuardUnderweight, ActionMaintain, 0)
		v.GapKcal = -v.MeasuredPctWeek / 100 * e.TrendKg * plan.KcalPerKg / 7
	case (ph.Type == plan.Cut || ph.Type == plan.Recomp) && b.LeanMeasured && b.LeanKgPerWeek <= -p.LeanLossKgWeek:
		v.trip(GuardLeanMass, ActionRaiseProteinOrKcal, step)
	case ph.Type == plan.Bulk && b.BodyFatMeasured && b.LeanMeasured &&
		b.BodyFatPerWeek-tNormal*b.BodyFatPerWeekSE > 0 && b.LeanKgPerWeek < leanFlatKgWeek:
		v.trip(GuardFatGain, ActionShrinkSurplus, -step)
	case b.StepsBefore > 0 && math.Abs(b.StepsRecent/b.StepsBefore-1) > p.StepsChange:
		v.trip(GuardActivity, ActionHold, 0)
	}
}

// trip sets the action a guard takes, with its step.
func (v *Verdict) trip(guard, action string, step float64) {
	v.Guards = append(v.Guards, guard)
	v.Action, v.StepKcal = action, step
}

// flag adds what the summary shows beside the verdict.
func (v *Verdict) flag(e Estimate, ph *plan.Phase, p Params) {
	if e.Body.StepsBefore == 0 {
		v.Flags = append(v.Flags, FlagNoSteps)
	}
	if ph.Type == plan.Maintain && e.PhaseStartTrendKg > 0 && math.Abs(e.TrendKg/e.PhaseStartTrendKg-1) > p.DriftShare {
		v.Flags = append(v.Flags, FlagMaintenanceDrift)
	}
}

// StateOf returns what the limits read of the data on today
// (limits.State), from e, the estimate Calibrate gave on the day before:
// the trend weight now and when the phase ph started, the measured TDEE
// and whether it is trusted, which a TDEE from before the phase may be,
// the fat-free mass and the active energy of the last 7 days, and the
// days logged and weighed in them. The autopilot's own history, its last
// step and drift, is the autopilot's (P1).
func StateOf(e Estimate, today day.Date, pr profile.Profile, ph *plan.Phase, t *plan.Target, training plan.Weekdays) limits.State {
	s := limits.State{
		Today: today, Profile: pr, TrendKg: e.TrendKg,
		Phase: ph, PhaseStartTrendKg: e.PhaseStartTrendKg, Target: t, TrainingDays: training,
		LeanMassKg: e.Now.LeanMassKg, ActiveKcal: e.Now.ActiveKcal,
		Autopilot: limits.Autopilot{LoggedDays: e.Now.LoggedDays, WeighIns: e.Now.WeighIns},
	}
	if e.Measured {
		s.TDEE, s.TDEETrusted = e.TDEE, e.Trusted
	}
	if ph != nil {
		s.Autopilot.DaysSincePhaseChange = today.DaysSince(ph.Start)
	}
	return s
}
