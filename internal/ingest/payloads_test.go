package ingest

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// The payloads in testdata/hae have the shape of what Health Auto Export
// sends with Summarize Data on and the time grouped by day; their values
// are made up.

// readPayload returns the file at path, gunzipped when it ends in .gz, as
// the archive keeps a body.
func readPayload(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			t.Fatal(err)
		}
		r = zr
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// shapeProblems returns what r leaves out because its shape did not
// read: a unit, a point, a date or a sleep stage. A value out of its
// bounds has its date and is a glitch of the source's, not of the shape.
func shapeProblems(r Result) []Rejected {
	var out []Rejected
	for _, x := range r.Rejected {
		if x.Date.IsZero() {
			out = append(out, x)
		}
	}
	return out
}

func checkShape(t *testing.T, r Result) {
	t.Helper()
	for _, x := range shapeProblems(r) {
		t.Errorf("%s left out (%q): %s", x.Kind, x.RawDate, x.Reason)
	}
}

// unread returns the names of the metrics tdeemon reads that body has
// points of, but that gave r no value: points without a field the parser
// knows, or every value out of its bounds, as a unit or a scale off by a
// factor gives. Either is the shape, not a glitch.
func unread(t *testing.T, body []byte, r Result) []string {
	t.Helper()
	var p struct {
		Data struct {
			Metrics []struct {
				Name string            `json:"name"`
				Data []json.RawMessage `json:"data"`
			} `json:"metrics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	read := map[metric.Kind]bool{}
	for _, d := range r.Days {
		for _, v := range d.Values {
			read[v.Kind] = true
		}
	}
	var out []string
	for _, m := range p.Data.Metrics {
		if k, ok := haeKinds[m.Name]; ok && len(m.Data) > 0 && !read[k] {
			out = append(out, m.Name)
		}
	}
	return out
}

// Each summarized payload reads whole: nothing left out for its shape, and
// a value from every metric tdeemon reads.
func TestPayloadsRead(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"morning-sync.json", "evening-sync.json", "evening-weighing.json"} {
		body := readPayload(t, filepath.Join("testdata", "hae", name))
		r, err := ParseHAE(body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		checkShape(t, r)
		if u := unread(t, body, r); len(u) != 0 {
			t.Errorf("%s: unread %v", name, u)
		}
	}
}

// A morning's sync: yesterday whole and today so far, a night, a
// weighing on a scale that gives body fat and lean mass, and metrics
// tdeemon does not read.
func TestPayloadMorningSync(t *testing.T) {
	t.Parallel()

	r, err := ParseHAE(readPayload(t, filepath.Join("testdata", "hae", "morning-sync.json")))
	if err != nil {
		t.Fatal(err)
	}
	checkShape(t, r)
	for _, c := range []struct {
		date string
		kind metric.Kind
		want float64
	}{
		{"2026-10-20", metric.Steps, 8650},
		{"2026-10-20", metric.ActiveKcal, 600}, // 2510.4 kJ
		{"2026-10-21", metric.Steps, 1920},
		{"2026-10-21", metric.ActiveKcal, 100},
		{"2026-10-21", metric.BasalKcal, 640},
		{"2026-10-21", metric.ExerciseMin, 4},
		{"2026-10-21", metric.Sleep, 6.8}, // the total, not the time in bed
		{"2026-10-21", metric.Weight, 78.4},
		{"2026-10-21", metric.BodyFat, 19.5},
		{"2026-10-21", metric.LeanMass, 63.1},
		{"2026-10-21", metric.RestingHR, 59},
		{"2026-10-21", metric.HRV, 44.5},
		{"2026-10-21", metric.VO2Max, 40.2},
	} {
		if got, ok := valueOf(t, r, c.date, c.kind); !ok || !near(got, c.want) {
			t.Errorf("%s %s = %v, %v; want %v", c.date, c.kind, got, ok, c.want)
		}
	}
	if !slices.Equal(r.Ignored, []string{"heart_rate", "walking_running_distance"}) {
		t.Errorf("ignored = %v", r.Ignored)
	}
}

// The evening's sync sends the whole day again, with a second weighing:
// the morning's stays, with its body fat and lean mass.
func TestPayloadEveningSync(t *testing.T) {
	t.Parallel()

	r, err := ParseHAE(readPayload(t, filepath.Join("testdata", "hae", "evening-sync.json")))
	if err != nil {
		t.Fatal(err)
	}
	checkShape(t, r)
	if len(r.Days) != 1 {
		t.Fatalf("days = %+v, want 2026-10-21 alone", r.Days)
	}
	for k, want := range map[metric.Kind]float64{
		metric.Steps: 11380, metric.ActiveKcal: 500, metric.Weight: 78.4, metric.BodyFat: 19.5, metric.LeanMass: 63.1,
	} {
		if got, ok := valueOf(t, r, "2026-10-21", k); !ok || !near(got, want) {
			t.Errorf("%s = %v, %v; want %v", k, got, ok, want)
		}
	}
}

// An export HAE does not summarize gives a night in stages, and steps in
// parts: the stages are left out, once, with what to turn on, and the
// parts add up.
func TestPayloadUnsummarizedSleep(t *testing.T) {
	t.Parallel()

	r, err := ParseHAE(readPayload(t, filepath.Join("testdata", "hae", "unsummarized-sleep.json")))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rejected) != 1 || r.Rejected[0].Kind != metric.Sleep || !strings.Contains(r.Rejected[0].Reason, "Summarize Data") ||
		r.Rejected[0].RawDate != "2026-10-22 01:10:00 +0200" {
		t.Errorf("rejected = %+v, want the stages once, at the first one's end", r.Rejected)
	}
	if _, ok := valueOf(t, r, "2026-10-22", metric.Sleep); ok {
		t.Error("a stage was taken for the night")
	}
	if got, ok := valueOf(t, r, "2026-10-22", metric.Steps); !ok || got != 310 {
		t.Errorf("steps = %v, %v; want the points added up, 310", got, ok)
	}
}

// A metric whose points give no value, or only values out of bounds, is
// unread; one that gives a value, or that tdeemon skips, is not.
func TestUnread(t *testing.T) {
	t.Parallel()

	body := []byte(`{"data": {"metrics": [
		{"name": "step_count", "units": "count", "data": [{"date": "2026-10-21 00:00:00 +0200", "Avg": 5}]},
		{"name": "body_fat_percentage", "units": "%", "data": [{"date": "2026-10-21 07:00:00 +0200", "qty": 0.195}]},
		{"name": "weight_body_mass", "units": "kg", "data": [
			{"date": "2026-10-21 07:00:00 +0200", "qty": 0.5}, {"date": "2026-10-21 08:00:00 +0200", "qty": 80}
		]},
		{"name": "lean_body_mass", "units": "kg", "data": []},
		{"name": "heart_rate", "units": "count/min", "data": [{"date": "2026-10-21 00:00:00 +0200", "Avg": 60}]}
	]}}`)
	r, err := ParseHAE(body)
	if err != nil {
		t.Fatal(err)
	}
	if got := unread(t, body, r); !slices.Equal(got, []string{"step_count", "body_fat_percentage"}) {
		t.Errorf("unread = %v", got)
	}
}

// A value out of bounds is no problem of the shape; a unit, a point and a
// date that do not read are.
func TestShapeProblems(t *testing.T) {
	t.Parallel()

	r := parseHAE(t, `{"data": {"metrics": [
		{"name": "heart_rate_variability", "units": "ms", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 0}]},
		{"name": "weight_body_mass", "units": "st", "data": []},
		{"name": "step_count", "units": "count", "data": [{"date": "today", "qty": 1}, {"date": "2026-10-21", "qty": "many"}]}
	]}}`)
	var kinds []metric.Kind
	for _, x := range shapeProblems(r) {
		kinds = append(kinds, x.Kind)
	}
	if len(r.Rejected) != 4 || !slices.Equal(kinds, []metric.Kind{metric.Weight, metric.Steps, metric.Steps}) {
		t.Errorf("rejected %+v; problems of the shape: %v", r.Rejected, kinds)
	}
}

// TestRealPayloads reads the payloads in the directory
// TDEEMON_HAE_PAYLOADS names: real ones, kept outside git, such as the
// archive of an instance (TDEEMON_DATA_DIR/ingest/<user>). Each must read
// as HAE's, with nothing left out for its shape and a value from every
// metric tdeemon reads that it has points of; with -v the test says what
// each day kept, what it left out and what it ignored, without the
// values. It is skipped without the variable.
func TestRealPayloads(t *testing.T) {
	t.Parallel()

	dir := os.Getenv("TDEEMON_HAE_PAYLOADS")
	if dir == "" {
		t.Skip("TDEEMON_HAE_PAYLOADS is not set")
	}
	var paths []string
	//nolint:gosec // G703: the directory is the one the test's runner names
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		switch {
		case d.IsDir():
			return nil
		// The simple form, in the archive beside HAE's bodies.
		case strings.HasSuffix(name, "-metrics.json.gz"):
		case strings.HasSuffix(name, ".json"), strings.HasSuffix(name, ".json.gz"):
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no .json or .json.gz in %s", dir)
	}
	for _, path := range paths {
		rel, _ := filepath.Rel(dir, path)
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			body := readPayload(t, path)
			r, err := ParseHAE(body)
			if err != nil {
				t.Fatal(err)
			}
			checkShape(t, r)
			for _, name := range unread(t, body, r) {
				t.Errorf("%s: no value read; its points have no field tdeemon reads, or every value is out of its bounds", name)
			}
			for _, d := range r.Days {
				kinds := make([]metric.Kind, 0, len(d.Values))
				for _, v := range d.Values {
					kinds = append(kinds, v.Kind)
				}
				t.Logf("%s: %v", d.Date, kinds)
			}
			t.Logf("ignored %v", r.Ignored)
			// A value out of bounds has its value in the reason, which stays
			// out of the log.
			for _, x := range r.Rejected {
				if !x.Date.IsZero() {
					t.Logf("left out %s on %s: out of its bounds", x.Kind, x.Date)
				}
			}
		})
	}
}
