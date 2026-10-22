package ingest

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// ErrNotHAE reports a body that is no Health Auto Export payload: no
// "data" object with a "metrics" list.
var ErrNotHAE = errors.New("not a Health Auto Export payload: want {\"data\": {\"metrics\": [...]}}")

// haeKinds maps Health Auto Export's metric names to kinds. HAE's
// workouts are not imported: training days come from elsewhere (DESIGN.md
// §7), and the exercise minutes are a metric of their own.
var haeKinds = map[string]metric.Kind{
	"step_count":             metric.Steps,
	"active_energy":          metric.ActiveKcal,
	"basal_energy_burned":    metric.BasalKcal,
	"apple_exercise_time":    metric.ExerciseMin,
	"sleep_analysis":         metric.Sleep,
	"weight_body_mass":       metric.Weight,
	"body_fat_percentage":    metric.BodyFat,
	"lean_body_mass":         metric.LeanMass,
	"resting_heart_rate":     metric.RestingHR,
	"heart_rate_variability": metric.HRV,
	"vo2_max":                metric.VO2Max,
	"vo2max":                 metric.VO2Max,
	"cardio_fitness":         metric.VO2Max,
}

// HAEMetrics lists the HAE metric names ParseHAE reads, sorted: the ones
// to turn on in the app.
func HAEMetrics() []string {
	out := make([]string, 0, len(haeKinds))
	for name := range haeKinds {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

type haePayload struct {
	Data *struct {
		Metrics *[]haeMetric `json:"metrics"`
	} `json:"data"`
}

type haeMetric struct {
	Name  string     `json:"name"`
	Units string     `json:"units"`
	Data  []haePoint `json:"data"`
}

// haePoint is one sample: a quantity, or for sleep a session with its
// stages.
type haePoint struct {
	Date       string   `json:"date"`
	Qty        *float64 `json:"qty"`
	TotalSleep *float64 `json:"totalSleep"`
	Asleep     *float64 `json:"asleep"`
	Core       *float64 `json:"core"`
	Deep       *float64 `json:"deep"`
	Rem        *float64 `json:"rem"`
}

// sample is a value at an instant; at is zero when the point gives a date
// alone.
type sample struct {
	at time.Time
	v  float64
}

// haeTime is the form of HAE's dates.
const haeTime = "2006-01-02 15:04:05 -0700"

// ParseHAE reads a Health Auto Export payload (REST API automation, JSON).
// A value belongs to the calendar date of its point, as HAE gives it: a
// day's total is dated at its midnight, and a night's sleep at the
// morning it ends. Units are converted by the ones each metric names (kJ
// to kcal, lb to kg, minutes or hours as the kind needs). When a day has
// several points of a kind:
//   - the parts of a running total (steps, active and basal energy,
//     exercise minutes) add up;
//   - the longest sleep is the night, naps aside;
//   - the earliest weighing stays, and body fat and lean mass come from
//     the same weighing, the one nearest in time;
//   - for the others (resting heart rate, HRV, VO2 max) the latest stays.
//
// Names tdeemon does not know are ignored and reported.
func ParseHAE(body []byte) (Result, error) {
	var p haePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Result{}, fmt.Errorf("health auto export payload: %w", err)
	}
	if p.Data == nil || p.Data.Metrics == nil {
		return Result{}, ErrNotHAE
	}
	samples := map[day.Date]map[metric.Kind][]sample{}
	ignored := map[string]bool{}
	var bad []Rejected
	for _, m := range *p.Data.Metrics {
		k, ok := haeKinds[m.Name]
		if !ok {
			ignored[m.Name] = true
			continue
		}
		for _, pt := range m.Data {
			v, ok := pt.value(k)
			if !ok {
				continue
			}
			d, at, err := pointDate(pt.Date)
			if err != nil {
				bad = append(bad, Rejected{Kind: k, Value: v, Reason: err.Error()})
				continue
			}
			if samples[d] == nil {
				samples[d] = map[metric.Kind][]sample{}
			}
			samples[d][k] = append(samples[d][k], sample{at, convert(k, m.Units, v)})
		}
	}
	byDay := map[day.Date]map[metric.Kind]float64{}
	for d, kinds := range samples {
		byDay[d] = pick(kinds)
	}
	r := build(byDay, ignored)
	r.Rejected = append(bad, r.Rejected...)
	return r, nil
}

// value returns the point's value for kind k: for sleep, the hours slept
// (the total, else the stages added up, else asleep or qty, as HAE fills
// them in), else the quantity.
func (pt haePoint) value(k metric.Kind) (float64, bool) {
	if k != metric.Sleep {
		return deref(pt.Qty)
	}
	if v, ok := deref(pt.TotalSleep); ok {
		return v, true
	}
	var sum float64
	var found bool
	for _, stage := range []*float64{pt.Core, pt.Deep, pt.Rem} {
		if v, ok := deref(stage); ok {
			sum, found = sum+v, true
		}
	}
	if found {
		return sum, true
	}
	if v, ok := deref(pt.Asleep); ok {
		return v, true
	}
	return deref(pt.Qty)
}

func deref(p *float64) (float64, bool) {
	if p == nil {
		return 0, false
	}
	return *p, true
}

// pointDate returns the calendar date of a point's date, as written, and
// its instant, zero when it gives a date alone.
func pointDate(s string) (day.Date, time.Time, error) {
	if t, err := time.Parse(haeTime, s); err == nil {
		d, err := day.ParseDate(s[:10])
		return d, t, err
	}
	d, err := day.ParseDate(s)
	if err != nil {
		return day.Date{}, time.Time{}, fmt.Errorf("date %q: want 2006-01-02 15:04:05 -0700, or a date alone", s)
	}
	return d, time.Time{}, nil
}

// convert turns v, in units, into k's unit.
func convert(k metric.Kind, units string, v float64) float64 {
	u := strings.ToLower(strings.TrimSpace(units))
	switch k {
	case metric.ActiveKcal, metric.BasalKcal:
		if u == "kj" || u == "kilojoules" {
			return v / 4.184
		}
	case metric.Weight, metric.LeanMass:
		if u == "lb" || u == "lbs" {
			return v * 0.45359237
		}
	case metric.Sleep:
		if u == "min" {
			return v / 60
		}
	case metric.ExerciseMin:
		if u == "hr" || u == "h" {
			return v * 60
		}
	case metric.BodyFat, metric.Steps, metric.RestingHR, metric.HRV, metric.VO2Max:
	}
	return v
}

// pick returns the value a day keeps of each kind, from its samples.
func pick(kinds map[metric.Kind][]sample) map[metric.Kind]float64 {
	out := map[metric.Kind]float64{}
	var weighed time.Time // when the day's weighing was, if it was timed
	if ws, ok := kinds[metric.Weight]; ok {
		w := earliest(ws)
		out[metric.Weight], weighed = w.v, w.at
	}
	for k, ss := range kinds {
		switch {
		case k == metric.Weight:
		case k == metric.Sleep:
			out[k] = slices.MaxFunc(ss, func(a, b sample) int { return cmp.Compare(a.v, b.v) }).v
		case k.Grows():
			var sum float64
			for _, s := range ss {
				sum += s.v
			}
			out[k] = sum
		case k.KeepsFirst() && !weighed.IsZero():
			out[k] = nearest(ss, weighed).v
		case k.KeepsFirst():
			out[k] = earliest(ss).v
		default:
			out[k] = latest(ss).v
		}
	}
	return out
}

func earliest(ss []sample) sample {
	return slices.MinFunc(ss, func(a, b sample) int { return a.at.Compare(b.at) })
}

func latest(ss []sample) sample {
	return slices.MaxFunc(ss, func(a, b sample) int { return a.at.Compare(b.at) })
}

// nearest returns the sample nearest in time to t, the earliest of those
// as near.
func nearest(ss []sample, t time.Time) sample {
	return slices.MinFunc(ss, func(a, b sample) int {
		if c := cmp.Compare(math.Abs(float64(a.at.Sub(t))), math.Abs(float64(b.at.Sub(t)))); c != 0 {
			return c
		}
		return a.at.Compare(b.at)
	})
}
