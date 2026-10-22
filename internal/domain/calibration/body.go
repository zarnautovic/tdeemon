package calibration

import (
	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// Body is what a window says of the body besides the weight: the data of
// the verdict's guards (DESIGN.md §7).
type Body struct {
	// StepsRecent is the average steps of the window's last 7 days, and
	// StepsBefore of the up to 21 days before them in the window; each 0
	// for too few days with steps, 4 of the 7 and 7 before.
	StepsRecent, StepsBefore float64
	// LeanMeasured is true when the lean mass was measured enough for a
	// slope, as the weight is: six times, from the first to the last at
	// least a week apart, and the last in the window's last 14 days. A day
	// without a lean mass but with a weigh-in and a body fat has the
	// weight's share that is not fat. The slope is LeanKgPerWeek, and
	// LeanHalfWidth half its 80 % interval.
	LeanMeasured                 bool
	LeanKgPerWeek, LeanHalfWidth float64
	// BodyFatMeasured, BodyFatPerWeek and BodyFatHalfWidth are the same
	// of the body fat, in % points a week.
	BodyFatMeasured                  bool
	BodyFatPerWeek, BodyFatHalfWidth float64
}

// Now is what the 7 days up to a day say, whatever the window: the data
// of the limits (limits.State).
type Now struct {
	// LoggedDays and WeighIns count the days logged and weighed.
	LoggedDays, WeighIns int
	// LeanMassKg and ActiveKcal are the averages of the days that have
	// one, 0 for none.
	LeanMassKg, ActiveKcal float64
}

// The fewest days with steps the activity guard compares: 4 of the last
// 7, and 7 of the up to 21 before them.
const (
	minRecentSteps = 4
	minBeforeSteps = 7
	stepsBefore    = 21
)

// bodyOf returns what the days from to to say of the body.
func bodyOf(days map[day.Date]Day, from, to day.Date) Body {
	n := to.DaysSince(from) + 1
	var b Body
	var leanX, leanY, fatX, fatY []float64
	var recent, before []float64
	for i := range max(n, 0) {
		d := days[from.AddDays(i)]
		if d.Status == day.Excluded {
			continue
		}
		if kg, ok := leanMass(d); ok {
			leanX, leanY = append(leanX, float64(i)), append(leanY, kg)
		}
		if measured(d.BodyFatPct) {
			fatX, fatY = append(fatX, float64(i)), append(fatY, d.BodyFatPct)
		}
		switch {
		case !measured(d.Steps):
		case i >= n-week:
			recent = append(recent, d.Steps)
		case i >= n-week-stepsBefore:
			before = append(before, d.Steps)
		}
	}
	if len(recent) >= minRecentSteps && len(before) >= minBeforeSteps {
		b.StepsRecent, b.StepsBefore = mean(recent), mean(before)
	}
	if b.LeanMeasured = spread(leanX) && fresh(leanX, n); b.LeanMeasured {
		b.LeanKgPerWeek, b.LeanHalfWidth = slopeOf(leanX, leanY)
	}
	if b.BodyFatMeasured = spread(fatX) && fresh(fatX, n); b.BodyFatMeasured {
		b.BodyFatPerWeek, b.BodyFatHalfWidth = slopeOf(fatX, fatY)
	}
	return b
}

// bodyRecent is how many of a window's last days its last measurement of
// the body must lie in: a slope that ended weeks ago says little of the
// body now.
const bodyRecent = 14

// fresh reports whether the last of the days xs, in order, of a window
// of n days lies in its last bodyRecent.
func fresh(xs []float64, n int) bool { return xs[len(xs)-1] >= float64(n-bodyRecent) }

// slopeOf returns the slope a week of the values ys on the days xs, and
// half its 80 % interval, by the regression the weight's slope has.
func slopeOf(xs, ys []float64) (perWeek, halfWidth float64) {
	f := regress(xs, ys)
	return 7 * f.slope, 7 * tQuantile(f.df) * f.se
}

// leanMass returns d's lean mass: the one measured, else the weight's
// share that is not fat, when the day has a weigh-in and a body fat.
func leanMass(d Day) (float64, bool) {
	switch {
	case measured(d.LeanMassKg):
		return d.LeanMassKg, true
	case weighed(d) && measured(d.BodyFatPct) && d.BodyFatPct < 100:
		return d.WeightKg * (1 - d.BodyFatPct/100), true
	}
	return 0, false
}

// nowOf returns what the 7 days up to last say.
func nowOf(days map[day.Date]Day, last day.Date) Now {
	var n Now
	var lean, active []float64
	for i := range week {
		d := days[last.AddDays(-i)]
		if d.Status == day.Excluded {
			continue
		}
		n.WeighIns += btoi(weighed(d))
		n.LoggedDays += btoi(loggedDay(d))
		if kg, ok := leanMass(d); ok {
			lean = append(lean, kg)
		}
		if measured(d.ActiveKcal) {
			active = append(active, d.ActiveKcal)
		}
	}
	if len(lean) > 0 {
		n.LeanMassKg = mean(lean)
	}
	if len(active) > 0 {
		n.ActiveKcal = mean(active)
	}
	return n
}

// measured reports whether v is a measurement: a number more than 0.
func measured(v float64) bool { return v > 0 && finite(v) }
