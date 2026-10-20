//go:build webui

package webui

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// references finds the files a built index.html loads.
var references = regexp.MustCompile(`(?:src|href)="(/[^"]+)"`)

// TestEmbeddedApp runs only with the webui tag, after the web app has been
// built: the real build must be in the binary and served, and every file
// its index.html loads must get the long cache that hashed names allow.
// It catches a Vite assetsDir that no longer matches this package's.
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

	refs := references.FindAllStringSubmatch(rec.Body.String(), -1)
	if len(refs) == 0 {
		t.Fatal("index.html references no script or stylesheet")
	}
	for _, ref := range refs {
		asset := get(t, h, http.MethodGet, ref[1])
		if asset.Code != 200 || asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("GET %s = %d, Cache-Control %q; want 200 with the immutable cache",
				ref[1], asset.Code, asset.Header().Get("Cache-Control"))
		}
	}
}
