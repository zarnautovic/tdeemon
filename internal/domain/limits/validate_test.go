package limits

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

func date(tb testing.TB, s string) day.Date {
	tb.Helper()
	d, err := day.ParseDate(s)
	if err != nil {
		tb.Fatalf("ParseDate(%s): %v", s, err)
	}
	return d
}

// A made-up man of 36, 180 cm tall: a BMI of 18.5 is 59.94 kg, of 30
// 97.2 kg.
func base(tb testing.TB) State {
	tb.Helper()
	training, err := plan.WeekdaysOf(time.Monday, time.Wednesday, time.Friday)
	if err != nil {
		tb.Fatal(err)
	}
	return State{
		Today:        date(tb, "2026-10-21"),
		Profile:      profile.Profile{Sex: profile.Male, BirthDate: date(tb, "1990-05-10"), HeightCm: 180},
		TrendKg:      85,
		TrainingDays: training,
	}
}

func levels() Levels { return Levels{Instance: Instance(), User: UserDefaults()} }

func phase(tb testing.TB, t plan.PhaseType, start string, rate float64) *plan.Phase {
	tb.Helper()
	return &plan.Phase{Type: t, Start: date(tb, start), RatePctWeek: rate}
}

func unsplit(kcal, protein, fat float64) *plan.Target {
	d := plan.DayTarget{Kcal: kcal, Protein: protein, Fat: fat}
	return &plan.Target{Training: d, Rest: d}
}

// violations lists what a result refuses, as "rule level day-type".
func violations(r Result) []string {
	var out []string
	for _, v := range r.Violations {
		out = append(out, strings.TrimSpace(fmt.Sprintf("%s %s %s", v.Rule, v.Level, v.DayType)))
	}
	return out
}

type validateCase struct {
	name   string
	state  func(*State)
	change Change
	levels func(*Levels)
	want   []string
}

func runValidate(t *testing.T, cases []validateCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, lv := base(t), levels()
			if tc.state != nil {
				tc.state(&s)
			}
			if tc.levels != nil {
				tc.levels(&lv)
			}
			r := Validate(tc.change, s, lv)
			if got := violations(r); !slices.Equal(got, tc.want) {
				t.Errorf("Validate = %q, want %q", got, tc.want)
			}
			if r.OK() != (len(tc.want) == 0) {
				t.Errorf("OK() = %v with %q", r.OK(), violations(r))
			}
		})
	}
}

func TestValidateNothing(t *testing.T) {
	t.Parallel()

	if r := Validate(Change{}, base(t), levels()); !r.OK() || r.Warnings != nil {
		t.Errorf("Validate of no change = %+v", r)
	}
	// A target that would warn is not checked when nothing changes.
	s := base(t)
	s.Target = unsplit(1700, 170, 60)
	if r := Validate(Change{}, s, levels()); !r.OK() || r.Warnings != nil {
		t.Errorf("Validate of no change with a target = %+v", r)
	}
	// New training days with no target move nothing.
	days := plan.Weekdays(0)
	if r := Validate(Change{TrainingDays: &days}, State{}, levels()); !r.OK() {
		t.Errorf("Validate of training days with no target = %+v", r)
	}
}

func TestValidateNeeds(t *testing.T) {
	t.Parallel()

	cut := phase(t, plan.Cut, "2026-10-26", -0.5)
	runValidate(t, []validateCase{
		{"no sex", func(s *State) { s.Profile.Sex = "" }, Change{Phase: cut}, nil, []string{"profile_missing"}},
		{"no height", func(s *State) { s.Profile.HeightCm = 0 }, Change{Target: unsplit(2500, 170, 75)}, nil, []string{"profile_missing"}},
		{"no trend weight", func(s *State) { s.TrendKg = 0 }, Change{Phase: cut}, nil, []string{"weight_missing"}},
		{"neither", func(s *State) { s.Profile = profile.Profile{}; s.TrendKg = 0 }, Change{Phase: cut}, nil, []string{"profile_missing", "weight_missing"}},
		// Under 18 nothing else is checked: there is no plan.
		{"under 18", func(s *State) { s.Profile.BirthDate = date(t, "2008-10-22") }, Change{Phase: phase(t, plan.Cut, "2026-10-26", -0.8)}, nil, []string{"min_age code"}},
		{"18 today", func(s *State) { s.Profile.BirthDate = date(t, "2008-10-21") }, Change{Phase: cut}, nil, nil},
		{"training days with a target, and no profile", func(s *State) {
			s.Target = unsplit(2500, 170, 75)
			s.Profile = profile.Profile{}
		}, Change{TrainingDays: new(plan.Weekdays(0))}, nil, []string{"profile_missing"}},
	})
}

func TestValidatePhaseRate(t *testing.T) {
	t.Parallel()

	start := "2026-10-26"
	current := func(rate, startKg float64) func(*State) {
		return func(s *State) {
			s.Phase = phase(t, plan.Cut, "2026-09-07", rate)
			s.PhaseStartTrendKg = startKg
		}
	}
	runValidate(t, []validateCase{
		{"a cut", nil, Change{Phase: phase(t, plan.Cut, start, -0.5)}, nil, nil},
		{"a cut at the user's limit", nil, Change{Phase: phase(t, plan.Cut, start, -0.7)}, nil, nil},
		{"a cut past the user's limit", nil, Change{Phase: phase(t, plan.Cut, start, -0.8)}, nil, []string{"cut_rate user"}},
		{"a cut just past it", nil, Change{Phase: phase(t, plan.Cut, start, -0.71)}, nil, []string{"cut_rate user"}},
		{"a cut past the instance's", nil, Change{Phase: phase(t, plan.Cut, start, -1.1)}, nil, []string{"cut_rate instance"}},
		// No configuration allows a rate past the code's.
		{"a cut past the code's loss", nil, Change{Phase: phase(t, plan.Cut, start, -1.6)}, nil, []string{"cut_rate code"}},
		{"a bulk past the code's gain", nil, Change{Phase: phase(t, plan.Bulk, start, 1.1)}, nil, []string{"bulk_rate code"}},
		{"a recomp losing past it", nil, Change{Phase: phase(t, plan.Recomp, start, -1.6)}, nil, []string{"recomp_rate code"}},
		{"a recomp gaining past it", nil, Change{Phase: phase(t, plan.Recomp, start, 1.1)}, nil, []string{"recomp_rate code"}},
		{"a cut the user loosened", nil, Change{Phase: phase(t, plan.Cut, start, -0.9)}, func(lv *Levels) { lv.User.CutRate = -1.0 }, nil},
		{
			"a user's limit past the instance's holds no more", nil,
			Change{Phase: phase(t, plan.Cut, start, -1.1)},
			func(lv *Levels) { lv.User.CutRate = -1.5 },
			[]string{"cut_rate instance"},
		},
		// 110 kg at 180 cm is a BMI of 34.
		{"an obese cut", func(s *State) { s.TrendKg = 110 }, Change{Phase: phase(t, plan.Cut, start, -0.9)}, nil, nil},
		{"an obese cut past the user's limit", func(s *State) { s.TrendKg = 110 }, Change{Phase: phase(t, plan.Cut, start, -1.1)}, nil, []string{"cut_rate user"}},
		{"an obese cut past the instance's", func(s *State) { s.TrendKg = 110 }, Change{Phase: phase(t, plan.Cut, start, -1.3)}, nil, []string{"cut_rate instance"}},
		// −1.0 % of 160 kg is 1.6 kg a week.
		{"more than 1.5 kg a week", func(s *State) { s.TrendKg = 160 }, Change{Phase: phase(t, plan.Cut, start, -1.0)}, nil, []string{"loss_kg_week instance"}},
		{"1.44 kg a week", func(s *State) { s.TrendKg = 160 }, Change{Phase: phase(t, plan.Cut, start, -0.9)}, nil, nil},
		// The BMI when the phase started picks the row: 100 kg then (30.9),
		// 95 kg now (29.3).
		{
			"an edit keeps the row of the phase's start", func(s *State) { current(-0.5, 100)(s); s.TrendKg = 95 },
			Change{Phase: phase(t, plan.Cut, "2026-09-07", -0.9)},
			nil, nil,
		},
		{
			"a new phase takes the BMI now", func(s *State) { current(-0.5, 100)(s); s.TrendKg = 95 },
			Change{Phase: phase(t, plan.Cut, start, -0.9)},
			nil,
			[]string{"cut_rate user"},
		},
		{
			"a phase that started with no known weight takes the BMI now", func(s *State) { current(-0.5, 0)(s); s.TrendKg = 110 },
			Change{Phase: phase(t, plan.Cut, "2026-09-07", -0.9)},
			nil,
			nil,
		},
		{"a new type on the same start is a new phase", func(s *State) {
			current(-0.5, 100)(s)
			s.Phase.Type, s.Phase.RatePctWeek = plan.Bulk, 0.1
			s.TrendKg = 95
		}, Change{Phase: phase(t, plan.Cut, "2026-09-07", -0.9)}, nil, []string{"cut_rate user"}},
		{"a bulk", nil, Change{Phase: phase(t, plan.Bulk, start, 0.25)}, nil, nil},
		{"a bulk past the user's limit", nil, Change{Phase: phase(t, plan.Bulk, start, 0.3)}, nil, []string{"bulk_rate user"}},
		{"a bulk past the instance's", nil, Change{Phase: phase(t, plan.Bulk, start, 0.6)}, nil, []string{"bulk_rate instance"}},
		{"a recomp at its least", nil, Change{Phase: phase(t, plan.Recomp, start, -0.5)}, nil, nil},
		{"a recomp at its most", nil, Change{Phase: phase(t, plan.Recomp, start, 0.25)}, nil, nil},
		{"a recomp losing too fast", nil, Change{Phase: phase(t, plan.Recomp, start, -0.6)}, nil, []string{"recomp_rate instance"}},
		{"a recomp gaining too fast", nil, Change{Phase: phase(t, plan.Recomp, start, 0.3)}, nil, []string{"recomp_rate instance"}},
		{
			"a recomp the user narrowed", nil,
			Change{Phase: phase(t, plan.Recomp, start, -0.3)},
			func(lv *Levels) { lv.User.RecompMin = -0.2 },
			[]string{"recomp_rate user"},
		},
		{"maintenance", nil, Change{Phase: phase(t, plan.Maintain, start, 0)}, nil, nil},
		// A rate new data or a tightened limit put past it: a change toward
		// the limit passes, one away from it does not, and one that leaves
		// it does not set it.
		{"toward the limit", current(-1.0, 85), Change{Phase: phase(t, plan.Cut, "2026-09-07", -0.8)}, nil, nil},
		{"away from it", current(-1.0, 85), Change{Phase: phase(t, plan.Cut, "2026-09-07", -1.1)}, nil, []string{"cut_rate instance"}},
		{"the rate left as it is", current(-1.0, 85), Change{Phase: &plan.Phase{Type: plan.Cut, Start: date(t, "2026-09-07"), RatePctWeek: -1.0, GoalWeight: new(80.0)}}, nil, nil},
		{"the same rate in a new phase", current(-1.0, 85), Change{Phase: phase(t, plan.Cut, start, -1.0)}, nil, []string{"cut_rate user"}},
	})
}

func TestValidateUnderweight(t *testing.T) {
	t.Parallel()

	start := "2026-10-26"
	thin := func(s *State) { s.TrendKg = 59 } // a BMI of 18.2
	cutting := func(rate float64) func(*State) {
		return func(s *State) {
			thin(s)
			s.Phase = phase(t, plan.Cut, "2026-09-07", rate)
			s.PhaseStartTrendKg = 62
		}
	}
	withGoal := func(goal, milestone *float64) *plan.Phase {
		p := phase(t, plan.Cut, start, -0.5)
		p.GoalWeight, p.MilestoneWeight = goal, milestone
		return p
	}
	runValidate(t, []validateCase{
		{"a cut starts a loss", thin, Change{Phase: phase(t, plan.Cut, start, -0.3)}, nil, []string{"underweight_loss code"}},
		{"a recomp below 0 starts one", thin, Change{Phase: phase(t, plan.Recomp, start, -0.1)}, nil, []string{"underweight_loss code"}},
		{"a recomp above 0 does not", thin, Change{Phase: phase(t, plan.Recomp, start, 0.1)}, nil, nil},
		{"maintenance does not", thin, Change{Phase: phase(t, plan.Maintain, start, 0)}, nil, nil},
		{"a slower loss under way", cutting(-0.5), Change{Phase: phase(t, plan.Cut, "2026-09-07", -0.3)}, nil, nil},
		{"a deeper loss under way", cutting(-0.5), Change{Phase: phase(t, plan.Cut, "2026-09-07", -0.6)}, nil, []string{"underweight_loss code"}},
		{"a cut at a BMI of 18.5", func(s *State) { s.TrendKg = 59.94 }, Change{Phase: phase(t, plan.Cut, start, -0.3)}, nil, nil},
		{"a goal below a BMI of 18.5", nil, Change{Phase: withGoal(new(59.0), nil)}, nil, []string{"underweight_goal code"}},
		{"a milestone below it", nil, Change{Phase: withGoal(nil, new(59.0))}, nil, []string{"underweight_milestone code"}},
		{"a goal at it", nil, Change{Phase: withGoal(new(59.94), new(60.0))}, nil, nil},
		{"a goal left as it is", func(s *State) {
			s.Phase = withGoal(new(59.0), nil)
			s.PhaseStartTrendKg = 85
		}, Change{Phase: withGoal(new(59.0), nil)}, nil, nil},
		{"a goal changed below it", func(s *State) {
			s.Phase = withGoal(new(59.0), nil)
			s.PhaseStartTrendKg = 85
		}, Change{Phase: withGoal(new(58.0), nil)}, nil, []string{"underweight_goal code"}},
	})
}

func TestValidateTargetDays(t *testing.T) {
	t.Parallel()

	woman := func(s *State) {
		s.Profile = profile.Profile{Sex: profile.Female, BirthDate: date(t, "1988-02-14"), HeightCm: 165}
		s.TrendKg = 60
	}
	split := &plan.Target{Training: plan.DayTarget{Kcal: 2600, Protein: 170, Fat: 75}, Rest: plan.DayTarget{Kcal: 1400, Protein: 170, Fat: 60}}
	runValidate(t, []validateCase{
		{"a target", nil, Change{Target: unsplit(2500, 170, 75)}, nil, nil},
		{"under a man's floor", nil, Change{Target: unsplit(1400, 170, 60)}, nil, []string{"kcal_floor user"}},
		{"under the instance's", nil, Change{Target: unsplit(1100, 150, 40)}, nil, []string{"kcal_floor instance"}},
		{"at 801 kcal", nil, Change{Target: unsplit(801, 136, 25)}, nil, []string{"kcal_floor instance"}},
		{"at the code's 800", nil, Change{Target: unsplit(800, 136, 25)}, nil, []string{"kcal_floor code"}},
		{"a woman's floor", woman, Change{Target: unsplit(1200, 110, 40)}, nil, nil},
		{"under a woman's floor", woman, Change{Target: unsplit(1150, 110, 40)}, nil, []string{"kcal_floor instance"}},
		{"a rest day under the floor", nil, Change{Target: split}, nil, []string{"kcal_floor user rest"}},
		{"fat under the user's floor", nil, Change{Target: unsplit(2400, 170, 50)}, nil, []string{"fat_floor user"}},
		{"fat under the instance's", nil, Change{Target: unsplit(2400, 170, 35)}, nil, []string{"fat_floor instance"}},
		{"fat at 14.25 %", nil, Change{Target: unsplit(2400, 170, 38)}, nil, []string{"fat_floor instance"}},
		// 85 kg is the reference mass: 1.6 g/kg is 136 g.
		{"protein under the user's floor", nil, Change{Target: unsplit(2500, 120, 75)}, nil, []string{"protein_floor user"}},
		{"protein under the code's floor", nil, Change{Target: unsplit(2500, 60, 75)}, nil, []string{"protein_floor code"}},
		{"protein over the ceiling", nil, Change{Target: unsplit(2500, 270, 75)}, nil, []string{"protein_ceiling instance"}},
		{
			"protein under the user's grams", nil,
			Change{Target: unsplit(2500, 170, 75)},
			func(lv *Levels) { lv.User.ProteinFloorGrams = 180 },
			[]string{"protein_floor_grams user"},
		},
		// Above a BMI of 30 the reference mass is the weight at 30, 97.2
		// kg: 150 g is 1.54 g/kg of it.
		{
			"protein of a reference mass capped at a BMI of 30", func(s *State) { s.TrendKg = 120 },
			Change{Target: unsplit(2500, 150, 75)},
			nil,
			[]string{"protein_floor user"},
		},
		{"protein of the capped mass, enough", func(s *State) { s.TrendKg = 120 }, Change{Target: unsplit(2500, 160, 75)}, nil, nil},
		// A value new data put past a limit: protein of 120 g at 85 kg.
		{
			"the kcal changed, the protein left as it is", func(s *State) { s.Target = unsplit(2500, 120, 75) },
			Change{Target: unsplit(2400, 120, 75)},
			nil, nil,
		},
		{"protein toward the floor", func(s *State) { s.Target = unsplit(2500, 120, 75) }, Change{Target: unsplit(2500, 130, 75)}, nil, nil},
		{
			"protein away from it", func(s *State) { s.Target = unsplit(2500, 120, 75) },
			Change{Target: unsplit(2500, 110, 75)},
			nil,
			[]string{"protein_floor user"},
		},
		// A kcal step sets the fat share.
		{
			"a kcal step that takes the fat share under the floor", func(s *State) { s.Target = unsplit(2400, 170, 55) },
			Change{Target: unsplit(2600, 170, 55)},
			nil,
			[]string{"fat_floor user"},
		},
		{
			"the fat share left as it is", func(s *State) { s.Target = unsplit(2400, 170, 50) },
			Change{Target: unsplit(2400, 160, 50)},
			nil, nil,
		},
		{"a split where only the rest day changes", func(s *State) {
			s.Target = &plan.Target{Training: split.Training, Rest: plan.DayTarget{Kcal: 1600, Protein: 170, Fat: 60}}
		}, Change{Target: split}, nil, []string{"kcal_floor user rest"}},
		{
			"a split target made unsplit", func(s *State) { s.Target = split },
			Change{Target: unsplit(1400, 170, 60)},
			nil,
			[]string{"kcal_floor user training"},
		},
	})
}

func TestValidateImplied(t *testing.T) {
	t.Parallel()

	trusted := func(s *State) {
		s.TrendKg, s.TDEE, s.TDEETrusted = 80, 2600, true
	}
	runValidate(t, []validateCase{
		// 700 kcal a day under 2600: 0.64 kg, 0.80 % of 80 kg a week.
		{"a loss past the user's limit", trusted, Change{Target: unsplit(1900, 170, 60)}, nil, []string{"implied_loss_rate user"}},
		{"a loss within it", trusted, Change{Target: unsplit(2000, 170, 60)}, nil, nil},
		{"a loss before calibration trusts the TDEE", func(s *State) { trusted(s); s.TDEETrusted = false }, Change{Target: unsplit(1900, 170, 60)}, nil, nil},
		{"a gain past the user's limit", trusted, Change{Target: unsplit(2900, 170, 75)}, nil, []string{"implied_gain_rate user"}},
		{"a gain past the instance's", trusted, Change{Target: unsplit(3100, 170, 75)}, nil, []string{"implied_gain_rate instance"}},
		// 1000 kcal a day over is 0.91 kg a week, 1.14 % of 80 kg.
		{"a gain past the code's", trusted, Change{Target: unsplit(3600, 170, 85)}, nil, []string{"implied_gain_rate code"}},
		// 1500 kcal a day under 3200 is 1.36 kg a week, 1.70 % of 80 kg.
		{"a loss past the code's", func(s *State) { trusted(s); s.TDEE = 3200 }, Change{Target: unsplit(1700, 140, 40)}, nil, []string{"implied_loss_rate code"}},
		// Maintenance at 2650 kcal with a TDEE of 2600 passes.
		{
			"a slow gain in maintenance", func(s *State) { trusted(s); s.Phase = phase(t, plan.Maintain, "2026-09-07", 0) },
			Change{Target: unsplit(2650, 170, 75)},
			nil, nil,
		},
		// 1700 kcal a day under is 1.55 kg a week: 0.97 % of 160 kg, within
		// the obese row, but more than 1.5 kg.
		{
			"more than 1.5 kg a week", func(s *State) { trusted(s); s.TrendKg = 160; s.TDEE = 3700 },
			Change{Target: unsplit(2000, 170, 60)},
			nil,
			[]string{"implied_loss_kg_week instance"},
		},
		{
			"protein alone leaves the rate", func(s *State) { trusted(s); s.Target = unsplit(1900, 170, 60) },
			Change{Target: unsplit(1900, 180, 60)},
			nil, nil,
		},
		{
			"toward the limit", func(s *State) { trusted(s); s.Target = unsplit(1800, 170, 60) },
			Change{Target: unsplit(1900, 170, 60)},
			nil, nil,
		},
		{
			"away from it", func(s *State) { trusted(s); s.Target = unsplit(1900, 170, 60) },
			Change{Target: unsplit(1850, 170, 60)},
			nil,
			[]string{"implied_loss_rate user"},
		},
		// The weekly average of 2300 on rest days and 2900 on training days
		// moves with the number of training days.
		{"training days that raise the average", func(s *State) {
			trusted(s)
			s.TrainingDays = 0
			s.Target = &plan.Target{Training: plan.DayTarget{Kcal: 2900, Protein: 170, Fat: 75}, Rest: plan.DayTarget{Kcal: 2300, Protein: 170, Fat: 75}}
		}, Change{TrainingDays: new(plan.Weekdays(1<<7 - 1))}, nil, []string{"implied_gain_rate user"}},
		{"training days before the TDEE is trusted", func(s *State) {
			s.TrainingDays = 0
			s.Target = &plan.Target{Training: plan.DayTarget{Kcal: 2900, Protein: 170, Fat: 75}, Rest: plan.DayTarget{Kcal: 2300, Protein: 170, Fat: 75}}
		}, Change{TrainingDays: new(plan.Weekdays(1<<7 - 1))}, nil, nil},
		{"a loss below a BMI of 18.5", func(s *State) {
			s.TrendKg, s.TDEE, s.TDEETrusted = 59, 2200, true
			s.Target = unsplit(2200, 120, 60)
		}, Change{Target: unsplit(2000, 120, 60)}, nil, []string{"underweight_loss code"}},
		{"a smaller loss below it", func(s *State) {
			s.TrendKg, s.TDEE, s.TDEETrusted = 59, 2200, true
			s.Target = unsplit(1900, 120, 60)
		}, Change{Target: unsplit(2000, 120, 60)}, nil, nil},
	})
}

func TestValidateAutopilot(t *testing.T) {
	t.Parallel()

	since := func(n int) *int { return &n }
	ready := func(s *State) {
		s.Phase = phase(t, plan.Cut, "2026-09-07", -0.5)
		s.PhaseStartTrendKg = 85
		s.Target = unsplit(2400, 170, 70)
		s.Autopilot = Autopilot{DaysSinceStep: since(7), DaysSincePhaseChange: 30, LoggedDays: 6, WeighIns: 5, DriftBaseTDEE: 2600}
	}
	and := func(f func(*State)) func(*State) { return func(s *State) { ready(s); f(s) } }
	step := func(kcal float64) Change { return Change{Target: unsplit(2400+kcal, 170, 70), By: ByAutopilot} }
	runValidate(t, []validateCase{
		{"a step", ready, step(-100), nil, nil},
		{"a step past the user's limit", ready, step(-101), nil, []string{"autopilot_step user"}},
		{"a step past the instance's", ready, step(-210), nil, []string{"autopilot_step instance"}},
		{"a step up", ready, step(100), nil, nil},
		// 5 % of 1800 is 90 kcal.
		{
			"a step past 5 %", and(func(s *State) { s.Target = unsplit(1800, 170, 60) }),
			Change{Target: unsplit(1705, 170, 60), By: ByAutopilot},
			nil,
			[]string{"autopilot_step user"},
		},
		{"the same step by a coach", ready, Change{Target: unsplit(2190, 170, 70)}, nil, nil},
		{"a step too soon", and(func(s *State) { s.Autopilot.DaysSinceStep = since(6) }), step(-50), nil, []string{"autopilot_interval instance"}},
		{"the first step of a phase", and(func(s *State) { s.Autopilot.DaysSinceStep = nil }), step(-50), nil, nil},
		{
			"too soon after the phase started", and(func(s *State) { s.Autopilot.DaysSincePhaseChange = 13 }), step(-50), nil,
			[]string{"autopilot_first_step instance"},
		},
		{"a longer wait the user set", ready, step(-50), func(lv *Levels) { lv.User.IntervalDays = 10 }, []string{"autopilot_interval user"}},
		{"too few logged days", and(func(s *State) { s.Autopilot.LoggedDays = 3 }), step(-50), nil, []string{"autopilot_logged_days instance"}},
		{"too few weigh-ins", and(func(s *State) { s.Autopilot.WeighIns = 2 }), step(-50), nil, []string{"autopilot_weigh_ins instance"}},
		// 400 kcal from the base, 15.4 % of 2600.
		{"past the user's drift", and(func(s *State) { s.Autopilot.DriftKcal = -300 }), step(-100), nil, []string{"autopilot_drift user"}},
		{"a step too big, as a proposal", ready, Change{Target: unsplit(2250, 170, 70), By: ByAutopilotProposal}, nil, []string{"autopilot_step user"}},
		// 10 % of 1800 is 180 kcal, less than 200.
		{"a step past 10 %", and(func(s *State) { s.Target = unsplit(1800, 170, 60) }), Change{Target: unsplit(1615, 170, 60), By: ByAutopilot}, nil, []string{"autopilot_step instance"}},
		{"past it, as a proposal", and(func(s *State) { s.Autopilot.DriftKcal = -300 }), Change{Target: unsplit(2300, 170, 70), By: ByAutopilotProposal}, nil, nil},
		{"past the instance's drift", and(func(s *State) { s.Autopilot.DriftKcal = -700 }), step(-50), nil, []string{"autopilot_drift instance"}},
		{"back toward the base", and(func(s *State) { s.Autopilot.DriftKcal = -700 }), step(50), nil, nil},
		{"a drift a step up undoes", and(func(s *State) { s.Autopilot.DriftKcal = 380 }), step(-100), nil, nil},
		{"no TDEE to measure the drift", and(func(s *State) { s.Autopilot.DriftBaseTDEE = 0 }), step(-50), nil, []string{"tdee_missing"}},
		{"no TDEE, as a proposal", and(func(s *State) { s.Autopilot.DriftBaseTDEE = 0 }), Change{Target: unsplit(2350, 170, 70), By: ByAutopilotProposal}, nil, nil},
		{"a step during a loss below a BMI of 18.5", and(func(s *State) { s.TrendKg = 59 }), step(50), nil, []string{"underweight_loss code"}},
		// A step down deepens the loss too: the violation once.
		{"a step down during a loss below a BMI of 18.5", and(func(s *State) { s.TrendKg = 59 }), step(-50), nil, []string{"underweight_loss code"}},
		{"a step below it in maintenance", and(func(s *State) {
			s.TrendKg = 59
			s.Phase = phase(t, plan.Maintain, "2026-09-07", 0)
		}), step(50), nil, nil},
		{"a step below it, the target implying a loss", and(func(s *State) {
			s.TrendKg = 59
			s.Phase = phase(t, plan.Maintain, "2026-09-07", 0)
			s.TDEE, s.TDEETrusted = 2600, true
		}), step(50), nil, []string{"underweight_loss code"}},
		{
			"a step with no target before", func(s *State) { ready(s); s.Target = nil },
			Change{Target: unsplit(2400, 170, 70), By: ByAutopilotProposal},
			nil,
			[]string{"autopilot_shape"},
		},
		{"a step that changes the protein", ready, Change{Target: unsplit(2350, 180, 70), By: ByAutopilot}, nil, []string{"autopilot_shape"}},
		{"a step that changes the fat", ready, Change{Target: unsplit(2350, 170, 75), By: ByAutopilotProposal}, nil, []string{"autopilot_shape"}},
		{"a step that changes the split", ready, Change{Target: &plan.Target{
			Training: plan.DayTarget{Kcal: 2500, Protein: 170, Fat: 70}, Rest: plan.DayTarget{Kcal: 2300, Protein: 170, Fat: 70},
		}, By: ByAutopilot}, nil, []string{"autopilot_shape"}},
		{
			"a step that changes the training day's protein", and(func(s *State) { s.Target = splitStep(0, 0, 0, 0, 0) }),
			Change{Target: splitStep(-50, 5, 0, 0, 0), By: ByAutopilot},
			nil,
			[]string{"autopilot_shape"},
		},
		{
			"a step that changes the training day's fat", and(func(s *State) { s.Target = splitStep(0, 0, 0, 0, 0) }),
			Change{Target: splitStep(-50, 0, 5, 0, 0), By: ByAutopilot},
			nil,
			[]string{"autopilot_shape"},
		},
		{
			"a step that changes the rest day's protein", and(func(s *State) { s.Target = splitStep(0, 0, 0, 0, 0) }),
			Change{Target: splitStep(-50, 0, 0, 5, 0), By: ByAutopilot},
			nil,
			[]string{"autopilot_shape"},
		},
		{
			"a step that changes the rest day's fat", and(func(s *State) { s.Target = splitStep(0, 0, 0, 0, 0) }),
			Change{Target: splitStep(-50, 0, 0, 0, 5), By: ByAutopilot},
			nil,
			[]string{"autopilot_shape"},
		},
		{"a step that moves a split", and(func(s *State) {
			s.Target = &plan.Target{Training: plan.DayTarget{Kcal: 2600, Protein: 170, Fat: 70}, Rest: plan.DayTarget{Kcal: 2200, Protein: 170, Fat: 70}}
		}), Change{Target: &plan.Target{
			Training: plan.DayTarget{Kcal: 2550, Protein: 170, Fat: 70}, Rest: plan.DayTarget{Kcal: 2150, Protein: 170, Fat: 70},
		}, By: ByAutopilot}, nil, nil},
		{
			"a step that also sets a phase", ready,
			Change{Phase: phase(t, plan.Cut, "2026-09-07", -0.5), Target: unsplit(2350, 170, 70), By: ByAutopilot},
			nil,
			[]string{"autopilot_shape"},
		},
		{
			"a step that also sets training days", ready,
			Change{Target: unsplit(2350, 170, 70), TrainingDays: new(plan.Weekdays(1)), By: ByAutopilot},
			nil,
			[]string{"autopilot_shape"},
		},
	})
}

// splitStep is a split target of 2600 kcal on training days and 2200 on
// rest days moved by kcal, with the grams of protein and fat of each day
// type moved by the rest.
func splitStep(kcal, trainingProtein, trainingFat, restProtein, restFat float64) *plan.Target {
	return &plan.Target{
		Training: plan.DayTarget{Kcal: 2600 + kcal, Protein: 170 + trainingProtein, Fat: 70 + trainingFat},
		Rest:     plan.DayTarget{Kcal: 2200 + kcal, Protein: 170 + restProtein, Fat: 70 + restFat},
	}
}

func TestValidateWarnings(t *testing.T) {
	t.Parallel()

	// The man's BMR at 85 kg is 1800 kcal.
	s := base(t)
	r := Validate(Change{Target: unsplit(1700, 170, 60)}, s, levels())
	if !r.OK() {
		t.Fatalf("Validate = %q", violations(r))
	}
	want := []Warning{
		{Rule: WarnBelowBMR, Threshold: 1800, Value: 1700},
		{Rule: WarnLowCarbs, Threshold: 130, Value: 120},
	}
	if !slices.Equal(r.Warnings, want) {
		t.Errorf("Warnings = %+v, want %+v", r.Warnings, want)
	}

	// Energy availability with a known lean mass: (1700 − 400) / 60.
	s.LeanMassKg, s.ActiveKcal = 60, 400
	r = Validate(Change{Target: unsplit(1700, 170, 60)}, s, levels())
	if !slices.Contains(r.Warnings, Warning{Rule: WarnLowEnergyAvailability, Threshold: 30, Value: 1300.0 / 60}) {
		t.Errorf("Warnings = %+v, want low energy availability", r.Warnings)
	}
	r = Validate(Change{Target: unsplit(2500, 170, 75)}, s, levels())
	if r.Warnings != nil {
		t.Errorf("Warnings of a target with none = %+v", r.Warnings)
	}
	// Without a lean mass there is no warning, whatever the active energy.
	s.LeanMassKg, s.ActiveKcal = 0, 3000
	r = Validate(Change{Target: unsplit(2500, 170, 75)}, s, levels())
	if r.Warnings != nil {
		t.Errorf("Warnings without a lean mass = %+v", r.Warnings)
	}

	// A split target warns for the day type that has it.
	split := &plan.Target{Training: plan.DayTarget{Kcal: 2600, Protein: 170, Fat: 75}, Rest: plan.DayTarget{Kcal: 1700, Protein: 170, Fat: 60}}
	r = Validate(Change{Target: split}, base(t), levels())
	if len(r.Warnings) != 2 || r.Warnings[0].DayType != plan.Rest || r.Warnings[1].DayType != plan.Rest {
		t.Errorf("Warnings of a split target = %+v, want the rest day's", r.Warnings)
	}
	// A violation keeps its limit and the value proposed.
	r = Validate(Change{Target: unsplit(1400, 170, 60)}, base(t), levels())
	if v := r.Violations[0]; v.Limit != 1500 || v.Proposed != 1400 {
		t.Errorf("Violation = %+v, want limit 1500 and 1400 proposed", v)
	}
}

// A value on a limit passes, though float arithmetic puts it a hair past:
// the defaults never pass a limit, and limits are inclusive.
func TestValidateAtTheEdge(t *testing.T) {
	t.Parallel()

	lv := levels()
	eff := Effective(lv.User, lv.Instance)
	// A cut's default at 1.5 kg a week, from 150 to 300 kg.
	for kg := 150.0; kg <= 300; kg += 0.5 {
		s := base(t)
		s.Profile.HeightCm = 200
		s.TrendKg = kg
		rate := DefaultRate(plan.Cut, s.Profile.BMI(kg), kg, eff)
		if r := Validate(Change{Phase: phase(t, plan.Cut, "2026-10-26", rate)}, s, lv); !r.OK() {
			t.Fatalf("the default cut at %v kg = %q", kg, violations(r))
		}
	}
	// Maintenance's protein, 1.6 g/kg, in grams and back.
	for kg := 50.0; kg <= 97; kg += 0.1 {
		s := base(t)
		s.TrendKg = kg
		grams := plan.ProteinGrams(DefaultProtein(plan.Maintain, kg, eff), kg)
		if r := Validate(Change{Target: unsplit(2800, grams, 80)}, s, lv); !r.OK() {
			t.Fatalf("1.6 g/kg at %v kg (%v g) = %q", kg, grams, violations(r))
		}
	}
	// Protein at the ceiling, 3.0 g/kg, in grams and back.
	for kg := 50.0; kg <= 97; kg += 0.1 {
		s := base(t)
		s.TrendKg = kg
		if r := Validate(Change{Target: unsplit(3200, plan.ProteinGrams(3.0, kg), 80)}, s, lv); !r.OK() {
			t.Fatalf("3.0 g/kg at %v kg = %q", kg, violations(r))
		}
	}
	// The code's floor holds whatever the levels say.
	near800 := levels()
	near800.Instance.KcalFloorMale, near800.User.KcalFloorMale = 800.0000001, 800.0000001
	if r := Validate(Change{Target: unsplit(800, 136, 25)}, base(t), near800); !slices.Equal(violations(r), []string{"kcal_floor code"}) || r.Violations[0].Limit != 800 {
		t.Errorf("800 kcal under a floor a hair over it = %+v", r.Violations)
	}
	// A hair over the code's 800 kcal is 800, which the code refuses.
	if r := Validate(Change{Target: unsplit(800+1e-13, 136, 25)}, base(t), lv); !slices.Equal(violations(r), []string{"kcal_floor code"}) {
		t.Errorf("800 kcal and a hair = %q", violations(r))
	}
	// Fat at exactly 20 % of the day.
	for kcal := 1500.0; kcal <= 4000; kcal++ {
		if r := Validate(Change{Target: unsplit(kcal, 170, kcal*0.2/9)}, base(t), lv); !r.OK() {
			t.Fatalf("20 %% fat at %v kcal = %q", kcal, violations(r))
		}
	}
}

// Below a BMI of 18.5 the floor in the code holds before calibration
// trusts the TDEE too: a target that lowers the weekly average is taken
// for a deeper loss.
func TestValidateUnderweightBeforeTheTDEE(t *testing.T) {
	t.Parallel()

	thin := func(s *State) {
		s.Profile = profile.Profile{Sex: profile.Female, BirthDate: date(t, "1990-03-01"), HeightCm: 165}
		s.TrendKg = 46 // a BMI of 16.9
		s.Phase = phase(t, plan.Maintain, "2026-09-07", 0)
		s.Target = unsplit(2000, 100, 60)
	}
	runValidate(t, []validateCase{
		{"a lower target", thin, Change{Target: unsplit(1250, 100, 40)}, nil, []string{"underweight_loss code"}},
		{"a higher target", thin, Change{Target: unsplit(2100, 100, 60)}, nil, nil},
		{"the same average, protein changed", thin, Change{Target: unsplit(2000, 110, 60)}, nil, nil},
		{"training days that lower the average", func(s *State) {
			thin(s)
			s.TrainingDays = 1<<7 - 1
			s.Target = &plan.Target{Training: plan.DayTarget{Kcal: 2400, Protein: 100, Fat: 60}, Rest: plan.DayTarget{Kcal: 2000, Protein: 100, Fat: 60}}
		}, Change{TrainingDays: new(plan.Weekdays(0))}, nil, []string{"underweight_loss code"}},
		{"a lower target above a BMI of 18.5", func(s *State) { thin(s); s.TrendKg = 60 }, Change{Target: unsplit(1700, 100, 50)}, nil, nil},
		{"a first target", func(s *State) { thin(s); s.Target = nil }, Change{Target: unsplit(1800, 100, 50)}, nil, nil},
	})
}

func TestValidateRefusesAChangeOfTheWrongShape(t *testing.T) {
	t.Parallel()

	start := "2026-10-26"
	runValidate(t, []validateCase{
		{"kcal that are no number", nil, Change{Target: unsplit(math.NaN(), 170, 70)}, nil, []string{"invalid_change"}},
		{"a training day of the wrong shape", nil, Change{Target: &plan.Target{
			Training: plan.DayTarget{Kcal: 500, Protein: 170, Fat: 70}, Rest: plan.DayTarget{Kcal: 2400, Protein: 170, Fat: 70},
		}}, nil, []string{"invalid_change"}},
		{"a rest day of the wrong shape", nil, Change{Target: &plan.Target{
			Training: plan.DayTarget{Kcal: 2400, Protein: 170, Fat: 70}, Rest: plan.DayTarget{Kcal: 500, Protein: 170, Fat: 70},
		}}, nil, []string{"invalid_change"}},
		{"a rate that is no number", nil, Change{Phase: phase(t, plan.Cut, start, math.NaN())}, nil, []string{"invalid_change"}},
		{"a bulk that loses", nil, Change{Phase: phase(t, plan.Bulk, start, -0.9)}, nil, []string{"invalid_change"}},
		{"maintenance that loses", nil, Change{Phase: phase(t, plan.Maintain, start, -1.5)}, nil, []string{"invalid_change"}},
		{"no day of the week", nil, Change{TrainingDays: new(plan.Weekdays(1 << 7))}, nil, []string{"invalid_change"}},
		{
			"a trend weight that is no number", func(s *State) { s.TrendKg = math.NaN() },
			Change{Phase: phase(t, plan.Cut, start, -0.5)},
			nil,
			[]string{"invalid_change"},
		},
		{
			"a trend weight at the phase's start that is no number", func(s *State) { s.PhaseStartTrendKg = math.Inf(1) },
			Change{Phase: phase(t, plan.Cut, start, -0.5)},
			nil,
			[]string{"invalid_change"},
		},
		// A regression with nothing to regress gives a TDEE of 0/0.
		{
			"a trusted TDEE that is no number", func(s *State) { s.TDEE, s.TDEETrusted = math.NaN(), true },
			Change{Target: unsplit(1500, 170, 60)},
			nil,
			[]string{"invalid_change"},
		},
		{
			"a lean mass that is no number", func(s *State) { s.LeanMassKg = math.NaN() },
			Change{Target: unsplit(2500, 170, 75)},
			nil,
			[]string{"invalid_change"},
		},
		{
			"active energy that is no number", func(s *State) { s.ActiveKcal = math.NaN() },
			Change{Target: unsplit(2500, 170, 75)},
			nil,
			[]string{"invalid_change"},
		},
		{
			"a drift that is no number", func(s *State) { s.Autopilot.DriftKcal = math.NaN() },
			Change{Target: unsplit(2500, 170, 75)},
			nil,
			[]string{"invalid_change"},
		},
		{
			"a stored phase of the wrong shape", func(s *State) { s.Phase = phase(t, plan.Cut, "2026-09-07", math.NaN()) },
			Change{Phase: phase(t, plan.Cut, start, -0.5)},
			nil,
			[]string{"invalid_change"},
		},
		{
			"a stored target of the wrong shape", func(s *State) { s.Target = unsplit(math.NaN(), 170, 70) },
			Change{Target: unsplit(2400, 170, 70)},
			nil,
			[]string{"invalid_change"},
		},
		{"a stored training day of the wrong shape", func(s *State) {
			s.Target = &plan.Target{Training: plan.DayTarget{Kcal: math.NaN(), Protein: 170, Fat: 70}, Rest: plan.DayTarget{Kcal: 2400, Protein: 170, Fat: 70}}
		}, Change{Target: unsplit(2400, 170, 70)}, nil, []string{"invalid_change"}},
		{"a stored rest day of the wrong shape", func(s *State) {
			s.Target = &plan.Target{Training: plan.DayTarget{Kcal: 2400, Protein: 170, Fat: 70}, Rest: plan.DayTarget{Kcal: math.NaN(), Protein: 170, Fat: 70}}
		}, Change{Target: unsplit(2400, 170, 70)}, nil, []string{"invalid_change"}},
		{
			"a height that is no number", func(s *State) { s.Profile.HeightCm = math.NaN() },
			Change{Phase: phase(t, plan.Cut, start, -0.5)},
			nil,
			[]string{"invalid_change"},
		},
		{
			"a height in metres", func(s *State) { s.Profile.HeightCm = 1.8 },
			Change{Phase: phase(t, plan.Cut, start, -0.5)},
			nil,
			[]string{"invalid_change"},
		},
		{
			"a user's limit that is no number", nil,
			Change{Phase: phase(t, plan.Cut, start, -5)},
			func(lv *Levels) { lv.User.CutRate = math.NaN() },
			[]string{"invalid_change"},
		},
		{
			"an instance's limit that is no number", nil,
			Change{Phase: phase(t, plan.Cut, start, -5)},
			func(lv *Levels) { lv.Instance.LossKgWeek = math.NaN() },
			[]string{"invalid_change"},
		},
		{
			"a drift's base that is no number", func(s *State) { s.Autopilot.DriftBaseTDEE = math.NaN() },
			Change{Target: unsplit(2500, 170, 75)},
			nil,
			[]string{"invalid_change"},
		},
	})
}

// A new phase that loses more slowly than the current one starts no loss
// below a BMI of 18.5.
func TestValidateUnderweightNewPhase(t *testing.T) {
	t.Parallel()

	cutting := func(s *State) {
		s.TrendKg = 59
		s.Phase = phase(t, plan.Cut, "2026-09-07", -0.5)
		s.PhaseStartTrendKg = 62
	}
	runValidate(t, []validateCase{
		{"a slower recomp", cutting, Change{Phase: phase(t, plan.Recomp, "2026-10-26", -0.2)}, nil, nil},
		{"a slower cut", cutting, Change{Phase: phase(t, plan.Cut, "2026-10-26", -0.2)}, nil, nil},
		{"a faster cut", cutting, Change{Phase: phase(t, plan.Cut, "2026-10-26", -0.6)}, nil, []string{"underweight_loss code"}},
		{"maintenance after a bulk", func(s *State) {
			s.TrendKg = 59
			s.Phase = phase(t, plan.Bulk, "2026-09-07", 0.25)
		}, Change{Phase: phase(t, plan.Maintain, "2026-10-26", 0)}, nil, nil},
	})
}

func TestUnderweight(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		bmi  float64
		want bool
	}{{18.4, true}, {18.5, false}, {18.5 - 1e-12, false}, {25, false}} {
		if got := Underweight(tc.bmi); got != tc.want {
			t.Errorf("Underweight(%v) = %v, want %v", tc.bmi, got, tc.want)
		}
	}
}

func TestLossUnderWay(t *testing.T) {
	t.Parallel()

	cut := plan.Phase{Type: plan.Cut, Start: day.Date{Year: 2026, Month: time.March, Day: 1}, RatePctWeek: -0.5}
	bulk := plan.Phase{Type: plan.Bulk, Start: cut.Start, RatePctWeek: 0.25}
	under := plan.Target{
		EffectiveFrom: cut.Start,
		Training:      plan.DayTarget{Kcal: 2000, Protein: 150, Fat: 60}, Rest: plan.DayTarget{Kcal: 2000, Protein: 150, Fat: 60},
	}
	for _, tc := range []struct {
		name string
		s    State
		want bool
	}{
		{"a cut", State{Phase: &cut, TrendKg: 70}, true},
		{"a bulk", State{Phase: &bulk, TrendKg: 70}, false},
		{"a bulk whose target is under a trusted TDEE", State{Phase: &bulk, Target: &under, TDEE: 2600, TDEETrusted: true, TrendKg: 70}, true},
		{"the same under a TDEE not trusted", State{Phase: &bulk, Target: &under, TDEE: 2600, TrendKg: 70}, false},
		// A target at the TDEE, by float arithmetic, is no loss.
		{"a bulk whose target is the TDEE", State{Phase: &bulk, Target: &under, TDEE: 2000 + 1e-12, TDEETrusted: true, TrendKg: 70}, false},
		{"nothing", State{TrendKg: 70}, false},
	} {
		if got := LossUnderWay(tc.s); got != tc.want {
			t.Errorf("%s: LossUnderWay = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUnderweightLoss(t *testing.T) {
	t.Parallel()

	cut := plan.Phase{Type: plan.Cut, Start: day.Date{Year: 2026, Month: time.March, Day: 1}, RatePctWeek: -0.5}
	pr := profile.Profile{Sex: profile.Male, BirthDate: day.Date{Year: 1990, Month: time.May, Day: 1}, HeightCm: 180}
	for _, tc := range []struct {
		name string
		s    State
		want bool
	}{
		// 58 kg at 180 cm is a BMI of 17.9; 60 kg is 18.5.
		{"a cut at a BMI of 17.9", State{Profile: pr, Phase: &cut, TrendKg: 58}, true},
		{"a cut at 18.5", State{Profile: pr, Phase: &cut, TrendKg: 60}, false},
		{"no loss at 17.9", State{Profile: pr, TrendKg: 58}, false},
	} {
		if got := UnderweightLoss(tc.s); got != tc.want {
			t.Errorf("%s: UnderweightLoss = %v, want %v", tc.name, got, tc.want)
		}
	}
}
