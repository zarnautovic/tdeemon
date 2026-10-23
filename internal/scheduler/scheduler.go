// Package scheduler runs the jobs that come due in each user's day, such
// as the morning summary and the watchdog, at most once per occurrence:
// the store records each run, so a restart does not run it again. It also
// keeps the database's query planner statistics fresh.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// A Job is a task that comes due in a user's day.
type Job struct {
	// Name names the job in the record of its runs, 1 to 64 characters.
	Name string
	// Due returns the occurrence due for u at now, a name of at most 64
	// characters such as the date it is for, and whether one is.
	Due func(u user.User, now time.Time) (occurrence string, ok bool)
	// Run does the job. An error leaves the occurrence due, so a later
	// tick tries it again, after RetryAfter.
	Run func(ctx context.Context, u user.User, occurrence string, now time.Time) error
}

// RetryAfter is how long a job that failed for a user waits before the
// scheduler tries it again, so a channel that is down is not called every
// minute.
const RetryAfter = 10 * time.Minute

// OptimizeEvery is how often the scheduler refreshes the query planner's
// statistics (store.DB.Optimize); Migrate does it at the start.
const OptimizeEvery = 24 * time.Hour

// Scheduler runs Jobs for every user.
type Scheduler struct {
	DB     *store.DB
	Jobs   []Job
	Logger *slog.Logger
	// Now is the clock; time.Now when nil.
	Now func() time.Time

	mu sync.Mutex
	// failed holds when a job last failed for a user, by the job's name
	// and the user's id.
	failed map[string]time.Time
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Tick runs every job due now for every user, each occurrence once. A job
// that fails is logged and left due; the error returned is only the
// store's, when it cannot list the users or read and record the runs.
func (s *Scheduler) Tick(ctx context.Context) error {
	users, err := s.DB.Users(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	var errs []error
	for i := range users {
		for _, j := range s.Jobs {
			if err := s.run(ctx, j, &users[i], now); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// run runs j for u when an occurrence is due that has not run, and has
// not failed within RetryAfter, and records it when it succeeds.
func (s *Scheduler) run(ctx context.Context, j Job, u *user.User, now time.Time) error {
	occurrence, ok := j.Due(*u, now)
	if !ok {
		return nil
	}
	key := j.Name + "/" + u.ID.String()
	s.mu.Lock()
	failedAt, failed := s.failed[key]
	s.mu.Unlock()
	if failed && now.Sub(failedAt) < RetryAfter {
		return nil
	}
	ran, err := s.DB.JobRan(ctx, u.ID, j.Name, occurrence)
	if err != nil || ran {
		return err
	}
	if err := j.Run(ctx, *u, occurrence, now); err != nil {
		s.mu.Lock()
		if s.failed == nil {
			s.failed = map[string]time.Time{}
		}
		s.failed[key] = now
		s.mu.Unlock()
		s.Logger.Error("a scheduled job failed", "job", j.Name, "user_id", u.ID, "occurrence", occurrence, "error", err)
		return nil
	}
	s.mu.Lock()
	delete(s.failed, key)
	s.mu.Unlock()
	s.Logger.Info("ran a scheduled job", "job", j.Name, "user_id", u.ID, "occurrence", occurrence)
	return s.DB.RecordJobRun(ctx, u.ID, j.Name, occurrence, now)
}

// Start ticks at once and then every interval, and refreshes the
// statistics every OptimizeEvery, in the background until ctx is done.
// The returned wait blocks until it has stopped; cancel ctx first.
func (s *Scheduler) Start(ctx context.Context, every time.Duration) (wait func()) {
	var running sync.WaitGroup
	running.Go(func() {
		tick := time.NewTicker(every)
		defer tick.Stop()
		optimize := time.NewTicker(OptimizeEvery)
		defer optimize.Stop()
		s.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				s.tick(ctx)
			case <-optimize.C:
				if err := s.DB.Optimize(ctx); err != nil && ctx.Err() == nil {
					s.Logger.Warn("refreshing query planner statistics failed", "error", err)
				}
			}
		}
	})
	return running.Wait
}

func (s *Scheduler) tick(ctx context.Context) {
	if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
		s.Logger.Error("the scheduler's tick failed", "error", err)
	}
}

// DailyAt returns a Job's Due for a job each day at the wall-clock time
// at (an offset into the day, such as 10 hours) in the user's time zone,
// and up to within after it: a server down at that time catches up when
// it starts within that, and later leaves the day out. The occurrence is
// the user's logical day (day.Calendar) the time falls on, so a time
// before the day's start hour belongs to the day before.
func DailyAt(at, within time.Duration) func(u user.User, now time.Time) (string, bool) {
	return func(u user.User, now time.Time) (string, bool) {
		cal, err := day.NewCalendar(u.TimeZone, u.DayStartHour)
		if err != nil {
			return "", false
		}
		d := cal.DayOf(now)
		due := wallClock(cal, d, at)
		if due.Before(cal.Start(d)) {
			// The time comes after midnight, in the day's last hours.
			due = wallClock(cal, d.AddDays(1), at)
		}
		if now.Before(due) || !now.Before(due.Add(within)) {
			return "", false
		}
		return d.String(), true
	}
}

// wallClock returns the instant the wall clock of cal shows the time at
// (its hours and minutes) on the date d, so a day that summer time makes
// shorter or longer keeps the hour.
func wallClock(cal day.Calendar, d day.Date, at time.Duration) time.Time {
	return time.Date(d.Year, d.Month, d.Day, int(at/time.Hour), int(at%time.Hour/time.Minute), 0, 0, cal.Location)
}
