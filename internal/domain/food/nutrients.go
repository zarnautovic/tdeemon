package food

import (
	"errors"
	"fmt"
	"math"
)

// Nutrients are energy (kcal) and grams of each macronutrient in some
// amount of food. The optional ones are nil when unknown: nobody has to
// enter them, and what uses them uses them only when they are there.
type Nutrients struct {
	Kcal    float64
	Protein float64
	Carbs   float64
	Fat     float64

	Fiber        *float64
	SaturatedFat *float64
	Sugars       *float64
	Salt         *float64
	// Polyols (sugar alcohols) are part of Carbs on an EU label, at 2.4
	// kcal/g instead of 4.
	Polyols *float64
	// Alcohol, in grams (7 kcal/g), so a drink's energy adds up.
	Alcohol *float64
}

// optional lists the optional nutrients of n with their names, for code
// that treats them alike.
func optional(n *Nutrients) []struct {
	name string
	v    **float64
} {
	return []struct {
		name string
		v    **float64
	}{
		{"fiber", &n.Fiber},
		{"saturated fat", &n.SaturatedFat},
		{"sugars", &n.Sugars},
		{"salt", &n.Salt},
		{"polyols", &n.Polyols},
		{"alcohol", &n.Alcohol},
	}
}

// Scale returns n for f times the amount: 1.5 servings, 250/100 g.
// Optional nutrients that are unknown stay unknown.
func (n Nutrients) Scale(f float64) Nutrients {
	out := Nutrients{Kcal: n.Kcal * f, Protein: n.Protein * f, Carbs: n.Carbs * f, Fat: n.Fat * f}
	src, dst := optional(&n), optional(&out)
	for i := range src {
		if v := *src[i].v; v != nil {
			s := *v * f
			*dst[i].v = &s
		}
	}
	return out
}

// Add returns the sum of n and m. An optional nutrient is the sum of the
// amounts known; it stays unknown only when neither knows it, so a total
// can be a floor ("at least 12 g of fiber").
func (n Nutrients) Add(m Nutrients) Nutrients {
	out := Nutrients{Kcal: n.Kcal + m.Kcal, Protein: n.Protein + m.Protein, Carbs: n.Carbs + m.Carbs, Fat: n.Fat + m.Fat}
	a, b, dst := optional(&n), optional(&m), optional(&out)
	for i := range a {
		x, y := *a[i].v, *b[i].v
		if x == nil && y == nil {
			continue
		}
		var s float64
		if x != nil {
			s += *x
		}
		if y != nil {
			s += *y
		}
		*dst[i].v = &s
	}
	return out
}

// Validate reports nutrients that cannot be: negative or not a number.
func (n Nutrients) Validate() error {
	var errs []error
	for _, c := range []struct {
		name string
		v    float64
	}{{"kcal", n.Kcal}, {"protein", n.Protein}, {"carbs", n.Carbs}, {"fat", n.Fat}} {
		if !finite(c.v) || c.v < 0 {
			errs = append(errs, fmt.Errorf("%s %v: want a number, zero or more", c.name, c.v))
		}
	}
	for _, o := range optional(&n) {
		if v := *o.v; v != nil && (!finite(*v) || *v < 0) {
			errs = append(errs, fmt.Errorf("%s %v: want a number, zero or more", o.name, *v))
		}
	}
	return errors.Join(errs...)
}

// ValidatePer100 checks nutrients per 100 g or 100 ml: Validate, at most
// MaxKcalPer100 kcal, and no more than 100 g of protein, carbs, fat and
// alcohol together (a little over for rounding on labels).
func (n Nutrients) ValidatePer100() error {
	if err := n.Validate(); err != nil {
		return err
	}
	var errs []error
	if n.Kcal > MaxKcalPer100 {
		errs = append(errs, fmt.Errorf("%v kcal per 100: want at most %d, the energy of pure fat", round1(n.Kcal), MaxKcalPer100))
	}
	mass := n.Protein + n.Carbs + n.Fat + value(n.Alcohol)
	if mass > 101 {
		errs = append(errs, fmt.Errorf("protein, carbs, fat and alcohol add up to %v g per 100: want at most 100", round1(mass)))
	}
	for _, o := range optional(&n) {
		if v := *o.v; v != nil && *v > 100 {
			errs = append(errs, fmt.Errorf("%s %v g per 100: want at most 100", o.name, round1(*v)))
		}
	}
	return errors.Join(errs...)
}

// Kcal from each gram, the Atwater factors (Regulation (EU) No 1169/2011,
// Annex XIV).
const (
	kcalPerGramProtein = 4
	kcalPerGramCarbs   = 4
	kcalPerGramFat     = 9
	kcalPerGramAlcohol = 7
	kcalPerGramFiber   = 2
	kcalPerGramPolyols = 2.4
)

// AtwaterKcal returns the energy the macronutrients account for: 4 kcal
// per gram of protein and carbs, 9 per gram of fat, 7 of alcohol, 2 of
// fiber, and polyols, counted in carbs, at 2.4 instead of 4.
func (n Nutrients) AtwaterKcal() float64 {
	k := kcalPerGramProtein*n.Protein + kcalPerGramCarbs*n.Carbs + kcalPerGramFat*n.Fat +
		kcalPerGramAlcohol*value(n.Alcohol) + kcalPerGramFiber*value(n.Fiber)
	return k - (kcalPerGramCarbs-kcalPerGramPolyols)*value(n.Polyols)
}

// Rounded returns n as it is shown: kcal to a whole number, grams to one
// decimal. Stored values keep their precision, so sums stay exact.
func (n Nutrients) Rounded() Nutrients {
	out := Nutrients{Kcal: math.Round(n.Kcal), Protein: round1(n.Protein), Carbs: round1(n.Carbs), Fat: round1(n.Fat)}
	src, dst := optional(&n), optional(&out)
	for i := range src {
		if v := *src[i].v; v != nil {
			r := round1(*v)
			*dst[i].v = &r
		}
	}
	return out
}

func value(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
