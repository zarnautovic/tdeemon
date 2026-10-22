package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

var testInfo = api.ServerInfo{Name: "tdeemon", Version: "v9.9.9"}

// newHandler returns the root handler over a fresh database, for an
// instance at http://localhost.
func newHandler(tb testing.TB, app http.Handler) http.Handler {
	tb.Helper()

	h, err := api.NewHandler(api.Config{
		Info:    testInfo,
		App:     app,
		DB:      storetest.Open(tb),
		BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
		Logger:  slog.New(slog.DiscardHandler),
	})
	if err != nil {
		tb.Fatalf("NewHandler: %v", err)
	}
	return h
}

func serve(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	h := newHandler(t, nil)
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	checkAnswer(t, req, rec)
	return rec
}

func TestRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "healthz", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK, wantBody: "ok\n"},
		{name: "healthz rejects other methods", method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed},
		{name: "unknown path", method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound},
		{name: "info", method: http.MethodGet, path: "/api/v1/info", wantStatus: http.StatusOK},
		{name: "openapi document", method: http.MethodGet, path: "/api/v1/openapi.json", wantStatus: http.StatusOK},
		// Both would serve huma's extras that are switched off on purpose.
		{name: "no docs page", method: http.MethodGet, path: "/docs", wantStatus: http.StatusNotFound},
		{name: "no schemas route", method: http.MethodGet, path: "/schemas/ServerInfo.json", wantStatus: http.StatusNotFound},
		// One document, the committed one: no unchecked variants.
		{name: "no YAML document", method: http.MethodGet, path: "/api/v1/openapi.yaml", wantStatus: http.StatusNotFound},
		{name: "no OpenAPI 3.0 document", method: http.MethodGet, path: "/api/v1/openapi-3.0.json", wantStatus: http.StatusNotFound},
		{name: "unknown query parameter", method: http.MethodGet, path: "/api/v1/info?verbose=1", wantStatus: http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, tt.method, tt.path)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestUnhandledAPIRequestsGetProblemDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  string
	}{
		{name: "unknown operation", method: http.MethodGet, path: "/api/v1/nope", wantStatus: http.StatusNotFound},
		{name: "api root", method: http.MethodGet, path: "/api", wantStatus: http.StatusNotFound},
		{name: "wrong method", method: http.MethodPost, path: "/api/v1/info", wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, HEAD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, tt.method, tt.path)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", got)
			}
			if got := rec.Header().Get("Allow"); got != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
			}
			var problem struct {
				Title  string `json:"title"`
				Status int    `json:"status"`
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode %q: %v", rec.Body.String(), err)
			}
			if problem.Status != tt.wantStatus || problem.Title != http.StatusText(tt.wantStatus) ||
				!strings.Contains(problem.Detail, tt.path) {
				t.Errorf("problem = %+v", problem)
			}
		})
	}
}

func TestNonAPIErrorsStayPlain(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/nope", "/apiary"} {
		rec := serve(t, http.MethodGet, path)
		if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") == "application/problem+json" {
			t.Errorf("GET %s = %d %q, want the mux's plain 404", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

// TestAppGetsEveryOtherGET mounts a stand-in web app: it answers paths the
// API and /healthz do not own, and never an /api/ path.
func TestAppGetsEveryOtherGET(t *testing.T) {
	t.Parallel()

	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	})
	h := newHandler(t, app)
	for path, want := range map[string]string{
		"/":             "text/html; charset=utf-8",
		"/day/today":    "text/html; charset=utf-8",
		"/healthz":      "text/plain; charset=utf-8",
		"/api/v1/info":  "application/json",
		"/api/v1/typo":  "application/problem+json",
		"/api/whatever": "application/problem+json",
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		checkAnswer(t, req, rec)
		if got := rec.Header().Get("Content-Type"); got != want {
			t.Errorf("GET %s: Content-Type = %q, want %q", path, got, want)
		}
	}
}

func TestHealthzIsNotCached(t *testing.T) {
	t.Parallel()

	rec := serve(t, http.MethodGet, "/healthz")
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestInfo(t *testing.T) {
	t.Parallel()

	rec := serve(t, http.MethodGet, "/api/v1/info")
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := rec.Header().Get("Link"); got != "" {
		t.Errorf("Link header %q, want none", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	want := map[string]any{"name": "tdeemon", "version": "v9.9.9"}
	if len(body) != len(want) || body["name"] != want["name"] || body["version"] != want["version"] {
		t.Errorf("body = %v, want exactly %v (no $schema field)", body, want)
	}
}

func TestErrorsAreProblemDetails(t *testing.T) {
	t.Parallel()

	rec := serve(t, http.MethodGet, "/api/v1/info?verbose=1")
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json (RFC 9457)", got)
	}
	var problem struct {
		Status int `json:"status"`
		Errors []struct {
			Location string `json:"location"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if problem.Status != http.StatusUnprocessableEntity || len(problem.Errors) != 1 ||
		!strings.Contains(problem.Errors[0].Location, "verbose") {
		t.Errorf("problem = %+v, want status 422 and the error located at the verbose parameter", problem)
	}
}
