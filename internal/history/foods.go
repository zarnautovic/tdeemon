package history

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// FoodRow is a row of the predecessor's food table (the vault's
// Baza-hrane.md): a food and its values for the portion it names.
type FoodRow struct {
	Line      int
	Name      string
	Portion   string
	Nutrients food.Nutrients
	Note      string
}

// foodColumns is how many cells a row of the food table has: the food,
// the portion, kcal, protein, carbs, fat and a note.
const foodColumns = 7

// ReadFoods returns the rows of the food table in r, a Markdown table of
// foodColumns cells, and the lines it skipped, each with why. A row
// followed by a separator (|---|:---:|) is a header; every other row of a
// table is a food, a blank line between rows notwithstanding. A row may
// start or end with a pipe too many, as a note written by hand can; a
// value may carry its unit ("99 kcal"), be a bound ("<0.5", read as the
// bound) or a dash for none (0); kcal must be given.
func ReadFoods(r io.Reader) ([]FoodRow, []string, error) {
	var lines []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		lines = append(lines, strings.TrimSpace(sc.Text()))
	}
	if err := sc.Err(); err != nil {
		return nil, nil, fmt.Errorf("the food table: %w", err)
	}
	var rows []FoodRow
	var skipped []string
	// inFoods is whether the rows are under a header of the food table's,
	// one that names kcal; the rows of another table are no foods.
	inFoods := false
	for i, s := range lines {
		line := i + 1
		switch {
		case !strings.HasPrefix(s, "|") || separator(s):
			continue
		case i+1 < len(lines) && separator(lines[i+1]):
			cells := splitRow(s)
			inFoods = len(cells) == foodColumns && strings.Contains(strings.ToLower(cells[2]), "kcal")
			continue
		case !inFoods:
			skipped = append(skipped, fmt.Sprintf("line %d: a row of no food table", line))
			continue
		}
		row, err := foodRow(s)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("line %d, %s", line, err))
			continue
		}
		row.Line = line
		rows = append(rows, row)
	}
	return rows, skipped, nil
}

// separatorCell is a cell of the line under a table's header.
var separatorCell = regexp.MustCompile(`^\s*:?-+:?\s*$`)

func separator(s string) bool {
	if !strings.HasPrefix(s, "|") {
		return false
	}
	for _, c := range splitRow(s) {
		if !separatorCell.MatchString(c) {
			return false
		}
	}
	return true
}

// splitRow returns the cells of a table's row, trimmed: between its
// first and last pipe, or, when that gives no row of the food table's
// width, between the pipes a hand wrote too many of at either end.
func splitRow(s string) []string {
	cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(s, "|"), "|"), "|")
	if len(cells) != foodColumns {
		cells = strings.Split(strings.Trim(s, "|"), "|")
	}
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// foodRow reads a row of the table.
func foodRow(s string) (FoodRow, error) {
	cells := splitRow(s)
	if len(cells) != foodColumns {
		return FoodRow{}, fmt.Errorf("%d cells, want %d", len(cells), foodColumns)
	}
	values := make([]float64, 4)
	for i := range values {
		unit := "g"
		if i == 0 {
			unit = "kcal"
		}
		v, err := number(cells[2+i], unit)
		if err != nil {
			return FoodRow{}, fmt.Errorf("%s: %w", cells[0], err)
		}
		values[i] = v
	}
	// No kcal, a dash or nothing, is 0 only for what has nothing else
	// either, water or black coffee; else it is missing.
	if none(cells[2]) && (values[1] != 0 || values[2] != 0 || values[3] != 0) {
		return FoodRow{}, fmt.Errorf("%s: no kcal", cells[0])
	}
	return FoodRow{
		Name: cells[0], Portion: cells[1], Note: cells[6],
		Nutrients: food.Nutrients{Kcal: values[0], Protein: values[1], Carbs: values[2], Fat: values[3]},
	}, nil
}

// none reports whether a value cell gives no value: a dash or nothing.
func none(s string) bool {
	switch valueText(s) {
	case "", "—", "–", "-":
		return true
	}
	return false
}

// valueLabel is the name of a macronutrient a hand may write before its
// value, as the table's header does: "P 25", "UH —".
var valueLabel = regexp.MustCompile(`^(?:P|UH|M)\s+`)

// valueText returns a value cell without its spaces and a label before.
func valueText(s string) string {
	return valueLabel.ReplaceAllString(strings.TrimSpace(s), "")
}

// valueCell is a value: a number with a decimal comma or point, after a
// bound or an approximation sign, and a unit if any.
var valueCell = regexp.MustCompile(`^[<≤~≈]?\s*(\d+(?:[.,]\d+)?)\s*([a-z]*)$`)

// number reads a value cell in unit, which it may name; a dash or nothing
// is 0. Anything else after the number, a range ("350-400") or another
// unit, is no value.
func number(s, unit string) (float64, error) {
	if none(s) {
		return 0, nil
	}
	m := valueCell.FindStringSubmatch(valueText(s))
	if len(m) != 3 || m[2] != "" && m[2] != unit {
		return 0, fmt.Errorf("value %q: want a number of %s", s, unit)
	}
	return strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
}

// portionAmount reads a portion given by its weight or volume, "220 g",
// "~400 g", "330 ml", as its amount and the food's basis.
var portionAmount = regexp.MustCompile(`^~?\s*(\d+(?:[.,]\d+)?)\s*(g|ml)$`)

// ServingName names the portion a row gave by weight or volume, on a food
// stored per 100 g or ml.
const ServingName = "portion"

// toFood returns row as a food: per 100 g or 100 ml, with the row's portion
// as a serving, when the portion is a weight or a volume and the values
// per 100 make sense; else per portion, the portion in its note. Its
// source is a label when its note names a declaration, else an estimate.
func toFood(row FoodRow, name string) (food.Food, error) {
	f := food.Food{ID: uuid.NewV7(), Name: name, Source: food.Estimate, Note: cutNote(row.Note)}
	if note := strings.ToLower(row.Note); strings.Contains(note, "deklaracij") && !strings.Contains(note, "bez deklaracij") {
		f.Source = food.Label
	}
	if m := portionAmount.FindStringSubmatch(row.Portion); m != nil {
		amount, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		basis := food.Per100g
		if m[2] == "ml" {
			basis = food.Per100ml
		}
		// Per 100 when the food per 100 makes sense, its values and its
		// serving; else per portion.
		if amount > 0 {
			per100 := f
			per100.Basis, per100.Nutrients = basis, row.Nutrients.Scale(100/amount)
			if amount != 100 {
				per100.Servings = []food.Serving{{Name: ServingName, Amount: amount}}
			}
			if per100.Validate() == nil {
				return per100, nil
			}
		}
	}
	f.Basis, f.Nutrients = food.PerPortion, row.Nutrients
	f.Note = cutNote(strings.TrimSpace("Portion: " + row.Portion + ". " + row.Note))
	return f, f.Validate()
}

// cutNote returns s cut to the longest note a food may have.
func cutNote(s string) string {
	if r := []rune(s); len(r) > food.MaxNoteLen {
		return strings.TrimSpace(string(r[:food.MaxNoteLen]))
	}
	return s
}

// FoodReport is what an import of foods did, or would do.
type FoodReport struct {
	// Created counts the foods added, Kept those the user has already,
	// by their name, which stay as they are.
	Created, Kept int
	// Renamed are the foods created whose name was cut, or given its
	// portion as another row has its name.
	Renamed []string
	// Skipped are the rows no food could be made of, each with why.
	Skipped []string
}

// ImportFoods adds the foods of rows to u's food database, or with dryRun
// only counts them. A food whose name u has already (from a run before, or
// their own) stays as it is, so a run again changes nothing. A name
// several rows share takes each row's portion.
func ImportFoods(ctx context.Context, db *store.DB, u user.User, rows []FoodRow, now time.Time, dryRun bool) (FoodReport, error) {
	var r FoodReport
	count := map[string]int{}
	for i := range rows {
		count[food.Key(rows[i].Name)]++
	}
	seen := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		name := row.Name
		if count[food.Key(name)] > 1 {
			name += " (" + row.Portion + ")"
		}
		fitted, cut := fitName(name, food.MaxNameLen, "")
		if fitted == "" {
			r.Skipped = append(r.Skipped, fmt.Sprintf("line %d: %q is no name", row.Line, row.Name))
			continue
		}
		if seen[food.Key(fitted)] {
			r.Skipped = append(r.Skipped, fmt.Sprintf("line %d, %s: a row before has its name and portion", row.Line, fitted))
			continue
		}
		seen[food.Key(fitted)] = true
		f, err := toFood(*row, fitted)
		if err != nil {
			r.Skipped = append(r.Skipped, fmt.Sprintf("line %d, %s: %v", row.Line, fitted, err))
			continue
		}
		_, err = db.FoodByName(ctx, u.ID, f.Name, f.Brand)
		switch {
		case err == nil:
			r.Kept++
			continue
		case !errors.Is(err, store.ErrNotFound):
			return r, err
		}
		r.Created++
		if cut || fitted != row.Name {
			r.Renamed = append(r.Renamed, fitted)
		}
		if dryRun {
			continue
		}
		if _, err := db.SaveFood(ctx, u.ID, Actor, f, now); err != nil {
			return r, fmt.Errorf("food %s: %w", f.Name, err)
		}
	}
	return r, nil
}
