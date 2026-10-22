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
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/ingest"
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
	// OIDC, when not nil, adds sign-in through that provider.
	OIDC *config.OIDC
	// SecretKey seals the OIDC flow cookie. Required with OIDC.
	SecretKey []byte
	// TrustedProxies are the reverse proxies whose X-Forwarded-For names
	// the client, for the limits on failed sign-ins and bad tokens.
	TrustedProxies []netip.Prefix
	// Now returns the current time; time.Now when nil.
	Now func() time.Time
	// Limits are the instance's limits of a plan; limits.Instance() when
	// zero.
	Limits limits.Limits
	// IngestArchive keeps the raw bodies of the ingest; none when nil.
	IngestArchive *ingest.Archive
}

// NewHandler returns the root HTTP handler with every route registered.
//
// Every request goes through, in order: the security headers; finding the
// client's address (behind TrustedProxies, the forwarded one);
// http.CrossOriginProtection, which refuses a cross-origin browser
// request that is not GET, HEAD or OPTIONS (CSRF); the session or token
// lookup for API requests; the routes.
func NewHandler(cfg Config) (http.Handler, error) {
	if cfg.DB == nil || cfg.BaseURL == nil || cfg.Logger == nil {
		return nil, errors.New("api: the database, the base URL and the logger are required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Limits == (limits.Limits{}) {
		cfg.Limits = limits.Instance()
	}
	s := newSessions(cfg)
	auth.PrepareLogin()
	var o *oidcLogin
	if cfg.OIDC != nil {
		if len(cfg.SecretKey) == 0 {
			return nil, errors.New("api: OIDC needs the secret key")
		}
		o = newOIDCLogin(s, *cfg.OIDC, cfg.SecretKey, cfg.BaseURL.Scheme+"://"+cfg.BaseURL.Host+oidcCallbackPath)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	if cfg.App != nil {
		mux.Handle("GET /", cfg.App)
	}
	a := newAPI(mux, cfg.Logger)
	register(a, cfg.Info, s, o)
	if o != nil {
		o.register(mux)
	}

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
	cl := &clients{trusted: cfg.TrustedProxies, logger: cfg.Logger}
	return securityHeaders(s.secure, cl.resolve(cop.Handler(s.authenticate(recordBodyErrors(withAPIErrors(mux)))))), nil
}

// bodyErrorKey is the key of a request's bodyError in its context.
type bodyErrorKey struct{}

// bodyError is what reading a request's body failed with, if it did.
type bodyError struct{ err error }

// recordBodyErrors keeps, in the request's context, what reading its body
// failed with. huma answers a body cut short with a 500 whose detail has
// only the error's text, and the log must tell the client's doing from
// the server's (logInternalError).
func recordBodyErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		be := &bodyError{}
		r = r.WithContext(context.WithValue(r.Context(), bodyErrorKey{}, be))
		r.Body = recordingBody{ReadCloser: r.Body, into: be}
		next.ServeHTTP(w, r)
	})
}

// recordingBody is a request's body that records a failed read.
type recordingBody struct {
	io.ReadCloser
	into *bodyError
}

func (b recordingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		b.into.err = err
	}
	return n, err
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
	return marshalSpec(specAPI())
}

// specAPI returns the API with every operation registered, for its
// description only: the handlers never run, so they need no database.
func specAPI() huma.API {
	a := newAPI(http.NewServeMux(), slog.New(slog.DiscardHandler))
	register(a, ServerInfo{}, &sessions{}, nil)
	return a
}

func marshalSpec(a huma.API) ([]byte, error) {
	b, err := json.MarshalIndent(a.OpenAPI(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func newAPI(mux *http.ServeMux, logger *slog.Logger) huma.API {
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
	// hideInternalErrors first: it reads the errors dropErrorValues drops.
	cfg.Transformers = append(cfg.Transformers, hideInternalErrors(logger), dropErrorValues)
	cfg.OnAddOperation = append(cfg.OnAddOperation, documentRequestErrors)
	a := humago.New(mux, cfg)
	// Before any operation registers: huma fixes an operation's
	// middlewares when it registers.
	a.UseMiddleware(gateTokens(a))
	return a
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

// documentRequestErrors lists in an operation's responses the problems
// answered before the handler runs, which huma does not list itself: the
// server's 401 for a bad API token and 429 for an address that sent too
// many (authenticateToken), huma's 422 for an unknown query parameter
// (RejectUnknownQueryParameters), the 403 gateTokens answers a token an
// operation not open to every token does not let in (which also covers a
// browser's cross-origin request: no operation open to every token is
// anything but a read), and for a body huma's 400 (not JSON), 408 (too
// slow), 413 (too large) and 415 (not JSON's content type).
//
// huma lists 500 for every operation that lists its errors; one that
// lists none has a default response instead, which covers these too.
func documentRequestErrors(_ *huma.OpenAPI, op *huma.Operation) {
	problem := op.Responses[strconv.Itoa(http.StatusInternalServerError)]
	if problem == nil {
		return
	}
	codes := []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusUnprocessableEntity}
	if scope, _ := op.Metadata[tokenScopeKey].(auth.Scope); scope != anyScope {
		codes = append(codes, http.StatusForbidden)
	}
	if op.RequestBody != nil {
		codes = append(codes, http.StatusBadRequest, http.StatusRequestTimeout,
			http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType)
	}
	for _, code := range codes {
		// A response the operation describes itself (its own headers or
		// words) stays.
		if _, ok := op.Responses[strconv.Itoa(code)]; !ok {
			op.Responses[strconv.Itoa(code)] = &huma.Response{Description: http.StatusText(code), Content: problem.Content}
		}
	}
}

// internalError is a handler's answer to a failure inside the server: a
// 500 with msg, whose detail carries err for hideInternalErrors to log.
func internalError(msg string, err error) error {
	return huma.Error500InternalServerError(msg, internalDetail(err))
}

// internalDetail is the detail of a 5xx that carries err, so the log can
// tell a client that went away (logLevel) from a failure.
func internalDetail(err error) *huma.ErrorDetail {
	return &huma.ErrorDetail{Message: err.Error(), Value: err}
}

// hideInternalErrors keeps what failed inside the server out of a 5xx
// answer and logs it instead, once, with the request it came from. huma
// answers a handler's error with its text in the answer's errors, and that
// text can name a file, a query or a user; so a handler returns the error
// (internalError, or plain) and does not log it itself. A 5xx without
// details (the server is busy) is no failure, and is not logged.
//
// The request comes from humago.Unwrap, not from the huma.Context, which
// has the same: contextcheck knows a request carries its context, and
// takes the context from a huma.Context for one made up. The API is
// humago's (newAPI).
func hideInternalErrors(logger *slog.Logger) huma.Transformer {
	return func(ctx huma.Context, _ string, v any) (any, error) {
		e, ok := v.(*huma.ErrorModel)
		if !ok || e.Status < http.StatusInternalServerError || len(e.Errors) == 0 {
			return v, nil
		}
		r, _ := humago.Unwrap(ctx)
		logInternalError(r, logger, ctx.Operation(), e)
		e.Errors = nil
		return e, nil
	}
}

// logInternalError logs the details of e, the answer of op to r, at the
// level of its worst error (logLevel); a detail that carries no error (a
// handler's plain error), or no detail at all, is a failure. A body that
// did not come whole (recordBodyErrors) is the client's doing.
func logInternalError(r *http.Request, logger *slog.Logger, op *huma.Operation, e *huma.ErrorModel) {
	msgs := make([]string, 0, len(e.Errors))
	level := slog.LevelDebug
	for _, d := range e.Errors {
		if d == nil {
			continue
		}
		msgs = append(msgs, d.Message)
		l := slog.LevelError
		if err, ok := d.Value.(error); ok {
			l = logLevel(err)
		}
		level = max(level, l)
	}
	if len(msgs) == 0 {
		level = slog.LevelError
	}
	if be, ok := r.Context().Value(bodyErrorKey{}).(*bodyError); ok && be.err != nil {
		// The body never came whole, which huma answers before any
		// handler or limit runs: the client's doing, as anyone can.
		level = slog.LevelDebug
	}
	attrs := []any{
		"status", e.Status, "error", strings.Join(msgs, "; "),
		"operation_id", op.OperationID, "method", r.Method, "path", r.URL.Path,
	}
	if p, ok := auth.PrincipalFrom(r.Context()); ok {
		attrs = append(attrs, "user_id", p.User.ID.String())
	}
	logger.Log(r.Context(), level, "an API operation failed", attrs...)
}

// register adds every API operation to a.
func register(a huma.API, info ServerInfo, s *sessions, o *oidcLogin) {
	s.register(a)
	s.registerTokens(a)
	s.registerPlan(a)
	s.registerIngest(a)
	registerMethods(a, o)
	huma.Register(a, huma.Operation{
		OperationID: "get-info",
		Method:      http.MethodGet,
		Path:        Prefix + "/info",
		Summary:     "Get server info",
		Description: "Name and version of the running server.",
		Tags:        []string{"meta"},
		// A Bearer request from an address with too many bad tokens.
		Errors:   []int{http.StatusTooManyRequests},
		Metadata: openToAnyToken(),
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
