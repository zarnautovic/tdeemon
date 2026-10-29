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
// followed by a separator (|-|, |---|, |:---:|) is a header; the rows
// under a header that names kcal are foods, a blank line between them
// notwithstanding, and the rows of another table are skipped. A row may
// start or end with a pipe too many, as a note written by hand can,
// told from an empty last cell. A value may carry its column's unit ("99
// kcal", "2 g") or its macronutrient's name before it ("P 24"), be a
// bound ("<0.5", read as the bound) or a dash for none (0); kcal must be
// given, but for a food of no values at all.
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
	var noKcal bool
	for i, col := range valueColumns {
		v, none, err := cellValue(cells[2+i], col.unit, col.label)
		if err != nil {
			return FoodRow{}, fmt.Errorf("%s: %w", cells[0], err)
		}
		values[i] = v
		noKcal = noKcal || i == 0 && none
	}
	// No kcal, a dash or nothing, is 0 only for what has nothing else
	// either, water or black coffee; else it is missing.
	if noKcal && (values[1] != 0 || values[2] != 0 || values[3] != 0) {
		return FoodRow{}, fmt.Errorf("%s: no kcal", cells[0])
	}
	return FoodRow{
		Name: cells[0], Portion: cells[1], Note: cells[6],
		Nutrients: food.Nutrients{Kcal: values[0], Protein: values[1], Carbs: values[2], Fat: values[3]},
	}, nil
}

// valueColumns are the value columns, kcal, protein, carbs and fat: the
// unit each may name, and the name a hand may write before a value, as
// the table's header does ("P 25", "UH —").
var valueColumns = []struct{ unit, label string }{{"kcal", ""}, {"g", "P"}, {"g", "UH"}, {"g", "M"}}

// valueCell is a value: a macronutrient's name if any, then a dash for
// none, or a number with a decimal comma or point after a bound or an
// approximation sign, and a unit if any.
var valueCell = regexp.MustCompile(`^(?:(P|UH|M)\s+)?(?:(—|–|-)|[<≤~≈]?\s*(\d+(?:[.,]\d+)?)\s*([a-z]*))$`)

// cellValue reads a value cell of a column of unit and label, which it may
// name; a dash or nothing is none, 0. Anything else after the number, a
// range ("350-400"), another unit or another column's name, is no value.
func cellValue(s, unit, label string) (float64, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, true, nil
	}
	m := valueCell.FindStringSubmatch(s)
	if len(m) != 5 || m[1] != "" && m[1] != label || m[4] != "" && m[4] != unit {
		return 0, false, fmt.Errorf("value %q: want a number of %s", s, unit)
	}
	if m[2] != "" {
		return 0, true, nil
	}
	v, err := strconv.ParseFloat(strings.Replace(m[3], ",", ".", 1), 64)
	return v, false, err
}

// portionAmount reads a portion given by its weight or volume, "220 g",
// "~400 g", "0,5 kg", "330 ml", "2 dl", "1 L", as its amount and unit.
var portionAmount = regexp.MustCompile(`(?i)^~?\s*(\d+(?:[.,]\d+)?)\s*(g|kg|ml|dl|l)$`)

// portionUnits are the units of a portion's amount: the food's basis, and
// how many grams or ml one is.
var portionUnits = map[string]struct {
	basis  food.Basis
	factor float64
}{
	"g": {food.Per100g, 1}, "kg": {food.Per100g, 1000},
	"ml": {food.Per100ml, 1}, "dl": {food.Per100ml, 100}, "l": {food.Per100ml, 1000},
}

// ServingName names the portion a row gave by weight or volume, on a food
// stored per 100 g or ml.
const ServingName = "portion"

// toFood returns row as a food: per 100 g or 100 ml, with the row's portion
// as a serving, when the portion is a weight or a volume and the food per
// 100 makes sense; else per portion, the portion in its note, and whether
// it fell back so from a weight or a volume. A combination is a plate, per
// portion whatever its portion. Its source is a label when its note names
// a declaration, else an estimate.
func toFood(row FoodRow, name string) (food.Food, bool, error) {
	f := food.Food{ID: uuid.NewV7(), Name: name, Source: food.Estimate, Note: cutNote(row.Note)}
	if note := strings.ToLower(row.Note); strings.Contains(note, "deklaracij") && !strings.Contains(note, "bez deklaracij") {
		f.Source = food.Label
	}
	m := portionAmount.FindStringSubmatch(row.Portion)
	if combination(&row) {
		m = nil
	}
	if m != nil {
		amount, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		u := portionUnits[strings.ToLower(m[2])]
		amount *= u.factor
		// Per 100 when the food per 100 makes sense, its values and its
		// serving; else per portion.
		if amount > 0 {
			per100 := f
			per100.Basis, per100.Nutrients = u.basis, row.Nutrients.Scale(100/amount)
			if amount != 100 {
				per100.Servings = []food.Serving{{Name: ServingName, Amount: amount}}
			}
			if per100.Validate() == nil {
				return per100, false, nil
			}
		}
	}
	f.Basis, f.Nutrients = food.PerPortion, row.Nutrients
	f.Note = cutNote(strings.TrimSpace("Portion: " + row.Portion + ". " + row.Note))
	return f, m != nil, f.Validate()
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
	// PerPortion are the foods created per portion though their portion
	// was a weight or a volume, as per 100 they would be no food: a typo,
	// or a portion past a serving's largest; worth a look.
	PerPortion []string
	// Skipped are the rows no food could be made of, each with why.
	Skipped []string
	// Combinations are the foods made of rows of several foods eaten
	// together, a plus in their name or portion: plates, per portion,
	// logged in a tap as they were eaten.
	Combinations []string
}

// foodNames names the foods of rows: a row by its name, or, when other
// rows have its name too, with its portion after it; a combination gives
// way to a food of one, which keeps its name. Each name is cut to the
// longest a food may have, and says whether it was.
type foodNames struct {
	// plain counts the rows of each name that are no combination, all
	// every row of it.
	plain, all map[string]int
}

func namesOf(rows []FoodRow) foodNames {
	n := foodNames{plain: map[string]int{}, all: map[string]int{}}
	for i := range rows {
		key := food.Key(rows[i].Name)
		n.all[key]++
		if !combination(&rows[i]) {
			n.plain[key]++
		}
	}
	return n
}

func (n foodNames) of(row *FoodRow) (string, bool) {
	key := food.Key(row.Name)
	name := row.Name
	if combination(row) && (n.plain[key] > 0 || n.all[key] > 1) || !combination(row) && n.plain[key] > 1 {
		name += " (" + row.Portion + ")"
	}
	return fitName(name, food.MaxNameLen, "")
}

// ImportFoods adds the foods of rows to u's food database, or with dryRun
// only counts them. A food whose name u has already (from a run before,
// or their own) stays as it is, so a run again changes nothing. A name
// several rows share takes each row's portion (foodNames).
func ImportFoods(ctx context.Context, db *store.DB, u user.User, rows []FoodRow, now time.Time, dryRun bool) (FoodReport, error) {
	var r FoodReport
	names := namesOf(rows)
	seen := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		fitted, cut := names.of(row)
		if fitted == "" {
			r.Skipped = append(r.Skipped, fmt.Sprintf("line %d: %q is no name", row.Line, row.Name))
			continue
		}
		if seen[food.Key(fitted)] {
			r.Skipped = append(r.Skipped, fmt.Sprintf("line %d, %s: a row before has its name and portion", row.Line, fitted))
			continue
		}
		seen[food.Key(fitted)] = true
		f, fellBack, err := toFood(*row, fitted)
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
		if !dryRun {
			if _, err := db.SaveFood(ctx, u.ID, Actor, f, now); err != nil {
				return r, fmt.Errorf("food %s: %w", f.Name, err)
			}
		}
		r.Created++
		if combination(row) {
			r.Combinations = append(r.Combinations, fmt.Sprintf("line %d, %s", row.Line, fitted))
		}
		if cut || fitted != row.Name {
			r.Renamed = append(r.Renamed, fitted)
		}
		if fellBack {
			r.PerPortion = append(r.PerPortion, fmt.Sprintf("line %d, %s (%s)", row.Line, fitted, row.Portion))
		}
	}
	return r, nil
}
