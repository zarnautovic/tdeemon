package api_test

import (
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/ingest"
)

// The values below are made up.

const haeBody = `{"data": {"metrics": [
	{"name": "step_count", "units": "count", "data": [{"date": "2026-10-20 00:00:00 +0200", "qty": 7300}, {"date": "yesterday", "qty": 1}]},
	{"name": "weight_body_mass", "units": "kg", "data": [{"date": "2026-10-21 07:10:00 +0200", "qty": 77.7}]},
	{"name": "sleep_analysis", "units": "hr", "data": [{"date": "2026-10-21 06:30:00 +0200", "totalSleep": 26}]},
	{"name": "walking_running_distance", "units": "km", "data": []}
]}}`

// ingestToken returns a token of in's user with scopes.
func ingestToken(t *testing.T, in *instance, scopes string) map[string]string {
	t.Helper()
	tok, rec := in.createToken(t, sessionOf(t, in), `{"name":"phone","scopes":[`+scopes+`]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body.String())
	}
	return bearer(tok.Token)
}

type ingestResponse struct {
	Days []struct {
		Date    string `json:"date"`
		Changed []struct {
			Metric string   `json:"metric"`
			Value  float64  `json:"value"`
			Before *float64 `json:"before"`
		} `json:"changed"`
	} `json:"days"`
	Rejected []struct {
		Date, Metric, Reason string
		RawDate              string `json:"raw_date"`
		Value                float64
	} `json:"rejected"`
	Ignored []string `json:"ignored"`
}

// checkRejected checks what TestIngestHAE's body leaves out: a point
// whose date does not read, with that date, and a sleep out of bounds.
func checkRejected(t *testing.T, got ingestResponse) {
	t.Helper()
	if len(got.Rejected) != 2 {
		t.Fatalf("rejected = %+v, want two", got.Rejected)
	}
	if r := got.Rejected[0]; r.Metric != "steps" || r.RawDate != "yesterday" || r.Date != "" {
		t.Errorf("the bad date = %+v", r)
	}
	if r := got.Rejected[1]; r.Metric != "sleep_h" || r.Date != "2026-10-21" || r.RawDate != "" || !strings.Contains(r.Reason, "0 to 24") {
		t.Errorf("the bad sleep = %+v", r)
	}
}

func TestIngestHAE(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	tok := ingestToken(t, in, `"ingest"`)
	rec := in.do(t, http.MethodPost, "/api/v1/ingest/hae", haeBody, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body.String())
	}
	got := decodeJSON[ingestResponse](t, rec.Body.Bytes())
	if len(got.Days) != 2 || got.Days[0].Date != "2026-10-20" || got.Days[0].Changed[0].Metric != "steps" || got.Days[0].Changed[0].Value != 7300 ||
		got.Days[1].Changed[0].Metric != "weight" {
		t.Errorf("days = %+v", got.Days)
	}
	checkRejected(t, got)
	if strings.Join(got.Ignored, ",") != "walking_running_distance" {
		t.Errorf("ignored = %v", got.Ignored)
	}
}

// What an import stores is the import's, and a sync sent again changes
// nothing.
func TestIngestHAEStores(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	tok := ingestToken(t, in, `"ingest"`)
	in.do(t, http.MethodPost, "/api/v1/ingest/hae", haeBody, nil, tok)
	ms, err := in.db.Metrics(t.Context(), in.user.ID, day.Date{Year: 2026, Month: 10, Day: 20}, day.Date{Year: 2026, Month: 10, Day: 21})
	if err != nil || len(ms) != 2 {
		t.Fatalf("Metrics = %+v, %v", ms, err)
	}
	es, err := in.db.Events(t.Context(), in.user.ID, 10)
	if err != nil || len(es) == 0 || es[0].Actor != (event.Actor{Kind: event.Import, Client: "hae"}) {
		t.Errorf("events = %+v, %v; want the import's", es, err)
	}

	// The same sync again changes nothing, and says so.
	rec := in.do(t, http.MethodPost, "/api/v1/ingest/hae", haeBody, nil, tok)
	got := decodeJSON[ingestResponse](t, rec.Body.Bytes())
	if rec.Code != http.StatusOK || len(got.Days) != 2 || len(got.Days[0].Changed) != 0 || len(got.Days[1].Changed) != 0 {
		t.Errorf("the same sync again = %d %s", rec.Code, rec.Body.String())
	}
}

// An import does not change a value the user gave, and says what it
// changed of an import before.
func TestIngestMetrics(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	d := day.Date{Year: 2026, Month: 10, Day: 21}
	if _, err := in.db.SetMetric(t.Context(), in.user.ID, event.Actor{Kind: event.User, Client: "web"}, d,
		metric.Value{Kind: metric.Weight, Value: 80, Source: metric.Manual}, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	tok := ingestToken(t, in, `"ingest"`)
	post := func(body string) (int, ingestResponse, string) {
		t.Helper()
		rec := in.do(t, http.MethodPost, "/api/v1/ingest/metrics", body, nil, tok)
		var got ingestResponse
		if rec.Code == http.StatusOK {
			got = decodeJSON[ingestResponse](t, rec.Body.Bytes())
		}
		return rec.Code, got, rec.Body.String()
	}
	code, got, body := post(`{"date": "2026-10-21", "metrics": {"weight": 79, "steps": 4000, "mood": 3}}`)
	if code != http.StatusOK || len(got.Days) != 1 || len(got.Days[0].Changed) != 1 || got.Days[0].Changed[0].Metric != "steps" ||
		got.Days[0].Changed[0].Before != nil || strings.Join(got.Ignored, ",") != "mood" {
		t.Errorf("POST = %d %s", code, body)
	}
	code, got, body = post(`{"date": "2026-10-21", "metrics": {"steps": 6000}}`)
	if code != http.StatusOK || got.Days[0].Changed[0].Before == nil || *got.Days[0].Changed[0].Before != 4000 || got.Days[0].Changed[0].Value != 6000 {
		t.Errorf("a larger total = %d %s", code, body)
	}
	v, _ := in.db.Metrics(t.Context(), in.user.ID, d, d)
	if !slices.Contains(v[d], metric.Value{Kind: metric.Weight, Value: 80, Source: metric.Manual}) {
		t.Errorf("the day = %+v, want the manual weight kept", v[d])
	}
}

func TestIngestMetricsRefuses(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	tok := ingestToken(t, in, `"ingest"`)
	for _, bad := range []string{`{"date": "21.10.2026.", "metrics": {}}`, `{"metrics": {"steps": 1}}`, `{"date": "2026-10-21", "metrics": {"steps": "many"}}`} {
		if rec := in.do(t, http.MethodPost, "/api/v1/ingest/metrics", bad, nil, tok); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("POST %s = %d %s, want 422", bad, rec.Code, rec.Body.String())
		}
	}
}

func TestIngestRefuses(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	tok := ingestToken(t, in, `"ingest"`)
	const hae, metrics = "/api/v1/ingest/hae", "/api/v1/ingest/metrics"
	simple := `{"date": "2026-10-21", "metrics": {"steps": 1}}`
	session := []*http.Cookie{sessionOf(t, in)}
	for _, tc := range []struct {
		name, path, body string
		header           map[string]string
		cookies          []*http.Cookie
		code             int
	}{
		{"not HAE's shape", hae, `{"data": {"workouts": []}}`, tok, nil, http.StatusUnprocessableEntity},
		{"not JSON", hae, `steps=4000`, tok, nil, http.StatusUnprocessableEntity},
		{"a token without the ingest scope", hae, haeBody, ingestToken(t, in, `"read","tools"`), nil, http.StatusForbidden},
		{"a web session", hae, haeBody, nil, session, http.StatusForbidden},
		{"a web session, the simple form", metrics, simple, nil, session, http.StatusForbidden},
		{"no one", hae, haeBody, nil, nil, http.StatusUnauthorized},
		{"10 MB", hae, `{"data": {"metrics": [], "pad": "` + strings.Repeat("x", 10<<20) + `"}}`, tok, nil, http.StatusRequestEntityTooLarge},
		{"10 MB, the simple form", metrics, simple + strings.Repeat(" ", 10<<20), tok, nil, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if rec := in.do(t, http.MethodPost, tc.path, tc.body, tc.cookies, tc.header); rec.Code != tc.code {
				t.Errorf("POST = %d %s, want %d", rec.Code, rec.Body.String()[:min(200, rec.Body.Len())], tc.code)
			}
		})
	}
	if ms, _ := in.db.Metrics(t.Context(), in.user.ID, day.Date{Year: 2026, Month: 10, Day: 1}, day.Date{Year: 2026, Month: 10, Day: 31}); len(ms) != 0 {
		t.Errorf("a refused ingest stored %+v", ms)
	}
}

// A body just under 10 MB, a first export of months, is taken.
func TestIngestTakesNineMB(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	tok := ingestToken(t, in, `"ingest"`)
	pad := strings.Repeat(" ", 10<<20-1<<10)
	for path, body := range map[string]string{
		"/api/v1/ingest/hae":     haeBody + pad,
		"/api/v1/ingest/metrics": `{"date": "2026-10-21", "metrics": {"steps": 1}}` + pad,
	} {
		if rec := in.do(t, http.MethodPost, path, body, nil, tok); rec.Code != http.StatusOK {
			t.Errorf("POST %s = %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

// A value no answer could hold, after its conversion, is left out, and
// the answer is still JSON.
func TestIngestHAETooLarge(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	tok := ingestToken(t, in, `"ingest"`)
	rec := in.do(t, http.MethodPost, "/api/v1/ingest/hae", `{"data": {"metrics": [
		{"name": "apple_exercise_time", "units": "hr", "data": [{"date": "2026-10-21 00:00:00 +0200", "qty": 1e308}]}
	]}}`, nil, tok)
	got := decodeJSON[ingestResponse](t, rec.Body.Bytes())
	if rec.Code != http.StatusOK || len(got.Rejected) != 1 || got.Rejected[0].Metric != "exercise_min" {
		t.Errorf("POST = %d %s", rec.Code, rec.Body.String())
	}
}

// The raw body goes to the user's archive, even one that does not read.
func TestIngestArchives(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	dir := t.TempDir()
	base, _ := url.Parse("http://localhost")
	h, err := api.NewHandler(api.Config{
		Info: testInfo, DB: in.db, BaseURL: base, Logger: slog.New(slog.NewJSONHandler(in.logs, nil)), Now: in.clock.Now,
		IngestArchive: &ingest.Archive{Dir: dir, Keep: 30 * 24 * time.Hour, Now: in.clock.Now},
	})
	if err != nil {
		t.Fatal(err)
	}
	in.h = h
	tok := ingestToken(t, in, `"ingest"`)
	in.do(t, http.MethodPost, "/api/v1/ingest/hae", haeBody, nil, tok)
	in.do(t, http.MethodPost, "/api/v1/ingest/hae", `{"not": "hae"}`, nil, tok)
	in.do(t, http.MethodPost, "/api/v1/ingest/metrics", `{"date": "2026-10-21", "metrics": {"steps": 1}}`, nil, tok)
	files, err := filepath.Glob(filepath.Join(dir, in.user.ID.String(), in.clock.Now().UTC().Format("2006-01-02"), "*.json.gz"))
	if err != nil || len(files) != 3 {
		t.Fatalf("archived %v, %v; want 3 bodies", files, err)
	}
	var hae, metrics int
	for _, f := range files {
		switch {
		case strings.HasSuffix(f, "-hae.json.gz"):
			hae++
		case strings.HasSuffix(f, "-metrics.json.gz"):
			metrics++
		}
	}
	if hae != 2 || metrics != 1 {
		t.Errorf("archived %d of HAE and %d of the simple form", hae, metrics)
	}
	// An archive that cannot be written does not stop the import; the
	// log says whose body it lost.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("a file where the archive was"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rec := in.do(t, http.MethodPost, "/api/v1/ingest/metrics", `{"date": "2026-10-22", "metrics": {"steps": 2}}`, nil, tok); rec.Code != http.StatusOK {
		t.Errorf("POST with a broken archive = %d %s", rec.Code, rec.Body.String())
	}
	in.checkLogged(t, logRecord{Level: "ERROR", Msg: "archiving an ingest failed", UserID: in.user.ID.String()})
}

// HAE's body may arrive past the server's own deadline, as a large export
// does on a phone's slow upload.
func TestIngestHAESlowBody(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	tok := ingestToken(t, in, `"ingest"`)
	srv := httptest.NewUnstartedServer(in.h)
	srv.Config.ReadTimeout = 300 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)

	pr, pw := io.Pipe()
	go func() {
		half := len(haeBody) / 2
		_, _ = pw.Write([]byte(haeBody[:half]))
		time.Sleep(time.Second)
		_, _ = pw.Write([]byte(haeBody[half:]))
		_ = pw.Close()
	}()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/ingest/hae", pr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range tok {
		req.Header.Set(k, v)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("POST = %d %s", resp.StatusCode, b)
	}
}

// haePayload returns a payload of internal/ingest's testdata, of the shape
// Health Auto Export sends.
func haePayload(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "ingest", "testdata", "hae", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// changedOn returns the metrics got changed on date, sorted, and whether
// got has the day.
func changedOn(got ingestResponse, date string) ([]string, bool) {
	for _, d := range got.Days {
		if d.Date != date {
			continue
		}
		out := []string{}
		for _, c := range d.Changed {
			out = append(out, c.Metric)
		}
		slices.Sort(out)
		return out, true
	}
	return nil, false
}

// A day of syncs as a phone sends them, each through the route into the
// database, checks the merge rules (DESIGN.md §4): a value the user gave
// stays; the first weighing stays, with its body fat and lean mass, when a
// later one comes alone; a running total and the night only grow, so the
// morning's, sent late, changes neither; a reading is the latest to
// arrive; and a sync sent again changes nothing.
func TestIngestHAESyncs(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	d := day.Date{Year: 2026, Month: 10, Day: 21}
	if _, err := in.db.SetMetric(t.Context(), in.user.ID, event.Actor{Kind: event.User, Client: "web"}, d,
		metric.Value{Kind: metric.HRV, Value: 50, Source: metric.Manual}, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	tok := ingestToken(t, in, `"ingest"`)
	for i, sync := range []struct {
		name    string
		changed map[string][]string // by day
	}{
		{"morning-sync.json", map[string][]string{
			"2026-10-20": {"active_kcal", "steps"},
			"2026-10-21": {"active_kcal", "basal_kcal", "body_fat", "exercise_min", "lean_mass", "rhr", "sleep_h", "steps", "vo2max", "weight"},
		}},
		{"evening-weighing.json", map[string][]string{"2026-10-21": {}}},
		{"evening-sync.json", map[string][]string{"2026-10-21": {"active_kcal", "basal_kcal", "exercise_min", "rhr", "sleep_h", "steps"}}},
		{"evening-sync.json", map[string][]string{"2026-10-21": {}}},
		{"morning-sync.json", map[string][]string{"2026-10-20": {}, "2026-10-21": {"rhr"}}},
	} {
		rec := in.do(t, http.MethodPost, "/api/v1/ingest/hae", haePayload(t, sync.name), nil, tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("sync %d, %s = %d %s", i, sync.name, rec.Code, rec.Body.String())
		}
		got := decodeJSON[ingestResponse](t, rec.Body.Bytes())
		for date, want := range sync.changed {
			if changed, ok := changedOn(got, date); !ok || !slices.Equal(changed, want) {
				t.Errorf("sync %d, %s changed %v on %s, want %v", i, sync.name, changed, date, want)
			}
		}
	}

	ms, err := in.db.Metrics(t.Context(), in.user.ID, d.AddDays(-1), d)
	if err != nil {
		t.Fatal(err)
	}
	imported := func(k metric.Kind, v float64) metric.Value {
		return metric.Value{Kind: k, Value: v, Source: metric.Import}
	}
	want := map[day.Date][]metric.Value{
		d.AddDays(-1): {imported(metric.Steps, 8650), imported(metric.ActiveKcal, 600)},
		d: {
			imported(metric.Steps, 11380), imported(metric.ActiveKcal, 500), imported(metric.BasalKcal, 1710),
			imported(metric.ExerciseMin, 38), imported(metric.Sleep, 6.8), imported(metric.Weight, 78.4),
			imported(metric.BodyFat, 19.5), imported(metric.LeanMass, 63.1), imported(metric.RestingHR, 59),
			{Kind: metric.HRV, Value: 50, Source: metric.Manual},
			imported(metric.VO2Max, 40.2),
		},
	}
	for dd, values := range want {
		if len(ms[dd]) != len(values) {
			t.Errorf("%s = %+v, want %+v", dd, ms[dd], values)
		}
		for _, w := range values {
			if !slices.ContainsFunc(ms[dd], func(v metric.Value) bool {
				return v.Kind == w.Kind && v.Source == w.Source && math.Abs(v.Value-w.Value) < 1e-9
			}) {
				t.Errorf("%s: want %+v in %+v", dd, w, ms[dd])
			}
		}
	}
}

// A night in stages is left out, once, with what to turn on in the app;
// the payload's other values are stored.
func TestIngestHAEUnsummarizedSleep(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	tok := ingestToken(t, in, `"ingest"`)
	rec := in.do(t, http.MethodPost, "/api/v1/ingest/hae", haePayload(t, "unsummarized-sleep.json"), nil, tok)
	got := decodeJSON[ingestResponse](t, rec.Body.Bytes())
	if rec.Code != http.StatusOK || len(got.Rejected) != 1 || !strings.Contains(got.Rejected[0].Reason, "Summarize Data") ||
		got.Rejected[0].RawDate != "2026-10-22 01:10:00 +0200" {
		t.Errorf("POST = %d %s", rec.Code, rec.Body.String())
	}
	if len(got.Days) != 1 || got.Days[0].Date != "2026-10-22" || len(got.Days[0].Changed) != 1 ||
		got.Days[0].Changed[0].Metric != "steps" || got.Days[0].Changed[0].Value != 310 {
		t.Errorf("days = %+v, want 2026-10-22 with the steps added up", got.Days)
	}
}
