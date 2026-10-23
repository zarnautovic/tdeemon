package store_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// Users lists every user in the order they were created.
func TestUsers(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	if us, err := db.Users(t.Context()); err != nil || len(us) != 0 {
		t.Fatalf("Users of none = %v, %v", us, err)
	}
	ana, ivo := newUser("ana@example.com"), newUser("ivo@example.com")
	ivo.CreatedAt = ana.CreatedAt.Add(-time.Hour)
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateUser(t.Context(), ivo, ""); err != nil {
		t.Fatal(err)
	}
	us, err := db.Users(t.Context())
	if err != nil || len(us) != 2 || us[0].Email != "ivo@example.com" || us[1].Email != "ana@example.com" {
		t.Errorf("Users = %+v, %v", us, err)
	}
}

// A job's run is recorded per user, job and occurrence, once.
func TestJobRuns(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	ana, ivo := newUser("ana@example.com"), newUser("ivo@example.com")
	for _, err := range []error{db.CreateUser(t.Context(), ana, ""), db.CreateUser(t.Context(), ivo, "")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	for range 2 {
		if err := db.RecordJobRun(t.Context(), ana.ID, "morning", "2026-10-23", now); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name            string
		ran             bool
		job, occurrence string
		ofIvo           bool
	}{
		{"recorded", true, "morning", "2026-10-23", false},
		{"another occurrence", false, "morning", "2026-10-24", false},
		{"another job", false, "watchdog", "2026-10-23", false},
		{"another user", false, "morning", "2026-10-23", true},
	} {
		id := ana.ID
		if tc.ofIvo {
			id = ivo.ID
		}
		if ran, err := db.JobRan(t.Context(), id, tc.job, tc.occurrence); err != nil || ran != tc.ran {
			t.Errorf("%s: JobRan = %v, %v", tc.name, ran, err)
		}
	}
	if err := db.RecordJobRun(t.Context(), ana.ID, "", "x", now); err == nil {
		t.Error("a run of a job without a name was recorded")
	}
}

// Optimize refreshes the statistics of a table that grew since they were
// taken, though no query of its connection used it.
func TestOptimize(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	exec := func(q string) {
		t.Helper()
		if err := db.WriteTx(t.Context(), func(tx *sql.Tx) error { _, err := tx.ExecContext(t.Context(), q); return err }); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE grows (n INTEGER NOT NULL); CREATE INDEX grows_n ON grows (n)`)
	exec(`WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c WHERE i < 100) INSERT INTO grows SELECT i FROM c`)
	exec(`ANALYZE grows`)
	exec(`WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c WHERE i < 5000) INSERT INTO grows SELECT i FROM c`)
	if err := db.Optimize(t.Context()); err != nil {
		t.Fatal(err)
	}
	var stat string
	if err := db.Read().QueryRowContext(t.Context(), "SELECT stat FROM sqlite_stat1 WHERE idx = 'grows_n'").Scan(&stat); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(stat, "100 ") {
		t.Errorf("the statistics are the old ones: %q", stat)
	}
}

// DeleteJobRuns removes a user's runs of a job made before a time, and
// nothing else.
func TestDeleteJobRuns(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	ana, ivo := newUser("ana@example.com"), newUser("ivo@example.com")
	for _, err := range []error{db.CreateUser(t.Context(), ana, ""), db.CreateUser(t.Context(), ivo, "")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		id       uuid.UUID
		job, occ string
		ran      time.Time
	}{
		{ana.ID, "watchdog", "old", at.Add(-time.Hour)},
		{ana.ID, "watchdog", "new", at},
		{ana.ID, "morning", "old", at.Add(-time.Hour)},
		{ivo.ID, "watchdog", "old", at.Add(-time.Hour)},
	} {
		if err := db.RecordJobRun(t.Context(), r.id, r.job, r.occ, r.ran); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DeleteJobRuns(t.Context(), ana.ID, "watchdog", at); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id       uuid.UUID
		job, occ string
		want     bool
	}{{ana.ID, "watchdog", "old", false}, {ana.ID, "watchdog", "new", true}, {ana.ID, "morning", "old", true}, {ivo.ID, "watchdog", "old", true}} {
		if ran, _ := db.JobRan(t.Context(), c.id, c.job, c.occ); ran != c.want {
			t.Errorf("%s %s: ran %v, want %v", c.job, c.occ, ran, c.want)
		}
	}
}
