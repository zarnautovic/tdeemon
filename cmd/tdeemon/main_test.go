package main

import (
	"bytes"
	"context"
	"errors"
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

	err := run(t.Context(), nil, lookup(nil), noInput(), io.Discard, io.Discard)
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
	go func() { done <- run(ctx, nil, env, noInput(), io.Discard, &logs) }()

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

// TestRunRejectsArguments runs with a valid configuration: a command line
// tdeemon does not accept must stop run before it creates the data
// directory or the database.
func TestRunRejectsArguments(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "data")
	env := lookup(map[string]string{
		"TDEEMON_LISTEN":     "127.0.0.1:0",
		"TDEEMON_BASE_URL":   "http://localhost",
		"TDEEMON_SECRET_KEY": strings.Repeat("s", 32),
		"TDEEMON_DATA_DIR":   dataDir,
	})

	// If an argument were ignored, run would serve until ctx ends; the
	// deadline turns that into a failure instead of a hung test.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	for _, args := range [][]string{
		{"healthcheck"},
		{"serve", "--port", "1"},
		{"-v"},
		{"user"},
		{"user", "delete"},
		{"user", "create"},
		{"user", "create", "--email"},
		{"user", "create", "--email", "ana@example.com", "--role", "admin"},
		{"user", "create", "--email", "ana@example.com", "extra"},
	} {
		err := run(ctx, args, env, noInput(), io.Discard, io.Discard)
		if !errors.Is(err, errUsage) {
			t.Errorf("run(%q) = %v, want errUsage", args, err)
		}
	}
	for _, help := range []string{"-h", "--help", "help"} {
		var out bytes.Buffer
		if err := run(ctx, []string{help}, env, noInput(), &out, io.Discard); err != nil {
			t.Errorf("run(%q) = %v, want the usage", help, err)
		}
		if !strings.Contains(out.String(), "Usage:\n  tdeemon ") {
			t.Errorf("run(%q) printed %q, want the usage", help, out.String())
		}
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the data directory exists after arguments were rejected (stat: %v)", err)
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	usageErr := run(t.Context(), []string{"healthcheck"}, lookup(nil), noInput(), io.Discard, io.Discard)
	for _, tc := range []struct {
		name string
		err  error
		code int
		out  string
	}{
		{"success", nil, 0, ""},
		{"arguments", usageErr, 2, "tdeemon: invalid command line: unknown command \"healthcheck\"\n\n" + usage},
		{"failure", errors.New("database: disk full"), 1, "tdeemon: database: disk full\n"},
	} {
		var out bytes.Buffer
		if code := exitCode(tc.err, &out); code != tc.code {
			t.Errorf("%s: exit code %d, want %d", tc.name, code, tc.code)
		}
		if out.String() != tc.out {
			t.Errorf("%s: printed %q, want %q", tc.name, out.String(), tc.out)
		}
	}
}

// noInput is standard input with nothing on it.
func noInput() io.Reader { return strings.NewReader("") }

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
