// Package food defines a food in the user's food database and the
// nutrients it carries, independent of how foods are stored.
//
// A food is measured by weight or volume, with its nutrients per 100 g or
// 100 ml, so any amount eaten is a multiplication; or it is a whole
// portion (a canteen meal, a restaurant dish), with its nutrients per
// portion, eaten whole or in part. A food may name servings ("piece" is
// 60 g of egg) so a client can log "4 eggs" without guessing grams.
package food

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"uuid"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/zarnautovic/tdeemon/internal/domain/text"
)

// Basis is what a food's nutrients refer to.
type Basis string

// Bases.
const (
	Per100g    Basis = "100g"
	Per100ml   Basis = "100ml"
	PerPortion Basis = "portion"
)

// Unit returns the unit amounts of the food are in: "g", "ml" or
// "portion".
func (b Basis) Unit() string {
	switch b {
	case Per100g:
		return "g"
	case Per100ml:
		return "ml"
	case PerPortion:
	}
	return "portion"
}

// Source is where a food's values come from, which says how far to trust
// them.
type Source string

// Sources.
const (
	// Label is read off a nutrition label or a package: exact.
	Label Source = "label"
	// Estimate is a judgement: a restaurant dish, a guess from a photo.
	Estimate Source = "estimate"
	// User is what the user stated, from wherever they know it.
	User Source = "user"
	// Dish is a cooked dish worked out from its ingredients and the weight
	// of the pot, without a stored recipe.
	Dish Source = "dish"
)

// Sources lists every source.
func Sources() []Source { return []Source{Label, Estimate, User, Dish} }

// Limits on a food's fields.
const (
	MaxNameLen  = 200
	MaxBrandLen = 100
	MaxNoteLen  = 2000
	MaxServings = 20
	MaxAliases  = 20
	MaxAliasLen = 100
	// MaxKcalPer100 is pure fat: nothing has more energy per 100 g.
	MaxKcalPer100 = 900
	// MaxKcalPerPortion bounds one portion: a feast, not a typo.
	MaxKcalPerPortion = 10000
)

// Food is one entry in a user's food database.
type Food struct {
	ID    uuid.UUID
	Name  string
	Brand string
	Basis Basis
	// Nutrients are per 100 g or 100 ml, or per portion (Basis).
	Nutrients Nutrients
	// PortionGrams is the weight of one portion of a PerPortion food, when
	// known; then it can be logged in grams too. Zero when unknown.
	PortionGrams float64
	// Servings are named amounts of a food measured by weight or volume:
	// "piece" is 60 g, "slice" 40 g.
	Servings []Serving
	// Aliases are other names the user calls the food by: "my bread", "the
	// dessert from Sunday". Search finds a food by them too.
	Aliases []string
	// Barcodes are the product's barcodes, each in its canonical form
	// (Barcode), one for each size of its package: a scan of one finds the
	// food.
	Barcodes []string
	Source   Source
	Note     string
	Archived bool
}

// Serving is a named amount of a food, in the food's unit: "piece", 60.
type Serving struct {
	Name   string
	Amount float64
}

// Serving returns the serving called name, compared by Key.
func (f Food) Serving(name string) (Serving, bool) {
	for _, s := range f.Servings {
		if Key(s.Name) == Key(name) {
			return s, true
		}
	}
	return Serving{}, false
}

// Validate reports every field that breaks the rules, joined into one
// error.
func (f Food) Validate() error {
	var errs []error
	if f.ID == uuid.Nil() {
		errs = append(errs, errors.New("id is missing"))
	}
	errs = append(errs, text.Check("name", f.Name, MaxNameLen, true), text.Check("brand", f.Brand, MaxBrandLen, false),
		text.CheckLines("note", f.Note, MaxNoteLen, false))
	if !validSource(f.Source) {
		errs = append(errs, fmt.Errorf("source %q: want one of label, estimate, user, dish", f.Source))
	}
	switch f.Basis {
	case Per100g, Per100ml:
		errs = append(errs, f.Nutrients.ValidatePer100(f.Basis))
		if f.PortionGrams != 0 {
			errs = append(errs, errors.New("portion grams belong to a food per portion"))
		}
		errs = append(errs, validateServings(f.Servings))
	case PerPortion:
		errs = append(errs, f.Nutrients.Validate())
		if f.Nutrients.Kcal > MaxKcalPerPortion {
			errs = append(errs, fmt.Errorf("%v kcal in a portion: want at most %d", f.Nutrients.Kcal, MaxKcalPerPortion))
		}
		if !finite(f.PortionGrams) || f.PortionGrams < 0 {
			errs = append(errs, fmt.Errorf("portion grams %v: want zero (unknown) or more", f.PortionGrams))
		} else if f.PortionGrams > 0 {
			// Known weight: the values per 100 g must make sense too.
			errs = append(errs, f.Nutrients.Scale(100/f.PortionGrams).ValidatePer100(Per100g))
		}
		if len(f.Servings) > 0 {
			errs = append(errs, errors.New("a food per portion has no named servings; log a share of the portion"))
		}
	default:
		errs = append(errs, fmt.Errorf("basis %q: want 100g, 100ml or portion", f.Basis))
	}
	errs = append(errs, validateAliases(f.Aliases), validateBarcodes(f.Barcodes))
	return errors.Join(errs...)
}

func validateAliases(as []string) error {
	if len(as) > MaxAliases {
		return fmt.Errorf("%d aliases, want at most %d", len(as), MaxAliases)
	}
	var errs []error
	seen := map[string]bool{}
	for _, a := range as {
		if err := text.Check("alias", a, MaxAliasLen, true); err != nil {
			errs = append(errs, err)
			continue
		}
		if seen[Key(a)] {
			errs = append(errs, fmt.Errorf("alias %q is given twice", a))
		}
		seen[Key(a)] = true
	}
	return errors.Join(errs...)
}

// Key returns the form names are compared in: Unicode NFC, letter case
// folded, runs of space collapsed. "Greek  yoghurt" and "greek Yoghurt"
// are the same name; "Cevapi" and "Ćevapi" are not (search finds both).
func Key(s string) string {
	return strings.Join(strings.Fields(cases.Fold().String(norm.NFC.String(s))), " ")
}

func validSource(s Source) bool { return slices.Contains(Sources(), s) }

func validateServings(ss []Serving) error {
	if len(ss) > MaxServings {
		return fmt.Errorf("%d servings, want at most %d", len(ss), MaxServings)
	}
	var errs []error
	seen := map[string]bool{}
	for _, s := range ss {
		if err := text.Check("serving name", s.Name, 50, true); err != nil {
			errs = append(errs, err)
			continue
		}
		key := Key(s.Name)
		if seen[key] {
			errs = append(errs, fmt.Errorf("serving %q is named twice", s.Name))
		}
		seen[key] = true
		if !finite(s.Amount) || s.Amount <= 0 || s.Amount > 10000 {
			errs = append(errs, fmt.Errorf("serving %q: amount %v, want more than 0 and at most 10000", s.Name, s.Amount))
		}
	}
	return errors.Join(errs...)
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
