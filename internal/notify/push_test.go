package notify_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/notify"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
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
