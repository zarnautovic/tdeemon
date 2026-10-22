package recovery

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

var last = day.Date{Year: 2026, Month: time.March, Day: 29}

// days returns 28 days up to last, the last 7 with sleep, rhr and hrv of
// recent's, the 21 before with base's.
func days(recent, base [3]float64) map[day.Date][]metric.Value {
	out := map[day.Date][]metric.Value{}
	for i := range 28 {
		v := base
		if i < 7 {
			v = recent
		}
		var vs []metric.Value
		for k, kind := range []metric.Kind{metric.Sleep, metric.RestingHR, metric.HRV} {
			if v[k] > 0 {
				vs = append(vs, metric.Value{Kind: kind, Value: v[k], Source: metric.Import})
			}
		}
		vs = append(vs, metric.Value{Kind: metric.Weight, Value: 80, Source: metric.Import})
		out[last.AddDays(-i)] = vs
	}
	return out
}

func TestOf(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		recent, base [3]float64
		want         []string
	}{
		{"well", [3]float64{7.5, 55, 60}, [3]float64{7.5, 55, 60}, nil},
		{"short sleep", [3]float64{6.2, 55, 60}, [3]float64{7.5, 55, 60}, []string{FlagShortSleep}},
		// 58 against 55 is 5.5 % over; 57.5 is 4.5 %.
		{"a high resting heart rate", [3]float64{7.5, 58, 60}, [3]float64{7.5, 55, 60}, []string{FlagHighRestingHR}},
		{"a resting heart rate a little up", [3]float64{7.5, 57.5, 60}, [3]float64{7.5, 55, 60}, nil},
		// 50 against 60 is 16.7 % under.
		{"a low HRV", [3]float64{7.5, 55, 50}, [3]float64{7.5, 55, 60}, []string{FlagLowHRV}},
		{"all three", [3]float64{6, 60, 45}, [3]float64{7.5, 55, 60}, []string{FlagShortSleep, FlagHighRestingHR, FlagLowHRV}},
		{"nothing recent", [3]float64{}, [3]float64{7.5, 55, 60}, nil},
		{"no base", [3]float64{7, 70, 30}, [3]float64{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := Of(days(tc.recent, tc.base), last)
			if !slices.Equal(r.Flags, tc.want) {
				t.Errorf("flags %v, want %v", r.Flags, tc.want)
			}
			got := []float64{r.SleepH, r.RestingHR, r.HRV, r.RestingHRBase, r.HRVBase}
			want := []float64{tc.recent[0], tc.recent[1], tc.recent[2], tc.base[1], tc.base[2]}
			if !slices.EqualFunc(got, want, func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }) {
				t.Errorf("recovery %+v, want the recent and base values", r)
			}
		})
	}
}

func TestOfNothing(t *testing.T) {
	t.Parallel()

	if r := Of(nil, last); r.SleepH != 0 || r.RestingHR != 0 || r.HRV != 0 || len(r.Flags) != 0 {
		t.Errorf("recovery of nothing %+v, want zeros", r)
	}
}

func TestOfFewDays(t *testing.T) {
	t.Parallel()

	// Of 3 of the last 7 days and 4 of the 21 before them: too few.
	values := map[day.Date][]metric.Value{}
	for i := range 28 {
		if i < 7 && i >= MinRecentDays-1 || i >= 7 && i >= 7+MinBaseDays-1 {
			continue
		}
		hr := 50.0
		if i < 7 {
			hr = 70
		}
		values[last.AddDays(-i)] = []metric.Value{
			{Kind: metric.Sleep, Value: 5, Source: metric.Import},
			{Kind: metric.RestingHR, Value: hr, Source: metric.Import},
		}
	}
	if r := Of(values, last); r.SleepH != 0 || r.RestingHR != 0 || r.RestingHRBase != 0 || len(r.Flags) != 0 {
		t.Errorf("recovery of too few days %+v, want none", r)
	}
	// One more of each: enough.
	values[last.AddDays(-(MinRecentDays - 1))] = values[last]
	values[last.AddDays(-(7 + MinBaseDays - 1))] = values[last.AddDays(-7)]
	if r := Of(values, last); r.SleepH != 5 || r.RestingHR != 70 || r.RestingHRBase != 50 || !slices.Equal(r.Flags, []string{FlagShortSleep, FlagHighRestingHR}) {
		t.Errorf("recovery of enough days %+v", r)
	}
}

func TestOfLeavesOutZero(t *testing.T) {
	t.Parallel()

	// A night of 0 is a watch not worn, not a night without sleep.
	values := days([3]float64{7.5, 55, 60}, [3]float64{7.5, 55, 60})
	values[last] = append(values[last], metric.Value{Kind: metric.Sleep, Value: 0, Source: metric.Import})
	values[last.AddDays(-1)] = []metric.Value{{Kind: metric.Sleep, Value: 0, Source: metric.Import}}
	if r := Of(values, last); r.SleepH != 7.5 || len(r.Flags) != 0 {
		t.Errorf("recovery %+v, want 7.5 hours of sleep and no flag", r)
	}
}
