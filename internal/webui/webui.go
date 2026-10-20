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

// assetsDir holds Vite's output files, whose names carry a content hash
// (build.assetsDir in web/vite.config.ts).
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
//   - index.html for every other path, so the app's client-side router can
//     own it, never cached, so a deploy takes effect on the next page load;
//   - 404 instead for a missing file under assets/, so a tab still open
//     from the previous build gets an error for its old script, not HTML,
//     and for a missing path with an extension that a page asks for (a
//     favicon, robots.txt). A navigation to such a path still gets the app,
//     so a client route may end in a dot-separated token (/invite/a.b.c).
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
				// The answer below depends on these headers; caches must
				// not hand a 404 for a fetch to a later navigation.
				w.Header().Add("Vary", "Sec-Fetch-Mode, Accept")
			}
			if strings.HasPrefix(name, assetsDir) || (path.Ext(name) != "" && !isNavigation(r)) {
				w.Header().Set("Cache-Control", "no-store")
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

// isNavigation reports whether r loads a page, as opposed to a script,
// image or fetch: browsers say so in Sec-Fetch-Mode; for clients that do
// not send it, a request that accepts HTML counts.
func isNavigation(r *http.Request) bool {
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" {
		return mode == "navigate"
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
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
