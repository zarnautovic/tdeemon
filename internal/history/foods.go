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
// foodColumns cells, and the lines it skipped, each with why. A row may
// start or end with a pipe too many, as a note written by hand can; a
// value may carry its unit ("99 kcal"), be a bound ("<0.5", read as the
// bound) or a dash for none (0).
func ReadFoods(r io.Reader) ([]FoodRow, []string, error) {
	var rows []FoodRow
	var skipped []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	inTable := false
	for line := 1; sc.Scan(); line++ {
		s := strings.TrimSpace(sc.Text())
		row := strings.HasPrefix(s, "|")
		// A table's first row is its header.
		header := row && !inTable
		inTable = row
		if !row || header || strings.HasPrefix(strings.ReplaceAll(s, " ", ""), "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(s, "|"), "|")
		if len(cells) != foodColumns {
			skipped = append(skipped, fmt.Sprintf("line %d: %d cells, want %d", line, len(cells), foodColumns))
			continue
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		values := make([]float64, 4)
		var bad error
		for i := range values {
			if values[i], bad = number(cells[2+i]); bad != nil {
				break
			}
		}
		if bad != nil {
			skipped = append(skipped, fmt.Sprintf("line %d, %s: %v", line, cells[0], bad))
			continue
		}
		rows = append(rows, FoodRow{
			Line: line, Name: cells[0], Portion: cells[1], Note: cells[6],
			Nutrients: food.Nutrients{Kcal: values[0], Protein: values[1], Carbs: values[2], Fat: values[3]},
		})
	}
	if err := sc.Err(); err != nil {
		return nil, nil, fmt.Errorf("the food table: %w", err)
	}
	return rows, skipped, nil
}

// leadingNumber is a number at the start of a cell, after a bound or an
// approximation sign.
var leadingNumber = regexp.MustCompile(`^[<≤~≈]?\s*(\d+(?:[.,]\d+)?)`)

// number reads a value cell: a number, with a decimal comma or point,
// what follows it ignored; a dash or nothing is 0.
func number(s string) (float64, error) {
	switch strings.TrimSpace(s) {
	case "", "—", "–", "-":
		return 0, nil
	}
	m := leadingNumber.FindStringSubmatch(strings.TrimSpace(s))
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
	f := food.Food{ID: uuid.NewV7(), Name: name, Source: food.Estimate, Note: row.Note}
	if strings.Contains(strings.ToLower(row.Note), "deklaracij") {
		f.Source = food.Label
	}
	if m := portionAmount.FindStringSubmatch(row.Portion); m != nil {
		amount, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		f.Basis = food.Per100g
		if m[2] == "ml" {
			f.Basis = food.Per100ml
		}
		if amount > 0 {
			f.Nutrients = row.Nutrients.Scale(100 / amount)
			if amount != 100 {
				f.Servings = []food.Serving{{Name: ServingName, Amount: amount}}
			}
			if f.Validate() == nil {
				return f, nil
			}
		}
	}
	f.Basis, f.Nutrients, f.Servings = food.PerPortion, row.Nutrients, nil
	f.Note = strings.TrimSpace("Portion: " + row.Portion + ". " + row.Note)
	if len([]rune(f.Note)) > food.MaxNoteLen {
		f.Note = string([]rune(f.Note)[:food.MaxNoteLen])
	}
	return f, f.Validate()
}

// FoodReport is what an import of foods did, or would do.
type FoodReport struct {
	// Created counts the foods added, Kept those the user has already,
	// by their name, which stay as they are.
	Created, Kept int
	// Renamed are the foods whose name was cut, or given its portion as
	// another row has its name.
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
		if cut || fitted != row.Name {
			r.Renamed = append(r.Renamed, fitted)
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
		if dryRun {
			continue
		}
		if _, err := db.SaveFood(ctx, u.ID, Actor, f, now); err != nil {
			return r, fmt.Errorf("food %s: %w", f.Name, err)
		}
	}
	return r, nil
}
