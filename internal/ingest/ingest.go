// Package ingest turns what devices and apps send into a day's metrics:
// a Health Auto Export payload (ParseHAE), or tdeemon's own simple form
// (FromDays). It decides which value of a kind a day keeps when a payload
// holds several, converts units by the ones the source names, and checks
// each value on its own, so one bad value does not cost the others; the
// merge into what a day holds is the store's (metric.Merge).
package ingest

import (
	"slices"
	"strings"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// Day is the values a payload gives one day, at most one of each kind,
// every one valid, in the order of metric.Kinds.
type Day struct {
	Date   day.Date
	Values []metric.Value
}

// Rejected is what a payload gives that is left out:
//   - a value that breaks its kind's rule (metric.Value.Validate), with its
//     Date, in the kind's unit;
//   - a point whose date does not read, with the date as written (RawDate),
//     the zero Date and its value in the kind's unit;
//   - a source's sleep stages, once, with the first one's date as written;
//   - a point or a metric that does not read at all, or a metric in a unit
//     its kind does not take, with no date and no value.
type Rejected struct {
	Date    day.Date
	RawDate string
	Kind    metric.Kind
	Value   float64
	Reason  string
}

// Result is what a payload gives: the days with their valid values, in
// order; the values refused; and the names of the metrics tdeemon does
// not know, sorted, which are left out.
type Result struct {
	Days     []Day
	Rejected []Rejected
	Ignored  []string
}

// build turns values by day into a Result: each value checked as an
// import, the days and kinds in order.
func build(byDay map[day.Date]map[metric.Kind]float64, ignored map[string]bool) Result {
	var r Result
	dates := make([]day.Date, 0, len(byDay))
	for d := range byDay {
		dates = append(dates, d)
	}
	slices.SortFunc(dates, func(a, b day.Date) int { return strings.Compare(a.String(), b.String()) })
	for _, d := range dates {
		out := Day{Date: d}
		for _, k := range metric.Kinds() {
			v, ok := byDay[d][k]
			if !ok {
				continue
			}
			mv := metric.Value{Kind: k, Value: v, Source: metric.Import}
			if err := mv.Validate(); err != nil {
				r.Rejected = append(r.Rejected, Rejected{Date: d, Kind: k, Value: v, Reason: err.Error()})
				continue
			}
			out.Values = append(out.Values, mv)
		}
		if out.Values != nil {
			r.Days = append(r.Days, out)
		}
	}
	for name := range ignored {
		r.Ignored = append(r.Ignored, name)
	}
	slices.Sort(r.Ignored)
	return r
}

// FromDays reads tdeemon's own form: for each date (YYYY-MM-DD), values by
// the names of metric.Kinds, in their units (kg, %, count, kcal, hours,
// beats per minute, ms, ml/kg/min, minutes). A name that is no kind is
// ignored and reported; a date that does not read is an error.
func FromDays(days map[string]map[string]float64) (Result, error) {
	byDay := map[day.Date]map[metric.Kind]float64{}
	ignored := map[string]bool{}
	for s, values := range days {
		d, err := day.ParseDate(s)
		if err != nil {
			return Result{}, err
		}
		for name, v := range values {
			k := metric.Kind(name)
			if !k.Known() {
				ignored[name] = true
				continue
			}
			if byDay[d] == nil {
				byDay[d] = map[metric.Kind]float64{}
			}
			byDay[d][k] = v
		}
	}
	return build(byDay, ignored), nil
}
