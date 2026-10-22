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
	dates := make([]day.Date, 0, len(days))
	for d, v := range days {
		if weighed(v) {
			dates = append(dates, d)
		}
	}
	slices.SortFunc(dates, day.Date.Compare)
	out := make([]Point, 0, len(dates))
	var kg float64
	for i, d := range dates {
		if w := days[d].WeightKg; i == 0 {
			kg = w
		} else {
			kg += (1 - math.Pow(1-alpha, float64(d.DaysSince(dates[i-1])))) * (w - kg)
		}
		out = append(out, Point{Date: d, Kg: kg})
	}
	return out
}

// trendOn returns the trend weight on day d, from the weigh-ins up to it;
// 0 before the first.
func trendOn(days map[day.Date]Day, d day.Date, alpha float64) float64 {
	trend := Trend(days, alpha)
	i, found := slices.BinarySearchFunc(trend, d, func(pt Point, d day.Date) int { return pt.Date.Compare(d) })
	if found {
		i++
	}
	if i == 0 {
		return 0
	}
	return trend[i-1].Kg
}
