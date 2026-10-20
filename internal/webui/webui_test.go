package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var app = fstest.MapFS{
	"index.html":                {Data: []byte("<!doctype html><div id=root></div>")},
	"assets/index-B7Vj6WSe.js":  {Data: []byte("console.log(1)")},
	"assets/index-CMUHlmXG.css": {Data: []byte("body{}")},
	"favicon.svg":               {Data: []byte("<svg/>")},
}

func get(t *testing.T, h http.Handler, method, path string, header ...string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandler(t *testing.T) {
	t.Parallel()

	h, err := Handler(app)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	const (
		index     = "<!doctype html><div id=root></div>"
		immutable = "public, max-age=31536000, immutable"
	)
	navigate := []string{"Sec-Fetch-Mode", "navigate"}
	tests := []struct {
		name         string
		method       string
		path         string
		header       []string
		wantStatus   int
		wantType     string
		wantCache    string
		wantBody     string
		wantNoBodyOf string
	}{
		{name: "root", method: http.MethodGet, path: "/", wantStatus: 200, wantType: "text/html; charset=utf-8", wantCache: "no-store", wantBody: index},
		{name: "client route", method: http.MethodGet, path: "/day/2026-10-20", wantStatus: 200, wantType: "text/html; charset=utf-8", wantCache: "no-store", wantBody: index},
		{name: "index.html itself", method: http.MethodGet, path: "/index.html", wantStatus: 200, wantCache: "no-store", wantBody: index},
		{name: "directory", method: http.MethodGet, path: "/assets", wantStatus: 200, wantCache: "no-store", wantBody: index},
		{name: "hashed script", method: http.MethodGet, path: "/assets/index-B7Vj6WSe.js", wantStatus: 200, wantType: "text/javascript; charset=utf-8", wantCache: immutable, wantBody: "console.log(1)"},
		{name: "hashed stylesheet", method: http.MethodGet, path: "/assets/index-CMUHlmXG.css", wantStatus: 200, wantType: "text/css; charset=utf-8", wantCache: immutable},
		{name: "unhashed file", method: http.MethodGet, path: "/favicon.svg", wantStatus: 200, wantType: "image/svg+xml", wantCache: "no-cache"},
		{name: "script of an old build", method: http.MethodGet, path: "/assets/index-OLD.js", wantStatus: 404, wantNoBodyOf: index},
		{name: "missing file", method: http.MethodGet, path: "/robots.txt", wantStatus: 404, wantNoBodyOf: index},
		{name: "HEAD", method: http.MethodHead, path: "/", wantStatus: 200, wantCache: "no-store"},
		// A client route may end in a dot-separated token.
		{name: "navigation to a dotted route", method: http.MethodGet, path: "/invite/aaa.bbb.ccc", header: navigate, wantStatus: 200, wantCache: "no-store", wantBody: index},
		{name: "dotted route, no Sec-Fetch-Mode", method: http.MethodGet, path: "/day/1.5", header: []string{"Accept", "text/html,application/xhtml+xml"}, wantStatus: 200, wantBody: index},
		{name: "fetch of a dotted path", method: http.MethodGet, path: "/favicon.ico", header: []string{"Sec-Fetch-Mode", "no-cors"}, wantStatus: 404, wantNoBodyOf: index},
		{name: "navigation to an old asset", method: http.MethodGet, path: "/assets/index-OLD.js", header: navigate, wantStatus: 404, wantNoBodyOf: index},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := get(t, h, tt.method, tt.path, tt.header...)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantType != "" && rec.Header().Get("Content-Type") != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", rec.Header().Get("Content-Type"), tt.wantType)
			}
			if tt.wantCache != "" && rec.Header().Get("Cache-Control") != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), tt.wantCache)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantNoBodyOf != "" && strings.Contains(rec.Body.String(), tt.wantNoBodyOf) {
				t.Errorf("body is the app's index.html, want a plain 404")
			}
		})
	}
}

func TestHandlerNeedsIndex(t *testing.T) {
	t.Parallel()

	if _, err := Handler(fstest.MapFS{"assets/x.js": {Data: []byte("1")}}); err == nil {
		t.Fatal("Handler accepted a build without index.html")
	}
}

func TestNoApp(t *testing.T) {
	t.Parallel()

	h := http.HandlerFunc(noApp)
	if rec := get(t, h, http.MethodGet, "/"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "no web app") {
		t.Errorf("GET / = %d %q, want the notice", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, http.MethodGet, "/day/today"); rec.Code != 404 {
		t.Errorf("GET /day/today = %d, want 404", rec.Code)
	}
}
