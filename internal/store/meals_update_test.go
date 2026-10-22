package store_test

import (
	"errors"
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
			if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, func(m meal.Meal) (meal.Meal, error) {
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
	if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, func(meal.Meal) (meal.Meal, error) { return meal.Meal{}, refused }, t0); !errors.Is(err, refused) {
		t.Errorf("a change that fails: %v", err)
	}
	if _, err := db.UpdateMeal(t.Context(), ana, testActor, uuid.NewV7(), func(m meal.Meal) (meal.Meal, error) { return m, nil }, t0); !errors.Is(err, store.ErrNotFound) {
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
