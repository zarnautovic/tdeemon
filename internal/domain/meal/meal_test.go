package meal

import (
	"math"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

func egg() food.Food {
	return food.Food{
		ID: uuid.NewV7(), Name: "Egg", Basis: food.Per100g, Source: food.Estimate,
		Nutrients: food.Nutrients{Kcal: 143, Protein: 12.6, Carbs: 0.7, Fat: 9.5, Fiber: new(0.0)},
		Servings:  []food.Serving{{Name: "piece", Amount: 60}},
	}
}

// tray is eggs by the tray of 30.
func tray() food.Food {
	f := egg()
	f.Servings = []food.Serving{{Name: "tray", Amount: 1800}}
	return f
}

func milk() food.Food {
	return food.Food{
		ID: uuid.NewV7(), Name: "Milk 2.8 %", Brand: "Dukat", Basis: food.Per100ml, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 58, Protein: 3.3, Carbs: 4.7, Fat: 2.8},
	}
}

func canteen(portionGrams float64) food.Food {
	return food.Food{
		ID: uuid.NewV7(), Name: "Bean stew with sausage", Basis: food.PerPortion, Source: food.Estimate,
		Nutrients: food.Nutrients{Kcal: 850, Protein: 40, Carbs: 90, Fat: 30}, PortionGrams: portionGrams,
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestFromFood(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		f          food.Food
		q          Quantity
		wantAmount float64
		wantUnit   string
		wantKcal   float64
		wantName   string
	}{
		{"grams", egg(), Quantity{Amount: 150, Unit: "g"}, 150, "g", 214.5, "Egg"},
		{"four pieces", egg(), Quantity{Servings: 4, Serving: "piece"}, 240, "g", 343.2, "Egg"},
		{"half a piece, the serving in another case", egg(), Quantity{Servings: 0.5, Serving: "Piece"}, 30, "g", 42.9, "Egg"},
		{"ml, with the brand in the name", milk(), Quantity{Amount: 250, Unit: "ml"}, 250, "ml", 145, "Dukat Milk 2.8 %"},
		{"a whole portion", canteen(0), Quantity{Portions: 1}, 1, "portion", 850, "Bean stew with sausage"},
		{"half a portion", canteen(0), Quantity{Portions: 0.5}, 0.5, "portion", 425, "Bean stew with sausage"},
		{"grams of a portion of known weight", canteen(500), Quantity{Amount: 250, Unit: "g"}, 0.5, "portion", 425, "Bean stew with sausage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			it, err := FromFood(tc.f, tc.q, "")
			if err != nil {
				t.Fatalf("FromFood: %v", err)
			}
			if !near(it.Amount, tc.wantAmount) || it.Unit != tc.wantUnit || !near(it.Nutrients.Kcal, tc.wantKcal) ||
				it.Name != tc.wantName || it.FoodID != tc.f.ID || it.Source != tc.f.Source || it.Quantity != tc.q {
				t.Errorf("item = %+v; want %v %s, %v kcal, %q", it, tc.wantAmount, tc.wantUnit, tc.wantKcal, tc.wantName)
			}
		})
	}
}

// Optional nutrients scale with the item and stay unknown when unknown.
func TestFromFoodScalesOptionalNutrients(t *testing.T) {
	t.Parallel()

	it, err := FromFood(egg(), Quantity{Amount: 200, Unit: "g"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if it.Nutrients.Fiber == nil || *it.Nutrients.Fiber != 0 || it.Nutrients.Salt != nil {
		t.Errorf("optional nutrients = fiber %v, salt %v; want 0 and unknown", it.Nutrients.Fiber, it.Nutrients.Salt)
	}
}

// The item is a snapshot: changing the food afterwards changes nothing.
func TestFromFoodIsASnapshot(t *testing.T) {
	t.Parallel()

	f := egg()
	it, err := FromFood(f, Quantity{Amount: 100, Unit: "g"}, "")
	if err != nil {
		t.Fatal(err)
	}
	f.Nutrients.Kcal, f.Name = 999, "Ostrich egg"
	*f.Nutrients.Fiber = 50
	if it.Nutrients.Kcal != 143 || it.Name != "Egg" || *it.Nutrients.Fiber != 0 {
		t.Errorf("the item changed with the food: %+v", it)
	}
}

func TestFromFoodRefuses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		f    food.Food
		q    Quantity
		note string
		want string
	}{
		{"nothing", egg(), Quantity{}, "", "exactly one"},
		{"two at once", egg(), Quantity{Amount: 10, Portions: 1}, "", "exactly one"},
		{"a negative amount", egg(), Quantity{Amount: -5, Unit: "g"}, "", "more than 0"},
		{"NaN", egg(), Quantity{Amount: math.NaN(), Unit: "g"}, "", "more than 0"},
		{"too much", egg(), Quantity{Amount: MaxAmount + 1, Unit: "g"}, "", "at most 10000"},
		{"too many servings", egg(), Quantity{Servings: MaxServings + 1, Serving: "piece"}, "", "at most 100"},
		{"servings adding up to too much", tray(), Quantity{Servings: 100, Serving: "tray"}, "", "at most 10000"},
		{"too many portions", canteen(0), Quantity{Portions: MaxPortions + 1}, "", "at most 50"},
		{"servings without a name", egg(), Quantity{Servings: 2}, "", "the serving's name"},
		{"a name without servings", egg(), Quantity{Amount: 10, Unit: "g", Serving: "piece"}, "", "needs a number"},
		{"an unknown serving", egg(), Quantity{Servings: 1, Serving: "slice"}, "", `the food has "piece"`},
		{"a serving of a food with none", milk(), Quantity{Servings: 1, Serving: "glass"}, "", "has none"},
		{"portions of a food by weight", egg(), Quantity{Portions: 1}, "", "measured in g"},
		{"an amount without a unit", egg(), Quantity{Amount: 100}, "", `unit, g or ml, not ""`},
		{"an amount in cups", egg(), Quantity{Amount: 1, Unit: "cup"}, "", `not "cup"`},
		{"a unit without an amount", egg(), Quantity{Servings: 1, Serving: "piece", Unit: "g"}, "", "a unit needs an amount"},
		{"ml of a food by weight", egg(), Quantity{Amount: 250, Unit: "ml"}, "", "measured in g, not ml; give g"},
		{"grams of a food by volume", milk(), Quantity{Amount: 250, Unit: "g"}, "", "measured in ml, not g"},
		{"ml of a portion", canteen(500), Quantity{Amount: 250, Unit: "ml"}, "", "not ml"},
		{"grams making too many portions", canteen(100), Quantity{Amount: 6000, Unit: "g"}, "", "60 portions: want at most 50"},
		{"grams of a portion of unknown weight", canteen(0), Quantity{Amount: 300, Unit: "g"}, "", "weight of a portion is not known"},
		{"servings of a portion", canteen(0), Quantity{Servings: 1, Serving: "piece"}, "", "takes portions"},
		{"a note with a control character", egg(), Quantity{Amount: 10, Unit: "g"}, "a\x00b", "control or invisible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := FromFood(tc.f, tc.q, tc.note); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("FromFood = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func TestEstimate(t *testing.T) {
	t.Parallel()

	it, err := Estimate("Restaurant pasta", food.Nutrients{Kcal: 850, Protein: 32, Carbs: 105, Fat: 30}, "no oil seen")
	if err != nil {
		t.Fatal(err)
	}
	if it.FoodID != uuid.Nil() || it.Unit != "portion" || it.Amount != 1 || it.Source != food.Estimate || it.Nutrients.Kcal != 850 {
		t.Errorf("estimate = %+v", it)
	}
	for _, tc := range []struct {
		name, item string
		n          food.Nutrients
		want       string
	}{
		{"no name", "", food.Nutrients{Kcal: 100}, "name is required"},
		{"negative fat", "Pasta", food.Nutrients{Kcal: 100, Fat: -1}, "fat -1"},
		{"a typo of a feast", "Pasta", food.Nutrients{Kcal: 85000}, "at most 10000"},
		{"a name too long", strings.Repeat("a", MaxNameLen+1), food.Nutrients{Kcal: 1}, "at most 200"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Estimate(tc.item, tc.n, ""); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Estimate = %v, want an error with %q", err, tc.want)
			}
		})
	}
	if _, err := Estimate("Pasta", food.Nutrients{Kcal: 1}, " "); err == nil {
		t.Error("a note of only space was accepted")
	}
	if _, err := Estimate("Pasta", food.Nutrients{Kcal: 1}, "no oil\nlots of cheese"); err != nil {
		t.Errorf("a note over two lines: %v", err)
	}
	if _, err := Estimate("Pasta\ncarbonara", food.Nutrients{Kcal: 1}, ""); err == nil {
		t.Error("a name over two lines was accepted")
	}
}

func meal(t *testing.T, at time.Time, items ...Item) Meal {
	t.Helper()
	return Meal{ID: uuid.NewV7(), EatenAt: at, Day: day.DateOf(at), Client: ClientMCP, Items: items}
}

func item(t *testing.T, f food.Food, q Quantity) Item {
	t.Helper()
	it, err := FromFood(f, q, "")
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestTotals(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 8, 30, 0, 0, time.UTC)
	breakfast := meal(t, at, item(t, egg(), Quantity{Servings: 2, Serving: "piece"}), item(t, milk(), Quantity{Amount: 200, Unit: "ml"}))
	got := breakfast.Total()
	// 120 g of egg (171.6 kcal, 15.12 P) and 200 ml of milk (116 kcal, 6.6 P).
	if !near(got.Kcal, 287.6) || !near(got.Protein, 21.72) || got.Fiber == nil || *got.Fiber != 0 {
		t.Errorf("Total = %+v", got)
	}
	est, _ := Estimate("Cake", food.Nutrients{Kcal: 300, Protein: 5, Carbs: 40, Fat: 14}, "")
	dinner := meal(t, at.Add(10*time.Hour), est)
	if d := DayTotal([]Meal{breakfast, dinner}); !near(d.Kcal, 587.6) || !near(d.Fat, 11.4+5.6+14) {
		t.Errorf("DayTotal = %+v", d)
	}
	if d := DayTotal(nil); d.Kcal != 0 || d.Fiber != nil {
		t.Errorf("DayTotal of no meals = %+v", d)
	}
}

func TestMealValidate(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 8, 30, 0, 0, time.UTC)
	if err := meal(t, at, item(t, egg(), Quantity{Amount: 50, Unit: "g"})).Validate(); err != nil {
		t.Fatalf("a valid meal: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Meal)
		want   string
	}{
		{"no id", func(m *Meal) { m.ID = uuid.Nil() }, "id is missing"},
		{"no time", func(m *Meal) { m.EatenAt = time.Time{} }, "time it was eaten"},
		{"before 1970", func(m *Meal) { m.EatenAt = time.Date(1969, 12, 31, 23, 0, 0, 0, time.UTC) }, "eaten in 1969"},
		{"past year 9999", func(m *Meal) { m.EatenAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }, "eaten in 10000"},
		{"no day", func(m *Meal) { m.Day = day.Date{} }, "day is missing"},
		{"an unknown client", func(m *Meal) { m.Client = "fax" }, "client"},
		{"no items", func(m *Meal) { m.Items = nil }, "at least one item"},
		{"too many items", func(m *Meal) {
			for range MaxItems {
				m.Items = append(m.Items, m.Items[0])
			}
		}, "at most 50"},
		{"a description with space around", func(m *Meal) { m.Description = " breakfast" }, "space at the start"},
		{"a note too long", func(m *Meal) { m.Note = strings.Repeat("n", MaxNoteLen+1) }, "at most 2000"},
		{"a note not UTF-8", func(m *Meal) { m.Note = "\xff" }, "not valid UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := meal(t, at, item(t, egg(), Quantity{Amount: 50, Unit: "g"}))
			tc.change(&m)
			if err := m.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

// The fingerprint is what the client said: the same meal sent again has
// the same one, whatever the code worked out since; anything the client
// changed gives another.
func TestFingerprint(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 8, 30, 0, 0, time.UTC)
	e, m := egg(), milk()
	est, _ := Estimate("Cake", food.Nutrients{Kcal: 300, Protein: 5, Carbs: 40, Fat: 14, Sugars: new(20.0)}, "")
	base := func() Meal {
		return meal(t, at, item(t, e, Quantity{Servings: 2, Serving: "piece"}), item(t, m, Quantity{Amount: 200, Unit: "ml"}), est)
	}
	fp := base().Fingerprint()
	if len(fp) != 64 {
		t.Fatalf("fingerprint %q, want 64 hex digits", fp)
	}
	same := map[string]func(*Meal){
		"another id":           func(x *Meal) { x.ID = uuid.NewV7() },
		"another day":          func(x *Meal) { x.Day = x.Day.AddDays(1) },
		"another client":       func(x *Meal) { x.Client = ClientWeb },
		"the time in a zone":   func(x *Meal) { x.EatenAt = x.EatenAt.In(time.FixedZone("CEST", 2*3600)) },
		"item ids":             func(x *Meal) { x.Items[0].ID = uuid.NewV7() },
		"the food's new kcal":  func(x *Meal) { x.Items[0].Nutrients.Kcal = 999 },
		"the food's new name":  func(x *Meal) { x.Items[1].Name = "Milk 3.2 %" },
		"a food item's source": func(x *Meal) { x.Items[1].Source = food.User },
	}
	for name, change := range same {
		x := base()
		change(&x)
		if x.Fingerprint() != fp {
			t.Errorf("%s changed the fingerprint", name)
		}
	}
	other := map[string]func(*Meal){
		"another time":           func(x *Meal) { x.EatenAt = x.EatenAt.Add(time.Second) },
		"a description":          func(x *Meal) { x.Description = "breakfast" },
		"a note":                 func(x *Meal) { x.Note = "late" },
		"another food":           func(x *Meal) { x.Items[0].FoodID = uuid.NewV7() },
		"more servings":          func(x *Meal) { x.Items[0].Quantity.Servings = 3 },
		"another serving":        func(x *Meal) { x.Items[0].Quantity.Serving = "Piece" },
		"another amount":         func(x *Meal) { x.Items[1].Quantity.Amount = 250 },
		"another unit":           func(x *Meal) { x.Items[1].Quantity.Unit = "g" },
		"portions":               func(x *Meal) { x.Items[2].Quantity.Portions = 0.5 },
		"an item's note":         func(x *Meal) { x.Items[1].Note = "cold" },
		"an estimate's name":     func(x *Meal) { x.Items[2].Name = "Pie" },
		"an estimate's kcal":     func(x *Meal) { x.Items[2].Nutrients.Kcal = 301 },
		"an estimate's fat":      func(x *Meal) { x.Items[2].Nutrients.Fat = 15 },
		"an estimate's sugars":   func(x *Meal) { x.Items[2].Nutrients.Sugars = new(21.0) },
		"an estimate's no salt":  func(x *Meal) { x.Items[2].Nutrients.Salt = new(0.0) },
		"no sugars known":        func(x *Meal) { x.Items[2].Nutrients.Sugars = nil },
		"another order":          func(x *Meal) { x.Items[0], x.Items[1] = x.Items[1], x.Items[0] },
		"an item less":           func(x *Meal) { x.Items = x.Items[:2] },
		"a note moved to a line": func(x *Meal) { x.Note, x.Description = "", "\"\"\n\"\"" },
	}
	for name, change := range other {
		x := base()
		change(&x)
		if x.Fingerprint() == fp {
			t.Errorf("%s left the fingerprint as it was", name)
		}
	}
}

func TestMealValidateItemIDs(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 8, 30, 0, 0, time.UTC)
	a, b := item(t, egg(), Quantity{Amount: 50, Unit: "g"}), item(t, egg(), Quantity{Amount: 60, Unit: "g"})
	a.ID, b.ID = uuid.NewV7(), uuid.NewV7()
	if err := meal(t, at, a, b, item(t, egg(), Quantity{Amount: 70, Unit: "g"}), item(t, egg(), Quantity{Amount: 80, Unit: "g"})).Validate(); err != nil {
		t.Errorf("distinct ids and items without one: %v", err)
	}
	b.ID = a.ID
	if err := meal(t, at, a, b).Validate(); err == nil || !strings.Contains(err.Error(), "given twice") {
		t.Errorf("an item id twice = %v", err)
	}
}
