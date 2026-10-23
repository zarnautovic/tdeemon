// Package watchdog tells a user when their data stop coming: no metric
// for more than NoMetricsAfter, or an ingest token unused for more than
// TokenUnusedAfter. It is the scheduler's job, each hour; an alarm goes
// out once for each stop, not again until data come and stop again.
package watchdog

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/notify"
	"github.com/zarnautovic/tdeemon/internal/scheduler"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// NoMetricsAfter is how long without a metric before the alarm: a day of
// syncs missed, and two hours for a phone that syncs late.
const NoMetricsAfter = 26 * time.Hour

// TokenUnusedAfter is how long an ingest token may go unused before the
// alarm: the device that sends with it has stopped.
const TokenUnusedAfter = 72 * time.Hour

// alarmJob is the name the store records an alarm sent under, with the
// stop it is of as the occurrence, so each stop alarms once.
const alarmJob = "watchdog_alarm"

// Job is the watchdog, the scheduler's job each hour. logger takes what
// it does not return: a chat that blocked the bot. It keeps in memory the
// alarms it sent whose record failed, so it does not send them again.
func Job(db *store.DB, bot *telegram.Bot, logger *slog.Logger) scheduler.Job {
	if logger == nil {
		logger = slog.Default()
	}
	w := &watcher{db: db, bot: bot, logger: logger, sent: map[string]bool{}}
	return scheduler.Job{
		Name: "watchdog",
		// Each hour, by the clock, whatever the user's zone.
		Due: func(_ user.User, now time.Time) (string, bool) { return now.UTC().Format("2006-01-02T15"), true },
		Run: w.run,
	}
}

// keepRuns is how long the watchdog's record of its hourly runs is kept:
// long enough for a restart not to run an hour again.
const keepRuns = 48 * time.Hour

// recordTimeout bounds the record of an alarm sent, which is written even
// as the scheduler stops.
const recordTimeout = 10 * time.Second

type watcher struct {
	db     *store.DB
	bot    *telegram.Bot
	logger *slog.Logger

	mu sync.Mutex
	// sent holds the alarms sent whose record failed, by user and key.
	sent map[string]bool
}

// alarm is one stop of a user's data: what to say, and the stop's key.
type alarm struct {
	key, text string
}

func (w *watcher) run(ctx context.Context, u user.User, _ string, now time.Time) error {
	if err := w.db.DeleteJobRuns(ctx, u.ID, "watchdog", now.Add(-keepRuns)); err != nil {
		return err
	}
	alarms, err := alarmsOf(ctx, w.db, u, now)
	if err != nil {
		return err
	}
	for _, a := range alarms {
		if err := w.raise(ctx, u, a, now); err != nil {
			return err
		}
	}
	return nil
}

// raise sends a once: not when it was sent before, nor to a user without
// a chat, whom it is kept for. A chat that blocked the bot counts it sent.
func (w *watcher) raise(ctx context.Context, u user.User, a alarm, now time.Time) error {
	key := u.ID.String() + "/" + a.key
	w.mu.Lock()
	sentHere := w.sent[key]
	w.mu.Unlock()
	if sentHere {
		return nil
	}
	if sent, err := w.db.JobRan(ctx, u.ID, alarmJob, a.key); err != nil || sent {
		return err
	}
	sent, err := notify.Send(ctx, w.db, w.bot, u, a.text, now)
	switch {
	case notify.Blocked(err):
		w.logger.Warn("the watchdog's chat refuses the bot", "user_id", u.ID, "error", err)
	case err != nil || !sent:
		return err
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	if err := w.db.RecordJobRun(rctx, u.ID, alarmJob, a.key, now); err != nil {
		w.mu.Lock()
		w.sent[key] = true
		w.mu.Unlock()
		w.logger.Error("recording a watchdog alarm failed", "user_id", u.ID, "error", err)
	}
	return nil
}

// alarmsOf returns u's stops at now, each with the key of when the data
// last came, so the same stop keys the same.
func alarmsOf(ctx context.Context, db *store.DB, u user.User, now time.Time) ([]alarm, error) {
	w := phrasesOf(u.Locale)
	var out []alarm
	last, ok, err := db.LastMetricUpdate(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if ok && now.Sub(last) > NoMetricsAfter {
		out = append(out, alarm{
			key:  "metrics:" + last.UTC().Format(time.RFC3339),
			text: fmt.Sprintf(w.noMetrics, hours(now.Sub(last))),
		})
	}
	tokens, err := db.APITokens(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, t := range tokens {
		scopes, err := auth.ParseScopes(t.Scopes)
		if err != nil || !slices.Contains(scopes, auth.ScopeIngest) {
			continue
		}
		used := t.LastUsedAt
		if used.IsZero() {
			used = t.CreatedAt
		}
		if now.Sub(used) > TokenUnusedAfter {
			out = append(out, alarm{
				key:  "token:" + t.ID.String() + ":" + used.UTC().Format("20060102T150405"),
				text: fmt.Sprintf(w.silentDevice, t.Name, w.days(days(now.Sub(used)))),
			})
		}
	}
	return out, nil
}

func hours(d time.Duration) int { return int(d / time.Hour) }
func days(d time.Duration) int  { return int(d / (24 * time.Hour)) }

// phrases are the alarms' words in a language.
type phrases struct {
	noMetrics, silentDevice string
	// days writes a number of days with its word.
	days func(n int) string
}

// phrasesOf returns the phrases of locale: Croatian for hr, English for
// any other.
func phrasesOf(locale string) phrases {
	if locale == "hr" {
		return phrases{
			noMetrics:    "Upozorenje: tdeemon već %d h nije primio nijednu metriku. Provjeri Health Auto Export na telefonu.",
			silentDevice: "Upozorenje: token %q nije korišten %s. Je li uređaj koji šalje s njim stao?",
			days: func(n int) string {
				if n%10 == 1 && n%100 != 11 {
					return fmt.Sprintf("%d dan", n)
				}
				return fmt.Sprintf("%d dana", n)
			},
		}
	}
	return phrases{
		noMetrics:    "Alert: tdeemon has had no metric for %d h. Check Health Auto Export on the phone.",
		silentDevice: "Alert: the token %q has not been used for %s. Has the device that sends with it stopped?",
		days: func(n int) string {
			if n == 1 {
				return "1 day"
			}
			return fmt.Sprintf("%d days", n)
		},
	}
}
