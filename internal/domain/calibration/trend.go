package calibration

import (
	"math"
	"slices"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// Point is the trend weight on a day.
type Point struct {
	Date day.Date
	Kg   float64
}

// Trend returns the trend weight on each day of days with a weigh-in, in
// the order of their dates: an exponentially smoothed average, starting
// at the first weigh-in, that takes alpha of each weigh-in's difference
// from the trend a day. After a gap of n days it takes 1 − (1 − alpha)ⁿ,
// as if the days between had passed, so a trend after weeks without a
// weigh-in follows the weight again at once. It shows where the weight is
// going without the day-to-day swing of water and salt; the TDEE's slope
// is the regression's, which a trend that lags behind would flatten.
func Trend(days map[day.Date]Day, alpha float64) []Point {
	dates := weighIns(days)
	out := make([]Point, 0, len(dates))
	smooth(days, dates, alpha, func(d day.Date, kg float64) { out = append(out, Point{Date: d, Kg: kg}) })
	return out
}

// trendOn returns the trend weight on day d, from the weigh-ins up to it;
// 0 before the first.
func trendOn(days map[day.Date]Day, d day.Date, alpha float64) float64 {
	dates := slices.DeleteFunc(weighIns(days), d.Before)
	var kg float64
	smooth(days, dates, alpha, func(_ day.Date, v float64) { kg = v })
	return kg
}

// weighIns returns the dates of days with a weigh-in, in order.
func weighIns(days map[day.Date]Day) []day.Date {
	dates := make([]day.Date, 0, len(days))
	for d, v := range days {
		if weighed(v) {
			dates = append(dates, d)
		}
	}
	slices.SortFunc(dates, func(a, b day.Date) int { return a.DaysSince(b) })
	return dates
}

// smooth runs the trend over the weigh-ins on dates, in order, and gives
// visit the trend on each.
func smooth(days map[day.Date]Day, dates []day.Date, alpha float64, visit func(day.Date, float64)) {
	var kg float64
	for i, d := range dates {
		if w := days[d].WeightKg; i == 0 {
			kg = w
		} else {
			kg += (1 - math.Pow(1-alpha, float64(d.DaysSince(dates[i-1])))) * (w - kg)
		}
		visit(d, kg)
	}
}
