package api_test

import (
	"bytes"
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

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// botMessage is what the fake Bot API was sent.
type botMessage struct {
	chat int64
	text string
}

// commandsEnv is a database where Ana (hr) has the chat 853 and slept
// 7.5 h last night, and Ivo (en) and Mia (hr) share the group -100123;
// at 13:05 in Zagreb on 23 October 2026.
type commandsEnv struct {
	db   *store.DB
	ana  user.User
	now  time.Time
	logs *bytes.Buffer
	// ask has the commands answer text sent in chat age ago, and returns
	// what was sent.
	ask func(chat int64, text string, age time.Duration) []botMessage
}

func newCommandsEnv(t *testing.T) commandsEnv {
	t.Helper()
	var mu sync.Mutex
	var sent []botMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		sent = append(sent, botMessage{body.ChatID, body.Text})
		mu.Unlock()
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	db := storetest.Open(t)
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	now := time.Date(2026, 10, 23, 13, 5, 0, 0, zagreb)
	newU := func(email, locale string, chat int64) user.User {
		u := user.User{
			ID: uuid.NewV7(), Email: email, Name: "Ana", Role: user.RoleUser, Locale: locale,
			TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
		}
		if err := db.CreateUser(t.Context(), u, ""); err != nil {
			t.Fatal(err)
		}
		if err := db.SetTelegramChat(t.Context(), u.ID, chat, now); err != nil {
			t.Fatal(err)
		}
		return u
	}
	ana := newU("ana@example.com", "hr", 853)
	newU("ivo@example.com", "en", -100123)
	newU("mia@example.com", "hr", -100123)
	if _, err := db.MergeMetrics(t.Context(), ana.ID, isolationActor, day.Date{Year: 2026, Month: 10, Day: 23}, []metric.Value{
		{Kind: metric.Sleep, Value: 7.5, Source: metric.Import},
	}, now); err != nil {
		t.Fatal(err)
	}

	logs := &bytes.Buffer{}
	handle := api.TelegramCommands(api.Config{DB: db, Logger: slog.New(slog.NewTextHandler(logs, nil)), Now: func() time.Time { return now }},
		&telegram.Bot{Token: "123456:made-up-token-for-tests-only-0123", BaseURL: srv.URL})
	return commandsEnv{db: db, ana: ana, now: now, logs: logs, ask: func(chat int64, text string, age time.Duration) []botMessage {
		t.Helper()
		mu.Lock()
		sent = nil
		mu.Unlock()
		handle(t.Context(), telegram.Update{ID: 1, ChatID: chat, Text: text, Sent: now.Add(-age)})
		mu.Lock()
		defer mu.Unlock()
		return sent
	}}
}

// A command in a user's chat is answered there, in the user's language:
// /sazetak and /summary with the morning summary as it is now, anything
// else with the commands. A command writes nothing.
func TestTelegramCommands(t *testing.T) {
	t.Parallel()

	env := newCommandsEnv(t)
	summary := "Jutarnji sažetak, petak 23. 10.\nSan: 7 h 30 min"
	for _, text := range []string{"/sazetak", "/summary", " /SAZETAK@tdeemon_test_bot ", "/sazetak danas"} {
		if got := env.ask(853, text, time.Minute); len(got) != 1 || got[0].chat != 853 || !strings.HasPrefix(got[0].text, summary) {
			t.Errorf("%q = %+v, want Ana's summary", text, got)
		}
	}
	for _, text := range []string{"/pomoc", "/help", "/start", "sažetak", "/nepoznata", ""} {
		if got := env.ask(853, text, 0); len(got) != 1 || got[0].chat != 853 || !strings.HasPrefix(got[0].text, "Naredbe:\n/sazetak") {
			t.Errorf("%q = %+v, want the commands in Croatian", text, got)
		}
	}
	// The morning summary of the day is still to be sent.
	if ran, err := env.db.JobRan(t.Context(), env.ana.ID, "morning_summary", "2026-10-23"); ran || err != nil {
		t.Errorf("the morning summary ran = %v, %v", ran, err)
	}
}

// A group's command is answered for each of its users, with their own
// summary in their own language.
func TestTelegramCommandsInAGroup(t *testing.T) {
	t.Parallel()

	env := newCommandsEnv(t)
	got := env.ask(-100123, "/help", 0)
	if len(got) != 2 || got[0].chat != -100123 || !strings.HasPrefix(got[0].text, "Commands:\n/summary") ||
		got[1].chat != -100123 || !strings.HasPrefix(got[1].text, "Naredbe:\n/sazetak") {
		t.Errorf("/help = %+v, want Ivo's in English and Mia's in Croatian", got)
	}
	got = env.ask(-100123, "/summary", 0)
	if len(got) != 2 || !strings.HasPrefix(got[0].text, "Morning summary, Friday Oct 23") ||
		!strings.HasPrefix(got[1].text, "Jutarnji sažetak") || strings.Contains(got[1].text, "San:") {
		t.Errorf("/summary = %+v, want each user's own, without Ana's sleep", got)
	}
}

// A chat that is no user's gets nothing, and its id is logged for the
// admin; a command older than 5 minutes gets nothing.
func TestTelegramCommandsIgnored(t *testing.T) {
	t.Parallel()

	env := newCommandsEnv(t)
	if got := env.ask(999, "/sazetak", 0); got != nil {
		t.Errorf("a chat that is no user's got %+v", got)
	}
	if !strings.Contains(env.logs.String(), "chat_id=999") {
		t.Errorf("the chat that is no user's is not logged:\n%s", env.logs.String())
	}
	if got := env.ask(853, "/sazetak", 6*time.Minute); got != nil {
		t.Errorf("a command 6 minutes old got %+v", got)
	}
	if got := env.ask(853, "/sazetak", 4*time.Minute); len(got) != 1 {
		t.Errorf("a command 4 minutes old got %+v", got)
	}
}
