package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// newTestSessions is the sessions of a plain-HTTP instance at now.
func newTestSessions(db *store.DB, now time.Time) *sessions {
	return newSessions(Config{
		DB: db, Logger: slog.New(slog.DiscardHandler), BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
		Now: func() time.Time { return now },
	})
}

// A password reset that commits while a sign-in with the old password is
// between its check and its writes must win: no session, no rehash over
// the new password.
func TestLoginLosesToAConcurrentReset(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, hash string }{
		{"current cost", ""},
		// Reference hash (argon2-cffi 25.1.0, m=8, t=1, p=1) of the
		// password: the sign-in would rehash it.
		{"old cost", "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			const password = "correct horse battery staple"
			hash := tc.hash
			if hash == "" {
				var err error
				if hash, err = auth.HashPassword(t.Context(), password); err != nil {
					t.Fatalf("HashPassword: %v", err)
				}
			}
			db := storetest.Open(t)
			now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
			u := user.User{
				ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser,
				Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
			}
			if err := db.CreateUser(t.Context(), u, hash); err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			s := newTestSessions(db, now)
			s.afterVerify = func(ctx context.Context) {
				if err := db.SetPasswordHash(ctx, u.ID, "reset-hash", now); err != nil {
					t.Errorf("reset: %v", err)
				}
			}

			in := &loginInput{}
			in.Body.Email, in.Body.Password = "ana@example.com", password
			_, err := s.login(t.Context(), in)
			if se, ok := errors.AsType[huma.StatusError](err); !ok || se.GetStatus() != 401 {
				t.Errorf("login during a reset = %v, want 401", err)
			}
			var sessionsLeft int
			if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM sessions").Scan(&sessionsLeft); err != nil {
				t.Fatalf("count sessions: %v", err)
			}
			if sessionsLeft != 0 {
				t.Errorf("%d sessions outlived the reset", sessionsLeft)
			}
			if _, h, err := db.UserCredentials(t.Context(), "ana@example.com"); err != nil || h != "reset-hash" {
				t.Errorf("hash after the sign-in = %q, %v; want the reset's", h, err)
			}
		})
	}
}

// Without a reset the hook changes nothing: the same sign-in succeeds.
func TestLoginWithoutAReset(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	hash, err := auth.HashPassword(t.Context(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	u := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(t.Context(), u, hash); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	called := false
	s := newTestSessions(db, now)
	s.afterVerify = func(context.Context) { called = true }
	in := &loginInput{}
	in.Body.Email, in.Body.Password = "ana@example.com", "correct horse battery staple"
	out, err := s.login(t.Context(), in)
	if err != nil || !called || out.SetCookie.Value == "" || !strings.HasPrefix(out.SetCookie.Name, "tdeemon_session") {
		t.Errorf("login = %+v, %v (hook called %v); want a session", out, err, called)
	}
}

// Two sign-ins at once with a password whose hash needs a rehash: the one
// that loses the race to store its new hash goes on with the winner's,
// which takes the same password, instead of failing as if a reset had
// come in between.
func TestLoginAfterAnotherSignInRehashed(t *testing.T) {
	t.Parallel()

	const password = "correct horse battery staple"
	// Reference hash (argon2-cffi 25.1.0, m=8, t=1, p=1) of the password.
	const old = "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE"
	db := storetest.Open(t)
	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	u := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(t.Context(), u, old); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	s := newTestSessions(db, now)
	var winner string
	s.afterVerify = func(ctx context.Context) {
		h, err := auth.HashPassword(ctx, password)
		if err != nil {
			t.Errorf("HashPassword: %v", err)
			return
		}
		if ok, err := db.ReplacePasswordHash(ctx, u.ID, old, h, now); err != nil || !ok {
			t.Errorf("the other sign-in's rehash: %v, %v", ok, err)
		}
		winner = h
	}

	in := &loginInput{}
	in.Body.Email, in.Body.Password = "ana@example.com", password
	out, err := s.login(t.Context(), in)
	if err != nil || out.SetCookie.Value == "" {
		t.Fatalf("login = %+v, %v; want a session", out, err)
	}
	if _, h, err := db.UserCredentials(t.Context(), "ana@example.com"); err != nil || h != winner {
		t.Errorf("hash after both sign-ins = %q, %v; want the winner's", h, err)
	}
}

// A sign-in that was not checked gives its attempts back, for it is no
// failure: 503 with Retry-After when no hashing slot came free in time,
// 500 when the client went away before the credentials were read.
func TestLoginNotCheckedGivesBack(t *testing.T) {
	t.Parallel()

	gone, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tc := range []struct {
		name   string
		ctx    context.Context //nolint:containedctx // A table of contexts to sign in with.
		verify error           // what verifyLogin returns; nil for the real one
		status int
	}{
		{"no hashing slot in time", t.Context(), auth.ErrBusy, http.StatusServiceUnavailable},
		{"the client went away while it waited for a slot", t.Context(), context.Canceled, http.StatusServiceUnavailable},
		{"the client went away before the credentials were read", gone, nil, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newTestSessions(storetest.Open(t), time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC))
			if tc.verify != nil {
				s.verifyLogin = func(context.Context, string, string) (bool, error) { return false, tc.verify }
			}
			in := &loginInput{}
			in.Body.Email, in.Body.Password = "ana@example.com", "correct horse battery staple"
			_, err := s.login(tc.ctx, in)
			se, ok := errors.AsType[huma.StatusError](err)
			if !ok || se.GetStatus() != tc.status {
				t.Errorf("login = %v, want %d", err, tc.status)
			}
			if he, ok := errors.AsType[huma.HeadersError](err); tc.status == http.StatusServiceUnavailable && (!ok || he.GetHeaders().Get("Retry-After") == "") {
				t.Errorf("the 503 has no Retry-After: %v", err)
			}
			for _, l := range []*limiter{s.signInByAddr, s.signInByAccount} {
				for _, f := range l.keys {
					if f.n != 0 {
						t.Errorf("%d attempts kept, want 0", f.n)
					}
				}
			}
		})
	}
}

// A client that leaves while its password is being checked gets no
// attempt back: the hash ran, and the attempt counts as the failure it
// was, or one address could keep the hashing slots busy for free.
func TestLoginCheckedAfterTheClientLeftCounts(t *testing.T) {
	t.Parallel()

	s := newTestSessions(storetest.Open(t), time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC))
	ctx, cancel := context.WithCancel(t.Context())
	s.verifyLogin = func(context.Context, string, string) (bool, error) {
		cancel() // the client leaves while the hash runs
		return false, nil
	}
	in := &loginInput{}
	in.Body.Email, in.Body.Password = "ana@example.com", "wrong password, surely"
	_, err := s.login(ctx, in)
	if se, ok := errors.AsType[huma.StatusError](err); !ok || se.GetStatus() != http.StatusUnauthorized {
		t.Errorf("login = %v, want 401", err)
	}
	for name, l := range map[string]*limiter{"address": s.signInByAddr, "account": s.signInByAccount} {
		kept := 0
		for _, f := range l.keys {
			kept += f.n
		}
		if kept != 1 {
			t.Errorf("%s: %d attempts kept, want 1", name, kept)
		}
	}
}

// A client that leaves while its session or API token is being looked
// up, or its new session written, logs at debug level: it says nothing
// about the server, and a client can do it at will.
func TestGoneClientsLogAtDebug(t *testing.T) {
	t.Parallel()

	gone, cancel := context.WithCancel(t.Context())
	cancel()
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for name, run := range map[string]func(s *sessions){
		"a session lookup": func(s *sessions) {
			r := httptest.NewRequestWithContext(gone, http.MethodGet, "/api/v1/auth/session", http.NoBody)
			r.AddCookie(&http.Cookie{Name: "tdeemon_session", Value: "token"}) //nolint:gosec // G124: a request's cookie.
			s.authenticate(next).ServeHTTP(httptest.NewRecorder(), r)
		},
		"a token lookup": func(s *sessions) {
			r := httptest.NewRequestWithContext(gone, http.MethodGet, "/api/v1/auth/whoami", http.NoBody)
			s.authenticateToken(httptest.NewRecorder(), r, auth.TokenPrefix+"token", next)
		},
		"a session write": func(s *sessions) {
			_, _, _ = s.start(gone, user.User{ID: uuid.NewV7()}, s.now(), "password", func(store.Session) error { return context.Canceled })
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var log bytes.Buffer
			s := newSessions(Config{
				DB: storetest.Open(t), BaseURL: &url.URL{Scheme: "http", Host: "localhost"}, Now: time.Now,
				Logger: slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})),
			})
			run(s)
			if !strings.Contains(log.String(), "level=DEBUG") || strings.Contains(log.String(), "level=ERROR") {
				t.Errorf("log = %q, want the failure at debug level only", log.String())
			}
		})
	}
}
