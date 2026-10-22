// Package recovery says how a user recovers: the last 7 days' sleep,
// resting heart rate and HRV against the 21 days before them, with flags
// where they fall short (DESIGN.md §5), as the predecessor's summary had
// them.
package recovery

import (
	"math"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// Flags of a recovery that falls short.
const (
	// FlagShortSleep: the last 7 days' sleep averages under ShortSleepH.
	FlagShortSleep = "short_sleep"
	// FlagHighRestingHR: the last 7 days' resting heart rate is more than
	// HighRestingHR over the base's.
	FlagHighRestingHR = "high_resting_hr"
	// FlagLowHRV: the last 7 days' HRV is more than LowHRV under the
	// base's.
	FlagLowHRV = "low_hrv"
)

// The bounds of the flags, the predecessor's: 6.5 hours of sleep, a
// resting heart rate 5 % over its base, an HRV 15 % under it.
const (
	ShortSleepH   = 6.5
	HighRestingHR = 0.05
	LowHRV        = 0.15
)

// The days: the last 7, against the 21 before them.
const (
	recentDays = 7
	baseDays   = 21
)

// An average needs MinRecentDays of the last 7 days, and MinBaseDays of
// the 21 before them, as calibration's steps guard does; of fewer it is
// none, and raises no flag.
const (
	MinRecentDays = 4
	MinBaseDays   = 5
)

// Recovery is the last 7 days' averages and their bases, 0 for none, and
// the flags they raise. A value of 0 or less is none, as no night has no
// sleep and no heart rests at 0. The averages are rounded as they are
// shown, sleep to a hundredth of an hour and the rest to a tenth, and the
// flags judged on them, so that what is shown does not say otherwise.
type Recovery struct {
	SleepH                   float64
	RestingHR, RestingHRBase float64
	HRV, HRVBase             float64
	Flags                    []string
}

// Of returns the recovery of the 7 days up to last, against the 21 before
// them, from the days' measurements.
func Of(values map[day.Date][]metric.Value, last day.Date) Recovery {
	var sleep, rhr, rhrBase, hrv, hrvBase []float64
	for i := range recentDays + baseDays {
		recent := i < recentDays
		for _, v := range values[last.AddDays(-i)] {
			if v.Value <= 0 {
				continue
			}
			switch v.Kind {
			case metric.Sleep:
				if recent {
					sleep = append(sleep, v.Value)
				}
			case metric.RestingHR:
				rhr, rhrBase = add(rhr, rhrBase, recent, v.Value)
			case metric.HRV:
				hrv, hrvBase = add(hrv, hrvBase, recent, v.Value)
			default:
			}
		}
	}
	r := Recovery{
		SleepH: round(mean(sleep, MinRecentDays), 100), RestingHR: round(mean(rhr, MinRecentDays), 10),
		RestingHRBase: round(mean(rhrBase, MinBaseDays), 10),
		HRV:           round(mean(hrv, MinRecentDays), 10), HRVBase: round(mean(hrvBase, MinBaseDays), 10),
	}
	r.flag()
	return r
}

// flag adds the flags of what falls short.
func (r *Recovery) flag() {
	if r.SleepH > 0 && r.SleepH < ShortSleepH {
		r.Flags = append(r.Flags, FlagShortSleep)
	}
	if r.RestingHR > 0 && r.RestingHRBase > 0 && r.RestingHR > r.RestingHRBase*(1+HighRestingHR) {
		r.Flags = append(r.Flags, FlagHighRestingHR)
	}
	if r.HRV > 0 && r.HRVBase > 0 && r.HRV < r.HRVBase*(1-LowHRV) {
		r.Flags = append(r.Flags, FlagLowHRV)
	}
}

// add appends v to recent or to base, as recent says.
func add(recentVs, baseVs []float64, recent bool, v float64) (recentOut, baseOut []float64) {
	if recent {
		return append(recentVs, v), baseVs
	}
	return recentVs, append(baseVs, v)
}

// round rounds v to the nearest 1/per; it divides last, so a hundredth
// prints as one.
func round(v, per float64) float64 { return math.Round(v*per) / per }

// mean returns the average of vs, 0 for fewer than lowest.
func mean(vs []float64, lowest int) float64 {
	if len(vs) < max(1, lowest) {
		return 0
	}
	var sum float64
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}
