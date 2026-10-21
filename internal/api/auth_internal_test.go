package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
		busy   bool
		status int
	}{
		{"no hashing slot in time", t.Context(), true, http.StatusServiceUnavailable},
		{"the client went away", gone, false, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newTestSessions(storetest.Open(t), time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC))
			if tc.busy {
				s.verifyLogin = func(context.Context, string, string) (bool, error) { return false, auth.ErrBusy }
			}
			in := &loginInput{}
			in.Body.Email, in.Body.Password = "ana@example.com", "correct horse battery staple"
			_, err := s.login(tc.ctx, in)
			se, ok := errors.AsType[huma.StatusError](err)
			if !ok || se.GetStatus() != tc.status {
				t.Errorf("login = %v, want %d", err, tc.status)
			}
			if he, ok := errors.AsType[huma.HeadersError](err); tc.busy && (!ok || he.GetHeaders().Get("Retry-After") == "") {
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
