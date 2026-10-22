package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/history"
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
		"Days excluded from calibration (1):\n  2026-09-02: At a wedding",
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
		{"no flags", nil, true, "--email is required"},
		{"nothing to import", []string{"--email", "ana@example.com"}, true, "--health-db, --foods, --plan or more"},
		{"no food table", []string{"--email", "ana@example.com", "--foods", filepath.Join(t.TempDir(), "none.md")}, false, "the food table"},
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

// The food table imports alone, or with the days; a run again keeps the
// foods.
func TestImportHistoryFoods(t *testing.T) {
	t.Parallel()

	env, _ := instance(t, nil)
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com", "--timezone", "Europe/Zagreb"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	foods := filepath.Join("..", "..", "internal", "history", "testdata", "foods.txt")
	var out bytes.Buffer
	if err := run(t.Context(), []string{"import-history", "--email", "ana@example.com", "--foods", foods}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Imported foods for ana@example.com: 15 new; 0 kept",
		"Foods renamed (2):\n  Stew (1/4 of the pot)",
		"Foods per portion though given by weight or volume, as per 100 they would be no food (1):\n  line 19, Odd (5 g)",
		"Rows skipped (8):\n  line 18, Broken",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the import said:\n%s\nwant %q in it", out.String(), want)
		}
	}
	out.Reset()
	if err := run(t.Context(), []string{"import-history", "--email", "ana@example.com", "--foods", foods, "--health-db", predecessor(t), "--dry-run"}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Would import (a dry run, nothing written) foods for ana@example.com: 0 new; 15 kept") ||
		!strings.Contains(out.String(), "whose days start at 04:00") {
		t.Errorf("both, again, dry:\n%s", out.String())
	}
}

// A failure after the foods leaves their report, as they stay: here the
// days fail on a calendar that does not read.
func TestImportHistoryReportsBeforeAFailure(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com", "--timezone", "Europe/Zagreb"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE users SET timezone = 'Nowhere/Bogus'"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	foods := filepath.Join("..", "..", "internal", "history", "testdata", "foods.txt")
	var out bytes.Buffer
	err = run(t.Context(), []string{"import-history", "--email", "ana@example.com", "--foods", foods, "--health-db", predecessor(t)}, env, noInput(), &out, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "calendar") || !strings.Contains(out.String(), "Imported foods for ana@example.com: 15 new") {
		t.Errorf("err = %v, said:\n%s", err, out.String())
	}
}

// A failure among the foods leaves the report of those written before it.
func TestImportHistoryReportsFoodsBeforeAFailure(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com", "--timezone", "Europe/Zagreb"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER refuse_the_third BEFORE INSERT ON foods
		WHEN (SELECT count(*) FROM foods) >= 2 BEGIN SELECT RAISE(ABORT, 'refused by the test'); END`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	foods := filepath.Join("..", "..", "internal", "history", "testdata", "foods.txt")
	var out bytes.Buffer
	err = run(t.Context(), []string{"import-history", "--email", "ana@example.com", "--foods", foods}, env, noInput(), &out, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "refused by the test") || !strings.Contains(out.String(), "Imported foods for ana@example.com: 2 new") {
		t.Errorf("err = %v, said:\n%s", err, out.String())
	}
}

// The plan file imports with the limits held to the user's weigh-ins of
// the last week, which this test writes by the real clock; without any,
// the limits refuse it.
func TestImportHistoryPlan(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	for _, email := range []string{"ana@example.com", "ivo@example.com"} {
		if err := run(t.Context(), []string{"user", "create", "--email", email, "--timezone", "Europe/Zagreb"}, env, noInput(), io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ana, err := db.UserByEmail(t.Context(), "ana@example.com")
	if err != nil {
		t.Fatal(err)
	}
	today := day.DateOf(time.Now())
	for i := range 7 {
		v := []metric.Value{{Kind: metric.Weight, Value: 72, Source: metric.Import}}
		if _, err := db.MergeMetrics(t.Context(), ana.ID, history.Actor, today.AddDays(-i), v, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()

	planFile := filepath.Join("..", "..", "internal", "history", "testdata", "plan.json")
	var out bytes.Buffer
	if err := run(t.Context(), []string{"import-history", "--email", "ana@example.com", "--plan", planFile}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Imported the plan for ana@example.com: 6 parts.\nWritten (6):\n  profile") {
		t.Errorf("the import said:\n%s", out.String())
	}
	out.Reset()
	if err := run(t.Context(), []string{"import-history", "--email", "ana@example.com", "--plan", planFile, "--dry-run"}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "the plan for ana@example.com: 0 parts.\n") {
		t.Errorf("the run again said:\n%s", out.String())
	}
	err = run(t.Context(), []string{"import-history", "--email", "ivo@example.com", "--plan", planFile}, env, noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), limits.RuleWeightMissing) {
		t.Errorf("a plan without weigh-ins: %v", err)
	}
	err = run(t.Context(), []string{"import-history", "--email", "ivo@example.com", "--plan", filepath.Join(t.TempDir(), "none.json")}, env, noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "the plan file") {
		t.Errorf("no plan file: %v", err)
	}
}

// The instance's limits from TDEEMON_LIMIT_* hold a plan, and a dry run of
// the days and the plan holds it to the days' weigh-ins, which it does not
// write.
func TestImportHistoryPlanLimits(t *testing.T) {
	t.Parallel()

	env, _ := instance(t, map[string]string{"TDEEMON_LIMIT_CUT_RATE": "-0.4"})
	loose, _ := instance(t, nil)
	for _, e := range []func(string) (string, bool){env, loose} {
		if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com", "--timezone", "Europe/Zagreb"}, e, noInput(), io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	old := weighedDays(t, 72, 72, 72, 72, 72, 72, 72)
	planFile := filepath.Join("..", "..", "internal", "history", "testdata", "plan.json")
	args := []string{"import-history", "--email", "ana@example.com", "--health-db", old, "--plan", planFile, "--dry-run"}
	var out bytes.Buffer
	if err := run(t.Context(), args, loose, noInput(), &out, io.Discard); err != nil || !strings.Contains(out.String(), "the plan for ana@example.com: 6 parts.") {
		t.Errorf("the dry run: %v\n%s", err, out.String())
	}
	if err := run(t.Context(), args, env, noInput(), io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "cut_rate") || !strings.Contains(err.Error(), "(instance)") {
		t.Errorf("past the instance's limit: %v", err)
	}
}

// weighedDays returns a predecessor's database whose days before today
// have the weigh-ins kg, the latest first.
func weighedDays(t *testing.T, kg ...float64) string {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "history", "testdata", "predecessor.sql"))
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(t.TempDir(), "health.db")
	db, err := sql.Open("sqlite", old)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows := string(schema)
	today := day.DateOf(time.Now())
	for i, w := range kg {
		rows += fmt.Sprintf("INSERT INTO days (date, kalorije, kilaza) VALUES ('%s', 1800, %v);\n", today.AddDays(-1-i), w)
	}
	if _, err := db.ExecContext(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	return old
}

// A weigh-in the days leave out, out of its bounds, does not hold the
// plan, in a dry run or not: with it the trend would not be underweight,
// where no cut is allowed.
func TestImportHistoryPlanLeftOutWeighIn(t *testing.T) {
	t.Parallel()

	env, _ := instance(t, nil)
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com", "--timezone", "Europe/Zagreb"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	old := weighedDays(t, 450, 45, 45, 45, 45, 45, 45)
	planFile := filepath.Join("..", "..", "internal", "history", "testdata", "plan.json")
	for _, dry := range []bool{true, false} {
		args := []string{"import-history", "--email", "ana@example.com", "--health-db", old, "--plan", planFile}
		if dry {
			args = append(args, "--dry-run")
		}
		var out bytes.Buffer
		err := run(t.Context(), args, env, noInput(), &out, io.Discard)
		if err == nil || !strings.Contains(err.Error(), limits.RuleUnderweightLoss) || !strings.Contains(out.String(), "weight 450") {
			t.Errorf("a dry run %v: %v\n%s", dry, err, out.String())
		}
	}
}

// A warning names its rule, its day type if any, and its values to two
// decimals at most.
func TestWritePlanReportWarnings(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	writePlanReport(&b, user.User{Email: "ana@example.com"}, history.PlanReport{Warnings: []limits.Warning{
		{Rule: "below_bmr", DayType: plan.Rest, Value: 1400, Threshold: 1441.4999},
		{Rule: "protein_low", Value: 1.4583333, Threshold: 1.6},
	}}, false)
	for _, want := range []string{"  below_bmr (rest day): 1400, the threshold 1441.5\n", "  protein_low: 1.46, the threshold 1.6\n"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no %q in\n%s", want, b.String())
		}
	}
}
