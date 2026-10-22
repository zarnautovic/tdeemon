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
	// the measured TDEE, or a guard holds them.
	ActionHold = "hold"
	// ActionRaiseKcal and ActionLowerKcal move the target's weekly
	// average to the one that gives the target rate.
	ActionRaiseKcal = "raise_kcal"
	ActionLowerKcal = "lower_kcal"
	// ActionRaiseProteinOrKcal: the lean mass falls too fast for a cut
	// or a recomp to cut deeper.
	ActionRaiseProteinOrKcal = "raise_protein_or_kcal"
	// ActionShrinkSurplus: a bulk gains fat with the lean mass flat.
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
	// measured rate's miss.
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
	// FlagAdherence: the rate misses the target, but the target gives it
	// with the measured TDEE: the intake missed the target, not the
	// target the rate. It and the flags below show with a verdict only.
	FlagAdherence = "adherence"
	// FlagNoSteps: too few days with steps for the activity guard, which
	// is off.
	FlagNoSteps = "no_steps"
	// FlagNoLeanMass: the lean mass is not measured enough, so the
	// lean-mass and fat-gain guards are off.
	FlagNoLeanMass = "no_lean_mass"
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

// epsilon keeps the error of float arithmetic from moving a value on a
// bound past it, as the limits' does.
const epsilon = 1e-9

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
	// when the action keeps the kcal or is no step's to take.
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
// own days, not one from before it. Its action moves the target, from
// the measured TDEE, so it holds what a user who eats beside the target
// misses (FlagAdherence). Its guards come in order, the first that trips
// taking the action: a loss under way below a BMI of 18.5, by the plan as
// the limits read it, lean mass lost on a cut or a recomp, fat gained on
// a bulk, and a change of activity.
func Judge(e Estimate, ph *plan.Phase, t *plan.Target, training plan.Weekdays, pr profile.Profile, lim limits.Limits, p Params) Verdict {
	v := Verdict{Rate: RateNone, Action: ActionNone, Reasons: reasons(e, ph, t)}
	underweight := v.flagPlan(e, ph, t, training, pr, p)
	if len(v.Reasons) > 0 {
		return v
	}
	v.MeasuredPctWeek = plan.RatePctWeek(e.KgPerWeek, e.TrendKg)
	v.TargetPctWeek, v.TolerancePctWeek = ph.RatePctWeek, p.TolerancePctWeek
	switch {
	case v.MeasuredPctWeek < v.TargetPctWeek-v.TolerancePctWeek-epsilon:
		v.Rate = RateBelow
	case v.MeasuredPctWeek > v.TargetPctWeek+v.TolerancePctWeek+epsilon:
		v.Rate = RateAbove
	default:
		v.Rate = RateOnTarget
	}
	avg := t.WeeklyAverage(training.Count()).Kcal
	step := lim.Step(avg)
	// The weekly average that gives the target rate with the measured
	// TDEE, less the target's; the tolerance in kcal is the same rate's.
	v.GapKcal = e.TDEE + kcalOf(v.TargetPctWeek, e.TrendKg) - avg
	switch tolerance := kcalOf(v.TolerancePctWeek, e.TrendKg); {
	case v.GapKcal > tolerance+epsilon:
		v.Action, v.StepKcal = ActionRaiseKcal, min(v.GapKcal, step)
	case v.GapKcal < -tolerance-epsilon:
		v.Action, v.StepKcal = ActionLowerKcal, max(v.GapKcal, -step)
	default:
		v.Action, v.GapKcal = ActionHold, 0
	}
	if (v.Rate == RateBelow && v.Action != ActionRaiseKcal) || (v.Rate == RateAbove && v.Action != ActionLowerKcal) {
		v.Flags = append(v.Flags, FlagAdherence)
	}
	v.guard(e, ph, underweight, avg, step, p)
	v.flagData(e.Body)
	return v
}

// kcalOf returns the kcal a day a rate of pctWeek % a week gives at the
// trend weight trendKg: its kg a week, KcalPerKg a kg, over 7 days.
func kcalOf(pctWeek, trendKg float64) float64 {
	return plan.KgPerWeek(pctWeek, trendKg) * plan.KcalPerKg / 7
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
	case e.TrendKg > 0 && limits.Underweight(pr.BMI(e.TrendKg)):
		if underweight = limits.LossUnderWay(StateOf(e, day.Date{}, pr, ph, t, training)); underweight {
			v.Flags = append(v.Flags, FlagUnderweight)
		}
	}
	if ph != nil && ph.Type == plan.Maintain && e.TrendKg > 0 && e.PhaseStartTrendKg > 0 &&
		math.Abs(e.TrendKg/e.PhaseStartTrendKg-1) > p.DriftShare+epsilon {
		v.Flags = append(v.Flags, FlagMaintenanceDrift)
	}
	return underweight
}

// flagData adds the flags of the guards the window's data leave off.
func (v *Verdict) flagData(b Body) {
	if b.StepsBefore == 0 {
		v.Flags = append(v.Flags, FlagNoSteps)
	}
	if !b.LeanMeasured {
		v.Flags = append(v.Flags, FlagNoLeanMass)
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
		b.LeanKgPerWeek <= -p.LeanLossKgWeek && b.LeanKgPerWeek+b.LeanHalfWidth < 0:
		v.trip(GuardLeanMass, ActionRaiseProteinOrKcal, max(v.GapKcal, step), step)
	case ph.Type == plan.Bulk && b.BodyFatMeasured && b.LeanMeasured &&
		b.BodyFatPerWeek-b.BodyFatHalfWidth > 0 && b.LeanKgPerWeek < leanFlatKgWeek:
		v.trip(GuardFatGain, ActionShrinkSurplus, min(v.GapKcal, -step), -step)
	case b.StepsBefore > 0 && activityExplains(b.StepsRecent/b.StepsBefore-1, v.Rate, p):
		v.trip(GuardActivity, ActionHold, 0, 0)
	}
}

// activityExplains reports whether steps that changed by the share
// change explain a rate that stands as rate does: fewer steps a rate
// above the target (a slower loss, a faster gain), more a rate below it.
func activityExplains(change float64, rate string, p Params) bool {
	switch {
	case change < -p.StepsChange-epsilon:
		return rate == RateAbove
	case change > p.StepsChange+epsilon:
		return rate == RateBelow
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
