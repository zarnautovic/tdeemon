package food

import (
	"math"
	"strings"
	"testing"
	"uuid"
)

// An egg, by weight, with a named serving.
func egg() Food {
	return Food{
		ID: uuid.NewV7(), Name: "Egg", Basis: Per100g, Source: Estimate,
		Nutrients: Nutrients{Kcal: 143, Protein: 12.6, Carbs: 0.7, Fat: 9.5},
		Servings:  []Serving{{Name: "piece", Amount: 60}},
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	canteen := Food{
		ID: uuid.NewV7(), Name: "Canteen bean stew", Basis: PerPortion, Source: Estimate,
		Nutrients: Nutrients{Kcal: 850, Protein: 40, Carbs: 90, Fat: 30}, PortionGrams: 500,
	}
	// Honey by volume: 100 ml weigh 142 g, more than 100 g of sugar.
	honey := Food{
		ID: uuid.NewV7(), Name: "Honey", Basis: Per100ml, Source: Label,
		Nutrients: Nutrients{Kcal: 432, Protein: 0.4, Carbs: 116, Fat: 0, Sugars: new(115.0)},
	}
	for name, f := range map[string]Food{"a food by weight": egg(), "a food per portion": canteen, "a food by volume": honey} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := f.Validate(); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*Food)
		want   string
	}{
		{"no id", func(f *Food) { f.ID = uuid.Nil() }, "id is missing"},
		{"no name", func(f *Food) { f.Name = " " }, "name is required"},
		{"space around the name", func(f *Food) { f.Name = "Egg " }, "space at the start or end"},
		{"a control character", func(f *Food) { f.Name = "E\tgg" }, "control or invisible"},
		{"a zero-width space, a look-alike", func(f *Food) { f.Name = "Egg\u200b" }, "control or invisible"},
		{"a line break in the name", func(f *Food) { f.Name = "Egg\nwhite" }, "control or invisible"},
		{"a name too long", func(f *Food) { f.Name = strings.Repeat("ž", MaxNameLen+1) }, "at most 200"},
		{"a brand of spaces", func(f *Food) { f.Brand = "  " }, "brand is only space"},
		{"not UTF-8", func(f *Food) { f.Note = "\xff" }, "not valid UTF-8"},
		{"an unknown source", func(f *Food) { f.Source = "off" }, "source"},
		{"an unknown basis", func(f *Food) { f.Basis = "cup" }, "basis"},
		{"negative protein", func(f *Food) { f.Nutrients.Protein = -1 }, "protein -1"},
		{"kcal not a number", func(f *Food) { f.Nutrients.Kcal = math.NaN() }, "kcal NaN"},
		{"more energy than fat", func(f *Food) { f.Nutrients.Kcal = 950 }, "at most 900"},
		{"more than 100 g in 100 g", func(f *Food) { f.Nutrients.Protein, f.Nutrients.Fat = 60, 50 }, "add up to"},
		{"negative fiber", func(f *Food) { f.Nutrients.Fiber = new(-2.0) }, "fiber -2"},
		{"salt over 100 g", func(f *Food) { f.Nutrients.Salt = new(120.0) }, "salt 120 g per 100 g"},
		{"more than 150 g in 100 ml", func(f *Food) {
			f.Basis, f.Nutrients.Kcal, f.Nutrients.Protein, f.Nutrients.Carbs, f.Nutrients.Fat = Per100ml, 600, 0, 151, 0
		}, "add up to 151 g per 100 ml: want at most 150"},
		{"sugars over 150 g in 100 ml", func(f *Food) { f.Basis, f.Nutrients.Sugars = Per100ml, new(151.0) }, "sugars 151"},
		{"portion grams on a food by weight", func(f *Food) { f.PortionGrams = 50 }, "portion grams belong"},
		{"a serving without a name", func(f *Food) { f.Servings = []Serving{{Amount: 10}} }, "serving name is required"},
		{"a serving of nothing", func(f *Food) { f.Servings = []Serving{{Name: "crumb", Amount: 0}} }, "amount 0"},
		{"a serving named twice", func(f *Food) {
			f.Servings = []Serving{{Name: "piece", Amount: 60}, {Name: "Piece", Amount: 50}}
		}, "named twice"},
		{"too many servings", func(f *Food) {
			for i := range MaxServings + 1 {
				f.Servings = append(f.Servings, Serving{Name: strings.Repeat("s", i+1), Amount: 1})
			}
		}, "at most 20"},
		{"servings on a portion", func(f *Food) {
			f.Basis, f.Servings = PerPortion, []Serving{{Name: "piece", Amount: 1}}
		}, "no named servings"},
		{"negative portion grams", func(f *Food) { f.Basis, f.Servings, f.PortionGrams = PerPortion, nil, -5 }, "portion grams -5"},
		{"a portion too big", func(f *Food) {
			f.Basis, f.Servings, f.Nutrients.Kcal = PerPortion, nil, MaxKcalPerPortion+1
		}, "10001 kcal in a portion"},
		{"a portion with more than 100 g in 100 g", func(f *Food) {
			f.Basis, f.Servings, f.PortionGrams, f.Nutrients = PerPortion, nil, 100, Nutrients{Kcal: 480, Carbs: 120}
		}, "add up to 120 g per 100 g"},
		{"a portion denser than fat", func(f *Food) {
			f.Basis, f.Servings, f.PortionGrams, f.Nutrients.Kcal = PerPortion, nil, 10, 500
		}, "at most 900"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := egg()
			tc.change(&f)
			if err := f.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func TestServing(t *testing.T) {
	t.Parallel()

	f := egg()
	if s, ok := f.Serving(" PIECE "); !ok || s.Amount != 60 {
		t.Errorf("Serving(PIECE) = %v, %v; want the piece", s, ok)
	}
	if _, ok := f.Serving("slice"); ok {
		t.Error("found a serving the food does not have")
	}
}

func TestBasisUnit(t *testing.T) {
	t.Parallel()

	for b, want := range map[Basis]string{Per100g: "g", Per100ml: "ml", PerPortion: "portion"} {
		if got := b.Unit(); got != want {
			t.Errorf("%s.Unit() = %s, want %s", b, got, want)
		}
	}
}

func TestScaleAndAdd(t *testing.T) {
	t.Parallel()

	n := Nutrients{Kcal: 100, Protein: 10, Carbs: 20, Fat: 1, Fiber: new(4.0)}
	half := n.Scale(0.5)
	if half.Kcal != 50 || half.Protein != 5 || half.Carbs != 10 || half.Fat != 0.5 || *half.Fiber != 2 || half.Sugars != nil {
		t.Errorf("Scale(0.5) = %+v", half)
	}
	if *n.Fiber != 4 {
		t.Error("Scale changed the original")
	}
	sum := half.Add(Nutrients{Kcal: 10, Sugars: new(3.0)})
	if sum.Kcal != 60 || *sum.Fiber != 2 || *sum.Sugars != 3 || sum.Salt != nil {
		t.Errorf("Add = %+v", sum)
	}
}

func TestAtwaterRange(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		n           Nutrients
		least, most float64
	}{
		{"macros", Nutrients{Protein: 10, Carbs: 20, Fat: 5}, 165, 165},
		{"with alcohol (wine)", Nutrients{Carbs: 2.6, Alcohol: new(10.0)}, 80.4, 80.4},
		// EU: 40 + 10; left out: 40; US, fiber inside the carbs: 40 - 20.
		{"with fiber", Nutrients{Carbs: 10, Fiber: new(5.0)}, 20, 50},
		// As carbs: 80; EU, at 2.4: 64; erythritol: 40.
		{"with polyols in the carbs", Nutrients{Carbs: 20, Polyols: new(10.0)}, 40, 80},
		{"never below zero", Nutrients{Carbs: 1, Fiber: new(5.0)}, 0, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			least, most := tc.n.AtwaterRange()
			if math.Abs(least-tc.least) > 1e-9 || math.Abs(most-tc.most) > 1e-9 {
				t.Errorf("AtwaterRange = %v, %v, want %v, %v", least, most, tc.least, tc.most)
			}
		})
	}
}

func TestRounded(t *testing.T) {
	t.Parallel()

	r := Nutrients{Kcal: 199.375, Protein: 15.95, Carbs: 17.05, Fat: 9.075, Salt: new(0.649), Sugars: new(0.649)}.Rounded()
	if r.Kcal != 199 || r.Protein != 16 || r.Carbs != 17.1 || r.Fat != 9.1 || *r.Salt != 0.65 || *r.Sugars != 0.6 || r.Fiber != nil {
		t.Errorf("Rounded = %+v", r)
	}
}
