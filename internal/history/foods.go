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
	for i, s := range lines {
		line := i + 1
		if !strings.HasPrefix(s, "|") || separator(s) || i+1 < len(lines) && separator(lines[i+1]) {
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
var separatorCell = regexp.MustCompile(`^\s*:?-{3,}:?\s*$`)

func separator(s string) bool {
	if !strings.HasPrefix(s, "|") {
		return false
	}
	for c := range strings.SplitSeq(strings.Trim(s, "|"), "|") {
		if !separatorCell.MatchString(c) {
			return false
		}
	}
	return true
}

// foodRow reads a row of the table: its cells, trimmed.
func foodRow(s string) (FoodRow, error) {
	cells := strings.Split(strings.Trim(s, "|"), "|")
	if len(cells) != foodColumns {
		return FoodRow{}, fmt.Errorf("%d cells, want %d", len(cells), foodColumns)
	}
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	if cells[2] == "" {
		return FoodRow{}, fmt.Errorf("%s: no kcal", cells[0])
	}
	values := make([]float64, 4)
	for i := range values {
		v, err := number(cells[2+i])
		if err != nil {
			return FoodRow{}, fmt.Errorf("%s: %w", cells[0], err)
		}
		values[i] = v
	}
	return FoodRow{
		Name: cells[0], Portion: cells[1], Note: cells[6],
		Nutrients: food.Nutrients{Kcal: values[0], Protein: values[1], Carbs: values[2], Fat: values[3]},
	}, nil
}

// valueCell is a value: a number with a decimal comma or point, after a
// bound or an approximation sign, and its unit if any.
var valueCell = regexp.MustCompile(`^[<≤~≈]?\s*(\d+(?:[.,]\d+)?)\s*(kcal|g)?$`)

// number reads a value cell; a dash or nothing is 0. Anything else after
// the number, a range ("350-400") for one, is no value.
func number(s string) (float64, error) {
	switch strings.TrimSpace(s) {
	case "", "—", "–", "-":
		return 0, nil
	}
	m := valueCell.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("value %q: want a number", s)
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
		// Per 100 when its values per 100 make sense; else per portion.
		if per100 := row.Nutrients.Scale(100 / amount); amount > 0 && per100.ValidatePer100(basis) == nil {
			f.Basis, f.Nutrients = basis, per100
			if amount != 100 {
				f.Servings = []food.Serving{{Name: ServingName, Amount: amount}}
			}
			return f, f.Validate()
		}
	}
	f.Basis, f.Nutrients = food.PerPortion, row.Nutrients
	f.Note = cutNote(strings.TrimSpace("Portion: " + row.Portion + ". " + row.Note))
	return f, f.Validate()
}

// cutNote returns s cut to the longest note a food may have.
func cutNote(s string) string {
	if r := []rune(s); len(r) > food.MaxNoteLen {
		return string(r[:food.MaxNoteLen])
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
