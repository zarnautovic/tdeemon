package storetest_test

import (
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// schema returns the SQL of every table, index, view and trigger of db,
// and its journal mode.
func schema(t *testing.T, db *sql.DB) ([]string, string) {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT coalesce(sql, name) FROM sqlite_schema ORDER BY type, name")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	return out, mode
}

// A database of Open has the schema a migration of a new file makes, in
// WAL mode, and each is the test's own: a write to one is not in another.
func TestOpen(t *testing.T) {
	t.Parallel()

	migrated, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrated.Close() })
	if err := migrated.Migrate(t.Context(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	want, _ := schema(t, migrated.Read())

	a, b := storetest.Open(t), storetest.Open(t)
	got, mode := schema(t, a.Read())
	if !slices.Equal(got, want) || mode != "wal" {
		t.Errorf("schema of %d entries in %s mode, want the migrated one of %d in wal", len(got), mode, len(want))
	}
	now := time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)
	ana := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleAdmin, Locale: "hr",
		TimeZone: "Europe/Zagreb", DayStartHour: 5, CreatedAt: now, UpdatedAt: now,
	}
	if err := a.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.UserByEmail(t.Context(), ana.Email); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another database has the user: %v", err)
	}
}
