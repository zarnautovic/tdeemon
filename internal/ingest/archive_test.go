package ingest

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"
)

func TestArchive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// The day and the time are UTC's: 01:30 at +02 is 23:30 the day before.
	now := time.Date(2026, 10, 22, 1, 30, 0, 0, time.FixedZone("+02", 2*60*60))
	a := &Archive{Dir: dir, Keep: 30 * 24 * time.Hour, Now: func() time.Time { return now }}
	user := uuid.NewV7()
	if err := a.Save(user, "hae.json", []byte(`{"data": {}}`)); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, user.String(), "2026-10-21", "233000-*-hae.json.gz"))
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	info, err := os.Stat(files[0])
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file's mode = %v, %v; want 0600", info.Mode(), err)
	}
	if dirInfo, _ := os.Stat(filepath.Dir(files[0])); dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("the day's mode = %v, want 0700", dirInfo.Mode())
	}
	f, err := os.Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(zr); !bytes.Equal(got, []byte(`{"data": {}}`)) {
		t.Errorf("the archived body = %s", got)
	}
}

// 31 days on, the old day goes; a day within the 30, anything that is no
// day and another user's days stay.
func TestArchivePrunes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Date(2026, 10, 21, 8, 30, 0, 0, time.UTC)
	a := &Archive{Dir: dir, Keep: 30 * 24 * time.Hour, Now: func() time.Time { return now }}
	user := uuid.NewV7()
	if err := a.Save(user, "hae.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	// A name that is no day, though it sorts before the oldest kept.
	for _, keep := range []string{"2026-11-01", "1999-notes"} {
		if err := os.MkdirAll(filepath.Join(dir, user.String(), keep), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	now = time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	if err := a.Save(user, "metrics.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"2026-10-21": false, "2026-11-01": true, "1999-notes": true, "2026-11-21": true} {
		if _, err := os.Stat(filepath.Join(dir, user.String(), name)); (err == nil) != want {
			t.Errorf("%s there = %v, want %v", name, err == nil, want)
		}
	}
	// Another user's archive is left alone.
	other := uuid.NewV7()
	if err := os.MkdirAll(filepath.Join(dir, other.String(), "2026-01-01"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(user, "hae.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, other.String(), "2026-01-01")); err != nil {
		t.Errorf("another user's day went: %v", err)
	}
}

func TestArchiveFails(t *testing.T) {
	t.Parallel()

	// A file where the archive's directory should be.
	file := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&Archive{Dir: file, Keep: time.Hour}).Save(uuid.NewV7(), "hae.json", []byte(`{}`)); err == nil {
		t.Error("Save into a file succeeded")
	}
	// The real clock, when Now is nil.
	dir := t.TempDir()
	user := uuid.NewV7()
	if err := (&Archive{Dir: dir, Keep: time.Hour}).Save(user, "hae.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, user.String(), time.Now().UTC().Format("2006-01-02"))); err != nil {
		t.Errorf("today's directory: %v", err)
	}
}

// The latest export stays past the days that go, and the next replaces
// it.
func TestArchiveSaveLatest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Date(2026, 10, 21, 8, 30, 0, 0, time.UTC)
	a := &Archive{Dir: dir, Keep: 30 * 24 * time.Hour, Now: func() time.Time { return now }}
	user := uuid.NewV7()
	if err := a.SaveLatest(user, "strong.csv", []byte("an export")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(31 * 24 * time.Hour)
	if err := a.Save(user, "hae.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveLatest(user, "strong.csv", []byte("a newer export")); err != nil {
		t.Fatal(err)
	}
	latest := filepath.Join(dir, user.String(), "latest-strong.csv.gz")
	if got := gunzip(t, latest); got != "a newer export" {
		t.Errorf("the latest = %q", got)
	}
	if info, err := os.Stat(latest); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the latest's mode = %v, %v; want 0600", info, err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, user.String()))
	if len(entries) != 2 {
		t.Errorf("the user's archive holds %v, want today's day and the latest", entries)
	}
	if err := (&Archive{Dir: latest}).SaveLatest(user, "strong.csv", nil); err == nil {
		t.Error("SaveLatest into a file succeeded")
	}
}

func gunzip(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
