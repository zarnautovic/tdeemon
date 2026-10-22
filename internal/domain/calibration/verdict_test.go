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
// moving rate % a week, with steps that did not change.
func trusted(rate float64) Estimate {
	return Estimate{
		Measured: true, Trusted: true, TrendKg: 80, KgPerWeek: plan.KgPerWeek(rate, 80),
		Body: Body{StepsRecent: 7000, StepsBefore: 7000},
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
		{"a cut on target", phase(plan.Cut, -0.5), -0.45, RateOnTarget, ActionHold, -44, 0},
		// A gap smaller than a step is the step.
		{"a cut a little too fast", phase(plan.Cut, -0.5), -0.61, RateBelow, ActionRaiseKcal, 96.8, 96.8},
		{"a bulk gaining too slowly", phase(plan.Bulk, 0.25), 0.05, RateBelow, ActionRaiseKcal, 176, 100},
		{"a bulk gaining too fast", phase(plan.Bulk, 0.25), 0.5, RateAbove, ActionLowerKcal, -220, -100},
		{"maintenance gaining", phase(plan.Maintain, 0), 0.15, RateAbove, ActionLowerKcal, -132, -100},
		{"a recomp on target", phase(plan.Recomp, -0.1), -0.05, RateOnTarget, ActionHold, -44, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := Judge(trusted(tc.measured), tc.ph, target, training, person, limits.UserDefaults(), DefaultParams())
			if v.Rate != tc.wantRate || v.Action != tc.wantAction || len(v.Guards) != 0 || len(v.Reasons) != 0 {
				t.Errorf("rate %s, action %s, guards %v, reasons %v; want %s and %s",
					v.Rate, v.Action, v.Guards, v.Reasons, tc.wantRate, tc.wantAction)
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
			if v.Rate != RateNone || v.Action != ActionNone || !slices.Equal(v.Reasons, tc.want) || v.StepKcal != 0 {
				t.Errorf("rate %s, action %s, step %v, reasons %v; want none for %v", v.Rate, v.Action, v.StepKcal, v.Reasons, tc.want)
			}
		})
	}
}

func TestJudgeGuards(t *testing.T) {
	t.Parallel()

	with := func(rate float64, change func(*Estimate)) Estimate {
		e := trusted(rate)
		change(&e)
		return e
	}
	leanLoss := func(e *Estimate) { e.Body.LeanMeasured, e.Body.LeanKgPerWeek = true, -0.6 }
	fatGain := func(e *Estimate) {
		e.Body.BodyFatMeasured, e.Body.BodyFatPerWeek, e.Body.BodyFatPerWeekSE = true, 0.1, 0.02
		e.Body.LeanMeasured, e.Body.LeanKgPerWeek = true, 0.05
	}
	for _, tc := range []struct {
		name       string
		e          Estimate
		ph         *plan.Phase
		pr         profile.Profile
		wantAction string
		wantGuards []string
		wantStep   float64
	}{
		// 58 kg at 180 cm is a BMI of 17.9: no deeper loss, maintenance.
		{
			"a loss below a BMI of 18.5", with(-0.5, func(e *Estimate) { e.TrendKg, e.KgPerWeek = 58, plan.KgPerWeek(-0.5, 58) }),
			phase(plan.Cut, -0.5), person, ActionMaintain,
			[]string{GuardUnderweight},
			0,
		},
		{"the underweight guard before the lean mass's", with(-0.5, func(e *Estimate) {
			e.TrendKg, e.KgPerWeek = 58, plan.KgPerWeek(-0.5, 58)
			leanLoss(e)
		}), phase(plan.Cut, -0.5), person, ActionMaintain, []string{GuardUnderweight}, 0},
		{
			"no height, no BMI", with(-0.5, func(e *Estimate) { e.TrendKg, e.KgPerWeek = 58, plan.KgPerWeek(-0.5, 58) }),
			phase(plan.Cut, -0.5),
			profile.Profile{},
			ActionHold, nil, 0,
		},
		{"lean mass lost on a cut", with(-0.2, leanLoss), phase(plan.Cut, -0.5), person, ActionRaiseProteinOrKcal, []string{GuardLeanMass}, 100},
		{"lean mass lost on a recomp", with(-0.1, leanLoss), phase(plan.Recomp, -0.1), person, ActionRaiseProteinOrKcal, []string{GuardLeanMass}, 100},
		{"lean mass lost on a bulk", with(0.25, leanLoss), phase(plan.Bulk, 0.25), person, ActionHold, nil, 0},
		{"fat gained on a bulk", with(0.25, fatGain), phase(plan.Bulk, 0.25), person, ActionShrinkSurplus, []string{GuardFatGain}, -100},
		{"fat gained with the lean mass", with(0.25, func(e *Estimate) {
			fatGain(e)
			e.Body.LeanKgPerWeek = 0.2
		}), phase(plan.Bulk, 0.25), person, ActionHold, nil, 0},
		{"body fat within its error", with(0.25, func(e *Estimate) {
			fatGain(e)
			e.Body.BodyFatPerWeekSE = 0.1
		}), phase(plan.Bulk, 0.25), person, ActionHold, nil, 0},
		// 4500 steps against 7000 is 36 % fewer: the slower loss is the
		// activity's.
		{"fewer steps", with(-0.2, func(e *Estimate) { e.Body.StepsRecent = 4500 }), phase(plan.Cut, -0.5), person, ActionHold, []string{GuardActivity}, 0},
		{"more steps", with(-0.8, func(e *Estimate) { e.Body.StepsRecent = 9500 }), phase(plan.Cut, -0.5), person, ActionHold, []string{GuardActivity}, 0},
		{"steps 25 % fewer", with(-0.2, func(e *Estimate) { e.Body.StepsRecent = 5250 }), phase(plan.Cut, -0.5), person, ActionLowerKcal, nil, -100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := Judge(tc.e, tc.ph, target, training, tc.pr, limits.UserDefaults(), DefaultParams())
			if v.Action != tc.wantAction || !slices.Equal(v.Guards, tc.wantGuards) || !near(v.StepKcal, tc.wantStep, 1e-6) {
				t.Errorf("action %s, guards %v, step %v; want %s, %v, %v", v.Action, v.Guards, v.StepKcal, tc.wantAction, tc.wantGuards, tc.wantStep)
			}
		})
	}
}

func TestJudgeTheUnderweightGap(t *testing.T) {
	t.Parallel()

	// The gap is to maintenance: 0.5 % a week of 58 kg is 0.29 kg, 319
	// kcal a day.
	e := trusted(-0.5)
	e.TrendKg, e.KgPerWeek = 58, plan.KgPerWeek(-0.5, 58)
	if v := Judge(e, phase(plan.Cut, -0.5), target, training, person, limits.UserDefaults(), DefaultParams()); !near(v.GapKcal, 319, 1e-6) {
		t.Errorf("gap %v, want 319 to maintenance", v.GapKcal)
	}
}

func TestJudgeFlags(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		change func(*Estimate)
		ph     *plan.Phase
		want   []string
	}{
		{"steps", func(*Estimate) {}, phase(plan.Cut, -0.5), nil},
		{"no steps", func(e *Estimate) { e.Body = Body{} }, phase(plan.Cut, -0.5), []string{FlagNoSteps}},
		// 83 kg against 80 at the phase's start is 3.75 % off.
		{"maintenance drifting", func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 83, 80 }, phase(plan.Maintain, 0), []string{FlagMaintenanceDrift}},
		{"maintenance within 3 %", func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 82, 80 }, phase(plan.Maintain, 0), nil},
		{"a cut 3 % down", func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 77, 80 }, phase(plan.Cut, -0.5), nil},
		{"no start", func(e *Estimate) { e.TrendKg, e.PhaseStartTrendKg = 83, 0 }, phase(plan.Maintain, 0), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := trusted(0)
			tc.change(&e)
			if v := Judge(e, tc.ph, target, training, person, limits.UserDefaults(), DefaultParams()); !slices.Equal(v.Flags, tc.want) {
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

func TestCalibrateKnowsThePhasesStart(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last, 100, 80, -0.05, 2000)
	phases := []plan.Phase{{Type: plan.Cut, Start: last.AddDays(-50), RatePctWeek: -0.5}}
	want, _ := trendOn(days, last.AddDays(-50), DefaultParams().TrendAlpha)
	if e := Calibrate(days, phases, last, DefaultParams()); e.PhaseStartTrendKg != want || want == 0 {
		t.Errorf("trend at the phase's start %v, want %v", e.PhaseStartTrendKg, want)
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
	want := limits.Autopilot{LoggedDays: 6, WeighIns: 5, DaysSincePhaseChange: 61}
	if a := s.Autopilot; a != want {
		t.Errorf("autopilot %+v; want %+v: 6 logged, 5 weighed, 61 days in the phase, no step", a, want)
	}
}

func TestStateOfLittle(t *testing.T) {
	t.Parallel()

	// Without an estimate there is no TDEE, and without a phase no days
	// in it.
	if s := StateOf(Estimate{TrendKg: 80}, last.AddDays(1), person, nil, nil, 0); s.TDEE != 0 || s.TDEETrusted || s.Autopilot.DaysSincePhaseChange != 0 {
		t.Errorf("no estimate, no phase: TDEE %v trusted %v, %d days", s.TDEE, s.TDEETrusted, s.Autopilot.DaysSincePhaseChange)
	}
}
