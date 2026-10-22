package calibration

import (
	"slices"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// Point is the trend weight on a day.
type Point struct {
	Date day.Date
	Kg   float64
}

// Trend returns the trend weight on each day of days with a weigh-in, in
// the order of their dates: an exponentially smoothed average that takes
// alpha of each weigh-in's difference from the trend before, starting at
// the first weigh-in. A day without one leaves the trend as it was. The
// trend shows where the weight is going without the day-to-day swing of
// water and salt; the TDEE's slope is the regression's, which a trend
// that lags behind would flatten.
func Trend(days map[day.Date]Day, alpha float64) []Point {
	dates := make([]day.Date, 0, len(days))
	for d, v := range days {
		if v.WeightKg > 0 {
			dates = append(dates, d)
		}
	}
	slices.SortFunc(dates, func(a, b day.Date) int { return a.DaysSince(b) })
	out := make([]Point, 0, len(dates))
	var kg float64
	for i, d := range dates {
		if w := days[d].WeightKg; i == 0 {
			kg = w
		} else {
			kg += alpha * (w - kg)
		}
		out = append(out, Point{Date: d, Kg: kg})
	}
	return out
}

// trendOn returns the trend weight on day d, from the weigh-ins up to it;
// 0 before the first.
func trendOn(days map[day.Date]Day, d day.Date, alpha float64) float64 {
	var kg float64
	for _, pt := range Trend(days, alpha) {
		if d.Before(pt.Date) {
			break
		}
		kg = pt.Kg
	}
	return kg
}
