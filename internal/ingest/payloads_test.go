package ingest

import (
	"compress/gzip"
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

// An export HAE does not summarize gives a night in stages: they are left
// out, once, with what to turn on, and the rest of the payload is read.
func TestPayloadUnsummarizedSleep(t *testing.T) {
	t.Parallel()

	r, err := ParseHAE(readPayload(t, filepath.Join("testdata", "hae", "unsummarized-sleep.json")))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rejected) != 1 || r.Rejected[0].Kind != metric.Sleep || !strings.Contains(r.Rejected[0].Reason, "Summarize Data") {
		t.Errorf("rejected = %+v, want the stages once", r.Rejected)
	}
	if _, ok := valueOf(t, r, "2026-10-22", metric.Sleep); ok {
		t.Error("a stage was taken for the night")
	}
	if got, ok := valueOf(t, r, "2026-10-22", metric.Steps); !ok || got != 310 {
		t.Errorf("steps = %v, %v; want 310", got, ok)
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
// archive of an instance (DATA_DIR/ingest/<user>). Each must read as
// HAE's, with nothing left out for its shape; the test says what it left
// out and ignored, without the values. It is skipped without the
// variable.
func TestRealPayloads(t *testing.T) {
	t.Parallel()

	dir := os.Getenv("TDEEMON_HAE_PAYLOADS")
	if dir == "" {
		t.Skip("TDEEMON_HAE_PAYLOADS is not set")
	}
	var paths []string
	//nolint:gosec // G703: the directory is the one the test's runner names
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		name := d.Name()
		switch {
		case err != nil:
			return err
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
			r, err := ParseHAE(readPayload(t, path))
			if err != nil {
				t.Fatal(err)
			}
			checkShape(t, r)
			t.Logf("%d days, ignored %v", len(r.Days), r.Ignored)
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
