package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/config"
)

// An https issuer whose discovery document sends the token exchange or
// the keys over plain HTTP is refused, and not kept.
func TestProviderEndpointsMustBeHTTPS(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, token, jwks string }{
		{"token endpoint", "http://idp.example/token", ""},
		{"keys", "", "http://idp.example/keys"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var srv *httptest.Server
			srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				token, jwks := srv.URL+"/token", srv.URL+"/keys"
				if tc.token != "" {
					token = tc.token
				}
				if tc.jwks != "" {
					jwks = tc.jwks
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issuer": srv.URL, "authorization_endpoint": srv.URL + "/authorize",
					"token_endpoint": token, "jwks_uri": jwks,
				})
			}))
			t.Cleanup(srv.Close)

			now := time.Now()
			s := &sessions{logger: slog.New(slog.DiscardHandler), now: func() time.Time { return now }}
			o := newOIDCLogin(s, config.OIDC{Issuer: srv.URL, ClientID: "tdeemon"}, []byte(strings.Repeat("k", 32)), "https://tdeemon.example/cb")
			o.client.Transport = srv.Client().Transport // trusts the test server's certificate
			if _, err := o.getProvider(t.Context()); err == nil || !strings.Contains(err.Error(), "not https") {
				t.Errorf("getProvider = %v, want a refusal of the plain HTTP endpoint", err)
			}
			if o.provider != nil {
				t.Error("the refused provider was kept")
			}
		})
	}
}

// The client newOIDCLogin builds follows no redirect off https: the
// discovery document would come over plain HTTP otherwise.
func TestOIDCClientRefusesInsecureRedirects(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.RedirectHandler("http://127.0.0.1:1/.well-known/openid-configuration", http.StatusFound))
	t.Cleanup(srv.Close)
	now := time.Now()
	s := &sessions{logger: slog.New(slog.DiscardHandler), now: func() time.Time { return now }}
	o := newOIDCLogin(s, config.OIDC{Issuer: srv.URL, ClientID: "tdeemon"}, []byte(strings.Repeat("k", 32)), "https://tdeemon.example/cb")
	o.client.Transport = srv.Client().Transport // trusts the test server's certificate; the rest is newOIDCLogin's
	if _, err := o.getProvider(t.Context()); err == nil || !strings.Contains(err.Error(), "which is not https") {
		t.Errorf("getProvider = %v, want the redirect to plain HTTP refused", err)
	}
}

func TestRedirectsStayOnHTTPS(t *testing.T) {
	t.Parallel()

	req := func(u string) *http.Request {
		r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, u, http.NoBody)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		return r
	}
	strict, loopback := refuseInsecureRedirects(false), refuseInsecureRedirects(true)
	if err := strict(req("http://idp.example/x"), nil); err == nil {
		t.Error("a redirect from an https provider to http was followed")
	}
	if err := strict(req("https://idp.example/x"), nil); err != nil {
		t.Errorf("an https redirect was refused: %v", err)
	}
	if err := loopback(req("http://127.0.0.1/x"), nil); err != nil {
		t.Errorf("a plain HTTP redirect for a loopback issuer was refused: %v", err)
	}
	via := make([]*http.Request, 10)
	if err := strict(req("https://idp.example/x"), via); err == nil {
		t.Error("an eleventh redirect was followed")
	}
}
