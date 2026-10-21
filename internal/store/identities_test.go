package store_test

import (
	"errors"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

const issuer = "https://auth.example.com/application/o/tdeemon/"

func identityCount(t *testing.T, db *store.DB) int {
	t.Helper()
	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM user_identities").Scan(&n); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	return n
}

func TestSignInWithIdentity(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	ana := newUser("ana@example.com")
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// The first sign-in links by the verified email.
	u, err := db.SignInWithIdentity(t.Context(), store.Identity{Issuer: issuer, Subject: "s1", Email: "ana@example.com", EmailVerified: true}, t0)
	if err != nil || u.ID != ana.ID {
		t.Fatalf("first sign-in = %v, %v; want ana", u.ID, err)
	}
	// Later ones go by the subject, whatever the email says now.
	for _, id := range []store.Identity{
		{Issuer: issuer, Subject: "s1", Email: "ana.new@example.com", EmailVerified: true},
		{Issuer: issuer, Subject: "s1"},
	} {
		if u, err := db.SignInWithIdentity(t.Context(), id, t0); err != nil || u.ID != ana.ID || u.Email != "ana@example.com" {
			t.Errorf("sign-in with %+v = %v %q, %v; want ana", id, u.ID, u.Email, err)
		}
	}
	if n := identityCount(t, db); n != 1 {
		t.Errorf("%d identities, want 1", n)
	}
}

func TestSignInWithIdentityRefuses(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	ana := newUser("ana@example.com")
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := db.SignInWithIdentity(t.Context(), store.Identity{Issuer: issuer, Subject: "s1", Email: "ana@example.com", EmailVerified: true}, t0); err != nil {
		t.Fatalf("link ana: %v", err)
	}

	for _, tc := range []struct {
		name string
		id   store.Identity
	}{
		{"unverified email", store.Identity{Issuer: issuer, Subject: "s2", Email: "ana@example.com"}},
		{"no email", store.Identity{Issuer: issuer, Subject: "s2", EmailVerified: true}},
		{"unknown email", store.Identity{Issuer: issuer, Subject: "s2", Email: "ivo@example.com", EmailVerified: true}},
		// ana already has an identity at this issuer: a second account
		// there with her address does not take hers over.
		{"another subject for a linked user", store.Identity{Issuer: issuer, Subject: "s2", Email: "ana@example.com", EmailVerified: true}},
	} {
		if _, err := db.SignInWithIdentity(t.Context(), tc.id, t0); !errors.Is(err, store.ErrNoAccount) {
			t.Errorf("%s: %v, want ErrNoAccount", tc.name, err)
		}
	}
	if n := identityCount(t, db); n != 1 {
		t.Errorf("%d identities, want 1", n)
	}

	// Another provider is a separate identity for the same user.
	other := store.Identity{Issuer: "https://other.example.com", Subject: "s1", Email: "ana@example.com", EmailVerified: true}
	if u, err := db.SignInWithIdentity(t.Context(), other, t0); err != nil || u.ID != ana.ID {
		t.Errorf("another issuer = %v, %v; want ana", u.ID, err)
	}
}

func TestSignInWithIdentityFailsOnAClosedDatabase(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	_, err := db.SignInWithIdentity(t.Context(), store.Identity{Issuer: issuer, Subject: "s1"}, t0)
	if err == nil || errors.Is(err, store.ErrNoAccount) {
		t.Errorf("SignInWithIdentity on a closed database = %v, want an error that is not ErrNoAccount", err)
	}
}
