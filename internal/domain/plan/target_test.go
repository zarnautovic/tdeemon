package plan

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

func TestDayTargetMacros(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name            string
		t               DayTarget
		carbs, fatShare float64
	}{
		// 2400 − 4 × 160 − 9 × 70 = 1130 kcal, 282.5 g of carbs.
		{"a day", DayTarget{2400, 160, 70}, 282.5, 630.0 / 2400},
		{"no carbs left", DayTarget{1000, 115, 60}, 0, 0.54},
		{"no fat", DayTarget{2000, 150, 0}, 350, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.t.Carbs(); !near(got, tc.carbs) {
				t.Errorf("Carbs() = %v, want %v", got, tc.carbs)
			}
			if got := tc.t.FatShare(); !near(got, tc.fatShare) {
				t.Errorf("FatShare() = %v, want %v", got, tc.fatShare)
			}
		})
	}
}

func TestDayTargetValidate(t *testing.T) {
	t.Parallel()

	for _, d := range []DayTarget{
		{2400, 160, 70},
		{1000, 115, 60}, // protein and fat give exactly the day's energy
		// The same with grams with decimals: 4 × 100 + 9 × 30.1 comes to a
		// little over 670.9 in float arithmetic.
		{670.9, 100, 30.1},
		{676.3, 100, 30.7},
		{4*176.66 + 9*61.3, 176.66, 61.3},
		{MaxKcal, 0, 0},
		{0.5, 0, 0}, // a floor is a limit, not a rule of the shape
	} {
		if err := d.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", d, err)
		}
	}
	for _, tc := range []struct {
		name string
		t    DayTarget
		want string
	}{
		{"no energy", DayTarget{0, 0, 0}, "kcal 0: want more than 0 and at most 15000"},
		{"negative energy", DayTarget{-100, 0, 0}, "kcal -100"},
		{"a typo of a zero", DayTarget{25000, 160, 70}, "kcal 25000"},
		{"energy that is no number", DayTarget{math.NaN(), 0, 0}, "kcal NaN"},
		{"negative protein", DayTarget{2000, -1, 60}, "protein -1 g"},
		{"protein that is no number", DayTarget{2000, math.Inf(1), 60}, "protein +Inf g"},
		{"negative fat", DayTarget{2000, 150, -5}, "fat -5 g"},
		{"more in protein and fat than the day", DayTarget{1000, 120, 60}, "protein and fat give 1020 kcal, more than the day's 1000"},
		{"a hundredth of a gram of fat too much", DayTarget{676.3, 100, 30.71}, "protein and fat give 676.39 kcal, more than the day's 676.3"},
		// The energy is shown as a person would write it, not as a float.
		{"a tenth of a kcal too little", DayTarget{670.8, 100, 30.1}, "protein and fat give 670.9 kcal, more than the day's 670.8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.t.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error with %q", err, tc.want)
			}
		})
	}
	// A value that breaks its rule is not also reported as too much
	// energy.
	for _, d := range []DayTarget{{1000, math.NaN(), 0}, {0, 100, 0}, {2000, 100, math.Inf(1)}} {
		if err := d.Validate(); err == nil || strings.Contains(err.Error(), "more than the day") {
			t.Errorf("Validate(%+v) = %v, want only the value that breaks its rule", d, err)
		}
	}
}

// A split target: 2600 kcal on training days, 2200 on rest days.
func split(tb testing.TB) Target {
	tb.Helper()
	return Target{
		EffectiveFrom: date(tb, "2026-11-02"),
		Training:      DayTarget{2600, 170, 75},
		Rest:          DayTarget{2200, 160, 70},
	}
}

func TestTargetFor(t *testing.T) {
	t.Parallel()

	tg := split(t)
	if got := tg.For(Training); got != tg.Training {
		t.Errorf("For(training) = %+v", got)
	}
	if got := tg.For(Rest); got != tg.Rest {
		t.Errorf("For(rest) = %+v", got)
	}
}

func TestWeeklyAverage(t *testing.T) {
	t.Parallel()

	tg := split(t)
	for _, tc := range []struct {
		name               string
		training           int
		kcal, protein, fat float64
	}{
		{"no training", 0, 2200, 160, 70},
		// (3 × 2600 + 4 × 2200) / 7
		{"three training days", 3, 16600.0 / 7, (3*170 + 4*160) / 7.0, (3*75 + 4*70) / 7.0},
		{"five training days", 5, (5*2600 + 2*2200) / 7.0, (5*170 + 2*160) / 7.0, (5*75 + 2*70) / 7.0},
		{"every day", 7, 2600, 170, 75},
		{"less than none is none", -1, 2200, 160, 70},
		{"more than a week is every day", 8, 2600, 170, 75},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tg.WeeklyAverage(tc.training)
			if !near(got.Kcal, tc.kcal) || !near(got.Protein, tc.protein) || !near(got.Fat, tc.fat) {
				t.Errorf("WeeklyAverage(%d) = %+v, want %v kcal, %v g protein, %v g fat", tc.training, got, tc.kcal, tc.protein, tc.fat)
			}
		})
	}
	// An unsplit target's average is the target itself.
	same := Target{Training: DayTarget{2300, 150, 70}, Rest: DayTarget{2300, 150, 70}}
	if got := same.WeeklyAverage(3); !near(got.Kcal, 2300) || !near(got.Carbs(), same.Rest.Carbs()) {
		t.Errorf("WeeklyAverage of an unsplit target = %+v", got)
	}
}

// A step moves the weekly average by the step, whatever the number of
// training days, and keeps the difference between the day types and the
// grams of protein and fat.
func TestShiftKcal(t *testing.T) {
	t.Parallel()

	tg := split(t)
	for _, delta := range []float64{-100, 50, 0} {
		t.Run(strconv.FormatFloat(delta, 'f', -1, 64), func(t *testing.T) {
			t.Parallel()
			got := tg.ShiftKcal(delta)
			for n := range 8 {
				if diff := got.WeeklyAverage(n).Kcal - tg.WeeklyAverage(n).Kcal; !near(diff, delta) {
					t.Errorf("with %d training days the average moved by %v", n, diff)
				}
			}
			if got.Training.Kcal-got.Rest.Kcal != 400 {
				t.Errorf("the split is %v, want 400", got.Training.Kcal-got.Rest.Kcal)
			}
			want := tg
			want.Training.Kcal, want.Rest.Kcal = got.Training.Kcal, got.Rest.Kcal
			if got != want {
				t.Errorf("ShiftKcal = %+v, changed more than the kcal of %+v", got, tg)
			}
			if !near(got.Rest.Carbs()-tg.Rest.Carbs(), delta/4) {
				t.Errorf("the carbs moved by %v, want %v", got.Rest.Carbs()-tg.Rest.Carbs(), delta/4)
			}
		})
	}
	if tg.Training.Kcal != 2600 {
		t.Error("ShiftKcal changed the target it was called on")
	}
}

func TestTargetValidate(t *testing.T) {
	t.Parallel()

	if err := split(t).Validate(); err != nil {
		t.Errorf("Validate = %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Target)
		want   string
	}{
		{"no day it takes effect", func(tg *Target) { tg.EffectiveFrom = day.Date{} }, "needs the day it takes effect"},
		{"a bad training day", func(tg *Target) { tg.Training.Kcal = 0 }, "training days: kcal 0"},
		{"a bad rest day", func(tg *Target) { tg.Rest.Fat = -1 }, "rest days: fat -1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tg := split(t)
			tc.change(&tg)
			if err := tg.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func history(tb testing.TB) []Target {
	tb.Helper()
	first := split(tb)
	second := first.ShiftKcal(-100)
	second.EffectiveFrom = date(tb, "2026-11-16")
	third := second
	third.EffectiveFrom = date(tb, "2026-12-07")
	third.Training = third.Rest // unsplit from here
	return []Target{first, second, third}
}

func TestValidateHistory(t *testing.T) {
	t.Parallel()

	if err := ValidateHistory(history(t)); err != nil {
		t.Errorf("ValidateHistory = %v", err)
	}
	if err := ValidateHistory(nil); err != nil {
		t.Errorf("ValidateHistory(nil) = %v", err)
	}
	hs := history(t)
	hs[1].Rest.Kcal = -1
	if err := ValidateHistory(hs); err == nil || !strings.Contains(err.Error(), "target 2: rest days: kcal -1") {
		t.Errorf("ValidateHistory with a bad target = %v", err)
	}
	hs = history(t)
	hs[2].EffectiveFrom = hs[1].EffectiveFrom
	if err := ValidateHistory(hs); err == nil || !strings.Contains(err.Error(), "target 3 takes effect on 2026-11-16, not after target 2's 2026-11-16") {
		t.Errorf("ValidateHistory with two on one day = %v", err)
	}
}

func TestTargetOn(t *testing.T) {
	t.Parallel()

	hs := history(t)
	for _, tc := range []struct {
		d    string
		want int // the target's index, or -1 for none
	}{
		{"2026-11-01", -1},
		{"2026-11-02", 0},
		{"2026-11-15", 0},
		{"2026-11-16", 1},
		{"2026-12-07", 2},
		{"2028-01-01", 2},
	} {
		t.Run(tc.d, func(t *testing.T) {
			t.Parallel()
			got, ok := TargetOn(hs, date(t, tc.d))
			switch {
			case tc.want < 0 && ok:
				t.Errorf("TargetOn(%s) = %+v, want none", tc.d, got)
			case tc.want >= 0 && (!ok || got != hs[tc.want]):
				t.Errorf("TargetOn(%s) = %+v, %v; want target %d", tc.d, got, ok, tc.want+1)
			}
		})
	}
}

// A day's target follows from the plan alone: the target in effect, for
// the day's type.
func TestDayTargetOn(t *testing.T) {
	t.Parallel()

	hs := history(t)
	monday, tuesday := date(t, "2026-11-16"), date(t, "2026-11-17")
	plan := weekdays(t, monday.Weekday())
	if got, ok := DayTargetOn(hs, monday, TypeOf(monday, plan, false, nil)); !ok || got.Kcal != 2500 {
		t.Errorf("Monday's target = %+v, %v; want 2500 kcal", got, ok)
	}
	if got, ok := DayTargetOn(hs, tuesday, TypeOf(tuesday, plan, false, nil)); !ok || got.Kcal != 2100 {
		t.Errorf("Tuesday's target = %+v, %v; want 2100 kcal", got, ok)
	}
	if got, ok := DayTargetOn(hs, date(t, "2026-11-01"), Rest); ok || got != (DayTarget{}) {
		t.Errorf("a target before the first = %+v, %v", got, ok)
	}
}

func TestProtein(t *testing.T) {
	t.Parallel()

	if got := ProteinGrams(2.2, 80); !near(got, 176) {
		t.Errorf("ProteinGrams(2.2, 80) = %v, want 176", got)
	}
	if got := ProteinPerKg(176, 80); !near(got, 2.2) {
		t.Errorf("ProteinPerKg(176, 80) = %v, want 2.2", got)
	}
}
