package limits

import (
	"math"
	"slices"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

// The rules a violation names, machine-readable so a model can explain a
// refusal and propose something allowed (DESIGN.md §8).
const (
	// A plan needs the profile and a trend weight; Limit and Proposed are
	// 0.
	RuleProfileMissing = "profile_missing"
	RuleWeightMissing  = "weight_missing"
	// RuleMinAge: no plan for someone under MinAge; the age in years.
	RuleMinAge = "min_age"
	// RuleUnderweightLoss: no change that starts or deepens a loss below
	// a BMI of UnderweightBMI, and no autopilot step during a loss there;
	// the BMI by the trend weight now.
	RuleUnderweightLoss = "underweight_loss"
	// RuleUnderweightGoal and RuleUnderweightMilestone: no goal or
	// milestone weight below a BMI of UnderweightBMI; in kg.
	RuleUnderweightGoal      = "underweight_goal"
	RuleUnderweightMilestone = "underweight_milestone"
	// The phase's rate, in % a week, and in kg a week for a loss.
	RuleCutRate    = "cut_rate"
	RuleBulkRate   = "bulk_rate"
	RuleRecompRate = "recomp_rate"
	RuleLossKgWeek = "loss_kg_week"
	// The rate the weekly average implies with the measured TDEE, once
	// calibration trusts it, in % a week, and in kg a week for a loss.
	RuleImpliedLossRate   = "implied_loss_rate"
	RuleImpliedGainRate   = "implied_gain_rate"
	RuleImpliedLossKgWeek = "implied_loss_kg_week"
	// A day type's kcal, protein in g/kg of the reference mass and in
	// grams, and fat in % of the day's kcal.
	RuleKcalFloor         = "kcal_floor"
	RuleProteinFloor      = "protein_floor"
	RuleProteinCeiling    = "protein_ceiling"
	RuleProteinFloorGrams = "protein_floor_grams"
	RuleFatFloor          = "fat_floor"
	// An autopilot step: its size in kcal of the weekly average, the days
	// since the last step and since the phase started or changed, the
	// days logged and the weigh-ins of the last 7 days, and the drift in
	// % of the TDEE. RuleTDEEMissing: a step without a TDEE to measure
	// its drift against.
	RuleStep        = "autopilot_step"
	RuleInterval    = "autopilot_interval"
	RuleFirstStep   = "autopilot_first_step"
	RuleLoggedDays  = "autopilot_logged_days"
	RuleWeighIns    = "autopilot_weigh_ins"
	RuleDrift       = "autopilot_drift"
	RuleTDEEMissing = "tdee_missing"
	// RuleAutopilotShape: an autopilot step is a shift of the weekly
	// average of the target before (plan.Target.ShiftKcal), and nothing
	// else; Limit and Proposed are 0.
	RuleAutopilotShape = "autopilot_shape"
	// RuleInvalidChange: the change breaks a rule of its shape (a rate of
	// the wrong sign for its phase, a value that is no number), or the
	// state or the limits break theirs; Limit and Proposed are 0.
	RuleInvalidChange = "invalid_change"
)

// The rules a warning names; warnings show in a preview without refusing
// a change (docs/limits.md, "Warnings, not limits").
const (
	// WarnBelowBMR: a day type's kcal under the estimated BMR.
	WarnBelowBMR = "below_bmr"
	// WarnLowEnergyAvailability: a day type's kcal less the active energy,
	// per kg of fat-free mass, under 30 kcal; only with a known lean mass.
	WarnLowEnergyAvailability = "low_energy_availability"
	// WarnLowCarbs: a day type's carbs under 130 g, the RDA.
	WarnLowCarbs = "low_carbs"
)

// Thresholds of the warnings.
const (
	EnergyAvailabilityKcalPerKg = 30
	CarbsRDA                    = 130
)

// Level is which level of limits a value breaks.
type Level string

// Levels, from the outermost.
const (
	// LevelCode is the floor in the code: nothing allows the value.
	LevelCode Level = "code"
	// LevelInstance is the instance's limit: only the admin may loosen it.
	LevelInstance Level = "instance"
	// LevelUser is the user's limit: the user may loosen it, up to the
	// instance's, in a web session.
	LevelUser Level = "user"
)

// Violation is one rule a change breaks: the limit that holds for the
// user (the strictest of the levels) and the value proposed, in the
// units the rule names.
type Violation struct {
	Rule string
	// DayType is the type of day the value is of, for the limits that
	// hold for each; "" for the others, and for an unsplit target.
	DayType  plan.DayType
	Limit    float64
	Proposed float64
	// Level is the outermost level the value breaks, so whoever reads it
	// knows who could allow more: nothing (code), the admin (instance) or
	// the user (user). Limit stays the one that holds for the user. ""
	// for missing data and a change of the wrong shape.
	Level Level
}

// Warning is a value past a threshold that does not refuse a change.
type Warning struct {
	Rule      string
	DayType   plan.DayType
	Threshold float64
	Value     float64
}

// By is who makes a change.
type By int

// Who makes a change.
const (
	// ByPlan is a change of the plan by the user or a coach: a proposal,
	// or the user's edit. It meets every limit but the autopilot's.
	ByPlan By = iota
	// ByAutopilot is a step the autopilot applies: every limit, its step,
	// interval, data and drift too.
	ByAutopilot
	// ByAutopilotProposal is a step the autopilot proposes instead, past
	// its drift: every limit but the drift.
	ByAutopilotProposal
)

// Change is a change of the plan, as it would leave it.
type Change struct {
	// Phase is the current phase as the change leaves it, when the change
	// sets one: a new phase, or an edit of the current one (one with the
	// same type and start).
	Phase *plan.Phase
	// Target is the target as the change leaves it, when it sets one.
	Target *plan.Target
	// TrainingDays are the plan's training weekdays, when the change sets
	// them: they move the weekly average.
	TrainingDays *plan.Weekdays
	By           By
}

// Autopilot is what the data say about the autopilot's last steps. What
// starts its waits again and what counts as a logged day are the
// autopilot's to decide; Validate holds the numbers to the limits.
type Autopilot struct {
	// DaysSinceStep is the number of days since the last step; nil when
	// there was none in the phase.
	DaysSinceStep *int
	// DaysSincePhaseChange is the number of days since the phase started
	// or changed.
	DaysSincePhaseChange int
	// LoggedDays and WeighIns count the last 7 days.
	LoggedDays int
	WeighIns   int
	// DriftKcal is the autopilot's net change of the weekly average since
	// the last kcal target the user agreed to, and DriftBaseTDEE the TDEE
	// then (the measured one, else an estimate), which it is a share of.
	DriftKcal     float64
	DriftBaseTDEE float64
}

// State is the profile and what the data say now: what a change is
// checked against besides the limits.
type State struct {
	Today   day.Date
	Profile profile.Profile
	// TrendKg is the trend weight now; 0 when there is none.
	TrendKg float64
	// Phase is the current phase before the change, nil for none, and
	// PhaseStartTrendKg the trend weight when it started, which picks a
	// cut's row for its whole length (0 when unknown: the trend now).
	Phase             *plan.Phase
	PhaseStartTrendKg float64
	// Target is the current target before the change, nil for none, and
	// TrainingDays the plan's training weekdays.
	Target       *plan.Target
	TrainingDays plan.Weekdays
	// TDEE is the measured TDEE, and TDEETrusted whether calibration
	// trusts it enough for a verdict; only then does a target's implied
	// rate count.
	TDEE        float64
	TDEETrusted bool
	// LeanMassKg is the fat-free mass, 0 when unknown, and ActiveKcal the
	// average active energy of a day, for the warning on energy
	// availability.
	LeanMassKg float64
	ActiveKcal float64
	Autopilot  Autopilot
}

// Levels are the instance's limits and the user's.
type Levels struct {
	Instance Limits
	User     Limits
}

// Result is what Validate finds: the rules a change breaks, which refuse
// it, and the warnings, which do not.
type Result struct {
	Violations []Violation
	Warnings   []Warning
}

// OK reports whether the change breaks no rule.
func (r Result) OK() bool { return len(r.Violations) == 0 }

// Validate checks a change of the plan against the floor in the code, the
// instance's limits and the user's (docs/limits.md). It refuses a change
// only for the values it sets: those it changes, and those derived from
// them (a kcal step sets the fat share and the implied rate). A value new
// data put outside a limit does not stop other changes, and a change that
// brings it toward the limit passes. It runs on a proposal, again when
// one is applied, on every autopilot step, and on a manual edit of a
// target.
func Validate(c Change, s State, lv Levels) Result {
	v := validator{c: c, s: s, levels: lv, user: Effective(lv.User, lv.Instance), instance: lv.Instance}
	if c.Phase == nil && c.Target == nil && c.TrainingDays == nil {
		return v.r
	}
	if !v.shape() || !v.needs() {
		return v.r
	}
	v.prepare()
	if c.Phase != nil {
		v.phase()
	}
	if v.target() != nil {
		v.days()
		v.implied()
		v.warn()
	}
	if c.By != ByPlan {
		v.autopilot()
	}
	return v.r
}

// epsilon keeps the error of float arithmetic from breaking a limit a
// value meets: 59.94 kg at 180 cm is a BMI of 18.5, which float
// arithmetic puts a hair under it.
const epsilon = 1e-9

// validator holds what one run of Validate works with.
type validator struct {
	c              Change
	s              State
	levels         Levels
	user, instance Limits
	r              Result

	trend       float64 // the trend weight now
	bmiNow      float64
	bmiStart    float64 // the BMI when the phase started
	newPhase    bool    // the change starts a phase, rather than editing the current one
	trainingNow plan.Weekdays
}

// shape refuses a change that breaks a rule of its shape, which no limit
// would catch (a comparison with a value that is no number is false; a
// bulk that loses is no cut), and a state or limits that break theirs:
// a number that is no number (a TDEE from a regression with nothing to
// regress), a stored phase, target or profile of the wrong shape. It
// reports false for one.
func (v *validator) shape() bool {
	var bad bool
	switch c := v.c; {
	case c.Phase != nil && c.Phase.Validate() != nil,
		c.Target != nil && (c.Target.Training.Validate() != nil || c.Target.Rest.Validate() != nil),
		c.TrainingDays != nil && c.TrainingDays.Validate() != nil:
		bad = true
	}
	st := v.s
	// What is stored passed these rules when it was written; one that
	// fails them now was written past them, and its numbers would make
	// every comparison false.
	switch {
	case st.Phase != nil && st.Phase.Validate() != nil,
		st.Target != nil && (st.Target.Training.Validate() != nil || st.Target.Rest.Validate() != nil),
		st.Profile.Validate(st.Today) != nil,
		v.levels.User.Validate() != nil, v.levels.Instance.Validate() != nil:
		bad = true
	}
	for _, x := range []float64{
		st.TrendKg, st.PhaseStartTrendKg, st.TDEE, st.LeanMassKg, st.ActiveKcal,
		st.Autopilot.DriftKcal, st.Autopilot.DriftBaseTDEE,
	} {
		bad = bad || !finite(x)
	}
	if bad {
		v.violate(Violation{Rule: RuleInvalidChange})
		return false
	}
	return true
}

// needs checks what a plan needs before anything else: the profile, a
// trend weight and an adult. It reports false when one is missing, and
// the rest cannot be checked, or when the change moves nothing.
func (v *validator) needs() bool {
	if v.c.Phase == nil && v.c.Target == nil && v.s.Target == nil {
		return false // new training days move only an existing target
	}
	if !v.s.Profile.Complete() {
		v.r.Violations = append(v.r.Violations, Violation{Rule: RuleProfileMissing})
	}
	if v.s.TrendKg <= 0 {
		v.r.Violations = append(v.r.Violations, Violation{Rule: RuleWeightMissing})
	}
	if v.r.Violations != nil {
		return false
	}
	if age := v.s.Profile.AgeOn(v.s.Today); age < MinAge {
		v.r.Violations = append(v.r.Violations, Violation{Rule: RuleMinAge, Limit: MinAge, Proposed: float64(age), Level: LevelCode})
		return false
	}
	return true
}

func (v *validator) prepare() {
	v.trend = v.s.TrendKg
	v.bmiNow = v.s.Profile.BMI(v.trend)
	v.newPhase = v.c.Phase != nil && (v.s.Phase == nil || v.c.Phase.Type != v.s.Phase.Type || v.c.Phase.Start != v.s.Phase.Start)
	v.bmiStart = v.bmiNow
	if !v.newPhase && v.s.Phase != nil && v.s.PhaseStartTrendKg > 0 {
		v.bmiStart = v.s.Profile.BMI(v.s.PhaseStartTrendKg)
	}
	v.trainingNow = v.s.TrainingDays
	if v.c.TrainingDays != nil {
		v.trainingNow = *v.c.TrainingDays
	}
}

// cutRate returns the fastest loss of limits l for the BMI when the
// phase started.
func (v *validator) cutRate(l Limits) float64 {
	if v.bmiStart >= ObeseBMI {
		return l.CutRateObese
	}
	return l.CutRate
}

// phase checks the phase the change sets: its rate when it sets one, and
// its goal and milestone weights.
func (v *validator) phase() {
	p := *v.c.Phase
	var old *plan.Phase
	if !v.newPhase {
		old = v.s.Phase
	}
	if old == nil || p.RatePctWeek != old.RatePctWeek {
		var oldRate *float64
		if old != nil {
			oldRate = &old.RatePctWeek
		}
		v.rate(p, oldRate)
	}
	floor := v.s.Profile.WeightAt(UnderweightBMI)
	for _, w := range []struct {
		rule     string
		now, was *float64
	}{
		{RuleUnderweightGoal, p.GoalWeight, oldWeight(old, func(p *plan.Phase) *float64 { return p.GoalWeight })},
		{RuleUnderweightMilestone, p.MilestoneWeight, oldWeight(old, func(p *plan.Phase) *float64 { return p.MilestoneWeight })},
	} {
		if w.now != nil && (w.was == nil || *w.now != *w.was) && *w.now < floor-epsilon {
			v.violate(Violation{Rule: w.rule, Limit: floor, Proposed: *w.now, Level: LevelCode})
		}
	}
}

func oldWeight(p *plan.Phase, get func(*plan.Phase) *float64) *float64 {
	if p == nil {
		return nil
	}
	return get(p)
}

// rate checks a phase's rate the change sets, old being the one before
// when the change edits the phase.
func (v *validator) rate(p plan.Phase, old *float64) {
	r := p.RatePctWeek
	switch p.Type {
	case plan.Cut:
		v.hold(bound{rule: RuleCutRate, floor: true, user: v.cutRate(v.user), instance: v.cutRate(v.instance)}, r, old)
	case plan.Bulk:
		v.hold(bound{rule: RuleBulkRate, user: v.user.BulkRate, instance: v.instance.BulkRate}, r, old)
	case plan.Recomp:
		v.hold(bound{rule: RuleRecompRate, floor: true, user: v.user.RecompMin, instance: v.instance.RecompMin}, r, old)
		v.hold(bound{rule: RuleRecompRate, user: v.user.RecompMax, instance: v.instance.RecompMax}, r, old)
	case plan.Maintain:
	}
	if r < 0 {
		v.hold(bound{rule: RuleLossKgWeek, floor: true, user: -v.user.LossKgWeek, instance: -v.instance.LossKgWeek},
			plan.KgPerWeek(r, v.trend), scaled(old, func(o float64) float64 { return plan.KgPerWeek(o, v.trend) }))
	}
	// The loss under way is the current phase's, a new phase's too: a
	// new phase that loses more slowly does not start a loss.
	was := 0.0
	if v.s.Phase != nil {
		was = math.Min(v.s.Phase.RatePctWeek, 0)
	}
	v.underweight(r, was)
}

// underweight refuses a loss that starts or deepens, from was (0 for no
// loss; never more) to rate, below a BMI of UnderweightBMI.
func (v *validator) underweight(rate, was float64) {
	if v.underweightNow() && rate < was {
		v.violate(Violation{Rule: RuleUnderweightLoss, Limit: UnderweightBMI, Proposed: v.bmiNow, Level: LevelCode})
	}
}

// underweightNow reports whether the trend weight now is below a BMI of
// UnderweightBMI.
func (v *validator) underweightNow() bool { return v.bmiNow < UnderweightBMI-epsilon }

// target returns the target as the change leaves it, nil when there is
// none.
func (v *validator) target() *plan.Target {
	if v.c.Target != nil {
		return v.c.Target
	}
	return v.s.Target
}

// dayTypes returns the day types to check, each with its target, and the
// one before when there is one: an unsplit target once, with no type.
func (v *validator) dayTypes() []struct {
	dt       plan.DayType
	now, was plan.DayTarget
	hadOld   bool
} {
	t := v.target()
	type pair = struct {
		dt       plan.DayType
		now, was plan.DayTarget
		hadOld   bool
	}
	var out []pair
	for _, dt := range []plan.DayType{plan.Training, plan.Rest} {
		p := pair{dt: dt, now: t.For(dt)}
		if v.s.Target != nil {
			p.was, p.hadOld = v.s.Target.For(dt), true
		}
		out = append(out, p)
	}
	if t.Training == t.Rest && (!out[0].hadOld || out[0].was == out[1].was) {
		out[1].dt = ""
		return out[1:]
	}
	return out
}

// days checks each day type's kcal, protein and fat the change sets;
// new training days alone set none.
func (v *validator) days() {
	sex := v.s.Profile.Sex
	floorOf := func(l Limits) float64 {
		if sex == profile.Female {
			return l.KcalFloorFemale
		}
		return l.KcalFloorMale
	}
	ref := v.s.Profile.ProteinReferenceMass(v.trend)
	for _, d := range v.dayTypes() {
		was := func(get func(plan.DayTarget) float64) *float64 {
			if !d.hadOld {
				return nil
			}
			x := get(d.was)
			return &x
		}
		kcalSet := !d.hadOld || d.now.Kcal != d.was.Kcal
		if kcalSet {
			v.hold(bound{
				rule: RuleKcalFloor, dayType: d.dt, floor: true, user: floorOf(v.user), instance: floorOf(v.instance),
				code: CodeKcalFloor, hasCode: true, codeExclusive: true,
			}, d.now.Kcal, was(func(t plan.DayTarget) float64 { return t.Kcal }))
		}
		if kcalSet || d.now.Fat != d.was.Fat {
			v.hold(bound{rule: RuleFatFloor, dayType: d.dt, floor: true, user: v.user.FatFloorPct, instance: v.instance.FatFloorPct},
				d.now.FatShare()*100, was(func(t plan.DayTarget) float64 { return t.FatShare() * 100 }))
		}
		if !d.hadOld || d.now.Protein != d.was.Protein {
			perKg := func(t plan.DayTarget) float64 { return plan.ProteinPerKg(t.Protein, ref) }
			v.hold(bound{
				rule: RuleProteinFloor, dayType: d.dt, floor: true, user: v.user.ProteinFloor, instance: v.instance.ProteinFloor,
				code: CodeProteinFloor, hasCode: true,
			}, perKg(d.now), was(perKg))
			v.hold(bound{rule: RuleProteinCeiling, dayType: d.dt, user: v.user.ProteinCeiling, instance: v.instance.ProteinCeiling},
				perKg(d.now), was(perKg))
			v.hold(bound{rule: RuleProteinFloorGrams, dayType: d.dt, floor: true, user: v.user.ProteinFloorGrams},
				d.now.Protein, was(func(t plan.DayTarget) float64 { return t.Protein }))
		}
	}
}

// implied checks the rate the weekly average implies with the measured
// TDEE, once calibration trusts it, when the change moves the average.
func (v *validator) implied() {
	if !v.s.TDEETrusted || v.s.TDEE <= 0 {
		v.lowerWhileUnderweight()
		return
	}
	t := v.target()
	now := v.impliedRate(*t, v.trainingNow)
	var old *float64
	if v.s.Target != nil {
		o := v.impliedRate(*v.s.Target, v.s.TrainingDays)
		if o == now {
			return // the average does not move
		}
		old = &o
	}
	switch {
	case now < 0:
		v.hold(bound{rule: RuleImpliedLossRate, floor: true, user: v.cutRate(v.user), instance: v.cutRate(v.instance)}, now, old)
		v.hold(bound{rule: RuleImpliedLossKgWeek, floor: true, user: -v.user.LossKgWeek, instance: -v.instance.LossKgWeek},
			plan.KgPerWeek(now, v.trend), scaled(old, func(o float64) float64 { return plan.KgPerWeek(o, v.trend) }))
	case now > 0:
		v.hold(bound{rule: RuleImpliedGainRate, user: v.user.BulkRate, instance: v.instance.BulkRate}, now, old)
	}
	was := 0.0
	if old != nil {
		was = math.Min(*old, 0)
	}
	v.underweight(now, was)
}

// lowerWhileUnderweight holds the floor in the code below a BMI of
// UnderweightBMI before calibration trusts the TDEE: no rate can be
// implied then, so a target that lowers the weekly average is taken for
// one that deepens a loss, and refused.
func (v *validator) lowerWhileUnderweight() {
	if !v.underweightNow() || v.s.Target == nil {
		return
	}
	now := v.target().WeeklyAverage(v.trainingNow.Count()).Kcal
	before := v.s.Target.WeeklyAverage(v.s.TrainingDays.Count()).Kcal
	if (bound{floor: true}).breaks(now, before, false) {
		v.violate(Violation{Rule: RuleUnderweightLoss, Limit: UnderweightBMI, Proposed: v.bmiNow, Level: LevelCode})
	}
}

func (v *validator) impliedRate(t plan.Target, training plan.Weekdays) float64 {
	return plan.ImpliedRate(t.WeeklyAverage(training.Count()).Kcal, v.s.TDEE, v.trend)
}

// autopilot checks an autopilot step: its size, its waits, its data and,
// for a step it applies, its drift; and no step during a loss below a BMI
// of UnderweightBMI.
func (v *validator) autopilot() {
	a := v.s.Autopilot
	if v.underweightNow() && v.lossUnderWay() {
		v.violate(Violation{Rule: RuleUnderweightLoss, Limit: UnderweightBMI, Proposed: v.bmiNow, Level: LevelCode})
	}
	if !v.isStep() {
		v.violate(Violation{Rule: RuleAutopilotShape})
	} else {
		n := v.s.TrainingDays.Count()
		before := v.s.Target.WeeklyAverage(n).Kcal
		delta := v.c.Target.WeeklyAverage(n).Kcal - before
		step := func(l Limits) float64 { return math.Min(l.StepKcal, l.StepPct/100*before) }
		v.hold(bound{rule: RuleStep, user: step(v.user), instance: step(v.instance)}, math.Abs(delta), nil)
		if v.c.By == ByAutopilot {
			v.drift(delta)
		}
	}
	if a.DaysSinceStep != nil {
		v.hold(bound{rule: RuleInterval, floor: true, user: v.user.IntervalDays, instance: v.instance.IntervalDays}, float64(*a.DaysSinceStep), nil)
	}
	v.hold(bound{rule: RuleFirstStep, floor: true, user: v.user.FirstStepDays, instance: v.instance.FirstStepDays}, float64(a.DaysSincePhaseChange), nil)
	v.hold(bound{rule: RuleLoggedDays, floor: true, user: v.user.LoggedDays, instance: v.instance.LoggedDays}, float64(a.LoggedDays), nil)
	v.hold(bound{rule: RuleWeighIns, floor: true, user: v.user.WeighIns, instance: v.instance.WeighIns}, float64(a.WeighIns), nil)
}

// isStep reports whether the change is an autopilot step: a new target
// that shifts the weekly average of the one before, as
// plan.Target.ShiftKcal does, with nothing else changed.
func (v *validator) isStep() bool {
	c, before := v.c, v.s.Target
	if c.Target == nil || before == nil || c.Phase != nil || c.TrainingDays != nil {
		return false
	}
	want := before.ShiftKcal(c.Target.Training.Kcal - before.Training.Kcal)
	near := func(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }
	return near(c.Target.Rest.Kcal, want.Rest.Kcal) && c.Target.Training.Protein == want.Training.Protein &&
		c.Target.Training.Fat == want.Training.Fat && c.Target.Rest.Protein == want.Rest.Protein && c.Target.Rest.Fat == want.Rest.Fat
}

// drift checks the autopilot's net change from the last kcal target the
// user agreed to, with this step's delta.
func (v *validator) drift(delta float64) {
	a := v.s.Autopilot
	if a.DriftBaseTDEE <= 0 {
		v.violate(Violation{Rule: RuleTDEEMissing})
		return
	}
	pct := func(kcal float64) float64 { return math.Abs(kcal) / a.DriftBaseTDEE * 100 }
	was := pct(a.DriftKcal)
	v.hold(bound{rule: RuleDrift, user: v.user.DriftPct, instance: v.instance.DriftPct}, pct(a.DriftKcal+delta), &was)
}

// lossUnderWay reports whether the plan before the change loses weight:
// a phase with a loss, or a target whose implied rate is one.
func (v *validator) lossUnderWay() bool {
	if v.s.Phase != nil && v.s.Phase.RatePctWeek < 0 {
		return true
	}
	return v.s.TDEETrusted && v.s.TDEE > 0 && v.s.Target != nil && v.impliedRate(*v.s.Target, v.s.TrainingDays) < 0
}

// warn adds the warnings of the target the change leaves.
func (v *validator) warn() {
	bmr := v.s.Profile.BMR(v.trend, v.s.Today)
	for _, d := range v.dayTypes() {
		if d.now.Kcal < bmr {
			v.r.Warnings = append(v.r.Warnings, Warning{Rule: WarnBelowBMR, DayType: d.dt, Threshold: bmr, Value: d.now.Kcal})
		}
		if v.s.LeanMassKg > 0 {
			if ea := (d.now.Kcal - v.s.ActiveKcal) / v.s.LeanMassKg; ea < EnergyAvailabilityKcalPerKg {
				v.r.Warnings = append(v.r.Warnings, Warning{Rule: WarnLowEnergyAvailability, DayType: d.dt, Threshold: EnergyAvailabilityKcalPerKg, Value: ea})
			}
		}
		if c := d.now.Carbs(); c < CarbsRDA {
			v.r.Warnings = append(v.r.Warnings, Warning{Rule: WarnLowCarbs, DayType: d.dt, Threshold: CarbsRDA, Value: c})
		}
	}
}

// bound is one limit a value is held to, on each level.
type bound struct {
	rule    string
	dayType plan.DayType
	// floor is true for a least value, false for a most.
	floor          bool
	user, instance float64
	// code is the floor in the code, when hasCode; codeExclusive when the
	// floor itself breaks it (800 kcal).
	code          float64
	hasCode       bool
	codeExclusive bool
}

// breaks reports whether x breaks limit, as a floor or a ceiling. A value
// within the error of float arithmetic of the limit is on it: a default
// cut at 1.5 kg a week, or 1.6 g/kg of a reference mass turned into
// grams and back, comes a hair past it.
func (b bound) breaks(x, limit float64, exclusive bool) bool {
	tol := epsilon * math.Max(1, math.Abs(limit))
	switch {
	case b.floor && exclusive:
		return x <= limit+tol
	case b.floor:
		return x < limit-tol
	}
	return x > limit+tol
}

// hold reports a violation when x breaks b on the user's level, which is
// the strictest: the effective limits. old is the value before the
// change, when there was one: a value that broke the limit already and
// moves toward it passes (one that met it and moves toward it meets it
// still).
func (v *validator) hold(b bound, x float64, old *float64) {
	limit := b.user
	code := b.hasCode && b.breaks(x, b.code, b.codeExclusive)
	if !code && !b.breaks(x, limit, false) {
		return
	}
	if old != nil && (b.floor && x > *old || !b.floor && x < *old) {
		return
	}
	level := LevelUser
	switch {
	case code:
		level = LevelCode
		// The code's floor holds whatever the levels say; it is the limit
		// when theirs are looser.
		if !b.breaks(x, limit, false) {
			limit = b.code
		}
	case b.breaks(x, b.instance, false):
		level = LevelInstance
	}
	v.violate(Violation{Rule: b.rule, DayType: b.dayType, Limit: limit, Proposed: x, Level: level})
}

// violate adds x, unless the result has it already: a step down during a
// loss below a BMI of 18.5 deepens the loss and is a step during it.
func (v *validator) violate(x Violation) {
	if !slices.Contains(v.r.Violations, x) {
		v.r.Violations = append(v.r.Violations, x)
	}
}

// scaled returns f of *p, or nil for nil.
func scaled(p *float64, f func(float64) float64) *float64 {
	if p == nil {
		return nil
	}
	x := f(*p)
	return &x
}
