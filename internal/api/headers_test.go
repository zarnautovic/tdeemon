package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// Every response carries the security headers: the API's, its errors,
// what is outside the API, and a refused cross-origin request.
func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	for _, tc := range []struct {
		name, method, path string
		header             map[string]string
	}{
		{"an API operation", http.MethodGet, "/api/v1/info", nil},
		{"an API error", http.MethodGet, "/api/v1/nope", nil},
		{"outside the API", http.MethodGet, "/healthz", nil},
		{"a refused cross-origin request", http.MethodPost, "/api/v1/auth/logout", map[string]string{"Sec-Fetch-Site": "cross-site"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := in.do(t, tc.method, tc.path, "", nil, tc.header).Header()
			for name, want := range map[string]string{
				"X-Content-Type-Options":     "nosniff",
				"Referrer-Policy":            "no-referrer",
				"X-Frame-Options":            "DENY",
				"Cross-Origin-Opener-Policy": "same-origin",
			} {
				if got := h.Get(name); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
			csp := h.Get("Content-Security-Policy")
			for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'", "base-uri 'none'"} {
				if !strings.Contains(csp, want) {
					t.Errorf("Content-Security-Policy %q lacks %q", csp, want)
				}
			}
			if hsts := h.Get("Strict-Transport-Security"); hsts != "" {
				t.Errorf("HSTS over plain HTTP: %q", hsts)
			}
		})
	}

	https := newInstance(t, "https://tdeemon.example", "")
	if hsts := https.do(t, http.MethodGet, "/healthz", "", nil, nil).Header().Get("Strict-Transport-Security"); hsts != "max-age=31536000" {
		t.Errorf("HSTS over HTTPS = %q, want max-age=31536000", hsts)
	}
}
