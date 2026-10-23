package store_test

import (
	"testing"
	"time"

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
