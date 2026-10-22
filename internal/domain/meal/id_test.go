package meal_test

import (
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// A version 7 id carries the millisecond it was made; any other id
// carries none.
func TestIDTime(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 8, 0, 0, 123_000_000, time.UTC)
	id := meal.NewIDAt(at)
	if got, ok := meal.IDTime(id); !ok || !got.Equal(at) {
		t.Errorf("IDTime(NewIDAt(%v)) = %v, %v", at, got, ok)
	}
	if got, ok := meal.IDTime(uuid.NewV7()); !ok || time.Since(got) > time.Minute {
		t.Errorf("IDTime(NewV7()) = %v, %v; want about now", got, ok)
	}
	for name, id := range map[string]uuid.UUID{"version 4": uuid.NewV4(), "nil": uuid.Nil(), "max": uuid.Max()} {
		if _, ok := meal.IDTime(id); ok {
			t.Errorf("%s has a time", name)
		}
	}
	if a, b := meal.NewIDAt(at), meal.NewIDAt(at); a == b {
		t.Error("two ids of one time are the same")
	}
}

// A new meal's id is a version 7 id whose time is near the clock: from a
// day before to five minutes after.
func TestCheckNewID(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		id   uuid.UUID
		want string // in the error; "" for none
	}{
		{"now", meal.NewIDAt(now), ""},
		{"a day before", meal.NewIDAt(now.Add(-meal.MaxIDAge)), ""},
		{"five minutes ahead", meal.NewIDAt(now.Add(meal.MaxIDAhead)), ""},
		{"older than a day", meal.NewIDAt(now.Add(-meal.MaxIDAge - time.Second)), "more than a day before"},
		{"ahead of the clock", meal.NewIDAt(now.Add(meal.MaxIDAhead + time.Second)), "ahead of the clock"},
		{"version 4", uuid.NewV4(), "not a version 7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := meal.CheckNewID(tc.id, now)
			if (tc.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.want)) {
				t.Errorf("CheckNewID = %v, want %q", err, tc.want)
			}
		})
	}
}

// A meal is logged through one of the clients, the API with a token
// among them.
func TestMealClients(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	item, err := meal.Estimate("Soup", food.Nutrients{Kcal: 180, Protein: 6, Carbs: 20, Fat: 8}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []meal.Client{meal.ClientMCP, meal.ClientChat, meal.ClientWeb, meal.ClientTelegram, meal.ClientImport, meal.ClientAPI} {
		m := meal.Meal{ID: meal.NewIDAt(at), EatenAt: at, Day: day.DateOf(at), Client: c, Items: []meal.Item{item}}
		if err := m.Validate(); err != nil {
			t.Errorf("client %s: %v", c, err)
		}
	}
	m := meal.Meal{ID: meal.NewIDAt(at), EatenAt: at, Day: day.DateOf(at), Client: "fax", Items: []meal.Item{item}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "or api") {
		t.Errorf("client fax: %v", err)
	}
}
