package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	done := make(chan error, 1)
	go func() { done <- run(ctx, env, io.Discard) }()

	// Let run get past config and listen before asking it to stop.
	time.Sleep(200 * time.Millisecond)
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
}
