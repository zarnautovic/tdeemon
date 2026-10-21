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
	"unicode/utf8"
	"uuid"
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
	// MaxKcalPer100 is pure fat: nothing has more energy per 100 g.
	MaxKcalPer100 = 900
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
	Source   Source
	Note     string
	Archived bool
}

// Serving is a named amount of a food, in the food's unit: "piece", 60.
type Serving struct {
	Name   string
	Amount float64
}

// Serving returns the serving called name, ignoring letter case.
func (f Food) Serving(name string) (Serving, bool) {
	for _, s := range f.Servings {
		if strings.EqualFold(s.Name, strings.TrimSpace(name)) {
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
	errs = append(errs, validateText("name", f.Name, MaxNameLen, true), validateText("brand", f.Brand, MaxBrandLen, false),
		validateText("note", f.Note, MaxNoteLen, false))
	if !validSource(f.Source) {
		errs = append(errs, fmt.Errorf("source %q: want one of label, estimate, user, dish", f.Source))
	}
	switch f.Basis {
	case Per100g, Per100ml:
		errs = append(errs, f.Nutrients.ValidatePer100())
		if f.PortionGrams != 0 {
			errs = append(errs, errors.New("portion grams belong to a food per portion"))
		}
		errs = append(errs, validateServings(f.Servings))
	case PerPortion:
		errs = append(errs, f.Nutrients.Validate())
		if !finite(f.PortionGrams) || f.PortionGrams < 0 {
			errs = append(errs, fmt.Errorf("portion grams %v: want zero (unknown) or more", f.PortionGrams))
		} else if f.PortionGrams > 0 {
			// Known weight: the values per 100 g must make sense too.
			errs = append(errs, f.Nutrients.Scale(100/f.PortionGrams).ValidatePer100())
		}
		if len(f.Servings) > 0 {
			errs = append(errs, errors.New("a food per portion has no named servings; log a share of the portion"))
		}
	default:
		errs = append(errs, fmt.Errorf("basis %q: want 100g, 100ml or portion", f.Basis))
	}
	return errors.Join(errs...)
}

func validSource(s Source) bool { return slices.Contains(Sources(), s) }

func validateServings(ss []Serving) error {
	if len(ss) > MaxServings {
		return fmt.Errorf("%d servings, want at most %d", len(ss), MaxServings)
	}
	var errs []error
	seen := map[string]bool{}
	for _, s := range ss {
		if err := validateText("serving name", s.Name, 50, true); err != nil {
			errs = append(errs, err)
			continue
		}
		key := strings.ToLower(strings.TrimSpace(s.Name))
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

// validateText checks a text field: valid UTF-8, no control characters, no
// space around it, at most max characters, and present when required.
func validateText(field, s string, maxLen int, required bool) error {
	switch {
	case !utf8.ValidString(s):
		return fmt.Errorf("%s is not valid UTF-8", field)
	case strings.TrimSpace(s) == "":
		if required {
			return fmt.Errorf("%s is required", field)
		}
		if s != "" {
			return fmt.Errorf("%s is only space", field)
		}
	case s != strings.TrimSpace(s):
		return fmt.Errorf("%s %q: has space at the start or end", field, s)
	case strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 && r != '\n' || r == 0x7f }):
		return fmt.Errorf("%s contains a control character", field)
	case utf8.RuneCountInString(s) > maxLen:
		return fmt.Errorf("%s is %d characters long, want at most %d", field, utf8.RuneCountInString(s), maxLen)
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
