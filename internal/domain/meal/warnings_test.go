package meal

import (
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

func codes(ws []Warning) string {
	var cs []string
	for _, w := range ws {
		cs = append(cs, w.Code)
	}
	return strings.Join(cs, ",")
}

func TestCheckNutrients(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		n    food.Nutrients
		want string
	}{
		{"a label that adds up", food.Nutrients{Kcal: 363, Protein: 30, Carbs: 52, Fat: 10}, ""},
		{"within 15 %", food.Nutrients{Kcal: 200, Protein: 10, Carbs: 20, Fat: 10}, ""},             // 210 expected
		{"just within 15 %", food.Nutrients{Kcal: 240, Protein: 10, Carbs: 20, Fat: 10}, ""},        // 30 of 240: 12.5 %
		{"just past 15 %", food.Nutrients{Kcal: 250, Protein: 10, Carbs: 20, Fat: 10}, WarnAtwater}, // 40 of 250: 16 %
		{"kcal far off", food.Nutrients{Kcal: 500, Protein: 10, Carbs: 20, Fat: 10}, WarnAtwater},
		{"kcal too low", food.Nutrients{Kcal: 100, Protein: 10, Carbs: 20, Fat: 10}, WarnAtwater},
		{"a cucumber, a few kcal off", food.Nutrients{Kcal: 8, Protein: 0.3, Carbs: 1.8, Fat: 0.1}, ""},
		{"wine with its alcohol", food.Nutrients{Kcal: 83, Carbs: 2.6, Alcohol: new(10.5)}, ""},
		{"wine without it", food.Nutrients{Kcal: 83, Carbs: 2.6}, WarnAtwater},
		{"a bar with polyols", food.Nutrients{Kcal: 363, Protein: 30, Carbs: 52, Fat: 10, Polyols: new(27.0), Fiber: new(8.3)}, ""},
		{"saturated fat above fat", food.Nutrients{Kcal: 90, Fat: 10, SaturatedFat: new(12.0)}, WarnSaturatedFat},
		{"saturated fat equal to fat (butter, rounded)", food.Nutrients{Kcal: 90, Fat: 10, SaturatedFat: new(10.0)}, ""},
		{"sugars above carbs", food.Nutrients{Kcal: 40, Carbs: 10, Sugars: new(11.0)}, WarnSugars},
		{"polyols above carbs", food.Nutrients{Kcal: 21, Carbs: 10, Polyols: new(12.0)}, WarnPolyols},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ws := CheckNutrients("the bar", tc.n)
			if got := codes(ws); got != tc.want {
				t.Errorf("CheckNutrients = %q (%v), want %q", got, ws, tc.want)
			}
			for _, w := range ws {
				if !strings.HasPrefix(w.Message, "the bar: ") {
					t.Errorf("message %q does not name what it is about", w.Message)
				}
			}
		})
	}
}

func TestDuplicateOf(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 8, 30, 0, 0, time.UTC)
	e, m := egg(), milk()
	first := meal(t, at, item(t, e, Quantity{Amount: 120}), item(t, m, Quantity{Amount: 200}))
	for _, tc := range []struct {
		name string
		next Meal
		want bool
	}{
		{"the same items in another order, 3 minutes later", meal(t, at.Add(3*time.Minute),
			item(t, m, Quantity{Amount: 200}), item(t, e, Quantity{Servings: 2, Serving: "piece"})), true},
		{"the same, logged as earlier", meal(t, at.Add(-9*time.Minute), item(t, e, Quantity{Amount: 120}), item(t, m, Quantity{Amount: 200})), true},
		{"half a percent more milk", meal(t, at.Add(time.Minute), item(t, e, Quantity{Amount: 120}), item(t, m, Quantity{Amount: 201})), true},
		{"an hour later", meal(t, at.Add(time.Hour), item(t, e, Quantity{Amount: 120}), item(t, m, Quantity{Amount: 200})), false},
		{"more egg", meal(t, at.Add(time.Minute), item(t, e, Quantity{Amount: 180}), item(t, m, Quantity{Amount: 200})), false},
		{"an item less", meal(t, at.Add(time.Minute), item(t, e, Quantity{Amount: 120})), false},
		{"another food", meal(t, at.Add(time.Minute), item(t, egg(), Quantity{Amount: 120}), item(t, m, Quantity{Amount: 200})), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, dup := DuplicateOf(tc.next, []Meal{first})
			if dup != tc.want || (dup && (w.Code != WarnDuplicateMeal || !strings.Contains(w.Message, "08:30"))) {
				t.Errorf("DuplicateOf = %v, %v; want %v", w, dup, tc.want)
			}
		})
	}
	// The meal itself (an update) is no duplicate of itself.
	if _, dup := DuplicateOf(first, []Meal{first}); dup {
		t.Error("a meal is a duplicate of itself")
	}
	// Estimates compare by name.
	a, _ := Estimate("Burger", food.Nutrients{Kcal: 900}, "")
	b, _ := Estimate("Burger", food.Nutrients{Kcal: 900}, "")
	if _, dup := DuplicateOf(meal(t, at, b), []Meal{meal(t, at, a)}); !dup {
		t.Error("the same estimate twice is not a duplicate")
	}
}
