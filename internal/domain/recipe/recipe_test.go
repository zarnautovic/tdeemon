package recipe

import (
	"math"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

var (
	chicken = food.Food{
		ID: uuid.NewV7(), Name: "Chicken breast", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 110, Protein: 23, Fat: 2, Fiber: new(0.0), Salt: new(0.2)},
	}
	cream = food.Food{
		ID: uuid.NewV7(), Name: "Cooking cream", Basis: food.Per100ml, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 200, Protein: 2.5, Carbs: 3.5, Fat: 20, Salt: new(0.1)},
	}
	egg = food.Food{
		ID: uuid.NewV7(), Name: "Egg", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 143, Protein: 12.6, Carbs: 0.7, Fat: 9.5},
		Servings:  []food.Serving{{Name: "piece", Amount: 60}},
	}
	muffin = food.Food{
		ID: uuid.NewV7(), Name: "Muffin", Basis: food.PerPortion, Source: food.Estimate,
		Nutrients: food.Nutrients{Kcal: 400, Protein: 6, Carbs: 50, Fat: 19},
	}
)

func item(t *testing.T, f food.Food, q meal.Quantity) meal.Item {
	t.Helper()
	it, err := meal.FromFood(f, q, "")
	if err != nil {
		t.Fatalf("FromFood(%s): %v", f.Name, err)
	}
	return it
}

func curry(t *testing.T) Recipe {
	t.Helper()
	return Recipe{
		Food: food.Food{ID: uuid.NewV7(), Name: "Chicken curry"},
		Items: []meal.Item{
			item(t, chicken, meal.Quantity{Amount: 500, Unit: "g"}),
			item(t, cream, meal.Quantity{Amount: 200, Unit: "ml"}),
			item(t, egg, meal.Quantity{Servings: 2, Serving: "piece"}),
		},
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestMakeWeighed(t *testing.T) {
	t.Parallel()

	r := curry(t)
	r.Yield, r.Portions = 800, 4
	got, err := Make(r)
	if err != nil {
		t.Fatalf("Make: %v", err)
	}
	f := got.Food
	if f.Basis != food.Per100g || f.Source != food.Recipe || f.ID != r.Food.ID || f.Name != "Chicken curry" {
		t.Errorf("food = %+v, want the recipe's, by weight, of source recipe", f)
	}
	// 550 + 400 + 171.6 kcal in 800 g.
	if want := (550 + 400 + 171.6) / 8; !near(f.Nutrients.Kcal, want) {
		t.Errorf("kcal per 100 g = %v, want %v", f.Nutrients.Kcal, want)
	}
	// The whole dish logged is its total, whatever it weighs.
	whole := item(t, f, meal.Quantity{Servings: 1, Serving: Whole})
	if total := got.Total(); !near(whole.Nutrients.Kcal, total.Kcal) || !near(whole.Nutrients.Protein, total.Protein) {
		t.Errorf("the whole dish = %+v, want the total %+v", whole.Nutrients, total)
	}
	portion := item(t, f, meal.Quantity{Servings: 1, Serving: Portion})
	if !near(portion.Amount, 200) || !near(portion.Nutrients.Kcal, got.Total().Kcal/4) {
		t.Errorf("a portion = %v g, %v kcal; want 200 g, a quarter of the total", portion.Amount, portion.Nutrients.Kcal)
	}
	tenth := item(t, f, meal.Quantity{Servings: 0.1, Serving: Whole})
	if !near(tenth.Nutrients.Kcal, got.Total().Kcal/10) {
		t.Errorf("a tenth = %v kcal, want %v", tenth.Nutrients.Kcal, got.Total().Kcal/10)
	}
	if ws := got.Warnings(); len(ws) != 0 {
		t.Errorf("Warnings = %v, want none", ws)
	}
}

func TestMakeNotWeighed(t *testing.T) {
	t.Parallel()

	r := curry(t)
	got, err := Make(r)
	if err != nil {
		t.Fatalf("Make: %v", err)
	}
	// The raw ingredients stand for the dish: 500 g + 200 ml + 120 g.
	if s, ok := got.Food.Serving(Whole); !ok || !near(s.Amount, 820) {
		t.Errorf("servings = %+v, want the whole dish of 820 g", got.Food.Servings)
	}
	if len(got.Food.Servings) != 1 {
		t.Errorf("servings = %+v, want no portion of a recipe not divided", got.Food.Servings)
	}
	tenth := item(t, got.Food, meal.Quantity{Servings: 0.1, Serving: Whole})
	if !near(tenth.Nutrients.Kcal, got.Total().Kcal/10) {
		t.Errorf("a tenth = %v kcal, want a tenth of the total, %v", tenth.Nutrients.Kcal, got.Total().Kcal/10)
	}
}

func TestMakePortionsOfNoWeight(t *testing.T) {
	t.Parallel()

	r := Recipe{
		Food:  food.Food{ID: uuid.NewV7(), Name: "Trifle"},
		Items: []meal.Item{item(t, muffin, meal.Quantity{Portions: 2}), item(t, cream, meal.Quantity{Amount: 100, Unit: "ml"})},
	}
	if _, err := Make(r); err == nil || !strings.Contains(err.Error(), "weigh the cooked dish") {
		t.Errorf("Make of an ingredient of no weight, not weighed = %v, want a refusal to weigh it", err)
	}
	r.Yield = 300
	if _, err := Make(r); err != nil {
		t.Errorf("Make weighed: %v", err)
	}
}

func TestMakePortionsOfAKnownWeight(t *testing.T) {
	t.Parallel()

	weighed := muffin
	weighed.PortionGrams = 90
	r := Recipe{
		Food:  food.Food{ID: uuid.NewV7(), Name: "Trifle"},
		Items: []meal.Item{item(t, weighed, meal.Quantity{Portions: 2}), item(t, cream, meal.Quantity{Amount: 100, Unit: "ml"})},
	}
	got, err := Make(r)
	if err != nil {
		t.Fatalf("Make of portions of a known weight, not weighed: %v", err)
	}
	// 2 muffins of 90 g and 100 ml of cream.
	if raw, ok := got.RawWeight(); !ok || !near(raw, 280) {
		t.Errorf("RawWeight = %v, %v; want 280 g", raw, ok)
	}
	// The weight is the one the portion had when the recipe was saved.
	weighed.PortionGrams = 120
	if raw, _ := got.RawWeight(); !near(raw, 280) {
		t.Errorf("RawWeight after the food was corrected = %v, want 280 g as saved", raw)
	}
}

func TestTotalOptional(t *testing.T) {
	t.Parallel()

	r := curry(t)
	total := r.Total()
	// Salt: chicken and cream state it, the egg does not.
	if total.Salt != nil || total.Fiber != nil {
		t.Errorf("salt %v, fiber %v; want unknown, as the egg states neither", total.Salt, total.Fiber)
	}
	r.Items = r.Items[:2]
	if total := r.Total(); total.Salt == nil || !near(*total.Salt, 1+0.2) {
		t.Errorf("salt of chicken and cream = %v, want 1.2", total.Salt)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		change func(*Recipe)
		want   string
	}{
		{"brand", func(r *Recipe) { r.Food.Brand = "Mama's" }, "no brand"},
		{"no ingredients", func(r *Recipe) { r.Items = nil }, "at least one ingredient"},
		{"too many", func(r *Recipe) {
			for len(r.Items) <= MaxItems {
				r.Items = append(r.Items, r.Items[0])
			}
		}, "at most 50"},
		{"estimate", func(r *Recipe) { r.Items[0].FoodID = uuid.Nil() }, "not an estimate"},
		{"itself", func(r *Recipe) { r.Items[1].FoodID = r.Food.ID }, "its own ingredient"},
		{"yield", func(r *Recipe) { r.Yield = MaxWeight + 1 }, "yield"},
		{"negative yield", func(r *Recipe) { r.Yield = -1 }, "yield"},
		{"pot", func(r *Recipe) { r.Pot = math.NaN() }, "pot"},
		{"portions", func(r *Recipe) { r.Portions = MaxPortions + 1 }, "portions"},
	} {
		r := curry(t)
		tc.change(&r)
		if err := r.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate = %v, want an error with %q", tc.name, err, tc.want)
		}
		if _, err := Make(r); err == nil {
			t.Errorf("%s: Make took it", tc.name)
		}
	}
}

func TestMakeRefusesWhatNoFoodHas(t *testing.T) {
	t.Parallel()

	// 1121.6 kcal in 100 g: more than pure fat.
	r := curry(t)
	r.Yield = 100
	if _, err := Make(r); err == nil || !strings.Contains(err.Error(), "the dish of 100 g") {
		t.Errorf("Make of 1121 kcal in 100 g = %v, want a refusal", err)
	}
	// Raw ingredients past what a dish may weigh, not weighed.
	r = curry(t)
	r.Items[0] = item(t, chicken, meal.Quantity{Amount: meal.MaxAmount, Unit: "g"})
	if _, err := Make(r); err == nil || !strings.Contains(err.Error(), "weigh the cooked dish") {
		t.Errorf("Make of %v g raw = %v, want a refusal", meal.MaxAmount+320, err)
	}
}

func TestWarnings(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		yield float64
		want  string
	}{
		{820, ""},
		{250, ""},
		{240, WarnYieldLow},
		{2460, ""},
		{2470, WarnYieldHigh},
	} {
		r := curry(t)
		r.Yield = tc.yield
		ws := r.Warnings()
		switch {
		case tc.want == "" && len(ws) != 0:
			t.Errorf("yield %v: Warnings = %v, want none", tc.yield, ws)
		case tc.want != "" && (len(ws) != 1 || ws[0].Code != tc.want):
			t.Errorf("yield %v: Warnings = %v, want %s", tc.yield, ws, tc.want)
		}
	}
}
