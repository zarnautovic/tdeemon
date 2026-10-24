package store_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// RetryMeal returns the meal of a create sent again, by its fingerprint,
// as it is now; another meal under the id is a conflict, a deleted one is
// deleted, and an id no meal of the user's had is none. Ids are per user.
func TestRetryMeal(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	sent := breakfast(t, bar, t0)
	kept := createMeal(t, db, ana, sent)
	gone := createMeal(t, db, ana, breakfast(t, bar, t0))
	if err := db.DeleteMeal(t.Context(), ana, testActor, gone.ID, t0); err != nil {
		t.Fatal(err)
	}
	if got, err := db.RetryMeal(t.Context(), ana, kept.ID, sent.Fingerprint()); err != nil || !reflect.DeepEqual(got, kept) {
		t.Errorf("RetryMeal = %+v, %v; want the meal", got, err)
	}
	other := sent
	other.Note = "another"
	for _, tc := range []struct {
		name string
		user uuid.UUID
		id   uuid.UUID
		fp   string
		want error
	}{
		{"another meal", ana, kept.ID, other.Fingerprint(), store.ErrMealConflict},
		{"a deleted meal", ana, gone.ID, sent.Fingerprint(), store.ErrMealDeleted},
		{"no meal", ana, uuid.NewV7(), sent.Fingerprint(), store.ErrNotFound},
		{"another user's meal", ivo, kept.ID, sent.Fingerprint(), store.ErrNotFound},
	} {
		if _, err := db.RetryMeal(t.Context(), tc.user, tc.id, tc.fp); !errors.Is(err, tc.want) {
			t.Errorf("%s: RetryMeal = %v, want %v", tc.name, err, tc.want)
		}
	}
}

// An item of a new food keeps the food as sent; a meal of new foods sent
// twice at once, each time with the foods under ids of its own, is one
// meal, a retry, though its foods were stored under the first's ids.
func TestMealOfSentFoods(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar())
	kefir := func() (food.Food, meal.Item) {
		f := food.Food{ID: uuid.NewV7(), Name: "Kefir", Basis: food.Per100ml, Source: food.Label, Nutrients: food.Nutrients{Kcal: 50, Protein: 3.4, Carbs: 4.7, Fat: 1.5}}
		it, err := meal.FromFood(f, meal.Quantity{Amount: 250, Unit: "ml"}, "")
		if err != nil {
			t.Fatal(err)
		}
		it.SentFood = meal.SentFood(f)
		return f, it
	}
	f1, it1 := kefir()
	m := breakfast(t, b, t0)
	m.Items = []meal.Item{it1}
	got, created, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{f1}, t0)
	if err != nil || !created || got.Items[0].SentFood != it1.SentFood {
		t.Fatalf("CreateMealWithFoods = %+v, %v, %v; want the food as sent kept", got.Items, created, err)
	}
	f2, it2 := kefir()
	m.Items = []meal.Item{it2}
	if _, created, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{f2}, t0); err != nil || created {
		t.Errorf("the meal again, its food under another id = %v, %v; want a retry", created, err)
	}
	if _, err := db.FoodByID(t.Context(), ana, f2.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the second kefir: %v, want none", err)
	}
	// A change that names no food of those it brings saves none of them.
	jam := food.Food{ID: uuid.NewV7(), Name: "Jam", Basis: food.Per100g, Source: food.Label, Nutrients: food.Nutrients{Kcal: 250, Protein: 0.5, Carbs: 60, Fat: 0.1}}
	if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, []food.Food{jam}, func(m meal.Meal) (meal.Meal, error) { return m, nil }, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := foodNamed(t, db, ana, "Jam"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a food no item names: %v, want none", err)
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
	stored, err := foodNamed(t, db, ana, "Kefir")
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
	if f, _ := foodNamed(t, db, ana, "Kefir"); f.Nutrients.Kcal != 55 {
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
	if _, err := foodNamed(t, db, ana, "Milk"); !errors.Is(err, store.ErrNotFound) {
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
	if _, err := foodNamed(t, db, ana, "Jam"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the jam of a change that failed: %v, want none", err)
	}
	if _, err := db.UpdateMeal(t.Context(), ana, testActor, m.ID, []food.Food{tooRich(jam)}, func(m meal.Meal) (meal.Meal, error) { return m, nil }, t0); err == nil {
		t.Error("a change bringing an invalid food was made")
	}
	if _, err := foodNamed(t, db, ana, "Jam"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the invalid jam: %v, want none", err)
	}
}

// foodNamed is user's food of name and no brand, found by search, or
// store.ErrNotFound.
func foodNamed(t *testing.T, db *store.DB, user uuid.UUID, name string) (food.Food, error) {
	t.Helper()
	found, err := db.FindFoods(t.Context(), user, name, 50)
	if err != nil {
		t.Fatal(err)
	}
	for i := range found {
		if found[i].Name == name && found[i].Brand == "" {
			return found[i], nil
		}
	}
	return food.Food{}, store.ErrNotFound
}

// A new food a meal brings of the name and brand of a food the user has
// is that food as the user has it: it is not changed, and the item comes
// from its values. A quantity that food does not take is refused.
func TestMealOfAFoodTheUserHas(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar()) // a label's, with its servings and alias
	guess := food.Food{
		ID: uuid.NewV7(), Name: b.Name, Brand: b.Brand, Basis: food.Per100g, Source: food.Estimate,
		Nutrients: food.Nutrients{Kcal: 500, Protein: 10, Carbs: 50, Fat: 25},
	}
	it, err := meal.FromFood(guess, meal.Quantity{Amount: 50, Unit: "g"}, "")
	if err != nil {
		t.Fatal(err)
	}
	m := breakfast(t, b, t0)
	m.Items = []meal.Item{it}
	got, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{guess}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if after, err := db.FoodByID(t.Context(), ana, b.ID); err != nil || !reflect.DeepEqual(after, b) {
		t.Errorf("the user's food after a meal of a guess of it = %+v, %v; want it as it was", after, err)
	}
	want, err := meal.FromFood(b, meal.Quantity{Amount: 50, Unit: "g"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if g := got.Items[0]; g.FoodID != b.ID || !reflect.DeepEqual(g.Nutrients, want.Nutrients) || g.Name != want.Name {
		t.Errorf("the item = %+v, want one of the user's food, %+v", g, want)
	}
	// In ml, which the bar by weight does not take.
	guess.ID, guess.Basis = uuid.NewV7(), food.Per100ml
	it, err = meal.FromFood(guess, meal.Quantity{Amount: 50, Unit: "ml"}, "")
	if err != nil {
		t.Fatal(err)
	}
	other := breakfast(t, b, t0)
	other.Items = []meal.Item{it}
	var fie *store.FoodItemError
	if _, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, other, []food.Food{guess}, t0); !errors.As(err, &fie) || fie.Food.ID != b.ID {
		t.Errorf("a meal of 50 ml of a food by weight = %v, want a FoodItemError of the user's food", err)
	}
	// 50 without its unit, of the new food: in what unit is not the
	// user's food's to say.
	it.Quantity = meal.Quantity{Amount: 50}
	guess.ID = uuid.NewV7()
	it.FoodID = guess.ID
	other.Items = []meal.Item{it}
	if _, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, other, []food.Food{guess}, t0); !errors.As(err, &fie) || !strings.Contains(err.Error(), "names its unit") {
		t.Errorf("a meal of 50 of a new food, no unit = %v, want a FoodItemError asking for the unit", err)
	}
}

// A new food's servings without their name, of a food the user has by
// its name, are refused: of which serving, the user's food's one is not
// to say.
func TestMealOfAFoodTheUserHasServingsUnnamed(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := bar()
	b.Servings = []food.Serving{{Name: "bar", Amount: 50}}
	if _, err := db.SaveFood(t.Context(), ana, testActor, b, t0); err != nil {
		t.Fatal(err)
	}
	guess := b
	guess.ID, guess.Servings = uuid.NewV7(), []food.Serving{{Name: "box", Amount: 500}}
	it, err := meal.FromFood(guess, meal.Quantity{Servings: 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	m := breakfast(t, b, t0)
	m.Items = []meal.Item{it}
	var fie *store.FoodItemError
	if _, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{guess}, t0); !errors.As(err, &fie) || !strings.Contains(err.Error(), "name the serving") {
		t.Errorf("a meal of 2 servings of a new food, unnamed = %v, want a FoodItemError asking for the serving", err)
	}
}

// An archived food a new food of its name is comes back: the user eats it
// again, and a search finds it next time. Bringing it back is an event.
func TestMealOfAnArchivedFood(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar())
	if err := db.SetFoodArchived(t.Context(), ana, testActor, b.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	guess := food.Food{
		ID: uuid.NewV7(), Name: b.Name, Brand: b.Brand, Basis: food.Per100g, Source: food.Estimate,
		Nutrients: food.Nutrients{Kcal: 400, Protein: 20, Carbs: 40, Fat: 15},
	}
	it, err := meal.FromFood(guess, meal.Quantity{Amount: 50, Unit: "g"}, "")
	if err != nil {
		t.Fatal(err)
	}
	m := breakfast(t, b, t0)
	m.Items = []meal.Item{it}
	if _, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{guess}, t0); err != nil {
		t.Fatal(err)
	}
	after, err := db.FoodByID(t.Context(), ana, b.ID)
	if err != nil || after.Archived {
		t.Errorf("the bar after a meal of it = archived %v, %v; want it back", after.Archived, err)
	}
	es, err := db.Events(t.Context(), ana, 2)
	if err != nil || len(es) != 2 || es[1].Action != event.FoodArchive || es[1].Entity != b.ID.String() {
		t.Errorf("the events = %+v, %v; want the bar brought back before the meal", es, err)
	}
}
