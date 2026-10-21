// Package api wires the HTTP routes of the server: /healthz, and the JSON
// API under /api/v1. The API's operations are registered with huma, which
// validates every request against the operation's schema and generates the
// OpenAPI 3.1 document served at /api/v1/openapi.json and committed as
// api/openapi.json.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/store"
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
	Version string `json:"version" example:"v0.1.0" doc:"Version of the running server; dev when the build carries no version information."`
}

// Config is what NewHandler needs.
type Config struct {
	Info ServerInfo
	// App, if not nil, answers every GET request no other route takes
	// (the web app and its client-side routes); requests under /api/
	// never reach it.
	App http.Handler
	// DB is the database. Required.
	DB *store.DB
	// BaseURL is the instance's public URL. Required: over https the
	// session cookie is Secure, and the URL's origin is trusted for
	// cross-origin checks behind a proxy that rewrites Host.
	BaseURL *url.URL
	// Logger is required.
	Logger *slog.Logger
	// Now returns the current time; time.Now when nil.
	Now func() time.Time
}

// NewHandler returns the root HTTP handler with every route registered.
//
// Every request goes through, in order: http.CrossOriginProtection,
// which refuses a cross-origin browser request that is not GET, HEAD or
// OPTIONS (CSRF); the session lookup for API requests; the routes.
func NewHandler(cfg Config) (http.Handler, error) {
	if cfg.DB == nil || cfg.BaseURL == nil || cfg.Logger == nil {
		return nil, errors.New("api: the database, the base URL and the logger are required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &sessions{db: cfg.DB, logger: cfg.Logger, now: cfg.Now, secure: cfg.BaseURL.Scheme == "https"}
	auth.PrepareLogin()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	if cfg.App != nil {
		mux.Handle("GET /", cfg.App)
	}
	a := newAPI(mux)
	register(a, cfg.Info, s)

	// The document is served from bytes computed once, the same ones Spec
	// returns. huma's own route marshals it lazily without a lock, a data
	// race on concurrent first requests, and adds YAML and OpenAPI 3.0
	// variants that nothing checks.
	spec, err := marshalSpec(a)
	if err != nil {
		return nil, fmt.Errorf("openapi document: %w", err)
	}
	mux.HandleFunc("GET "+Prefix+"/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/openapi+json")
		w.Header().Set("Content-Length", strconv.Itoa(len(spec)))
		_, _ = w.Write(spec)
	})

	cop := http.NewCrossOriginProtection()
	if err := cop.AddTrustedOrigin(cfg.BaseURL.Scheme + "://" + cfg.BaseURL.Host); err != nil {
		return nil, fmt.Errorf("api: trusted origin: %w", err)
	}
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, http.StatusForbidden, "cross-origin request refused")
	}))
	return cop.Handler(s.authenticate(withAPIErrors(mux))), nil
}

// apiMethods are the methods withAPIErrors probes for an Allow header.
var apiMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost,
	http.MethodPut, http.MethodPatch, http.MethodDelete,
}

// withAPIErrors answers a request under /api/ that no API operation
// handles with problem details, like every other API error: 405 with an
// Allow header when the path exists for other methods, 404 otherwise.
// Without it the mux would answer in plain text, and a catch-all route
// added later (the web app's index.html) would answer API typos with HTML.
func withAPIErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAPIPath(r.URL.Path) || handlesAPI(mux, r) {
			mux.ServeHTTP(w, r)
			return
		}
		var allowed []string
		for _, m := range apiMethods {
			probe := *r
			probe.Method = m
			if m != r.Method && handlesAPI(mux, &probe) {
				allowed = append(allowed, m)
			}
		}
		if len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			writeProblem(w, http.StatusMethodNotAllowed, r.Method+" is not allowed on "+r.URL.Path)
			return
		}
		writeProblem(w, http.StatusNotFound, "no API operation at "+r.URL.Path)
	})
}

// handlesAPI reports whether mux routes r to an API operation.
func handlesAPI(mux *http.ServeMux, r *http.Request) bool {
	_, pattern := mux.Handler(r)
	// A pattern is "[METHOD ][HOST]/PATH"; ours carry no host.
	if _, path, ok := strings.Cut(pattern, " "); ok {
		pattern = path
	}
	return isAPIPath(pattern)
}

func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

// writeProblem writes an RFC 9457 problem in the shape huma uses for every
// other API error.
func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(&huma.ErrorModel{
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}

// Spec returns the OpenAPI document of the API, indented and with a
// trailing newline, exactly as committed in api/openapi.json and served at
// /api/v1/openapi.json. It does not depend on the running server.
func Spec() ([]byte, error) {
	a := newAPI(http.NewServeMux())
	// The operations' handlers never run here, so they need no database.
	register(a, ServerInfo{}, &sessions{})
	return marshalSpec(a)
}

func marshalSpec(a huma.API) ([]byte, error) {
	b, err := json.MarshalIndent(a.OpenAPI(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func newAPI(mux *http.ServeMux) huma.API {
	cfg := huma.DefaultConfig("tdeemon API", Version)
	// NewHandler serves the document itself.
	cfg.OpenAPIPath = ""
	// No docs page: huma's loads its JavaScript from a CDN, and a
	// self-hosted app with health data should not call out on page load.
	cfg.DocsPath = ""
	// No $schema fields, Link headers or /schemas route: the OpenAPI
	// document is the one description of the API.
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil
	// A misspelled query parameter is an error, not silently ignored.
	cfg.RejectUnknownQueryParameters = true
	cfg.Transformers = append(cfg.Transformers, dropErrorValues)
	return humago.New(mux, cfg)
}

// dropErrorValues keeps an error response from echoing what the request
// sent: huma puts the offending value into each error detail, and for a
// sign-in that is the password (a body that does not parse, a field of
// the wrong type). The location and the message say enough.
func dropErrorValues(_ huma.Context, _ string, v any) (any, error) {
	if e, ok := v.(*huma.ErrorModel); ok {
		for _, d := range e.Errors {
			if d != nil {
				d.Value = nil
			}
		}
	}
	return v, nil
}

// register adds every API operation to a.
func register(a huma.API, info ServerInfo, s *sessions) {
	s.register(a)
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
