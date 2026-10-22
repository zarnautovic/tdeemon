package main

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// predecessor writes the predecessor's database, with made-up rows, and
// returns its path.
func predecessor(t *testing.T) string {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "history", "testdata", "predecessor.sql"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "health.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(t.Context(), string(schema)+`
INSERT INTO days (date, kalorije, proteini, ugljikohidrati, masti, kilaza, san) VALUES ('2026-09-01', 2000, 150, 200, 60, 81.2, 30);
INSERT INTO meals VALUES ('2026-09-01', '08:10', 'Oats with milk', 500, 25, 70, 12);
INSERT INTO days (date, kalorije, proteini, ugljikohidrati, masti, napomena) VALUES ('2026-09-02', 900, 60, 90, 30, 'At a wedding');
INSERT INTO meals VALUES ('2026-09-02', '12:00', 'Stew', 1000, 70, 100, 40);
INSERT INTO meals VALUES ('2026-09-02', '13:00', '`+strings.Repeat("a", 210)+`', 10, 1, 1, 0);
INSERT INTO days (date, kilaza) VALUES ('soon', 80);
`); err != nil {
		t.Fatal(err)
	}
	return path
}

// The import says what it did and what it left out; a dry run says what
// it would do and writes nothing.
func TestImportHistory(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com", "--timezone", "Europe/Zagreb"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	old := predecessor(t)
	var dry bytes.Buffer
	if err := run(t.Context(), []string{"import-history", "--email", "ANA@example.com", "--health-db", old, "--dry-run"}, env, noInput(), &dry, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dry.String(), "Would import (a dry run, nothing written) for ana@example.com, whose days start at 04:00 in Europe/Zagreb: "+
		"2 days, 1 metric values, 4 meals, of them 1 rests of a day; 1 days excluded.") {
		t.Errorf("the dry run said:\n%s", dry.String())
	}
	if n := mealsOn(t, dbPath, "ana@example.com", day.Date{Year: 2026, Month: 9, Day: 1}); n != 0 {
		t.Fatalf("the dry run wrote %d meals", n)
	}

	var out bytes.Buffer
	if err := run(t.Context(), []string{"import-history", "--email", "ana@example.com", "--health-db", old}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Imported for ana@example.com, whose days start at 04:00 in Europe/Zagreb: 2 days, 1 metric values, 4 meals, of them 1 rests of a day; 1 days excluded.",
		"Values left out (1):\n  2026-09-01 sleep_h: sleep_h 30: want 0 to 24 h",
		"Days whose meals have more kcal than the day's total, kept without a rest (1):\n  2026-09-02",
		"Meals renamed (1):\n  2026-09-02: aaa",
		`Skipped (1):` + "\n" + `  "soon"`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the import said:\n%s\nwant %q in it", out.String(), want)
		}
	}
	if n := mealsOn(t, dbPath, "ana@example.com", day.Date{Year: 2026, Month: 9, Day: 1}); n != 2 {
		t.Errorf("meals of 1 September = %d, want the oats and the rest", n)
	}
}

func mealsOn(t *testing.T, dbPath, email string, d day.Date) int {
	t.Helper()
	db, err := store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	u, err := db.UserByEmail(t.Context(), email)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := db.MealsOfDay(t.Context(), u.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	return len(ms)
}

func TestImportHistoryRefuses(t *testing.T) {
	t.Parallel()

	env, _ := instance(t, nil)
	old := predecessor(t)
	for _, tc := range []struct {
		name  string
		args  []string
		usage bool
		want  string
	}{
		{"no flags", nil, true, "--email and --health-db are required"},
		{"no database of the predecessor's", []string{"--email", "ana@example.com"}, true, "required"},
		{"an argument", []string{"--email", "ana@example.com", "--health-db", old, "now"}, true, `unexpected argument "now"`},
		{"an unknown flag", []string{"--vault", "x"}, true, "flag provided but not defined"},
		{"no user", []string{"--email", "ivo@example.com", "--health-db", old}, false, `no user with the address "ivo@example.com"`},
		{"no file", []string{"--email", "ana@example.com", "--health-db", filepath.Join(t.TempDir(), "none.db")}, false, "no database at"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := run(t.Context(), append([]string{"import-history"}, tc.args...), env, noInput(), io.Discard, io.Discard)
			if err == nil || errors.Is(err, errUsage) != tc.usage || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q (usage %v)", err, tc.want, tc.usage)
			}
		})
	}
	// Help, and a configuration that does not load.
	var out bytes.Buffer
	if err := run(t.Context(), []string{"import-history", "-h"}, env, noInput(), &out, io.Discard); err != nil || !strings.Contains(out.String(), "Flags of import-history") {
		t.Errorf("help: %v\n%s", err, out.String())
	}
	if err := run(t.Context(), []string{"import-history", "--email", "a@example.com", "--health-db", old}, lookup(nil), noInput(), io.Discard, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "invalid configuration") {
		t.Errorf("no configuration: %v", err)
	}
}
