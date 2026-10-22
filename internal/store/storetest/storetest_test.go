package storetest_test

import (
	"database/sql"
	"errors"
	"fmt"
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
// and the migrations it records: their number and the last version.
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
	var n, last int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*), max(version) FROM schema_migrations").Scan(&n, &last); err != nil {
		t.Fatal(err)
	}
	return out, fmt.Sprintf("%d migrations, the last %d", n, last)
}

// A database of Open has the schema and the record of migrations a
// migration of a new file makes, and each is the test's own: a write to
// one is not in another. (store.Open refuses one not in WAL mode.)
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
	want, wantMigrations := schema(t, migrated.Read())

	a, b := storetest.Open(t), storetest.Open(t)
	got, migrations := schema(t, a.Read())
	if !slices.Equal(got, want) || migrations != wantMigrations {
		t.Errorf("schema of %d entries and %s, want %d and %s", len(got), migrations, len(want), wantMigrations)
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
