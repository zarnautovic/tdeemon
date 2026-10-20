// Package api wires the HTTP routes of the server: /healthz, and the JSON
// API under /api/v1. The API's operations are registered with huma, which
// validates every request against the operation's schema and generates the
// OpenAPI 3.1 document served at /api/v1/openapi.json and committed as
// api/openapi.json.
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// Version is the version of the JSON API: part of every path and the
// info.version of the OpenAPI document. It changes only with a breaking
// change to the API, not with every release of the server.
const Version = "v1"

// Prefix is the path every API operation lives under.
const Prefix = "/api/" + Version

// ServerInfo describes the running server.
type ServerInfo struct {
	Name    string `json:"name" example:"tdeemon" doc:"Name of the server software."`
	Version string `json:"version" example:"v0.1.0" doc:"Version of the running server; dev for a local build."`
}

// NewHandler returns the root HTTP handler with every route registered.
func NewHandler(info ServerInfo) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	register(newAPI(mux), info)
	return mux
}

// Spec returns the OpenAPI document of the API, indented and with a
// trailing newline, exactly as committed in api/openapi.json. It does not
// depend on the running server, so the served and the committed document
// are the same.
func Spec() ([]byte, error) {
	a := newAPI(http.NewServeMux())
	register(a, ServerInfo{})
	b, err := json.MarshalIndent(a.OpenAPI(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func newAPI(mux *http.ServeMux) huma.API {
	cfg := huma.DefaultConfig("tdeemon API", Version)
	cfg.OpenAPIPath = Prefix + "/openapi"
	// No docs page: huma's loads its JavaScript from a CDN, and a
	// self-hosted app with health data should not call out on page load.
	cfg.DocsPath = ""
	// No $schema fields, Link headers or /schemas route: the OpenAPI
	// document is the one description of the API.
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil
	// A misspelled query parameter is an error, not silently ignored.
	cfg.RejectUnknownQueryParameters = true
	return humago.New(mux, cfg)
}

// register adds every API operation to a.
func register(a huma.API, info ServerInfo) {
	huma.Register(a, huma.Operation{
		OperationID: "get-info",
		Method:      http.MethodGet,
		Path:        Prefix + "/info",
		Summary:     "Get server info",
		Description: "Name and version of the running server.",
		Tags:        []string{"meta"},
	}, func(context.Context, *struct{}) (*infoOutput, error) {
		return &infoOutput{Body: info}, nil
	})
}

type infoOutput struct {
	Body ServerInfo
}

// healthz reports that the process is up and serving HTTP. It checks
// nothing else on purpose: a liveness probe that fails on a dependency
// would restart a healthy process.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, "ok\n")
}
