// Package storetest gives tests a fresh, fully migrated database.
package storetest

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/store"
)

// Open returns a new database in tb.TempDir() with every migration
// applied. It is a real file opened exactly like in production (WAL,
// separate read and write connections), so tests see the same behaviour,
// and each test gets its own copy. The database is closed when the test
// ends and the directory is removed with it.
func Open(tb testing.TB) *store.DB {
	tb.Helper()

	db, err := store.Open(tb.Context(), filepath.Join(tb.TempDir(), store.FileName))
	if err != nil {
		tb.Fatalf("open test database: %v", err)
	}
	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Errorf("close test database: %v", err)
		}
	})
	if err := db.Migrate(tb.Context(), slog.New(slog.DiscardHandler)); err != nil {
		tb.Fatalf("migrate test database: %v", err)
	}
	return db
}
