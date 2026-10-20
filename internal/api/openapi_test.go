package api_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/api"
)

var update = flag.Bool("update", false, "rewrite api/openapi.json from the Go operations")

// specFile is the committed OpenAPI document, relative to this package.
var specFile = filepath.Join("..", "..", "api", "openapi.json")

// TestOpenAPISpecIsCurrent keeps api/openapi.json equal to the document
// the Go operations produce, so the TypeScript types generated from it
// cannot drift from the server, and every pull request that changes the
// API shows the change in the document.
func TestOpenAPISpecIsCurrent(t *testing.T) {
	t.Parallel()

	got, err := api.Spec()
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if *update {
		if err := os.WriteFile(specFile, got, 0o600); err != nil {
			t.Fatalf("write %s: %v", specFile, err)
		}
		return
	}
	want, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatalf("read %s: %v (create it with -update)", specFile, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale; refresh it with: go test ./internal/api -run TestOpenAPISpec -update", specFile)
	}
}

func TestServedSpecIsTheCommittedOne(t *testing.T) {
	t.Parallel()

	rec := serve(t, http.MethodGet, "/api/v1/openapi.json")
	if got := rec.Header().Get("Content-Type"); got != "application/openapi+json" {
		t.Errorf("Content-Type = %q, want application/openapi+json", got)
	}
	spec, err := api.Spec()
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if !bytes.Equal(rec.Body.Bytes(), spec) {
		t.Error("the served OpenAPI document differs from Spec(), which the committed file must equal")
	}
}

// TestSpecServesConcurrently matters under -race: concurrent first
// requests for the document on one handler must not race.
func TestSpecServesConcurrently(t *testing.T) {
	t.Parallel()

	h, err := api.NewHandler(testInfo)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/openapi.json", nil))
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		})
	}
	wg.Wait()
}

func TestSpecDescribesTheAPI(t *testing.T) {
	t.Parallel()

	spec, err := api.Spec()
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	var doc struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Version string `json:"version"`
		} `json:"info"`
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.OpenAPI != "3.1.0" || doc.Info.Version != api.Version {
		t.Errorf("openapi %q, info.version %q; want 3.1.0 and %q", doc.OpenAPI, doc.Info.Version, api.Version)
	}
	if _, ok := doc.Paths["/api/v1/info"]; !ok {
		t.Errorf("paths %v lack /api/v1/info", doc.Paths)
	}
	if bytes.Contains(spec, []byte(`"$schema"`)) {
		t.Error("the document still carries $schema properties")
	}
}
