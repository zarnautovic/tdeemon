package api

import "net/http"

// contentSecurityPolicy lets a page load only from this instance: no
// third-party CDN (docs/conventions.md), no inline script, no plugins, no
// <base> tricks, and no framing by anyone (clickjacking). Images may also
// be data: and blob: URLs (assets the build inlines, a photo picked for
// upload before it goes up).
const contentSecurityPolicy = "default-src 'self'; img-src 'self' data: blob:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

// securityHeaders sets, on every response, the headers that keep a
// browser strict with the app: the content security policy, no MIME
// sniffing, no Referer to other sites, no framing (X-Frame-Options for
// browsers without frame-ancestors), its own browsing context group and,
// over HTTPS, HSTS. A handler may still change them.
func securityHeaders(https bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if https {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}
