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

// secretKey is the test instances' TDEEMON_SECRET_KEY.
func secretKey() []byte { return []byte(strings.Repeat("k", 32)) }

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
	// jwksURI, when set, replaces the keys' address in the discovery
	// document.
	jwksURI string
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
	jwks := p.srv.URL + "/keys"
	p.mu.Lock()
	if p.jwksURI != "" {
		jwks = p.jwksURI
	}
	p.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                p.srv.URL,
		"authorization_endpoint":                p.srv.URL + "/authorize",
		"token_endpoint":                        p.srv.URL + "/token",
		"jwks_uri":                              jwks,
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

// newOIDCInstance is newInstance with the fake provider configured, for
// an instance at http://localhost. ana exists, without a password.
func newOIDCInstance(t *testing.T, p *idp) *instance {
	t.Helper()
	return newOIDCInstanceAt(t, p, &url.URL{Scheme: "http", Host: "localhost"})
}

func newOIDCInstanceAt(t *testing.T, p *idp, base *url.URL) *instance {
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
		Info: testInfo, DB: db, BaseURL: base,
		Logger: slog.New(slog.DiscardHandler), Now: c.Now,
		OIDC:      &config.OIDC{Issuer: p.srv.URL, ClientID: clientID, ClientSecret: "s3cret", Name: "Authentik"},
		SecretKey: secretKey(),
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return &instance{h: h, db: db, clock: c, user: u}
}

// requestCookie is a cookie as a client sends it: a name and a value.
func requestCookie(name, value string) *http.Cookie {
	return &http.Cookie{Name: name, Value: value} //nolint:gosec // G124: a request's cookie carries no attributes.
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
	if flow == nil {
		flow = cookieNamed(rec, "__Host-tdeemon_oidc")
	}
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
		want   string
	}{
		{"unverified email", map[string]any{"email_verified": false}, "does not confirm that your email address is verified"},
		{"unverified, as a string", map[string]any{"email_verified": "false"}, "does not confirm that your email address is verified"},
		{"unknown email", map[string]any{"email": "ivo@example.com"}, "no account for you on this instance"},
		{"no email", map[string]any{"email": ""}, "no account for you on this instance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newIDP(t)
			maps.Copy(p.claims, tc.claims)
			in := newOIDCInstance(t, p)
			flow, callback := in.begin(t, p, "")
			rec := in.finish(t, callback, flow)
			if rec.Code != http.StatusForbidden || cookieNamed(rec, "tdeemon_session") != nil ||
				!strings.Contains(rec.Body.String(), tc.want) {
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
	// The real flow, sealed with another key: only the MAC is wrong.
	genuine, err := auth.Open(secretKey(), "oidc flow", flow.Value)
	if err != nil {
		t.Fatalf("open the real flow: %v", err)
	}
	forged := requestCookie(flow.Name, auth.Seal([]byte(strings.Repeat("x", 32)), "oidc flow", genuine))

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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := in.finish(t, tc.callback, tc.cookies...)
			if rec.Code != http.StatusBadRequest || cookieNamed(rec, "tdeemon_session") != nil {
				t.Errorf("callback: %d %s, want 400 and no session", rec.Code, rec.Body.String())
			}
		})
	}

	// On an instance of its own: the clock moves.
	t.Run("after ten minutes", func(t *testing.T) {
		t.Parallel()
		p := newIDP(t)
		in := newOIDCInstance(t, p)
		flow, callback := in.begin(t, p, "")
		in.clock.Set(in.clock.Now().Add(10 * time.Minute))
		if rec := in.finish(t, callback, flow); rec.Code != http.StatusBadRequest {
			t.Errorf("callback: %d, want 400", rec.Code)
		}
	})
}

// The seal vouches for the flow cookie, not for the target in it: the
// callback checks return_to again, so a flow sealed with a leaked key
// cannot send the browser away.
func TestOIDCCallbackChecksTheReturnToAgain(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)
	flow, callback := in.begin(t, p, "")
	value, err := auth.Open(secretKey(), "oidc flow", flow.Value)
	if err != nil {
		t.Fatalf("open the flow: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal(value, &f); err != nil {
		t.Fatalf("decode the flow: %v", err)
	}
	f["return_to"] = "//evil.example"
	if value, err = json.Marshal(f); err != nil {
		t.Fatalf("encode the flow: %v", err)
	}
	resealed := requestCookie(flow.Name, auth.Seal(secretKey(), "oidc flow", value))
	if rec := in.finish(t, callback, resealed); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("callback: %d to %q, want 303 to /", rec.Code, rec.Header().Get("Location"))
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
	// The failure answers for itself for a while, without asking again…
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/oidc/login", "", nil, nil); rec.Code != http.StatusBadGateway {
		t.Errorf("login right after the failure: %d, want the cached 502", rec.Code)
	}
	// …then the next sign-in discovers the provider.
	in.clock.Set(in.clock.Now().Add(31 * time.Second))
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
		"/\xff":                 "/",
	} {
		t.Run(returnTo, func(t *testing.T) {
			t.Parallel()
			flow, callback := in.begin(t, p, returnTo)
			rec := in.finish(t, callback, flow)
			if got := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || got != want {
				t.Errorf("return_to %q: %d to %q, want %q", returnTo, rec.Code, got, want)
			}
		})
	}
}

// JSON escapes some characters into several bytes; at the longest
// return_to the flow cookie must still fit the 4 KB a browser keeps, or
// the sign-in fails without a word.
func TestOIDCFlowCookieFits(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)
	for _, char := range []string{"a", "&", "<", `"`, "\u2028", "č"} {
		t.Run(char, func(t *testing.T) {
			t.Parallel()
			returnTo := "/" + strings.Repeat(char, 1023/len(char))
			flow, callback := in.begin(t, p, returnTo)
			if n := len(flow.Name) + len(flow.Value); n > 4096 {
				t.Errorf("flow cookie is %d bytes, want at most 4096", n)
			}
			if rec := in.finish(t, callback, flow); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") == "/" {
				t.Errorf("callback: %d to %.40q…, want 303 to the return_to", rec.Code, rec.Header().Get("Location"))
			}
		})
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
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			p := newIDP(t)
			in := newOIDCInstance(t, p)
			flow, callback := in.begin(t, p, "")
			in.failWrites(t, "INSERT", table)
			if rec := in.finish(t, callback, flow); rec.Code != http.StatusInternalServerError || cookieNamed(rec, "tdeemon_session") != nil {
				t.Errorf("callback when writing %s fails: %d, want 500 and no session", table, rec.Code)
			}
		})
	}
}

// Over HTTPS the flow cookie is a __Host- cookie, like the session's.
func TestOIDCFlowCookieOverHTTPS(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstanceAt(t, p, &url.URL{Scheme: "https", Host: "tdeemon.example"})
	rec := in.do(t, http.MethodGet, "/api/v1/auth/oidc/login", "", nil, nil)
	c := cookieNamed(rec, "__Host-tdeemon_oidc")
	if rec.Code != http.StatusFound || c == nil || !c.Secure || c.Path != "/" || c.Domain != "" {
		t.Errorf("login: %d, flow cookie %+v; want __Host-tdeemon_oidc, Secure, Path=/", rec.Code, c)
	}
}

func TestOIDCAccountLinkedElsewhere(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)
	flow, callback := in.begin(t, p, "")
	if rec := in.finish(t, callback, flow); rec.Code != http.StatusSeeOther {
		t.Fatalf("first sign-in: %d", rec.Code)
	}
	// Another account at the provider with ana's address.
	p.mu.Lock()
	p.claims["sub"] = "subject-2"
	p.mu.Unlock()
	flow, callback = in.begin(t, p, "")
	rec := in.finish(t, callback, flow)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "linked to another account") {
		t.Errorf("second account: %d %s, want 403 naming the link", rec.Code, rec.Body.String())
	}
}

// email_verified sent as a string still counts.
func TestOIDCEmailVerifiedAsAString(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	p.claims["email_verified"] = "true"
	in := newOIDCInstance(t, p)
	flow, callback := in.begin(t, p, "")
	if rec := in.finish(t, callback, flow); rec.Code != http.StatusSeeOther {
		t.Errorf("sign-in with email_verified \"true\": %d %s, want 303", rec.Code, rec.Body.String())
	}
}

// When the keys cannot be fetched, the provider is at fault: a 502.
func TestOIDCKeysUnreachable(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	p.jwksURI = gone.URL + "/keys"
	in := newOIDCInstance(t, p)
	flow, callback := in.begin(t, p, "")
	if rec := in.finish(t, callback, flow); rec.Code != http.StatusBadGateway {
		t.Errorf("callback without keys: %d %s, want 502", rec.Code, rec.Body.String())
	}
}

func TestReturnToHasALimit(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	in := newOIDCInstance(t, p)
	flow, callback := in.begin(t, p, "/"+strings.Repeat("a", 1100))
	if rec := in.finish(t, callback, flow); rec.Header().Get("Location") != "/" {
		t.Errorf("a 1.1 KB return_to: redirect to %.40q…, want /", rec.Header().Get("Location"))
	}
}

// Case folding maps the Kelvin sign onto k: an address written with it at
// the provider must not link to kate@example.com.
func TestOIDCLookAlikeAddressDoesNotLink(t *testing.T) {
	t.Parallel()

	p := newIDP(t)
	p.claims["email"], p.claims["sub"] = "\u212Aate@example.com", "kelvin"
	in := newOIDCInstance(t, p)
	now := in.clock.Now()
	kate := user.User{
		ID: uuid.NewV7(), Email: "kate@example.com", Name: "Kate", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := in.db.CreateUser(t.Context(), kate, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	flow, callback := in.begin(t, p, "")
	rec := in.finish(t, callback, flow)
	if rec.Code != http.StatusForbidden || cookieNamed(rec, "tdeemon_session") != nil ||
		!strings.Contains(rec.Body.String(), "written differently from the one on your account here") {
		t.Errorf("callback: %d %s, want 403 saying the address is written differently, and no session", rec.Code, rec.Body.String())
	}
	// The same address in plain ASCII, in another case, links.
	p.mu.Lock()
	p.claims["email"] = "Kate@Example.com"
	p.mu.Unlock()
	flow, callback = in.begin(t, p, "")
	if rec := in.finish(t, callback, flow); rec.Code != http.StatusSeeOther {
		t.Errorf("the plain address: %d %s, want 303", rec.Code, rec.Body.String())
	}
}
