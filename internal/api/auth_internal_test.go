package api

import (
	"context"
	"errors"
	"log/slog"
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
				if hash, err = auth.HashPassword(password); err != nil {
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
	hash, err := auth.HashPassword("correct horse battery staple")
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
		h, err := auth.HashPassword(password)
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
