// Package storetest gives tests a fresh, fully migrated database.
package storetest

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/store"
)

// Open returns a new database in t.TempDir() with every migration
// applied. It is a real file opened exactly like in production (WAL,
// separate read and write connections), so tests see the same behaviour,
// and each test gets its own copy. The database is closed when the test
// ends and the directory is removed with it.
func Open(t testing.TB) *store.DB {
	t.Helper()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if err := db.Migrate(t.Context(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}
