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

	"github.com/zarnautovic/tdeemon/internal/store"
)

const testBotToken = "123456:made-up-token-for-tests-only-0123"

// fakeTelegram answers sendMessage with status and answer, and keeps the
// chats it was sent to.
func fakeTelegram(t *testing.T, status int, answer string) (url string, chats func() []float64) {
	t.Helper()
	var mu sync.Mutex
	var got []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		chat, _ := body["chat_id"].(float64)
		got = append(got, chat)
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []float64 { mu.Lock(); defer mu.Unlock(); return append([]float64(nil), got...) }
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

	api, chats := fakeTelegram(t, http.StatusOK, `{"ok":true}`)
	env, dbPath := instance(t, map[string]string{"TDEEMON_TELEGRAM_BOT_TOKEN": testBotToken, "TDEEMON_TELEGRAM_API_URL": api})
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "853"}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if chat, ok := telegramChat(t, dbPath); !ok || chat != 853 || len(chats()) != 1 || !strings.Contains(out.String(), "sent it a message") {
		t.Errorf("chat = %d, %v; sent to %v; said %q", chat, ok, chats(), out.String())
	}

	refusing, _ := fakeTelegram(t, http.StatusBadRequest, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
	refused, refusedDB := instance(t, map[string]string{"TDEEMON_TELEGRAM_BOT_TOKEN": testBotToken, "TDEEMON_TELEGRAM_API_URL": refusing})
	_ = run(t.Context(), []string{"user", "create", "--email", "ana@example.com"}, refused, noInput(), io.Discard, io.Discard)
	err := run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "999"}, refused, noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "chat not found") || strings.Contains(err.Error(), testBotToken) {
		t.Errorf("a chat that refuses: %v", err)
	}
	if _, ok := telegramChat(t, refusedDB); ok {
		t.Error("a chat that refused the message was set")
	}

	out.Reset()
	if err := run(t.Context(), []string{"user", "set-telegram", "--email", "ana@example.com", "--chat-id", "0"}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, ok := telegramChat(t, dbPath); ok || !strings.Contains(out.String(), "Removed") {
		t.Errorf("after 0: set %v; said %q", ok, out.String())
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
