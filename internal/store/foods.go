package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

// Why a food cannot be saved as asked.
var (
	// ErrNameTaken: another of the user's foods has the name and brand
	// the food is being renamed to.
	ErrNameTaken = errors.New("another food has this name and brand")
	// ErrAliasTaken: another of the user's foods goes by the alias.
	ErrAliasTaken = errors.New("another food goes by this alias")
)

// MaxFoodResults bounds FindFoods.
const MaxFoodResults = 50

const foodColumns = `f.id, f.name, f.brand, f.basis, f.kcal, f.protein, f.carbs, f.fat,
	f.fiber, f.saturated_fat, f.sugars, f.salt, f.polyols, f.alcohol, f.portion_g, f.source, f.note, f.archived`

// foodFields receives foodColumns.
type foodFields struct {
	id, name, brand, basis, source, note string
	kcal, protein, carbs, fat            float64
	optional                             [6]sql.NullFloat64
	portion                              sql.NullFloat64
	archived                             int64
}

func (r *foodFields) dest() []any {
	return []any{
		&r.id, &r.name, &r.brand, &r.basis, &r.kcal, &r.protein, &r.carbs, &r.fat,
		&r.optional[0], &r.optional[1], &r.optional[2], &r.optional[3], &r.optional[4], &r.optional[5],
		&r.portion, &r.source, &r.note, &r.archived,
	}
}

func (r *foodFields) food() (food.Food, error) {
	id, err := uuid.Parse(r.id)
	if err != nil {
		return food.Food{}, fmt.Errorf("food id %q: %w", r.id, err)
	}
	f := food.Food{
		ID: id, Name: r.name, Brand: r.brand, Basis: food.Basis(r.basis), Source: food.Source(r.source),
		Note: r.note, Archived: r.archived != 0,
		Nutrients: food.Nutrients{Kcal: r.kcal, Protein: r.protein, Carbs: r.carbs, Fat: r.fat},
	}
	for i, p := range optionalNutrients(&f.Nutrients) {
		if r.optional[i].Valid {
			v := r.optional[i].Float64
			*p = &v
		}
	}
	if r.portion.Valid {
		f.PortionGrams = r.portion.Float64
	}
	return f, nil
}

// optionalNutrients lists n's optional nutrients in the order of the
// columns fiber, saturated_fat, sugars, salt, polyols, alcohol.
func optionalNutrients(n *food.Nutrients) [6]**float64 {
	return [6]**float64{&n.Fiber, &n.SaturatedFat, &n.Sugars, &n.Salt, &n.Polyols, &n.Alcohol}
}

func nullable(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// SaveFood stores f in userID's food database and returns the food as
// stored, with its servings and aliases replaced by f's. Saving is
// idempotent and follows the rule that the same name and brand are the
// same food:
//   - a food with f's id is updated, renamed if f says so, unless another
//     food has the new name and brand (ErrNameTaken);
//   - otherwise a food with f's name and brand (food.Key) is updated in
//     place and keeps its own id, which the result carries;
//   - otherwise f is added. Ids are per user: another user's food under
//     f's id is not this user's, and does not change the answer.
//
// Saving the name and brand of an archived food corrects it and, as f is
// not archived, brings it back: it is in use again. An alias another
// food goes by is refused (ErrAliasTaken). The save is recorded as a's
// event, with the food before (none for a new one) and after.
func (db *DB) SaveFood(ctx context.Context, userID uuid.UUID, a event.Actor, f food.Food, now time.Time) (food.Food, error) {
	if err := f.Validate(); err != nil {
		return food.Food{}, fmt.Errorf("invalid food: %w", err)
	}
	if err := a.Validate(); err != nil {
		return food.Food{}, fmt.Errorf("invalid actor: %w", err)
	}
	nameKey, brandKey := food.Key(f.Name), food.Key(f.Brand)
	var stored food.Food
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		target, exists, err := saveTarget(ctx, tx, userID, f.ID, nameKey, brandKey)
		if err != nil {
			return err
		}
		f.ID = target
		if err := checkAliases(ctx, tx, userID, f); err != nil {
			return err
		}
		var before *food.Food
		if exists {
			old, err := foodInTx(ctx, tx, userID, f.ID)
			if err != nil {
				return err
			}
			before = &old
			err = updateFood(ctx, tx, userID, f, nameKey, brandKey, now)
			if err != nil {
				return err
			}
		} else if err := insertFood(ctx, tx, userID, f, nameKey, brandKey, now); err != nil {
			return err
		}
		if err := replaceFoodChildren(ctx, tx, userID, f); err != nil {
			return err
		}
		if stored, err = foodInTx(ctx, tx, userID, f.ID); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.FoodSave, f.ID.String(), before, stored, now)
		return err
	})
	if errors.Is(err, ErrNameTaken) || errors.Is(err, ErrAliasTaken) {
		return food.Food{}, err
	}
	if err != nil {
		return food.Food{}, fmt.Errorf("save food: %w", err)
	}
	return stored, nil
}

// saveTarget returns the id SaveFood writes to and whether that row
// exists.
func saveTarget(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID, nameKey, brandKey string) (uuid.UUID, bool, error) {
	uid := userID.String()
	var one int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM foods WHERE user_id = ? AND id = ?", uid, id.String()).Scan(&one)
	switch {
	case err == nil:
		var other string
		err := tx.QueryRowContext(ctx, "SELECT id FROM foods WHERE user_id = ? AND name_key = ? AND brand_key = ? AND id <> ?",
			uid, nameKey, brandKey, id.String()).Scan(&other)
		if err == nil {
			return uuid.Nil(), false, ErrNameTaken
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil(), false, err
		}
		return id, true, nil
	case !errors.Is(err, sql.ErrNoRows):
		return uuid.Nil(), false, err
	}
	var same string
	err = tx.QueryRowContext(ctx, "SELECT id FROM foods WHERE user_id = ? AND name_key = ? AND brand_key = ?",
		uid, nameKey, brandKey).Scan(&same)
	if errors.Is(err, sql.ErrNoRows) {
		return id, false, nil
	}
	if err != nil {
		return uuid.Nil(), false, err
	}
	existing, err := uuid.Parse(same)
	return existing, true, err
}

func checkAliases(ctx context.Context, tx *sql.Tx, userID uuid.UUID, f food.Food) error {
	for _, a := range f.Aliases {
		var other string
		err := tx.QueryRowContext(ctx, "SELECT food_id FROM food_aliases WHERE user_id = ? AND alias_key = ? AND food_id <> ?",
			userID.String(), food.Key(a), f.ID.String()).Scan(&other)
		if err == nil {
			return fmt.Errorf("%w: %q", ErrAliasTaken, a)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func foodValues(f food.Food) []any {
	o := optionalNutrients(&f.Nutrients)
	var portion any
	if f.PortionGrams > 0 {
		portion = f.PortionGrams
	}
	archived := 0
	if f.Archived {
		archived = 1
	}
	return []any{
		f.Name, f.Brand, string(f.Basis), f.Nutrients.Kcal, f.Nutrients.Protein, f.Nutrients.Carbs, f.Nutrients.Fat,
		nullable(*o[0]), nullable(*o[1]), nullable(*o[2]), nullable(*o[3]), nullable(*o[4]), nullable(*o[5]),
		portion, string(f.Source), f.Note, archived,
	}
}

func insertFood(ctx context.Context, tx *sql.Tx, userID uuid.UUID, f food.Food, nameKey, brandKey string, now time.Time) error {
	args := append([]any{f.ID.String(), userID.String(), nameKey, brandKey}, foodValues(f)...)
	args = append(args, formatTime(now), formatTime(now))
	_, err := tx.ExecContext(ctx, `INSERT INTO foods (id, user_id, name_key, brand_key,
		name, brand, basis, kcal, protein, carbs, fat, fiber, saturated_fat, sugars, salt, polyols, alcohol,
		portion_g, source, note, archived, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
	return err
}

func updateFood(ctx context.Context, tx *sql.Tx, userID uuid.UUID, f food.Food, nameKey, brandKey string, now time.Time) error {
	args := append([]any{nameKey, brandKey}, foodValues(f)...)
	args = append(args, formatTime(now), f.ID.String(), userID.String())
	_, err := tx.ExecContext(ctx, `UPDATE foods SET name_key = ?, brand_key = ?,
		name = ?, brand = ?, basis = ?, kcal = ?, protein = ?, carbs = ?, fat = ?, fiber = ?, saturated_fat = ?,
		sugars = ?, salt = ?, polyols = ?, alcohol = ?, portion_g = ?, source = ?, note = ?, archived = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`, args...)
	return err
}

// replaceFoodChildren writes f's servings, aliases and search row in
// place of the old ones.
func replaceFoodChildren(ctx context.Context, tx *sql.Tx, userID uuid.UUID, f food.Food) error {
	id, uid := f.ID.String(), userID.String()
	for _, q := range []string{
		"DELETE FROM food_servings WHERE food_id = ? AND user_id = ?",
		"DELETE FROM food_aliases WHERE food_id = ? AND user_id = ?",
		"DELETE FROM foods_fts WHERE food_id = ? AND user_id = ?",
	} {
		if _, err := tx.ExecContext(ctx, q, id, uid); err != nil {
			return err
		}
	}
	for _, s := range f.Servings {
		if _, err := tx.ExecContext(ctx, "INSERT INTO food_servings (food_id, user_id, name, name_key, amount) VALUES (?, ?, ?, ?, ?)",
			id, uid, s.Name, food.Key(s.Name), s.Amount); err != nil {
			return err
		}
	}
	for _, a := range f.Aliases {
		if _, err := tx.ExecContext(ctx, "INSERT INTO food_aliases (food_id, user_id, alias, alias_key) VALUES (?, ?, ?, ?)",
			id, uid, a, food.Key(a)); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO foods_fts (food_id, user_id, name, brand, aliases) VALUES (?, ?, ?, ?, ?)",
		id, uid, f.Name, f.Brand, strings.Join(f.Aliases, "\n"))
	return err
}

// FoodByID returns userID's food with id, archived or not, or
// ErrNotFound.
func (db *DB) FoodByID(ctx context.Context, userID, id uuid.UUID) (food.Food, error) {
	var f food.Food
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		f, err = foodInTx(ctx, tx, userID, id)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return food.Food{}, err
	}
	if err != nil {
		return food.Food{}, fmt.Errorf("food: %w", err)
	}
	return f, nil
}

// foodInTx reads userID's food id with its children in tx, or returns
// ErrNotFound.
func foodInTx(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID) (food.Food, error) {
	var r foodFields
	err := tx.QueryRowContext(ctx, "SELECT "+foodColumns+" FROM foods AS f WHERE f.id = ? AND f.user_id = ?",
		id.String(), userID.String()).Scan(r.dest()...)
	if errors.Is(err, sql.ErrNoRows) {
		return food.Food{}, ErrNotFound
	}
	if err != nil {
		return food.Food{}, err
	}
	f, err := r.food()
	if err != nil {
		return food.Food{}, err
	}
	err = foodChildren(ctx, tx, userID, &f)
	return f, err
}

// foodChildren reads f's servings and aliases, in the order saved.
func foodChildren(ctx context.Context, tx *sql.Tx, userID uuid.UUID, f *food.Food) error {
	rows, err := tx.QueryContext(ctx, "SELECT name, amount FROM food_servings WHERE food_id = ? AND user_id = ? ORDER BY rowid",
		f.ID.String(), userID.String())
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var s food.Serving
		if err := rows.Scan(&s.Name, &s.Amount); err != nil {
			return err
		}
		f.Servings = append(f.Servings, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	arows, err := tx.QueryContext(ctx, "SELECT alias FROM food_aliases WHERE food_id = ? AND user_id = ? ORDER BY rowid",
		f.ID.String(), userID.String())
	if err != nil {
		return err
	}
	defer func() { _ = arows.Close() }()
	for arows.Next() {
		var a string
		if err := arows.Scan(&a); err != nil {
			return err
		}
		f.Aliases = append(f.Aliases, a)
	}
	return arows.Err()
}

// SetFoodArchived archives userID's food with id (or brings it back): an
// archived food is kept, and the meals that used it keep their items, but
// search no longer offers it. It is recorded as a's event.
func (db *DB) SetFoodArchived(ctx context.Context, userID uuid.UUID, a event.Actor, id uuid.UUID, archived bool, now time.Time) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var was bool
		err := tx.QueryRowContext(ctx, "SELECT archived FROM foods WHERE user_id = ? AND id = ?",
			userID.String(), id.String()).Scan(&was)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := setArchived(ctx, tx, userID, id, archived, now); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.FoodArchive, id.String(), archivedState{was}, archivedState{archived}, now)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("archive food: %w", err)
	}
	return nil
}

// archivedState is a food.archive event's state.
type archivedState struct{ Archived bool }

func setArchived(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID, archived bool, now time.Time) error {
	v := 0
	if archived {
		v = 1
	}
	_, err := tx.ExecContext(ctx, "UPDATE foods SET archived = ?, updated_at = ? WHERE id = ? AND user_id = ?",
		v, formatTime(now), id.String(), userID.String())
	return err
}

// takeBackFood puts the food of e back as it was: archived or not as
// before, or as saved before. A food that e added goes, unless a meal
// uses it, which keeps it, archived.
func takeBackFood(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	id, err := uuid.Parse(e.Entity)
	if err != nil {
		return fmt.Errorf("food id %q: %w", e.Entity, err)
	}
	switch {
	case e.Action == event.FoodArchive:
		var s archivedState
		if err := json.Unmarshal(e.Before, &s); err != nil {
			return err
		}
		return setArchived(ctx, tx, userID, id, s.Archived, now)
	case e.Before == nil:
		var used int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM meal_items WHERE user_id = ? AND food_id = ?",
			userID.String(), id.String()).Scan(&used); err != nil {
			return err
		}
		if used > 0 {
			return setArchived(ctx, tx, userID, id, true, now)
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM foods WHERE user_id = ? AND id = ?", userID.String(), id.String())
		return err
	}
	var f food.Food
	if err := json.Unmarshal(e.Before, &f); err != nil {
		return err
	}
	if err := updateFood(ctx, tx, userID, f, food.Key(f.Name), food.Key(f.Brand), now); err != nil {
		return err
	}
	return replaceFoodChildren(ctx, tx, userID, f)
}

// FindFoods returns up to limit of userID's foods, not archived, whose
// name, brand or an alias has words starting with each word of query,
// without regard to case or diacritics: "cev" finds "Ćevapi", "grčki jog"
// finds "Grčki jogurt". Foods whose name holds every word come first,
// then those whose name and aliases do, then the rest; within each, the
// shorter name, then by name. The order depends on the user's foods
// alone, not on other users' (which a relevance score over the whole
// index would). A query with no words finds nothing.
func (db *DB) FindFoods(ctx context.Context, userID uuid.UUID, query string, limit int) ([]food.Food, error) {
	match := ftsQuery(query)
	if match == "" || limit <= 0 {
		return nil, nil
	}
	limit = min(limit, MaxFoodResults)
	uid := userID.String()
	var out []food.Food
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = queryFoods(ctx, tx, `
			SELECT `+foodColumns+` FROM foods AS f
			WHERE f.user_id = ? AND f.archived = 0
				AND f.id IN (SELECT food_id FROM foods_fts WHERE foods_fts MATCH ? AND user_id = ?)
			ORDER BY CASE
					WHEN f.id IN (SELECT food_id FROM foods_fts WHERE foods_fts MATCH ? AND user_id = ?) THEN 0
					WHEN f.id IN (SELECT food_id FROM foods_fts WHERE foods_fts MATCH ? AND user_id = ?) THEN 1
					ELSE 2
				END, length(f.name), f.name
			LIMIT ?`, uid, match, uid, "{name} : ("+match+")", uid, "{name aliases} : ("+match+")", uid, limit)
		if err != nil {
			return err
		}
		for i := range out {
			if err := foodChildren(ctx, tx, userID, &out[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("find foods: %w", err)
	}
	return out, nil
}

// queryFoods runs q, which selects foodColumns, and returns the foods,
// without their servings and aliases.
func queryFoods(ctx context.Context, tx *sql.Tx, q string, args ...any) ([]food.Food, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []food.Food
	for rows.Next() {
		var r foodFields
		if err := rows.Scan(r.dest()...); err != nil {
			return nil, err
		}
		f, err := r.food()
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// maxQueryWords bounds the words FindFoods matches on.
const maxQueryWords = 8

// ftsQuery turns what a person typed into an FTS5 query: each word, as a
// quoted prefix, all of them required. Quoting keeps FTS5's own syntax
// (AND, NEAR, column filters, *) out of it.
func ftsQuery(s string) string {
	// A combining mark (a caron pasted from a PDF, after its "c") is part
	// of its word; the tokenizer drops it with the other diacritics.
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.Is(unicode.Mn, r)
	})
	if len(words) > maxQueryWords {
		words = words[:maxQueryWords]
	}
	for i, w := range words {
		words[i] = `"` + w + `"*`
	}
	return strings.Join(words, " ")
}
