package history_test

import (
	"math"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/history"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// splitFoods are made-up foods of the user's: by weight, per portion, by
// weight with a serving, a combination.
func splitFoods(t *testing.T, db *store.DB, userID uuid.UUID) {
	t.Helper()
	foods := []food.Food{
		{Name: "Obični jogurt", Basis: food.Per100g, Nutrients: food.Nutrients{Kcal: 60, Protein: 3.5, Carbs: 4.7, Fat: 3}},
		{Name: "Banana", Basis: food.PerPortion, Nutrients: food.Nutrients{Kcal: 105, Protein: 1.3, Carbs: 27, Fat: 0.3}},
		{
			Name: "Jaje", Basis: food.Per100g, Nutrients: food.Nutrients{Kcal: 143, Protein: 12.6, Carbs: 0.7, Fat: 9.5},
			Servings: []food.Serving{{Name: "komad", Amount: 50}},
		},
		{Name: "Kruh", Basis: food.Per100g, Nutrients: food.Nutrients{Kcal: 250, Protein: 8.5, Carbs: 49, Fat: 3.2}},
		{Name: "Jogurt + banana", Basis: food.PerPortion, Nutrients: food.Nutrients{Kcal: 285, Protein: 12, Carbs: 41, Fat: 9}},
	}
	for i := range foods {
		f := &foods[i]
		f.ID, f.Source = uuid.NewV7(), food.Estimate
		if _, err := db.SaveFood(t.Context(), userID, history.Actor, *f, now); err != nil {
			t.Fatal(err)
		}
	}
}

// mealsOn imports one day of meals and returns them as stored.
func mealsOn(t *testing.T, db *store.DB, u user.User, meals []history.Meal) (history.Report, []meal.Meal) {
	t.Helper()
	r, err := history.Import(t.Context(), db, u, []history.Day{{Date: "2026-09-01", Meals: meals}}, nil, now, false)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := db.MealsOfDay(t.Context(), u.ID, day.Date{Year: 2026, Month: 9, Day: 1})
	if err != nil {
		t.Fatal(err)
	}
	return r, stored
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// A meal of parts of the user's foods is split into their items, the rest
// of its total an estimate of its own, so it keeps its total.
func TestImportSplitsMeals(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	splitFoods(t, db, u.ID)
	total := food.Nutrients{Kcal: 600, Protein: 30, Carbs: 70, Fat: 18}
	r, stored := mealsOn(t, db, u, []history.Meal{
		{Time: "08:00", Description: "jogurt (300 g) + banana, 1 srednja + 2 jaja + sir (50 g)", Nutrients: total},
	})
	if r.Split != 1 || len(stored) != 1 {
		t.Fatalf("split %d, meals %+v", r.Split, stored)
	}
	m := stored[0]
	names := []string{}
	for _, it := range m.Items {
		names = append(names, it.Name)
	}
	if strings.Join(names, ", ") != "Obični jogurt, Banana, Jaje, "+history.SplitRestName {
		t.Fatalf("items %v; want the three foods, and the cheese, no food of the user's, in the rest", names)
	}
	checkSplitItems(t, m.Items)
	// Its kcal are the note's; a macronutrient the items pass within
	// rounding stays theirs.
	if got := m.Total(); !near(got.Kcal, total.Kcal) || math.Abs(got.Protein-total.Protein) > 2 || math.Abs(got.Carbs-total.Carbs) > 2 || math.Abs(got.Fat-total.Fat) > 2 {
		t.Errorf("the meal's total = %+v, want the note's, %+v", got, total)
	}
	if m.Description != "jogurt (300 g) + banana, 1 srednja + 2 jaja + sir (50 g)" || m.Client != meal.ClientImport {
		t.Errorf("the meal = %+v, want its text and an import's", m)
	}
}

// checkSplitItems checks the items of the meal TestImportSplitsMeals
// splits: 300 g of the yoghurt, a portion of the banana, two eggs of their
// serving.
func checkSplitItems(t *testing.T, items []meal.Item) {
	t.Helper()
	if it := items[0]; it.FoodID == uuid.Nil() || it.Amount != 300 || !near(it.Nutrients.Kcal, 180) {
		t.Errorf("the yoghurt = %+v, want 300 g of the food, 180 kcal", it)
	}
	if it := items[1]; it.Amount != 1 || it.Unit != "portion" {
		t.Errorf("the banana = %+v, want one portion", it)
	}
	if it := items[2]; it.Quantity.Servings != 2 || it.Amount != 100 {
		t.Errorf("the eggs = %+v, want two of the serving, 100 g", it)
	}
}

// A meal stays one estimate when no part is of a food, a part's amount
// is none the food takes, or its items would pass its total; a split with
// less than a kcal of its total left has no rest.
func TestImportSplitsMealsOrNot(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	splitFoods(t, db, u.ID)
	r, stored := mealsOn(t, db, u, []history.Meal{
		{Time: "08:00", Description: "ručak u restoranu", Nutrients: kcal(900)},
		// 500 g of yoghurt are 300 kcal, past the note's 200.
		{Time: "09:00", Description: "jogurt (500 g)", Nutrients: food.Nutrients{Kcal: 200, Protein: 12, Carbs: 15, Fat: 9}},
		// Two pieces of a food by weight without a serving.
		{Time: "10:00", Description: "2 kruha", Nutrients: kcal(300)},
		// Parts without amounts.
		{Time: "11:00", Description: "jogurt + banana", Nutrients: kcal(285)},
		// 100 g of bread are 250 kcal, 2 past the note's: the meal keeps
		// its kcal.
		{Time: "11:30", Description: "kruh (100 g)", Nutrients: food.Nutrients{Kcal: 248, Protein: 8.5, Carbs: 49, Fat: 3.2}},
		// Less than a kcal of the total left: no rest.
		{Time: "12:00", Description: "kruh (100 g)", Nutrients: food.Nutrients{Kcal: 250.5, Protein: 8.5, Carbs: 49, Fat: 3.2}},
	})
	if r.Split != 1 {
		t.Errorf("split %d, want only the bread whose total it meets", r.Split)
	}
	for _, m := range stored[:5] {
		if len(m.Items) != 1 || m.Items[0].FoodID != uuid.Nil() || m.Items[0].Name != m.Description {
			t.Errorf("%s = %+v, want one estimate", m.Description, m.Items)
		}
	}
	if m := stored[5]; len(m.Items) != 1 || m.Items[0].Name != "Kruh" {
		t.Errorf("the bread = %+v, want its food alone, as its total is met", m.Items)
	}
}

// A dry run splits against the table's foods too, which it has not
// written; a combination of the table is no food.
func TestImportSplitsDry(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	table := []history.FoodRow{
		{Line: 3, Name: "Obični jogurt", Portion: "100 g", Nutrients: food.Nutrients{Kcal: 60, Protein: 3.5, Carbs: 4.7, Fat: 3}},
		{Line: 4, Name: "Jogurt + kruh", Portion: "200 g jogurta + 50 g kruha", Nutrients: kcal(245)},
	}
	days := []history.Day{{Date: "2026-09-01", Meals: []history.Meal{
		{Description: "jogurt (200 g)", Nutrients: food.Nutrients{Kcal: 130, Protein: 7, Carbs: 10, Fat: 6}},
		{Description: "jogurt + kruh (1 porcija)", Nutrients: kcal(245)},
	}}}
	r, err := history.Import(t.Context(), db, u, days, table, now, true)
	if err != nil || r.Split != 1 {
		t.Errorf("a dry run: split %d, %v; want the yoghurt", r.Split, err)
	}
	if r, err := history.Import(t.Context(), db, u, days, table, now, false); err != nil || r.Split != 0 {
		t.Errorf("a run that writes, of foods it has not: split %d, %v; want none", r.Split, err)
	}
}

// The food table's combinations are left out, as a food is one food.
func TestImportFoodsLeavesCombinationsOut(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	r, err := history.ImportFoods(t.Context(), db, u, []history.FoodRow{
		{Line: 3, Name: "Jagode", Portion: "100 g", Nutrients: food.Nutrients{Kcal: 32, Protein: 0.7, Carbs: 7.7, Fat: 0.3}},
		{Line: 4, Name: "Skyr + jagode", Portion: "150 g", Nutrients: kcal(130)},
		{Line: 5, Name: "Zdjela", Portion: "150 g skyra + 100 g jagoda", Nutrients: kcal(130)},
	}, now, false)
	if err != nil || r.Created != 1 || strings.Join(r.Combinations, "; ") != "line 4, Skyr + jagode; line 5, Zdjela" {
		t.Errorf("report %+v, %v; want one food, the combinations left out", r, err)
	}
	if _, err := db.FoodByName(t.Context(), u.ID, "Skyr + jagode", ""); err == nil {
		t.Error("the combination is a food")
	}
}

// A meal of more parts of foods than a meal takes items stays one
// estimate, and the import goes on.
func TestImportSplitsNoMoreItemsThanAMealTakes(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	splitFoods(t, db, u.ID)
	text := strings.TrimSuffix(strings.Repeat("kruh (1 g) + ", meal.MaxItems), " + ")
	// Room for the items' macronutrients: only their count is too many.
	r, stored := mealsOn(t, db, u, []history.Meal{{Description: text, Nutrients: food.Nutrients{Kcal: 300, Protein: 10, Carbs: 40, Fat: 5}}})
	if r.Split != 0 || len(stored) != 1 || len(stored[0].Items) != 1 {
		t.Errorf("split %d, meals %d; want one estimate", r.Split, len(stored))
	}
}

// A run again that would split a meal a run before wrote as one estimate
// leaves it, and counts it as no split.
func TestImportSplitsAgain(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	meals := []history.Meal{{Description: "kruh (100 g)", Nutrients: food.Nutrients{Kcal: 300, Protein: 10, Carbs: 50, Fat: 5}}}
	if r, _ := mealsOn(t, db, u, meals); r.Split != 0 {
		t.Fatalf("split %d without foods", r.Split)
	}
	splitFoods(t, db, u.ID)
	r, stored := mealsOn(t, db, u, meals)
	if r.Split != 0 || len(stored[0].Items) != 1 || len(r.Skipped) != 1 {
		t.Errorf("again: split %d, items %d, skipped %v; want the estimate of the run before", r.Split, len(stored[0].Items), r.Skipped)
	}
}

// A combination of a food's name gives it no portion in its name.
func TestImportFoodsCombinationOfAName(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	if _, err := history.ImportFoods(t.Context(), db, u, []history.FoodRow{
		{Line: 3, Name: "Zobena kaša", Portion: "100 g", Nutrients: food.Nutrients{Kcal: 370, Protein: 13, Carbs: 60, Fat: 7}},
		{Line: 4, Name: "Zobena kaša", Portion: "50 g zobenih + 200 ml mlijeka", Nutrients: kcal(285)},
	}, now, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.FoodByName(t.Context(), u.ID, "Zobena kaša", ""); err != nil {
		t.Errorf("the food: %v; want it by its own name", err)
	}
}
