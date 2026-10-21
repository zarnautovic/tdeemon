package store_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// withTwoUsers returns a database with ana and ivo.
func withTwoUsers(t *testing.T) (*store.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	db := storetest.Open(t)
	ana, ivo := newUser("ana@example.com"), newUser("ivo@example.com")
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.CreateUser(t.Context(), ivo, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return db, ana.ID, ivo.ID
}

func newToken(userID uuid.UUID, name string, created time.Time) store.APIToken {
	return store.APIToken{ID: uuid.NewV7(), UserID: userID, Name: name, Scopes: "ingest", CreatedAt: created}
}

func TestAPITokenLifecycle(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	tok := newToken(ana, "iPhone", t0)
	if err := db.CreateAPIToken(t.Context(), tok, sid("iphone")); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	got, u, err := db.APITokenUser(t.Context(), sid("iphone"))
	if err != nil || got.ID != tok.ID || got.UserID != ana || got.Name != "iPhone" || got.Scopes != "ingest" ||
		!got.CreatedAt.Equal(t0) || !got.LastUsedAt.IsZero() || u.ID != ana {
		t.Fatalf("APITokenUser = %+v, user %v, %v", got, u.ID, err)
	}
	if _, _, err := db.APITokenUser(t.Context(), sid("nope")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("APITokenUser of an unknown hash = %v, want ErrNotFound", err)
	}

	if err := db.TouchAPIToken(t.Context(), ana, tok.ID, t0.Add(time.Hour)); err != nil {
		t.Fatalf("TouchAPIToken: %v", err)
	}
	if got, _, _ := db.APITokenUser(t.Context(), sid("iphone")); !got.LastUsedAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("last used = %v, want %v", got.LastUsedAt, t0.Add(time.Hour))
	}

	if err := db.RevokeAPIToken(t.Context(), ana, tok.ID); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if _, _, err := db.APITokenUser(t.Context(), sid("iphone")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("APITokenUser after revoking = %v, want ErrNotFound", err)
	}
}

// One user's tokens are not another's to see or revoke.
func TestAPITokensBelongToTheirUser(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	first, second := newToken(ana, "iPhone", t0), newToken(ana, "Claude", t0.Add(time.Minute))
	ivos := newToken(ivo, "Hermes", t0)
	for i, tok := range []store.APIToken{second, first, ivos} {
		if err := db.CreateAPIToken(t.Context(), tok, sid(string(rune('a'+i)))); err != nil {
			t.Fatalf("CreateAPIToken: %v", err)
		}
	}

	list, err := db.APITokens(t.Context(), ana)
	if err != nil || len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Errorf("ana's tokens = %+v, %v; want iPhone then Claude", list, err)
	}
	if list, err := db.APITokens(t.Context(), ivo); err != nil || len(list) != 1 || list[0].ID != ivos.ID {
		t.Errorf("ivo's tokens = %+v, %v; want Hermes only", list, err)
	}
	if err := db.RevokeAPIToken(t.Context(), ana, ivos.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ana revoking ivo's token = %v, want ErrNotFound", err)
	}
	if err := db.RevokeAPIToken(t.Context(), ana, uuid.NewV7()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("revoking an unknown token = %v, want ErrNotFound", err)
	}
	if list, _ := db.APITokens(t.Context(), ivo); len(list) != 1 {
		t.Error("ivo's token is gone after ana tried to revoke it")
	}
	if list, err := db.APITokens(t.Context(), uuid.NewV7()); err != nil || len(list) != 0 {
		t.Errorf("the tokens of nobody = %+v, %v; want none", list, err)
	}
}

// Recording one token's use leaves the others alone, and another user
// cannot record it.
func TestTouchAPITokenTouchesOnlyItsToken(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	for _, tok := range []struct {
		hash string
		t    store.APIToken
	}{{"used", newToken(ana, "iPhone", t0)}, {"other", newToken(ana, "Claude", t0)}, {"ivos", newToken(ivo, "Hermes", t0)}} {
		if err := db.CreateAPIToken(t.Context(), tok.t, sid(tok.hash)); err != nil {
			t.Fatalf("CreateAPIToken: %v", err)
		}
	}
	used, _, _ := db.APITokenUser(t.Context(), sid("used"))
	ivos, _, _ := db.APITokenUser(t.Context(), sid("ivos"))
	if err := db.TouchAPIToken(t.Context(), ana, used.ID, t0.Add(time.Hour)); err != nil {
		t.Fatalf("TouchAPIToken: %v", err)
	}
	if err := db.TouchAPIToken(t.Context(), ana, ivos.ID, t0.Add(time.Hour)); err != nil {
		t.Fatalf("TouchAPIToken of ivo's token as ana: %v", err)
	}
	for hash, want := range map[string]time.Time{"used": t0.Add(time.Hour), "other": {}, "ivos": {}} {
		t.Run(hash, func(t *testing.T) {
			t.Parallel()
			if got, _, err := db.APITokenUser(t.Context(), sid(hash)); err != nil || !got.LastUsedAt.Equal(want) {
				t.Errorf("last used %v, %v; want %v", got.LastUsedAt, err, want)
			}
		})
	}
}

func TestAPITokensTableChecks(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	const insert = `INSERT INTO api_tokens (id, token_hash, user_id, name, scopes, created_at) VALUES (?, ?, ?, ?, ?, '2026-10-21T08:00:00Z')`
	for _, tc := range []struct{ name, id, hash, tokenName, scopes string }{
		{"short id", "1234", sid("a"), "x", "read"},
		{"short hash", uuid.NewV7().String(), "abc", "x", "read"},
		{"empty name", uuid.NewV7().String(), sid("b"), "", "read"},
		{"no scopes", uuid.NewV7().String(), sid("c"), "x", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
				_, err := tx.ExecContext(t.Context(), insert, tc.id, tc.hash, ana.String(), tc.tokenName, tc.scopes)
				return err
			})
			if err == nil || !strings.Contains(err.Error(), "CHECK constraint failed") {
				t.Errorf("insert = %v, want a CHECK constraint error", err)
			}
		})
	}
	// Two tokens cannot share a hash.
	if err := db.CreateAPIToken(t.Context(), newToken(ana, "a", t0), sid("same")); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if err := db.CreateAPIToken(t.Context(), newToken(ana, "b", t0), sid("same")); err == nil {
		t.Error("a second token with the same hash was stored")
	}
}
