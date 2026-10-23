// Package watchdog tells a user when their data stop coming: no metric
// for more than NoMetricsAfter, or an ingest token unused for more than
// TokenUnusedAfter. It is the scheduler's job, each hour; an alarm goes
// out once for each stop, not again until data come and stop again.
package watchdog

import (
	"context"
	"fmt"
	"slices"
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

// Job is the watchdog, the scheduler's job each hour.
func Job(db *store.DB, bot *telegram.Bot) scheduler.Job {
	return scheduler.Job{
		Name: "watchdog",
		// Each hour, by the clock, whatever the user's zone.
		Due: func(_ user.User, now time.Time) (string, bool) { return now.UTC().Format("2006-01-02T15"), true },
		Run: func(ctx context.Context, u user.User, _ string, now time.Time) error {
			return run(ctx, db, bot, u, now)
		},
	}
}

// alarm is one stop of a user's data: what to say, and the stop's key.
type alarm struct {
	key, text string
}

func run(ctx context.Context, db *store.DB, bot *telegram.Bot, u user.User, now time.Time) error {
	if _, ok, err := db.TelegramChat(ctx, u.ID); err != nil || !ok {
		return err
	}
	alarms, err := alarmsOf(ctx, db, u, now)
	if err != nil {
		return err
	}
	for _, a := range alarms {
		sent, err := db.JobRan(ctx, u.ID, alarmJob, a.key)
		if err != nil || sent {
			if err != nil {
				return err
			}
			continue
		}
		if _, err := notify.Send(ctx, db, bot, u, a.text, now); err != nil {
			return err
		}
		if err := db.RecordJobRun(context.WithoutCancel(ctx), u.ID, alarmJob, a.key, now); err != nil {
			return err
		}
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
				text: fmt.Sprintf(w.silentDevice, t.Name, days(now.Sub(used))),
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
}

// phrasesOf returns the phrases of locale: Croatian for hr, English for
// any other.
func phrasesOf(locale string) phrases {
	if locale == "hr" {
		return phrases{
			noMetrics:    "Upozorenje: tdeemon već %d h nije primio nijednu metriku. Provjeri Health Auto Export na telefonu.",
			silentDevice: "Upozorenje: token %q nije korišten %d dana. Uređaj koji šalje s njim je stao?",
		}
	}
	return phrases{
		noMetrics:    "Alert: tdeemon has had no metric for %d h. Check Health Auto Export on the phone.",
		silentDevice: "Alert: the token %q has not been used for %d days. Has the device that sends with it stopped?",
	}
}
