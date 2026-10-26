package calibration

import (
	"math"
	"slices"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

func TestMeasureASpan(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	// 0.07 kg a day: 539 kcal a day under the TDEE, as in the window of
	// Calibrate; the span is 28 days before last.
	line(days, last, 90, 80, -0.07, 2000)
	from, to := last.AddDays(-55), last.AddDays(-28)
	e := Measure(days, nil, from, to, DefaultParams())
	if e.From != from || e.To != to || e.Days != 28 || !e.Measured || !e.Trusted {
		t.Fatalf("Measure = %s to %s, %d days, measured %v, trusted %v; want the 28 days, trusted", e.From, e.To, e.Days, e.Measured, e.Trusted)
	}
	if !near(e.TDEE, 2539, 1e-6) || !near(e.KgPerWeek, -0.49, 1e-9) {
		t.Errorf("TDEE %v, %v kg a week; want 2539 and -0.49", e.TDEE, e.KgPerWeek)
	}
}

func TestMeasureLeavesOutAPhasesFirstDays(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last, 90, 80, -0.07, 2000)
	from, to := last.AddDays(-55), last.AddDays(-14)
	p := DefaultParams()
	for _, tc := range []struct {
		name   string
		phases []plan.Phase
		want   day.Date
	}{
		{"a phase from the span's first day", []plan.Phase{{Type: plan.Cut, Start: from, RatePctWeek: -0.5}}, from.AddDays(p.WaterDays)},
		{"a phase from a week before", []plan.Phase{{Type: plan.Cut, Start: from.AddDays(-7), RatePctWeek: -0.5}}, from.AddDays(7)},
		{"a phase long before", []plan.Phase{{Type: plan.Cut, Start: from.AddDays(-60), RatePctWeek: -0.5}}, from},
		{"a phase after the span", []plan.Phase{{Type: plan.Bulk, Start: to.AddDays(1), RatePctWeek: 0.2}}, from},
		{"one phase within the span, after another", []plan.Phase{
			{Type: plan.Cut, Start: from, End: from.AddDays(9), RatePctWeek: -0.5},
			{Type: plan.Maintain, Start: from.AddDays(10)},
		}, from.AddDays(10 + p.WaterDays)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if e := Measure(days, tc.phases, from, to, p); e.From != tc.want || e.To != to || !e.Measured {
				t.Errorf("Measure = %s to %s, measured %v; want from %s", e.From, e.To, e.Measured, tc.want)
			}
		})
	}
}

func TestMeasureAShortSpan(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last, 90, 80, -0.07, 2000)
	from := last.AddDays(-20)
	// Fourteen days left out of 21 leave 7, under the 14 a window needs.
	e := Measure(days, []plan.Phase{{Type: plan.Cut, Start: from, RatePctWeek: -0.5}}, from, last, DefaultParams())
	if e.Measured || e.Days != 7 || !slices.Equal(e.Doubts, []string{DoubtFewDays}) {
		t.Errorf("Measure = %d days, measured %v, doubts %v; want 7, none, few_days", e.Days, e.Measured, e.Doubts)
	}
	// A span the water days take whole has no day.
	e = Measure(days, []plan.Phase{{Type: plan.Cut, Start: last.AddDays(-5), RatePctWeek: -0.5}}, last.AddDays(-5), last, DefaultParams())
	if e.Measured || e.Days != 0 || e.From != last.AddDays(1) {
		t.Errorf("Measure of water days alone = %s, %d days, measured %v; want none", e.From, e.Days, e.Measured)
	}
}

func TestMeasureStartsAtTheFirstData(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last, 30, 80, -0.07, 2000)
	// Weigh-ins from before the first meal are no data of the intake.
	for i := 30; i < 60; i++ {
		days[last.AddDays(-i)] = Day{Status: day.Complete, WeightKg: 82}
	}
	if e := Measure(days, nil, last.AddDays(-59), last, DefaultParams()); e.From != last.AddDays(-29) || !e.Measured {
		t.Errorf("Measure = from %s, measured %v; want from the first meal, %s", e.From, e.Measured, last.AddDays(-29))
	}
}

// waves fills days with n days up to to: a line that loses 0.07 kg a day,
// a meal of kcal each, and water in waves of amp kg that last weeks, so
// the residuals carry over from day to day.
func waves(days map[day.Date]Day, to day.Date, n int, amp, kcal float64) {
	for i := range n {
		d := float64(i)
		days[to.AddDays(-i)] = Day{Status: day.Complete, Meals: 1, Kcal: kcal, WeightKg: 145 + 0.07*d + amp*math.Sin(2*math.Pi*d/23)}
	}
}

func TestCalibrateLengthensAWindowOfWater(t *testing.T) {
	t.Parallel()

	p := DefaultParams()
	for _, tc := range []struct {
		name   string
		amp    float64
		n      int
		phases []plan.Phase
		want   int
	}{
		{"water that carries over", 0.8, 90, nil, p.LongWindowDays},
		{"no water", 0, 90, nil, p.WindowDays},
		{"water, as far as the first data", 0.8, 55, nil, 55},
		{"water, as far as the phase's first days", 0.8, 90, []plan.Phase{{Type: plan.Cut, Start: last.AddDays(-49), RatePctWeek: -0.5}}, 50 - p.WaterDays},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			days := map[day.Date]Day{}
			waves(days, last, tc.n, tc.amp, 2000)
			e := Calibrate(days, tc.phases, last, p)
			if !e.Measured || e.Days != tc.want || e.To != last {
				t.Errorf("Calibrate = %d days to %s, measured %v, autocorrelation %.2f; want %d", e.Days, e.To, e.Measured, e.Autocorrelation, tc.want)
			}
		})
	}
}

// The estimate from before a phase too short to give one reaches back as
// far as the water asks.
func TestCalibrateLengthensTheWindowBeforeAPhase(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	waves(days, last, 120, 0.8, 2000)
	p := DefaultParams()
	e := Calibrate(days, []plan.Phase{{Type: plan.Cut, Start: last.AddDays(-9), RatePctWeek: -0.5}}, last, p)
	if !e.BeforePhase || e.Days != p.LongWindowDays || e.To != last.AddDays(-10) {
		t.Errorf("Calibrate = %d days to %s, before the phase %v; want %d to %s", e.Days, e.To, e.BeforePhase, p.LongWindowDays, last.AddDays(-10))
	}
}
