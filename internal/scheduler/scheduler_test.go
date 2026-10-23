package scheduler_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

// Start ticks at once, before its interval.
func TestStartTicksAtOnce(t *testing.T) {
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
}

// A job whose name or occurrence the record would refuse does not run.
func TestTickRefusesALongName(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	newUser(t, db, "ana@example.com", "Europe/Zagreb")
	r := &runs{}
	long := func(user.User, time.Time) (string, bool) { return strings.Repeat("x", 65), true }
	unnamed := r.job("", func(user.User, time.Time) (string, bool) { return "x", true })
	s := &scheduler.Scheduler{DB: db, Jobs: []scheduler.Job{r.job("long", long), unnamed}, Logger: slog.New(slog.DiscardHandler)}
	if err := s.Tick(t.Context()); err == nil || !strings.Contains(err.Error(), "1 to 64") {
		t.Errorf("Tick = %v", err)
	}
	if got := r.list(); len(got) != 0 {
		t.Errorf("runs = %v, want none", got)
	}
}

// Start ticks every interval, and stops when its context is done.
func TestStart(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	newUser(t, db, "ana@example.com", "Europe/Zagreb")
	r := &runs{}
	var mu sync.Mutex
	n := 0
	// A new occurrence at every tick, so each tick runs one.
	each := func(user.User, time.Time) (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprint(n), true
	}
	s := &scheduler.Scheduler{DB: db, Jobs: []scheduler.Job{r.job("each", each)}} // the default logger
	ctx, cancel := context.WithCancel(t.Context())
	wait := s.Start(ctx, 20*time.Millisecond)
	deadline := time.Now().Add(5 * time.Second)
	for len(r.list()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("runs = %v, want a tick at the start and another", r.list())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	wait()
}

// A run whose record fails is not run again by the same scheduler, and
// one that went out as the scheduler stops is recorded all the same.
func TestTickAfterARun(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	newUser(t, db, "ana@example.com", "Europe/Zagreb")
	err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER full BEFORE INSERT ON job_runs
			WHEN NEW.job = 'morning' BEGIN SELECT RAISE(FAIL, 'database or disk is full'); END`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r := &runs{}
	clock := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	daily := scheduler.DailyAt(10*time.Hour, 4*time.Hour)
	s := &scheduler.Scheduler{DB: db, Jobs: []scheduler.Job{r.job("morning", daily)}, Now: func() time.Time { return clock }}
	if err := s.Tick(t.Context()); err == nil {
		t.Error("a record that failed was no error")
	}
	for range 3 {
		clock = clock.Add(scheduler.RetryAfter)
		_ = s.Tick(t.Context())
	}
	if got := r.list(); len(got) != 1 {
		t.Errorf("runs with a record that fails = %v, want one", got)
	}

	// The run stops the scheduler, as a shutdown would while it sends.
	ctx, cancel := context.WithCancel(t.Context())
	var logs bytes.Buffer
	stopping := &scheduler.Scheduler{
		DB: db, Logger: slog.New(slog.NewTextHandler(&logs, nil)), Now: s.Now,
		Jobs: []scheduler.Job{{Name: "evening", Due: daily, Run: func(context.Context, user.User, string, time.Time) error {
			cancel()
			return nil
		}}},
	}
	if err := stopping.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	us, _ := db.Users(t.Context())
	if ran, err := db.JobRan(t.Context(), us[0].ID, "evening", "2026-10-23"); !ran || err != nil {
		t.Errorf("the run as it stopped was not recorded: %v, %v", ran, err)
	}

	// A run that fails as the scheduler stops is no error to log.
	ctx, cancel = context.WithCancel(t.Context())
	failing := &scheduler.Scheduler{
		DB: db, Logger: stopping.Logger, Now: s.Now,
		Jobs: []scheduler.Job{{Name: "night", Due: daily, Run: func(context.Context, user.User, string, time.Time) error {
			cancel()
			return context.Canceled
		}}},
	}
	_ = failing.Tick(ctx)
	if strings.Contains(logs.String(), "failed") {
		t.Errorf("logs = %s", logs.String())
	}
}
