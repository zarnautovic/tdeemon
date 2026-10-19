// Package api wires the HTTP routes of the server.
package api

import (
	"io"
	"net/http"
)

// NewHandler returns the root HTTP handler with every route registered.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	return mux
}

// healthz reports that the process is up and serving HTTP. It checks
// nothing else on purpose: a liveness probe that fails on a dependency
// would restart a healthy process.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, "ok\n")
}
