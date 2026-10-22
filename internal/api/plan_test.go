package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// sessionOf returns a session of in's user, straight from the store.
func sessionOf(t *testing.T, in *instance) *http.Cookie {
	t.Helper()
	token, id := auth.NewSessionToken()
	now := in.clock.Now()
	s := store.Session{ID: id, UserID: in.user.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(auth.SessionLifetime)}
	if err := in.db.CreateSession(t.Context(), s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sessionRequestCookie(token)
}

type profileResponse struct {
	Sex       *string  `json:"sex"`
	BirthDate *string  `json:"birth_date"`
	HeightCm  *float64 `json:"height_cm"`
	Missing   []string `json:"missing"`
}

func decodeJSON[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return v
}

func TestProfileAPI(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	rec := in.do(t, http.MethodGet, "/api/v1/profile", "", session, nil)
	if got := decodeJSON[profileResponse](t, rec.Body.Bytes()); rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" ||
		got.Sex != nil || strings.Join(got.Missing, ",") != "sex,birth_date,height" {
		t.Fatalf("GET of no profile = %d %s", rec.Code, rec.Body.String())
	}
	rec = in.do(t, http.MethodPut, "/api/v1/profile", `{"sex":"female","birth_date":"1990-06-15","height_cm":168.5}`, session, nil)
	want := `{"sex":"female","birth_date":"1990-06-15","height_cm":168.5,"missing":[]}`
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("PUT = %d %s, want %s", rec.Code, rec.Body.String(), want)
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/profile", "", session, nil); strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("GET after the PUT = %s", rec.Body.String())
	}
	p, err := in.db.Profile(t.Context(), in.user.ID)
	if err != nil || p.Sex != profile.Female || p.HeightCm != 168.5 {
		t.Errorf("the stored profile = %+v, %v", p, err)
	}
	es, err := in.db.Events(t.Context(), in.user.ID, 1)
	if err != nil || len(es) != 1 || es[0].Action != event.ProfileSet || es[0].Actor.Client != "web" {
		t.Errorf("the event = %+v, %v", es, err)
	}
}

// A field left out of a PUT is cleared.
func TestProfileAPIClears(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	in.do(t, http.MethodPut, "/api/v1/profile", `{"sex":"female","birth_date":"1990-06-15","height_cm":168.5}`, session, nil)
	rec := in.do(t, http.MethodPut, "/api/v1/profile", `{"sex":"female"}`, session, nil)
	if got := decodeJSON[profileResponse](t, rec.Body.Bytes()); rec.Code != http.StatusOK || strings.Join(got.Missing, ",") != "birth_date,height" {
		t.Errorf("PUT of the sex alone = %d %s", rec.Code, rec.Body.String())
	}
	if p, _ := in.db.Profile(t.Context(), in.user.ID); p != (profile.Profile{Sex: profile.Female}) {
		t.Errorf("the stored profile = %+v", p)
	}
}

func TestProfileAPIRefuses(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	for _, tc := range []struct{ name, body, want string }{
		{"a sex of no equation", `{"sex":"other"}`, "sex"},
		{"a height in metres", `{"height_cm":1.8}`, "height_cm"},
		{"a date that is none", `{"birth_date":"1990-02-30"}`, "birth"},
		// The user's day in Zagreb starts at 04:00: at 08:00 UTC it is the
		// 21st, and a birth on the 22nd is after it.
		{"a birth after today", `{"birth_date":"2026-10-22"}`, "after today"},
		{"older than anyone", `{"birth_date":"1880-01-01"}`, "at most 130"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := in.do(t, http.MethodPut, "/api/v1/profile", tc.body, session, nil)
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("PUT %s = %d %s, want 422 about %s", tc.body, rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
	if n, _ := in.db.Events(t.Context(), in.user.ID, 10); len(n) != 0 {
		t.Errorf("a refused profile recorded %d events", len(n))
	}
}

// The profile and the limits change only from a web session: no API
// token reads or writes them, whatever its scopes.
func TestPlanSettingsNeedAWebSession(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := sessionOf(t, in)
	tok, rec := in.createToken(t, session, `{"name":"phone","scopes":["ingest","read","tools"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body.String())
	}
	for _, r := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/profile", ""},
		{http.MethodPut, "/api/v1/profile", `{"sex":"male"}`},
		{http.MethodGet, "/api/v1/limits", ""},
		{http.MethodPut, "/api/v1/limits", `{"limits":{"cut_rate":-1},"confirm":true}`},
	} {
		if rec := in.do(t, r.method, r.path, r.body, nil, bearer(tok.Token)); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with a token = %d %s, want 403", r.method, r.path, rec.Code, rec.Body.String())
		}
		if rec := in.do(t, r.method, r.path, r.body, nil, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with no one = %d, want 401", r.method, r.path, rec.Code)
		}
	}
	if p, _ := in.db.Profile(t.Context(), in.user.ID); p != (profile.Profile{}) {
		t.Errorf("the profile = %+v, want none", p)
	}
	if l, _ := in.db.UserLimits(t.Context(), in.user.ID); l != limits.UserDefaults() {
		t.Errorf("the limits = %+v, want the defaults", l)
	}
}

type limitsResponse struct {
	Limits, Instance, Effective, Defaults map[string]float64
	LooserBelow                           []string `json:"looser_below"`
}

func TestLimitsAPI(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	rec := in.do(t, http.MethodGet, "/api/v1/limits", "", session, nil)
	got := decodeJSON[limitsResponse](t, rec.Body.Bytes())
	if rec.Code != http.StatusOK || got.Limits["cut_rate"] != -0.7 || got.Instance["cut_rate"] != -1 || got.Defaults["kcal_floor_male"] != 1500 ||
		got.Effective["fat_floor_pct"] != 20 || len(got.Limits) != 19 || len(got.Instance) != 18 {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
	}
	if below := strings.Join(got.LooserBelow, ","); !strings.Contains(below, "cut_rate") || strings.Contains(below, "bulk_rate") {
		t.Errorf("looser_below = %v", got.LooserBelow)
	}
}

func TestSetLimitsAPI(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	// The steps run in order, each on what the one before stored.
	for _, st := range []struct {
		name, body string
		code       int
		want       string
	}{
		{"a tightening needs nothing", `{"limits":{"cut_rate":-0.5,"protein_floor_grams":150}}`, http.StatusOK, `"cut_rate":-0.5`},
		{"a loosening needs confirm", `{"limits":{"cut_rate":-0.9}}`, http.StatusConflict, "loosening cut_rate needs confirm"},
		{"and is not stored without", `{"limits":{}}`, http.StatusOK, `"limits":{"bulk_rate":0.25,"cut_rate":-0.5,`},
		{"a confirmed loosening", `{"limits":{"cut_rate":-0.9},"confirm":true}`, http.StatusOK, `"cut_rate":-0.9`},
		{"past the instance's", `{"limits":{"cut_rate":-1.2},"confirm":true}`, http.StatusUnprocessableEntity, "past the instance's limit: cut_rate"},
		{"unknown names", `{"limits":{"cut_rate_bmi":-1,"x":1}}`, http.StatusUnprocessableEntity, "no such limit: cut_rate_bmi, x"},
		{"a limit of the wrong shape", `{"limits":{"bulk_rate":-1}}`, http.StatusUnprocessableEntity, "bulk_rate -1"},
	} {
		rec := in.do(t, http.MethodPut, "/api/v1/limits", st.body, session, nil)
		if rec.Code != st.code || !strings.Contains(rec.Body.String(), st.want) {
			t.Errorf("%s: PUT %s = %d %s, want %d with %s", st.name, st.body, rec.Code, rec.Body.String(), st.code, st.want)
		}
	}
	if l, _ := in.db.UserLimits(t.Context(), in.user.ID); l.CutRate != -0.9 || l.ProteinFloorGrams != 150 {
		t.Errorf("the stored limits = %+v", l)
	}
	// The two changes; a PUT that changes nothing records nothing.
	if es, err := in.db.Events(t.Context(), in.user.ID, 10); err != nil || len(es) != 2 || es[0].Action != event.LimitsSet {
		t.Errorf("events = %+v, %v; want the two changes", es, err)
	}
}

// A limit the admin tightened since is brought to the instance's when the
// user changes another, rather than refusing every change.
func TestLimitsAPIAfterTheInstanceTightened(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	l := limits.UserDefaults()
	l.KcalFloorFemale = 1200
	if err := in.db.SetUserLimits(t.Context(), in.user.ID, event.Actor{Kind: event.User, Client: "web"}, l, limits.Instance(), in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	tight := limits.Instance()
	tight.KcalFloorFemale = 1300
	base, _ := url.Parse("http://localhost")
	h, err := api.NewHandler(api.Config{Info: testInfo, DB: in.db, BaseURL: base, Logger: slog.New(slog.DiscardHandler), Now: in.clock.Now, Limits: tight})
	if err != nil {
		t.Fatal(err)
	}
	in.h = h
	session := []*http.Cookie{sessionOf(t, in)}
	rec := in.do(t, http.MethodGet, "/api/v1/limits", "", session, nil)
	if got := decodeJSON[limitsResponse](t, rec.Body.Bytes()); got.Effective["kcal_floor_female"] != 1300 || got.Instance["kcal_floor_female"] != 1300 {
		t.Errorf("GET = %s", rec.Body.String())
	}
	rec = in.do(t, http.MethodPut, "/api/v1/limits", `{"limits":{"drift_pct":10}}`, session, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT of another limit = %d %s", rec.Code, rec.Body.String())
	}
	if l, _ := in.db.UserLimits(t.Context(), in.user.ID); l.KcalFloorFemale != 1300 || l.DriftPct != 10 {
		t.Errorf("the stored limits = %+v", l)
	}
	rec = in.do(t, http.MethodPut, "/api/v1/limits", `{"limits":{"kcal_floor_female":1250},"confirm":true}`, session, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a floor under the instance's = %d %s", rec.Code, rec.Body.String())
	}
}
