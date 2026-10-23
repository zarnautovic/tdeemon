package store_test

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// breakfast is two bars and a one-off estimate, eaten at.
func breakfast(t *testing.T, bar food.Food, at time.Time) meal.Meal {
	t.Helper()
	two, err := meal.FromFood(bar, meal.Quantity{Servings: 2, Serving: "bar"}, "")
	if err != nil {
		t.Fatal(err)
	}
	coffee, err := meal.Estimate("Cappuccino", food.Nutrients{Kcal: 110, Protein: 6, Carbs: 9, Fat: 5.5, Sugars: new(9.0)}, "oat milk")
	if err != nil {
		t.Fatal(err)
	}
	return meal.Meal{
		ID: uuid.NewV7(), EatenAt: at, Day: day.DateOf(at), Description: "breakfast", Client: meal.ClientMCP,
		Items: []meal.Item{two, coffee}, Note: "at the desk",
	}
}

func createMeal(t *testing.T, db *store.DB, userID uuid.UUID, m meal.Meal) meal.Meal {
	t.Helper()
	got, created, err := db.CreateMeal(t.Context(), userID, testActor, m, t0)
	if err != nil || !created {
		t.Fatalf("CreateMeal = %v, %v", created, err)
	}
	return got
}

// sameButIDs reports whether got is want with the item ids the store gave.
func sameButIDs(got, want meal.Meal) bool {
	if len(got.Items) != len(want.Items) {
		return false
	}
	w := want
	w.Items = append([]meal.Item(nil), want.Items...)
	for i := range w.Items {
		if got.Items[i].ID == uuid.Nil() {
			return false
		}
		w.Items[i].ID = got.Items[i].ID
	}
	return reflect.DeepEqual(got, w)
}

func TestCreateMealAndReadItBack(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	want := breakfast(t, bar, t0.Add(30*time.Minute))
	got := createMeal(t, db, ana, want)
	// Every field, the items' quantities, snapshots and absent nutrients
	// included; the estimate has no food.
	if !sameButIDs(got, want) {
		t.Errorf("stored %+v, want %+v", got, want)
	}
	if got.Items[0].ID == got.Items[1].ID {
		t.Error("two items with one id")
	}
	again, err := db.MealByID(t.Context(), ana, want.ID)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Errorf("MealByID = %+v, %v", again, err)
	}
}

// The time eaten is kept to the second.
func TestCreateMealKeepsSeconds(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	m := breakfast(t, saveFood(t, db, ana, bar()), t0.Add(1500*time.Millisecond))
	got := createMeal(t, db, ana, m)
	if !got.EatenAt.Equal(t0.Add(time.Second)) {
		t.Errorf("eaten at %v, want %v", got.EatenAt, t0.Add(time.Second))
	}
	// The meal as stored, sent again, is a retry: the fingerprint counts
	// the time to the second, as stored.
	if _, created, err := db.CreateMeal(t.Context(), ana, testActor, got, t0); err != nil || created {
		t.Errorf("the stored meal sent again = %v, %v; want a retry", created, err)
	}
}

// The same meal sent again is the meal stored, even after its food
// changed; nothing is written twice.
func TestCreateMealIsIdempotent(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	m := breakfast(t, bar, t0)
	first := createMeal(t, db, ana, m)
	bar.Nutrients.Kcal = 400
	saveFood(t, db, ana, bar)
	retry := breakfast(t, bar, t0)
	retry.ID = m.ID
	got, created, err := db.CreateMeal(t.Context(), ana, testActor, retry, t0.Add(time.Minute))
	if err != nil || created || !reflect.DeepEqual(got, first) {
		t.Errorf("a retry = %+v, %v, %v; want the first meal, not created", got, created, err)
	}
	if n := countRows(t, db, "meal_items"); n != 2 {
		t.Errorf("%d items, want 2", n)
	}
}

// Another meal under a meal's id is a conflict, and the meal under it
// stays. Ids are per user: another user's meal under the id is no
// conflict, and the two meals stay apart.
func TestCreateMealConflicts(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	m := breakfast(t, saveFood(t, db, ana, bar()), t0)
	first := createMeal(t, db, ana, m)
	other := m
	other.Note = "standing"
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, other, t0); !errors.Is(err, store.ErrMealConflict) {
		t.Errorf("another meal under the id = %v, want ErrMealConflict", err)
	}
	ivos := breakfast(t, saveFood(t, db, ivo, bar()), t0.Add(time.Hour))
	ivos.ID = m.ID
	if got, created, err := db.CreateMeal(t.Context(), ivo, testActor, ivos, t0); err != nil || !created || got.ID != m.ID {
		t.Errorf("ivo under ana's meal id = %v, %v; want his meal, created under it", created, err)
	}
	if got, _ := db.MealByID(t.Context(), ana, m.ID); !reflect.DeepEqual(got, first) {
		t.Errorf("ana's meal became %+v", got)
	}
	if got, _ := db.MealByID(t.Context(), ivo, m.ID); !got.EatenAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("ivo's meal is %+v", got)
	}
	if err := db.DeleteMeal(t.Context(), ivo, testActor, m.ID, t0); err != nil {
		t.Fatal(err)
	}
	if got, err := db.MealByID(t.Context(), ana, m.ID); err != nil || len(got.Items) != 2 {
		t.Errorf("after ivo deleted his meal, ana's = %+v, %v", got, err)
	}
}

func TestCreateMealRefuses(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	bad := breakfast(t, saveFood(t, db, ana, bar()), t0)
	bad.Items = nil
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, bad, t0); err == nil || !strings.Contains(err.Error(), "invalid meal") {
		t.Errorf("a meal without items = %v", err)
	}
	// An item of another user's food: the foreign key refuses it, and
	// nothing of the meal is kept.
	theirs := breakfast(t, saveFood(t, db, ivo, bar()), t0)
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, theirs, t0); err == nil {
		t.Error("a meal of another user's food was stored")
	}
	if n := countRows(t, db, "meals"); n != 0 {
		t.Errorf("%d meals, want none", n)
	}
}

func TestReplaceMeal(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	original := breakfast(t, bar, t0)
	first := createMeal(t, db, ana, original)
	// Another time and note, the bars halved, the coffee removed, a bar
	// added under the client's id and one without an id.
	edit := first
	edit.EatenAt, edit.Note, edit.Client = t0.Add(time.Hour), "on the train", meal.ClientWeb
	half, err := meal.FromFood(bar, meal.Quantity{Servings: 1, Serving: "bar"}, "")
	if err != nil {
		t.Fatal(err)
	}
	half.ID = first.Items[0].ID
	added, _ := meal.FromFood(bar, meal.Quantity{Amount: 30, Unit: "g"}, "")
	added.ID = uuid.NewV7()
	another, _ := meal.FromFood(bar, meal.Quantity{Amount: 20, Unit: "g"}, "")
	edit.Items = []meal.Item{half, added, another}
	got, err := db.ReplaceMeal(t.Context(), ana, testActor, edit, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 3 || got.Items[0].ID != first.Items[0].ID || got.Items[1].ID != added.ID ||
		got.Items[2].ID == uuid.Nil() || got.Items[0].Amount != 55 || !got.EatenAt.Equal(t0.Add(time.Hour)) ||
		got.Note != "on the train" {
		t.Errorf("replaced as %+v", got)
	}
	// The client it was logged through stays.
	if got.Client != meal.ClientMCP {
		t.Errorf("client = %s, want the one it was logged through", got.Client)
	}
	if n := countRows(t, db, "meal_items"); n != 3 {
		t.Errorf("%d items, want 3", n)
	}
}

// Retries around an edit: the edit again adds nothing, a late retry of
// the create finds the edited meal, and the edited content is not what
// was created under the id.
func TestReplaceMealAndRetries(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	original := breakfast(t, bar, t0)
	first := createMeal(t, db, ana, original)
	edit := first
	edit.Note = "on the train"
	added, _ := meal.FromFood(bar, meal.Quantity{Amount: 30, Unit: "g"}, "")
	added.ID = uuid.NewV7()
	edit.Items = append(append([]meal.Item(nil), first.Items...), added)
	got, err := db.ReplaceMeal(t.Context(), ana, testActor, edit, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// The same edit again (a retried "add") adds nothing.
	again, err := db.ReplaceMeal(t.Context(), ana, testActor, edit, t0.Add(time.Hour))
	if err != nil || len(again.Items) != 3 || again.Items[2].ID != added.ID || countRows(t, db, "meal_items") != 3 {
		t.Errorf("the edit again = %+v, %v", again, err)
	}
	// A late retry of the create finds the meal as it is now; the edited
	// content under the id is not what was created.
	if m, created, err := db.CreateMeal(t.Context(), ana, testActor, original, t0); err != nil || created || !reflect.DeepEqual(m, again) {
		t.Errorf("a late retry of the create = %+v, %v, %v; want the edited meal", m, created, err)
	}
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, got, t0); !errors.Is(err, store.ErrMealConflict) {
		t.Errorf("creating the edited content under the id = %v, want ErrMealConflict", err)
	}
}

// An item under the id of another meal's item is a conflict, and nothing
// of the edit is kept.
func TestReplaceMealItemOfAnotherMeal(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	first := createMeal(t, db, ana, breakfast(t, bar, t0))
	other := createMeal(t, db, ana, breakfast(t, bar, t0.Add(5*time.Hour)))
	edit := first
	edit.Note = "changed"
	edit.Items = append([]meal.Item(nil), first.Items...)
	edit.Items[1].ID = other.Items[0].ID
	if _, err := db.ReplaceMeal(t.Context(), ana, testActor, edit, t0); !errors.Is(err, store.ErrMealConflict) {
		t.Errorf("an item of another meal = %v, want ErrMealConflict", err)
	}
	for _, m := range []meal.Meal{first, other} {
		if got, _ := db.MealByID(t.Context(), ana, m.ID); !reflect.DeepEqual(got, m) {
			t.Errorf("meal %s became %+v", m.ID, got)
		}
	}
	// The same at creation.
	fresh := breakfast(t, bar, t0.Add(9*time.Hour))
	fresh.Items[0].ID = other.Items[1].ID
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, fresh, t0); !errors.Is(err, store.ErrMealConflict) {
		t.Errorf("creating with another meal's item id = %v, want ErrMealConflict", err)
	}
}

// Two users with a meal under one id and the same day excluded: what one
// does to theirs leaves the other's as it was.
func TestMealsUnderOneIDStayApart(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	d := day.DateOf(t0)
	anasBar, ivosBar := saveFood(t, db, ana, bar()), saveFood(t, db, ivo, bar())
	anas := breakfast(t, anasBar, t0)
	anas = createMeal(t, db, ana, anas)
	ivos := breakfast(t, ivosBar, t0.Add(2*time.Hour))
	ivos.ID = anas.ID
	for i := range ivos.Items {
		ivos.Items[i].ID = anas.Items[i].ID // the same item ids too
	}
	ivos = createMeal(t, db, ivo, ivos)
	for _, u := range []uuid.UUID{ana, ivo} {
		if err := db.ExcludeDay(t.Context(), u, testActor, d, day.Exclusion{Reason: "wedding"}, t0); err != nil {
			t.Fatal(err)
		}
	}
	edit := anas
	edit.Items = []meal.Item{anas.Items[0]}
	edit.Items[0].Note = "only one"
	if _, err := db.ReplaceMeal(t.Context(), ana, testActor, edit, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.IncludeDay(t.Context(), ana, testActor, d, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.MealByID(t.Context(), ivo, ivos.ID); !reflect.DeepEqual(got, ivos) {
		t.Errorf("ivo's meal became %+v, want %+v", got, ivos)
	}
	if got, _ := db.Exclusions(t.Context(), ivo, d, d); len(got) != 1 {
		t.Errorf("ivo's exclusion = %v, want it kept", got)
	}
	uses, err := db.LastUses(t.Context(), ana, []uuid.UUID{anasBar.ID})
	if err != nil || !uses[anasBar.ID].At.Equal(t0) {
		t.Errorf("ana's last use = %+v, %v; want her own meal's time", uses, err)
	}
	if err := db.DeleteMeal(t.Context(), ana, testActor, anas.ID, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.MealByID(t.Context(), ivo, ivos.ID); !reflect.DeepEqual(got, ivos) {
		t.Errorf("after ana's delete ivo's meal is %+v", got)
	}
}

func TestReplaceMealRefuses(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	edit := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	edit.Note = "changed"
	if _, err := db.ReplaceMeal(t.Context(), ivo, testActor, edit, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ivo replacing ana's meal = %v, want ErrNotFound", err)
	}
	if got, _ := db.MealByID(t.Context(), ana, edit.ID); got.Note == "changed" {
		t.Error("ivo changed ana's meal")
	}
	nope := edit
	nope.ID = uuid.NewV7()
	if _, err := db.ReplaceMeal(t.Context(), ana, testActor, nope, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("replacing a meal nobody has = %v, want ErrNotFound", err)
	}
	nope.Items = nil
	if _, err := db.ReplaceMeal(t.Context(), ana, testActor, nope, t0); err == nil || !strings.Contains(err.Error(), "invalid meal") {
		t.Errorf("replacing with no items = %v", err)
	}
}

func TestDeleteMeal(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	if err := db.DeleteMeal(t.Context(), ivo, testActor, m.ID, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ivo deleting ana's meal = %v, want ErrNotFound", err)
	}
	if err := db.DeleteMeal(t.Context(), ana, testActor, m.ID, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MealByID(t.Context(), ana, m.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("MealByID after delete = %v", err)
	}
	if n := countRows(t, db, "meal_items"); n != 0 {
		t.Errorf("%d items left", n)
	}
	if err := db.DeleteMeal(t.Context(), ana, testActor, m.ID, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleting twice = %v, want ErrNotFound", err)
	}
}

func TestMealsOfDayAndBetween(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	late := createMeal(t, db, ana, breakfast(t, bar, t0.Add(10*time.Hour)))
	early := createMeal(t, db, ana, breakfast(t, bar, t0))
	createMeal(t, db, ana, breakfast(t, bar, t0.Add(24*time.Hour)))
	createMeal(t, db, ivo, breakfast(t, saveFood(t, db, ivo, bar), t0.Add(time.Hour)))
	got, err := db.MealsOfDay(t.Context(), ana, day.DateOf(t0))
	if err != nil || len(got) != 2 || got[0].ID != early.ID || got[1].ID != late.ID || len(got[0].Items) != 2 {
		t.Errorf("MealsOfDay = %d meals, %v; want ana's two of the day, in the order eaten", len(got), err)
	}
	got, err = db.MealsBetween(t.Context(), ana, t0, t0.Add(10*time.Hour))
	if err != nil || len(got) != 2 || got[0].ID != early.ID {
		t.Errorf("MealsBetween, both ends = %d meals, %v", len(got), err)
	}
	if got, _ := db.MealsBetween(t.Context(), ana, t0.Add(time.Second), t0.Add(10*time.Hour-time.Second)); len(got) != 0 {
		t.Errorf("MealsBetween, inside = %d meals, want none", len(got))
	}
	if got, _ := db.MealsOfDay(t.Context(), ivo, day.DateOf(t0)); len(got) != 1 {
		t.Errorf("ivo's day = %d meals, want his one", len(got))
	}
}

func TestIntakeOfDays(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	early := createMeal(t, db, ana, breakfast(t, bar, t0))
	// Of the day before by its day, though eaten on t0's: as a day that
	// starts later than midnight has it.
	late := breakfast(t, bar, t0.Add(-7*time.Hour))
	late.Day = day.DateOf(t0).AddDays(-1)
	late = createMeal(t, db, ana, late)
	createMeal(t, db, ana, breakfast(t, bar, t0.Add(-6*time.Hour)))
	createMeal(t, db, ana, breakfast(t, bar, t0.Add(48*time.Hour)))
	createMeal(t, db, ivo, breakfast(t, saveFood(t, db, ivo, bar), t0.Add(time.Hour)))
	got, err := db.IntakeOfDays(t.Context(), ana, late.Day, day.DateOf(t0).AddDays(1))
	if err != nil {
		t.Fatal(err)
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	n := early.Total()
	if in := got[day.DateOf(t0)]; len(got) != 2 || in.Meals != 2 || !near(in.Kcal, 2*n.Kcal) || !near(in.Protein, 2*n.Protein) ||
		!near(in.Carbs, 2*n.Carbs) || !near(in.Fat, 2*n.Fat) {
		t.Errorf("IntakeOfDays = %+v; want ana's two meals of t0's day, at %v kcal each, and the one of the day before", got, n.Kcal)
	}
	if in := got[late.Day]; in.Meals != 1 || !near(in.Kcal, late.Total().Kcal) {
		t.Errorf("the day before = %+v, want the meal of its day", in)
	}
	if got, err := db.IntakeOfDays(t.Context(), ivo, late.Day, late.Day); err != nil || len(got) != 0 {
		t.Errorf("ivo's day before = %v, %v; want none", got, err)
	}
}

func TestFirstMealDay(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	if _, ok, err := db.FirstMealDay(t.Context(), ana); ok || err != nil {
		t.Errorf("no meal: %v, %v; want none", ok, err)
	}
	bar := saveFood(t, db, ana, bar())
	createMeal(t, db, ana, breakfast(t, bar, t0))
	createMeal(t, db, ana, breakfast(t, bar, t0.Add(-48*time.Hour)))
	createMeal(t, db, ivo, breakfast(t, saveFood(t, db, ivo, bar), t0.Add(-96*time.Hour)))
	if got, ok, err := db.FirstMealDay(t.Context(), ana); !ok || err != nil || got != day.DateOf(t0).AddDays(-2) {
		t.Errorf("FirstMealDay = %s, %v, %v; want ana's of two days before t0", got, ok, err)
	}
}

func TestLastUses(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	unused := saveFood(t, db, ana, food.Food{
		ID: uuid.NewV7(), Name: "Rye bread", Basis: food.Per100g, Source: food.User,
		Nutrients: food.Nutrients{Kcal: 259, Protein: 8.5, Carbs: 48, Fat: 3.3},
	})
	createMeal(t, db, ana, breakfast(t, bar, t0))
	later, _ := meal.FromFood(bar, meal.Quantity{Amount: 30, Unit: "g"}, "")
	then, _ := meal.FromFood(bar, meal.Quantity{Servings: 0.5, Serving: "bar"}, "")
	m := meal.Meal{
		ID: uuid.NewV7(), EatenAt: t0.Add(2 * time.Hour), Day: day.DateOf(t0), Client: meal.ClientChat,
		Items: []meal.Item{later, then},
	}
	createMeal(t, db, ana, m)
	// Ivo's later use of his own bar, under the same food id (ids are per
	// user), is his, not ana's.
	ivosMeal := createMeal(t, db, ivo, breakfast(t, saveFood(t, db, ivo, bar), t0.Add(5*time.Hour)))
	got, err := db.LastUses(t.Context(), ana, []uuid.UUID{bar.ID, unused.ID, uuid.NewV7()})
	if err != nil {
		t.Fatal(err)
	}
	want := store.LastUse{At: t0.Add(2 * time.Hour), MealID: m.ID, Quantity: meal.Quantity{Servings: 0.5, Serving: "bar"}, Amount: 27.5, Unit: "g"}
	if len(got) != 1 || !reflect.DeepEqual(got[bar.ID], want) {
		t.Errorf("LastUses = %+v, want only the bar's last item, %+v", got, want)
	}
	if got, _ := db.LastUses(t.Context(), ivo, []uuid.UUID{bar.ID}); len(got) != 1 || got[bar.ID].MealID != ivosMeal.ID {
		t.Errorf("ivo's last use = %+v, want his own meal", got)
	}
	// The most a search finds are looked up, and no more.
	limit := store.MaxFoodQueries * store.MaxFoodResults
	many := make([]uuid.UUID, limit+1)
	for i := range many {
		many[i] = uuid.NewV7()
	}
	many[limit-1] = bar.ID
	if got, _ := db.LastUses(t.Context(), ana, many); len(got) != 1 {
		t.Errorf("the last id a search finds was not looked up: %+v", got)
	}
	many[limit-1], many[limit] = uuid.NewV7(), bar.ID
	if got, _ := db.LastUses(t.Context(), ana, many); len(got) != 0 {
		t.Errorf("an id past what a search finds was looked up: %+v", got)
	}
}

// Two meals in the same second: the last use is the later meal's, by id
// (a UUIDv7 grows with time).
func TestLastUsesInOneSecond(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	at := t0.Add(3 * time.Hour)
	earlier := breakfast(t, bar, at)
	later := breakfast(t, bar, at)
	later.Items[0], _ = meal.FromFood(bar, meal.Quantity{Amount: 40, Unit: "g"}, "")
	// Stored in the order made: SQLite would return the first row of a tie,
	// the earlier meal's, without the tie-break.
	createMeal(t, db, ana, earlier)
	createMeal(t, db, ana, later)
	got, err := db.LastUses(t.Context(), ana, []uuid.UUID{bar.ID})
	if err != nil || got[bar.ID].MealID != later.ID {
		t.Errorf("LastUses = %+v, %v; want the later meal %s", got, err, later.ID)
	}
}

func TestExcludedDays(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	d := day.DateOf(t0)
	if err := db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "restaurant"}, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.ExcludeDay(t.Context(), ana, testActor, d.AddDays(2), day.Exclusion{Reason: "sick"}, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "wedding"}, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.ExcludeDay(t.Context(), ivo, testActor, d.AddDays(1), day.Exclusion{Reason: "travel"}, t0); err != nil {
		t.Fatal(err)
	}
	got, err := db.Exclusions(t.Context(), ana, d, d.AddDays(2))
	want := map[day.Date]day.Exclusion{d: {Reason: "wedding"}, d.AddDays(2): {Reason: "sick"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Exclusions = %v, %v; want %v", got, err, want)
	}
	if got, _ := db.Exclusions(t.Context(), ana, d.AddDays(1), d.AddDays(1)); len(got) != 0 {
		t.Errorf("ana sees ivo's day: %v", got)
	}
	if err := db.IncludeDay(t.Context(), ana, testActor, d, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.IncludeDay(t.Context(), ana, testActor, d.AddDays(5), t0); err != nil {
		t.Errorf("including a day never excluded: %v", err)
	}
	if got, _ := db.Exclusions(t.Context(), ana, d, d.AddDays(5)); len(got) != 1 {
		t.Errorf("after including a day = %v", got)
	}
	if err := db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: " "}, t0); err == nil {
		t.Error("an exclusion without a reason was stored")
	}
	if err := db.ExcludeDay(t.Context(), ana, testActor, day.Date{}, day.Exclusion{Reason: "sick"}, t0); err == nil ||
		!strings.Contains(err.Error(), "the day is missing") {
		t.Errorf("an exclusion without a day = %v", err)
	}
}

// A user's deletion takes their meals, items and days with their foods;
// a food used by an item cannot go on its own.
func TestMealsGoWithTheirUser(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	anas := saveFood(t, db, ana, bar())
	createMeal(t, db, ana, breakfast(t, anas, t0))
	createMeal(t, db, ivo, breakfast(t, saveFood(t, db, ivo, bar()), t0))
	if err := db.ExcludeDay(t.Context(), ivo, testActor, day.DateOf(t0), day.Exclusion{Reason: "sick"}, t0); err != nil {
		t.Fatal(err)
	}
	err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "DELETE FROM foods WHERE id = ?", anas.ID.String())
		return err
	})
	if err == nil {
		t.Error("a food in use was deleted")
	}
	exec(t, db, "DELETE FROM users WHERE id = '"+ivo.String()+"'")
	if m, i, d := countRows(t, db, "meals"), countRows(t, db, "meal_items"), countRows(t, db, "days"); m != 1 || i != 2 || d != 0 {
		t.Errorf("%d meals, %d items, %d days left; want ana's 1, 2 and 0", m, i, d)
	}
}

func TestMealStoreFailures(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithoutCancel(t.Context())
	d := day.DateOf(t0)
	again := m
	again.ID = uuid.NewV7()
	if _, _, err := db.CreateMeal(ctx, ana, testActor, again, t0); err == nil {
		t.Error("CreateMeal on a closed database succeeded")
	}
	if _, err := db.ReplaceMeal(ctx, ana, testActor, m, t0); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Errorf("ReplaceMeal on a closed database = %v", err)
	}
	if err := db.DeleteMeal(ctx, ana, testActor, m.ID, t0); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Errorf("DeleteMeal on a closed database = %v", err)
	}
	if _, err := db.MealByID(ctx, ana, m.ID); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Errorf("MealByID on a closed database = %v", err)
	}
	if _, err := db.MealsOfDay(ctx, ana, d); err == nil {
		t.Error("MealsOfDay on a closed database succeeded")
	}
	if _, err := db.MealsBetween(ctx, ana, t0, t0); err == nil {
		t.Error("MealsBetween on a closed database succeeded")
	}
	if _, err := db.LastUses(ctx, ana, []uuid.UUID{m.Items[0].FoodID}); err == nil {
		t.Error("LastUses on a closed database succeeded")
	}
	if err := db.ExcludeDay(ctx, ana, testActor, d, day.Exclusion{Reason: "sick"}, t0); err == nil {
		t.Error("ExcludeDay on a closed database succeeded")
	}
	if err := db.IncludeDay(ctx, ana, testActor, d, t0); err == nil {
		t.Error("IncludeDay on a closed database succeeded")
	}
	if _, err := db.Exclusions(ctx, ana, d, d); err == nil {
		t.Error("Exclusions on a closed database succeeded")
	}
}

// A row the store did not write as it should (by hand, or by an older
// bug) is an error, not a meal with a zero field.
func TestMealsRejectCorruptRows(t *testing.T) {
	t.Parallel()

	const bad = "'zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz'" // UUID-sized, so the CHECK lets it in
	for _, tc := range []struct {
		name  string
		stmts []string
		byDay bool // read the day's meals, else the meals around t0
	}{
		{"meal id", []string{"UPDATE meal_items SET meal_id = " + bad, "UPDATE meals SET id = " + bad}, true},
		{"eaten at", []string{"UPDATE meals SET eaten_at = '2026-19-39T29:59:69Z'"}, true},
		{"day", []string{"UPDATE meals SET day = '2026-19-39'"}, false},
		{"item id", []string{"UPDATE meal_items SET id = " + bad + " WHERE position = 0"}, true},
		{"item food id", []string{
			"UPDATE foods SET id = " + bad, "UPDATE food_servings SET food_id = " + bad,
			"UPDATE food_aliases SET food_id = " + bad, "UPDATE meal_items SET food_id = " + bad + " WHERE food_id IS NOT NULL",
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, ana, _ := withTwoUsers(t)
			m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
			err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
				if _, err := tx.ExecContext(t.Context(), "PRAGMA defer_foreign_keys = ON"); err != nil {
					return err
				}
				for _, q := range tc.stmts {
					if _, err := tx.ExecContext(t.Context(), q); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("corrupt the row: %v", err)
			}
			if tc.byDay {
				_, err = db.MealsOfDay(t.Context(), ana, m.Day)
			} else {
				_, err = db.MealsBetween(t.Context(), ana, t0.Add(-time.Hour), t0.Add(time.Hour))
			}
			if err == nil {
				t.Errorf("read a meal with a bad %s", tc.name)
			}
		})
	}
	// A last use with a bad time.
	db, ana, _ := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	createMeal(t, db, ana, breakfast(t, bar, t0))
	exec(t, db, "UPDATE meals SET eaten_at = '2026-19-39T29:59:69Z'")
	if _, err := db.LastUses(t.Context(), ana, []uuid.UUID{bar.ID}); err == nil {
		t.Error("LastUses read a bad time")
	}
}

// A day excluded or included, now in or out, was decided; another user's
// day is not the user's, and a day never touched was not.
func TestDayExclusionRecorded(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	d := day.DateOf(t0)
	if err := db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "wedding"}, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.IncludeDay(t.Context(), ana, testActor, d, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.ExcludeDay(t.Context(), ana, testActor, d.AddDays(2), day.Exclusion{Reason: "sick"}, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.ExcludeDay(t.Context(), ivo, testActor, d.AddDays(1), day.Exclusion{Reason: "travel"}, t0); err != nil {
		t.Fatal(err)
	}
	for dd, want := range map[day.Date]bool{d: true, d.AddDays(2): true, d.AddDays(1): false, d.AddDays(3): false} {
		if got, err := db.DayExclusionRecorded(t.Context(), ana, dd); err != nil || got != want {
			t.Errorf("DayExclusionRecorded(%s) = %v, %v; want %v", dd, got, err, want)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DayExclusionRecorded(context.WithoutCancel(t.Context()), ana, d); err == nil {
		t.Error("DayExclusionRecorded on a closed database succeeded")
	}
}
