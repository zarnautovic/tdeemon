package notify_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/notify"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// Send sends to the user's chat; a user without one gets nothing; a group
// that became a supergroup gets the text in the new chat, which becomes
// the user's.
func TestSend(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var chats []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		chat, _ := body["chat_id"].(float64)
		mu.Lock()
		chats = append(chats, chat)
		mu.Unlock()
		if chat == -100 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: group chat was upgraded to a supergroup chat","parameters":{"migrate_to_chat_id":-1001234}}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	bot := &telegram.Bot{Token: "123456:made-up-token-for-tests-only-0123", BaseURL: srv.URL}

	db := storetest.Open(t)
	now := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	newUser := func(email string) user.User {
		u := user.User{
			ID: uuid.NewV7(), Email: email, Name: "Ana", Role: user.RoleUser, Locale: "en",
			TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
		}
		if err := db.CreateUser(t.Context(), u, ""); err != nil {
			t.Fatal(err)
		}
		return u
	}
	ana, ivo := newUser("ana@example.com"), newUser("ivo@example.com")
	if ok, err := (notify.Channels{DB: db, Bot: bot}).Send(t.Context(), ivo, notify.Note{Text: "x"}, now); ok || err != nil {
		t.Errorf("a user without a chat = %v, %v", ok, err)
	}
	if err := db.SetTelegramChat(t.Context(), ana.ID, -100, now); err != nil {
		t.Fatal(err)
	}
	if ok, err := (notify.Channels{DB: db, Bot: bot}).Send(t.Context(), ana, notify.Note{Text: "x"}, now); !ok || err != nil {
		t.Fatalf("to a group that moved = %v, %v", ok, err)
	}
	if chat, _, _ := db.TelegramChat(t.Context(), ana.ID); chat != -1001234 {
		t.Errorf("chat = %d, want the supergroup's", chat)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(chats) != 2 || chats[1] != -1001234 {
		t.Errorf("sent to %v", chats)
	}
}

// Blocked is Telegram's 403, or its 400 of a chat not found, and nothing
// else.
func TestBlocked(t *testing.T) {
	t.Parallel()

	for _, err := range []error{fmt.Errorf("sending: %w", &telegram.Error{Code: 403}), &telegram.Error{Code: 400, Description: "Bad Request: chat not found"}} {
		if !notify.Blocked(err) {
			t.Errorf("%v is not blocked", err)
		}
	}
	for _, err := range []error{nil, &telegram.Error{Code: 400, Description: "Bad Request: message is too long"}, errors.New("403")} {
		if notify.Blocked(err) {
			t.Errorf("%v is blocked", err)
		}
	}
}
