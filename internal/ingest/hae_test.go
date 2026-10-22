package ingest

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// The values in these payloads are made up.

func parseHAE(t *testing.T, body string) Result {
	t.Helper()
	r, err := ParseHAE([]byte(body))
	if err != nil {
		t.Fatalf("ParseHAE: %v", err)
	}
	return r
}

// valueOf returns the value r gives kind k on date d.
func valueOf(t *testing.T, r Result, d string, k metric.Kind) (float64, bool) {
	t.Helper()
	for _, dd := range r.Days {
		if dd.Date.String() != d {
			continue
		}
		for _, v := range dd.Values {
			if v.Kind == k {
				if v.Source != metric.Import {
					t.Errorf("%s %s: source %s, want import", d, k, v.Source)
				}
				return v.Value, true
			}
		}
	}
	return 0, false
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestParseHAE(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "step_count", "units": "count", "data": [
			{"date": "2026-10-20 00:00:00 +0200", "qty": 6400.4}
		]},
		{"name": "active_energy", "units": "kJ", "data": [
			{"date": "2026-10-20 00:00:00 +0200", "qty": 2092}
		]},
		{"name": "basal_energy_burned", "units": "kcal", "data": [
			{"date": "2026-10-20 00:00:00 +0200", "qty": 1650}
		]},
		{"name": "apple_exercise_time", "units": "min", "data": [
			{"date": "2026-10-20 00:00:00 +0200", "qty": 42}
		]},
		{"name": "resting_heart_rate", "units": "count/min", "data": [
			{"date": "2026-10-20 00:00:00 +0200", "qty": 54}
		]},
		{"name": "heart_rate_variability", "units": "ms", "data": [
			{"date": "2026-10-20 00:00:00 +0200", "qty": 61.5}
		]},
		{"name": "vo2_max", "units": "ml/(kg·min)", "data": [
			{"date": "2026-10-20 00:00:00 +0200", "qty": 44.1}
		]},
		{"name": "weight_body_mass", "units": "kg", "data": [
			{"date": "2026-10-21 07:05:00 +0200", "qty": 78.4}
		]},
		{"name": "walking_running_distance", "units": "km", "data": [
			{"date": "2026-10-20 00:00:00 +0200", "qty": 4.2}
		]},
		{"name": "flights_climbed", "units": "count", "data": []},
		{"name": "stand_hour", "units": "count", "data": []},
		{"name": "dietary_water", "units": "ml", "data": []},
		{"name": "blood_oxygen_saturation", "units": "%", "data": []},
		{"name": "apple_stand_time", "units": "min", "data": []}
	]}}`)
	for _, tc := range []struct {
		date string
		kind metric.Kind
		want float64
	}{
		{"2026-10-20", metric.Steps, 6400.4},
		{"2026-10-20", metric.ActiveKcal, 500}, // 2092 kJ
		{"2026-10-20", metric.BasalKcal, 1650},
		{"2026-10-20", metric.ExerciseMin, 42},
		{"2026-10-20", metric.RestingHR, 54},
		{"2026-10-20", metric.HRV, 61.5},
		{"2026-10-20", metric.VO2Max, 44.1},
		{"2026-10-21", metric.Weight, 78.4},
	} {
		if got, ok := valueOf(t, r, tc.date, tc.kind); !ok || !near(got, tc.want) {
			t.Errorf("%s %s = %v, %v; want %v", tc.date, tc.kind, got, ok, tc.want)
		}
	}
	if len(r.Days) != 2 || r.Days[0].Date.String() != "2026-10-20" {
		t.Errorf("Days = %+v, want 20 and 21 October in order", r.Days)
	}
	// The values of a day come in the order of metric.Kinds.
	var kinds []metric.Kind
	for _, v := range r.Days[0].Values {
		kinds = append(kinds, v.Kind)
	}
	want := []metric.Kind{metric.Steps, metric.ActiveKcal, metric.BasalKcal, metric.RestingHR, metric.HRV, metric.VO2Max, metric.ExerciseMin}
	if !slices.Equal(kinds, want) {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}
	if !slices.Equal(r.Ignored, []string{"apple_stand_time", "blood_oxygen_saturation", "dietary_water", "flights_climbed", "stand_hour", "walking_running_distance"}) {
		t.Errorf("Ignored = %v", r.Ignored)
	}
	if r.Rejected != nil {
		t.Errorf("Rejected = %+v", r.Rejected)
	}
}

// A sync of part of the day sends the hours so far; the parts add up.
func TestParseHAERunningTotalsAddUp(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "step_count", "units": "count", "data": [
			{"date": "2026-10-21 08:00:00 +0200", "qty": 1200},
			{"date": "2026-10-21 09:00:00 +0200", "qty": 800},
			{"date": "2026-10-22 00:30:00 +0200", "qty": 150}
		]},
		{"name": "apple_exercise_time", "units": "hr", "data": [
			{"date": "2026-10-21 18:00:00 +0200", "qty": 0.5},
			{"date": "2026-10-21 19:00:00 +0200", "qty": 0.25}
		]}
	]}}`)
	if got, _ := valueOf(t, r, "2026-10-21", metric.Steps); got != 2000 {
		t.Errorf("steps = %v, want 2000", got)
	}
	// Steps after midnight belong to the calendar day of their point.
	if got, _ := valueOf(t, r, "2026-10-22", metric.Steps); got != 150 {
		t.Errorf("steps after midnight = %v, want 150 on the 22nd", got)
	}
	if got, _ := valueOf(t, r, "2026-10-21", metric.ExerciseMin); got != 45 {
		t.Errorf("exercise = %v, want 45 minutes", got)
	}
}

// The night is the longest sleep of the date it ends on; a nap is not.
func TestParseHAESleep(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "sleep_analysis", "units": "hr", "data": [
			{"date": "2026-10-21 15:30:00 +0200", "totalSleep": 0.5},
			{"date": "2026-10-21 06:40:00 +0200", "totalSleep": 7.25, "asleep": 0, "core": 4, "deep": 1, "rem": 2},
			{"date": "2026-10-22 07:00:00 +0200", "core": 3.5, "deep": 1.5, "rem": 1.25},
			{"date": "2026-10-23 07:00:00 +0200", "asleep": 6.5},
			{"date": "2026-10-24 07:00:00 +0200", "qty": 6.75}
		]},
		{"name": "sleep_analysis", "units": "min", "data": [
			{"date": "2026-10-25 07:00:00 +0200", "totalSleep": 450}
		]}
	]}}`)
	for d, want := range map[string]float64{
		"2026-10-21": 7.25, // the night, not the nap; totalSleep over the stages
		"2026-10-22": 6.25, // the stages, when there is no total
		"2026-10-23": 6.5,  // asleep
		"2026-10-24": 6.75, // qty
		"2026-10-25": 7.5,  // minutes
	} {
		if got, ok := valueOf(t, r, d, metric.Sleep); !ok || !near(got, want) {
			t.Errorf("sleep on %s = %v, %v; want %v", d, got, ok, want)
		}
	}
}

// The day's earliest weighing stays, and body fat and lean mass come from
// that weighing: the evening's does not lock the day.
func TestParseHAEWeighings(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "weight_body_mass", "units": "kg", "data": [
			{"date": "2026-10-21 20:15:00 +0200", "qty": 79.6},
			{"date": "2026-10-21 07:02:00 +0200", "qty": 78.9}
		]},
		{"name": "body_fat_percentage", "units": "%", "data": [
			{"date": "2026-10-21 06:30:00 +0200", "qty": 19.9},
			{"date": "2026-10-21 07:02:30 +0200", "qty": 20.4},
			{"date": "2026-10-21 20:15:00 +0200", "qty": 21.1}
		]},
		{"name": "lean_body_mass", "units": "lb", "data": [
			{"date": "2026-10-21 07:03:00 +0200", "qty": 138.5},
			{"date": "2026-10-21 20:16:00 +0200", "qty": 139}
		]},
		{"name": "body_fat_percentage", "units": "%", "data": [
			{"date": "2026-10-22 08:00:00 +0200", "qty": 20.2},
			{"date": "2026-10-22 07:00:00 +0200", "qty": 20.6}
		]}
	]}}`)
	if got, _ := valueOf(t, r, "2026-10-21", metric.Weight); got != 78.9 {
		t.Errorf("weight = %v, want the morning's 78.9", got)
	}
	if got, _ := valueOf(t, r, "2026-10-21", metric.BodyFat); got != 20.4 {
		t.Errorf("body fat = %v, want 20.4, the morning weighing's", got)
	}
	if got, _ := valueOf(t, r, "2026-10-21", metric.LeanMass); !near(got, 138.5*0.45359237) {
		t.Errorf("lean mass = %v, want 138.5 lb in kg", got)
	}
	// With no weight that day, the earliest of each.
	if got, _ := valueOf(t, r, "2026-10-22", metric.BodyFat); got != 20.6 {
		t.Errorf("body fat with no weight = %v, want the earliest, 20.6", got)
	}
}

// Two samples as near to the weighing: the earlier.
func TestParseHAEWeighingTie(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "weight_body_mass", "units": "kg", "data": [{"date": "2026-10-21 07:00:00 +0200", "qty": 81}]},
		{"name": "body_fat_percentage", "units": "%", "data": [
			{"date": "2026-10-21 07:05:00 +0200", "qty": 23},
			{"date": "2026-10-21 06:55:00 +0200", "qty": 22}
		]}
	]}}`)
	if got, _ := valueOf(t, r, "2026-10-21", metric.BodyFat); got != 22 {
		t.Errorf("body fat = %v, want the earlier of two as near, 22", got)
	}
}

// HAE has called VO2 max by three names.
func TestParseHAEVO2MaxNames(t *testing.T) {
	t.Parallel()

	for i, name := range []string{"vo2_max", "vo2max", "cardio_fitness"} {
		r := parseHAE(t, `{"data": {"metrics": [{"name": "`+name+`", "units": "ml/(kg·min)", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 4`+string(rune('0'+i))+`}]}]}}`)
		if got, ok := valueOf(t, r, "2026-10-21", metric.VO2Max); !ok || got != float64(40+i) {
			t.Errorf("%s = %v, %v", name, got, ok)
		}
	}
}

// The fields of a sleep session mean nothing for another kind: its value
// is the quantity.
func TestParseHAEQuantityOfOtherKinds(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "step_count", "units": "count", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 3000, "totalSleep": 7}]},
		{"name": "resting_heart_rate", "units": "count/min", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 55, "core": 4}]}
	]}}`)
	if got, _ := valueOf(t, r, "2026-10-21", metric.Steps); got != 3000 {
		t.Errorf("steps = %v, want the quantity, 3000", got)
	}
	if got, _ := valueOf(t, r, "2026-10-21", metric.RestingHR); got != 55 {
		t.Errorf("resting heart rate = %v, want the quantity, 55", got)
	}
}

// For a reading that replaces the one before, the latest of the day.
func TestParseHAELatestReading(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "resting_heart_rate", "units": "count/min", "data": [
			{"date": "2026-10-21 23:00:00 +0200", "qty": 52},
			{"date": "2026-10-21 08:00:00 +0200", "qty": 57}
		]},
		{"name": "weight_body_mass", "units": "lbs", "data": [
			{"date": "2026-10-21 07:00:00 +0200", "qty": 176}
		]}
	]}}`)
	if got, _ := valueOf(t, r, "2026-10-21", metric.RestingHR); got != 52 {
		t.Errorf("resting heart rate = %v, want the latest, 52", got)
	}
	if got, _ := valueOf(t, r, "2026-10-21", metric.Weight); !near(got, 176*0.45359237) {
		t.Errorf("weight = %v, want 176 lb in kg", got)
	}
}

// A value out of its kind's bounds is refused alone; the day keeps the
// others.
func TestParseHAERefusesAValueAlone(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "sleep_analysis", "units": "hr", "data": [
			{"date": "2026-10-21 07:00:00 +0200", "totalSleep": 30}
		]},
		{"name": "step_count", "units": "count", "data": [
			{"date": "2026-10-21 00:00:00 +0200", "qty": 5000},
			{"date": "late on the 21st", "qty": 100}
		]},
		{"name": "weight_body_mass", "units": "kg", "data": [
			{"date": "2026-10-21 07:00:00 +0200"}
		]}
	]}}`)
	if got, ok := valueOf(t, r, "2026-10-21", metric.Steps); !ok || got != 5000 {
		t.Errorf("steps = %v, %v; want 5000 kept", got, ok)
	}
	if _, ok := valueOf(t, r, "2026-10-21", metric.Weight); ok {
		t.Error("a point without a quantity gave a weight")
	}
	if len(r.Rejected) != 2 {
		t.Fatalf("Rejected = %+v, want the sleep and the bad date", r.Rejected)
	}
	if sleep := r.Rejected[0]; sleep.Kind != metric.Sleep || sleep.Date.String() != "2026-10-21" || sleep.Value != 30 ||
		!strings.Contains(sleep.Reason, "want 0 to 24 h") {
		t.Errorf("the sleep = %+v", sleep)
	}
	if bad := r.Rejected[1]; bad.Kind != metric.Steps || !bad.Date.IsZero() || bad.RawDate != "late on the 21st" || bad.Value != 100 ||
		!strings.Contains(bad.Reason, `date "late on the 21st"`) {
		t.Errorf("the bad date = %+v", bad)
	}
}

// A point dated without a time belongs to that date.
func TestParseHAEDateAlone(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "weight_body_mass", "units": "kg", "data": [{"date": "2026-10-21", "qty": 80.5}]},
		{"name": "body_fat_percentage", "units": "%", "data": [
			{"date": "2026-10-21 09:00:00 +0200", "qty": 22},
			{"date": "2026-10-21", "qty": 21}
		]}
	]}}`)
	if got, _ := valueOf(t, r, "2026-10-21", metric.Weight); got != 80.5 {
		t.Errorf("weight = %v", got)
	}
	// The weighing has no time, so body fat is the earliest of its own.
	if got, _ := valueOf(t, r, "2026-10-21", metric.BodyFat); got != 21 {
		t.Errorf("body fat = %v, want 21", got)
	}
}

func TestParseHAERefusesWhatIsNoPayload(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{}`, `{"data": {}}`, `{"data": {"workouts": []}}`} {
		if _, err := ParseHAE([]byte(body)); !errors.Is(err, ErrNotHAE) {
			t.Errorf("ParseHAE(%s) = %v, want ErrNotHAE", body, err)
		}
	}
	if _, err := ParseHAE([]byte(`[]`)); err == nil || errors.Is(err, ErrNotHAE) {
		t.Errorf("ParseHAE of a list = %v, want an error that it does not read", err)
	}

	if r, err := ParseHAE([]byte(`{"data": {"metrics": []}}`)); err != nil || r.Days != nil {
		t.Errorf("an empty sync = %+v, %v", r, err)
	}
}

func TestHAEMetrics(t *testing.T) {
	t.Parallel()

	names := HAEMetrics()
	if !slices.IsSorted(names) || !slices.Contains(names, "step_count") || len(names) != len(haeKinds) {
		t.Errorf("HAEMetrics() = %v", names)
	}
	// Every kind but none is read from some HAE metric.
	seen := map[metric.Kind]bool{}
	for _, k := range haeKinds {
		seen[k] = true
	}
	for _, k := range metric.Kinds() {
		if !seen[k] {
			t.Errorf("no HAE metric gives %s", k)
		}
	}
}

func TestFromDays(t *testing.T) {
	t.Parallel()

	r, err := FromDays(map[string]map[string]float64{
		"2026-10-21": {"weight": 80.1, "steps": 9000, "mood": 4},
		"2026-10-20": {"sleep_h": 31},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Day{{Date: day.Date{Year: 2026, Month: 10, Day: 21}, Values: []metric.Value{
		{Kind: metric.Weight, Value: 80.1, Source: metric.Import},
		{Kind: metric.Steps, Value: 9000, Source: metric.Import},
	}}}
	if len(r.Days) != 1 || r.Days[0].Date != want[0].Date || !slices.Equal(r.Days[0].Values, want[0].Values) {
		t.Errorf("Days = %+v, want %+v", r.Days, want)
	}
	if !slices.Equal(r.Ignored, []string{"mood"}) {
		t.Errorf("Ignored = %v", r.Ignored)
	}
	if len(r.Rejected) != 1 || r.Rejected[0].Kind != metric.Sleep || r.Rejected[0].Date.String() != "2026-10-20" {
		t.Errorf("Rejected = %+v", r.Rejected)
	}
	if _, err := FromDays(map[string]map[string]float64{"21.10.2026.": {"steps": 1}}); err == nil {
		t.Error("a date that does not read passed")
	}
	if r, err := FromDays(nil); err != nil || r.Days != nil || r.Ignored != nil {
		t.Errorf("FromDays(nil) = %+v, %v", r, err)
	}
}

// A point or a metric that does not read is left out alone, and a metric
// tdeemon skips is not read at all.
func TestParseHAELeavesOutWhatDoesNotRead(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "some_new_metric", "units": "x", "data": [{"date": 1729468800, "qty": "5"}]},
		{"name": "step_count", "units": "count", "data": [
			{"date": "2026-10-21 00:00:00 +0200", "qty": 4000},
			{"date": "2026-10-21 00:00:00 +0200", "qty": "many"}
		]},
		{"name": "active_energy", "units": "kcal", "data": {}},
		{"name": "", "units": "count", "data": []}
	]}}`)
	if got, ok := valueOf(t, r, "2026-10-21", metric.Steps); !ok || got != 4000 {
		t.Errorf("steps = %v, %v; want 4000", got, ok)
	}
	if !slices.Equal(r.Ignored, []string{"some_new_metric"}) {
		t.Errorf("Ignored = %v, want the new metric and no empty name", r.Ignored)
	}
	if len(r.Rejected) != 2 || r.Rejected[0].Kind != metric.Steps || !strings.Contains(r.Rejected[0].Reason, "a point does not read") ||
		r.Rejected[1].Kind != metric.ActiveKcal || r.Rejected[1].Reason != "the data is no list of points" {
		t.Errorf("Rejected = %+v", r.Rejected)
	}
}

// A unit a kind does not take refuses the metric: 25 st would pass for
// 25 kg.
func TestParseHAEUnits(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "weight_body_mass", "units": "st", "data": [{"date": "2026-10-21 07:00:00 +0200", "qty": 25}]},
		{"name": "apple_exercise_time", "units": "s", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 600}]},
		{"name": "vo2_max", "units": "l/min", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 3}]},
		{"name": "active_energy", "units": " KJ ", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 418.4}]},
		{"name": "basal_energy_burned", "units": "kilojoules", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 6276}]},
		{"name": "apple_exercise_time", "units": "h", "data": [{"date": "2026-10-22 00:00:00 +0200", "qty": 1.5}]},
		{"name": "resting_heart_rate", "units": "bpm", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 50}]},
		{"name": "heart_rate_variability", "units": "", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 70}]},
		{"name": "cardio_fitness", "units": "mL/min·kg", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 41}]}
	]}}`)
	for _, tc := range []struct {
		date string
		kind metric.Kind
		want float64
	}{
		{"2026-10-21", metric.ActiveKcal, 100},
		{"2026-10-21", metric.BasalKcal, 1500},
		{"2026-10-22", metric.ExerciseMin, 90},
		{"2026-10-21", metric.RestingHR, 50},
		{"2026-10-21", metric.HRV, 70},
		{"2026-10-21", metric.VO2Max, 41},
	} {
		if got, ok := valueOf(t, r, tc.date, tc.kind); !ok || !near(got, tc.want) {
			t.Errorf("%s %s = %v, %v; want %v", tc.date, tc.kind, got, ok, tc.want)
		}
	}
	for _, k := range []metric.Kind{metric.Weight} {
		if _, ok := valueOf(t, r, "2026-10-21", k); ok {
			t.Errorf("%s in a unit it does not take was kept", k)
		}
	}
	var reasons []string
	for _, x := range r.Rejected {
		reasons = append(reasons, string(x.Kind)+": "+x.Reason)
	}
	want := []string{
		`weight: units "st": want kg or lb or lbs`,
		`exercise_min: units "s": want h or hr or min`,
		`vo2max: units "l/min": want ml/(kg·min)`,
	}
	if !slices.Equal(reasons, want) {
		t.Errorf("Rejected = %q, want %q", reasons, want)
	}
	if got, ok := valueOf(t, r, "2026-10-21", metric.ExerciseMin); ok {
		t.Errorf("exercise in seconds = %v, kept", got)
	}
}

// Each point is checked before a day's value is picked among them: a bad
// earliest weighing or a bad longest sleep does not cost the day its good
// one.
func TestParseHAEPicksAmongGoodPoints(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "weight_body_mass", "units": "kg", "data": [
			{"date": "2026-10-21 06:00:00 +0200", "qty": 0},
			{"date": "2026-10-21 07:00:00 +0200", "qty": 80}
		]},
		{"name": "sleep_analysis", "units": "hr", "data": [
			{"date": "2026-10-21 07:00:00 +0200", "totalSleep": 7},
			{"date": "2026-10-21 08:00:00 +0200", "totalSleep": 40}
		]}
	]}}`)
	if got, _ := valueOf(t, r, "2026-10-21", metric.Weight); got != 80 {
		t.Errorf("weight = %v, want 80", got)
	}
	if got, _ := valueOf(t, r, "2026-10-21", metric.Sleep); got != 7 {
		t.Errorf("sleep = %v, want 7", got)
	}
	if len(r.Rejected) != 2 {
		t.Errorf("Rejected = %+v, want the two bad points", r.Rejected)
	}
}

// A sleep segment of an export HAE does not summarize is no night.
func TestParseHAESleepSegment(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [{"name": "sleep_analysis", "units": "hr", "data": [
		{"date": "2026-10-21 02:00:00 +0200", "startDate": "2026-10-21 00:30:00 +0200", "endDate": "2026-10-21 03:00:00 +0200", "qty": 2.5, "value": "In Bed"}
	]}]}}`)
	if _, ok := valueOf(t, r, "2026-10-21", metric.Sleep); ok {
		t.Error("a segment gave the night")
	}
	if len(r.Rejected) != 1 || !strings.Contains(r.Rejected[0].Reason, "turn on Summarize Data") || r.Rejected[0].RawDate != "2026-10-21 02:00:00 +0200" {
		t.Errorf("Rejected = %+v", r.Rejected)
	}
}

// A date that starts as a date is that date, whatever follows; its time
// counts when it reads.
func TestParseHAEDates(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "step_count", "units": "count", "data": [
			{"date": "2026-10-21T00:00:00+02:00", "qty": 100},
			{"date": "2026-10-22 7:00:00 PM +0200", "qty": 200},
			{"date": "2026-10-23 07:00 +0200", "qty": 300},
			{"date": "2026", "qty": 400}
		]},
		{"name": "weight_body_mass", "units": "kg", "data": [
			{"date": "2026-10-24T08:00:00+02:00", "qty": 82},
			{"date": "2026-10-24T07:00:00+02:00", "qty": 81}
		]}
	]}}`)
	for d, want := range map[string]float64{"2026-10-21": 100, "2026-10-22": 200, "2026-10-23": 300} {
		if got, ok := valueOf(t, r, d, metric.Steps); !ok || got != want {
			t.Errorf("steps on %s = %v, %v; want %v", d, got, ok, want)
		}
	}
	if got, _ := valueOf(t, r, "2026-10-24", metric.Weight); got != 81 {
		t.Errorf("weight = %v, want the earlier by its RFC 3339 time, 81", got)
	}
	if len(r.Rejected) != 1 || r.Rejected[0].RawDate != "2026" {
		t.Errorf("Rejected = %+v, want the date too short", r.Rejected)
	}
}
