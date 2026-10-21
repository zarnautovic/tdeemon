package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
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
// the API token in its Authorization header or, without that header, the
// live session its cookie names. A request with neither goes on
// anonymous, and operations that need a user refuse it; one with a bad
// token is refused here (401), since it meant to authenticate.
func (s *sessions) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAPIPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if h := r.Header.Get("Authorization"); h != "" {
			s.authenticateToken(w, r, h, next)
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
			s.logger.ErrorContext(r.Context(), "looking up a session failed", "error", err)
			writeProblem(w, http.StatusInternalServerError, "the session could not be checked")
			return
		}
		if now.Sub(sess.LastSeenAt) >= touchEvery {
			if err := s.db.TouchSession(r.Context(), id, now); err != nil {
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
			"message; a body that breaks the schema answers 400, 413, 415 or 422, and no error detail echoes a value it sent.",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusUnauthorized},
	}, s.login)
	huma.Register(a, huma.Operation{
		OperationID:   "logout",
		Method:        http.MethodPost,
		Path:          Prefix + "/auth/logout",
		Summary:       "Sign out",
		Description:   "Ends the request's web session, if any, and clears the cookie. Signing out twice is fine.",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
	}, s.logout)
	huma.Register(a, huma.Operation{
		OperationID: "get-session",
		Method:      http.MethodGet,
		Path:        Prefix + "/auth/session",
		Summary:     "Get the current session",
		Description: "The signed-in user and when the session ends; 401 without a live session.",
		Tags:        []string{"auth"},
		Errors:      []int{http.StatusUnauthorized},
	}, s.current)
}

func (s *sessions) login(ctx context.Context, in *loginInput) (*sessionOutput, error) {
	u, hash, err := s.db.UserCredentials(ctx, user.NormalizeEmail(in.Body.Email))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.logger.ErrorContext(ctx, "reading credentials failed", "error", err)
		return nil, huma.Error500InternalServerError("signing in failed")
	}
	// hash is "" for an unknown address: VerifyLogin still spends a hash.
	ok, err := auth.VerifyLogin(in.Body.Password, hash)
	if err != nil {
		s.logger.ErrorContext(ctx, "a stored password hash cannot be read", "user_id", u.ID.String(), "error", err)
	}
	if !ok {
		s.logger.InfoContext(ctx, "sign-in failed")
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
		return nil, huma.Error500InternalServerError("signing in failed")
	}
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
		if err := s.db.DeleteSession(ctx, p.SessionID); err != nil {
			s.logger.WarnContext(ctx, "ending the previous session failed", "error", err)
		}
	}
	now = now.Truncate(time.Second)
	token, id := auth.NewSessionToken()
	sess := store.Session{ID: id, UserID: u.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(auth.SessionLifetime)}
	if err := create(sess); err != nil {
		if !errors.Is(err, store.ErrCredentialsChanged) {
			s.logger.ErrorContext(ctx, "starting a session failed", "error", err)
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
	newHash, err := auth.HashPassword(password)
	if err != nil {
		return oldHash
	}
	replaced, err := s.db.ReplacePasswordHash(ctx, u.ID, oldHash, newHash, now)
	if err != nil {
		s.logger.WarnContext(ctx, "updating a password hash failed", "user_id", u.ID.String(), "error", err)
	}
	if !replaced {
		return oldHash
	}
	return newHash
}

func (s *sessions) logout(ctx context.Context, _ *struct{}) (*logoutOutput, error) {
	if p, ok := auth.PrincipalFrom(ctx); ok && p.Via == auth.ViaSession {
		if err := s.db.DeleteSession(ctx, p.SessionID); err != nil {
			s.logger.ErrorContext(ctx, "ending a session failed", "error", err)
			return nil, huma.Error500InternalServerError("signing out failed")
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
