package store_test

import (
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

var t0 = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

func session(id string, userID uuid.UUID, created time.Time, lifetime time.Duration) store.Session {
	return store.Session{ID: id, UserID: userID, CreatedAt: created, LastSeenAt: created, ExpiresAt: created.Add(lifetime)}
}

// withSession returns a database with ana and her session s1, created at
// t0 for a day.
func withSession(t *testing.T) (*store.DB, uuid.UUID) {
	t.Helper()

	db := storetest.Open(t)
	u := newUser("ana@example.com")
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.CreateSession(t.Context(), session("s1", u.ID, t0, 24*time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return db, u.ID
}

func TestSessionUser(t *testing.T) {
	t.Parallel()

	db, userID := withSession(t)
	s, got, err := db.SessionUser(t.Context(), "s1", t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("SessionUser: %v", err)
	}
	want := newUser("ana@example.com")
	if got.ID != userID || got.Email != want.Email || got.Role != want.Role || got.TimeZone != want.TimeZone {
		t.Errorf("session user = %+v, want %+v", got, want)
	}
	if s.UserID != userID || !s.CreatedAt.Equal(t0) || !s.LastSeenAt.Equal(t0) || !s.ExpiresAt.Equal(t0.Add(24*time.Hour)) {
		t.Errorf("session = %+v", s)
	}
	if _, _, err := db.SessionUser(t.Context(), "nope", t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SessionUser of an unknown id = %v, want ErrNotFound", err)
	}
}

func TestTouchSession(t *testing.T) {
	t.Parallel()

	db, _ := withSession(t)
	if err := db.TouchSession(t.Context(), "s1", t0.Add(2*time.Hour)); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	if s, _, _ := db.SessionUser(t.Context(), "s1", t0.Add(2*time.Hour)); !s.LastSeenAt.Equal(t0.Add(2 * time.Hour)) {
		t.Errorf("last seen = %v after touch, want %v", s.LastSeenAt, t0.Add(2*time.Hour))
	}
}

// The instant a session expires, it is gone.
func TestSessionExpiry(t *testing.T) {
	t.Parallel()

	db, _ := withSession(t)
	if _, _, err := db.SessionUser(t.Context(), "s1", t0.Add(24*time.Hour-time.Second)); err != nil {
		t.Errorf("SessionUser a second before expiry = %v", err)
	}
	if _, _, err := db.SessionUser(t.Context(), "s1", t0.Add(24*time.Hour)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SessionUser at expiry = %v, want ErrNotFound", err)
	}
}

func TestDeleteSession(t *testing.T) {
	t.Parallel()

	db, _ := withSession(t)
	if err := db.DeleteSession(t.Context(), "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, _, err := db.SessionUser(t.Context(), "s1", t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SessionUser after delete = %v, want ErrNotFound", err)
	}
	if err := db.DeleteSession(t.Context(), "s1"); err != nil {
		t.Errorf("deleting a deleted session = %v, want nil", err)
	}
}

func sessionCount(t *testing.T, db *store.DB) int {
	t.Helper()

	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM sessions").Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return n
}

// A new session clears out the ones that have ended.
func TestCreateSessionRemovesExpired(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	u := newUser("ana@example.com")
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for _, s := range []store.Session{
		session("ended", u.ID, t0, time.Hour),
		session("live", u.ID, t0, 48*time.Hour),
	} {
		if err := db.CreateSession(t.Context(), s); err != nil {
			t.Fatalf("CreateSession %s: %v", s.ID, err)
		}
	}
	if err := db.CreateSession(t.Context(), session("new", u.ID, t0.Add(2*time.Hour), time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if n := sessionCount(t, db); n != 2 {
		t.Errorf("%d sessions, want 2 (live and new)", n)
	}
	if _, _, err := db.SessionUser(t.Context(), "live", t0.Add(2*time.Hour)); err != nil {
		t.Errorf("the live session is gone: %v", err)
	}
}

func TestSessionNeedsAUser(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	if err := db.CreateSession(t.Context(), session("s", uuid.NewV7(), t0, time.Hour)); err == nil {
		t.Error("a session for a user that does not exist was stored")
	}
}

func TestUserCredentials(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	withPassword, without := newUser("ana@example.com"), newUser("ivo@example.com")
	if err := db.CreateUser(t.Context(), withPassword, "$argon2id$hash"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.CreateUser(t.Context(), without, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	u, hash, err := db.UserCredentials(t.Context(), "ana@example.com")
	if err != nil || u.ID != withPassword.ID || hash != "$argon2id$hash" {
		t.Errorf("UserCredentials(ana) = %v, %q, %v", u.ID, hash, err)
	}
	if u, hash, err := db.UserCredentials(t.Context(), "ivo@example.com"); err != nil || u.ID != without.ID || hash != "" {
		t.Errorf("UserCredentials(ivo) = %v, %q, %v; want no hash", u.ID, hash, err)
	}
	if _, _, err := db.UserCredentials(t.Context(), "nobody@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("UserCredentials(nobody) = %v, want ErrNotFound", err)
	}
}

func TestSetPasswordHash(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	u := newUser("ana@example.com")
	if err := db.CreateUser(t.Context(), u, "old"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for _, id := range []string{"a", "b"} {
		if err := db.CreateSession(t.Context(), session(id, u.ID, t0, time.Hour)); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}
	later := t0.Add(time.Minute)

	// A rehash keeps the sessions.
	if err := db.SetPasswordHash(t.Context(), u.ID, "rehashed", later, false); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	if n := sessionCount(t, db); n != 2 {
		t.Errorf("%d sessions after a rehash, want 2", n)
	}
	// A reset ends them.
	if err := db.SetPasswordHash(t.Context(), u.ID, "reset", later, true); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	if n := sessionCount(t, db); n != 0 {
		t.Errorf("%d sessions after a reset, want 0", n)
	}
	got, hash, err := db.UserCredentials(t.Context(), "ana@example.com")
	if err != nil || hash != "reset" || !got.UpdatedAt.Equal(later) {
		t.Errorf("after the reset: hash %q, updated %v, err %v", hash, got.UpdatedAt, err)
	}

	if err := db.SetPasswordHash(t.Context(), uuid.NewV7(), "x", later, true); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetPasswordHash for nobody = %v, want ErrNotFound", err)
	}
	if err := db.SetPasswordHash(t.Context(), u.ID, "", later, true); err == nil {
		t.Error("an empty hash was stored")
	}
}
