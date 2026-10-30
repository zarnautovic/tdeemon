package store_test

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/recipe"
	"github.com/zarnautovic/tdeemon/internal/store"
)

func chickenBreast() food.Food {
	return food.Food{
		ID: uuid.NewV7(), Name: "Chicken breast", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 110, Protein: 23, Fat: 2},
	}
}

// curryOf is a recipe of chicken and kefir, 1 kg cooked in 4 portions.
func curryOf(t *testing.T, chicken, kefir food.Food) recipe.Recipe {
	t.Helper()
	return recipe.Recipe{
		Food: food.Food{ID: uuid.NewV7(), Name: "Chicken curry", Aliases: []string{"the curry"}},
		Items: []meal.Item{
			itemOf(t, chicken, meal.Quantity{Amount: 600, Unit: "g"}),
			itemOf(t, kefir, meal.Quantity{Amount: 300, Unit: "ml"}),
		},
		Yield: 1000, Portions: 4, Pot: 1500,
	}
}

func saveRecipe(t *testing.T, db *store.DB, userID uuid.UUID, r recipe.Recipe, foods ...food.Food) recipe.Recipe {
	t.Helper()
	got, err := db.SaveRecipe(t.Context(), userID, testActor, r, foods, t0)
	if err != nil {
		t.Fatalf("SaveRecipe(%s): %v", r.Food.Name, err)
	}
	return got
}

func TestSaveRecipeAndReadItBack(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := kefir()
	r := curryOf(t, chicken, k)
	got := saveRecipe(t, db, ana, r, k)

	if got.Food.ID != r.Food.ID || got.Food.Source != food.Recipe || got.Food.Basis != food.Per100g {
		t.Errorf("food = %+v, want the recipe's id, by weight, of source recipe", got.Food)
	}
	// 660 + 150 kcal in 1000 g.
	if math.Abs(got.Food.Nutrients.Kcal-81) > 1e-9 {
		t.Errorf("kcal per 100 g = %v, want 81", got.Food.Nutrients.Kcal)
	}
	if got.Yield != 1000 || got.Portions != 4 || got.Pot != 1500 || len(got.Items) != 2 {
		t.Errorf("recipe = %+v", got)
	}
	// The new food is saved, and the ingredient is of it.
	stored, err := foodNamed(t, db, ana, "Kefir")
	if err != nil || got.Items[1].FoodID != stored.ID {
		t.Errorf("kefir %+v, %v; the ingredient's food %v", stored, err, got.Items[1].FoodID)
	}
	recipeReads(t, db, ana, ivo, got, chicken)
}

// recipeReads checks that ana's recipe r reads back, is found and logged
// as a food, and is no other user's, nor of another food.
func recipeReads(t *testing.T, db *store.DB, ana, ivo uuid.UUID, got recipe.Recipe, chicken food.Food) {
	t.Helper()
	r := got
	again, err := db.RecipeByID(t.Context(), ana, r.Food.ID)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Errorf("RecipeByID = %+v, %v; want %+v", again, err, got)
	}
	// The recipe's food is found as any food.
	if fs, err := db.FindFoods(t.Context(), ana, "curry", 10); err != nil || len(fs) != 1 || fs[0].ID != r.Food.ID {
		t.Errorf("FindFoods(curry) = %v, %v", fs, err)
	}
	// And logged as any: a portion is a quarter of the dish.
	m := breakfast(t, saveFood(t, db, ana, bar()), t0)
	m.Items = []meal.Item{itemOf(t, got.Food, meal.Quantity{Servings: 1, Serving: recipe.Portion})}
	logged := createMeal(t, db, ana, m)
	if kcal := logged.Items[0].Nutrients.Kcal; math.Abs(kcal-810.0/4) > 1e-9 {
		t.Errorf("a portion logged = %v kcal, want %v", kcal, 810.0/4)
	}
	if _, err := db.RecipeByID(t.Context(), ivo, r.Food.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another user's RecipeByID = %v, want ErrNotFound", err)
	}
	if _, err := db.RecipeByID(t.Context(), ana, chicken.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("RecipeByID of a food = %v, want ErrNotFound", err)
	}
	if rs, err := db.RecipesOf(t.Context(), ana, []uuid.UUID{chicken.ID, r.Food.ID}); err != nil || len(rs) != 1 || rs[r.Food.ID].Yield != 1000 {
		t.Errorf("RecipesOf = %v, %v", rs, err)
	}
}

// A recipe saved again by its name is corrected, its ingredients taken
// as their foods are now; a meal logged before keeps what it was.
func TestSaveRecipeAgain(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	first := saveRecipe(t, db, ana, curryOf(t, chicken, k))

	chicken.Nutrients.Kcal = 120
	chicken = saveFood(t, db, ana, chicken)
	if r, _ := db.RecipeByID(t.Context(), ana, first.Food.ID); r.Items[0].Nutrients.Kcal != 660 {
		t.Errorf("after the chicken's correction the ingredient has %v kcal, want 660 until the recipe is saved", r.Items[0].Nutrients.Kcal)
	}
	again := curryOf(t, chicken, k)
	again.Food.ID, again.Food.Name, again.Yield, again.Portions = uuid.NewV7(), "chicken  CURRY", 0, 0
	got := saveRecipe(t, db, ana, again)
	if got.Food.ID != first.Food.ID {
		t.Errorf("saved by its name under %v, want the recipe's id %v", got.Food.ID, first.Food.ID)
	}
	// Not weighed: the raw 900 g stand for the dish.
	if s, ok := got.Food.Serving(recipe.Whole); !ok || s.Amount != 900 || len(got.Food.Servings) != 1 || got.Yield != 0 {
		t.Errorf("servings %+v, yield %v; want the whole of 900 g alone", got.Food.Servings, got.Yield)
	}
	if got.Items[0].Nutrients.Kcal != 720 {
		t.Errorf("the chicken brings %v kcal, want its values now, 720", got.Items[0].Nutrients.Kcal)
	}
}

func TestCreateAndCorrectRecipe(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	r := saveRecipe(t, db, ana, curryOf(t, chicken, k))

	dup := curryOf(t, chicken, k)
	var fc *store.FoodConflictError
	if _, err := db.CreateRecipe(t.Context(), ana, testActor, dup, nil, t0); !errors.As(err, &fc) || fc.Food.ID != r.Food.ID {
		t.Errorf("CreateRecipe of a name taken = %v, want a conflict with the recipe", err)
	}
	if _, err := db.CorrectRecipe(t.Context(), ana, testActor, dup, nil, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("CorrectRecipe of an id of none = %v, want ErrNotFound", err)
	}
	if _, err := db.CorrectRecipe(t.Context(), ivo, testActor, r, nil, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another user's CorrectRecipe = %v, want ErrNotFound", err)
	}
	r.Portions = 5
	if got, err := db.CorrectRecipe(t.Context(), ana, testActor, r, nil, t0); err != nil || got.Portions != 5 {
		t.Errorf("CorrectRecipe = %+v, %v", got, err)
	}
	// A food of the name that is no recipe is in the way.
	named := curryOf(t, chicken, k)
	named.Food.Name = "Kefir"
	if _, err := db.SaveRecipe(t.Context(), ana, testActor, named, nil, t0); !errors.As(err, &fc) || fc.Food.ID != k.ID {
		t.Errorf("SaveRecipe of a food's name = %v, want a conflict with the food", err)
	}
	named.Food.Name, named.Food.Aliases = "Kefir curry", []string{"the curry"}
	if _, err := db.SaveRecipe(t.Context(), ana, testActor, named, nil, t0); !errors.Is(err, store.ErrAliasTaken) {
		t.Errorf("SaveRecipe of another's alias = %v, want ErrAliasTaken", err)
	}
}

// A recipe may be an ingredient of another, but never of itself, at any
// depth.
func TestRecipeCycle(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	sauce := saveRecipe(t, db, ana, recipe.Recipe{
		Food:  food.Food{ID: uuid.NewV7(), Name: "Kefir sauce"},
		Items: []meal.Item{itemOf(t, k, meal.Quantity{Amount: 300, Unit: "ml"})},
	})
	dish := curryOf(t, chicken, k)
	dish.Items[1] = itemOf(t, sauce.Food, meal.Quantity{Servings: 1, Serving: recipe.Whole})
	dish = saveRecipe(t, db, ana, dish)

	sauce.Items = append(sauce.Items, itemOf(t, dish.Food, meal.Quantity{Amount: 100, Unit: "g"}))
	if _, err := db.SaveRecipe(t.Context(), ana, testActor, sauce, nil, t0); !errors.Is(err, store.ErrRecipeCycle) {
		t.Errorf("a sauce of the dish it is in = %v, want ErrRecipeCycle", err)
	}
	// Itself by its name: the id it comes under is the recipe's.
	self := dish
	self.Food.ID = uuid.NewV7()
	self.Items = append(self.Items, itemOf(t, dish.Food, meal.Quantity{Amount: 100, Unit: "g"}))
	if _, err := db.SaveRecipe(t.Context(), ana, testActor, self, nil, t0); !errors.Is(err, store.ErrRecipeCycle) {
		t.Errorf("the dish of itself = %v, want ErrRecipeCycle", err)
	}
}

func TestSaveRecipeInvalid(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	r := curryOf(t, chicken, k)
	r.Yield = 50 // 810 kcal in 50 g
	if _, err := db.SaveRecipe(t.Context(), ana, testActor, r, nil, t0); !errors.Is(err, store.ErrInvalidRecipe) {
		t.Errorf("SaveRecipe of no food that can be = %v, want ErrInvalidRecipe", err)
	}
	r.Items = nil
	if _, err := db.SaveRecipe(t.Context(), ana, testActor, r, nil, t0); !errors.Is(err, store.ErrInvalidRecipe) {
		t.Errorf("SaveRecipe of no ingredients = %v, want ErrInvalidRecipe", err)
	}
	if _, err := db.FoodByID(t.Context(), ana, r.Food.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a recipe refused left its food: %v", err)
	}
}

// A recipe's food changes with its recipe only: a save of it as a food
// is refused, and so is a food of source recipe that has none; barcodes
// added to it by a meal's new food are not a change of its values.
func TestRecipeFoodOnlyWithItsRecipe(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	r := saveRecipe(t, db, ana, curryOf(t, chicken, k))

	f := r.Food
	f.Nutrients.Kcal = 50
	if _, err := db.SaveFood(t.Context(), ana, testActor, f, t0); !errors.Is(err, store.ErrRecipeFood) {
		t.Errorf("SaveFood of a recipe's food = %v, want ErrRecipeFood", err)
	}
	f = r.Food
	f.Source = food.Label
	if _, err := db.CorrectFood(t.Context(), ana, testActor, f, t0); !errors.Is(err, store.ErrRecipeFood) {
		t.Errorf("CorrectFood to a label's = %v, want ErrRecipeFood", err)
	}
	orphan := chickenBreast()
	orphan.Name, orphan.Source = "Orphan", food.Recipe
	if _, err := db.SaveFood(t.Context(), ana, testActor, orphan, t0); !errors.Is(err, store.ErrRecipeFood) {
		t.Errorf("SaveFood of source recipe = %v, want ErrRecipeFood", err)
	}
	// A meal's new food of the recipe's name is the recipe, unchanged.
	sent := r.Food
	sent.ID, sent.Barcodes = uuid.NewV7(), []string{"4006381333931"}
	m := breakfast(t, saveFood(t, db, ana, bar()), t0)
	m.Items = []meal.Item{itemOf(t, sent, meal.Quantity{Amount: 250, Unit: "g"})}
	if _, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{sent}, t0); err != nil {
		t.Fatalf("a meal of the recipe as a new food: %v", err)
	}
	got, err := db.RecipeByID(t.Context(), ana, r.Food.ID)
	if err != nil || got.Food.Nutrients != r.Food.Nutrients || len(got.Food.Barcodes) != 1 || len(got.Items) != 2 {
		t.Errorf("the recipe after = %+v, %v; want it as it was, with the barcode", got, err)
	}
}

// A recipe's save is one change: undo takes it back whole, with the new
// foods it brought, or puts the recipe back as it was.
func TestUndoRecipe(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := kefir()
	r := saveRecipe(t, db, ana, curryOf(t, chicken, k), k)
	stored, err := foodNamed(t, db, ana, "Kefir")
	if err != nil {
		t.Fatal(err)
	}

	changed := r
	changed.Portions, changed.Items = 8, changed.Items[:1]
	saveRecipe(t, db, ana, changed)
	if e := undo(t, db, ana); e.Action != event.RecipeSave || e.Entity != r.Food.ID.String() {
		t.Fatalf("undone %s of %s, want the recipe's save", e.Action, e.Entity)
	}
	if got, err := db.RecipeByID(t.Context(), ana, r.Food.ID); err != nil || !reflect.DeepEqual(got, r) {
		t.Errorf("after the undo %+v, %v; want %+v", got, err, r)
	}

	if e := undo(t, db, ana); e.Action != event.RecipeSave {
		t.Fatalf("undone %s, want the recipe's create", e.Action)
	}
	foodGone(t, db, ana, r.Food.ID)
	foodGone(t, db, ana, stored.ID)
	if e := undo(t, db, ana); e.Action != event.FoodSave || e.Entity != chicken.ID.String() {
		t.Errorf("the next undo took back %s of %s, want the chicken's save", e.Action, e.Entity)
	}
}

// A recipe a meal has, or another recipe, stays when undo takes back its
// create: archived, with its ingredients, and their new foods with it.
func TestUndoRecipeInUse(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar())
	chicken := saveFood(t, db, ana, chickenBreast())
	k := kefir()
	r := saveRecipe(t, db, ana, curryOf(t, chicken, k), k)
	// An import's meal, which undo leaves.
	m := breakfast(t, b, t0)
	m.Items = []meal.Item{itemOf(t, r.Food, meal.Quantity{Servings: 1, Serving: recipe.Portion})}
	if _, _, err := db.CreateMeal(t.Context(), ana, importer, m, t0); err != nil {
		t.Fatal(err)
	}
	if e := undo(t, db, ana); e.Action != event.RecipeSave {
		t.Fatalf("undone %s, want the recipe's create", e.Action)
	}
	got, err := db.RecipeByID(t.Context(), ana, r.Food.ID)
	if err != nil || !got.Food.Archived || len(got.Items) != 2 {
		t.Errorf("the recipe in use after the undo = %+v, %v; want it archived, whole", got, err)
	}
	kept, err := db.FoodByID(t.Context(), ana, r.Items[1].FoodID)
	if err != nil || !kept.Archived {
		t.Errorf("its new food = %+v, %v; want it archived", kept, err)
	}
}

// A recipe changed since its save (archived) is not taken back.
func TestUndoRecipeChangedSince(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	r := saveRecipe(t, db, ana, curryOf(t, chicken, k))
	exec(t, db, "UPDATE recipes SET portions = 2 WHERE food_id = ?", r.Food.ID.String())
	var conflict *store.UndoConflictError
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0.Add(time.Hour)); !errors.As(err, &conflict) || conflict.Reason != store.UndoChanged {
		t.Errorf("undo = %v, want %s", err, store.UndoChanged)
	}
}

func TestRecentChangesNameARecipe(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	saveRecipe(t, db, ana, curryOf(t, chicken, k))
	cs, err := db.RecentChanges(t.Context(), ana, 1)
	if err != nil || len(cs) != 1 || cs[0].Event.Action != event.RecipeSave || cs[0].Name != "Chicken curry" {
		t.Errorf("RecentChanges = %+v, %v", cs, err)
	}
}

// A recipe saved again as it was read takes its ingredients' values as
// their foods have them now, not the ones it sends.
func TestSaveRecipeRefreshesItsIngredients(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	r := saveRecipe(t, db, ana, curryOf(t, chicken, k))
	chicken.Nutrients.Kcal = 120
	saveFood(t, db, ana, chicken)

	read, err := db.RecipeByID(t.Context(), ana, r.Food.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.CorrectRecipe(t.Context(), ana, testActor, read, nil, t0)
	if err != nil || got.Items[0].Nutrients.Kcal != 720 {
		t.Errorf("CorrectRecipe as read = %v kcal of chicken, %v; want its values now, 720", got.Items[0].Nutrients.Kcal, err)
	}
	read.Items[0].FoodID = uuid.NewV7()
	if _, err := db.CorrectRecipe(t.Context(), ana, testActor, read, nil, t0); !errors.Is(err, store.ErrInvalidRecipe) {
		t.Errorf("an ingredient of no food = %v, want ErrInvalidRecipe", err)
	}
}

// A barcode another food has is in the way of a recipe's, as of a
// food's.
func TestSaveRecipeBarcodeTaken(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, withBarcode(chickenBreast(), "4006381333931"))
	k := saveFood(t, db, ana, kefir())
	r := curryOf(t, chicken, k)
	r.Food.Barcodes = []string{"4006381333931"}
	var fc *store.FoodConflictError
	if _, err := db.SaveRecipe(t.Context(), ana, testActor, r, nil, t0); !errors.As(err, &fc) || fc.Food.ID != chicken.ID ||
		!errors.Is(err, store.ErrBarcodeTaken) {
		t.Errorf("SaveRecipe of a barcode taken = %v, want ErrBarcodeTaken by the chicken", err)
	}
}

// Undo does not put an ingredient back that is made of the recipe since:
// a recipe of itself.
func TestUndoRecipeIntoACycle(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	sauce := saveRecipe(t, db, ana, recipe.Recipe{
		Food:  food.Food{ID: uuid.NewV7(), Name: "Kefir sauce"},
		Items: []meal.Item{itemOf(t, k, meal.Quantity{Amount: 300, Unit: "ml"})},
	})
	dish := curryOf(t, chicken, k)
	dish.Items[1] = itemOf(t, sauce.Food, meal.Quantity{Servings: 1, Serving: recipe.Whole})
	dish = saveRecipe(t, db, ana, dish)
	dish.Items = dish.Items[:1]
	dish = saveRecipe(t, db, ana, dish)
	// An import makes the sauce of the dish; undo leaves its save.
	sauce.Items = append(sauce.Items, itemOf(t, dish.Food, meal.Quantity{Amount: 100, Unit: "g"}))
	if _, err := db.SaveRecipe(t.Context(), ana, importer, sauce, nil, t0); err != nil {
		t.Fatal(err)
	}
	var conflict *store.UndoConflictError
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0.Add(time.Hour)); !errors.As(err, &conflict) || conflict.Reason != store.UndoStuck {
		t.Errorf("undo into a cycle = %v, want %s", err, store.UndoStuck)
	}
	if got, err := db.RecipeByID(t.Context(), ana, dish.Food.ID); err != nil || len(got.Items) != 1 {
		t.Errorf("the dish after = %+v, %v; want it as it was, of the chicken alone", got, err)
	}
}

// A recipe saved again keeps the barcodes its food was given.
func TestSaveRecipeKeepsItsBarcodes(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	r := saveRecipe(t, db, ana, curryOf(t, chicken, k))
	f := r.Food
	f.Barcodes = []string{"4006381333931"}
	if _, err := db.SaveFood(t.Context(), ana, testActor, f, t0); err != nil {
		t.Fatal(err)
	}
	again := curryOf(t, chicken, k)
	again.Portions = 2
	if got := saveRecipe(t, db, ana, again); len(got.Food.Barcodes) != 1 || got.Portions != 2 {
		t.Errorf("saved again: barcodes %v, portions %d; want the barcode kept", got.Food.Barcodes, got.Portions)
	}
}

// A recipe saved as it was, but with an archived food it brings back,
// is one change: undo takes the food back to the archive with it.
func TestSaveRecipeUnchangedWithAFoodBroughtBack(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	r := saveRecipe(t, db, ana, curryOf(t, chicken, k))
	if err := db.SetFoodArchived(t.Context(), ana, testActor, k.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	sent := kefir()
	again := curryOf(t, chicken, sent)
	again.Food.ID = r.Food.ID
	saveRecipe(t, db, ana, again, sent)
	if e := undo(t, db, ana); e.Action != event.RecipeSave {
		t.Fatalf("undone %s, want the recipe's save", e.Action)
	}
	if got, err := db.FoodByID(t.Context(), ana, k.ID); err != nil || !got.Archived {
		t.Errorf("the kefir after the undo: %+v, %v; want it archived again", got, err)
	}
}

// A new ingredient of the recipe's name is in the way of the recipe.
func TestSaveRecipeOfANewIngredientsName(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := kefir()
	k.Name = "Chicken curry"
	if _, err := db.SaveRecipe(t.Context(), ana, testActor, curryOf(t, chicken, k), []food.Food{k}, t0); !errors.Is(err, store.ErrNameTaken) {
		t.Errorf("SaveRecipe = %v, want ErrNameTaken", err)
	}
}

// Correcting a recipe by the id of a food that is none is ErrNotFound.
func TestCorrectRecipeOfAFood(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	chicken := saveFood(t, db, ana, chickenBreast())
	k := saveFood(t, db, ana, kefir())
	r := curryOf(t, chicken, k)
	r.Food.ID = saveFood(t, db, ana, bar()).ID
	if _, err := db.CorrectRecipe(t.Context(), ana, testActor, r, nil, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("CorrectRecipe of a food's id = %v, want ErrNotFound", err)
	}
}
