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
	otherVariant := meal.NewIDAt(at)
	otherVariant[8] |= 0xc0
	for name, id := range map[string]uuid.UUID{"version 4": uuid.NewV4(), "nil": uuid.Nil(), "max": uuid.Max(), "version 7 of another variant": otherVariant} {
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

// Two items are the same when a client sent the same: the food and its
// quantity, an estimate's name and values, the note; not the id, nor what
// the server worked out from a food corrected since.
func TestItemSameAs(t *testing.T) {
	t.Parallel()

	egg := food.Food{
		ID: uuid.NewV7(), Name: "Egg", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 143, Protein: 12.6, Carbs: 0.7, Fat: 9.5}, Servings: []food.Serving{{Name: "piece", Amount: 60}},
	}
	two, err := meal.FromFood(egg, meal.Quantity{Servings: 2, Serving: "piece"}, "boiled")
	if err != nil {
		t.Fatal(err)
	}
	again := two
	again.ID = uuid.NewV7()
	egg.Nutrients.Kcal = 150
	corrected, err := meal.FromFood(egg, meal.Quantity{Servings: 2, Serving: "piece"}, "boiled")
	if err != nil {
		t.Fatal(err)
	}
	three, err := meal.FromFood(egg, meal.Quantity{Servings: 3, Serving: "piece"}, "boiled")
	if err != nil {
		t.Fatal(err)
	}
	soup, _ := meal.Estimate("Soup", food.Nutrients{Kcal: 180, Protein: 6, Carbs: 20, Fat: 8}, "")
	saltier, _ := meal.Estimate("Soup", food.Nutrients{Kcal: 180, Protein: 6, Carbs: 20, Fat: 8, Salt: new(2.0)}, "")
	for _, tc := range []struct {
		name string
		a, b meal.Item
		want bool
	}{
		{"another id", two, again, true},
		{"the food corrected since", two, corrected, true},
		{"another quantity", two, three, false},
		{"another note", two, func() meal.Item { i := two; i.Note = "fried"; return i }(), false},
		{"an estimate", soup, soup, true},
		{"an estimate's values", soup, saltier, false},
		{"a food and an estimate", two, soup, false},
	} {
		if got := tc.a.SameAs(tc.b); got != tc.want {
			t.Errorf("%s: SameAs = %v, want %v", tc.name, got, tc.want)
		}
	}
}
