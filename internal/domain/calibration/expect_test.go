package calibration

import (
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// weighEvery leaves a weigh-in on every n-th day of days, counted back
// from last.
func weighEvery(days map[day.Date]Day, n int) {
	for d, v := range days {
		if last.DaysSince(d)%n != 0 {
			v.WeightKg = 0
			days[d] = v
		}
	}
}

// weighOnly leaves a weigh-in on the days of days keep says, i days
// before last.
func weighOnly(days map[day.Date]Day, keep func(i int) bool) {
	for d, v := range days {
		if !keep(last.DaysSince(d)) {
			v.WeightKg = 0
			days[d] = v
		}
	}
}

func TestCalibrateCountsTowardAnEstimate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// days is how many days up to last are logged, and weighed in
		// where weighed says (i days before last).
		days    int
		weighed func(i int) bool
		// want are the window's days, its logged days between the first
		// weigh-in and the last, its weigh-ins and the days between them.
		want         [4]int
		wantMeasured bool
	}{
		// A new user's 10 days, each weighed and logged: what an estimate
		// needs but the days.
		{"10 days", 10, func(int) bool { return true }, [4]int{10, 10, 10, 9}, false},
		// Weigh-ins only on the last 3 days: too few for a slope, so the
		// logged days are the window's.
		{"3 weigh-ins", 10, func(i int) bool { return i < 3 }, [4]int{10, 10, 3, 2}, false},
		// Enough for a slope: the logged days between the first weigh-in
		// and the last, not the window's 30.
		{"weigh-ins of 16 days", 30, func(i int) bool { return i >= 5 && i <= 20 }, [4]int{30, 16, 16, 15}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			days := map[day.Date]Day{}
			line(days, last, tc.days, 80, -0.05, 2000)
			weighOnly(days, tc.weighed)
			e := Calibrate(days, nil, last, DefaultParams())
			got := [4]int{e.Days, e.SpanLoggedDays, e.WeighIns, e.WeighInDays}
			if got != tc.want || e.Measured != tc.wantMeasured {
				t.Errorf("days, logged between the weigh-ins, weigh-ins, days apart %v, measured %v; want %v, %v",
					got, e.Measured, tc.want, tc.wantMeasured)
			}
		})
	}
}

func TestCalibrateSaysWhenThePhaseIsItsOwn(t *testing.T) {
	t.Parallel()

	// A cut 20 days into its days after 60 of maintenance at more: the
	// estimate is from before it until its own 14 days after its first
	// 14, on its 28th day.
	start := last.AddDays(-20)
	days := map[day.Date]Day{}
	line(days, last.AddDays(30), 120, 80, -0.05, 2500)
	eatLessFrom(days, start)
	phases := []plan.Phase{
		{Type: plan.Maintain, Start: start.AddDays(-100)},
		{Type: plan.Cut, Start: start, RatePctWeek: -0.5},
	}
	e := Calibrate(days, phases, last, DefaultParams())
	own := start.AddDays(27)
	if !e.BeforePhase || e.OwnFrom != own {
		t.Fatalf("before the phase %v, its own from %s; want before it, its own from %s", e.BeforePhase, e.OwnFrom, own)
	}
	if b := Calibrate(days, phases, own.AddDays(-1), DefaultParams()); !b.BeforePhase || b.OwnFrom != own {
		t.Errorf("the day before: before the phase %v, its own from %s; want before it, from %s", b.BeforePhase, b.OwnFrom, own)
	}
	if o := Calibrate(days, phases, own, DefaultParams()); o.BeforePhase || !o.OwnFrom.IsZero() || o.From != start.AddDays(14) {
		t.Errorf("on it: before the phase %v, its own from %s, window from %s; want the phase's own from %s",
			o.BeforePhase, o.OwnFrom, o.From, start.AddDays(14))
	}
	// An estimate of the phase's own days says nothing of it.
	if e := Calibrate(days, nil, last, DefaultParams()); e.BeforePhase || !e.OwnFrom.IsZero() {
		t.Errorf("without a phase: before it %v, its own from %s", e.BeforePhase, e.OwnFrom)
	}
}

func TestExpected(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// days are the user's up to last; phases their plan.
		days   func() map[day.Date]Day
		phases []plan.Phase
		// want is how many days after last the window first gives an
		// estimate, 0 for none foreseen.
		want int
	}{
		// 9 days of 14: the 5 to come, each logged and weighed.
		{"every day logged and weighed", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 9, 80, -0.05, 2000)
			return days
		}, nil, 5},
		// A weigh-in every third day, the last on last: 3 in 9 days, 6 in
		// 18, at that pace, though 14 days would do.
		{"a weigh-in every third day", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 9, 80, -0.05, 2000)
			weighEvery(days, 3)
			return days
		}, nil, 9},
		// A weigh-in a week: 6 of them take 5 weeks more.
		{"a weigh-in a week", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 14, 80, -0.05, 2000)
			weighEvery(days, 7)
			return days
		}, nil, 28},
		// 3 days, weighed on the first alone: a third of the days ahead,
		// 5 more in 15.
		{"a first weigh-in of three days", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 3, 80, -0.05, 2000)
			for i := range 2 {
				d := days[last.AddDays(-i)]
				d.WeightKg = 0
				days[last.AddDays(-i)] = d
			}
			return days
		}, nil, 15},
		// A cut that starts with the first data leaves out its first 14
		// days: its 14 days after them end on its 28th.
		{"a phase from the first day", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 6, 80, -0.05, 2000)
			return days
		}, []plan.Phase{{Type: plan.Cut, Start: last.AddDays(-5), RatePctWeek: -0.5}}, 22},
		// A cut planned for tomorrow, the intake changing with it.
		{"a phase to come", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 9, 80, -0.05, 2000)
			return days
		}, []plan.Phase{{Type: plan.Cut, Start: last.AddDays(1), RatePctWeek: -0.5}}, 28},
		{"no weigh-in", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 9, 0, 0, 2000)
			return days
		}, nil, 0},
		{"nothing logged lately", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last.AddDays(-14), 9, 80, -0.05, 2000)
			for i := range 14 {
				days[last.AddDays(-i)] = Day{Status: day.Complete, WeightKg: 80}
			}
			return days
		}, nil, 0},
		{"no meal ever", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			for i := range 9 {
				days[last.AddDays(-i)] = Day{Status: day.Complete, WeightKg: 80}
			}
			return days
		}, nil, 0},
		// A weigh-in every 14 days: 3 in a window of 42, never 6.
		{"too few ever", func() map[day.Date]Day {
			days := map[day.Date]Day{}
			line(days, last, 14, 80, -0.05, 2000)
			weighEvery(days, 14)
			return days
		}, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			days := tc.days()
			got := Expected(days, tc.phases, last, DefaultParams())
			want := day.Date{}
			if tc.want > 0 {
				want = last.AddDays(tc.want)
			}
			if got != want {
				t.Errorf("expected on %s, want %s", got, want)
			}
		})
	}
}

// The day Expected foresees is the one Calibrate measures on, for days
// that come as they did.
func TestExpectedComesTrue(t *testing.T) {
	t.Parallel()

	days := map[day.Date]Day{}
	line(days, last.AddDays(30), 40, 80, -0.05, 2000)
	weighEvery(days, 2)
	// Up to last, 10 days of them.
	known := map[day.Date]Day{}
	for d, v := range days {
		if !last.Before(d) {
			known[d] = v
		}
	}
	on := Expected(known, nil, last, DefaultParams())
	if on.IsZero() {
		t.Fatal("no day foreseen")
	}
	if e := Calibrate(days, nil, on, DefaultParams()); !e.Measured {
		t.Errorf("on %s: no estimate, doubts %v", on, e.Doubts)
	}
	if e := Calibrate(days, nil, on.AddDays(-1), DefaultParams()); e.Measured {
		t.Errorf("the day before %s: an estimate already", on)
	}
}

func TestStarted(t *testing.T) {
	t.Parallel()

	// Weigh-ins alone are no start; a first meal is.
	days := map[day.Date]Day{last: {Status: day.Complete, WeightKg: 80}}
	if Started(days) {
		t.Error("weigh-ins alone started")
	}
	days[last.AddDays(-3)] = Day{Status: day.Complete, Meals: 1, Kcal: 2000}
	if !Started(days) {
		t.Error("a meal did not start")
	}
}
