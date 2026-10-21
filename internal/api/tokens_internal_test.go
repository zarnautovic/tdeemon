package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
)

// An operation open to one scope takes a token with it, and only that;
// sessions and anonymous requests are the operation's to judge.
func TestGateTokensChecksTheScope(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	a := newAPI(mux)
	huma.Register(a, huma.Operation{
		OperationID: "ingest", Method: http.MethodPost, Path: "/ingest", DefaultStatus: http.StatusNoContent,
		Metadata: openToTokens(auth.ScopeIngest),
	}, func(context.Context, *struct{}) (*struct{}, error) { return nil, nil }) //nolint:nilnil // 204
	for _, tc := range []struct {
		name string
		p    *auth.Principal
		want int
	}{
		{"no one", nil, http.StatusNoContent},
		{"a session", &auth.Principal{Via: auth.ViaSession}, http.StatusNoContent},
		{"a token with the scope", &auth.Principal{Via: auth.ViaToken, Scopes: []auth.Scope{auth.ScopeIngest}}, http.StatusNoContent},
		{"a token without it", &auth.Principal{Via: auth.ViaToken, Scopes: []auth.Scope{auth.ScopeRead, auth.ScopeTools}}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/ingest", http.NoBody)
			if tc.p != nil {
				r = r.WithContext(auth.WithPrincipal(r.Context(), *tc.p))
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, r)
			if rec.Code != tc.want || (tc.want == http.StatusForbidden && !strings.Contains(rec.Body.String(), "does not have the ingest scope")) {
				t.Errorf("%d %s, want %d", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
}

// An empty scope, a variable that was never set, opens nothing.
func TestGateTokensEmptyScopeOpensNothing(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	a := newAPI(mux)
	huma.Register(a, huma.Operation{
		OperationID: "empty", Method: http.MethodGet, Path: "/empty", DefaultStatus: http.StatusNoContent,
		Metadata: openToTokens(""),
	}, func(context.Context, *struct{}) (*struct{}, error) { return nil, nil }) //nolint:nilnil // 204
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/empty", http.NoBody)
	r = r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{Via: auth.ViaToken, Scopes: auth.Scopes()}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a token on an operation opened to scope \"\": %d, want 403", rec.Code)
	}
}

// requireSession refuses a token even where gateTokens let it through.
func TestRequireSessionRefusesATokenItself(t *testing.T) {
	t.Parallel()

	ctx := auth.WithPrincipal(t.Context(), auth.Principal{Via: auth.ViaToken, Scopes: auth.Scopes()})
	_, err := requireSession(ctx)
	if se, ok := errors.AsType[huma.StatusError](err); !ok || se.GetStatus() != http.StatusForbidden {
		t.Errorf("requireSession with a token = %v, want 403", err)
	}
}
