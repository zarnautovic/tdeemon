package notify_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/notify"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// answerByPath answers a push by the endpoint's last segment: its status.
type answerByPath map[string]int

func (a answerByPath) RoundTrip(r *http.Request) (*http.Response, error) {
	status := a[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]]
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
}

// subscribe subscribes a device of u's at a push service's endpoint
// ending in name.
func subscribe(t *testing.T, db *store.DB, u user.User, name string, now time.Time) uuid.UUID {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := store.PushSubscription{
		ID: uuid.NewV7(), Endpoint: "https://fcm.googleapis.com/fcm/send/" + name, Device: name,
		P256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), Auth: "AAAAAAAAAAAAAAAAAAAAAA",
	}
	if _, err := db.SavePushSubscription(t.Context(), u.ID, s, now); err != nil {
		t.Fatal(err)
	}
	return s.ID
}

// Push sends to each of the user's subscriptions: one that took it is
// marked sent, one that is gone is deleted, one that failed stays and
// says why.
func TestPush(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	u := user.User{ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser, Locale: "hr", TimeZone: "UTC", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatal(err)
	}
	ids := map[string]uuid.UUID{}
	for _, name := range []string{"ok", "gone", "down"} {
		ids[name] = subscribe(t, db, u, name, now)
	}
	sender, err := push.NewSender([]byte("a secret of at least thirty-two bytes"), "https://tdeemon.example.com")
	if err != nil {
		t.Fatal(err)
	}
	sender.Client = &http.Client{Transport: answerByPath{"ok": http.StatusCreated, "gone": http.StatusGone, "down": http.StatusInternalServerError}}

	later := now.Add(time.Hour)
	got := notify.Push(t.Context(), db, sender, u, push.Notification{Title: "t", Body: "b", URL: "/"}, push.Message{TTL: time.Hour}, later)
	if got.Sent != 1 || got.Gone != 1 || got.Failed != 1 || got.Err == nil || !strings.Contains(got.Err.Error(), "down") {
		t.Errorf("Push = %+v", got)
	}
	subs, err := db.PushSubscriptions(t.Context(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	left := map[uuid.UUID]store.PushSubscription{}
	for _, s := range subs {
		left[s.ID] = s
	}
	if _, ok := left[ids["gone"]]; ok || len(left) != 2 {
		t.Errorf("left %d subscriptions, the gone one among them: %v", len(left), ok)
	}
	if !left[ids["ok"]].LastSentAt.Equal(later) || !left[ids["down"]].LastSentAt.IsZero() {
		t.Errorf("last sent: ok %v, down %v", left[ids["ok"]].LastSentAt, left[ids["down"]].LastSentAt)
	}
}

// channelUsers are users with each mix of channels: a Telegram chat that
// blocked the bot beside a device that takes it, the chat alone, a device
// that is down alone, none, and the blocked chat beside a device that is
// down; and the channels, both on.
type channelUsers struct {
	both, chatOnly, downOnly, none, chatAndDown user.User
	c                                           notify.Channels
}

func newChannelUsers(t *testing.T) channelUsers {
	t.Helper()
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`)
	}))
	t.Cleanup(blocked.Close)
	db := storetest.Open(t)
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	newU := func(email string, chat bool, device string) user.User {
		u := user.User{ID: uuid.NewV7(), Email: email, Name: "Ana", Role: user.RoleUser, Locale: "en", TimeZone: "UTC", CreatedAt: now, UpdatedAt: now}
		if err := db.CreateUser(t.Context(), u, ""); err != nil {
			t.Fatal(err)
		}
		if chat {
			if err := db.SetTelegramChat(t.Context(), u.ID, 853, now); err != nil {
				t.Fatal(err)
			}
		}
		if device != "" {
			subscribe(t, db, u, device, now)
		}
		return u
	}
	sender, err := push.NewSender([]byte("a secret of at least thirty-two bytes"), "https://tdeemon.example.com")
	if err != nil {
		t.Fatal(err)
	}
	sender.Client = &http.Client{Transport: answerByPath{"ok": http.StatusCreated, "down": http.StatusServiceUnavailable, "down2": http.StatusServiceUnavailable}}
	return channelUsers{
		both: newU("both@example.com", true, "ok"), chatOnly: newU("chat@example.com", true, ""),
		downOnly: newU("down@example.com", false, "down"), none: newU("none@example.com", false, ""),
		chatAndDown: newU("chat-down@example.com", true, "down2"),
		c: notify.Channels{
			DB: db, Bot: &telegram.Bot{Token: "123456:made-up-token-for-tests-only-0123", BaseURL: blocked.URL},
			Push: sender, Logger: slog.New(slog.DiscardHandler),
		},
	}
}

// Send goes to each channel, and fails only when none delivered: a
// Telegram chat that blocked the bot beside a device that took it is no
// error, alone it is Blocked, and a device that is down alone is an error
// to retry.
func TestSendToEveryChannel(t *testing.T) {
	t.Parallel()

	u := newChannelUsers(t)
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	n := notify.Note{Text: "x", Push: push.Notification{Title: "t", Body: "b", URL: "/"}}
	if has, err := u.c.Send(t.Context(), u.both, n, now); !has || err != nil {
		t.Errorf("blocked chat and a device = %v, %v; want delivered", has, err)
	}
	if has, err := u.c.Send(t.Context(), u.chatOnly, n, now); !has || !notify.Blocked(err) {
		t.Errorf("blocked chat alone = %v, %v; want Blocked", has, err)
	}
	if has, err := u.c.Send(t.Context(), u.downOnly, n, now); !has || err == nil || notify.Blocked(err) {
		t.Errorf("a device down alone = %v, %v; want an error to retry", has, err)
	}
	// The device may take it later: an error to retry, not Blocked.
	if has, err := u.c.Send(t.Context(), u.chatAndDown, n, now); !has || err == nil || notify.Blocked(err) {
		t.Errorf("blocked chat and a device down = %v, %v; want an error to retry", has, err)
	}
	if has, err := u.c.Send(t.Context(), u.none, n, now); has || err != nil {
		t.Errorf("no channel = %v, %v", has, err)
	}
}

// Has is a chat or a device, of a channel that is on.
func TestHas(t *testing.T) {
	t.Parallel()

	u := newChannelUsers(t)
	for _, tc := range []struct {
		u    user.User
		want bool
	}{{u.both, true}, {u.chatOnly, true}, {u.downOnly, true}, {u.none, false}} {
		if has, err := u.c.Has(t.Context(), tc.u); has != tc.want || err != nil {
			t.Errorf("Has(%s) = %v, %v", tc.u.Email, has, err)
		}
	}
	if has, _ := (notify.Channels{DB: u.c.DB}).Has(t.Context(), u.both); has {
		t.Error("Has with every channel off")
	}
}
