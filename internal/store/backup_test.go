package store_test

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// backupsDir returns the backups directory next to db's file.
func backupsDir(t *testing.T, db *store.DB) string {
	t.Helper()
	var seq int
	var name, file string
	if err := db.Read().QueryRowContext(t.Context(), "PRAGMA database_list").Scan(&seq, &name, &file); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(file), "backups")
}

// files returns the names of the files in dir, sorted.
func files(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// A daily snapshot is a copy of the database, taken when the newest is
// DailyEvery old or there is none; the newest keep stay, and the
// snapshots before a migration, other files and the snapshots of another
// kind are not touched, but a daily snapshot's leftover temporary file
// goes.
func TestDailySnapshot(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	dir := backupsDir(t, db)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	others := []string{"daily-123.tmp", "notes.txt", "pre-migrate-0001-20260101T000000.000000Z.db"}
	for _, f := range others {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)
	first, err := db.DailySnapshot(t.Context(), at, 2)
	if err != nil || filepath.Base(first) != "daily-20261022T030000.000000Z.db" {
		t.Fatalf("the first = %q, %v", first, err)
	}
	checkSnapshot(t, first)
	if dst, err := db.DailySnapshot(t.Context(), at.Add(store.DailyEvery-time.Second), 2); dst != "" || err != nil {
		t.Errorf("before it is due = %q, %v", dst, err)
	}
	for _, later := range []time.Duration{store.DailyEvery, 48 * time.Hour} {
		if dst, err := db.DailySnapshot(t.Context(), at.Add(later), 2); dst == "" || err != nil {
			t.Fatalf("%v later = %q, %v", later, dst, err)
		}
	}
	want := []string{"daily-20261023T023000.000000Z.db", "daily-20261024T030000.000000Z.db", "notes.txt", "pre-migrate-0001-20260101T000000.000000Z.db"}
	if got := files(t, dir); !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
	if _, err := db.DailySnapshot(t.Context(), at, 0); err == nil {
		t.Error("keeping none succeeded")
	}
}

// Checks once an hour take a snapshot a day, at the same hour.
func TestDailySnapshotHourly(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	at := time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)
	for h := range 10 * 24 {
		if _, err := db.DailySnapshot(t.Context(), at.Add(time.Duration(h)*time.Hour), 30); err != nil {
			t.Fatal(err)
		}
	}
	got := files(t, backupsDir(t, db))
	if len(got) != 10 || slices.ContainsFunc(got, func(f string) bool { return !strings.Contains(f, "T030000") }) {
		t.Errorf("files = %v, want ten at 03:00", got)
	}
}

// A snapshot of a time to come, from a clock that was ahead, does not
// stop the daily ones, nor make one at every check.
func TestDailySnapshotAfterAClockAhead(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	ahead := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.DailySnapshot(t.Context(), ahead, 7); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)
	if dst, err := db.DailySnapshot(t.Context(), at, 7); dst == "" || err != nil {
		t.Errorf("after a clock ahead = %q, %v", dst, err)
	}
	if dst, err := db.DailySnapshot(t.Context(), at.Add(time.Hour), 7); dst != "" || err != nil {
		t.Errorf("an hour later = %q, %v", dst, err)
	}
}

// checkSnapshot checks that the file at path is a database with the
// schema's migrations.
func checkSnapshot(t *testing.T, path string) {
	t.Helper()
	snap, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = snap.Close() }()
	var n int
	if err := snap.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&n); err != nil || n == 0 {
		t.Errorf("the snapshot has %d migrations, %v", n, err)
	}
}

// StartDailySnapshots takes the snapshot due before it returns, then
// checks every interval until its context is done; a snapshot that fails
// is logged.
func TestStartDailySnapshots(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	dir := backupsDir(t, db)
	ctx, cancel := context.WithCancel(t.Context())
	wait := db.StartDailySnapshots(ctx, 1, 10*time.Millisecond, slog.New(slog.DiscardHandler))
	got := files(t, dir)
	if len(got) != 1 || !strings.HasPrefix(got[0], "daily-") {
		t.Fatalf("files at the start = %v", got)
	}
	// One gone is due again, at the next check.
	if err := os.Remove(filepath.Join(dir, got[0])); err != nil {
		t.Fatal(err)
	}
	// A finished one, not the temporary file of one under way.
	deadline := time.Now().Add(5 * time.Second)
	var snaps []string
	for len(snaps) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no snapshot at a later check")
		}
		time.Sleep(10 * time.Millisecond)
		snaps, _ = filepath.Glob(filepath.Join(dir, "daily-*.db"))
	}
	cancel()
	wait()
	checkSnapshot(t, snaps[0])

	broken := storetest.Open(t)
	if err := os.WriteFile(backupsDir(t, broken), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stop, cancelBroken := context.WithCancel(t.Context())
	var failed bytes.Buffer
	waitBroken := broken.StartDailySnapshots(stop, 1, time.Hour, slog.New(slog.NewTextHandler(&failed, nil)))
	cancelBroken()
	waitBroken()
	if !strings.Contains(failed.String(), "the daily snapshot failed") {
		t.Errorf("logs = %q", failed.String())
	}
}
