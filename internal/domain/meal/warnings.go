package meal

import (
	"fmt"
	"math"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

// Warning is something about a logged meal the client should look at, and
// usually tell the user about. The meal is stored anyway: a warning is a
// question, not a refusal.
type Warning struct {
	// Code names the kind of warning, for code that acts on it.
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Warning codes.
const (
	WarnAtwater       = "atwater"
	WarnSaturatedFat  = "saturated_fat"
	WarnSugars        = "sugars"
	WarnPolyols       = "polyols"
	WarnDuplicateMeal = "duplicate_meal"
)

// AtwaterTolerance is how far, as a share of the stated kcal, the kcal may
// be from what the macronutrients account for before CheckNutrients
// warns: labels round, and estimates are estimates. Below AtwaterMinKcal
// of difference it does not warn at all: a few kcal of a cucumber are no
// news.
const (
	AtwaterTolerance = 0.15
	AtwaterMinKcal   = 10
)

// CheckNutrients returns warnings about values a client stated (a new
// food from a label, a one-off estimate) that do not add up: kcal far
// from what the macronutrients give (the Atwater factors, by any of the
// conventions labels follow), saturated fat above fat, sugars or polyols
// above carbs. Values with kcal alone, no grams, are not checked against
// the grams. what names the values in the messages.
func CheckNutrients(what string, n food.Nutrients) []Warning {
	var ws []Warning
	if w, ok := checkAtwater(what, n); ok {
		ws = append(ws, w)
	}
	if n.SaturatedFat != nil && *n.SaturatedFat > n.Fat+0.05 {
		ws = append(ws, Warning{WarnSaturatedFat, fmt.Sprintf("%s: saturated fat (%v g) is more than all the fat (%v g)",
			what, round1(*n.SaturatedFat), round1(n.Fat))})
	}
	if n.Sugars != nil && *n.Sugars > n.Carbs+0.05 {
		ws = append(ws, Warning{WarnSugars, fmt.Sprintf("%s: sugars (%v g) are more than all the carbs (%v g)",
			what, round1(*n.Sugars), round1(n.Carbs))})
	}
	if n.Polyols != nil && *n.Polyols > n.Carbs+0.05 {
		ws = append(ws, Warning{WarnPolyols, fmt.Sprintf("%s: polyols (%v g) are more than all the carbs (%v g); on an EU label they are part of the carbs",
			what, round1(*n.Polyols), round1(n.Carbs))})
	}
	return ws
}

func checkAtwater(what string, n food.Nutrients) (Warning, bool) {
	if n.Protein == 0 && n.Carbs == 0 && n.Fat == 0 && (n.Alcohol == nil || *n.Alcohol == 0) {
		return Warning{}, false
	}
	least, most := n.AtwaterRange()
	off := max(least-n.Kcal, n.Kcal-most, 0)
	if off <= AtwaterMinKcal || off <= AtwaterTolerance*n.Kcal {
		return Warning{}, false
	}
	give := fmt.Sprintf("%v kcal", math.Round(least))
	if math.Round(least) != math.Round(most) {
		give = fmt.Sprintf("%v to %v kcal", math.Round(least), math.Round(most))
	}
	return Warning{WarnAtwater, fmt.Sprintf(
		"%s: %v kcal, but the protein, carbs and fat give %s; check the kcal or the grams (drinks: give the alcohol)",
		what, math.Round(n.Kcal), give)}, true
}

// DuplicateWindow is how close in time two meals with the same items must
// be for DuplicateOf to take the second for the first sent twice.
const DuplicateWindow = 10 * time.Minute

// DuplicateOf returns a warning when m looks like a meal in recent sent
// again: eaten within DuplicateWindow of it, with the same items in the
// same amounts. A client that retries without the meal's ID, or a model
// that logs twice, would otherwise double the day. The message gives the
// earlier meal's time in loc, the user's time zone (UTC when nil).
func DuplicateOf(m Meal, recent []Meal, loc *time.Location) (Warning, bool) {
	if loc == nil {
		loc = time.UTC
	}
	for i := range recent {
		r := &recent[i]
		if r.ID == m.ID || absDuration(r.EatenAt.Sub(m.EatenAt)) > DuplicateWindow || !sameItems(r.Items, m.Items) {
			continue
		}
		return Warning{WarnDuplicateMeal, fmt.Sprintf(
			"a meal with the same items was logged at %s; if this one was sent twice, delete it",
			r.EatenAt.In(loc).Format("15:04"))}, true
	}
	return Warning{}, false
}

// Distinct returns ms grouped by their items, a meal with the same items
// as one before it, in any amounts within 1 % (as DuplicateOf tells
// them), with that one: of the latest meals first, each meal a user eats
// again once, the latest, with the times before it after it. The groups
// are in the order of their first meal, each in the order of ms.
func Distinct(ms []Meal) [][]Meal {
	var out [][]Meal
next:
	for i := range ms {
		for j := range out {
			if sameItems(out[j][0].Items, ms[i].Items) {
				out[j] = append(out[j], ms[i])
				continue next
			}
		}
		out = append(out, []Meal{ms[i]})
	}
	return out
}

// sameItems reports whether a and b hold the same foods in amounts within
// 1 % of each other, or estimates of the same name and kcal within 1 %,
// in any order. A food's item is told by the food, not by the name and
// values it brought: a meal sent again after its food was corrected is
// the same meal.
func sameItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
next:
	for i := range a {
		x := &a[i]
		for j := range b {
			if !used[j] && sameItem(x, &b[j]) {
				used[j] = true
				continue next
			}
		}
		return false
	}
	return true
}

// sameItem reports whether x and y are the same food in amounts within
// 1 % of each other, or estimates of the same name and kcal within 1 %.
func sameItem(x, y *Item) bool {
	if x.FoodID != y.FoodID || x.Unit != y.Unit || !within1(x.Amount, y.Amount) {
		return false
	}
	return x.FoodID != uuid.Nil() || (x.Name == y.Name && within1(x.Nutrients.Kcal, y.Nutrients.Kcal))
}

func within1(a, b float64) bool { return math.Abs(a-b) <= 0.01*math.Max(a, b) }

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
