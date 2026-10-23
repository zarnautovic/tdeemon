package store_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"

	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// A user's Telegram chat is set, changed and removed, and is theirs.
func TestTelegramChat(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	ana, ivo := newUser("ana@example.com"), newUser("ivo@example.com")
	for _, err := range []error{db.CreateUser(t.Context(), ana, ""), db.CreateUser(t.Context(), ivo, "")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	if _, ok, err := db.TelegramChat(t.Context(), ana.ID); ok || err != nil {
		t.Fatalf("before = %v, %v", ok, err)
	}
	for _, chat := range []int64{853, -100123} {
		if err := db.SetTelegramChat(t.Context(), ana.ID, chat, now); err != nil {
			t.Fatal(err)
		}
		if got, ok, err := db.TelegramChat(t.Context(), ana.ID); got != chat || !ok || err != nil {
			t.Errorf("after %d = %d, %v, %v", chat, got, ok, err)
		}
	}
	if _, ok, _ := db.TelegramChat(t.Context(), ivo.ID); ok {
		t.Error("Ivo has Ana's chat")
	}
	if err := db.SetTelegramChat(t.Context(), ana.ID, 0, now); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.TelegramChat(t.Context(), ana.ID); ok || err != nil {
		t.Errorf("after 0 = %v, %v", ok, err)
	}
}

// The users of a chat are those whose notifications go there, in the
// order they were created; a chat that is no one's has none.
func TestTelegramChatUsers(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	now := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	ids := func(chat int64) []string {
		t.Helper()
		us, err := db.TelegramChatUsers(t.Context(), chat)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, u := range us {
			out = append(out, u.Email)
		}
		return out
	}
	if got := ids(853); got != nil {
		t.Errorf("a chat that is no one's = %v", got)
	}
	if err := db.SetTelegramChat(t.Context(), ana, 853, now); err != nil {
		t.Fatal(err)
	}
	if got := ids(853); len(got) != 1 || got[0] != "ana@example.com" {
		t.Errorf("Ana's chat = %v", got)
	}
	if got := ids(-100123); got != nil {
		t.Errorf("another chat = %v", got)
	}
	// A group both are in.
	for _, id := range []uuid.UUID{ivo, ana} {
		if err := db.SetTelegramChat(t.Context(), id, -100123, now); err != nil {
			t.Fatal(err)
		}
	}
	if got := ids(-100123); len(got) != 2 || got[0] != "ana@example.com" || got[1] != "ivo@example.com" {
		t.Errorf("the group = %v", got)
	}
	if got := ids(853); got != nil {
		t.Errorf("Ana's old chat = %v", got)
	}
}

// LastMetricUpdate is when a metric of the user last changed.
func TestLastMetricUpdate(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	ana := newUser("ana@example.com")
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.LastMetricUpdate(t.Context(), ana.ID); ok || err != nil {
		t.Fatalf("before = %v, %v", ok, err)
	}
	at := time.Date(2026, 10, 23, 7, 54, 15, 0, time.UTC)
	actor := event.Actor{Kind: event.User, Client: "web"}
	for i, when := range []time.Time{at.Add(-time.Hour), at} {
		v := []metric.Value{{Kind: metric.Steps, Value: float64(1000 * (i + 1)), Source: metric.Import}}
		if _, err := db.MergeMetrics(t.Context(), ana.ID, actor, day.DateOf(when), v, when); err != nil {
			t.Fatal(err)
		}
	}
	if got, ok, err := db.LastMetricUpdate(t.Context(), ana.ID); !ok || err != nil || !got.Equal(at) {
		t.Errorf("= %v, %v, %v, want %v", got, ok, err, at)
	}
}
