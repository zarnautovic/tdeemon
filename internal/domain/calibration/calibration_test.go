package calibration

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// last is the window's last day in the tests, a Sunday.
var last = day.Date{Year: 2026, Month: time.March, Day: 29}

func near(a, b, tolerance float64) bool { return math.Abs(a-b) <= tolerance }

// line fills days with n days up to to: each complete, with a meal of
// kcal and a weigh-in on a straight line that ends at kgEnd and moves
// kgPerDay a day, so the TDEE is exactly kcal − kgPerDay × 7700.
func line(days map[day.Date]Day, to day.Date, n int, kgEnd, kgPerDay, kcal float64) {
	for i := range n {
		days[to.AddDays(-i)] = Day{Status: day.Complete, Meals: 1, Kcal: kcal, WeightKg: kgEnd - float64(i)*kgPerDay}
	}
}

func TestCalibrateExactLine(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	// 0.07 kg a day: 0.49 kg a week, 539 kcal a day under the TDEE.
	line(days, last, 60, 80, -0.07, 2000)
	e := Calibrate(days, nil, last, DefaultParams())

	if e.From != last.AddDays(-41) || e.To != last || e.Days != 42 || e.BeforePhase {
		t.Errorf("window %s to %s, %d days, before the phase %v; want the 42 days to %s", e.From, e.To, e.Days, e.BeforePhase, last)
	}
	if !e.Measured || !e.Trusted || len(e.Doubts) != 0 {
		t.Fatalf("measured %v, trusted %v, doubts %v; want a trusted estimate", e.Measured, e.Trusted, e.Doubts)
	}
	if e.LoggedDays != 42 || e.WeighIns != 42 {
		t.Errorf("%d logged days and %d weigh-ins, want 42 of each", e.LoggedDays, e.WeighIns)
	}
	if !near(e.TDEE, 2539, 1e-6) || !near(e.IntakeKcal, 2000, 1e-9) || !near(e.KgPerWeek, -0.49, 1e-9) {
		t.Errorf("TDEE %v, intake %v, %v kg a week; want 2539, 2000 and -0.49", e.TDEE, e.IntakeKcal, e.KgPerWeek)
	}
	// A line without noise, every day logged: nothing is uncertain.
	if !near(e.HalfWidth, 0, 1e-6) || !near(e.KgPerWeekSE, 0, 1e-9) {
		t.Errorf("half the interval %v, the slope's error %v; want 0", e.HalfWidth, e.KgPerWeekSE)
	}
}

func TestCalibrateTrendKg(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last, 40, 80, -0.07, 2000)
	// The trend lags behind a falling weight, but stays near it: taking a
	// tenth of each weigh-in, it comes to lag 9 days, 0.63 kg, closing in
	// by a tenth of what is left with each of the 39 weigh-ins after the
	// first.
	want := 80 + 0.63*(1-math.Pow(0.9, 39))
	if e := Calibrate(days, nil, last, DefaultParams()); !near(e.TrendKg, want, 1e-9) {
		t.Errorf("trend weight %v, want %v", e.TrendKg, want)
	}
	// The trend on the window's last day, not on the last weigh-in given.
	line(days, last.AddDays(10), 10, 70, -1, 2000)
	if e := Calibrate(days, nil, last, DefaultParams()); !near(e.TrendKg, want, 1e-9) {
		t.Errorf("trend weight %v with later weigh-ins, want %v", e.TrendKg, want)
	}
}

func TestCalibrateCountsTheIntakeOfCompleteLoggedDays(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// day is what the window's fifth day becomes.
		day Day
		// wantLogged and wantWeighIns count the window's 42 days.
		wantLogged, wantWeighIns int
	}{
		// A day without a meal was not logged: its 0 kcal would lower the
		// intake, and the TDEE with it.
		{"a day without a meal is not 0 kcal", Day{Status: day.Complete, WeightKg: 79}, 41, 42},
		// The user excluded the day, a restaurant: its intake is not to
		// be trusted, but the weight is still the body's.
		{"an excluded day's intake", Day{Status: day.Excluded, Meals: 2, Kcal: 6000, WeightKg: 79}, 41, 42},
		{"an open day's intake", Day{Status: day.Open, Meals: 1, Kcal: 500, WeightKg: 79}, 41, 42},
		{"a day without a weigh-in", Day{Status: day.Complete, Meals: 1, Kcal: 2000}, 42, 41},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			days := map[day.Date]Day{}
			line(days, last, 42, 80, -1.0/28, 2000)
			// The weigh-in on the line, where the case keeps one.
			if tc.day.WeightKg > 0 {
				tc.day.WeightKg = days[last.AddDays(-23)].WeightKg
			}
			days[last.AddDays(-23)] = tc.day
			e := Calibrate(days, nil, last, DefaultParams())
			if e.LoggedDays != tc.wantLogged || e.WeighIns != tc.wantWeighIns {
				t.Errorf("%d logged days and %d weigh-ins, want %d and %d", e.LoggedDays, e.WeighIns, tc.wantLogged, tc.wantWeighIns)
			}
			if !near(e.IntakeKcal, 2000, 1e-9) || !near(e.TDEE, 2275, 1e-6) {
				t.Errorf("intake %v, TDEE %v; want 2000 and 2275", e.IntakeKcal, e.TDEE)
			}
		})
	}
}

func TestCalibrateLeavesOutAPhasesFirstDays(t *testing.T) {
	t.Parallel()

	// history is a phase of maintenance, then one that starts on start:
	// before it, 2500 kcal and a steady weight; in it, 2000 kcal and a
	// loss of 0.05 kg a day after 1.5 kg of water and glycogen in its
	// first days, which the estimate must not count as fat.
	history := func(start day.Date) map[day.Date]Day {
		days := map[day.Date]Day{}
		line(days, start.AddDays(-1), 60, 82, 0, 2500)
		for d := start; !last.Before(d); d = d.AddDays(1) {
			n := float64(d.DaysSince(start))
			days[d] = Day{Status: day.Complete, Meals: 3, Kcal: 2000, WeightKg: 82 - 0.05*n - 1.5*min(n/5, 1)}
		}
		return days
	}
	const before, during = 2500, 2000 + 0.05*plan.KcalPerKg
	for _, tc := range []struct {
		name string
		// ago is how many days before last the phase started.
		ago int
		// wantFrom and wantTo are the window's, in days before last.
		wantFrom, wantTo int
		wantBefore       bool
		wantTDEE         float64
	}{
		{"long under way", 60, 41, 0, false, during},
		// The phase's 14th day to last: 17 days, enough.
		{"its days after the first 14", 30, 16, 0, false, during},
		{"14 days after the first 14", 27, 13, 0, false, during},
		// 13 days after its first 14 are too few: the 42 days before it.
		{"13 days after the first 14", 26, 68, 27, true, before},
		{"in its first days", 5, 47, 6, true, before},
		{"starting on last", 0, 42, 1, true, before},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := last.AddDays(-tc.ago)
			phases := []plan.Phase{
				{Type: plan.Maintain, Start: start.AddDays(-200)},
				{Type: plan.Cut, Start: start, RatePctWeek: -0.5},
			}
			e := Calibrate(history(start), phases, last, DefaultParams())
			if e.From != last.AddDays(-tc.wantFrom) || e.To != last.AddDays(-tc.wantTo) || e.BeforePhase != tc.wantBefore {
				t.Errorf("window %s to %s, before the phase %v; want %s to %s, %v",
					e.From, e.To, e.BeforePhase, last.AddDays(-tc.wantFrom), last.AddDays(-tc.wantTo), tc.wantBefore)
			}
			if !e.Measured || !near(e.TDEE, tc.wantTDEE, 1e-6) {
				t.Errorf("measured %v, TDEE %v; want %v", e.Measured, e.TDEE, tc.wantTDEE)
			}
		})
	}
}

func TestCalibrateWithoutEnoughDaysOfAPhase(t *testing.T) {
	t.Parallel()

	start := last.AddDays(-5)
	for _, tc := range []struct {
		name   string
		phases []plan.Phase
		days   map[day.Date]Day
	}{
		// Nothing logged before the phase: no estimate from before it.
		{"nothing before it", []plan.Phase{{Type: plan.Cut, Start: start, RatePctWeek: -0.5}}, map[day.Date]Day{}},
		// A diet break of 10 days before it, and nothing logged before the
		// break: neither the break nor the time before it gives one.
		{"a short phase before it", []plan.Phase{
			{Type: plan.Maintain, Start: start.AddDays(-10)},
			{Type: plan.Cut, Start: start, RatePctWeek: -0.5},
		}, func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 15, 80, 0, 2400)
			return days
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := Calibrate(tc.days, tc.phases, last, DefaultParams())
			// Only the doubt the days to come settle, not more weigh-ins.
			if e.Measured || e.Trusted || e.BeforePhase || !slices.Equal(e.Doubts, []string{DoubtFewDays}) {
				t.Errorf("measured %v, trusted %v, before the phase %v, doubts %v; want no estimate for few days",
					e.Measured, e.Trusted, e.BeforePhase, e.Doubts)
			}
			// The phase's first days reach past last: the window is empty,
			// from the day after it.
			if e.Days != 0 || e.From != last.AddDays(1) || e.To != last {
				t.Errorf("window %s to %s, %d days; want none, after %s", e.From, e.To, e.Days, last)
			}
		})
	}
}

func TestCalibrateDoubts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// keep reports whether the window's day i (0 is last) keeps its
		// weigh-in and its meal.
		keep         func(i int) (weighIn, meal bool)
		wantMeasured bool
		wantDoubts   []string
	}{
		{"every day", func(int) (bool, bool) { return true, true }, true, nil},
		// 3 weigh-ins and 4 logged days a week, on average.
		{"just enough", func(i int) (bool, bool) { return i%7 < 3, i%7 < 4 }, true, nil},
		{"2 weigh-ins a week", func(i int) (bool, bool) { return i%7 < 2, true }, true, []string{DoubtFewWeighIns}},
		{"3 logged days a week", func(i int) (bool, bool) { return true, i%7 < 3 }, true, []string{DoubtFewLoggedDays}},
		{"2 weigh-ins", func(i int) (bool, bool) { return i < 2, true }, false, []string{DoubtFewWeighIns}},
		{"1 logged day", func(i int) (bool, bool) { return true, i == 0 }, false, []string{DoubtFewLoggedDays}},
		{"nothing", func(int) (bool, bool) { return false, false }, false, []string{DoubtFewWeighIns, DoubtFewLoggedDays}},
		// 3.5 weigh-ins a week on average, but none in the last 3 weeks:
		// the slope says little of the weight now.
		{"no weigh-in in the last 3 weeks", func(i int) (bool, bool) { return i >= 21, true }, true, []string{DoubtFewWeighIns}},
		{"nothing logged in the last week", func(i int) (bool, bool) { return true, i >= 7 }, true, []string{DoubtFewLoggedDays}},
		{"2 weigh-ins in the last week", func(i int) (bool, bool) { return i >= 7 || i < 2, true }, true, []string{DoubtFewWeighIns}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			days := map[day.Date]Day{}
			line(days, last, 42, 80, -0.05, 2000)
			for i := range 42 {
				d := days[last.AddDays(-i)]
				weighIn, meal := tc.keep(i)
				if !weighIn {
					d.WeightKg = 0
				}
				if !meal {
					d.Meals, d.Kcal = 0, 0
				}
				days[last.AddDays(-i)] = d
			}
			e := Calibrate(days, nil, last, DefaultParams())
			if e.Measured != tc.wantMeasured || !slices.Equal(e.Doubts, tc.wantDoubts) || e.Trusted != (tc.wantMeasured && tc.wantDoubts == nil) {
				t.Errorf("measured %v, trusted %v, doubts %v; want measured %v, doubts %v",
					e.Measured, e.Trusted, e.Doubts, tc.wantMeasured, tc.wantDoubts)
			}
			if !e.Measured && (e.TDEE != 0 || e.HalfWidth != 0) {
				t.Errorf("no estimate, but TDEE %v ± %v", e.TDEE, e.HalfWidth)
			}
		})
	}
}

func TestCalibrateDoubtsAWideInterval(t *testing.T) {
	t.Parallel()

	// Weigh-ins that swing by 4 kg from day to day, about a level weight.
	days := map[day.Date]Day{}
	for i := range 42 {
		days[last.AddDays(-i)] = Day{Status: day.Complete, Meals: 2, Kcal: 2400, WeightKg: 80 + 4*float64(i%2)}
	}
	e := Calibrate(days, nil, last, DefaultParams())
	if !e.Measured || e.Trusted || !slices.Equal(e.Doubts, []string{DoubtWideInterval}) || e.HalfWidth <= 200 {
		t.Errorf("measured %v, trusted %v, doubts %v, ± %v; want a measured estimate wider than ± 200",
			e.Measured, e.Trusted, e.Doubts, e.HalfWidth)
	}
	// The same, trusted at a wider bound.
	p := DefaultParams()
	p.TrustedKcal = e.HalfWidth
	if e := Calibrate(days, nil, last, p); !e.Trusted {
		t.Errorf("not trusted at ± %v, doubts %v", p.TrustedKcal, e.Doubts)
	}
}

// simulate returns n days up to last of someone whose TDEE is tdee: an
// intake about 2100 kcal, 400 more at the weekend, the body's mass moving
// by the energy balance, and weigh-ins that swing about it by water, 0.4
// kg on average, carry of which carries over to the next day. Each day is
// logged with the chance logged, a weekend never when weekdays, and
// weighed with the chance weighed.
func simulate(r *rand.Rand, n int, tdee, logged float64, weekdays bool, weighed, carry float64) map[day.Date]Day {
	days := map[day.Date]Day{}
	mass, water := 85.0, 0.0
	for i := range n {
		d := last.AddDays(i - n + 1)
		intake := 2100 + 250*r.NormFloat64()
		weekend := d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
		if weekend {
			intake += 400
		}
		mass += (intake - tdee) / plan.KcalPerKg
		water = carry*water + 0.4*math.Sqrt(1-carry*carry)*r.NormFloat64()
		v := Day{Status: day.Complete}
		if r.Float64() < logged && (!weekdays || !weekend) {
			v.Meals, v.Kcal = 3, intake
		}
		if r.Float64() < weighed {
			v.WeightKg = mass + water
		}
		days[d] = v
	}
	return days
}

func TestCalibrateIntervalCoversTheTDEE(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name            string
		logged          float64
		weekdays        bool
		weighed, carry  float64
		least, most     float64
		biasOf, biasPad float64
	}{
		{"every day", 1, false, 1, 0.5, 0.75, 0.88, 0, 25},
		{"most water carried over", 1, false, 1, 0.85, 0.75, 0.88, 0, 25},
		{"no water carried over", 1, false, 1, 0, 0.75, 0.88, 0, 25},
		// Days missing at random: the intake's error takes them to differ
		// together, so the interval covers more than 80 %.
		{"most days", 0.85, false, 0.8, 0.5, 0.75, 0.95, 0, 25},
		{"some days", 0.6, false, 0.5, 0.5, 0.75, 1, 0, 25},
		// Weekends never logged, 400 kcal over the weekdays: the TDEE is
		// 2 × 400 / 7 = 114 kcal low. No interval can know that; this one
		// still covers the TDEE in about 61 % of the windows, where one
		// that took the days not logged for a random sample covered 41 %.
		{"weekdays only", 1, true, 1, 0.5, 0.55, 0.7, -114, 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const runs, tdee = 1000, 2600
			r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: simulated data, no secret
			var covered, measured int
			var errSum float64
			for range runs {
				e := Calibrate(simulate(r, 42, tdee, tc.logged, tc.weekdays, tc.weighed, tc.carry), nil, last, DefaultParams())
				if !e.Measured {
					continue
				}
				measured++
				errSum += e.TDEE - tdee
				if math.Abs(e.TDEE-tdee) <= e.HalfWidth {
					covered++
				}
			}
			if measured < runs*9/10 {
				t.Fatalf("%d of %d runs measured", measured, runs)
			}
			// An 80 % interval. With every day logged, one that took
			// weigh-ins on following days for independent would cover 39
			// to 77 %, and one that did not raise the autocorrelation of a
			// short series 69 to 79 %.
			if share := float64(covered) / float64(measured); share < tc.least || share > tc.most {
				t.Errorf("the interval covers the TDEE in %.1f %% of the runs, want %.0f to %.0f %%", 100*share, 100*tc.least, 100*tc.most)
			}
			if bias := errSum / float64(measured); math.Abs(bias-tc.biasOf) > tc.biasPad {
				t.Errorf("the estimate is %+.0f kcal off on average, want %+.0f", bias, tc.biasOf)
			}
		})
	}
}

func TestCalibrateLooksPastAShortPhase(t *testing.T) {
	t.Parallel()

	// A long cut, a diet break of 7 days that ended 10 days ago, and the
	// cut again: neither the break nor the cut since has days enough, so
	// the estimate is the cut's before the break.
	breakStart := last.AddDays(-16)
	phases := []plan.Phase{
		{Type: plan.Cut, Start: last.AddDays(-200), RatePctWeek: -0.5},
		{Type: plan.Maintain, Start: breakStart, End: breakStart.AddDays(6)},
		{Type: plan.Cut, Start: breakStart.AddDays(7), RatePctWeek: -0.5},
	}
	days := map[day.Date]Day{}
	line(days, breakStart.AddDays(-1), 80, 90, -0.05, 2000)
	line(days, last, 17, 88, 0, 2700)
	e := Calibrate(days, phases, last, DefaultParams())
	if !e.BeforePhase || e.To != breakStart.AddDays(-1) || e.From != breakStart.AddDays(-42) {
		t.Errorf("window %s to %s, before the phase %v; want the 42 days before %s", e.From, e.To, e.BeforePhase, breakStart)
	}
	if !e.Measured || !near(e.TDEE, 2000+0.05*plan.KcalPerKg, 1e-6) {
		t.Errorf("measured %v, TDEE %v; want the cut's, %v", e.Measured, e.TDEE, 2000+0.05*plan.KcalPerKg)
	}
	// The trend weight is the one now, not the window's.
	trend := Trend(days, DefaultParams().TrendAlpha)
	if now := trend[len(trend)-1]; now.Date != last || e.TrendKg != now.Kg {
		t.Errorf("trend weight %v, want the one on %s, %v", e.TrendKg, now.Date, now.Kg)
	}
}

func TestCalibrateTakesAPhasesEndForAChange(t *testing.T) {
	t.Parallel()

	// A cut that ended 6 days ago, with no phase after it: eating at
	// maintenance since refilled 1 kg of glycogen and water, which the
	// estimate must not count as fat.
	end := last.AddDays(-6)
	phases := []plan.Phase{{Type: plan.Cut, Start: last.AddDays(-200), End: end, RatePctWeek: -0.5}}
	days := map[day.Date]Day{}
	line(days, end, 80, 90, -0.05, 2000)
	line(days, last, 6, 88.75, 0, 2800)
	e := Calibrate(days, phases, last, DefaultParams())
	if !e.BeforePhase || e.To != end || !near(e.TDEE, 2000+0.05*plan.KcalPerKg, 1e-6) {
		t.Errorf("window %s to %s, before the phase %v, TDEE %v; want the cut's to its end, %v",
			e.From, e.To, e.BeforePhase, e.TDEE, 2000+0.05*plan.KcalPerKg)
	}
}

func TestCalibrateLeavesOutValuesThatAreNoNumber(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last, 42, 80, -0.05, 2000)
	for i, d := range map[int]Day{
		3: {Status: day.Complete, Meals: 1, Kcal: math.NaN(), WeightKg: days[last.AddDays(-3)].WeightKg},
		4: {Status: day.Complete, Meals: 1, Kcal: math.Inf(1), WeightKg: days[last.AddDays(-4)].WeightKg},
		5: {Status: day.Complete, Meals: 1, Kcal: 2000, WeightKg: math.Inf(1)},
		6: {Status: day.Complete, Meals: 1, Kcal: 2000, WeightKg: math.NaN()},
	} {
		days[last.AddDays(-i)] = d
	}
	e := Calibrate(days, nil, last, DefaultParams())
	if e.LoggedDays != 40 || e.WeighIns != 40 {
		t.Errorf("%d logged days and %d weigh-ins, want 40 of each", e.LoggedDays, e.WeighIns)
	}
	if !e.Trusted || !near(e.TDEE, 2385, 1e-6) || !finite(e.TrendKg) {
		t.Errorf("trusted %v, TDEE %v, trend %v; want a trusted 2385 and a trend that is a number", e.Trusted, e.TDEE, e.TrendKg)
	}
}

func TestCalibrateWidensWithTheDaysNotLogged(t *testing.T) {
	t.Parallel()

	// A level weight and 1800 or 2200 kcal on the days logged: the
	// interval is the intake's alone, the spread of a day times the share
	// of the days not logged.
	var widths []float64
	for _, logged := range []int{7, 6, 5, 4} {
		days := map[day.Date]Day{}
		for i := range 42 {
			d := Day{Status: day.Complete, WeightKg: 80}
			if i%7 < logged {
				d.Meals, d.Kcal = 1, 1800+400*float64(i%2)
			}
			days[last.AddDays(-i)] = d
		}
		e := Calibrate(days, nil, last, DefaultParams())
		widths = append(widths, e.HalfWidth)
	}
	if widths[0] != 0 || !slices.IsSorted(widths) || widths[1] == widths[2] {
		t.Errorf("half the interval with 7 to 4 days a week logged: %v; want 0, then wider", widths)
	}
	// With 4 of 7 logged, 3/7 of a day's spread of about 200 kcal, times
	// the bound: about 110.
	if !near(widths[3], 110, 10) {
		t.Errorf("half the interval with 4 days a week logged: %v, want about 110", widths[3])
	}
}

func TestTrend(t *testing.T) {
	t.Parallel()

	d := last.AddDays(-10)
	days := map[day.Date]Day{
		d.AddDays(3): {WeightKg: 78},
		d:            {WeightKg: 80},
		d.AddDays(1): {WeightKg: 82},
		d.AddDays(2): {Status: day.Complete, Meals: 1, Kcal: 2000},
	}
	// The gap of 2 days before the last weigh-in takes 1 − 0.5² of it.
	want := []Point{{d, 80}, {d.AddDays(1), 81}, {d.AddDays(3), 78.75}}
	if got := Trend(days, 0.5); !slices.Equal(got, want) {
		t.Errorf("Trend = %v, want %v", got, want)
	}
	for _, tc := range []struct {
		on   day.Date
		want float64
	}{{d.AddDays(-1), 0}, {d, 80}, {d.AddDays(2), 81}, {last, 78.75}} {
		if got := trendOn(days, tc.on, 0.5); got != tc.want {
			t.Errorf("trendOn(%s) = %v, want %v", tc.on, got, tc.want)
		}
	}
	if got := Trend(map[day.Date]Day{}, 0.5); len(got) != 0 {
		t.Errorf("Trend of no days = %v, want none", got)
	}
}

func TestTQuantile(t *testing.T) {
	t.Parallel()

	// Student's t at 0.9, from its tables.
	for _, tc := range []struct {
		df   int
		want float64
	}{{1, 3.078}, {10, 1.372}, {30, 1.310}, {31, 1.309}, {40, 1.303}, {60, 1.296}, {120, 1.289}, {100000, tNormal}} {
		if got := tQuantile(tc.df); !near(got, tc.want, 0.001) {
			t.Errorf("tQuantile(%d) = %v, want %v", tc.df, got, tc.want)
		}
	}
}

func TestDefaultParamsTakeTheAutopilotsData(t *testing.T) {
	t.Parallel()

	p, l := DefaultParams(), limits.UserDefaults()
	if p.LoggedDays != l.LoggedDays || p.WeighIns != l.WeighIns {
		t.Errorf("%v logged days and %v weigh-ins a week, want the autopilot's %v and %v", p.LoggedDays, p.WeighIns, l.LoggedDays, l.WeighIns)
	}
}
