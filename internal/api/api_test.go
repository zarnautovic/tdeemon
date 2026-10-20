package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/api"
)

var testInfo = api.ServerInfo{Name: "tdeemon", Version: "v9.9.9"}

func serve(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	rec := httptest.NewRecorder()
	api.NewHandler(testInfo).ServeHTTP(rec, req)
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
