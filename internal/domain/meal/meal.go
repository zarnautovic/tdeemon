// Package meal computes what a meal holds: each item's nutrients from its
// food and quantity, kept as a snapshot, the meal's and the day's totals,
// and warnings about values that do not add up.
//
// The client (a model, the web app) says what was eaten and how much; the
// code here does the arithmetic, so totals are right whoever logs.
package meal

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

// Limits on what one item or meal may hold.
const (
	// MaxAmount is the most grams or ml of one item: 10 kg.
	MaxAmount = 10000
	// MaxServings and MaxPortions bound a count of servings or portions.
	MaxServings = 100
	MaxPortions = 50
	MaxItems    = 50
	MaxNameLen  = 200
	MaxNoteLen  = 2000
	// MaxEstimateKcal bounds a one-off estimate: a feast, not a typo.
	MaxEstimateKcal = 10000
)

// Quantity is how much of a food was eaten: an amount in the food's unit
// (grams or ml), a number of a named serving ("4 × piece", "0.5 × roll"),
// or a number of portions of a food per portion. Exactly one is given.
type Quantity struct {
	Amount   float64
	Servings float64
	Serving  string
	Portions float64
}

// Item is one thing eaten, as stored: what it was, how much, and the
// nutrients it brought, computed when it was logged. Editing the food
// later does not change the item.
type Item struct {
	// FoodID is the food the item came from; uuid.Nil for a one-off
	// estimate.
	FoodID uuid.UUID
	// Name is the food's name (with its brand) when the item was logged,
	// or the estimate's.
	Name     string
	Quantity Quantity
	// Amount is the quantity in Unit: grams, ml or portions.
	Amount float64
	Unit   string
	// Nutrients are the item's own, for Amount.
	Nutrients food.Nutrients
	// Source is the food's source, or food.Estimate for a one-off.
	Source food.Source
	Note   string
}

// FromFood returns the item for quantity q of food f.
func FromFood(f food.Food, q Quantity, note string) (Item, error) {
	if err := validateNote(note); err != nil {
		return Item{}, err
	}
	amount, err := resolve(f, q)
	if err != nil {
		return Item{}, fmt.Errorf("%s: %w", f.Name, err)
	}
	var n food.Nutrients
	if f.Basis == food.PerPortion {
		n = f.Nutrients.Scale(amount)
	} else {
		n = f.Nutrients.Scale(amount / 100)
	}
	name := f.Name
	if f.Brand != "" {
		name = f.Brand + " " + f.Name
	}
	return Item{
		FoodID: f.ID, Name: name, Quantity: q, Amount: amount, Unit: f.Basis.Unit(),
		Nutrients: n, Source: f.Source, Note: note,
	}, nil
}

// resolve returns the amount q stands for, in f's unit.
func resolve(f food.Food, q Quantity) (float64, error) {
	if err := q.validate(); err != nil {
		return 0, err
	}
	switch {
	case f.Basis == food.PerPortion && q.Portions > 0:
		return q.Portions, nil
	case f.Basis == food.PerPortion && q.Amount > 0:
		if f.PortionGrams == 0 {
			return 0, errors.New("the weight of a portion is not known; give the number of portions")
		}
		return q.Amount / f.PortionGrams, nil
	case f.Basis == food.PerPortion:
		return 0, errors.New("a food per portion takes portions (or grams when the portion's weight is known), not servings")
	case q.Portions > 0:
		return 0, fmt.Errorf("the food is measured in %s; give an amount or a serving", f.Basis.Unit())
	case q.Servings > 0:
		s, ok := f.Serving(q.Serving)
		if !ok {
			return 0, fmt.Errorf("no serving %q; the food has %s", q.Serving, servingNames(f))
		}
		amount := q.Servings * s.Amount
		if amount > MaxAmount {
			return 0, fmt.Errorf("%v %s: want at most %d", amount, f.Basis.Unit(), MaxAmount)
		}
		return amount, nil
	}
	return q.Amount, nil
}

func servingNames(f food.Food) string {
	if len(f.Servings) == 0 {
		return "none"
	}
	names := make([]string, len(f.Servings))
	for i, s := range f.Servings {
		names[i] = fmt.Sprintf("%q", s.Name)
	}
	return strings.Join(names, ", ")
}

func (q Quantity) validate() error {
	given := 0
	for _, v := range []float64{q.Amount, q.Servings, q.Portions} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return errors.New("a quantity must be a number, more than 0")
		}
		if v > 0 {
			given++
		}
	}
	switch {
	case given != 1:
		return errors.New("give exactly one of an amount, a number of servings or a number of portions")
	case q.Amount > MaxAmount:
		return fmt.Errorf("amount %v: want at most %d", q.Amount, MaxAmount)
	case q.Servings > MaxServings:
		return fmt.Errorf("%v servings: want at most %d", q.Servings, MaxServings)
	case q.Portions > MaxPortions:
		return fmt.Errorf("%v portions: want at most %d", q.Portions, MaxPortions)
	case q.Servings > 0 && strings.TrimSpace(q.Serving) == "":
		return errors.New("a number of servings needs the serving's name")
	case q.Servings == 0 && q.Serving != "":
		return errors.New("a serving's name needs a number of servings")
	}
	return nil
}

// Estimate returns a one-off item: a dish whose totals were judged as a
// whole (a restaurant plate), with no food behind it.
func Estimate(name string, n food.Nutrients, note string) (Item, error) {
	if err := validateText("name", name, MaxNameLen, true); err != nil {
		return Item{}, err
	}
	if err := validateNote(note); err != nil {
		return Item{}, err
	}
	if err := n.Validate(); err != nil {
		return Item{}, fmt.Errorf("%s: %w", name, err)
	}
	if n.Kcal > MaxEstimateKcal {
		return Item{}, fmt.Errorf("%s: %v kcal, want at most %d", name, n.Kcal, MaxEstimateKcal)
	}
	return Item{
		Name: name, Quantity: Quantity{Portions: 1}, Amount: 1, Unit: "portion",
		Nutrients: n, Source: food.Estimate, Note: note,
	}, nil
}

// Client is what a meal was logged through.
type Client string

// Clients.
const (
	ClientMCP      Client = "mcp"
	ClientChat     Client = "chat"
	ClientWeb      Client = "web"
	ClientTelegram Client = "telegram"
	ClientImport   Client = "import"
)

// Meal is a meal as stored.
type Meal struct {
	ID      uuid.UUID
	EatenAt time.Time
	// Day is the logical day the meal belongs to (day.Calendar.DayOf).
	Day         day.Date
	Description string
	Client      Client
	Items       []Item
	Note        string
}

// Validate reports every field that breaks the rules, joined into one
// error.
func (m Meal) Validate() error {
	var errs []error
	if m.ID == uuid.Nil() {
		errs = append(errs, errors.New("id is missing"))
	}
	if m.EatenAt.IsZero() {
		errs = append(errs, errors.New("the time it was eaten is missing"))
	}
	if m.Day.IsZero() {
		errs = append(errs, errors.New("the day is missing"))
	}
	switch m.Client {
	case ClientMCP, ClientChat, ClientWeb, ClientTelegram, ClientImport:
	default:
		errs = append(errs, fmt.Errorf("client %q: want mcp, chat, web, telegram or import", m.Client))
	}
	errs = append(errs, validateText("description", m.Description, MaxNameLen, false), validateNote(m.Note))
	switch {
	case len(m.Items) == 0:
		errs = append(errs, errors.New("a meal needs at least one item"))
	case len(m.Items) > MaxItems:
		errs = append(errs, fmt.Errorf("%d items, want at most %d", len(m.Items), MaxItems))
	}
	return errors.Join(errs...)
}

// Total returns the nutrients of every item of m.
func (m Meal) Total() food.Nutrients {
	var t food.Nutrients
	for i := range m.Items {
		t = t.Add(m.Items[i].Nutrients)
	}
	return t
}

// DayTotal returns the nutrients of every meal: a day's intake.
func DayTotal(meals []Meal) food.Nutrients {
	var t food.Nutrients
	for i := range meals {
		t = t.Add(meals[i].Total())
	}
	return t
}

func validateNote(note string) error { return validateText("note", note, MaxNoteLen, false) }

// validateText checks a text field: valid UTF-8, no control characters
// but line breaks, no space around it, at most maxLen characters, and
// present when required.
func validateText(field, s string, maxLen int, required bool) error {
	switch {
	case !utf8.ValidString(s):
		return fmt.Errorf("%s is not valid UTF-8", field)
	case strings.TrimSpace(s) == "" && (required || s != ""):
		return fmt.Errorf("%s is required", field)
	case s != strings.TrimSpace(s):
		return fmt.Errorf("%s has space at the start or end", field)
	case strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 && r != '\n' || r == 0x7f }):
		return fmt.Errorf("%s contains a control character", field)
	case utf8.RuneCountInString(s) > maxLen:
		return fmt.Errorf("%s is %d characters long, want at most %d", field, utf8.RuneCountInString(s), maxLen)
	}
	return nil
}
