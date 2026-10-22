package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
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
// are about the request, so they stay. The error decides the level: a
// client that went away, or a request that ran out of time, says nothing
// about the server and logs at debug level, but a failure that came while
// the client left is still a failure.
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
		unsaid    bool   // what failed is not in the log either
	}{
		{name: "a plain error", err: errors.New(inside), status: http.StatusInternalServerError, wantLevel: "ERROR", keeps: "unexpected error occurred"},
		{name: "a 500 with the error", err: huma.Error500InternalServerError("reading failed", errors.New(inside)), status: http.StatusInternalServerError, wantLevel: "ERROR", keeps: "reading failed"},
		{name: "a 500 with no error", err: huma.Error500InternalServerError("reading failed", nil, errors.New(inside)), status: http.StatusInternalServerError, wantLevel: "ERROR", keeps: "reading failed"},
		{name: "a 500 with only a nil error", err: huma.Error500InternalServerError("reading failed", nil), status: http.StatusInternalServerError, wantLevel: "ERROR", keeps: "reading failed", unsaid: true},
		{name: "a failure", err: internalError("reading failed", errors.New(inside)), status: http.StatusInternalServerError, wantLevel: "ERROR", keeps: "reading failed"},
		{name: "a client that went away", err: internalError("reading failed", fmt.Errorf("%s: %w", inside, context.Canceled)), gone: true, status: http.StatusInternalServerError, wantLevel: "DEBUG"},
		{name: "a timeout", err: internalError("reading failed", fmt.Errorf("%s: %w", inside, context.DeadlineExceeded)), status: http.StatusInternalServerError, wantLevel: "DEBUG"},
		{name: "no hashing slot", err: internalError("signing in failed", fmt.Errorf("%s: %w", inside, auth.ErrBusy)), status: http.StatusInternalServerError, wantLevel: "WARN"},
		{name: "a failure while the client went away", err: internalError("reading failed", errors.New(inside)), gone: true, status: http.StatusInternalServerError, wantLevel: "ERROR"},
		{name: "a plain error while the client went away", err: errors.New(inside), gone: true, status: http.StatusInternalServerError, wantLevel: "ERROR"},
		{name: "a failure and a cancellation", err: huma.Error500InternalServerError("reading failed", internalDetail(errors.New(inside)), internalDetail(context.Canceled)), status: http.StatusInternalServerError, wantLevel: "ERROR"},
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
			wants := []string{"level=" + tc.wantLevel, `msg="an API operation failed"`, "operation_id=fail", "method=GET", "path=/fail", "user_id=" + u.ID.String()}
			if !tc.unsaid {
				wants = append(wants, inside)
			}
			for _, want := range wants {
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

// A handler passes on the error it cannot handle whole (internalError),
// so a client that leaves while the handler runs, waiting for the
// database's write lock, is logged at debug level and a failure at error
// level. The handlers run directly, with the client gone after the
// session's lookup.
func TestHandlersPassOnTheirErrors(t *testing.T) {
	t.Parallel()

	s := newTestSessions(storetest.Open(t), time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC))
	female, born, height := "female", "1990-06-15", 168.5
	limitsIn := &setLimitsInput{}
	limitsIn.Body.Limits = map[string]float64{"cut_rate": -0.5}
	tokenIn := &createTokenInput{}
	tokenIn.Body.Name, tokenIn.Body.Scopes = "phone", []string{"read"}
	for op, run := range map[string]func(context.Context) error{
		"get-profile": func(ctx context.Context) error { _, err := s.getProfile(ctx, nil); return err },
		"set-profile": func(ctx context.Context) error {
			_, err := s.setProfile(ctx, &setProfileInput{Body: ProfileFields{Sex: &female, BirthDate: &born, HeightCm: &height}})
			return err
		},
		"get-limits":   func(ctx context.Context) error { _, err := s.getLimits(ctx, nil); return err },
		"set-limits":   func(ctx context.Context) error { _, err := s.setLimits(ctx, limitsIn); return err },
		"list-tokens":  func(ctx context.Context) error { _, err := s.listTokens(ctx, nil); return err },
		"create-token": func(ctx context.Context) error { _, err := s.createToken(ctx, tokenIn); return err },
		"revoke-token": func(ctx context.Context) error {
			_, err := s.revokeToken(ctx, &revokeTokenInput{ID: uuid.NewV7().String()})
			return err
		},
		"logout": func(ctx context.Context) error { _, err := s.logout(ctx, nil); return err },
	} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			u := user.User{ID: uuid.NewV7(), TimeZone: "Europe/Zagreb", DayStartHour: 4}
			ctx = auth.WithPrincipal(ctx, auth.Principal{User: u, Via: auth.ViaSession, SessionID: "a session"})
			var e *huma.ErrorModel
			if err := run(ctx); !errors.As(err, &e) || e.Status != http.StatusInternalServerError || len(e.Errors) != 1 {
				t.Fatalf("error %v, want a 500 with one detail", err)
			}
			err, ok := e.Errors[0].Value.(error)
			if !ok || !errors.Is(err, context.Canceled) || logLevel(err) != slog.LevelDebug {
				t.Errorf("the detail carries %#v, want the error, a cancellation", e.Errors[0].Value)
			}
		})
	}
}

// A response an operation describes itself (its own headers, its own
// words) stays: documentRequestErrors adds only those it lacks.
func TestDocumentRequestErrorsKeepsAnOperationsOwn(t *testing.T) {
	t.Parallel()

	a := newAPI(http.NewServeMux(), slog.New(slog.DiscardHandler))
	own := &huma.Response{Description: "Too many tries; wait as Retry-After says."}
	huma.Register(a, huma.Operation{
		OperationID: "patient", Method: http.MethodGet, Path: "/patient", Metadata: openToAnyToken(),
		Errors:    []int{http.StatusNotFound},
		Responses: map[string]*huma.Response{"429": own},
	}, func(context.Context, *struct{}) (*struct{}, error) { return nil, nil }) //nolint:nilnil // no output
	responses := a.OpenAPI().Paths["/patient"].Get.Responses
	if responses["429"] != own {
		t.Errorf("429 = %+v, want the operation's own", responses["429"])
	}
	if responses["401"] == nil {
		t.Error("no 401 added")
	}
}
