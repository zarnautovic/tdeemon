package store_test

import (
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// An id is used by a meal there is, or by one that was deleted (or whose
// create was undone): a retry of either is no new meal. Ids are per user.
func TestMealIDUsed(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	kept := createMeal(t, db, ana, breakfast(t, bar, t0))
	gone := createMeal(t, db, ana, breakfast(t, bar, t0))
	if err := db.DeleteMeal(t.Context(), ana, testActor, gone.ID, t0); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		user uuid.UUID
		id   uuid.UUID
		want bool
	}{
		{"a meal", ana, kept.ID, true},
		{"a deleted meal", ana, gone.ID, true},
		{"no meal", ana, uuid.NewV7(), false},
		{"another user's meal", ivo, kept.ID, false},
	} {
		if got, err := db.MealIDUsed(t.Context(), tc.user, tc.id); err != nil || got != tc.want {
			t.Errorf("%s: MealIDUsed = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}

// FoodByKey finds the food of a name and brand as SaveFood does, in any
// case, archived too; another user's is none.
func TestFoodByKey(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	f := saveFood(t, db, ana, bar())
	if err := db.SetFoodArchived(t.Context(), ana, testActor, f.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	if got, err := db.FoodByKey(t.Context(), ana, "PROTEIN BAR caramel", "barebells"); err != nil || got.ID != f.ID {
		t.Errorf("FoodByKey = %+v, %v; want the bar", got, err)
	}
	for name, user := range map[string]uuid.UUID{"another user's": ivo, "no such": ana} {
		brand := "Barebells"
		if name == "no such" {
			brand = "Other"
		}
		if _, err := db.FoodByKey(t.Context(), user, "Protein bar Caramel", brand); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s food: %v, want ErrNotFound", name, err)
		}
	}
}

// UpdateMeal applies a change to the meal as it is in the write's own
// transaction, so changes made at once are all kept; a change that fails
// writes nothing.
func TestUpdateMealKeepsEveryChange(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	tea, err := meal.Estimate("Tea", food.Nutrients{Kcal: 2, Carbs: 0.3}, "")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, nil, func(m meal.Meal) (meal.Meal, error) {
				m.Items = append(m.Items, tea)
				return m, nil
			}, t0); err != nil {
				t.Errorf("UpdateMeal: %v", err)
			}
		})
	}
	wg.Wait()
	got, err := db.MealByID(t.Context(), ana, m.ID)
	if err != nil || len(got.Items) != 12 {
		t.Errorf("the meal has %d items, %v; want 12", len(got.Items), err)
	}
	refused := errors.New("refused")
	if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, nil, func(meal.Meal) (meal.Meal, error) { return meal.Meal{}, refused }, t0); !errors.Is(err, refused) {
		t.Errorf("a change that fails: %v", err)
	}
	if _, err := db.UpdateMeal(t.Context(), ana, testActor, uuid.NewV7(), nil, func(m meal.Meal) (meal.Meal, error) { return m, nil }, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("no such meal: %v, want ErrNotFound", err)
	}
}

// An item under the id of another meal's item is ErrItemTaken, which is
// a conflict of meals too.
func TestItemTaken(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar())
	first := createMeal(t, db, ana, breakfast(t, b, t0))
	second := breakfast(t, b, t0)
	second.Items[0].ID = first.Items[0].ID
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, second, t0); !errors.Is(err, store.ErrItemTaken) || !errors.Is(err, store.ErrMealConflict) {
		t.Errorf("CreateMeal = %v, want ErrItemTaken", err)
	}
}

// The new foods a meal brings are saved in the meal's own transaction:
// an item naming a food takes the id it is stored under; a meal refused
// saves none of them; a meal sent again saves them no more.
func TestCreateMealWithFoods(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar()) // its alias: caramel bar
	kefir := food.Food{ID: uuid.NewV7(), Name: "Kefir", Basis: food.Per100ml, Source: food.Label, Nutrients: food.Nutrients{Kcal: 50, Protein: 3.4, Carbs: 4.7, Fat: 1.5}}
	// The bar again, under another id: the same food, by its name.
	again := b
	again.ID = uuid.NewV7()
	glass, err := meal.FromFood(kefir, meal.Quantity{Amount: 250, Unit: "ml"}, "")
	if err != nil {
		t.Fatal(err)
	}
	one, err := meal.FromFood(again, meal.Quantity{Servings: 1, Serving: "bar"}, "")
	if err != nil {
		t.Fatal(err)
	}
	m := breakfast(t, b, t0)
	m.Items = []meal.Item{glass, one}
	got, created, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{kefir, again}, t0)
	if err != nil || !created {
		t.Fatalf("CreateMealWithFoods = %v, %v", created, err)
	}
	stored, err := db.FoodByKey(t.Context(), ana, "Kefir", "")
	if err != nil || got.Items[0].FoodID != stored.ID || got.Items[1].FoodID != b.ID {
		t.Errorf("the items name %v and %v, want the kefir %v and the bar %v (%v)", got.Items[0].FoodID, got.Items[1].FoodID, stored.ID, b.ID, err)
	}
	// Sent again, after the kefir was corrected: a retry, which saves no
	// food, nor writes another meal.
	stored.Nutrients.Kcal = 55
	if _, err := db.SaveFood(t.Context(), ana, testActor, stored, t0); err != nil {
		t.Fatal(err)
	}
	m.Items[0].FoodID = stored.ID
	m.Items[1].FoodID = b.ID
	if _, created, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{kefir, again}, t0); err != nil || created {
		t.Errorf("the meal again = %v, %v; want a retry", created, err)
	}
	if f, _ := db.FoodByKey(t.Context(), ana, "Kefir", ""); f.Nutrients.Kcal != 55 {
		t.Errorf("the kefir after the retry = %v kcal, want the correction kept", f.Nutrients.Kcal)
	}
}

// A meal refused saves none of the new foods it brings: for a food in the
// way of another's alias, or a food that breaks the rules.
func TestCreateMealWithFoodsRefused(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar()) // its alias: caramel bar
	milk := food.Food{ID: uuid.NewV7(), Name: "Milk", Basis: food.Per100ml, Source: food.Label, Nutrients: food.Nutrients{Kcal: 64, Protein: 3.3, Carbs: 4.8, Fat: 3.5}}
	taken := food.Food{ID: uuid.NewV7(), Name: "Caramel", Basis: food.Per100g, Source: food.Label, Nutrients: food.Nutrients{Kcal: 400, Carbs: 80, Fat: 8}, Aliases: []string{"caramel bar"}}
	cup, _ := meal.FromFood(milk, meal.Quantity{Amount: 200, Unit: "ml"}, "")
	spoon, _ := meal.FromFood(taken, meal.Quantity{Amount: 10, Unit: "g"}, "")
	refused := breakfast(t, b, t0)
	refused.Items = []meal.Item{cup, spoon}
	if _, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, refused, []food.Food{milk, taken}, t0); !errors.Is(err, store.ErrAliasTaken) {
		t.Fatalf("a meal of a taken alias = %v, want ErrAliasTaken", err)
	}
	if _, err := db.FoodByKey(t.Context(), ana, "Milk", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the milk of a meal refused: %v, want none", err)
	}
	if _, err := db.MealByID(t.Context(), ana, refused.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the meal refused: %v, want none", err)
	}
	// A food that breaks the rules, though the table would take it.
	invalid := refused
	invalid.Items = []meal.Item{cup}
	if _, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, invalid, []food.Food{tooRich(milk)}, t0); err == nil {
		t.Error("a meal of an invalid food was created")
	}
	if _, err := db.MealByID(t.Context(), ana, invalid.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the meal of an invalid food: %v, want none", err)
	}
}

// tooRich is f with more energy than 100 g of anything holds.
func tooRich(f food.Food) food.Food {
	f.Nutrients.Kcal = 2000
	return f
}

// A change UpdateMeal makes, though it changes the meal's items in place,
// is recorded with the meal as it was: undo puts it back.
func TestUpdateMealUndoes(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	tea, err := meal.Estimate("Tea", food.Nutrients{Kcal: 2, Carbs: 0.3}, "")
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(meal.Meal) (meal.Meal, error){
		"a replace": func(m meal.Meal) (meal.Meal, error) { tea.ID = m.Items[0].ID; m.Items[0] = tea; return m, nil },
		"a remove":  func(m meal.Meal) (meal.Meal, error) { m.Items = slices.Delete(m.Items, 0, 1); return m, nil },
	} {
		if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, nil, change, t0); err != nil {
			t.Fatalf("%s: UpdateMeal: %v", name, err)
		}
		if _, err := db.UndoLast(t.Context(), ana, testActor, t0); err != nil {
			t.Fatalf("%s: UndoLast: %v", name, err)
		}
		if got, err := db.MealByID(t.Context(), ana, m.ID); err != nil || !reflect.DeepEqual(got, m) {
			t.Errorf("%s undone: %+v, %v; want %+v", name, got, err, m)
		}
	}
	// The foods a change brings are saved with it; a change that fails
	// saves none.
	jam := food.Food{ID: uuid.NewV7(), Name: "Jam", Basis: food.Per100g, Source: food.Label, Nutrients: food.Nutrients{Kcal: 250, Protein: 0.5, Carbs: 60, Fat: 0.1}}
	refused := errors.New("refused")
	if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, []food.Food{jam}, func(meal.Meal) (meal.Meal, error) { return meal.Meal{}, refused }, t0); !errors.Is(err, refused) {
		t.Fatalf("a change that fails: %v", err)
	}
	if _, err := db.FoodByKey(t.Context(), ana, "Jam", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the jam of a change that failed: %v, want none", err)
	}
	if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, []food.Food{tooRich(jam)}, func(m meal.Meal) (meal.Meal, error) { return m, nil }, t0); err == nil {
		t.Error("a change bringing an invalid food was made")
	}
	if _, err := db.FoodByKey(t.Context(), ana, "Jam", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the invalid jam: %v, want none", err)
	}
}
