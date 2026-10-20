package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/store"
)

func lookup(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), lookup(nil), io.Discard)
	if err == nil {
		t.Fatal("run succeeded without configuration, want an error")
	}
	for _, want := range []string{"invalid configuration", "TDEEMON_BASE_URL", "TDEEMON_SECRET_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestRunStartsAndStops(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "data")
	env := lookup(map[string]string{
		"TDEEMON_LISTEN":     "127.0.0.1:0",
		"TDEEMON_BASE_URL":   "http://localhost",
		"TDEEMON_SECRET_KEY": strings.Repeat("s", 32),
		"TDEEMON_DATA_DIR":   dataDir,
	})

	ctx, cancel := context.WithCancel(t.Context())
	var logs syncBuffer
	done := make(chan error, 1)
	go func() { done <- run(ctx, env, &logs) }()

	// Stop only once run has opened the database and is serving.
	waitFor(t, &logs, `"msg":"listening"`)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancel")
	}

	if info, err := os.Stat(dataDir); err != nil || !info.IsDir() {
		t.Errorf("data directory was not created: %v", err)
	}

	// The database exists and went through the migration runner.
	db, err := store.Open(t.Context(), filepath.Join(dataDir, store.FileName))
	if err != nil {
		t.Fatalf("open the database run created: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Errorf("schema_migrations is missing, migrations did not run: %v", err)
	}
}

// syncBuffer is a bytes.Buffer that run's logger and the test can use
// from different goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitFor(t *testing.T, logs *syncBuffer, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("no log line with %s within 5s; logs:\n%s", want, logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
