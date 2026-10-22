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
	// slope, as the weight is: six times, at least a week apart. Its
	// slope is LeanKgPerWeek, with the error LeanKgPerWeekSE.
	LeanMeasured                   bool
	LeanKgPerWeek, LeanKgPerWeekSE float64
	// BodyFatMeasured, BodyFatPerWeek and BodyFatPerWeekSE are the same
	// of the body fat, in % points a week.
	BodyFatMeasured                  bool
	BodyFatPerWeek, BodyFatPerWeekSE float64
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
// 7, and 7 before them.
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
		if measured(d.LeanMassKg) {
			leanX, leanY = append(leanX, float64(i)), append(leanY, d.LeanMassKg)
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
	if spread(leanX) {
		f := regress(leanX, leanY)
		b.LeanMeasured, b.LeanKgPerWeek, b.LeanKgPerWeekSE = true, 7*f.slope, 7*f.se
	}
	if spread(fatX) {
		f := regress(fatX, fatY)
		b.BodyFatMeasured, b.BodyFatPerWeek, b.BodyFatPerWeekSE = true, 7*f.slope, 7*f.se
	}
	return b
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
		n.LoggedDays += btoi(d.Status == day.Complete && d.Meals > 0 && finite(d.Kcal) && d.Kcal >= 0)
		if measured(d.LeanMassKg) {
			lean = append(lean, d.LeanMassKg)
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
