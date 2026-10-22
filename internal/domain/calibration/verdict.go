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
	// ActionHold keeps the kcal: the target gives the target rate with
	// the measured TDEE, though what was eaten may have missed it
	// (Verdict.IntakeKcal against TargetKcal), or a guard holds them.
	ActionHold = "hold"
	// ActionRaiseKcal and ActionLowerKcal move the target's weekly
	// average to the one that gives the target rate.
	ActionRaiseKcal = "raise_kcal"
	ActionLowerKcal = "lower_kcal"
	// ActionRaiseProteinOrKcal: the lean mass falls too fast for a cut
	// or a recomp to cut deeper.
	ActionRaiseProteinOrKcal = "raise_protein_or_kcal"
	// ActionShrinkSurplus: a bulk gains fat with the lean mass flat; the
	// surplus over the TDEE shrinks, by a step at most.
	ActionShrinkSurplus = "shrink_surplus"
	// ActionMaintain: a loss under way below a BMI of 18.5, which no step
	// deepens (docs/limits.md); maintenance is proposed.
	ActionMaintain = "maintain"
)

// Guards, which turn or hold a verdict's action, and flags, which the
// summary shows beside it.
const (
	GuardUnderweight = "underweight"
	GuardLeanMass    = "lean_mass"
	GuardFatGain     = "fat_gain"
	// GuardActivity: the last 7 days' steps differ from the days before
	// them by more than Params.StepsChange, the way that explains the
	// measured rate's miss the action would correct.
	GuardActivity = "activity"
	// FlagUnderweight: the plan loses weight below a BMI of 18.5. It and
	// FlagNoHeight and FlagMaintenanceDrift show with a verdict or
	// without.
	FlagUnderweight = "underweight"
	// FlagNoHeight: the profile has no height, so the underweight guard
	// is off.
	FlagNoHeight = "no_height"
	// FlagMaintenanceDrift: the trend weight strayed more than
	// Params.DriftShare from the one the maintenance phase started with.
	FlagMaintenanceDrift = "maintenance_drift"
	// FlagNoSteps: too few days with steps for the activity guard, which
	// is off. It and the flags below show with a verdict only, of the
	// guards the phase has.
	FlagNoSteps = "no_steps"
	// FlagNoLeanMass: the lean mass is not measured enough, recently, for
	// the lean-mass guard of a cut or a recomp, or the fat-gain guard of a
	// bulk, which are off.
	FlagNoLeanMass = "no_lean_mass"
	// FlagNoBodyFat: the body fat is not measured enough for a bulk's
	// fat-gain guard, which is off.
	FlagNoBodyFat = "no_body_fat"
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
	// GapKcal is the change of the target's weekly average, in kcal a
	// day, the action asks for, more than 0 to eat more: what a coach
	// works with. For the rate it is the gap to the weekly average that
	// gives the target rate with the measured TDEE, and it holds within
	// the tolerance; for ActionMaintain the gap to the TDEE, never a cut;
	// for a guard's own action at least its step. StepKcal is the step
	// toward it that the autopilot's limits allow (limits.Limits.Step), 0
	// when the action keeps the kcal or is no step's to take. A guard's
	// action, from the window's body data, comes again while they stay
	// as they are; the autopilot's interval spaces its steps.
	GapKcal, StepKcal float64
	// IntakeKcal is what was eaten on average over the window, and
	// TargetKcal the target's weekly average now: a rate that misses with
	// a target that holds is the intake's.
	IntakeKcal, TargetKcal float64
	// Flags are what the summary shows beside the verdict (Flag*).
	Flags []string
}

// Judge returns the verdict on the phase ph, whose target is t and
// training weekdays training, from e, the estimate Calibrate gave on the
// day before today. lim are the user's limits, as they hold
// (limits.Effective), for the step, and pr the profile, for the BMI.
//
// A verdict needs a phase with a target and a trusted estimate of its
// own days, not one from before it. Its action moves the target, from
// the measured TDEE, so a user who eats beside a target that holds is
// told to hold. Its guards come in order, the first that trips taking the
// action: a loss under way below a BMI of 18.5, by the plan as the limits
// read it, lean mass lost on a cut or a recomp, fat gained on a bulk, and
// a change of activity.
func Judge(e Estimate, ph *plan.Phase, t *plan.Target, training plan.Weekdays, pr profile.Profile, lim limits.Limits, p Params) Verdict {
	v := Verdict{Rate: RateNone, Action: ActionNone, Reasons: reasons(e, ph, t)}
	underweight := v.flagPlan(e, ph, t, training, pr, p)
	if len(v.Reasons) > 0 {
		return v
	}
	v.MeasuredPctWeek = plan.RatePctWeek(e.KgPerWeek, e.TrendKg)
	v.TargetPctWeek, v.TolerancePctWeek = ph.RatePctWeek, p.TolerancePctWeek
	switch {
	case v.MeasuredPctWeek < v.TargetPctWeek-v.TolerancePctWeek-limits.Epsilon:
		v.Rate = RateBelow
	case v.MeasuredPctWeek > v.TargetPctWeek+v.TolerancePctWeek+limits.Epsilon:
		v.Rate = RateAbove
	default:
		v.Rate = RateOnTarget
	}
	avg := t.WeeklyAverage(training.Count()).Kcal
	v.IntakeKcal, v.TargetKcal = e.IntakeKcal, avg
	step := lim.Step(avg)
	// The weekly average that gives the target rate with the measured
	// TDEE, less the target's; the tolerance in kcal is the same rate's.
	v.GapKcal = e.TDEE + plan.KcalPerDay(v.TargetPctWeek, e.TrendKg) - avg
	switch tolerance := plan.KcalPerDay(v.TolerancePctWeek, e.TrendKg); {
	case v.GapKcal > tolerance+limits.Epsilon:
		v.Action, v.StepKcal = ActionRaiseKcal, min(v.GapKcal, step)
	case v.GapKcal < -tolerance-limits.Epsilon:
		v.Action, v.StepKcal = ActionLowerKcal, max(v.GapKcal, -step)
	default:
		v.Action, v.GapKcal = ActionHold, 0
	}
	v.guard(e, ph, underweight, avg, step, p)
	v.flagData(ph, e.Body)
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

// flagPlan adds the flags of the plan and the trend, which show with a
// verdict or without, and reports whether the plan loses weight below a
// BMI of 18.5, by what the limits read (StateOf, limits.LossUnderWay).
func (v *Verdict) flagPlan(e Estimate, ph *plan.Phase, t *plan.Target, training plan.Weekdays, pr profile.Profile, p Params) bool {
	var underweight bool
	switch {
	case pr.HeightCm <= 0:
		v.Flags = append(v.Flags, FlagNoHeight)
	case e.TrendKg > 0:
		if underweight = limits.UnderweightLoss(StateOf(e, day.Date{}, pr, ph, t, training)); underweight {
			v.Flags = append(v.Flags, FlagUnderweight)
		}
	}
	if ph != nil && ph.Type == plan.Maintain && e.TrendKg > 0 && e.PhaseStartTrendKg > 0 &&
		math.Abs(e.TrendKg/e.PhaseStartTrendKg-1) > p.DriftShare+limits.Epsilon {
		v.Flags = append(v.Flags, FlagMaintenanceDrift)
	}
	return underweight
}

// flagData adds the flags of the guards of ph the window's data leave
// off.
func (v *Verdict) flagData(ph *plan.Phase, b Body) {
	if b.StepsBefore == 0 {
		v.Flags = append(v.Flags, FlagNoSteps)
	}
	switch ph.Type {
	case plan.Cut, plan.Recomp, plan.Bulk:
		if !b.LeanMeasured {
			v.Flags = append(v.Flags, FlagNoLeanMass)
		}
	case plan.Maintain:
	}
	if ph.Type == plan.Bulk && !b.BodyFatMeasured {
		v.Flags = append(v.Flags, FlagNoBodyFat)
	}
}

// guard applies the first guard that trips, in their order, to the
// action, the target's weekly average being avg and a step step.
func (v *Verdict) guard(e Estimate, ph *plan.Phase, underweight bool, avg, step float64, p Params) {
	b := e.Body
	switch {
	case underweight:
		// Maintenance: the gap to the TDEE, for a coach to propose, but
		// never a cut; no step deepens the loss, and none is the
		// autopilot's to take.
		v.trip(GuardUnderweight, ActionMaintain, max(e.TDEE-avg, 0), 0)
	case (ph.Type == plan.Cut || ph.Type == plan.Recomp) && b.LeanMeasured &&
		b.LeanKgPerWeek <= -p.LeanLossKgWeek+limits.Epsilon && b.LeanKgPerWeek+b.LeanHalfWidth < 0:
		v.trip(GuardLeanMass, ActionRaiseProteinOrKcal, max(v.GapKcal, step), step)
	case ph.Type == plan.Bulk && b.BodyFatMeasured && b.LeanMeasured && avg > e.TDEE+limits.Epsilon &&
		b.BodyFatPerWeek-b.BodyFatHalfWidth > 0 && b.LeanKgPerWeek < leanFlatKgWeek-limits.Epsilon:
		// The surplus shrinks by a step at most, never below the TDEE.
		shrink := min(step, avg-e.TDEE)
		v.trip(GuardFatGain, ActionShrinkSurplus, min(v.GapKcal, -shrink), -shrink)
	case b.StepsBefore > 0 && v.activityExplains(b.StepsRecent/b.StepsBefore-1, p):
		v.trip(GuardActivity, ActionHold, 0, 0)
	}
}

// activityExplains reports whether steps that changed by the share
// change explain the miss of the rate the action would correct: fewer
// steps a rate above the target (a slower loss, a faster gain) that the
// action lowers the kcal for, more a rate below it they would be raised
// for. A target that catches up with the intake is no miss of the rate's.
func (v *Verdict) activityExplains(change float64, p Params) bool {
	switch {
	case change < -p.StepsChange-limits.Epsilon:
		return v.Rate == RateAbove && v.Action == ActionLowerKcal
	case change > p.StepsChange+limits.Epsilon:
		return v.Rate == RateBelow && v.Action == ActionRaiseKcal
	}
	return false
}

// trip sets the action a guard takes, with its gap and its step.
func (v *Verdict) trip(guard, action string, gap, step float64) {
	v.Guards = append(v.Guards, guard)
	v.Action, v.GapKcal, v.StepKcal = action, gap, step
}

// StateOf returns what the limits read of the data on today
// (limits.State), from e, the estimate Calibrate gave on the day before:
// the trend weight now and when the phase ph started, the measured TDEE
// and whether it is trusted, which a TDEE from before the phase may be,
// the fat-free mass and the active energy of the last 7 days, and the
// days logged and weighed in them. The autopilot's own history, its last
// step, its drift and the days since the phase last changed, is the
// autopilot's (P1): left at 0, the wait for a first step holds.
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
	return s
}
