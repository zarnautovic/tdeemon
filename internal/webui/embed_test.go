//go:build webui

package webui

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var (
	// tags finds the script and link elements of a built index.html.
	tags = regexp.MustCompile(`<(?:script|link)\b[^>]*>`)
	// attr reads one attribute of such an element.
	attr = func(tag, name string) string {
		m := regexp.MustCompile(`\b` + name + `="([^"]*)"`).FindStringSubmatch(tag)
		if m == nil {
			return ""
		}
		return m[1]
	}
)

// TestEmbeddedApp runs only with the webui tag, after the web app has been
// built: the real build must be in the binary and served, every file its
// index.html references must be there, and the scripts and stylesheets,
// whose names carry a hash, must get the long cache. That catches a Vite
// assetsDir that no longer matches this package's. Other references
// (an icon from web/public) are revalidated by design.
func TestEmbeddedApp(t *testing.T) {
	t.Parallel()

	h, embedded, err := New()
	if err != nil || !embedded {
		t.Fatalf("New() embedded = %v, err = %v; want the built app", embedded, err)
	}
	rec := get(t, h, http.MethodGet, "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `<div id="root">`) {
		t.Fatalf("GET / = %d, body %q; want the app's index.html", rec.Code, rec.Body.String())
	}

	hashed := 0
	for _, tag := range tags.FindAllString(rec.Body.String(), -1) {
		ref := attr(tag, "src")
		if ref == "" {
			ref = attr(tag, "href")
		}
		if !strings.HasPrefix(ref, "/") {
			continue
		}
		rel := attr(tag, "rel")
		isHashed := strings.HasPrefix(tag, "<script") || rel == "stylesheet" || rel == "modulepreload"

		asset := get(t, h, http.MethodGet, ref)
		if asset.Code != 200 {
			t.Errorf("GET %s = %d, want 200", ref, asset.Code)
		}
		if isHashed {
			hashed++
			if got := asset.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
				t.Errorf("GET %s: Cache-Control %q, want the immutable cache", ref, got)
			}
		}
	}
	if hashed == 0 {
		t.Fatal("index.html references no script or stylesheet")
	}
}
