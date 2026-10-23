package scheduler_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/scheduler"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

func newUser(t *testing.T, db *store.DB, email, tz string) {
	t.Helper()
	now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	u := user.User{
		ID: uuid.NewV7(), Email: email, Name: "Ana", Role: user.RoleUser, Locale: "hr",
		TimeZone: tz, DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatal(err)
	}
}

// runs counts a job's runs, by user and occurrence, and fails while fail
// is set.
type runs struct {
	mu   sync.Mutex
	got  []string
	fail bool
}

func (r *runs) job(name string, due func(user.User, time.Time) (string, bool)) scheduler.Job {
	return scheduler.Job{Name: name, Due: due, Run: func(_ context.Context, u user.User, occurrence string, _ time.Time) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.fail {
			return errors.New("the channel is down")
		}
		r.got = append(r.got, u.Email+" "+occurrence)
		return nil
	}}
}

func (r *runs) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.got...)
}

// A job runs once for each occurrence, also across a restart (a new
// scheduler on the same database), and for each user in their own zone.
func TestTickRunsEachOccurrenceOnce(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	newUser(t, db, "ana@example.com", "Europe/Zagreb")
	newUser(t, db, "ivo@example.com", "America/New_York")
	r := &runs{}
	clock := time.Date(2026, 10, 23, 8, 30, 0, 0, time.UTC) // 10:30 in Zagreb, 04:30 in New York
	s := &scheduler.Scheduler{
		DB: db, Jobs: []scheduler.Job{r.job("morning", scheduler.DailyAt(10*time.Hour, 4*time.Hour))},
		Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return clock },
	}
	for range 2 {
		if err := s.Tick(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	restarted := &scheduler.Scheduler{DB: db, Jobs: s.Jobs, Logger: s.Logger, Now: s.Now}
	if err := restarted.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 10, 23, 14, 30, 0, 0, time.UTC) // 10:30 in New York
	if err := restarted.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []string{"ana@example.com 2026-10-23", "ivo@example.com 2026-10-23"}
	if got := r.list(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("runs = %v, want %v", got, want)
	}
}

// A job that fails is left due, and tried again after RetryAfter, not at
// every tick.
func TestTickRetriesAFailedJob(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	newUser(t, db, "ana@example.com", "Europe/Zagreb")
	r := &runs{fail: true}
	clock := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	s := &scheduler.Scheduler{
		DB: db, Jobs: []scheduler.Job{r.job("morning", scheduler.DailyAt(10*time.Hour, 4*time.Hour))},
		Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return clock },
	}
	if err := s.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	r.fail = false
	clock = clock.Add(scheduler.RetryAfter - time.Minute)
	if err := s.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.list(); len(got) != 0 {
		t.Fatalf("tried again before RetryAfter: %v", got)
	}
	clock = clock.Add(time.Minute)
	if err := s.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.list(); len(got) != 1 {
		t.Errorf("runs after RetryAfter = %v, want one", got)
	}
}

// DailyAt is due from its time for within, on the user's wall clock, and
// names the logical day; a time before the day's start hour belongs to
// the day before, and summer time keeps the hour.
func TestDailyAt(t *testing.T) {
	t.Parallel()

	ana := user.User{TimeZone: "Europe/Zagreb", DayStartHour: 4}
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	at := func(y int, m time.Month, d, h, mi int) time.Time { return time.Date(y, m, d, h, mi, 0, 0, zagreb) }
	for _, tc := range []struct {
		name   string
		due    func(user.User, time.Time) (string, bool)
		now    time.Time
		want   string
		wantOK bool
	}{
		{"before", scheduler.DailyAt(10*time.Hour, 4*time.Hour), at(2026, 10, 23, 9, 59), "", false},
		{"at", scheduler.DailyAt(10*time.Hour, 4*time.Hour), at(2026, 10, 23, 10, 0), "2026-10-23", true},
		{"within", scheduler.DailyAt(10*time.Hour, 4*time.Hour), at(2026, 10, 23, 13, 59), "2026-10-23", true},
		{"past within", scheduler.DailyAt(10*time.Hour, 4*time.Hour), at(2026, 10, 23, 14, 0), "", false},
		{"after midnight, the day before", scheduler.DailyAt(2*time.Hour, time.Hour), at(2026, 10, 24, 2, 30), "2026-10-23", true},
		{"the day summer time ends", scheduler.DailyAt(10*time.Hour, time.Hour), at(2026, 10, 25, 10, 0), "2026-10-25", true},
		{"a zone that does not read", scheduler.DailyAt(10*time.Hour, 4*time.Hour), at(2026, 10, 23, 11, 0), "", false},
	} {
		u := ana
		if tc.name == "a zone that does not read" {
			u.TimeZone = "Nowhere/Bogus"
		}
		got, ok := tc.due(u, tc.now)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("%s: = %q, %v, want %q, %v", tc.name, got, ok, tc.want, tc.wantOK)
		}
	}
}

// Start ticks at once, and stops when its context is done.
func TestStart(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	newUser(t, db, "ana@example.com", "Europe/Zagreb")
	r := &runs{}
	always := func(user.User, time.Time) (string, bool) { return "once", true }
	s := &scheduler.Scheduler{DB: db, Jobs: []scheduler.Job{r.job("always", always)}, Logger: slog.New(slog.DiscardHandler)}
	ctx, cancel := context.WithCancel(t.Context())
	wait := s.Start(ctx, time.Hour)
	deadline := time.Now().Add(5 * time.Second)
	for len(r.list()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no tick at the start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	wait()
	if got := r.list(); len(got) != 1 {
		t.Errorf("runs = %v, want one", got)
	}
}
