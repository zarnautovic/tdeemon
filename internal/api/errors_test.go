package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// TestAPITyposDoNotReachACatchAll adds the kind of catch-all route the web
// app will bring (index.html for every other path): a mistyped API path
// must still get a problem, not the page.
func TestAPITyposDoNotReachACatchAll(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html>")
	})
	register(newAPI(mux, slog.New(slog.DiscardHandler)), ServerInfo{}, newSessions(Config{BaseURL: &url.URL{Scheme: "http", Host: "localhost"}, Now: time.Now}), nil)
	h := withAPIErrors(mux)

	for path, want := range map[string]string{
		"/api/v1/typo": "application/problem+json",
		"/api/v1/info": "application/json",
		"/day/today":   "text/html; charset=utf-8",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
			if got := rec.Header().Get("Content-Type"); got != want {
				t.Errorf("Content-Type = %q, want %q", got, want)
			}
		})
	}
}

// A 5xx answer keeps what failed inside the server to the log, whatever
// the handler returned: a plain error, which huma would answer with its
// text, or a 500 with the error as its detail. A 5xx that carries no
// detail (the server is busy) is no failure to log, and a 4xx's details
// are about the request, so they stay. A client that went away logs at
// debug level: it says nothing about the server.
func TestHideInternalErrors(t *testing.T) {
	t.Parallel()

	const inside = "open /data/tdeemon.db: SQLITE_CORRUPT"
	for _, tc := range []struct {
		name      string
		err       error
		gone      bool
		status    int
		wantLevel string // "" for nothing logged
		keeps     string // in the answer
	}{
		{name: "a plain error", err: errors.New(inside), status: http.StatusInternalServerError, wantLevel: "ERROR", keeps: "unexpected error occurred"},
		{name: "a 500 with the error", err: huma.Error500InternalServerError("reading failed", errors.New(inside)), status: http.StatusInternalServerError, wantLevel: "ERROR", keeps: "reading failed"},
		{name: "a 500 with no error", err: huma.Error500InternalServerError("reading failed", nil, errors.New(inside)), status: http.StatusInternalServerError, wantLevel: "ERROR", keeps: "reading failed"},
		{name: "a client that went away", err: errors.New(inside), gone: true, status: http.StatusInternalServerError, wantLevel: "DEBUG"},
		{name: "a 503 without details", err: huma.Error503ServiceUnavailable("busy; try again shortly"), status: http.StatusServiceUnavailable, keeps: "busy; try again shortly"},
		{name: "a 422 with details", err: huma.Error422UnprocessableEntity("bad", errors.New(inside)), status: http.StatusUnprocessableEntity, keeps: inside},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var log bytes.Buffer
			mux := http.NewServeMux()
			a := newAPI(mux, slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
			huma.Register(a, huma.Operation{OperationID: "fail", Method: http.MethodGet, Path: "/fail", Metadata: openToAnyToken()},
				func(context.Context, *struct{}) (*struct{}, error) { return nil, tc.err })

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.gone {
				cancel()
			}
			u := user.User{ID: uuid.NewV7()}
			ctx = auth.WithPrincipal(ctx, auth.Principal{User: u, Via: auth.ViaSession})
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/fail", nil))

			body := rec.Body.String()
			if rec.Code != tc.status || !strings.Contains(body, tc.keeps) {
				t.Errorf("answer %d %s, want %d with %q", rec.Code, body, tc.status, tc.keeps)
			}
			if tc.status >= http.StatusInternalServerError && (strings.Contains(body, "SQLITE") || strings.Contains(body, `"errors"`)) {
				t.Errorf("the answer %s tells what failed inside", body)
			}
			got := log.String()
			if tc.wantLevel == "" {
				if got != "" {
					t.Errorf("logged %q, want nothing", got)
				}
				return
			}
			for _, want := range []string{"level=" + tc.wantLevel, `msg="an API operation failed"`, inside, "operation_id=fail", "method=GET", "path=/fail", "user_id=" + u.ID.String()} {
				if !strings.Contains(got, want) {
					t.Errorf("logged %q, want %q in it", got, want)
				}
			}
		})
	}
}

// An operation that lists no errors has huma's default response, which
// covers the problems documentRequestErrors adds to the others.
func TestDocumentRequestErrorsWithoutErrors(t *testing.T) {
	t.Parallel()

	a := newAPI(http.NewServeMux(), slog.New(slog.DiscardHandler))
	huma.Register(a, huma.Operation{OperationID: "quiet", Method: http.MethodPost, Path: "/quiet", Metadata: openToAnyToken()},
		func(context.Context, *struct{ Body struct{} }) (*struct{}, error) { return nil, nil }) //nolint:nilnil // no output
	responses := a.OpenAPI().Paths["/quiet"].Post.Responses
	if _, ok := responses["default"]; !ok || len(responses) != 2 {
		t.Errorf("responses %v, want the success and the default", slices.Collect(maps.Keys(responses)))
	}
}
