package meal

import (
	"fmt"
	"math"
	"time"

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

// AtwaterTolerance is how far the stated kcal may be from what the
// macronutrients account for before CheckNutrients warns: labels round,
// and estimates are estimates. Below AtwaterMinKcal of difference it does
// not warn at all: a few kcal of a cucumber are no news.
const (
	AtwaterTolerance = 0.15
	AtwaterMinKcal   = 10
)

// CheckNutrients returns warnings about values a client stated (a new
// food from a label, a one-off estimate) that do not add up: kcal far
// from what the macronutrients give (the Atwater factors), saturated fat
// above fat, sugars or polyols above carbs. what names the values in the
// messages.
func CheckNutrients(what string, n food.Nutrients) []Warning {
	var ws []Warning
	if exp := n.AtwaterKcal(); math.Abs(n.Kcal-exp) > AtwaterMinKcal && math.Abs(n.Kcal-exp) > AtwaterTolerance*math.Max(n.Kcal, exp) {
		ws = append(ws, Warning{WarnAtwater, fmt.Sprintf(
			"%s: %v kcal, but the protein, carbs and fat give %v kcal; check the kcal or the grams (drinks: give the alcohol)",
			what, math.Round(n.Kcal), math.Round(exp))})
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

// DuplicateWindow is how close in time two meals with the same items must
// be for DuplicateOf to take the second for the first sent twice.
const DuplicateWindow = 10 * time.Minute

// DuplicateOf returns a warning when m looks like a meal in recent sent
// again: eaten within DuplicateWindow of it, with the same items in the
// same amounts. A client that retries without the meal's ID, or a model
// that logs twice, would otherwise double the day.
func DuplicateOf(m Meal, recent []Meal) (Warning, bool) {
	for i := range recent {
		r := &recent[i]
		if r.ID == m.ID || absDuration(r.EatenAt.Sub(m.EatenAt)) > DuplicateWindow || !sameItems(r.Items, m.Items) {
			continue
		}
		return Warning{WarnDuplicateMeal, fmt.Sprintf(
			"a meal with the same items was logged at %s; if this one was sent twice, delete it",
			r.EatenAt.Format("15:04"))}, true
	}
	return Warning{}, false
}

// sameItems reports whether a and b hold the same foods (or estimates of
// the same name) in amounts within 1 % of each other, in any order.
func sameItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
next:
	for i := range a {
		x := &a[i]
		for j := range b {
			y := &b[j]
			if !used[j] && x.FoodID == y.FoodID && x.Name == y.Name && x.Unit == y.Unit &&
				math.Abs(x.Amount-y.Amount) <= 0.01*math.Max(x.Amount, y.Amount) {
				used[j] = true
				continue next
			}
		}
		return false
	}
	return true
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
