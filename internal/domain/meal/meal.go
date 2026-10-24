// Package meal computes what a meal holds: each item's nutrients from its
// food and quantity, kept as a snapshot, the meal's and the day's totals,
// and warnings about values that do not add up.
//
// The client (a model, the web app) says what was eaten and how much; the
// code here does the arithmetic, so totals are right whoever logs.
package meal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/text"
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
	// MinYear and MaxYear bound when a meal was eaten: a log, not a typo
	// or an id's clock gone wrong; four-digit years store as text that
	// sorts as the time.
	MinYear = 1970
	MaxYear = 9999
)

// Quantity is how much of a food was eaten: an amount in grams or ml, a
// number of a named serving ("4 × piece", "0.5 × roll"), or a number of
// portions of a food per portion. Exactly one is given. A unit given must
// be the food's, so 250 ml of a food by weight is refused, not taken for
// 250 g; an amount without one is in the food's unit, g or ml (FromFood),
// and a food per portion takes its grams by name.
type Quantity struct {
	Amount float64
	// Unit is "g" or "ml", the unit of Amount; empty without an amount,
	// or for the food's own.
	Unit     string
	Servings float64
	Serving  string
	Portions float64
}

// Item is one thing eaten, as stored: what it was, how much, and the
// nutrients it brought, computed when it was logged. Editing the food
// later does not change the item.
type Item struct {
	// ID names the item within its meal, for an edit of one item; the
	// store gives one to an item stored without.
	ID uuid.UUID
	// FoodID is the food the item came from; uuid.Nil for a one-off
	// estimate.
	FoodID uuid.UUID
	// Name is the food's name (with its brand) when the item was logged,
	// or the estimate's: up to food.MaxBrandLen + 1 + food.MaxNameLen
	// characters.
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
	// SentFood is SentFood of the new food a client sent with the item,
	// when it sent one rather than a food of the user's: the item is told
	// by it, not by FoodID, so a retry is known however the food is
	// stored, renamed or corrected since.
	SentFood string
}

// FromFood returns the item for quantity q of food f.
func FromFood(f food.Food, q Quantity, note string) (Item, error) {
	if err := validateNote(note); err != nil {
		return Item{}, err
	}
	// Servings without a name are of the food's one serving, when it has
	// one only: what else they could be of. The item keeps them as given,
	// as an amount without its unit, so a retry of it is the same.
	if strings.TrimSpace(q.Serving) == "" {
		q.Serving = ""
	}
	given := q
	if q.Servings > 0 && q.Serving == "" && len(f.Servings) == 1 {
		q.Serving = f.Servings[0].Name
	}
	// An amount without its unit is in the food's, g or ml; the item
	// keeps it as given, so a retry of it is the same.
	if q.Amount > 0 && q.Unit == "" && f.Basis == food.PerPortion {
		return Item{}, fmt.Errorf("%s: a food per portion takes portions, or an amount in g when its portion's weight is known", f.Name)
	}
	amount, err := resolve(f, q.inUnitOf(f.Basis))
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
		FoodID: f.ID, Name: name, Quantity: given, Amount: amount, Unit: f.Basis.Unit(),
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
		return portionsOf(f, q)
	case f.Basis == food.PerPortion:
		return 0, errors.New("a food per portion takes portions (or grams when the portion's weight is known), not servings")
	case q.Portions > 0:
		return 0, fmt.Errorf("the food is measured in %s; give an amount or a serving", f.Basis.Unit())
	case q.Amount > 0 && q.Unit != f.Basis.Unit():
		return 0, fmt.Errorf("the food is measured in %s, not %s; give %[1]s or a serving", f.Basis.Unit(), q.Unit)
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

// portionsOf returns the portions of PerPortion food f that q's grams
// make.
func portionsOf(f food.Food, q Quantity) (float64, error) {
	switch {
	case q.Unit != "g":
		return 0, fmt.Errorf("a food per portion takes portions, or grams when the portion's weight is known; not %s", q.Unit)
	case f.PortionGrams == 0:
		return 0, errors.New("the weight of a portion is not known; give the number of portions")
	}
	p := q.Amount / f.PortionGrams
	if p > MaxPortions {
		return 0, fmt.Errorf("%v g is %v portions: want at most %d", q.Amount, p, MaxPortions)
	}
	return p, nil
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

// inUnitOf returns q with an amount's unit, when left out, b's: g or
// ml, the unit of a food of the user's. A food per portion has none to
// give.
func (q Quantity) inUnitOf(b food.Basis) Quantity {
	if q.Amount > 0 && q.Unit == "" && b != food.PerPortion {
		q.Unit = b.Unit()
	}
	return q
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
	return q.validateUnit()
}

func (q Quantity) validateUnit() error {
	switch {
	case q.Amount > 0 && q.Unit != "g" && q.Unit != "ml":
		return fmt.Errorf("an amount needs its unit, g or ml, not %q", q.Unit)
	case q.Amount == 0 && q.Unit != "":
		return errors.New("a unit needs an amount")
	}
	return nil
}

// Estimate returns a one-off item: a dish whose totals were judged as a
// whole (a restaurant plate), with no food behind it.
func Estimate(name string, n food.Nutrients, note string) (Item, error) {
	if err := text.Check("name", name, MaxNameLen, true); err != nil {
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
	// ClientAPI is the API with a token: a script's, or an agent's
	// before MCP names itself.
	ClientAPI Client = "api"
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
	switch y := m.EatenAt.UTC().Year(); {
	case m.EatenAt.IsZero():
		errs = append(errs, errors.New("the time it was eaten is missing"))
	case y < MinYear || y > MaxYear:
		errs = append(errs, fmt.Errorf("eaten in %d: want %d to %d", y, MinYear, MaxYear))
	}
	switch {
	case m.Day.IsZero():
		errs = append(errs, errors.New("the day is missing"))
	case m.Day.Year < MinYear || m.Day.Year > MaxYear:
		errs = append(errs, fmt.Errorf("day %s: want a year from %d to %d", m.Day, MinYear, MaxYear))
	}
	switch m.Client {
	case ClientMCP, ClientChat, ClientWeb, ClientTelegram, ClientImport, ClientAPI:
	default:
		errs = append(errs, fmt.Errorf("client %q: want mcp, chat, web, telegram, import or api", m.Client))
	}
	errs = append(errs, text.Check("description", m.Description, MaxNameLen, false), validateNote(m.Note))
	switch {
	case len(m.Items) == 0:
		errs = append(errs, errors.New("a meal needs at least one item"))
	case len(m.Items) > MaxItems:
		errs = append(errs, fmt.Errorf("%d items, want at most %d", len(m.Items), MaxItems))
	}
	errs = append(errs, validateItems(m.Items)...)
	return errors.Join(errs...)
}

// validateItems reports an item id given twice and a SentFood out of form.
func validateItems(items []Item) []error {
	var errs []error
	seen := map[uuid.UUID]bool{}
	for i := range items {
		it := &items[i]
		if id := it.ID; id != uuid.Nil() && seen[id] {
			errs = append(errs, fmt.Errorf("item id %s is given twice", id))
		} else {
			seen[id] = true
		}
		if it.SentFood != "" && (it.FoodID == uuid.Nil() || !sentFoodForm.MatchString(it.SentFood)) {
			errs = append(errs, fmt.Errorf("item %d: the food sent %q: want a digest of 64 hex digits, with the food saved", i+1, it.SentFood))
		}
	}
	return errs
}

// Fingerprint returns a digest of what a client said the meal is: when
// it was eaten, its description and note, and each item's food and
// quantity, or an estimate's name and values, with its note. What the
// code works out (the day, a food's values and name) and the ids of
// items are left out, so the same meal sent again has the same
// fingerprint even after its foods changed, and the store can tell a
// retry from a different meal under the same id. The time counts to the
// second, as stored. The items count in their order: the same items in
// another order are another request. The text hashed is stored state:
// a field added later must be written only when it is set, so that the
// meals stored before keep their fingerprint. A client that leaves out when the
// meal was eaten must get the same time on a retry: the API takes it
// from the meal's UUIDv7, not from the clock.
func (m Meal) Fingerprint() string {
	// Each value quoted or in its shortest exact form, one item to a line,
	// so no two meals write the same text.
	lines := []string{fmt.Sprintf("%q %q %q %d", m.EatenAt.UTC().Truncate(time.Second).Format(time.RFC3339), m.Description, m.Note, len(m.Items))}
	for i := range m.Items {
		lines = append(lines, m.Items[i].sent())
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// sent is what a client sent for the item, as Fingerprint writes it: the
// food (the user's, by its id, or a new one, by SentFood) and the
// quantity, or an estimate's name and values, and the note.
func (it Item) sent() string {
	q := it.Quantity
	what := it.FoodID.String()
	if it.SentFood != "" {
		what = "sent " + it.SentFood
	}
	line := fmt.Sprintf("%s %v %q %v %q %v %q", what, q.Amount, q.Unit, q.Servings, q.Serving, q.Portions, it.Note)
	if it.FoodID == uuid.Nil() && it.SentFood == "" {
		line += fmt.Sprintf(" %q %s", it.Name, nutrientsText(it.Nutrients))
	}
	return line
}

// nutrientsText is n as sent writes it: each value in its shortest exact
// form, "-" for an optional one not given.
func nutrientsText(n food.Nutrients) string {
	line := []string{fmt.Sprintf("%v %v %v %v", n.Kcal, n.Protein, n.Carbs, n.Fat)}
	for _, v := range []*float64{n.Fiber, n.SaturatedFat, n.Sugars, n.Salt, n.Polyols, n.Alcohol} {
		if v == nil {
			line = append(line, "-")
		} else {
			line = append(line, fmt.Sprint(*v))
		}
	}
	return strings.Join(line, " ")
}

// sentFoodForm is the form of a SentFood digest.
var sentFoodForm = regexp.MustCompile(`^[0-9a-f]{64}$`)

// SentFood returns a digest of new food f as a client sent it, its id
// left out: every field in a fixed order, each quoted or in its shortest
// exact form, so no two foods write the same text. Its servings and
// aliases are sets: in any order, the same digest.
func SentFood(f food.Food) string {
	servings := slices.Clone(f.Servings)
	// A food names each serving once.
	slices.SortFunc(servings, func(a, b food.Serving) int { return strings.Compare(a.Name, b.Name) })
	lines := []string{
		fmt.Sprintf("%q %q %q %q %q %v", f.Name, f.Brand, f.Basis, f.Source, f.Note, f.PortionGrams),
		nutrientsText(f.Nutrients),
		fmt.Sprintf("%q", slices.Sorted(slices.Values(f.Aliases))),
	}
	for _, sv := range servings {
		lines = append(lines, fmt.Sprintf("%q %v", sv.Name, sv.Amount))
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// SameAs reports whether it and other are what a client sent alike: the
// same food (the user's, or a new one as sent) and quantity, or
// estimate, and note, whatever their ids or a food corrected since. An op
// of an item the meal has already as it was sent is a retry.
func (it Item) SameAs(other Item) bool { return it.sent() == other.sent() }

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

func validateNote(note string) error { return text.CheckLines("note", note, MaxNoteLen, false) }
