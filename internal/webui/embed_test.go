//go:build webui

package webui

import (
	"net/http"
	"strings"
	"testing"
)

// TestEmbeddedApp runs only with the webui tag, after the web app has been
// built: the real build must be in the binary and served.
func TestEmbeddedApp(t *testing.T) {
	t.Parallel()

	h, embedded, err := New()
	if err != nil || !embedded {
		t.Fatalf("New() embedded = %v, err = %v; want the built app", embedded, err)
	}
	rec := get(t, h, http.MethodGet, "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `<div id="root">`) {
		t.Errorf("GET / = %d, body %q; want the app's index.html", rec.Code, rec.Body.String())
	}
}
