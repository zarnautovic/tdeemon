package calibration

import (
	"math"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// Expected reads how often the user logged and weighed in over the
// expectDays up to last, of the days from the first with a meal, and
// looks expectHorizon days ahead. expectSlack keeps a count that comes to
// a need from falling under it by the rounding of floats.
const (
	expectDays    = 14
	expectHorizon = 90
	expectSlack   = 1e-9
)

// Expected returns the first last day after last whose window would give
// an estimate, if the days after last are logged and weighed in as often
// as the expectDays up to it were: one that holds Params.MinDays,
// MinLoggedDays logged days, and MinWeighIns weigh-ins, the first
// MinWeighInsSpan days before the last, at the day's end. It counts the
// days to come by those shares, not by chance, so the day is about when,
// as the window's logged days stand for those between its weigh-ins;
// a phase that starts in the meantime moves the window as Calibrate
// would, though a change of intake the days to come tell is none, which
// would move it back, is not foreseen. The zero Date when those days had
// no meal or no weigh-in, or when the day is more than expectHorizon
// days away.
func Expected(days map[day.Date]Day, phases []plan.Phase, last day.Date, p Params) day.Date {
	first := firstData(days)
	if first.IsZero() || last.Before(first) {
		return day.Date{}
	}
	from := later(last.AddDays(1-expectDays), first)
	var logged, weighed int
	for d := from; !last.Before(d); d = d.AddDays(1) {
		logged += btoi(days[d].Logged())
		weighed += btoi(days[d].Weighed())
	}
	if logged == 0 || weighed == 0 {
		return day.Date{}
	}
	n := float64(last.DaysSince(from) + 1)
	logs, weighs := float64(logged)/n, float64(weighed)/n
	changes := intakeChanges(days, phases, last, p)
	for k := 1; k <= expectHorizon; k++ {
		end := last.AddDays(k)
		start := windowFrom(end, latestChange(changes, end.AddDays(1)), first, p.WindowDays, p)
		if end.DaysSince(start)+1 < p.MinDays {
			continue
		}
		// The days to come in the window, and the window's of those up to
		// last.
		ahead := later(start, last.AddDays(1))
		coming := float64(end.DaysSince(ahead) + 1)
		weighIns, loggedDays := weighs*coming, logs*coming
		var firstWeighIn day.Date
		for d := start; !last.Before(d); d = d.AddDays(1) {
			if days[d].Weighed() {
				weighIns++
				if firstWeighIn.IsZero() {
					firstWeighIn = d
				}
			}
			loggedDays += float64(btoi(days[d].Logged()))
		}
		if firstWeighIn.IsZero() {
			// The first to come, a weigh-in in 1/weighs days.
			firstWeighIn = ahead.AddDays(int(math.Ceil(1/weighs)) - 1)
		}
		if weighIns+expectSlack >= MinWeighIns && loggedDays+expectSlack >= MinLoggedDays && end.DaysSince(firstWeighIn) >= MinWeighInsSpan {
			return end
		}
	}
	return day.Date{}
}

// Started reports whether days have a day with a meal: a window's days
// count toward an estimate from the first such day, and without one,
// though a window then reaches back as far as it would, none do yet.
func Started(days map[day.Date]Day) bool { return !firstData(days).IsZero() }

// later returns the later of a and b.
func later(a, b day.Date) day.Date {
	if a.Before(b) {
		return b
	}
	return a
}
