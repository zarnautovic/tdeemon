package store_test

import (
	"testing"
	"time"

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
	if err := db.Optimize(t.Context()); err != nil {
		t.Errorf("Optimize: %v", err)
	}
}
