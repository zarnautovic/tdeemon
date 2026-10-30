// Package recipe works out a recipe: a dish cooked of the user's foods in
// their amounts, which is a food of its own (food.Recipe), its values per
// 100 g of the cooked dish. A share of it is logged as any food's is: its
// grams, or servings of the whole dish (Whole) or of a portion (Portion),
// so a tenth of the pot is 0.1 of Whole.
//
// A recipe keeps its ingredients as items, each with the values it
// brought when the recipe was saved, as a meal's items do: correcting a
// food later does not change the recipe until it is saved again.
package recipe

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// Limits on a recipe.
const (
	// MaxItems is the most ingredients of a recipe.
	MaxItems = 50
	// MaxPortions is the most portions a recipe is divided into.
	MaxPortions = 100
	// MaxWeight is the most grams the cooked dish, its raw ingredients
	// or the empty pot may weigh: a serving's most (food.Serving).
	MaxWeight = 10000
)

// The servings of a recipe's food: the whole dish, and one portion of it
// when the recipe is divided into portions.
const (
	Whole   = "recipe"
	Portion = "portion"
)

// The cooked dish may weigh from LowYield to HighYield times its raw
// ingredients before Warnings says so: water boils off and rice takes it
// up, but a tenth or ten times is more likely a typo.
const (
	LowYield  = 0.3
	HighYield = 3.0
)

// Warning codes.
const (
	WarnYieldLow  = "yield_low"
	WarnYieldHigh = "yield_high"
)

// Recipe is a dish of the user's, as stored.
type Recipe struct {
	// Food is the recipe as a food of the user's. Its id, name, aliases,
	// note and whether it is archived are the recipe's; Make works out
	// the rest. A recipe has no brand.
	Food food.Food
	// Items are the ingredients, in order, each an item of a food of the
	// user's with the values it brought when the recipe was saved.
	Items []meal.Item
	// Yield is the weight of the cooked dish in grams, or 0 when it was
	// not weighed: the raw ingredients' weight stands for it then.
	Yield float64
	// Portions is how many portions the dish is divided into, or 0.
	Portions int
	// Pot is the weight of the empty pot in grams, kept for the next
	// weighing of the dish in it, or 0.
	Pot float64
}

// Validate reports every field that breaks the rules, joined into one
// error. It checks what the client gives; Make checks the food it works
// out.
func (r Recipe) Validate() error {
	var errs []error
	if r.Food.Brand != "" {
		errs = append(errs, errors.New("a recipe has no brand"))
	}
	switch {
	case len(r.Items) == 0:
		errs = append(errs, errors.New("a recipe needs at least one ingredient"))
	case len(r.Items) > MaxItems:
		errs = append(errs, fmt.Errorf("%d ingredients, want at most %d", len(r.Items), MaxItems))
	}
	for i := range r.Items {
		switch it := &r.Items[i]; {
		case it.FoodID == uuid.Nil():
			errs = append(errs, fmt.Errorf("ingredient %q: an ingredient is a food, not an estimate", it.Name))
		case it.FoodID == r.Food.ID:
			errs = append(errs, fmt.Errorf("ingredient %q: a recipe cannot be its own ingredient", it.Name))
		}
	}
	errs = append(errs, checkWeight("yield", r.Yield), checkWeight("pot", r.Pot))
	if r.Portions < 0 || r.Portions > MaxPortions {
		errs = append(errs, fmt.Errorf("%d portions: want 0 (not divided) to %d", r.Portions, MaxPortions))
	}
	return errors.Join(errs...)
}

func checkWeight(what string, g float64) error {
	if math.IsNaN(g) || math.IsInf(g, 0) || g < 0 || g > MaxWeight {
		return fmt.Errorf("%s %v g: want 0 (not known) to %d", what, g, MaxWeight)
	}
	return nil
}

// Total returns the nutrients of the whole dish: the sum of its
// ingredients'. An optional nutrient is known only when every ingredient
// states it, as a sum of some would pass for all of it.
func (r Recipe) Total() food.Nutrients {
	var n food.Nutrients
	known := [6]bool{true, true, true, true, true, true}
	for i := range r.Items {
		m := r.Items[i].Nutrients
		n = n.Add(m)
		for j, p := range optionals(&m) {
			known[j] = known[j] && *p != nil
		}
	}
	for j, p := range optionals(&n) {
		if !known[j] {
			*p = nil
		}
	}
	return n
}

// optionals lists n's optional nutrients.
func optionals(n *food.Nutrients) [6]**float64 {
	return [6]**float64{&n.Fiber, &n.SaturatedFat, &n.Sugars, &n.Salt, &n.Polyols, &n.Alcohol}
}

// RawWeight returns the weight of the raw ingredients in grams, and
// whether it is known: an amount in g, or in ml taken as g, and a food
// per portion by the grams it was given in. An ingredient of portions
// alone has no weight to add.
func (r Recipe) RawWeight() (float64, bool) {
	var g float64
	for i := range r.Items {
		it := &r.Items[i]
		switch {
		case it.Unit == "g" || it.Unit == "ml":
			g += it.Amount
		case it.Quantity.Unit == "g":
			g += it.Quantity.Amount
		default:
			return 0, false
		}
	}
	return g, true
}

// Weight returns the weight of the cooked dish: the yield, or the raw
// ingredients' when it was not weighed.
func (r Recipe) Weight() (float64, error) {
	if r.Yield > 0 {
		return r.Yield, nil
	}
	g, ok := r.RawWeight()
	switch {
	case !ok:
		return 0, errors.New("an ingredient is in portions of no known weight: weigh the cooked dish, or give that ingredient in grams")
	case g > MaxWeight:
		return 0, fmt.Errorf("the raw ingredients weigh %v g: want at most %d; weigh the cooked dish", g, MaxWeight)
	}
	return g, nil
}

// Make returns r with its food worked out of its ingredients: by weight,
// its nutrients per 100 g of the cooked dish, and its servings the whole
// dish (Whole) and a portion (Portion) when it is divided. It refuses a
// recipe that breaks the rules (Validate), and one whose values per 100 g
// cannot be, as a food's are checked.
func Make(r Recipe) (Recipe, error) {
	if err := r.Validate(); err != nil {
		return Recipe{}, err
	}
	w, err := r.Weight()
	if err != nil {
		return Recipe{}, err
	}
	f := r.Food
	f.Basis, f.Source, f.PortionGrams = food.Per100g, food.Recipe, 0
	f.Nutrients = r.Total().Scale(100 / w)
	f.Servings = []food.Serving{{Name: Whole, Amount: w}}
	if r.Portions > 0 {
		f.Servings = append(f.Servings, food.Serving{Name: Portion, Amount: w / float64(r.Portions)})
	}
	if err := f.Validate(); err != nil {
		return Recipe{}, fmt.Errorf("the dish of %v g: %w", w, err)
	}
	r.Food = f
	return r, nil
}

// Warnings returns what the client should look at in r, which is stored
// anyway: a cooked dish that weighs far less or more than its raw
// ingredients (LowYield, HighYield).
func (r Recipe) Warnings() []meal.Warning {
	raw, ok := r.RawWeight()
	if r.Yield == 0 || !ok || raw == 0 {
		return nil
	}
	share := r.Yield / raw
	switch {
	case share < LowYield:
		return []meal.Warning{{Code: WarnYieldLow, Message: fmt.Sprintf(
			"the cooked dish (%s g) weighs %s %% of its raw ingredients (%s g): is the weight right?",
			grams(r.Yield), grams(share*100), grams(raw))}}
	case share > HighYield:
		return []meal.Warning{{Code: WarnYieldHigh, Message: fmt.Sprintf(
			"the cooked dish (%s g) weighs %s times its raw ingredients (%s g): is the weight right, or is an ingredient missing?",
			grams(r.Yield), strconv.FormatFloat(math.Round(share*10)/10, 'f', -1, 64), grams(raw))}}
	}
	return nil
}

func grams(g float64) string { return strconv.FormatFloat(math.Round(g), 'f', -1, 64) }
