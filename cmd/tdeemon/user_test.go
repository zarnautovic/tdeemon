package main

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// instance returns the environment of an instance whose data directory
// holds a migrated database, as a server that has started once leaves it.
func instance(t *testing.T, extra map[string]string) (env func(string) (string, bool), dbPath string) {
	t.Helper()

	dataDir := t.TempDir()
	dbPath = filepath.Join(dataDir, store.FileName)
	db, err := store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(t.Context(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	m := map[string]string{
		"TDEEMON_BASE_URL":   "http://localhost",
		"TDEEMON_SECRET_KEY": strings.Repeat("s", 32),
		"TDEEMON_DATA_DIR":   dataDir,
	}
	maps.Copy(m, extra)
	return lookup(m), dbPath
}

// stored reads back the user with email and their password hash.
func stored(t *testing.T, dbPath, email string) (user.User, string) {
	t.Helper()

	db, err := store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	u, err := db.UserByEmail(t.Context(), email)
	if err != nil {
		t.Fatalf("user by email %s: %v", email, err)
	}
	var hash string
	if err := db.Read().QueryRowContext(t.Context(), "SELECT password_hash FROM users WHERE id = ?", u.ID.String()).Scan(&hash); err != nil {
		t.Fatalf("read password hash: %v", err)
	}
	return u, hash
}

func userCount(t *testing.T, dbPath string) int {
	t.Helper()

	db, err := store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

var printedPassword = regexp.MustCompile(`(?m)^Password: (\S+)$`)

func TestUserCreateAdminWithGeneratedPassword(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	var out bytes.Buffer
	args := []string{"user", "create", "--email", " Ana@Example.com ", "--admin", "--timezone", "Europe/Zagreb"}
	if err := run(t.Context(), args, env, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("user create: %v", err)
	}

	u, hash := stored(t, dbPath, "ana@example.com")
	if u.Role != user.RoleAdmin || u.Name != "ana" || u.TimeZone != "Europe/Zagreb" ||
		u.Locale != user.DefaultLocale || u.DayStartHour != user.DefaultDayStartHour {
		t.Errorf("stored user = %+v, want an admin named ana in Europe/Zagreb with the defaults", u)
	}
	if !strings.HasPrefix(out.String(), "Created admin ana@example.com (id "+u.ID.String()+").\n") {
		t.Errorf("output %q does not report the new user", out.String())
	}
	m := printedPassword.FindStringSubmatch(out.String())
	if m == nil {
		t.Fatalf("output %q prints no password", out.String())
	}
	if ok, err := auth.VerifyPassword(m[1], hash); err != nil || !ok {
		t.Errorf("the printed password does not match the stored hash (ok %v, err %v)", ok, err)
	}
}

func TestUserCreatePasswordFromStdin(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, input string }{
		{"line", "correct horse battery staple\n"},
		{"crlf", "correct horse battery staple\r\n"},
		{"no line ending", "correct horse battery staple"},
		{"only the first line", "correct horse battery staple\nsecond line\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env, dbPath := instance(t, map[string]string{"TZ": "America/New_York"})
			var out bytes.Buffer
			args := []string{"user", "create", "--email", "ivo@example.com", "--name", "Ivo", "--password-stdin"}
			if err := run(t.Context(), args, env, strings.NewReader(tc.input), &out, io.Discard); err != nil {
				t.Fatalf("user create: %v", err)
			}
			if strings.Contains(out.String(), "Password:") {
				t.Errorf("output %q prints a password the user typed", out.String())
			}
			u, hash := stored(t, dbPath, "ivo@example.com")
			if u.Role != user.RoleUser || u.Name != "Ivo" || u.TimeZone != "America/New_York" {
				t.Errorf("stored user = %+v, want a user named Ivo in TZ's zone", u)
			}
			if ok, err := auth.VerifyPassword("correct horse battery staple", hash); err != nil || !ok {
				t.Errorf("the password from stdin does not match the stored hash (ok %v, err %v)", ok, err)
			}
		})
	}
}

func TestUserCreateRejects(t *testing.T) {
	t.Parallel()

	tooLong := strings.Repeat("x", maxPasswordInput+1)
	for _, tc := range []struct {
		name, input string
		email       string // ana@example.com when empty
		args        []string
		want        error  // checked with errors.Is when set
		wantText    string // otherwise, a part of the message
	}{
		{"short password", "too short\n", "", []string{"--password-stdin"}, auth.ErrPasswordTooShort, ""},
		{"overlong password", tooLong, "", []string{"--password-stdin"}, auth.ErrPasswordTooLong, ""},
		{"overlong password line", tooLong + "\n", "", []string{"--password-stdin"}, auth.ErrPasswordTooLong, ""},
		{"too many characters", strings.Repeat("ž", auth.MaxPasswordLen+1), "", []string{"--password-stdin"}, auth.ErrPasswordTooLong, ""},
		// 300 characters in 1200 bytes: cut at the byte limit, the first 256
		// would pass the rules, so the overlong input must be noticed.
		{"overlong in bytes", strings.Repeat("😀", 300), "", []string{"--password-stdin"}, auth.ErrPasswordTooLong, ""},
		{"zero-width space in the address", "", "ana@example.com\u200b", nil, nil, "invisible character"},
		{"combining grapheme joiner in the address", "", "ana\u034f@example.com", nil, nil, "invisible character"},
		{"variation selector in the address", "", "ana\ufe0f@example.com", nil, nil, "invisible character"},
		// "čaba" typed in a Latin-2 terminal: one byte that is not UTF-8.
		{"address not UTF-8", "", "\xe8aba@primjer.hr", nil, nil, "not valid UTF-8"},
		// A bad address is reported as such, not as a name to set.
		{"control character in the address", "", "ana\x01@example.com", nil, nil, "email"},
		{"long local part without a name", "", strings.Repeat("a", 101) + "@example.com", nil, nil, "set one with --name"},
		{"no password", "", "", []string{"--password-stdin"}, nil, "no password on standard input"},
		{"empty line", "\n", "", []string{"--password-stdin"}, nil, "no password on standard input"},
		{"bad time zone", "", "", []string{"--timezone", "Mars/Olympus"}, nil, "invalid user"},
		{"display name in the address", "", "Ana <ana@example.com>", nil, nil, "invalid user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env, dbPath := instance(t, nil)
			email := tc.email
			if email == "" {
				email = "ana@example.com"
			}
			args := append([]string{"user", "create", "--email", email}, tc.args...)
			var out bytes.Buffer
			err := run(t.Context(), args, env, strings.NewReader(tc.input), &out, io.Discard)
			switch {
			case err == nil:
				t.Fatal("user create succeeded, want an error")
			case tc.want != nil && !errors.Is(err, tc.want):
				t.Errorf("error = %v, want %v", err, tc.want)
			case tc.wantText != "" && !strings.Contains(err.Error(), tc.wantText):
				t.Errorf("error = %v, want it to contain %q", err, tc.wantText)
			}
			if n := userCount(t, dbPath); n != 0 {
				t.Errorf("%d users stored, want none", n)
			}
			if out.Len() > 0 {
				t.Errorf("printed %q on failure", out.String())
			}
		})
	}
}

// A long local part is fine when --name gives the name.
func TestUserCreateLongLocalPartWithName(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	email := strings.Repeat("a", 101) + "@example.com"
	if err := run(t.Context(), []string{"user", "create", "--email", email, "--name", "Ana"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("user create: %v", err)
	}
	if u, _ := stored(t, dbPath, email); u.Name != "Ana" {
		t.Errorf("name = %q, want Ana", u.Name)
	}
}

func TestUserCreateHelp(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"user", "help"},
		{"user", "--help"},
		{"user", "create", "--help"},
		{"user", "create", "-h"},
		{"user", "create", "--email", "ana@example.com", "--help"},
	} {
		var out bytes.Buffer
		// No configuration: help must not need any.
		if err := run(t.Context(), args, lookup(nil), noInput(), &out, io.Discard); err != nil {
			t.Errorf("run(%q) = %v, want the usage", args, err)
		}
		if !strings.Contains(out.String(), "Flags of user create:") {
			t.Errorf("run(%q) printed %q, want the usage", args, out.String())
		}
	}
}

func TestUserCreateTakenEmail(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("first user create: %v", err)
	}
	err := run(t.Context(), []string{"user", "create", "--email", "ANA@example.com", "--admin"}, env, noInput(), io.Discard, io.Discard)
	if !errors.Is(err, store.ErrEmailTaken) {
		t.Errorf("second user create = %v, want ErrEmailTaken", err)
	}
	// "č" as one character, then as c + combining caron: one address.
	if err := run(t.Context(), []string{"user", "create", "--email", "\u010daba@example.com"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("user create čaba: %v", err)
	}
	err = run(t.Context(), []string{"user", "create", "--email", "c\u030caba@example.com"}, env, noInput(), io.Discard, io.Discard)
	if !errors.Is(err, store.ErrEmailTaken) {
		t.Errorf("user create with a decomposed č = %v, want ErrEmailTaken", err)
	}
	if n := userCount(t, dbPath); n != 2 {
		t.Errorf("%d users stored, want 2", n)
	}
}

// A data directory without a database belongs to a server that has not
// started yet: user create must not create one.
func TestUserCreateNeedsADatabase(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	env := lookup(map[string]string{
		"TDEEMON_BASE_URL":   "http://localhost",
		"TDEEMON_SECRET_KEY": strings.Repeat("s", 32),
		"TDEEMON_DATA_DIR":   dataDir,
	})
	err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, env, noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "start the server once first") {
		t.Errorf("error = %v, want one that says to start the server first", err)
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("read data directory: %v", err)
	}
	if len(entries) > 0 {
		t.Errorf("user create left %d files in the data directory, want none", len(entries))
	}
}

// user create never migrates: the server may be running, and only the
// server migrates.
func TestUserCreateRefusesAnUnmigratedDatabase(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, store.FileName)
	db, err := store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	env := lookup(map[string]string{
		"TDEEMON_BASE_URL":   "http://localhost",
		"TDEEMON_SECRET_KEY": strings.Repeat("s", 32),
		"TDEEMON_DATA_DIR":   dataDir,
	})

	err = run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, env, noInput(), io.Discard, io.Discard)
	if !errors.Is(err, store.ErrSchemaOutdated) {
		t.Fatalf("error = %v, want ErrSchemaOutdated", err)
	}
	db, err = store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema").Scan(&n); err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if n != 0 {
		t.Errorf("the database has %d schema objects after user create, want none: it migrated", n)
	}
}

func TestUserCreateRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, lookup(nil), noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid configuration") {
		t.Errorf("error = %v, want an invalid configuration", err)
	}
}

func TestNewUserTimeZone(t *testing.T) {
	t.Parallel()

	zagreb, err := time.LoadLocation("Europe/Zagreb")
	if err != nil {
		t.Fatalf("load Europe/Zagreb: %v", err)
	}
	now := time.Now()
	for _, tc := range []struct {
		name, flag string
		instance   *time.Location
		want       string
	}{
		{"the flag wins", "Asia/Tokyo", zagreb, "Asia/Tokyo"},
		{"TZ when set", "", zagreb, "Europe/Zagreb"},
		{"UTC when TZ is not set", "", time.Local, "UTC"},
		{"UTC without a location", "", nil, "UTC"},
	} {
		got := newUser(createUserFlags{email: "ana@example.com", timeZone: tc.flag}, tc.instance, now).TimeZone
		if got != tc.want {
			t.Errorf("%s: time zone %q, want %q", tc.name, got, tc.want)
		}
	}
}
