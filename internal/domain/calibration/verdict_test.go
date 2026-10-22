package calibration

import (
	"slices"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

// The plan the verdicts are judged on: 2400 kcal on training days, 2200
// on rest days, training on Monday, Wednesday and Friday, a weekly
// average of 2286, so the user's default step is 100 kcal (5 % is 114).
var (
	target = &plan.Target{
		EffectiveFrom: last.AddDays(-60),
		Training:      plan.DayTarget{Kcal: 2400, Protein: 180, Fat: 70},
		Rest:          plan.DayTarget{Kcal: 2200, Protein: 180, Fat: 70},
	}
	training, _ = plan.WeekdaysOf(time.Monday, time.Wednesday, time.Friday)
	person      = profile.Profile{Sex: profile.Male, BirthDate: day.Date{Year: 1990, Month: time.May, Day: 1}, HeightCm: 180}
)

// trusted returns a trusted estimate of the phase's own days at 80 kg,
// moving rate % a week, with steps that did not change and the lean mass
// level. Its TDEE is the one with which the target gives that rate, as
// for someone who eats the target.
func trusted(rate float64) Estimate { return trustedAt(rate, 80) }

func trustedAt(rate, kg float64) Estimate {
	return Estimate{
		Measured: true, Trusted: true, TrendKg: kg, KgPerWeek: plan.KgPerWeek(rate, kg),
		TDEE: target.WeeklyAverage(training.Count()).Kcal - kcalOf(rate, kg),
		Body: Body{StepsRecent: 7000, StepsBefore: 7000, LeanMeasured: true, LeanHalfWidth: 0.1},
	}
}

func phase(t plan.PhaseType, rate float64) *plan.Phase {
	return &plan.Phase{Type: t, Start: last.AddDays(-60), RatePctWeek: rate}
}

func TestJudgeTheRate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		ph             *plan.Phase
		measured       float64
		wantRate       string
		wantAction     string
		wantGap, wantS float64
	}{
		// 0.3 % a week of 80 kg is 0.24 kg, 264 kcal a day.
		{"a cut losing too fast", phase(plan.Cut, -0.5), -0.8, RateBelow, ActionRaiseKcal, 264, 100},
		{"a cut losing too slowly", phase(plan.Cut, -0.5), -0.2, RateAbove, ActionLowerKcal, -264, -100},
		{"a cut on target", phase(plan.Cut, -0.5), -0.45, RateOnTarget, ActionHold, 0, 0},
		// A gap smaller than a step is the step.
		{"a cut a little too fast", phase(plan.Cut, -0.5), -0.61, RateBelow, ActionRaiseKcal, 96.8, 96.8},
		{"a bulk gaining too slowly", phase(plan.Bulk, 0.25), 0.05, RateBelow, ActionRaiseKcal, 176, 100},
		{"a bulk gaining too fast", phase(plan.Bulk, 0.25), 0.5, RateAbove, ActionLowerKcal, -220, -100},
		{"maintenance gaining", phase(plan.Maintain, 0), 0.15, RateAbove, ActionLowerKcal, -132, -100},
		{"a recomp on target", phase(plan.Recomp, -0.1), -0.05, RateOnTarget, ActionHold, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := Judge(trusted(tc.measured), tc.ph, target, training, person, limits.UserDefaults(), DefaultParams())
			if v.Rate != tc.wantRate || v.Action != tc.wantAction || len(v.Guards) != 0 || len(v.Reasons) != 0 || len(v.Flags) != 0 {
				t.Errorf("rate %s, action %s, guards %v, reasons %v, flags %v; want %s and %s",
					v.Rate, v.Action, v.Guards, v.Reasons, v.Flags, tc.wantRate, tc.wantAction)
			}
			if !near(v.GapKcal, tc.wantGap, 1e-6) || !near(v.StepKcal, tc.wantS, 1e-6) {
				t.Errorf("gap %v, step %v; want %v and %v", v.GapKcal, v.StepKcal, tc.wantGap, tc.wantS)
			}
			if !near(v.MeasuredPctWeek, tc.measured, 1e-9) || v.TargetPctWeek != tc.ph.RatePctWeek || v.TolerancePctWeek != 0.1 {
				t.Errorf("measured %v, target %v ± %v", v.MeasuredPctWeek, v.TargetPctWeek, v.TolerancePctWeek)
			}
		})
	}
}

func TestJudgeTheTargetNotTheIntake(t *testing.T) {
	t.Parallel()

	// Someone who eats beside the target: the action moves the target to
	// the weekly average that gives the target rate with the measured
	// TDEE, and the flag says the intake missed it.
	avg := target.WeeklyAverage(training.Count()).Kcal
	for _, tc := range []struct {
		name       string
		rate, tdee float64
		wantRate   string
		wantAction string
		wantGap    float64
		wantFlags  []string
	}{
		// Losing 0.8 % a week on a cut of 0.5, but the target, with a TDEE
		// of avg + 440, gives 0.5: the intake was under it.
		{"under the right target", -0.8, avg + 440, RateBelow, ActionHold, 0, []string{FlagAdherence}},
		// Losing too fast, but the target is 200 kcal over what gives the
		// target rate: it comes down, though the user should eat more of
		// it.
		{"under a target too high", -0.8, avg + 240, RateBelow, ActionLowerKcal, -200, []string{FlagAdherence}},
		{"over the right target", -0.2, avg + 440, RateAbove, ActionHold, 0, []string{FlagAdherence}},
		// On target with a target 300 kcal under what gives the rate: the
		// target catches up with the intake.
		{"on target, over a target too low", -0.5, avg + 740, RateOnTarget, ActionRaiseKcal, 300, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := trusted(tc.rate)
			e.TDEE = tc.tdee
			v := Judge(e, phase(plan.Cut, -0.5), target, training, person, limits.UserDefaults(), DefaultParams())
			if v.Rate != tc.wantRate || v.Action != tc.wantAction || !near(v.GapKcal, tc.wantGap, 1e-6) || !slices.Equal(v.Flags, tc.wantFlags) {
				t.Errorf("rate %s, action %s, gap %v, flags %v; want %s, %s, %v, %v",
					v.Rate, v.Action, v.GapKcal, v.Flags, tc.wantRate, tc.wantAction, tc.wantGap, tc.wantFlags)
			}
		})
	}
}

func TestJudgeWithoutAVerdict(t *testing.T) {
	t.Parallel()

	before := trusted(-0.5)
	before.BeforePhase = true
	untrusted := trusted(-0.5)
	untrusted.Trusted = false
	for _, tc := range []struct {
		name string
		e    Estimate
		ph   *plan.Phase
		t    *plan.Target
		want []string
	}{
		{"no phase", trusted(-0.5), nil, target, []string{ReasonNoPhase}},
		{"no target", trusted(-0.5), phase(plan.Cut, -0.5), nil, []string{ReasonNoTarget}},
		{"no estimate", Estimate{}, phase(plan.Cut, -0.5), target, []string{ReasonNoEstimate}},
		{"an estimate from before the phase", before, phase(plan.Cut, -0.5), target, []string{ReasonBeforePhase}},
		{"an estimate not trusted", untrusted, phase(plan.Cut, -0.5), target, []string{ReasonNotTrusted}},
		{"nothing", Estimate{}, nil, nil, []string{ReasonNoPhase, ReasonNoTarget, ReasonNoEstimate}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := Judge(tc.e, tc.ph, tc.t, training, person, limits.UserDefaults(), DefaultParams())
			if v.Rate != RateNone || v.Action != ActionNone || !slices.Equal(v.Reasons, tc.want) || v.StepKcal != 0 || v.GapKcal != 0 {
				t.Errorf("rate %s, action %s, gap %v, step %v, reasons %v; want none for %v",
					v.Rate, v.Action, v.GapKcal, v.StepKcal, v.Reasons, tc.want)
			}
		})
	}
}

func TestJudgeGuards(t *testing.T) {
	t.Parallel()

	with := func(e Estimate, change ...func(*Estimate)) Estimate {
		for _, c := range change {
			c(&e)
		}
		return e
	}
	leanLoss := func(e *Estimate) { e.Body.LeanMeasured, e.Body.LeanKgPerWeek, e.Body.LeanHalfWidth = true, -0.6, 0.1 }
	fatGain := func(e *Estimate) {
		e.Body.BodyFatMeasured, e.Body.BodyFatPerWeek, e.Body.BodyFatHalfWidth = true, 0.1, 0.03
		e.Body.LeanMeasured, e.Body.LeanKgPerWeek = true, 0.05
	}
	steps := func(recent float64) func(*Estimate) { return func(e *Estimate) { e.Body.StepsRecent = recent } }
	for _, tc := range []struct {
		name       string
		e          Estimate
		ph         *plan.Phase
		pr         profile.Profile
		wantAction string
		wantGuards []string
		// wantGap and wantStep are the action's gap and step, in kcal.
		wantGap, wantStep float64
	}{
		// 58 kg at 180 cm is a BMI of 17.9. A cut there: maintenance, the
		// gap to the TDEE, 0.5 % a week of 58 kg being 319 kcal a day; no
		// step.
		{"a cut below a BMI of 18.5", trustedAt(-0.5, 58), phase(plan.Cut, -0.5), person, ActionMaintain, []string{GuardUnderweight}, 319, 0},
		// The plan loses, though the weight stalled: no cut at all.
		{"a stalled cut below a BMI of 18.5", trustedAt(0.05, 58), phase(plan.Cut, -0.5), person, ActionMaintain, []string{GuardUnderweight}, 0, 0},
		{"a cut gaining below a BMI of 18.5", trustedAt(0.4, 58), phase(plan.Cut, -0.5), person, ActionMaintain, []string{GuardUnderweight}, 0, 0},
		// A bulk whose target is under the TDEE loses by the limits'
		// reading too: maintenance, eating 191 kcal more.
		{"a bulk losing below a BMI of 18.5", trustedAt(-0.3, 58), phase(plan.Bulk, 0.25), person, ActionMaintain, []string{GuardUnderweight}, 191.4, 0},
		{
			"the underweight guard before the lean mass's", with(trustedAt(-0.5, 58), leanLoss), phase(plan.Cut, -0.5), person,
			ActionMaintain,
			[]string{GuardUnderweight},
			319, 0,
		},
		{"no height, no BMI", trustedAt(-0.5, 58), phase(plan.Cut, -0.5), profile.Profile{}, ActionHold, nil, 0, 0},
		// A guard asks for at least its step, and keeps a larger gap of
		// its own way.
		{
			"lean mass lost on a cut losing too slowly", with(trusted(-0.2), leanLoss), phase(plan.Cut, -0.5), person,
			ActionRaiseProteinOrKcal,
			[]string{GuardLeanMass},
			100, 100,
		},
		{
			"lean mass lost on a cut losing too fast", with(trusted(-0.8), leanLoss), phase(plan.Cut, -0.5), person,
			ActionRaiseProteinOrKcal,
			[]string{GuardLeanMass},
			264, 100,
		},
		{"lean mass lost on a recomp", with(trusted(-0.1), leanLoss), phase(plan.Recomp, -0.1), person, ActionRaiseProteinOrKcal, []string{GuardLeanMass}, 100, 100},
		{
			"lean mass lost within its error", with(trusted(-0.2), leanLoss, func(e *Estimate) { e.Body.LeanHalfWidth = 0.8 }),
			phase(plan.Cut, -0.5), person, ActionLowerKcal, nil, -264, -100,
		},
		{"lean mass lost on a bulk", with(trusted(0.25), leanLoss), phase(plan.Bulk, 0.25), person, ActionHold, nil, 0, 0},
		{"fat gained on a bulk", with(trusted(0.25), fatGain), phase(plan.Bulk, 0.25), person, ActionShrinkSurplus, []string{GuardFatGain}, -100, -100},
		{
			"fat gained on a bulk gaining too fast", with(trusted(0.5), fatGain), phase(plan.Bulk, 0.25), person,
			ActionShrinkSurplus,
			[]string{GuardFatGain},
			-220, -100,
		},
		{
			"fat gained with the lean mass", with(trusted(0.25), fatGain, func(e *Estimate) { e.Body.LeanKgPerWeek = 0.2 }),
			phase(plan.Bulk, 0.25), person, ActionHold, nil, 0, 0,
		},
		{
			"body fat within its error", with(trusted(0.25), fatGain, func(e *Estimate) { e.Body.BodyFatHalfWidth = 0.15 }),
			phase(plan.Bulk, 0.25), person, ActionHold, nil, 0, 0,
		},
		// 4500 steps against 7000 is 36 % fewer: the slower loss is the
		// activity's, and so is a faster one with 36 % more.
		{"fewer steps, a slower loss", with(trusted(-0.2), steps(4500)), phase(plan.Cut, -0.5), person, ActionHold, []string{GuardActivity}, 0, 0},
		{"more steps, a faster loss", with(trusted(-0.8), steps(9500)), phase(plan.Cut, -0.5), person, ActionHold, []string{GuardActivity}, 0, 0},
		// More steps do not explain a slower loss.
		{"more steps, a slower loss", with(trusted(-0.2), steps(9800)), phase(plan.Cut, -0.5), person, ActionLowerKcal, nil, -264, -100},
		{"steps 25 % fewer", with(trusted(-0.2), steps(5250)), phase(plan.Cut, -0.5), person, ActionLowerKcal, nil, -264, -100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := Judge(tc.e, tc.ph, target, training, tc.pr, limits.UserDefaults(), DefaultParams())
			if v.Action != tc.wantAction || !slices.Equal(v.Guards, tc.wantGuards) {
				t.Errorf("action %s, guards %v; want %s, %v", v.Action, v.Guards, tc.wantAction, tc.wantGuards)
			}
			if !near(v.GapKcal, tc.wantGap, 0.1) || !near(v.StepKcal, tc.wantStep, 1e-6) {
				t.Errorf("gap %v, step %v; want %v and %v", v.GapKcal, v.StepKcal, tc.wantGap, tc.wantStep)
			}
		})
	}
}

func TestJudgeFlags(t *testing.T) {
	t.Parallel()

	untrusted := func(e *Estimate) { e.Trusted = false }
	for _, tc := range []struct {
		name   string
		e      Estimate
		change func(*Estimate)
		ph     *plan.Phase
		pr     *profile.Profile
		want   []string
	}{
		{"steps and lean mass", trusted(0), func(*Estimate) {}, phase(plan.Cut, -0.5), nil, nil},
		{"no steps", trusted(0), func(e *Estimate) { e.Body.StepsBefore = 0 }, phase(plan.Cut, -0.5), nil, []string{FlagNoSteps}},
		{"no lean mass", trusted(0), func(e *Estimate) { e.Body.LeanMeasured = false }, phase(plan.Cut, -0.5), nil, []string{FlagNoLeanMass}},
		// The window's data say nothing without a verdict.
		{"no data, no verdict", trusted(0), func(e *Estimate) { e.Trusted, e.Body = false, Body{} }, phase(plan.Cut, -0.5), nil, nil},
		// The plan's flags hold without a verdict too.
		{"a cut below a BMI of 18.5", trustedAt(0, 58), untrusted, phase(plan.Cut, -0.5), nil, []string{FlagUnderweight}},
		{"a bulk below a BMI of 18.5", trustedAt(0, 58), func(*Estimate) {}, phase(plan.Bulk, 0.25), nil, nil},
		{"no height", trusted(0), func(*Estimate) {}, phase(plan.Cut, -0.5), &profile.Profile{}, []string{FlagNoHeight}},
		// 83 kg against 80 at the phase's start is 3.75 % off; 82.4 is 3
		// %, which is not more.
		{"maintenance drifting", trusted(0), func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 83, 80 }, phase(plan.Maintain, 0), nil, []string{FlagMaintenanceDrift}},
		{
			"maintenance drifting, no verdict", trusted(0), func(e *Estimate) { e.Measured, e.TrendKg, e.PhaseStartTrendKg = false, 83, 80 },
			phase(plan.Maintain, 0), nil,
			[]string{FlagMaintenanceDrift},
		},
		{"maintenance 3 % off", trusted(0), func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 82.4, 80 }, phase(plan.Maintain, 0), nil, nil},
		{"a cut 3 % down", trusted(0), func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 77, 80 }, phase(plan.Cut, -0.5), nil, nil},
		{"no start", trusted(0), func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 83, 0 }, phase(plan.Maintain, 0), nil, nil},
		{"no trend", trusted(0), func(e *Estimate) { e.Measured, e.TrendKg, e.PhaseStartTrendKg = false, 0, 80 }, phase(plan.Maintain, 0), nil, nil},
		{"no phase", trusted(0), func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 83, 80 }, nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, pr := tc.e, person
			tc.change(&e)
			if tc.pr != nil {
				pr = *tc.pr
			}
			if v := Judge(e, tc.ph, target, training, pr, limits.UserDefaults(), DefaultParams()); !slices.Equal(v.Flags, tc.want) {
				t.Errorf("flags %v, want %v", v.Flags, tc.want)
			}
		})
	}
}

func TestCalibrateReadsTheBody(t *testing.T) {
	t.Parallel()

	// 42 days: lean mass falling 0.1 kg a day, body fat rising 0.02 %
	// points a day, 8000 steps a day but 5000 in the last week, and an
	// excluded day whose measurements are off.
	days := map[day.Date]Day{}
	line(days, last, 60, 80, -0.05, 2000)
	for i := range 60 {
		d := days[last.AddDays(-i)]
		d.LeanMassKg, d.BodyFatPct, d.Steps, d.ActiveKcal = 60+0.1*float64(i), 25-0.02*float64(i), 8000, 500
		if i < 7 {
			d.Steps = 5000
		}
		days[last.AddDays(-i)] = d
	}
	days[last.AddDays(-2)] = Day{Status: day.Excluded, WeightKg: 70, LeanMassKg: 40, BodyFatPct: 40, Steps: 30000, ActiveKcal: 5000}
	e := Calibrate(days, nil, last, DefaultParams())
	b := e.Body
	if !b.LeanMeasured || !near(b.LeanKgPerWeek, -0.7, 1e-6) || !b.BodyFatMeasured || !near(b.BodyFatPerWeek, 0.14, 1e-6) {
		t.Errorf("lean %v %v, body fat %v %v; want -0.7 kg and +0.14 points a week", b.LeanMeasured, b.LeanKgPerWeek, b.BodyFatMeasured, b.BodyFatPerWeek)
	}
	if b.StepsRecent != 5000 || b.StepsBefore != 8000 {
		t.Errorf("steps %v in the last week and %v before, want 5000 and 8000", b.StepsRecent, b.StepsBefore)
	}
	// The 6 days of the last 7 that are not excluded: the lean mass of
	// days 0, 1, 3, 4, 5 and 6 before last.
	n, lean := e.Now, 60+0.1*19/6
	if n.LoggedDays != 6 || n.WeighIns != 6 || !near(n.LeanMassKg, lean, 1e-9) || n.ActiveKcal != 500 {
		t.Errorf("now: %d logged, %d weighed, lean %v, active %v; want 6, 6, %v, 500", n.LoggedDays, n.WeighIns, n.LeanMassKg, n.ActiveKcal, lean)
	}
}

func TestCalibrateBodyIntervals(t *testing.T) {
	t.Parallel()

	// On lines, only the scale's step is uncertain.
	days := map[day.Date]Day{}
	line(days, last, 42, 80, -0.05, 2000)
	for i := range 42 {
		d := days[last.AddDays(-i)]
		d.LeanMassKg, d.BodyFatPct = 60+0.1*float64(i), 25-0.02*float64(i)
		days[last.AddDays(-i)] = d
	}
	b := Calibrate(days, nil, last, DefaultParams()).Body
	if b.LeanHalfWidth <= 0 || b.LeanHalfWidth > 0.01 || b.BodyFatHalfWidth <= 0 || b.BodyFatHalfWidth > 0.01 {
		t.Errorf("half the intervals %v and %v, want a little over 0", b.LeanHalfWidth, b.BodyFatHalfWidth)
	}
}

func TestCalibrateReadsLittleOfTheBody(t *testing.T) {
	t.Parallel()

	// Lean mass measured 5 times, steps on 3 of the last 7 days: too
	// little for either.
	days := map[day.Date]Day{}
	line(days, last, 42, 80, -0.05, 2000)
	for i := range 5 {
		d := days[last.AddDays(-7*i)]
		d.LeanMassKg, d.BodyFatPct = 60, 25
		days[last.AddDays(-7*i)] = d
	}
	for i := range 3 {
		d := days[last.AddDays(-i)]
		d.Steps = 6000
		days[last.AddDays(-i)] = d
	}
	e := Calibrate(days, nil, last, DefaultParams())
	if e.Body != (Body{}) || e.Now.ActiveKcal != 0 || e.Now.LeanMassKg != 60 {
		t.Errorf("body %+v, now %+v; want nothing measured, lean 60 now", e.Body, e.Now)
	}
}

func TestCalibrateTakesTheLeanMassFromTheBodyFat(t *testing.T) {
	t.Parallel()

	// A scale that reports the body fat, not the lean mass: 60 kg of it
	// throughout, while the fat goes at 0.05 kg a day.
	days := map[day.Date]Day{}
	for i := range 42 {
		w := 80 + 0.05*float64(i)
		days[last.AddDays(-i)] = Day{Status: day.Complete, Meals: 1, Kcal: 2000, WeightKg: w, BodyFatPct: 100 * (w - 60) / w}
	}
	e := Calibrate(days, nil, last, DefaultParams())
	if b := e.Body; !b.LeanMeasured || !near(b.LeanKgPerWeek, 0, 1e-9) || !b.BodyFatMeasured {
		t.Errorf("lean %v %v, body fat %v; want the lean mass level", b.LeanMeasured, b.LeanKgPerWeek, b.BodyFatMeasured)
	}
	if !near(e.Now.LeanMassKg, 60, 1e-9) {
		t.Errorf("lean mass now %v, want 60", e.Now.LeanMassKg)
	}
}

func TestCalibrateKnowsThePhasesStart(t *testing.T) {
	t.Parallel()

	// The trend the day before the phase started, and never from a
	// weigh-in past last: a phase that starts today, after today's.
	days := map[day.Date]Day{}
	line(days, last.AddDays(1), 100, 80, -0.05, 2000)
	trend := Trend(days, DefaultParams().TrendAlpha)
	for _, start := range []day.Date{last.AddDays(-50), last.AddDays(1)} {
		want, _ := trendAt(trend, start.AddDays(-1))
		phases := []plan.Phase{{Type: plan.Cut, Start: start, RatePctWeek: -0.5}}
		if e := Calibrate(days, phases, last, DefaultParams()); e.PhaseStartTrendKg != want || want == 0 {
			t.Errorf("a phase from %s: the trend at its start %v, want %v", start, e.PhaseStartTrendKg, want)
		}
	}
	if e := Calibrate(days, nil, last, DefaultParams()); e.PhaseStartTrendKg != 0 {
		t.Errorf("trend at the start of no phase %v, want 0", e.PhaseStartTrendKg)
	}
}

func TestStateOf(t *testing.T) {
	t.Parallel()

	today := last.AddDays(1)
	ph := phase(plan.Cut, -0.5)
	e := trusted(-0.5)
	e.TDEE, e.PhaseStartTrendKg = 2600, 84
	e.Now = Now{LoggedDays: 6, WeighIns: 5, LeanMassKg: 60, ActiveKcal: 450}
	s := StateOf(e, today, person, ph, target, training)
	if s.Today != today || s.Profile != person || s.TrendKg != 80 || s.Phase != ph || s.PhaseStartTrendKg != 84 ||
		s.Target != target || s.TrainingDays != training {
		t.Errorf("state %+v: the plan and the profile as given, the trend weights 80 and 84", s)
	}
	if s.TDEE != 2600 || !s.TDEETrusted || s.LeanMassKg != 60 || s.ActiveKcal != 450 {
		t.Errorf("TDEE %v trusted %v, lean %v, active %v; want 2600, true, 60, 450", s.TDEE, s.TDEETrusted, s.LeanMassKg, s.ActiveKcal)
	}
	// The days since the phase changed are the autopilot's to count: 0
	// holds its wait for a first step.
	want := limits.Autopilot{LoggedDays: 6, WeighIns: 5}
	if a := s.Autopilot; a != want {
		t.Errorf("autopilot %+v; want %+v: 6 logged, 5 weighed, nothing of the autopilot's", a, want)
	}
}

func TestStateOfLittle(t *testing.T) {
	t.Parallel()

	// Without an estimate there is no TDEE.
	if s := StateOf(Estimate{TrendKg: 80}, last.AddDays(1), person, nil, nil, 0); s.TDEE != 0 || s.TDEETrusted {
		t.Errorf("no estimate: TDEE %v trusted %v", s.TDEE, s.TDEETrusted)
	}
}
