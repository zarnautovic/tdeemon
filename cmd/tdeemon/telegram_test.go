package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

const testBotToken = "123456:made-up-token-for-tests-only-0123"

// message is what a fake Bot API was sent.
type message struct {
	chat float64
	text string
}

// fakeTelegram answers sendMessage with status and answer, and keeps the
// messages it was sent.
func fakeTelegram(t *testing.T, status int, answer string) (url string, messages func() []message) {
	t.Helper()
	var mu sync.Mutex
	var got []message
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		chat, _ := body["chat_id"].(float64)
		text, _ := body["text"].(string)
		got = append(got, message{chat, text})
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []message { mu.Lock(); defer mu.Unlock(); return append([]message(nil), got...) }
}

// telegramChat reads back Ana's Telegram chat.
func telegramChat(t *testing.T, dbPath string) (int64, bool) {
	t.Helper()
	u, _ := stored(t, dbPath, "ana@example.com")
	db, err := store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	chat, ok, err := db.TelegramChat(t.Context(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return chat, ok
}

// user set-telegram sends the chat a message first, and sets it only when
// it took it; without a bot it sets it and says so; 0 removes it.
func TestSetTelegram(t *testing.T) {
	t.Parallel()

	api, sent := fakeTelegram(t, http.StatusOK, `{"ok":true}`)
	env, dbPath := instance(t, map[string]string{"TDEEMON_TELEGRAM_BOT_TOKEN": testBotToken, "TDEEMON_TELEGRAM_API_URL": api})
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "853"}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	msgs := sent()
	if chat, ok := telegramChat(t, dbPath); !ok || chat != 853 || len(msgs) != 1 || msgs[0].chat != 853 || !strings.Contains(out.String(), "sent it a message") {
		t.Errorf("chat = %d, %v; sent %v; said %q", chat, ok, msgs, out.String())
	}
	// Ana's locale is English, the default.
	if len(msgs) == 1 && !strings.HasPrefix(msgs[0].text, "tdeemon is connected") {
		t.Errorf("hello = %q", msgs[0].text)
	}

	out.Reset()
	if err := run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "0"}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, ok := telegramChat(t, dbPath); ok || !strings.Contains(out.String(), "Removed") {
		t.Errorf("after 0: set %v; said %q", ok, out.String())
	}
}

// A chat that refuses the message is not set, and neither is one while
// Telegram does not answer, which the error tells apart.
func TestSetTelegramFails(t *testing.T) {
	t.Parallel()

	refusing, _ := fakeTelegram(t, http.StatusBadRequest, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
	refused, refusedDB := instance(t, map[string]string{"TDEEMON_TELEGRAM_BOT_TOKEN": testBotToken, "TDEEMON_TELEGRAM_API_URL": refusing})
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, refused, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	err := run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "999"}, refused, noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "chat not found") || strings.Contains(err.Error(), testBotToken) {
		t.Errorf("a chat that refuses: %v", err)
	}
	if _, ok := telegramChat(t, refusedDB); ok {
		t.Error("a chat that refused the message was set")
	}

	// A Telegram that does not answer is not the chat's fault.
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	down, _ := instance(t, map[string]string{"TDEEMON_TELEGRAM_BOT_TOKEN": testBotToken, "TDEEMON_TELEGRAM_API_URL": gone.URL})
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, down, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	err = run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "853"}, down, noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "telegram did not answer") {
		t.Errorf("a Telegram that does not answer: %v", err)
	}
}

// Without a bot, user set-telegram sets the chat and says no message went.
func TestSetTelegramWithoutABot(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "-100123"}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if chat, ok := telegramChat(t, dbPath); !ok || chat != -100123 || !strings.Contains(out.String(), "TDEEMON_TELEGRAM_BOT_TOKEN is not set") {
		t.Errorf("chat = %d, %v; said %q", chat, ok, out.String())
	}
	err := run(t.Context(), []string{"user", "set-telegram", "--email", "ivo@example.com", "--chat-id", "1"}, env, noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no user") {
		t.Errorf("no such user: %v", err)
	}
	for name, args := range map[string][]string{
		"no chat":                     {"--email", "ana@example.com"},
		"no email":                    {"--chat-id", "1"},
		"a chat id that is no number": {"--email", "ana@example.com", "--chat-id", "@ana"},
		"an argument":                 {"--email", "ana@example.com", "--chat-id", "1", "more"},
		"an unknown flag":             {"--email", "ana@example.com", "--chat", "1"},
	} {
		err := run(t.Context(), append([]string{"user", "set-telegram"}, args...), env, noInput(), io.Discard, io.Discard)
		if !errors.Is(err, errUsage) {
			t.Errorf("%s: %v, want a usage error", name, err)
		}
	}
}

// The hello is in the user's locale, and English for another.
func TestHelloIn(t *testing.T) {
	t.Parallel()

	for locale, want := range map[string]string{"hr": "tdeemon je spojen", "en": "tdeemon is connected", "de": "tdeemon is connected"} {
		if got := helloIn(locale); !strings.HasPrefix(got, want) {
			t.Errorf("%s: %q", locale, got)
		}
	}
}

// The hello is in the user's locale: Croatian for hr.
func TestSetTelegramInTheUsersLocale(t *testing.T) {
	t.Parallel()

	api, sent := fakeTelegram(t, http.StatusOK, `{"ok":true}`)
	env, dbPath := instance(t, map[string]string{"TDEEMON_TELEGRAM_BOT_TOKEN": testBotToken, "TDEEMON_TELEGRAM_API_URL": api})
	db, err := store.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ana := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser, Locale: "hr",
		TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	err = db.CreateUser(t.Context(), ana, "")
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "853"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if msgs := sent(); len(msgs) != 1 || !strings.HasPrefix(msgs[0].text, "tdeemon je spojen") {
		t.Errorf("sent = %v", msgs)
	}
}
