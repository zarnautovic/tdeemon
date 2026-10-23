package watchdog_test

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
	"github.com/zarnautovic/tdeemon/internal/telegram"
	"github.com/zarnautovic/tdeemon/internal/watchdog"
)

var actor = event.Actor{Kind: event.User, Client: "web"}

// fakeBot returns a bot whose Bot API keeps the texts it was sent.
func fakeBot(t *testing.T) (*telegram.Bot, func() []string) {
	t.Helper()
	return fakeBotAnswering(t, http.StatusOK, `{"ok":true}`)
}

// fakeBotAnswering is fakeBot with the Bot API's answer.
func fakeBotAnswering(t *testing.T, status int, answer string) (*telegram.Bot, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		text, _ := body["text"].(string)
		mu.Lock()
		texts = append(texts, text)
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return &telegram.Bot{Token: "123456:made-up-token-for-tests-only-0123", BaseURL: srv.URL},
		func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), texts...) }
}

func newUser(t *testing.T, db *store.DB, email, locale string, chat int64) user.User {
	t.Helper()
	now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	u := user.User{
		ID: uuid.NewV7(), Email: email, Name: "Ana", Role: user.RoleUser, Locale: locale,
		TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatal(err)
	}
	if chat != 0 {
		if err := db.SetTelegramChat(t.Context(), u.ID, chat, now); err != nil {
			t.Fatal(err)
		}
	}
	return u
}

func weigh(t *testing.T, db *store.DB, u user.User, kg float64, at time.Time) {
	t.Helper()
	if _, err := db.MergeMetrics(t.Context(), u.ID, actor, day.DateOf(at), []metric.Value{{Kind: metric.Weight, Value: kg, Source: metric.Import}}, at); err != nil {
		t.Fatal(err)
	}
}

// No metric for more than 26 hours alarms once; a metric that comes and
// stops again alarms again.
func TestNoMetrics(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	bot, sent := fakeBot(t)
	u := newUser(t, db, "ana@example.com", "hr", 853)
	job := watchdog.Job(db, bot, slog.New(slog.DiscardHandler))
	last := time.Date(2026, 10, 22, 6, 0, 0, 0, time.UTC)
	weigh(t, db, u, 80, last)
	run := func(at time.Time) {
		t.Helper()
		if err := job.Run(t.Context(), u, "", at); err != nil {
			t.Fatal(err)
		}
	}
	run(last.Add(watchdog.NoMetricsAfter))
	if got := sent(); len(got) != 0 {
		t.Fatalf("an alarm at 26 h: %v", got)
	}
	run(last.Add(watchdog.NoMetricsAfter + time.Hour))
	run(last.Add(watchdog.NoMetricsAfter + 2*time.Hour))
	got := sent()
	if len(got) != 1 || !strings.Contains(got[0], "već 27 h nije primio nijednu metriku") {
		t.Fatalf("alarms = %q, want one, in Croatian", got)
	}

	again := last.Add(48 * time.Hour)
	weigh(t, db, u, 79.8, again)
	run(again.Add(time.Hour))
	run(again.Add(watchdog.NoMetricsAfter + time.Hour))
	if got := sent(); len(got) != 2 {
		t.Errorf("alarms after data came and stopped again = %q, want two", got)
	}
}

// An ingest token unused for more than 3 days alarms, once; a token of
// another scope does not; a user without a chat gets nothing.
func TestTokenUnused(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	bot, sent := fakeBot(t)
	u := newUser(t, db, "ana@example.com", "en", 853)
	silent := newUser(t, db, "ivo@example.com", "en", 0)
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	for i, tc := range []struct {
		name, scopes string
		owner        user.User
	}{{"iPhone", "ingest", u}, {"Hermes", "tools", u}, {"iPad", "ingest", silent}} {
		hash := strings.Repeat("0", 63) + string(rune('a'+i))
		tok := store.APIToken{ID: uuid.NewV7(), UserID: tc.owner.ID, Name: tc.name, Scopes: tc.scopes, CreatedAt: created}
		if err := db.CreateAPIToken(t.Context(), tok, hash); err != nil {
			t.Fatal(err)
		}
		if err := db.TouchAPIToken(t.Context(), tc.owner.ID, tok.ID, created.Add(24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	job := watchdog.Job(db, bot, slog.New(slog.DiscardHandler))
	if err := job.Run(t.Context(), u, "", created.Add(24*time.Hour+watchdog.TokenUnusedAfter)); err != nil {
		t.Fatal(err)
	}
	if got := sent(); len(got) != 0 {
		t.Fatalf("an alarm at exactly 3 days: %q", got)
	}
	now := created.Add(24*time.Hour + watchdog.TokenUnusedAfter + time.Hour)
	for _, who := range []user.User{u, u, silent} {
		if err := job.Run(t.Context(), who, "", now); err != nil {
			t.Fatal(err)
		}
	}
	got := sent()
	if len(got) != 1 || !strings.Contains(got[0], `the token "iPhone" has not been used for 3 days`) {
		t.Errorf("alarms = %q, want one, of the iPhone's token, in English", got)
	}
}

// The watchdog is due each hour, by the clock.
func TestDue(t *testing.T) {
	t.Parallel()

	job := watchdog.Job(nil, nil, nil)
	a, _ := job.Due(user.User{}, time.Date(2026, 10, 23, 10, 5, 0, 0, time.UTC))
	b, _ := job.Due(user.User{}, time.Date(2026, 10, 23, 10, 55, 0, 0, time.UTC))
	c, ok := job.Due(user.User{}, time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC))
	if a != b || a == c || !ok {
		t.Errorf("occurrences %q %q %q", a, b, c)
	}
}

// A user without a chat gets no alarm, and none is kept as sent: one who
// sets a chat later gets the alarm of a stop that goes on.
func TestAlarmAfterAChatIsSet(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	bot, sent := fakeBot(t)
	u := newUser(t, db, "ana@example.com", "en", 0)
	last := time.Date(2026, 10, 22, 6, 0, 0, 0, time.UTC)
	weigh(t, db, u, 80, last)
	job := watchdog.Job(db, bot, slog.New(slog.DiscardHandler))
	at := last.Add(watchdog.NoMetricsAfter + time.Hour)
	if err := job.Run(t.Context(), u, "", at); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTelegramChat(t.Context(), u.ID, 853, at); err != nil {
		t.Fatal(err)
	}
	if err := job.Run(t.Context(), u, "", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := sent(); len(got) != 1 || !strings.Contains(got[0], "no metric for 28 h") {
		t.Errorf("alarms = %q", got)
	}
}

// A token never used counts from its creation: no alarm before 3 days,
// one after.
func TestTokenNeverUsed(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	bot, sent := fakeBot(t)
	u := newUser(t, db, "ana@example.com", "hr", 853)
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	tok := store.APIToken{ID: uuid.NewV7(), UserID: u.ID, Name: "iPhone", Scopes: "ingest", CreatedAt: created}
	if err := db.CreateAPIToken(t.Context(), tok, strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	job := watchdog.Job(db, bot, slog.New(slog.DiscardHandler))
	for _, at := range []time.Time{created.Add(time.Hour), created.Add(watchdog.TokenUnusedAfter), created.Add(21*24*time.Hour + time.Hour)} {
		if err := job.Run(t.Context(), u, "", at); err != nil {
			t.Fatal(err)
		}
	}
	if got := sent(); len(got) != 1 || !strings.Contains(got[0], `token "iPhone" nije korišten 21 dan.`) {
		t.Errorf("alarms = %q", got)
	}
}

// A chat that blocked the bot counts the alarm sent: it is not tried
// again, and the job does not fail.
func TestBlockedBot(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	bot, sent := fakeBotAnswering(t, http.StatusForbidden, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`)
	u := newUser(t, db, "ana@example.com", "en", 853)
	last := time.Date(2026, 10, 22, 6, 0, 0, 0, time.UTC)
	weigh(t, db, u, 80, last)
	job := watchdog.Job(db, bot, slog.New(slog.DiscardHandler))
	for h := range 3 {
		if err := job.Run(t.Context(), u, "", last.Add(watchdog.NoMetricsAfter+time.Duration(h+1)*time.Hour)); err != nil {
			t.Fatalf("run %d: %v", h, err)
		}
	}
	if got := sent(); len(got) != 1 {
		t.Errorf("tries = %d, want one", len(got))
	}
}

// An alarm sent whose record fails is not sent again by the same job.
func TestAlarmRecordFails(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	bot, sent := fakeBot(t)
	u := newUser(t, db, "ana@example.com", "en", 853)
	last := time.Date(2026, 10, 22, 6, 0, 0, 0, time.UTC)
	weigh(t, db, u, 80, last)
	err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER full BEFORE INSERT ON job_runs
			WHEN NEW.job = 'watchdog_alarm' BEGIN SELECT RAISE(ABORT, 'database or disk is full'); END`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	job := watchdog.Job(db, bot, slog.New(slog.DiscardHandler))
	for h := range 3 {
		if err := job.Run(t.Context(), u, "", last.Add(watchdog.NoMetricsAfter+time.Duration(h+1)*time.Hour)); err != nil {
			t.Fatalf("run %d: %v", h, err)
		}
	}
	if got := sent(); len(got) != 1 {
		t.Errorf("alarms = %d, want one", len(got))
	}
}
