package store_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

var pushNow = time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)

// pushUsers returns a database with Ana and Ivo, and Ana's phone and
// laptop subscribed, the laptop later.
func pushUsers(t *testing.T) (db *store.DB, ana, ivo user.User, phone, laptop store.PushSubscription) {
	t.Helper()
	db = storetest.Open(t)
	ana, ivo = newUser("ana@example.com"), newUser("ivo@example.com")
	for _, err := range []error{db.CreateUser(t.Context(), ana, ""), db.CreateUser(t.Context(), ivo, "")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	phone = store.PushSubscription{ID: uuid.NewV7(), Endpoint: "https://web.push.apple.com/phone", P256dh: "k1", Auth: "a1", Device: "iPhone · Safari"}
	laptop = store.PushSubscription{ID: uuid.NewV7(), Endpoint: "https://fcm.googleapis.com/fcm/send/laptop", P256dh: "k3", Auth: "a3", Device: "Mac · Chrome"}
	if _, err := db.SavePushSubscription(t.Context(), ana.ID, phone, pushNow); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SavePushSubscription(t.Context(), ana.ID, laptop, pushNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return db, ana, ivo, phone, laptop
}

func pushIDs(t *testing.T, db *store.DB, userID uuid.UUID) []uuid.UUID {
	t.Helper()
	subs, err := db.PushSubscriptions(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]uuid.UUID, 0, len(subs))
	for i := range subs {
		ids = append(ids, subs[i].ID)
	}
	return ids
}

// The same endpoint again is the same subscription, its keys renewed and
// what was sent to it kept.
func TestSavePushSubscriptionByEndpoint(t *testing.T) {
	t.Parallel()

	db, ana, _, phone, laptop := pushUsers(t)
	if err := db.MarkPushSent(t.Context(), ana.ID, phone.ID, pushNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	later := pushNow.Add(2 * time.Hour)
	again := phone
	again.ID, again.P256dh, again.Auth = uuid.NewV7(), "k2", "a2"
	saved, err := db.SavePushSubscription(t.Context(), ana.ID, again, later)
	if err != nil {
		t.Fatal(err)
	}
	want := store.PushSubscription{
		ID: phone.ID, UserID: ana.ID, Endpoint: phone.Endpoint, P256dh: "k2", Auth: "a2", Device: phone.Device,
		CreatedAt: pushNow, UpdatedAt: later, LastSentAt: pushNow.Add(time.Minute),
	}
	if saved != want {
		t.Errorf("saved again\n got %+v\nwant %+v", saved, want)
	}
	if ids := pushIDs(t, db, ana.ID); len(ids) != 2 || ids[0] != laptop.ID || ids[1] != phone.ID {
		t.Errorf("Ana's = %v, want the laptop, then the phone", ids)
	}
}

// Another user can neither see, delete nor mark a user's subscriptions.
func TestPushSubscriptionsAreTheirs(t *testing.T) {
	t.Parallel()

	db, ana, ivo, phone, laptop := pushUsers(t)
	if ids := pushIDs(t, db, ivo.ID); len(ids) != 0 {
		t.Errorf("Ivo has %v", ids)
	}
	if ok, err := db.DeletePushSubscription(t.Context(), ivo.ID, phone.ID); ok || err != nil {
		t.Errorf("Ivo deletes Ana's: %v, %v", ok, err)
	}
	if err := db.MarkPushSent(t.Context(), ivo.ID, laptop.ID, pushNow); err != nil {
		t.Fatal(err)
	}
	subs, _ := db.PushSubscriptions(t.Context(), ana.ID)
	if len(subs) != 2 || !subs[0].LastSentAt.IsZero() {
		t.Errorf("Ana's after Ivo's tries: %+v", subs)
	}
	if ok, err := db.DeletePushSubscription(t.Context(), ana.ID, laptop.ID); !ok || err != nil {
		t.Errorf("delete = %v, %v", ok, err)
	}
	if ok, _ := db.DeletePushSubscription(t.Context(), ana.ID, laptop.ID); ok {
		t.Error("deleted twice")
	}
}

// A device another user subscribes on becomes theirs, a new subscription.
func TestPushSubscriptionTakenByAnotherUser(t *testing.T) {
	t.Parallel()

	db, ana, ivo, phone, laptop := pushUsers(t)
	if err := db.MarkPushSent(t.Context(), ana.ID, phone.ID, pushNow); err != nil {
		t.Fatal(err)
	}
	later := pushNow.Add(3 * time.Hour)
	taken := phone
	taken.ID = uuid.NewV7()
	saved, err := db.SavePushSubscription(t.Context(), ivo.ID, taken, later)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != taken.ID || saved.UserID != ivo.ID || !saved.CreatedAt.Equal(later) || !saved.LastSentAt.IsZero() {
		t.Errorf("Ivo's from Ana's phone: %+v", saved)
	}
	if ids := pushIDs(t, db, ana.ID); len(ids) != 1 || ids[0] != laptop.ID {
		t.Errorf("Ana's after Ivo took the phone: %v", ids)
	}
}
