package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// TestAPITyposDoNotReachACatchAll adds the kind of catch-all route the web
// app will bring (index.html for every other path): a mistyped API path
// must still get a problem, not the page.
func TestAPITyposDoNotReachACatchAll(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html>")
	})
	register(newAPI(mux), ServerInfo{}, newSessions(Config{BaseURL: &url.URL{Scheme: "http", Host: "localhost"}, Now: time.Now}), nil)
	h := withAPIErrors(mux)

	for path, want := range map[string]string{
		"/api/v1/typo": "application/problem+json",
		"/api/v1/info": "application/json",
		"/day/today":   "text/html; charset=utf-8",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
			if got := rec.Header().Get("Content-Type"); got != want {
				t.Errorf("Content-Type = %q, want %q", got, want)
			}
		})
	}
}
