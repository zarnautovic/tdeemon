// Package webui serves the web app from the binary. Vite builds web/ into
// this package's dist/ directory, and the build tag webui compiles it in:
//
//	(cd web && npm ci && npm run build)
//	go build -tags webui ./cmd/tdeemon
//
// Without the tag the server works the same but serves a short notice at /
// instead of the app, so Go builds and tests never need Node.
package webui

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// assetsDir holds Vite's output files, whose names carry a content hash.
const assetsDir = "assets/"

// New returns the handler for the paths the API does not own, and whether
// the web app is compiled in.
func New() (http.Handler, bool, error) {
	fsys, err := embeddedFS()
	if err != nil {
		return nil, false, err
	}
	if fsys == nil {
		return http.HandlerFunc(noApp), false, nil
	}
	h, err := Handler(fsys)
	if err != nil {
		return nil, false, err
	}
	return h, true, nil
}

// Handler serves the built web app in fsys:
//   - files under assets/ with a year-long immutable cache, because a new
//     build gives every changed file a new name;
//   - other files (favicon and the like) with revalidation on every use;
//   - index.html for every other path without a file extension, so the
//     app's client-side router can own it, never cached, so a deploy
//     takes effect on the next page load;
//   - 404 for a missing path with an extension, so a tab still open from
//     the previous build gets an error for its old script, not HTML.
func Handler(fsys fs.FS) (http.Handler, error) {
	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		return nil, fmt.Errorf("web app: %w", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The mux has already cleaned the path; this only drops the slash.
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				w.Header().Set("Cache-Control", cacheControl(name))
				http.ServeFileFS(w, r, fsys, name)
				return
			}
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(index)))
		_, _ = w.Write(index)
	}), nil
}

func cacheControl(name string) string {
	if strings.HasPrefix(name, assetsDir) {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

// noApp answers in a build without the web app.
func noApp(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, "tdeemon is running, but this build has no web app.\n"+
		"Build it with `npm ci && npm run build` in web/, then `go build -tags webui ./cmd/tdeemon`.\n")
}
