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
	Name  string `json:"name"`
	Units string `json:"units"`
	// Data is read for the metrics tdeemon knows, a point at a time, so a
	// metric it skips or a point that does not read costs nothing else.
	Data json.RawMessage `json:"data"`
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
	// Value names the stage of a sleep segment, in an export HAE does not
	// summarize; a segment is no night.
	Value *string `json:"value"`
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
	c := collector{samples: map[day.Date]map[metric.Kind][]sample{}, ignored: map[string]bool{}}
	for _, m := range *p.Data.Metrics {
		c.metric(m)
	}
	byDay := map[day.Date]map[metric.Kind]float64{}
	for d, kinds := range c.samples {
		byDay[d] = pick(kinds)
	}
	r := build(byDay, c.ignored)
	r.Rejected = append(c.bad, r.Rejected...)
	return r, nil
}

// collector gathers a payload's samples by day and kind, and what it
// leaves out.
type collector struct {
	samples map[day.Date]map[metric.Kind][]sample
	ignored map[string]bool
	bad     []Rejected
}

// metric reads the points of m, each checked on its own before a day's
// value is picked among them.
func (c *collector) metric(m haeMetric) {
	if m.Name == "" {
		return
	}
	k, ok := haeKinds[m.Name]
	if !ok {
		c.ignored[m.Name] = true
		return
	}
	conv, err := units(k, m.Units)
	if err != nil {
		c.bad = append(c.bad, Rejected{Kind: k, Reason: err.Error()})
		return
	}
	var points []json.RawMessage
	if err := json.Unmarshal(m.Data, &points); err != nil {
		c.bad = append(c.bad, Rejected{Kind: k, Reason: "the data is no list of points"})
		return
	}
	for _, raw := range points {
		c.point(k, conv, raw)
	}
}

func (c *collector) point(k metric.Kind, conv func(float64) float64, raw json.RawMessage) {
	var pt haePoint
	if err := json.Unmarshal(raw, &pt); err != nil {
		c.bad = append(c.bad, Rejected{Kind: k, Reason: "a point does not read: " + err.Error()})
		return
	}
	if k == metric.Sleep && pt.Value != nil {
		c.bad = append(c.bad, Rejected{
			Kind: k, RawDate: pt.Date,
			Reason: "a sleep stage (" + *pt.Value + "), not a night: turn on Summarize Data in Health Auto Export",
		})
		return
	}
	v, ok := pt.value(k)
	if !ok {
		return
	}
	v = conv(v)
	d, at, err := pointDate(pt.Date)
	if err != nil {
		c.bad = append(c.bad, Rejected{Kind: k, Value: v, RawDate: pt.Date, Reason: err.Error()})
		return
	}
	if err := (metric.Value{Kind: k, Value: v, Source: metric.Import}).Validate(); err != nil {
		c.bad = append(c.bad, Rejected{Date: d, Kind: k, Value: v, Reason: err.Error()})
		return
	}
	if c.samples[d] == nil {
		c.samples[d] = map[metric.Kind][]sample{}
	}
	c.samples[d][k] = append(c.samples[d][k], sample{at, v})
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
// its instant, zero when it gives no time that reads: HAE writes
// 2006-01-02 15:04:05 -0700, and a date that starts so is that date.
func pointDate(s string) (day.Date, time.Time, error) {
	if len(s) < 10 {
		return day.Date{}, time.Time{}, fmt.Errorf("date %q: want 2006-01-02 15:04:05 -0700", s)
	}
	d, err := day.ParseDate(s[:10])
	if err != nil {
		return day.Date{}, time.Time{}, fmt.Errorf("date %q: want 2006-01-02 15:04:05 -0700", s)
	}
	for _, layout := range []string{haeTime, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return d, t, nil
		}
	}
	return d, time.Time{}, nil
}

// unitsOf lists the units each kind takes, with the conversion to its
// own; "" is the kind's own unit, for a source that names none.
var unitsOf = map[metric.Kind]map[string]float64{
	metric.Steps:       {"": 1, "count": 1},
	metric.ActiveKcal:  {"": 1, "kcal": 1, "kj": 1 / 4.184, "kilojoules": 1 / 4.184},
	metric.BasalKcal:   {"": 1, "kcal": 1, "kj": 1 / 4.184, "kilojoules": 1 / 4.184},
	metric.ExerciseMin: {"": 1, "min": 1, "hr": 60, "h": 60},
	metric.Sleep:       {"": 1, "hr": 1, "h": 1, "min": 1.0 / 60},
	metric.Weight:      {"": 1, "kg": 1, "lb": 0.45359237, "lbs": 0.45359237},
	metric.LeanMass:    {"": 1, "kg": 1, "lb": 0.45359237, "lbs": 0.45359237},
	metric.BodyFat:     {"": 1, "%": 1},
	metric.RestingHR:   {"": 1, "count/min": 1, "bpm": 1},
	metric.HRV:         {"": 1, "ms": 1},
}

// units returns the conversion of units into k's unit, or an error for a
// unit k does not take: a value in it would pass for another (25 st for
// 25 kg). VO2 max takes any unit in ml (HAE writes it several ways).
func units(k metric.Kind, name string) (func(float64) float64, error) {
	u := strings.ToLower(strings.TrimSpace(name))
	if k == metric.VO2Max && (u == "" || strings.HasPrefix(u, "ml")) {
		return func(v float64) float64 { return v }, nil
	}
	f, ok := unitsOf[k][u]
	if !ok {
		var want []string
		for w := range unitsOf[k] {
			if w != "" {
				want = append(want, w)
			}
		}
		if k == metric.VO2Max {
			want = []string{"ml/(kg·min)"}
		}
		slices.Sort(want)
		return nil, fmt.Errorf("units %q: want %s", name, strings.Join(want, " or "))
	}
	return func(v float64) float64 { return v * f }, nil
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
