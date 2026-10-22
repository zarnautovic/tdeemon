// Package storetest gives tests a fresh, fully migrated database.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/store"
)

// template is a database with every migration applied, as the bytes of one
// file: migrated once per test binary, it is copied for each test, as
// migrating takes about two seconds under the race detector.
var template struct {
	once  sync.Once
	bytes []byte
	err   error
}

// Open returns a new database in tb.TempDir() with every migration
// applied. It is a real file opened exactly like in production (WAL,
// separate read and write connections), so tests see the same behaviour,
// and each test gets its own copy, of a database migrated once for the
// test binary. The database is closed when the test ends and the
// directory is removed with it.
func Open(tb testing.TB) *store.DB {
	tb.Helper()

	template.once.Do(func() { template.bytes, template.err = migrated() })
	if template.err != nil {
		tb.Fatalf("migrate the template database: %v", template.err)
	}
	path := filepath.Join(tb.TempDir(), store.FileName)
	if err := os.WriteFile(path, template.bytes, 0o600); err != nil {
		tb.Fatalf("copy the template database: %v", err)
	}
	db, err := store.Open(tb.Context(), path)
	if err != nil {
		tb.Fatalf("open test database: %v", err)
	}
	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Errorf("close test database: %v", err)
		}
	})
	return db
}

// migrated returns a database with every migration applied, as the bytes
// of one file, and leaves nothing on disk. Closed, the database has its
// write-ahead log checkpointed into the file and removed, so the file is
// the whole database.
func migrated() ([]byte, error) {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "tdeemon-storetest-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, store.FileName)
	db, err := store.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(db.Migrate(ctx, slog.New(slog.DiscardHandler)), db.Close()); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path + "-wal"); err == nil && info.Size() > 0 {
		return nil, fmt.Errorf("a write-ahead log of %d bytes is left beside the database", info.Size())
	}
	return os.ReadFile(path) //nolint:gosec // G304: a file this function named, in its own temporary directory
}
