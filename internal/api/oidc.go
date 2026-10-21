package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/danielgtaylor/huma/v2"
	"golang.org/x/oauth2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// The browser flow of an OpenID Connect sign-in (authorization code with
// PKCE): GET oidcLoginPath sends the browser to the provider, which sends
// it back to GET oidcCallbackPath with a code.
const (
	oidcLoginPath    = Prefix + "/auth/oidc/login"
	oidcCallbackPath = Prefix + "/auth/oidc/callback"
)

// flowLifetime is how long a user has at the provider before the flow
// cookie, and with it the sign-in, expires.
const flowLifetime = 10 * time.Minute

// sealPurpose keeps flow cookies from opening anything else sealed with
// the secret key.
const sealPurpose = "oidc flow"

// providerTimeout bounds every request to the provider.
const providerTimeout = 10 * time.Second

// discoveryRetry is how long a failed discovery answers for itself before
// the next sign-in tries again, so a provider that hangs costs one wait,
// not one per request.
const discoveryRetry = 30 * time.Second

// maxReturnTo caps return_to: it rides in the flow cookie, and a browser
// drops a cookie over 4 KB, which would fail the sign-in without a word.
// JSON without HTML escaping at most doubles it (a quote, U+2028) and
// base64 adds a third, so the cookie stays near 3 KB.
const maxReturnTo = 1024

// oidcLogin signs users in through an OpenID Connect provider.
type oidcLogin struct {
	sessions    *sessions
	cfg         config.OIDC
	secret      []byte
	redirectURL string
	client      *http.Client

	// The provider is discovered on first use and kept, so tdeemon starts
	// while the provider is down (the password still works); after a
	// failure, sign-ins answer with it for discoveryRetry, then try again.
	mu       sync.Mutex
	provider *oidc.Provider
	failed   error
	failedAt time.Time
	insecure bool // the issuer is plain HTTP on loopback
}

func newOIDCLogin(s *sessions, cfg config.OIDC, secret []byte, redirectURL string) *oidcLogin {
	// config.Load allows plain HTTP only for an issuer on loopback; then
	// its endpoints may be plain HTTP too.
	insecure := strings.HasPrefix(cfg.Issuer, "http://")
	return &oidcLogin{
		sessions: s, cfg: cfg, secret: secret, redirectURL: redirectURL, insecure: insecure,
		client: &http.Client{Timeout: providerTimeout, CheckRedirect: refuseInsecureRedirects(insecure)},
	}
}

// flow is what the flow cookie carries between the two requests.
type flow struct {
	State     string    `json:"state"`
	Nonce     string    `json:"nonce"`
	Verifier  string    `json:"verifier"`
	ReturnTo  string    `json:"return_to"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (o *oidcLogin) register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+oidcLoginPath, o.start)
	mux.HandleFunc("GET "+oidcCallbackPath, o.callback)
}

// cookieName is the flow cookie's name, __Host- prefixed over HTTPS like
// the session cookie.
func (o *oidcLogin) cookieName() string {
	if o.sessions.secure {
		return "__Host-tdeemon_oidc"
	}
	return "tdeemon_oidc"
}

func (o *oidcLogin) flowCookie(value string, maxAge int) *http.Cookie {
	//nolint:gosec // G124: Secure only over HTTPS, as for the session cookie.
	return &http.Cookie{
		Name: o.cookieName(), Value: value, Path: "/", MaxAge: maxAge,
		Secure: o.sessions.secure, HttpOnly: true,
		// Lax: the provider sends the browser back with a top-level GET,
		// which carries Lax cookies.
		SameSite: http.SameSiteLaxMode,
	}
}

// ctx returns a context for requests to the provider: with the client
// (its timeout and redirect rule; go-oidc also keeps it to fetch the keys
// later), and not cancelled with the request, so a client that goes away
// mid-discovery does not count as the provider failing.
func (o *oidcLogin) ctx(ctx context.Context) context.Context {
	return oidc.ClientContext(context.WithoutCancel(ctx), o.client)
}

func (o *oidcLogin) getProvider(ctx context.Context) (*oidc.Provider, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil {
		return o.provider, nil
	}
	now := o.sessions.now()
	if o.failed != nil && now.Sub(o.failedAt) < discoveryRetry {
		return nil, o.failed
	}
	p, err := oidc.NewProvider(o.ctx(ctx), o.cfg.Issuer)
	if err == nil {
		err = o.checkEndpoints(p)
	}
	if err != nil {
		o.failed, o.failedAt = err, now
		return nil, err
	}
	o.provider, o.failed = p, nil
	return p, nil
}

// checkEndpoints refuses a provider whose discovery document points the
// token exchange or the keys at plain HTTP, which would undo the https
// the issuer had to use.
func (o *oidcLogin) checkEndpoints(p *oidc.Provider) error {
	var doc struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := p.Claims(&doc); err != nil {
		return err
	}
	for _, raw := range []string{p.Endpoint().AuthURL, p.Endpoint().TokenURL, doc.JWKSURI} {
		u, err := url.Parse(raw)
		if err != nil || !allowedScheme(u.Scheme, o.insecure) {
			return fmt.Errorf("the provider's discovery document has an endpoint that is not https: %q", raw)
		}
	}
	return nil
}

// allowedScheme reports whether a URL of the provider may use scheme:
// https, or http when the issuer itself is plain HTTP on loopback.
func allowedScheme(scheme string, insecure bool) bool {
	return scheme == "https" || (insecure && scheme == "http")
}

// refuseInsecureRedirects is the client's CheckRedirect: a redirect from
// the provider may not leave https.
func refuseInsecureRedirects(insecure bool) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if !allowedScheme(req.URL.Scheme, insecure) {
			return fmt.Errorf("refused a redirect to %s, which is not https", req.URL.Redacted())
		}
		return nil
	}
}

func (o *oidcLogin) oauth2Config(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     o.cfg.ClientID,
		ClientSecret: o.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  o.redirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

// start sends the browser to the provider. return_to, a path on this
// instance, is where the browser lands after signing in.
func (o *oidcLogin) start(w http.ResponseWriter, r *http.Request) {
	p, err := o.getProvider(r.Context())
	if err != nil {
		o.sessions.logger.ErrorContext(r.Context(), "the OIDC provider cannot be reached", "error", err)
		writeProblem(w, http.StatusBadGateway, "the sign-in provider cannot be reached; try again, or sign in with a password")
		return
	}
	f := flow{
		State:     rand.Text(),
		Nonce:     rand.Text(),
		Verifier:  oauth2.GenerateVerifier(),
		ReturnTo:  safeReturnTo(r.URL.Query().Get("return_to")),
		ExpiresAt: o.sessions.now().Add(flowLifetime),
	}
	// Without HTML escaping: it would turn every & or < of return_to into
	// six bytes. A struct of strings and a time always encodes.
	var value bytes.Buffer
	enc := json.NewEncoder(&value)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(f)
	http.SetCookie(w, o.flowCookie(auth.Seal(o.secret, sealPurpose, value.Bytes()), int(flowLifetime.Seconds())))
	w.Header().Set("Cache-Control", "no-store")
	to := o.oauth2Config(p).AuthCodeURL(f.State, oidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier))
	http.Redirect(w, r, to, http.StatusFound)
}

// callback finishes the sign-in: it checks that this browser started the
// flow, trades the code for an ID token, verifies the token and signs in
// the user the identity belongs to.
func (o *oidcLogin) callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// The flow cookie is good for one attempt, whatever happens.
	http.SetCookie(w, o.flowCookie("", -1))
	w.Header().Set("Cache-Control", "no-store")

	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		// The query is the browser's to write: the log gets a code from the
		// standards' list, or "other".
		o.sessions.logger.InfoContext(ctx, "the OIDC provider refused the sign-in", "error", knownErrorCode(e))
		writeProblem(w, http.StatusUnauthorized, "the sign-in provider refused the sign-in ("+safeCode(e)+")")
		return
	}
	f, ok := o.readFlow(r)
	if !ok || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(f.State)) != 1 {
		writeProblem(w, http.StatusBadRequest, "this sign-in did not start here, or took too long; start it again")
		return
	}
	id, err := o.verify(ctx, q.Get("code"), f)
	if err != nil {
		if _, ok := errors.AsType[providerError](err); ok {
			o.sessions.logger.ErrorContext(ctx, "the OIDC provider failed", "error", err)
			writeProblem(w, http.StatusBadGateway, "the sign-in provider failed; try again, or sign in with a password")
			return
		}
		o.sessions.logger.WarnContext(ctx, "an OIDC ID token failed verification", "error", err)
		writeProblem(w, http.StatusUnauthorized, "the sign-in could not be verified")
		return
	}

	now := o.sessions.now()
	u, err := o.sessions.db.SignInWithIdentity(ctx, id, now)
	if msg, refused := refusal(err); refused {
		// The reason, never the address: the log is no place for it.
		o.sessions.logger.InfoContext(ctx, "an OIDC sign-in was refused", "issuer", id.Issuer, "reason", err.Error())
		writeProblem(w, http.StatusForbidden, msg)
		return
	}
	if err != nil {
		o.sessions.logger.ErrorContext(ctx, "linking an OIDC identity failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "signing in failed")
		return
	}
	cookie, _, err := o.sessions.start(ctx, u, now, "oidc", func(sess store.Session) error {
		return o.sessions.db.CreateSession(ctx, sess)
	})
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "signing in failed")
		return
	}
	http.SetCookie(w, &cookie)
	// Checked again: the seal vouches for the cookie, this for the target.
	http.Redirect(w, r, safeReturnTo(f.ReturnTo), http.StatusSeeOther)
}

// refusal returns what to tell a person whose identity cannot be linked,
// and whether err is such a refusal.
func refusal(err error) (string, bool) {
	switch {
	case errors.Is(err, store.ErrEmailNotVerified):
		return "the sign-in provider does not confirm that your email address is verified, so it cannot be linked " +
			"to an account here; ask the admin (see the README on single sign-on)", true
	case errors.Is(err, store.ErrAlreadyLinked):
		return "your account here is linked to another account at this sign-in provider; ask the admin", true
	case errors.Is(err, store.ErrEmailNotLinkable):
		return "the sign-in provider gives your email address in a form that cannot be matched to an account here " +
			"(only ASCII letter case may differ); ask the admin", true
	case errors.Is(err, store.ErrNoAccount):
		return "there is no account for you on this instance; ask its admin to add you", true
	}
	return "", false
}

// readFlow returns the flow the request's flow cookie carries, if it is
// sealed with this instance's key and has not expired.
func (o *oidcLogin) readFlow(r *http.Request) (flow, bool) {
	c, err := r.Cookie(o.cookieName())
	if err != nil {
		return flow{}, false
	}
	value, err := auth.Open(o.secret, sealPurpose, c.Value)
	if err != nil {
		return flow{}, false
	}
	var f flow
	if err := json.Unmarshal(value, &f); err != nil || !o.sessions.now().Before(f.ExpiresAt) || f.State == "" {
		return flow{}, false
	}
	return f, true
}

// providerError marks a failure on the provider's side (or on the way to
// it), as opposed to a token that does not verify.
type providerError struct{ err error }

func (e providerError) Error() string { return e.err.Error() }
func (e providerError) Unwrap() error { return e.err }

// verify trades code for tokens and returns the identity the ID token
// vouches for.
func (o *oidcLogin) verify(ctx context.Context, code string, f flow) (store.Identity, error) {
	p, err := o.getProvider(ctx)
	if err != nil {
		return store.Identity{}, providerError{err}
	}
	tok, err := o.oauth2Config(p).Exchange(o.ctx(ctx), code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return store.Identity{}, providerError{err}
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return store.Identity{}, providerError{errors.New("the token response has no id_token")}
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: o.cfg.ClientID}).Verify(o.ctx(ctx), raw)
	if err != nil {
		// go-oidc does not wrap a failure to fetch the keys, so ask the key
		// endpoint directly: if it does not answer, the provider is at
		// fault, not the token.
		if !o.keysReachable(ctx, p) {
			return store.Identity{}, providerError{fmt.Errorf("%w (the provider's keys cannot be fetched)", err)}
		}
		return store.Identity{}, err
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(f.Nonce)) != 1 {
		return store.Identity{}, errors.New("the ID token's nonce is not this sign-in's")
	}
	var claims struct {
		Email         string   `json:"email"`
		EmailVerified flexBool `json:"email_verified"`
	}
	if err := idt.Claims(&claims); err != nil {
		return store.Identity{}, err
	}
	email, linkable := linkableEmail(claims.Email)
	return store.Identity{
		Issuer: idt.Issuer, Subject: idt.Subject,
		Email: email, EmailNotLinkable: !linkable, EmailVerified: bool(claims.EmailVerified),
	}, nil
}

// keysReachable reports whether the provider's key endpoint answers.
func (o *oidcLogin) keysReachable(ctx context.Context, p *oidc.Provider) bool {
	var doc struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := p.Claims(&doc); err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(o.ctx(ctx), http.MethodGet, doc.JWKSURI, http.NoBody)
	if err != nil {
		return false
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// flexBool reads a boolean claim sent as true or false, or, as some
// providers do, as the string "true" or "false". Anything else is false.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch strings.ToLower(strings.Trim(string(data), `"`)) {
	case "true":
		*b = true
	default:
		*b = false
	}
	return nil
}

// linkableEmail returns the provider's address normalized and true, if
// normalizing changes nothing but ASCII letter case; otherwise "" and
// false, so the identity cannot be linked by email. Case folding maps
// some other characters onto ASCII (the Kelvin sign onto k), and an
// address written that way at the provider must not land on another
// user's account.
func linkableEmail(raw string) (string, bool) {
	email := user.NormalizeEmail(raw)
	if asciiLower(strings.TrimSpace(raw)) != email {
		return "", false
	}
	return email, true
}

func asciiLower(s string) string {
	return strings.Map(func(r rune) rune {
		if 'A' <= r && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, s)
}

// safeReturnTo keeps a return_to that is a path on this instance, in
// UTF-8, and replaces anything else with "/", so the sign-in cannot send
// the browser to another site (and JSON does not triple stray bytes into
// U+FFFD).
func safeReturnTo(s string) string {
	// One slash first, and no slash or backslash after it: a browser reads
	// "//host" and "/\host" as another site.
	if s == "" || s[0] != '/' || (len(s) > 1 && (s[1] == '/' || s[1] == '\\')) {
		return "/"
	}
	if len(s) > maxReturnTo || !utf8.ValidString(s) || strings.ContainsAny(s, "\\\r\n\t") {
		return "/"
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	return s
}

// errorCodes are the error codes a provider sends back to the redirect
// URI: RFC 6749 §4.1.2.1 and OpenID Connect Core §3.1.2.6.
var errorCodes = []string{
	"invalid_request", "unauthorized_client", "access_denied", "unsupported_response_type", "invalid_scope",
	"server_error", "temporarily_unavailable", "interaction_required", "login_required",
	"account_selection_required", "consent_required", "invalid_request_uri", "invalid_request_object",
	"request_not_supported", "request_uri_not_supported", "registration_not_supported",
}

// knownErrorCode returns s if it is one of errorCodes, and "other" if not.
// The result is always one of the list's own strings, never s itself.
func knownErrorCode(s string) string {
	for _, c := range errorCodes {
		if s == c {
			return c
		}
	}
	return "other"
}

// safeCode keeps an OAuth error code (RFC 6749: printable ASCII) short and
// free of anything but letters, digits and _.-, before it goes into a
// response.
func safeCode(s string) string {
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || r == '.' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') {
			return r
		}
		return -1
	}, s)
}

type methodsBody struct {
	Password bool        `json:"password" doc:"Signing in with an email and a password works."`
	OIDC     *oidcMethod `json:"oidc,omitempty" doc:"The OpenID Connect provider, when the instance has one."`
}

type oidcMethod struct {
	Name     string `json:"name" example:"Authentik" doc:"What to call the provider on the sign-in page."`
	LoginURL string `json:"login_url" example:"/api/v1/auth/oidc/login" doc:"Open it in the browser (add ?return_to=/a/path) to sign in through the provider."`
}

type methodsOutput struct {
	Body methodsBody
}

func registerMethods(a huma.API, o *oidcLogin) {
	huma.Register(a, huma.Operation{
		OperationID: "get-auth-methods",
		Method:      http.MethodGet,
		Path:        Prefix + "/auth/methods",
		Summary:     "Get the ways to sign in",
		Description: "What the sign-in page offers: a password, and an OpenID Connect provider when one is configured.",
		Tags:        []string{"auth"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, func(context.Context, *struct{}) (*methodsOutput, error) {
		out := &methodsOutput{Body: methodsBody{Password: true}}
		if o != nil {
			out.Body.OIDC = &oidcMethod{Name: o.cfg.Name, LoginURL: oidcLoginPath}
		}
		return out, nil
	})
}
