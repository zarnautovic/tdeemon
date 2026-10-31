package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/push"
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

// startAndStop runs the server with env and the data directory dataDir
// until it serves, stops it, and returns its logs.
func startAndStop(t *testing.T, dataDir string, env map[string]string) string {
	t.Helper()
	vars := map[string]string{
		"TDEEMON_LISTEN":     "127.0.0.1:0",
		"TDEEMON_BASE_URL":   "http://localhost",
		"TDEEMON_SECRET_KEY": strings.Repeat("s", 32),
		"TDEEMON_DATA_DIR":   dataDir,
	}
	maps.Copy(vars, env)

	ctx, cancel := context.WithCancel(t.Context())
	var logs syncBuffer
	done := make(chan error, 1)
	go func() { done <- run(ctx, nil, lookup(vars), noInput(), io.Discard, &logs) }()

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
	return logs.String()
}

func TestRunStartsAndStops(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "data")
	startAndStop(t, dataDir, nil)

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

	// It took the daily snapshot before it served.
	if snaps, _ := filepath.Glob(filepath.Join(dataDir, "backups", "daily-*.db")); len(snaps) != 1 {
		t.Errorf("daily snapshots = %v, want one", snaps)
	}
}

// With TDEEMON_BACKUP_KEEP=0 the server takes no daily snapshot.
func TestRunWithoutDailySnapshots(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "data")
	logs := startAndStop(t, dataDir, map[string]string{"TDEEMON_BACKUP_KEEP": "0"})
	if snaps, _ := filepath.Glob(filepath.Join(dataDir, "backups", "daily-*.db")); len(snaps) != 0 || strings.Contains(logs, "daily snapshot") {
		t.Errorf("daily snapshots = %v, logs:\n%s", snaps, logs)
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

	// Generous: under -race, with the other packages' argon2 tests on the
	// same cores, migrating and preparing sign-in can take seconds. The
	// deadline only turns a hang into a failure.
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(logs.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("no log line with %s within 30s; logs:\n%s", want, logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TDEEMON_TRUSTED_PROXIES reaches the server: behind a trusted proxy,
// the limit on failed sign-ins counts the forwarded client, so ten
// failures from one client do not lock out the next one.
func TestRunTrustsTheConfiguredProxies(t *testing.T) {
	t.Parallel()

	env := lookup(map[string]string{
		"TDEEMON_LISTEN":          "127.0.0.1:0",
		"TDEEMON_BASE_URL":        "http://localhost",
		"TDEEMON_SECRET_KEY":      strings.Repeat("s", 32),
		"TDEEMON_DATA_DIR":        filepath.Join(t.TempDir(), "data"),
		"TDEEMON_TRUSTED_PROXIES": "127.0.0.1",
	})
	ctx, cancel := context.WithCancel(t.Context())
	var logs syncBuffer
	done := make(chan error, 1)
	go func() { done <- run(ctx, nil, env, noInput(), io.Discard, &logs) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run returned %v, want nil", err)
		}
	})
	waitFor(t, &logs, `"msg":"listening"`)
	addr := regexp.MustCompile(`"msg":"listening","addr":"([^"]+)"`).FindStringSubmatch(logs.String())
	if addr == nil {
		t.Fatalf("no address in the logs:\n%s", logs.String())
	}

	signIn := func(client string, i int) int {
		t.Helper()
		body := fmt.Sprintf(`{"email":"guess%d@example.com","password":"wrong password, surely"}`, i)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr[1]+"/api/v1/auth/login", strings.NewReader(body))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", client)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("sign-in: %v", err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	for i := range 10 {
		if code := signIn("203.0.113.9", i); code != http.StatusUnauthorized {
			t.Fatalf("failure %d: %d, want 401", i+1, code)
		}
	}
	if code := signIn("203.0.113.9", 10); code != http.StatusTooManyRequests {
		t.Errorf("the same client after ten failures: %d, want 429", code)
	}
	if code := signIn("203.0.113.10", 11); code != http.StatusUnauthorized {
		t.Errorf("another client behind the proxy: %d, want 401, not the first one's 429", code)
	}
}

// The scheduler has the morning summary and the watchdog with a Telegram
// bot or Web Push to send them, and not without either.
func TestJobs(t *testing.T) {
	t.Parallel()

	if bot := telegramBot(config.Config{}); bot != nil {
		t.Errorf("a bot without a token = %v", bot)
	}
	if js := jobs(api.Config{}, nil); len(js) != 1 || js[0].Name != "autopilot_review" {
		t.Errorf("jobs without a bot = %v, want the weekly review alone", js)
	}
	bot := telegramBot(config.Config{TelegramBotToken: testBotToken, TelegramAPIURL: "http://localhost"})
	if bot == nil || bot.Token != testBotToken || bot.BaseURL != "http://localhost" {
		t.Fatalf("the bot = %+v", bot)
	}
	js := jobs(api.Config{}, bot)
	if len(js) != 3 || js[0].Name != "autopilot_review" || js[1].Name != "morning_summary" || js[2].Name != "watchdog" {
		t.Errorf("jobs with a bot = %v", js)
	}
	sender, err := push.NewSender([]byte(strings.Repeat("k", 32)), "https://tdeemon.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if js := jobs(api.Config{Push: sender}, nil); len(js) != 3 {
		t.Errorf("jobs with push alone = %v", js)
	}
}
