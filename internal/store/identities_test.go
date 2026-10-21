package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

const issuer = "https://auth.example.com/application/o/tdeemon/"

// identityCount counts the stored identities. It takes ctx so that a
// cleanup can call it: t.Context is done by the time cleanups run.
func identityCount(ctx context.Context, t *testing.T, db *store.DB) int {
	t.Helper()
	var n int
	if err := db.Read().QueryRowContext(ctx, "SELECT count(*) FROM user_identities").Scan(&n); err != nil {
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
	for _, tc := range []struct {
		name string
		id   store.Identity
	}{
		{"another email", store.Identity{Issuer: issuer, Subject: "s1", Email: "ana.new@example.com", EmailVerified: true}},
		{"no email", store.Identity{Issuer: issuer, Subject: "s1"}},
		{"an address that cannot link", store.Identity{Issuer: issuer, Subject: "s1", EmailNotLinkable: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if u, err := db.SignInWithIdentity(t.Context(), tc.id, t0); err != nil || u.ID != ana.ID || u.Email != "ana@example.com" {
				t.Errorf("SignInWithIdentity = %v %q, %v; want ana", u.ID, u.Email, err)
			}
		})
	}
	// After the parallel subtests: none of them linked another identity.
	t.Cleanup(func() {
		if n := identityCount(context.WithoutCancel(t.Context()), t, db); n != 1 {
			t.Errorf("%d identities, want 1", n)
		}
	})
}

func TestSignInWithIdentityRefuses(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	// ana is linked already; ivo is not, so a refusal for him is about the
	// identity, not about a link that exists.
	ana, ivo := newUser("ana@example.com"), newUser("ivo@example.com")
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.CreateUser(t.Context(), ivo, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := db.SignInWithIdentity(t.Context(), store.Identity{Issuer: issuer, Subject: "s1", Email: "ana@example.com", EmailVerified: true}, t0); err != nil {
		t.Fatalf("link ana: %v", err)
	}

	for _, tc := range []struct {
		name string
		id   store.Identity
		want error
	}{
		{"unverified email", store.Identity{Issuer: issuer, Subject: "s2", Email: "ivo@example.com"}, store.ErrEmailNotVerified},
		{"no email", store.Identity{Issuer: issuer, Subject: "s2", EmailVerified: true}, store.ErrNoAccount},
		{"an address that cannot link", store.Identity{Issuer: issuer, Subject: "s2", EmailNotLinkable: true, EmailVerified: true}, store.ErrEmailNotLinkable},
		{"unknown email", store.Identity{Issuer: issuer, Subject: "s2", Email: "eva@example.com", EmailVerified: true}, store.ErrNoAccount},
		// ana already has an identity at this issuer: a second account
		// there with her address does not take hers over.
		{"another subject for a linked user", store.Identity{Issuer: issuer, Subject: "s2", Email: "ana@example.com", EmailVerified: true}, store.ErrAlreadyLinked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := db.SignInWithIdentity(t.Context(), tc.id, t0); !errors.Is(err, tc.want) {
				t.Errorf("SignInWithIdentity = %v, want %v", err, tc.want)
			}
		})
	}
	// After the parallel subtests: no refusal linked anything.
	t.Cleanup(func() {
		if n := identityCount(context.WithoutCancel(t.Context()), t, db); n != 1 {
			t.Errorf("%d identities, want 1", n)
		}
	})
}

// Another provider is a separate identity for the same user.
func TestSignInWithIdentityAtAnotherIssuer(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	ana := newUser("ana@example.com")
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for _, iss := range []string{issuer, "https://other.example.com"} {
		u, err := db.SignInWithIdentity(t.Context(), store.Identity{Issuer: iss, Subject: "s1", Email: "ana@example.com", EmailVerified: true}, t0)
		if err != nil || u.ID != ana.ID {
			t.Errorf("issuer %s = %v, %v; want ana", iss, u.ID, err)
		}
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
