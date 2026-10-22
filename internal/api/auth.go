package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/ingest"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// touchEvery is how stale a session's last_seen_at may get before a
// request updates it: often enough to show when a session was last used,
// rarely enough that reads do not turn into writes.
const touchEvery = time.Hour

// sessions signs users in and out and finds the session a request
// carries.
type sessions struct {
	db     *store.DB
	logger *slog.Logger
	now    func() time.Time
	// secure is true when the instance is served over HTTPS.
	secure bool
	// afterVerify, for tests, runs once a sign-in has checked the password
	// and before it writes anything.
	afterVerify func(context.Context)
	// verifyLogin is auth.VerifyLogin; tests replace it.
	verifyLogin func(ctx context.Context, password, encoded string) (bool, error)

	// Failures per client address and per account (limit.go).
	signInByAddr, signInByAccount, badTokens *limiter

	// instance holds the instance's limits of a plan.
	instance limits.Limits
	// ingestArchive keeps the bodies of the ingest, when not nil.
	ingestArchive *ingest.Archive
}

func newSessions(cfg Config) *sessions {
	return &sessions{
		db: cfg.DB, logger: cfg.Logger, now: cfg.Now, secure: cfg.BaseURL.Scheme == "https",
		verifyLogin:     auth.VerifyLogin,
		signInByAddr:    newLimiter(signInFailures, signInWindow, cfg.Now),
		signInByAccount: newLimiter(signInFailures, signInWindow, cfg.Now),
		badTokens:       newLimiter(badTokenFailures, badTokenWindow, cfg.Now),
		instance:        cfg.Limits,
		ingestArchive:   cfg.IngestArchive,
	}
}

// accountKey is what the sign-in limit counts an account under: a hash of
// the normalized address, so the limiter keeps no addresses and a key's
// size does not depend on what was typed.
func accountKey(email string) string {
	sum := sha256.Sum256([]byte(email))
	return string(sum[:])
}

// reserveSignIn reserves a sign-in attempt for the address and for the
// account (limit.go), or refuses one from an address, or for an account,
// with too many failed sign-ins lately: 429 with Retry-After. The
// attempts count as failures until giveBack returns them.
func (s *sessions) reserveSignIn(ctx context.Context, addr, account string) (signInAttempt, error) {
	var a signInAttempt
	var wait time.Duration
	a.addr, wait, a.ok = s.signInByAddr.take(addr)
	if a.ok {
		if a.account, wait, a.ok = s.signInByAccount.take(account); !a.ok {
			s.signInByAddr.release(a.addr)
		}
	}
	if a.ok {
		return a, nil
	}
	s.logger.DebugContext(ctx, "sign-in refused: too many failed sign-ins", "addr", addr)
	return a, huma.ErrorWithHeaders(huma.NewError(http.StatusTooManyRequests, "too many failed sign-ins; try again later"), retryAfter(wait))
}

// signInAttempt is what reserveSignIn reserved.
type signInAttempt struct {
	addr, account reservation
	ok            bool
}

// giveBack returns the attempts of a sign-in that did not fail.
func (s *sessions) giveBack(a signInAttempt) {
	s.signInByAddr.release(a.addr)
	s.signInByAccount.release(a.account)
}

// logLevel is the level to log a request that failed with err at: debug
// when the client went away (it says nothing about the server, and a
// client can do it at will), warn when hashing was busy, error otherwise.
func logLevel(err error) slog.Level {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return slog.LevelDebug
	case errors.Is(err, auth.ErrBusy):
		return slog.LevelWarn
	}
	return slog.LevelError
}

// errBusy answers a sign-in that found no hashing slot in time (or whose
// client went away).
func errBusy() error {
	return huma.ErrorWithHeaders(huma.NewError(http.StatusServiceUnavailable, "too many sign-ins at once; try again shortly"),
		http.Header{"Retry-After": {"5"}})
}

// cookieName is the session cookie's name. Over HTTPS it carries the
// __Host- prefix, which browsers accept only with Secure, Path=/ and no
// Domain, so no other host, a subdomain included, can set or replace it.
// Plain HTTP (a test instance on localhost) cannot use the prefix.
func (s *sessions) cookieName() string {
	if s.secure {
		return "__Host-tdeemon_session"
	}
	return "tdeemon_session"
}

// cookie is the session cookie for token, set at now, ending at expires.
func (s *sessions) cookie(token string, now, expires time.Time) http.Cookie {
	//nolint:gosec // G124: Secure only over HTTPS, where the __Host- prefix also demands it; over plain HTTP a browser would drop a Secure cookie.
	return http.Cookie{
		Name:     s.cookieName(),
		Value:    token,
		Path:     "/",
		Expires:  expires.UTC(),
		MaxAge:   int(expires.Sub(now).Seconds()),
		Secure:   s.secure,
		HttpOnly: true,
		// Lax keeps the cookie off cross-site POSTs (a CSRF defence next to
		// CrossOriginProtection) and still sends it when a link from
		// elsewhere opens the app.
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *sessions) clearedCookie() http.Cookie {
	now := s.now()
	c := s.cookie("", now, now) //nolint:gosec // G124: the same attributes as cookie, which says why.
	c.Expires = time.Unix(0, 0).UTC()
	c.MaxAge = -1
	return c
}

// authenticate attaches the principal of an API request to its context:
// the API token in its Authorization header (the Bearer scheme) or,
// without one, the live session its cookie names. A request with neither
// goes on anonymous, and operations that need a user refuse it; one with
// a bad token is refused here (401), since it meant to authenticate.
func (s *sessions) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAPIPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if token, ok := bearerToken(r.Header.Get("Authorization")); ok {
			s.authenticateToken(w, r, token, next)
			return
		}
		c, err := r.Cookie(s.cookieName())
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		now := s.now()
		id := auth.SessionID(c.Value)
		sess, u, err := s.db.SessionUser(r.Context(), id, now)
		if errors.Is(err, store.ErrNotFound) {
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			s.logger.Log(r.Context(), logLevel(err), "looking up a session failed", "error", err)
			writeProblem(w, http.StatusInternalServerError, "the session could not be checked")
			return
		}
		if now.Sub(sess.LastSeenAt) >= touchEvery {
			if err := s.db.TouchSession(r.Context(), u.ID, id, now); err != nil {
				s.logger.WarnContext(r.Context(), "recording session use failed", "error", err)
			}
		}
		ctx := auth.WithPrincipal(r.Context(), auth.Principal{User: u, Via: auth.ViaSession, SessionID: id, SessionExpiresAt: sess.ExpiresAt})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// userBody is a user as the API shows them.
type userBody struct {
	ID           string `json:"id" format:"uuid" doc:"The user's ID."`
	Email        string `json:"email" doc:"The address the user signs in with, in lower case."`
	Name         string `json:"name" doc:"The name the app shows."`
	Role         string `json:"role" enum:"admin,user" doc:"admin manages the instance and its users."`
	Locale       string `json:"locale" enum:"en,hr" doc:"The language of the app."`
	TimeZone     string `json:"timezone" example:"Europe/Zagreb" doc:"An IANA time zone."`
	DayStartHour int    `json:"day_start_hour" minimum:"0" maximum:"23" doc:"The hour at which the user's day starts; a meal before it belongs to the day before."`
}

func newUserBody(u user.User) userBody {
	return userBody{
		ID: u.ID.String(), Email: u.Email, Name: u.Name, Role: string(u.Role),
		Locale: u.Locale, TimeZone: u.TimeZone, DayStartHour: u.DayStartHour,
	}
}

type sessionBody struct {
	User      userBody  `json:"user"`
	ExpiresAt time.Time `json:"expires_at" doc:"When the session ends; then the user signs in again."`
}

type loginInput struct {
	Body struct {
		Email    string `json:"email" maxLength:"254" doc:"The address the user signs in with; case does not matter."`
		Password string `json:"password" minLength:"1" maxLength:"1024"`
	}
}

type sessionOutput struct {
	SetCookie    http.Cookie `header:"Set-Cookie" doc:"The session cookie: __Host-tdeemon_session over HTTPS (tdeemon_session over plain HTTP), HttpOnly, SameSite=Lax, for the session's lifetime."`
	CacheControl string      `header:"Cache-Control" doc:"no-store"`
	Body         sessionBody
}

type logoutOutput struct {
	SetCookie    http.Cookie `header:"Set-Cookie" doc:"Clears the session cookie."`
	CacheControl string      `header:"Cache-Control" doc:"no-store"`
}

type currentSessionOutput struct {
	CacheControl string `header:"Cache-Control" doc:"no-store"`
	Body         sessionBody
}

// errBadLogin is the one answer to every failed sign-in: which part was
// wrong is not the client's to learn.
func errBadLogin() error { return huma.Error401Unauthorized("wrong email or password") }

func (s *sessions) register(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        Prefix + "/auth/login",
		Summary:     "Sign in with a password",
		Description: "Checks the email and password and starts a web session: the response sets the session cookie. " +
			"A session the request already carries ends. A wrong address or password answers 401, always with the same " +
			"message; a body that breaks the schema answers 400, 413, 415 or 422, and no error detail echoes a value it sent. " +
			"After 10 failed sign-ins in 15 minutes from one address, or for one account, sign-ins from there or for it " +
			"answer 429 with Retry-After until the 15 minutes have passed; sign-ins still being checked count too. When too " +
			"many passwords are being checked at once, a sign-in that waited 10 seconds answers 503 with Retry-After.",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable},
	}, s.login)
	huma.Register(a, huma.Operation{
		OperationID:   "logout",
		Method:        http.MethodPost,
		Path:          Prefix + "/auth/logout",
		Summary:       "Sign out",
		Description:   "Ends the request's web session, if any, and clears the cookie. Signing out twice is fine.",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.logout)
	huma.Register(a, huma.Operation{
		OperationID: "get-session",
		Method:      http.MethodGet,
		Path:        Prefix + "/auth/session",
		Summary:     "Get the current session",
		Description: "The signed-in user and when the session ends; 401 without a live session.",
		Tags:        []string{"auth"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.current)
}

func (s *sessions) login(ctx context.Context, in *loginInput) (*sessionOutput, error) {
	email := user.NormalizeEmail(in.Body.Email)
	addr, account := addrKey(clientAddrFrom(ctx)), accountKey(email)
	attempt, err := s.reserveSignIn(ctx, addr, account)
	if err != nil {
		return nil, err
	}
	u, hash, err := s.db.UserCredentials(ctx, email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.giveBack(attempt)
		return nil, internalError("signing in failed", err)
	}
	// hash is "" for an unknown address: VerifyLogin still spends a hash.
	ok, err := s.verifyLogin(ctx, in.Body.Password, hash)
	switch {
	case errors.Is(err, auth.ErrBusy), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// No hash ran, so this is no failure. A client that leaves while
		// its hash runs gets no error here: that attempt counts.
		s.giveBack(attempt)
		s.logger.Log(ctx, logLevel(err), "a sign-in was not checked", "error", err)
		return nil, errBusy()
	case err != nil:
		s.logger.ErrorContext(ctx, "a stored password hash cannot be read", "user_id", u.ID.String(), "error", err)
	}
	if !ok {
		// The attempts stay reserved: they are the failures.
		s.logger.InfoContext(ctx, "sign-in failed", "addr", addr)
		return nil, errBadLogin()
	}
	if s.afterVerify != nil {
		s.afterVerify(ctx)
	}

	now := s.now()
	if auth.NeedsRehash(hash) {
		hash = s.rehash(ctx, u, hash, in.Body.Password, now)
	}
	// The session goes in only while the password is still the one just
	// checked: a reset that committed meanwhile wins.
	cookie, sess, err := s.start(ctx, u, now, "password", func(sess store.Session) error {
		return s.db.CreatePasswordSession(ctx, sess, hash)
	})
	if errors.Is(err, store.ErrCredentialsChanged) {
		s.logger.InfoContext(ctx, "sign-in failed: the password changed meanwhile", "user_id", u.ID.String())
		return nil, errBadLogin()
	}
	if err != nil {
		s.giveBack(attempt)
		return nil, huma.Error500InternalServerError("signing in failed")
	}
	// The account's count starts over; the address gets its attempt back
	// but keeps its failures, or signing in to one's own account would
	// wipe the failures against others.
	s.signInByAddr.release(attempt.addr)
	s.signInByAccount.reset(account)
	return &sessionOutput{
		SetCookie:    cookie,
		CacheControl: "no-store",
		Body:         sessionBody{User: newUserBody(u), ExpiresAt: sess.ExpiresAt.UTC()},
	}, nil
}

// start signs u in: it ends the session the request came with, if any, so
// a token planted before sign-in never becomes a signed-in one, and
// stores a new session with create. Times are kept to the second, as the
// database keeps them. method (password, oidc) only goes to the log.
func (s *sessions) start(ctx context.Context, u user.User, now time.Time, method string,
	create func(store.Session) error,
) (http.Cookie, store.Session, error) {
	if p, ok := auth.PrincipalFrom(ctx); ok && p.Via == auth.ViaSession {
		if err := s.db.DeleteSession(ctx, p.User.ID, p.SessionID); err != nil {
			s.logger.WarnContext(ctx, "ending the previous session failed", "error", err)
		}
	}
	now = now.Truncate(time.Second)
	token, id := auth.NewSessionToken()
	sess := store.Session{ID: id, UserID: u.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(auth.SessionLifetime)}
	if err := create(sess); err != nil {
		if !errors.Is(err, store.ErrCredentialsChanged) {
			s.logger.Log(ctx, logLevel(err), "starting a session failed", "error", err)
		}
		return http.Cookie{}, store.Session{}, err
	}
	s.logger.InfoContext(ctx, "signed in", "user_id", u.ID.String(), "method", method)
	return s.cookie(token, now, sess.ExpiresAt), sess, nil
}

// rehash stores a hash with today's parameters after a sign-in proved the
// password, and returns the hash the user has now. It replaces only
// oldHash: a reset that came in between stays. A password set under
// older, laxer rules cannot be hashed under today's and keeps its old
// hash.
func (s *sessions) rehash(ctx context.Context, u user.User, oldHash, password string, now time.Time) string {
	newHash, err := auth.HashPassword(ctx, password)
	if err != nil {
		return oldHash
	}
	replaced, err := s.db.ReplacePasswordHash(ctx, u.ID, oldHash, newHash, now)
	if err != nil {
		s.logger.WarnContext(ctx, "updating a password hash failed", "user_id", u.ID.String(), "error", err)
	}
	if !replaced {
		// Another sign-in may have rehashed it first: a stored hash that
		// takes this password is the one to go on with. Otherwise a reset
		// came in between, and the sign-in fails on oldHash.
		if _, current, err := s.db.UserCredentials(ctx, u.Email); err == nil && current != oldHash {
			if ok, _ := auth.VerifyPassword(ctx, password, current); ok {
				return current
			}
		}
		return oldHash
	}
	return newHash
}

func (s *sessions) logout(ctx context.Context, _ *struct{}) (*logoutOutput, error) {
	if p, ok := auth.PrincipalFrom(ctx); ok && p.Via == auth.ViaSession {
		if err := s.db.DeleteSession(ctx, p.User.ID, p.SessionID); err != nil {
			return nil, internalError("signing out failed", err)
		}
		s.logger.InfoContext(ctx, "signed out", "user_id", p.User.ID.String())
	}
	return &logoutOutput{SetCookie: s.clearedCookie(), CacheControl: "no-store"}, nil
}

func (s *sessions) current(ctx context.Context, _ *struct{}) (*currentSessionOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	return &currentSessionOutput{
		CacheControl: "no-store",
		Body:         sessionBody{User: newUserBody(p.User), ExpiresAt: p.SessionExpiresAt.UTC()},
	}, nil
}
