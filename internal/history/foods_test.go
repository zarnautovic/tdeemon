package history_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/history"
	"github.com/zarnautovic/tdeemon/internal/store"
)

func readFoods(t *testing.T) []history.FoodRow {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "foods.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, skipped, err := history.ReadFoods(f)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Broken", "3 cells", "8 cells", `"350-400"`, "No kcal: no kcal", "Shake: no kcal", `"99 g"`, "a row of no food table"}
	if len(skipped) != len(want) {
		t.Fatalf("skipped = %q", skipped)
	}
	for i, w := range want {
		if !strings.Contains(skipped[i], w) {
			t.Errorf("skipped[%d] = %q, want %q in it", i, skipped[i], w)
		}
	}
	return rows
}

// The table's rows read past its header and its aligned separator, a
// blank line, a pipe too many, a value with its unit, a dash and a bound.
func TestReadFoods(t *testing.T) {
	t.Parallel()

	rows := readFoods(t)
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
	}
	if !slices.Equal(names, []string{"Rye bread", "Skyr cup", "Orange juice", "Kefir", "Apple", "Stew", "Stew", "Cereal bar", "Black coffee", "Pickles", "Odd", "Guessed", "Water", "Tea", "Whey"}) {
		t.Fatalf("names = %q", names)
	}
	if bar := rows[7]; bar.Portion != "25 g" || bar.Nutrients != (food.Nutrients{Kcal: 99, Protein: 2.5, Carbs: 16, Fat: 3}) || bar.Line != 15 {
		t.Errorf("the cereal bar = %+v", bar)
	}
	if coffee, pickles := rows[8].Nutrients, rows[9].Nutrients; coffee != (food.Nutrients{Kcal: 4}) || pickles.Protein != 0.5 {
		t.Errorf("a dash %+v, a bound %+v", coffee, pickles)
	}
}

// Each row becomes a food: per 100 g or ml with its portion as a serving
// when the portion is a weight or a volume, per portion otherwise; a name
// rows share takes the portion; a run again keeps what is there.
func TestImportFoods(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	rows := readFoods(t)
	dry, err := history.ImportFoods(t.Context(), db, u, rows, now, true)
	if err != nil || dry.Created != 15 {
		t.Fatalf("the dry run = %+v, %v", dry, err)
	}
	if es, _ := db.Events(t.Context(), u.ID, 1); len(es) != 0 {
		t.Fatal("the dry run wrote")
	}
	r, err := history.ImportFoods(t.Context(), db, u, rows, now, false)
	if err != nil || r.Created != 15 || r.Kept != 0 || len(r.Renamed) != 2 || len(r.Skipped) != 0 {
		t.Fatalf("report = %+v, %v", r, err)
	}
	checkFoods(t, db, u.ID)

	again, err := history.ImportFoods(t.Context(), db, u, rows, now, false)
	if err != nil || again.Created != 0 || again.Kept != 15 || len(again.Renamed) != 0 {
		t.Errorf("the run again = %+v, %v", again, err)
	}
}

// checkFoods checks the foods the table of testdata/foods.txt gave.
func checkFoods(t *testing.T, db *store.DB, userID uuid.UUID) {
	t.Helper()
	for name, want := range map[string]struct {
		basis    food.Basis
		kcal     float64
		serving  float64
		source   food.Source
		noteHead string
	}{
		"Rye bread":             {food.Per100g, 240, 0, food.Estimate, "standard"},
		"Skyr cup":              {food.Per100g, 64, 150, food.Label, "prema"},
		"Orange juice":          {food.Per100ml, 45, 0, food.Estimate, "estimate"},
		"Kefir":                 {food.Per100ml, 50, 330, food.Estimate, "estimate"},
		"Apple":                 {food.PerPortion, 95, 0, food.Estimate, "Portion: 1 medium. estimate"},
		"Stew (1/4 of the pot)": {food.PerPortion, 520, 0, food.Estimate, "Portion: 1/4 of the pot."},
		"Cereal bar":            {food.Per100g, 396, 25, food.Estimate, "a pipe"},
		"Odd":                   {food.PerPortion, 300, 0, food.Estimate, "Portion: 5 g."},
		"Guessed":               {food.Per100g, 120, 0, food.Estimate, "bez"},
		"Water":                 {food.Per100ml, 0, 250, food.Estimate, ""},
		"Whey":                  {food.Per100g, 400, 30, food.Estimate, "labels"},
	} {
		f, err := db.FoodByName(t.Context(), userID, name, "")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var serving float64
		if s, ok := f.Serving(history.ServingName); ok {
			serving = s.Amount
		}
		if f.Basis != want.basis || f.Nutrients.Kcal != want.kcal || serving != want.serving || f.Source != want.source || !strings.HasPrefix(f.Note, want.noteHead) {
			t.Errorf("%s = %+v", name, f)
		}
	}
}

// A food the user has by the name stays theirs; a name that is no name,
// or a row whose values no food allows, is skipped.
func TestImportFoodsKeepsAndSkips(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	mine := food.Food{ID: uuid.NewV7(), Name: "Rye bread", Basis: food.Per100g, Source: food.User, Nutrients: food.Nutrients{Kcal: 250}}
	if _, err := db.SaveFood(t.Context(), u.ID, history.Actor, mine, now); err != nil {
		t.Fatal(err)
	}
	rows := []history.FoodRow{
		{Line: 1, Name: "Rye bread", Portion: "100 g", Nutrients: food.Nutrients{Kcal: 240}},
		{Line: 2, Name: "tab\there", Portion: "1 piece", Nutrients: food.Nutrients{Kcal: 10}},
		{Line: 3, Name: "A feast", Portion: "1 pot", Nutrients: food.Nutrients{Kcal: 20000}},
		{Line: 4, Name: strings.Repeat("ž", 250), Portion: "1 piece", Nutrients: food.Nutrients{Kcal: 10}},
		{Line: 5, Name: "Twice", Portion: "1 piece", Nutrients: food.Nutrients{Kcal: 10}},
		{Line: 6, Name: "Twice", Portion: "1 piece", Nutrients: food.Nutrients{Kcal: 12}},
		{Line: 7, Name: "", Portion: "1 piece", Nutrients: food.Nutrients{Kcal: 12}},
	}
	r, err := history.ImportFoods(t.Context(), db, u, rows, now, false)
	if err != nil || r.Kept != 1 || r.Created != 2 || len(r.Skipped) != 4 {
		t.Fatalf("report = %+v, %v", r, err)
	}
	if f, _ := db.FoodByName(t.Context(), u.ID, "Rye bread", ""); f.Nutrients.Kcal != 250 || f.Source != food.User {
		t.Errorf("the user's own = %+v", f)
	}
}

// A note too long for a food is cut, a space at its cut end too, and the
// food stays per 100 g; a portion larger than a serving may be is a food
// per portion.
func TestImportFoodsEdges(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	rows := []history.FoodRow{
		{Line: 1, Name: "Granola", Portion: "50 g", Nutrients: food.Nutrients{Kcal: 220}, Note: strings.Repeat("n", 2100)},
		{Line: 2, Name: "Muesli", Portion: "50 g", Nutrients: food.Nutrients{Kcal: 200}, Note: strings.Repeat("n", food.MaxNoteLen-1) + " more"},
		{Line: 3, Name: "Pot of stew", Portion: "20000 g", Nutrients: food.Nutrients{Kcal: 2000}},
	}
	if r, err := history.ImportFoods(t.Context(), db, u, rows, now, false); err != nil || r.Created != 3 {
		t.Fatalf("report = %+v, %v", r, err)
	}
	f, err := db.FoodByName(t.Context(), u.ID, "Granola", "")
	if err != nil || f.Basis != food.Per100g || f.Nutrients.Kcal != 440 || len([]rune(f.Note)) != food.MaxNoteLen {
		t.Errorf("granola = %+v, %v", f, err)
	}
	if f, err := db.FoodByName(t.Context(), u.ID, "Muesli", ""); err != nil || len([]rune(f.Note)) != food.MaxNoteLen-1 {
		t.Errorf("muesli = %+v, %v", f, err)
	}
	if f, err := db.FoodByName(t.Context(), u.ID, "Pot of stew", ""); err != nil || f.Basis != food.PerPortion {
		t.Errorf("the pot = %+v, %v", f, err)
	}
}
