package api_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/go-jose/go-jose/v4"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

const clientID = "tdeemon"

// grant is what the fake provider remembers about a code it issued.
type grant struct {
	challenge, nonce string
	claims           map[string]any
}

// idp is a fake OpenID Connect provider: discovery, keys, and a token
// endpoint that checks PKCE and returns a signed ID token.
type idp struct {
	srv  *httptest.Server
	key  *rsa.PrivateKey
	mu   sync.Mutex
	down bool
	// claims go into the next ID tokens, on top of iss, aud, exp and iat.
	claims map[string]any
	codes  map[string]grant
	// override, when set, replaces claims of the issued token (a wrong
	// audience, a wrong nonce…).
	override map[string]any
	// otherKey signs the next tokens with a key the provider does not
	// publish.
	otherKey bool
}

func newIDP(t *testing.T) *idp {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	p := &idp{key: key, codes: map[string]grant{}, claims: map[string]any{
		"sub": "subject-1", "email": "Ana@Example.com", "email_verified": true,
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /keys", p.keys)
	mux.HandleFunc("POST /token", p.token)
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *idp) isDown() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.down }

func (p *idp) discovery(w http.ResponseWriter, _ *http.Request) {
	if p.isDown() {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                p.srv.URL,
		"authorization_endpoint":                p.srv.URL + "/authorize",
		"token_endpoint":                        p.srv.URL + "/token",
		"jwks_uri":                              p.srv.URL + "/keys",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (p *idp) keys(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: &p.key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
	}})
}

// authorize plays the provider's sign-in page: it takes the URL tdeemon
// sent the browser to and returns the callback URL with a fresh code.
func (p *idp) authorize(t *testing.T, location string) string {
	t.Helper()

	u, err := url.Parse(location)
	if err != nil || !strings.HasPrefix(location, p.srv.URL+"/authorize") {
		t.Fatalf("the login did not send the browser to the provider: %q", location)
	}
	q := u.Query()
	if q.Get("client_id") != clientID || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" ||
		!strings.Contains(q.Get("scope"), "openid") || q.Get("nonce") == "" {
		t.Fatalf("authorization request %v lacks the client, code, PKCE, openid or nonce", q)
	}
	p.mu.Lock()
	claims := maps.Clone(p.claims)
	p.mu.Unlock()
	code := rand.Text()
	p.mu.Lock()
	p.codes[code] = grant{challenge: q.Get("code_challenge"), nonce: q.Get("nonce"), claims: claims}
	p.mu.Unlock()
	cb, _ := url.Parse(q.Get("redirect_uri"))
	cq := cb.Query()
	cq.Set("code", code)
	cq.Set("state", q.Get("state"))
	cb.RawQuery = cq.Encode()
	return cb.String()
}

func (p *idp) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	g, ok := p.codes[r.Form.Get("code")]
	delete(p.codes, r.Form.Get("code"))
	override, otherKey := p.override, p.otherKey
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	claims := map[string]any{
		"iss": p.srv.URL, "aud": clientID, "nonce": g.nonce,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}
	maps.Copy(claims, g.claims)
	maps.Copy(claims, override)
	key := p.key
	if otherKey {
		key, _ = rsa.GenerateKey(rand.Reader, 2048)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	payload, _ := json.Marshal(claims)
	sig, err := signer.Sign(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idToken, _ := sig.CompactSerialize()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": idToken})
}

// newOIDCInstance is newInstance with the fake provider configured. ana
// exists, without a password.
func newOIDCInstance(t *testing.T, p *idp) *instance {
	t.Helper()

	db := storetest.Open(t)
	c := &clock{now: time.Now().UTC().Truncate(time.Second)}
	u := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleAdmin,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: c.now, UpdatedAt: c.now,
	}
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	h, err := api.NewHandler(api.Config{
		Info: testInfo, DB: db, BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
		Logger: slog.New(slog.DiscardHandler), Now: c.Now,
		OIDC:      &config.OIDC{Issuer: p.srv.URL, ClientID: clientID, ClientSecret: "s3cret", Name: "Authentik"},
		SecretKey: []byte(strings.Repeat("k", 32)),
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return &instance{h: h, db: db, clock: c, user: u}
}

// cookieNamed returns the response's cookie called name, or nil.
func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// begin starts a sign-in and returns the flow cookie and where the
// provider would send the browser back.
func (in *instance) begin(t *testing.T, p *idp, returnTo string) (*http.Cookie, string) {
	t.Helper()

	path := "/api/v1/auth/oidc/login"
	if returnTo != "" {
		path += "?return_to=" + url.QueryEscape(returnTo)
	}
	rec := in.do(t, http.MethodGet, path, "", nil, nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("oidc login: %d %s", rec.Code, rec.Body.String())
	}
	flow := cookieNamed(rec, "tdeemon_oidc")
	if flow == nil || !flow.HttpOnly || flow.SameSite != http.SameSiteLaxMode || flow.MaxAge != 600 {
		t.Fatalf("flow cookie = %+v, want HttpOnly, SameSite=Lax, ten minutes", flow)
	}
	return flow, p.authorize(t, rec.Header().Get("Location"))
}

// finish follows the provider's redirect back to tdeemon.
func (in *instance) finish(t *testing.T, callback string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	u, err := url.Parse(callback)
	if err != nil {
		t.Fatalf("parse callback: %v", err)
	}
	return in.do(t, http.MethodGet, u.RequestURI(), "", cookies, nil)
}

func TestOIDCSignIn(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)
	flow, callback := in.begin(t, p, "/day/today?x=1")
	if !strings.HasPrefix(callback, "http://localhost/api/v1/auth/oidc/callback?") {
		t.Errorf("redirect URI %q is not the instance's callback", callback)
	}
	rec := in.finish(t, callback, flow)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/day/today?x=1" {
		t.Fatalf("callback: %d to %q (%s), want 303 to /day/today?x=1", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if c := cookieNamed(rec, "tdeemon_oidc"); c == nil || c.MaxAge >= 0 {
		t.Errorf("the flow cookie was not cleared: %+v", c)
	}
	session := cookieNamed(rec, "tdeemon_session")
	if session == nil {
		t.Fatal("no session cookie")
	}
	got := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{session}, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"email":"ana@example.com"`) {
		t.Errorf("session: %d %s, want ana", got.Code, got.Body.String())
	}

	// The next sign-in goes by the subject, whatever the email says.
	p.mu.Lock()
	p.claims["email"], p.claims["email_verified"] = "someone.else@example.com", false
	p.mu.Unlock()
	flow, callback = in.begin(t, p, "")
	if rec := in.finish(t, callback, flow); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("second sign-in: %d to %q, want 303 to /", rec.Code, rec.Header().Get("Location"))
	}
}

func TestOIDCSignInWithoutAnAccount(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		claims map[string]any
	}{
		{"unverified email", map[string]any{"email_verified": false}},
		{"unknown email", map[string]any{"email": "ivo@example.com"}},
		{"no email", map[string]any{"email": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newIDP(t)
			maps.Copy(p.claims, tc.claims)
			in := newOIDCInstance(t, p)
			flow, callback := in.begin(t, p, "")
			rec := in.finish(t, callback, flow)
			if rec.Code != http.StatusForbidden || cookieNamed(rec, "tdeemon_session") != nil ||
				!strings.Contains(rec.Body.String(), "no account for you on this instance") {
				t.Errorf("callback: %d %s, want 403 and no session", rec.Code, rec.Body.String())
			}
		})
	}
}

// The callback must come from this browser's own sign-in, untouched and
// in time.
func TestOIDCCallbackRefusesForeignFlows(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)
	flow, callback := in.begin(t, p, "")
	_, otherCallback := in.begin(t, p, "") // someone else's flow
	forged := requestCookie(flow.Name, auth.Seal([]byte(strings.Repeat("x", 32)), "oidc flow", []byte(`{"state":"s"}`)))

	for _, tc := range []struct {
		name     string
		callback string
		cookies  []*http.Cookie
	}{
		{"no flow cookie (login CSRF)", callback, nil},
		{"another flow's state", otherCallback, []*http.Cookie{flow}},
		{"a cookie sealed with another key", callback, []*http.Cookie{forged}},
		{"a cookie that is not sealed", callback, []*http.Cookie{requestCookie("tdeemon_oidc", "x")}},
	} {
		rec := in.finish(t, tc.callback, tc.cookies...)
		if rec.Code != http.StatusBadRequest || cookieNamed(rec, "tdeemon_session") != nil {
			t.Errorf("%s: %d %s, want 400 and no session", tc.name, rec.Code, rec.Body.String())
		}
	}

	in.clock.Set(in.clock.Now().Add(10 * time.Minute))
	if rec := in.finish(t, callback, flow); rec.Code != http.StatusBadRequest {
		t.Errorf("after ten minutes: %d, want 400", rec.Code)
	}
}

// Tokens that fail verification sign nobody in.
func TestOIDCRefusesBadTokens(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		override map[string]any
		otherKey bool
	}{
		{"another audience", map[string]any{"aud": "someone-else"}, false},
		{"another issuer", map[string]any{"iss": "https://evil.example"}, false},
		{"another nonce", map[string]any{"nonce": "replayed"}, false},
		{"expired", map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}, false},
		{"unknown key", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newIDP(t)
			p.override, p.otherKey = tc.override, tc.otherKey
			in := newOIDCInstance(t, p)
			flow, callback := in.begin(t, p, "")
			rec := in.finish(t, callback, flow)
			if rec.Code != http.StatusUnauthorized || cookieNamed(rec, "tdeemon_session") != nil {
				t.Errorf("callback: %d %s, want 401 and no session", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestOIDCProviderFailures(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)

	// Down at the first sign-in: a 502, and the next attempt discovers it.
	p.mu.Lock()
	p.down = true
	p.mu.Unlock()
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/oidc/login", "", nil, nil); rec.Code != http.StatusBadGateway {
		t.Errorf("login while the provider is down: %d, want 502", rec.Code)
	}
	p.mu.Lock()
	p.down = false
	p.mu.Unlock()
	flow, callback := in.begin(t, p, "")

	// A code the provider does not know (or a wrong PKCE verifier).
	u, _ := url.Parse(callback)
	q := u.Query()
	q.Set("code", "not-a-code")
	u.RawQuery = q.Encode()
	if rec := in.finish(t, u.String(), flow); rec.Code != http.StatusBadGateway {
		t.Errorf("callback with an unknown code: %d %s, want 502", rec.Code, rec.Body.String())
	}

	// The provider says no.
	rec := in.do(t, http.MethodGet, "/api/v1/auth/oidc/callback?error=access_denied%3Cscript%3E&state=x", "", []*http.Cookie{flow}, nil)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "(access_deniedscript)") {
		t.Errorf("callback with an error: %d %s, want 401 naming the cleaned-up code", rec.Code, rec.Body.String())
	}
}

// The PKCE verifier stays in the flow cookie: a code without it is worth
// nothing to whoever intercepts it.
func TestOIDCSendsTheVerifier(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)
	flow, callback := in.begin(t, p, "")
	// Swap the challenge the provider remembers: the verifier tdeemon
	// sends no longer matches, and the exchange fails.
	p.mu.Lock()
	for code, g := range p.codes {
		g.challenge = "something-else"
		p.codes[code] = g
	}
	p.mu.Unlock()
	if rec := in.finish(t, callback, flow); rec.Code != http.StatusBadGateway {
		t.Errorf("callback with a mismatched verifier: %d, want 502", rec.Code)
	}
}

func TestReturnToStaysOnTheInstance(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)
	for returnTo, want := range map[string]string{
		"/foods":                "/foods",
		"//evil.example/x":      "/",
		"https://evil.example/": "/",
		"/\\evil.example":       "/",
		"javascript:alert(1)":   "/",
		"foods":                 "/",
		"/x\r\nSet-Cookie: a=b": "/",
		"/%2F%2Fevil.example":   "/%2F%2Fevil.example",
	} {
		flow, callback := in.begin(t, p, returnTo)
		rec := in.finish(t, callback, flow)
		if got := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || got != want {
			t.Errorf("return_to %q: %d to %q, want %q", returnTo, rec.Code, got, want)
		}
	}
}

func TestAuthMethods(t *testing.T) {
	t.Parallel()

	rec := newInstance(t, "http://localhost", "").do(t, http.MethodGet, "/api/v1/auth/methods", "", nil, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"password":true}` {
		t.Errorf("methods without OIDC: %d %s", rec.Code, rec.Body.String())
	}
	rec = newOIDCInstance(t, newIDP(t)).do(t, http.MethodGet, "/api/v1/auth/methods", "", nil, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"oidc":{"name":"Authentik","login_url":"/api/v1/auth/oidc/login"}`) {
		t.Errorf("methods with OIDC: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOIDCNeedsTheSecretKey(t *testing.T) {
	t.Parallel()

	_, err := api.NewHandler(api.Config{
		DB: storetest.Open(t), BaseURL: &url.URL{Scheme: "http", Host: "localhost"}, Logger: slog.New(slog.DiscardHandler),
		OIDC: &config.OIDC{Issuer: "https://auth.example.com", ClientID: clientID},
	})
	if err == nil {
		t.Error("NewHandler with OIDC and no secret key succeeded")
	}
}

// When the database fails after the provider vouched for the user, the
// sign-in fails with a 500 and no session.
func TestOIDCStoreFailures(t *testing.T) {
	t.Parallel()

	for _, table := range []string{"user_identities", "sessions"} {
		p := newIDP(t)
		in := newOIDCInstance(t, p)
		flow, callback := in.begin(t, p, "")
		in.failWrites(t, "INSERT", table)
		if rec := in.finish(t, callback, flow); rec.Code != http.StatusInternalServerError || cookieNamed(rec, "tdeemon_session") != nil {
			t.Errorf("callback when writing %s fails: %d, want 500 and no session", table, rec.Code)
		}
	}
}
