package calibration

import (
	"math"
	"reflect"
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
			// Each phase changes the intake.
			days := map[day.Date]Day{}
			line(days, last, 90, 80, -0.07, 2000)
			for _, ph := range tc.phases {
				eatLessFrom(days, ph.Start)
			}
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
	eatLessFrom(days, from)
	// Fourteen days left out of 21 leave 7, under the 14 a window needs.
	e := Measure(days, []plan.Phase{{Type: plan.Cut, Start: from, RatePctWeek: -0.5}}, from, last, DefaultParams())
	if e.Measured || e.Days != 7 || !slices.Equal(e.Doubts, []string{DoubtFewDays}) {
		t.Errorf("Measure = %d days, measured %v, doubts %v; want 7, none, few_days", e.Days, e.Measured, e.Doubts)
	}
	// A span the water days take whole has no day.
	days = map[day.Date]Day{}
	line(days, last, 90, 80, -0.07, 2000)
	eatLessFrom(days, last.AddDays(-5))
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
			for _, ph := range tc.phases {
				eatLessFrom(days, ph.Start)
			}
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
	eatLessFrom(days, last.AddDays(-9))
	p := DefaultParams()
	e := Calibrate(days, []plan.Phase{{Type: plan.Cut, Start: last.AddDays(-9), RatePctWeek: -0.5}}, last, p)
	if !e.BeforePhase || e.Days != p.LongWindowDays || e.To != last.AddDays(-10) {
		t.Errorf("Calibrate = %d days to %s, before the phase %v; want %d to %s", e.Days, e.To, e.BeforePhase, p.LongWindowDays, last.AddDays(-10))
	}
}

// A phase's start leaves out its first days of water only when the
// intake changes with it: a step of a few percent moves no water, a cut
// of a fifth or a reverse diet does; with days too few after it to tell,
// it is a change.
func TestCalibrateLeavesOutWaterOnlyAfterAChangeOfIntake(t *testing.T) {
	t.Parallel()

	p := DefaultParams()
	for _, tc := range []struct {
		name    string
		ago     int
		share   float64
		spanned bool
	}{
		{"the same intake", 20, 1.025, true},
		{"a little less", 20, 0.92, true},
		{"a cut of a fifth", 20, 0.8, false},
		{"a reverse diet", 20, 1.2, false},
		{"days too few after it", 1, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			days := map[day.Date]Day{}
			line(days, last, 90, 145, -0.09, 2340)
			start := last.AddDays(-tc.ago)
			for d, v := range days {
				if !d.Before(start) {
					v.Kcal *= tc.share
					days[d] = v
				}
			}
			e := Calibrate(days, []plan.Phase{{Type: plan.Cut, Start: start, RatePctWeek: -0.4}}, last, p)
			if spanned := !e.BeforePhase && e.To == last && e.From.Before(start); spanned != tc.spanned {
				t.Errorf("Calibrate = %s to %s, before the phase %v; want the window across the start %v", e.From, e.To, e.BeforePhase, tc.spanned)
			}
		})
	}
}

// A planned end the intake goes on through is no change either.
func TestCalibrateTakesAnEndWithoutAChangeForNone(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last, 90, 145, -0.09, 2340)
	end := last.AddDays(-10)
	e := Calibrate(days, []plan.Phase{{Type: plan.Cut, Start: last.AddDays(-80), End: end, RatePctWeek: -0.4}}, last, DefaultParams())
	if e.BeforePhase || e.To != last || !e.From.Before(end) {
		t.Errorf("Calibrate = %s to %s, before the phase %v; want the window across the end", e.From, e.To, e.BeforePhase)
	}
}

// A short phase's intake is held to its own days, and the next phase's to
// theirs: a diet break of a week, 400 kcal over the cut around it, is two
// changes, not an average that hides both.
func TestIntakeChangesOfAShortPhase(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last, 90, 145, -0.09, 2000)
	brk, back := last.AddDays(-20), last.AddDays(-13)
	for d, v := range days {
		if !d.Before(brk) && d.Before(back) {
			v.Kcal = 2400
			days[d] = v
		}
	}
	phases := []plan.Phase{
		{Type: plan.Cut, Start: last.AddDays(-80), RatePctWeek: -0.5},
		{Type: plan.Maintain, Start: brk},
		{Type: plan.Cut, Start: back, RatePctWeek: -0.5},
	}
	if got := intakeChanges(days, phases, last, DefaultParams()); !slices.Contains(got, brk) || !slices.Contains(got, back) {
		t.Errorf("intakeChanges = %v, want the break's start %s and its end %s", got, brk, back)
	}
}

// Of the days before a span Measure reads the LeadDays: the intake
// around a boundary whose water days reach it. One 13 days before the
// span, of the same intake as the one day, the furthest of the 14 before
// it, tells, leaves the span whole.
func TestMeasureReadsTheLeadDays(t *testing.T) {
	t.Parallel()

	p := DefaultParams()
	from, to := last.AddDays(-20), last
	days := map[day.Date]Day{}
	line(days, last, 100, 145, -0.09, 2000)
	for d, v := range days {
		if ago := from.DaysSince(d); ago > 13 && ago < 27 {
			v.Meals = 0
			days[d] = v
		}
	}
	phases := []plan.Phase{{Type: plan.Maintain, Start: from.AddDays(-13)}}
	cut := map[day.Date]Day{}
	first := firstData(days)
	for d, v := range days {
		if from.DaysSince(d) <= LeadDays(p) || d == first {
			cut[d] = v
		} else if v.WeightKg > 0 {
			cut[d] = Day{Status: v.Status, WeightKg: v.WeightKg}
		}
	}
	want := Measure(days, phases, from, to, p)
	if want.From != from {
		t.Fatalf("Measure of the whole history from %s, want %s", want.From, from)
	}
	if got := Measure(cut, phases, from, to, p); !reflect.DeepEqual(got, want) {
		t.Errorf("Measure of the cut history = %+v, of the whole %+v", got, want)
	}
}
