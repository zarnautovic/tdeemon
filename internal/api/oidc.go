package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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

// oidcLogin signs users in through an OpenID Connect provider.
type oidcLogin struct {
	sessions    *sessions
	cfg         config.OIDC
	secret      []byte
	redirectURL string
	client      *http.Client

	// The provider is discovered on first use and kept, so tdeemon starts
	// while the provider is down (the password still works) and tries
	// again on the next sign-in.
	mu       sync.Mutex
	provider *oidc.Provider
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

// ctx returns a context for requests to the provider: with the timeout
// client, and not cancelled with the request, because go-oidc keeps the
// discovery context to fetch the provider's keys later.
func (o *oidcLogin) ctx(ctx context.Context) context.Context {
	return oidc.ClientContext(context.WithoutCancel(ctx), o.client)
}

func (o *oidcLogin) getProvider(ctx context.Context) (*oidc.Provider, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil {
		return o.provider, nil
	}
	p, err := oidc.NewProvider(o.ctx(ctx), o.cfg.Issuer)
	if err != nil {
		return nil, err
	}
	o.provider = p
	return p, nil
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
	value, _ := json.Marshal(f) // A struct of strings and a time always marshals.
	http.SetCookie(w, o.flowCookie(auth.Seal(o.secret, sealPurpose, value), int(flowLifetime.Seconds())))
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
		o.sessions.logger.InfoContext(ctx, "the OIDC provider refused the sign-in", "error", e)
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
	if errors.Is(err, store.ErrNoAccount) {
		o.sessions.logger.InfoContext(ctx, "an OIDC sign-in has no account", "issuer", id.Issuer)
		writeProblem(w, http.StatusForbidden, "there is no account for you on this instance; ask its admin to add you")
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
	http.Redirect(w, r, f.ReturnTo, http.StatusSeeOther)
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
		return store.Identity{}, err
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(f.Nonce)) != 1 {
		return store.Identity{}, errors.New("the ID token's nonce is not this sign-in's")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idt.Claims(&claims); err != nil {
		return store.Identity{}, err
	}
	return store.Identity{
		Issuer: idt.Issuer, Subject: idt.Subject,
		Email: user.NormalizeEmail(claims.Email), EmailVerified: claims.EmailVerified,
	}, nil
}

// safeReturnTo keeps a return_to that is a path on this instance and
// replaces anything else with "/", so the sign-in cannot send the browser
// to another site.
func safeReturnTo(s string) string {
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.ContainsAny(s, "\\\r\n\t") {
		return "/"
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	return s
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
	}, func(context.Context, *struct{}) (*methodsOutput, error) {
		out := &methodsOutput{Body: methodsBody{Password: true}}
		if o != nil {
			out.Body.OIDC = &oidcMethod{Name: o.cfg.Name, LoginURL: oidcLoginPath}
		}
		return out, nil
	})
}
