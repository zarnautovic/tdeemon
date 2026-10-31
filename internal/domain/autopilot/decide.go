package autopilot

import (
	"math"
	"slices"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/calibration"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
)

// Outcome is what a review did.
type Outcome string

// Outcomes.
const (
	// Applied: the autopilot took a step by itself.
	Applied Outcome = "applied"
	// Proposed: it made a proposal that waits for the user.
	Proposed Outcome = "proposed"
	// Hold: the target stays, as the verdict or the user's answer asks.
	Hold Outcome = "hold"
	// Skipped: no step could be judged or taken; Reason says why.
	Skipped Outcome = "skipped"
)

// Reason is why a review came out as it did. The app and the messages
// say it in the user's language, from the review's numbers.
type Reason string

// Reasons of a step, or of a proposal of one.
const (
	// A cut or a recomp losing too slowly (the target goes down) or too
	// fast (up).
	ReasonCutSlow Reason = "cut_slow"
	ReasonCutFast Reason = "cut_fast"
	// A bulk gaining too slowly (up) or too fast (down).
	ReasonBulkSlow Reason = "bulk_slow"
	ReasonBulkFast Reason = "bulk_fast"
	// A maintenance phase whose weight drifts down (up) or up (down).
	ReasonMaintainLoss Reason = "maintain_loss"
	ReasonMaintainGain Reason = "maintain_gain"
	// ReasonBulkFat: a bulk gaining fat with the lean mass flat shrinks
	// the surplus, never below the TDEE.
	ReasonBulkFat Reason = "bulk_fat"
	// ReasonLeanLoss: lean mass lost on a cut or a recomp; a step up, as
	// protein stays the coach's.
	ReasonLeanLoss Reason = "lean_loss"
	// ReasonUnderweight: a loss under way below a BMI of 18.5; a proposal
	// of maintenance, never a step.
	ReasonUnderweight Reason = "underweight"
)

// Reasons of a hold.
const (
	// ReasonOnTarget: the pace is on the plan.
	ReasonOnTarget Reason = "on_target"
	// ReasonIntake: the target gives the plan's pace with the measured
	// TDEE; the intake missed it, which no step of the target corrects.
	ReasonIntake Reason = "intake"
	// ReasonStepsGuard: a change of the steps explains the pace.
	ReasonStepsGuard Reason = "steps_guard"
	// ReasonRejected: the user rejected the autopilot's last proposal,
	// and the gap has not grown by a step since; the target stays as the
	// user chose.
	ReasonRejected Reason = "rejected"
)

// Reasons no step is taken.
const (
	// ReasonNoPlan: no phase or no kcal target to judge the pace by.
	ReasonNoPlan Reason = "no_plan"
	// ReasonNotTrusted: calibration does not trust the TDEE yet.
	ReasonNotTrusted Reason = "not_trusted"
	// ReasonFirstWait: the phase is new, or its target was set lately;
	// the first step waits 14 days (and the estimate a phase's own).
	ReasonFirstWait Reason = "first_wait"
	// ReasonInterval: 7 days have not passed since the last step.
	ReasonInterval Reason = "interval"
	// ReasonFewData: too few logged days or weigh-ins in the last 7.
	ReasonFewData Reason = "few_data"
	// ReasonPending: a proposal of the user's or a coach's waits.
	ReasonPending Reason = "pending"
	// ReasonFewCarbs: a step down would leave a type of day under
	// limits.CarbsRDA of carbs; protein and fat are the coach's.
	ReasonFewCarbs Reason = "few_carbs"
	// ReasonLimit: the step breaks another limit (Review.Rule).
	ReasonLimit Reason = "limit"
)

// Waiting is the proposal that waits for the user when a review comes.
type Waiting int

// What waits.
const (
	WaitsNone Waiting = iota
	// WaitsOwn is the autopilot's own: a new one supersedes it.
	WaitsOwn
	// WaitsOther is the user's or a coach's: the review waits for it.
	WaitsOther
)

// Input is what a review is decided from.
type Input struct {
	Today    day.Date
	Settings Settings
	// Estimate is calibration's on the day before today, and Verdict its
	// verdict on the phase (calibration.Judge).
	Estimate calibration.Estimate
	Verdict  calibration.Verdict
	// Expected is the first day a review could read a first estimate:
	// the day after calibration.Expected's last day of its window, the
	// zero Date for none.
	Expected day.Date
	// State is what the limits read today (calibration.StateOf); Decide
	// fills in the autopilot's part from History.
	State   limits.State
	Levels  limits.Levels
	History History
	// Phases and Targets are the timelines, for a proposal of
	// maintenance.
	Phases  []plan.Phase
	Targets []plan.Target
	Waiting Waiting
	// Rejected is the autopilot's last proposal the user rejected, with
	// no change of the plan applied since; nil for none.
	Rejected *Rejected
}

// Rejected is a proposal of the autopilot's the user rejected: why it
// was made, and the gap the verdict asked for then.
type Rejected struct {
	Reason  Reason
	GapKcal float64
}

// Review is a weekly review: what it did, why, and the numbers it went
// by, which the app and the messages say.
type Review struct {
	Date    day.Date
	Outcome Outcome
	Reason  Reason
	// Rule is the limit's rule, for ReasonLimit.
	Rule string
	// StepKcal is the step applied or proposed, a minus being down, and
	// GapKcal the change the verdict asks for.
	StepKcal, GapKcal float64
	// PastDrift is true when the autopilot proposes only because the step
	// would take the drift past its limit.
	PastDrift bool
	// Changes are what it applies or proposes, each as it leaves the
	// plan (the store fills in each Before); none for a hold or a skip.
	Changes proposal.Changes
	// The verdict's pace, in % of the trend weight a week.
	MeasuredPctWeek, TargetPctWeek, TolerancePctWeek float64
	// TDEE and its half interval; IntakeKcal the window's intake; and the
	// weekly average of the target before and after.
	TDEE, HalfWidth                   float64
	IntakeKcal, TargetKcal, AfterKcal float64
	// The data of the last 7 days and what a step needs of them.
	LoggedDays, WeighIns           int
	NeedsLoggedDays, NeedsWeighIns int
	// StepsChangePct is the change of the steps the activity guard read,
	// in %.
	StepsChangePct float64
	// From is the first review that may step, for a wait; Expected when
	// calibration expects an estimate.
	From, Expected day.Date
	// CarbsG are the carbs a step down would leave on DayType, for
	// ReasonFewCarbs.
	CarbsG  float64
	DayType plan.DayType
	// BMI is the BMI by the trend weight, for ReasonUnderweight.
	BMI float64
	// DriftKcal is the drift as the review leaves it, and DriftLimitKcal
	// its limit, 0 until there is a base.
	DriftKcal, DriftLimitKcal float64
}

// Decide decides the review in on in.Today. It applies nothing: a step it
// takes is in Changes, for the store to apply after limits.Validate holds
// it again.
func Decide(in Input) Review {
	user := limits.Effective(in.Levels.User, in.Levels.Instance)
	st := in.State
	st.Autopilot = in.History.Of(in.Today, st.Autopilot.LoggedDays, st.Autopilot.WeighIns, trustedTDEE(in.Estimate))
	r := newReview(in, st, user)
	v := in.Verdict
	switch {
	case st.Phase == nil || st.Target == nil:
		return r.skip(ReasonNoPlan)
	case in.Waiting == WaitsOther:
		return r.skip(ReasonPending)
	case v.Rate == calibration.RateNone:
		if slices.Contains(v.Reasons, calibration.ReasonBeforePhase) {
			// OwnFrom is the last day of an estimate from before the phase,
			// and a review reads the day before its own.
			r.From = NextOn(later(in.Estimate.OwnFrom.AddDays(2), in.History.WaitsUntil(user)), in.Settings.Weekday)
			return r.skip(ReasonFirstWait)
		}
		return r.skip(ReasonNotTrusted)
	case v.Action == calibration.ActionMaintain || slices.Contains(v.Guards, calibration.GuardUnderweight):
		// A loss under way below a BMI of 18.5: no step, and a proposal
		// to stop it, as the limits allow no step during it.
		return r.maintain(in, st)
	}
	step := math.Trunc(v.StepKcal)
	if v.Action == calibration.ActionHold || v.Action == calibration.ActionNone || step == 0 {
		return r.hold(v)
	}
	r.Reason = stepReason(st.Phase.Type, v.Action, step)
	if stillRejected(in.Rejected, v.GapKcal, step) {
		r.Outcome, r.Reason = Hold, ReasonRejected
		return r
	}
	after := st.Target.ShiftKcal(step)
	after.EffectiveFrom = in.Today
	if c, dt, few := fewCarbs(after); step < 0 && few {
		r.CarbsG, r.DayType = c, dt
		return r.skip(ReasonFewCarbs)
	}
	return r.step(in, st, after, step)
}

// newReview returns the review of in with its numbers, st being the
// state the limits read and user the limits that hold.
func newReview(in Input, st limits.State, user limits.Limits) Review {
	v := in.Verdict
	r := Review{
		Date: in.Today, GapKcal: v.GapKcal,
		MeasuredPctWeek: v.MeasuredPctWeek, TargetPctWeek: v.TargetPctWeek, TolerancePctWeek: v.TolerancePctWeek,
		IntakeKcal: v.IntakeKcal, TargetKcal: v.TargetKcal,
		LoggedDays: st.Autopilot.LoggedDays, WeighIns: st.Autopilot.WeighIns,
		NeedsLoggedDays: int(user.LoggedDays), NeedsWeighIns: int(user.WeighIns),
		DriftKcal: in.History.DriftKcal, Expected: in.Expected,
	}
	if st.Profile.HeightCm > 0 && st.TrendKg > 0 {
		r.BMI = st.Profile.BMI(st.TrendKg)
	}
	if in.Estimate.Measured {
		r.TDEE, r.HalfWidth = in.Estimate.TDEE, in.Estimate.HalfWidth
	}
	if b := in.Estimate.Body; b.StepsBefore > 0 {
		r.StepsChangePct = (b.StepsRecent/b.StepsBefore - 1) * 100
	}
	if in.History.BaseTDEE > 0 {
		r.DriftLimitKcal = user.DriftPct / 100 * in.History.BaseTDEE
	}
	if st.Target != nil {
		r.TargetKcal = st.Target.WeeklyAverage(st.TrainingDays.Count()).Kcal
		r.AfterKcal = r.TargetKcal
	}
	return r
}

// fewCarbs returns the fewest carbs after leaves under limits.CarbsRDA,
// with their type of day ("" for an unsplit target), and whether there
// are any.
func fewCarbs(after plan.Target) (float64, plan.DayType, bool) {
	var carbs float64
	var dt plan.DayType
	few := false
	for _, t := range []plan.DayType{plan.Training, plan.Rest} {
		if c := after.For(t).Carbs(); c < limits.CarbsRDA-limits.Epsilon && (!few || c < carbs) {
			few, carbs, dt = true, c, t
		}
	}
	if few && after.Training == after.Rest {
		dt = ""
	}
	return carbs, dt, few
}

// step holds the step to after to the limits, and applies or proposes it
// as the mode and the drift ask.
func (r Review) step(in Input, st limits.State, after plan.Target, step float64) Review {
	user := limits.Effective(in.Levels.User, in.Levels.Instance)
	by := limits.ByAutopilotProposal
	if in.Settings.Mode == Auto {
		by = limits.ByAutopilot
	}
	res := limits.Validate(limits.Change{Target: &after, By: by}, st, in.Levels)
	if by == limits.ByAutopilot && onlyDrift(res) {
		r.PastDrift = true
		by = limits.ByAutopilotProposal
		res = limits.Validate(limits.Change{Target: &after, By: by}, st, in.Levels)
	}
	if !res.OK() {
		reason, rule := refusal(res)
		r.Rule = rule
		if reason == ReasonFirstWait || reason == ReasonInterval {
			r.From = NextOn(later(in.Today.AddDays(1), in.History.WaitsUntil(user)), in.Settings.Weekday)
		}
		return r.skip(reason)
	}
	r.StepKcal = step
	r.AfterKcal = after.WeeklyAverage(st.TrainingDays.Count()).Kcal
	r.Changes = proposal.Changes{Targets: []proposal.TargetChange{{From: in.Today, After: &after}}}
	r.Outcome = Proposed
	if by == limits.ByAutopilot {
		r.Outcome = Applied
	}
	r.DriftKcal = in.History.DriftKcal + step
	if r.DriftLimitKcal == 0 && st.Autopilot.DriftBaseTDEE > 0 && r.Outcome == Applied {
		// The first step takes the base.
		r.DriftLimitKcal = user.DriftPct / 100 * st.Autopilot.DriftBaseTDEE
	}
	if r.Outcome == Proposed && !r.PastDrift {
		r.DriftKcal = in.History.DriftKcal
	}
	return r
}

// maintain proposes to stop a loss under way below a BMI of 18.5: a
// phase that loses ends yesterday and a maintenance phase starts today,
// and the target is raised by the verdict's gap, to the TDEE at least;
// held to the limits as any plan's change. A rejected proposal of it
// comes again only for a larger gap.
func (r Review) maintain(in Input, st limits.State) Review {
	r.Reason = ReasonUnderweight
	if g := in.Rejected; g != nil && g.Reason == ReasonUnderweight && in.Verdict.GapKcal < g.GapKcal+limits.Epsilon {
		r.Outcome = Hold
		r.Reason = ReasonRejected
		return r
	}
	var c proposal.Changes
	if cur := *st.Phase; cur.RatePctWeek < 0 {
		m := plan.Phase{Type: plan.Maintain, Start: in.Today, End: cur.End}
		if cur.Start.Before(in.Today) {
			ended := cur
			ended.End = in.Today.AddDays(-1)
			c.Phases = append(c.Phases, proposal.PhaseChange{Start: cur.Start, After: &ended})
		}
		c.Phases = append(c.Phases, proposal.PhaseChange{Start: in.Today, After: &m})
	}
	if gap := math.Trunc(in.Verdict.GapKcal); gap > 0 {
		after := st.Target.ShiftKcal(gap)
		after.EffectiveFrom = in.Today
		c.Targets = []proposal.TargetChange{{From: in.Today, After: &after}}
		r.StepKcal = gap
		r.AfterKcal = after.WeeklyAverage(st.TrainingDays.Count()).Kcal
	}
	for _, ck := range c.Check(st, in.Levels, in.Phases, in.Targets) {
		if !ck.OK() {
			_, r.Rule = refusal(ck.Result)
			r.StepKcal, r.AfterKcal = 0, r.TargetKcal
			return r.skip(ReasonLimit)
		}
	}
	if len(c.Phases) == 0 && len(c.Targets) == 0 {
		return r.skip(ReasonUnderweight)
	}
	r.Outcome, r.Changes = Proposed, c
	return r
}

// hold is a review whose verdict keeps the target.
func (r Review) hold(v calibration.Verdict) Review {
	r.Outcome = Hold
	switch {
	case slices.Contains(v.Guards, calibration.GuardActivity):
		r.Reason = ReasonStepsGuard
	case v.Rate == calibration.RateOnTarget:
		r.Reason = ReasonOnTarget
	default:
		r.Reason = ReasonIntake
	}
	return r
}

func (r Review) skip(reason Reason) Review {
	r.Outcome, r.Reason = Skipped, reason
	return r
}

// stepReason names a step of the verdict's action on a phase of type t.
func stepReason(t plan.PhaseType, action string, step float64) Reason {
	switch action {
	case calibration.ActionShrinkSurplus:
		return ReasonBulkFat
	case calibration.ActionRaiseProteinOrKcal:
		return ReasonLeanLoss
	}
	up := step > 0
	switch t {
	case plan.Bulk:
		return pick(up, ReasonBulkSlow, ReasonBulkFast)
	case plan.Maintain:
		return pick(up, ReasonMaintainLoss, ReasonMaintainGain)
	case plan.Cut, plan.Recomp:
	}
	return pick(up, ReasonCutFast, ReasonCutSlow)
}

func pick(up bool, ifUp, ifDown Reason) Reason {
	if up {
		return ifUp
	}
	return ifDown
}

// refusal names the reason a step the limits refuse is not taken, by the
// first of its rules in the order the autopilot waits: the phase's first
// days, the interval, the data, and any other limit, with its rule.
func refusal(res limits.Result) (Reason, string) {
	rules := make([]string, 0, len(res.Violations))
	for _, v := range res.Violations {
		rules = append(rules, v.Rule)
	}
	switch {
	case slices.Contains(rules, limits.RuleFirstStep):
		return ReasonFirstWait, limits.RuleFirstStep
	case slices.Contains(rules, limits.RuleInterval):
		return ReasonInterval, limits.RuleInterval
	case slices.Contains(rules, limits.RuleLoggedDays):
		return ReasonFewData, limits.RuleLoggedDays
	case slices.Contains(rules, limits.RuleWeighIns):
		return ReasonFewData, limits.RuleWeighIns
	}
	if len(rules) == 0 {
		return ReasonLimit, ""
	}
	return ReasonLimit, rules[0]
}

// onlyDrift reports whether res refuses a step for its drift alone.
func onlyDrift(res limits.Result) bool {
	if res.OK() {
		return false
	}
	for _, v := range res.Violations {
		if v.Rule != limits.RuleDrift {
			return false
		}
	}
	return true
}

// trustedTDEE returns the TDEE of e when calibration trusts it, else 0.
func trustedTDEE(e calibration.Estimate) float64 {
	if e.Measured && e.Trusted && !e.BeforePhase {
		return e.TDEE
	}
	return 0
}

// stillRejected reports whether g, a rejected step's proposal, holds a
// step of step for gap: the gap the same way, grown by less than a step.
func stillRejected(g *Rejected, gap, step float64) bool {
	return g != nil && g.Reason != ReasonUnderweight && sameSign(g.GapKcal, gap) &&
		math.Abs(gap) < math.Abs(g.GapKcal)+math.Abs(step)-limits.Epsilon
}

func sameSign(a, b float64) bool { return a > 0 && b > 0 || a < 0 && b < 0 }

func later(a, b day.Date) day.Date {
	if a.Before(b) {
		return b
	}
	return a
}

// Repeats reports whether r says what last said, so its message would
// say nothing new: the same hold or skip for the same reason, and for a
// hold of the pace, the intake or the steps, or too little data, the
// same numbers. A step or a
// proposal always says something new.
func (r Review) Repeats(last Review) bool {
	if r.Outcome != last.Outcome || r.Reason != last.Reason || r.Outcome == Applied || r.Outcome == Proposed {
		return false
	}
	if r.Reason == ReasonOnTarget || r.Reason == ReasonIntake || r.Reason == ReasonStepsGuard {
		return round2(r.MeasuredPctWeek) == round2(last.MeasuredPctWeek) && r.TargetKcal == last.TargetKcal &&
			math.Round(r.IntakeKcal) == math.Round(last.IntakeKcal) && math.Round(r.StepsChangePct) == math.Round(last.StepsChangePct)
	}
	if r.Reason == ReasonFewData {
		return r.LoggedDays == last.LoggedDays && r.WeighIns == last.WeighIns
	}
	return true
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// FirstStep returns the first review on or after today, on weekday, that
// may take a step: after the waits of l, and on or after expected when
// calibration trusts no TDEE yet: the first day a review could read a
// first estimate (Input.Expected), the zero Date for none. The data of the days to come it cannot know.
func FirstStep(today day.Date, weekday time.Weekday, h History, l limits.Limits, trusted bool, expected day.Date) day.Date {
	d := later(today, h.WaitsUntil(l))
	if !trusted && !expected.IsZero() {
		d = later(d, expected)
	}
	return NextOn(d, weekday)
}
