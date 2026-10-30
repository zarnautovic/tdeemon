package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// Why a food cannot be saved as asked.
var (
	// ErrNameTaken: another of the user's foods has the name and brand
	// the food is being renamed to.
	ErrNameTaken = errors.New("another food has this name and brand")
	// ErrAliasTaken: another of the user's foods goes by the alias.
	ErrAliasTaken = errors.New("another food goes by this alias")
	// ErrBarcodeTaken: another of the user's foods has the barcode.
	ErrBarcodeTaken = errors.New("another food has this barcode")
)

// refusedFood reports whether err is a save of a food refused for what
// the user's foods have: a name, an alias or a barcode another food
// has, or a recipe's food (ErrRecipeFood).
func refusedFood(err error) bool {
	return errors.Is(err, ErrNameTaken) || errors.Is(err, ErrAliasTaken) || errors.Is(err, ErrBarcodeTaken) ||
		errors.Is(err, ErrRecipeFood)
}

// FoodConflictError is a save SaveFood refuses because another of the
// user's foods is in the way: it has the name and brand (ErrNameTaken),
// goes by Alias (ErrAliasTaken) or has Barcode (ErrBarcodeTaken). Food is
// that food, archived perhaps, which a search would not find, so a client
// can rename or correct it.
type FoodConflictError struct {
	// Err is ErrNameTaken, ErrAliasTaken or ErrBarcodeTaken.
	Err     error
	Food    food.Food
	Alias   string
	Barcode string
}

func (e *FoodConflictError) Error() string {
	switch {
	case e.Alias != "":
		return fmt.Sprintf("%v: %q, by food %s %q", e.Err, e.Alias, e.Food.ID, e.Food.Name)
	case e.Barcode != "":
		return fmt.Sprintf("%v: %s, by food %s %q", e.Err, e.Barcode, e.Food.ID, e.Food.Name)
	}
	return fmt.Sprintf("%v: food %s %q", e.Err, e.Food.ID, e.Food.Name)
}

func (e *FoodConflictError) Unwrap() error { return e.Err }

// conflict returns the FoodConflictError of the food with id in the way:
// of the name and brand, or of what, its alias or barcode (as err says).
func conflict(ctx context.Context, tx *sql.Tx, userID uuid.UUID, err error, id, what string) error {
	other, perr := uuid.Parse(id)
	if perr != nil {
		return fmt.Errorf("food id %q: %w", id, perr)
	}
	f, ferr := foodInTx(ctx, tx, userID, other)
	if ferr != nil {
		return ferr
	}
	if errors.Is(err, ErrBarcodeTaken) {
		return &FoodConflictError{Err: err, Food: f, Barcode: what}
	}
	return &FoodConflictError{Err: err, Food: f, Alias: what}
}

// MaxFoodResults bounds FindFoods.
const MaxFoodResults = 50

// MaxFoodQueries bounds the queries of one search, whose foods' last
// uses LastUses looks up at once.
const MaxFoodQueries = 20

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
// stored, with its servings, aliases and barcodes replaced by f's. Saving is
// idempotent and follows the rule that the same name and brand are the
// same food:
//   - a food with f's id is updated, renamed if f says so, unless another
//     food has the new name and brand (ErrNameTaken);
//   - otherwise a food with f's name and brand (food.Key) is updated in
//     place and keeps its own id, which the result carries;
//   - otherwise f is added. Ids are per user: another user's food under
//     f's id is not this user's, and does not change the answer.
//
// Saving the name and brand of an archived food corrects it and, unless f
// is archived too, brings it back: it is in use again. An alias another
// food goes by is refused (ErrAliasTaken), and so is a barcode another
// food has (ErrBarcodeTaken). The save is recorded as a's
// event, with the food before (none for a new one) and after.
func (db *DB) SaveFood(ctx context.Context, userID uuid.UUID, a event.Actor, f food.Food, now time.Time) (food.Food, error) {
	return db.saveFood(ctx, userID, a, f, saveOrCorrect, now)
}

// CreateFood saves f as a new food of userID's, and only that: a food of
// its name and brand (food.Key), archived too, is in the way, a
// *FoodConflictError of ErrNameTaken, in the save's own transaction.
func (db *DB) CreateFood(ctx context.Context, userID uuid.UUID, a event.Actor, f food.Food, now time.Time) (food.Food, error) {
	return db.saveFood(ctx, userID, a, f, createOnly, now)
}

// CorrectFood saves f as SaveFood does, as a correction of userID's food
// with f's id, which must be one of theirs: ErrNotFound otherwise, in the
// save's own transaction, rather than a new food or the food of f's name
// and brand. A client correcting a food by an id it has, mistyped or
// made up, or of a food undone meanwhile, changes nothing.
func (db *DB) CorrectFood(ctx context.Context, userID uuid.UUID, a event.Actor, f food.Food, now time.Time) (food.Food, error) {
	return db.saveFood(ctx, userID, a, f, correctOnly, now)
}

// saveMode is what a save of a food may do.
type saveMode int

const (
	// saveOrCorrect saves a new food, or corrects the food of the id or
	// of the name and brand (SaveFood).
	saveOrCorrect saveMode = iota
	// correctOnly corrects the food of the id (CorrectFood).
	correctOnly
	// createOnly saves a new food (CreateFood).
	createOnly
)

// saveFood is SaveFood, CorrectFood or CreateFood, as mode says.
func (db *DB) saveFood(ctx context.Context, userID uuid.UUID, a event.Actor, f food.Food, mode saveMode, now time.Time) (food.Food, error) {
	if err := f.Validate(); err != nil {
		return food.Food{}, fmt.Errorf("invalid food: %w", err)
	}
	if err := a.Validate(); err != nil {
		return food.Food{}, fmt.Errorf("invalid actor: %w", err)
	}
	var stored food.Food
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		stored, _, err = saveFoodInTx(ctx, tx, userID, a, f, mode, now)
		return err
	})
	if refusedFood(err) {
		return food.Food{}, err
	}
	if err != nil {
		return food.Food{}, fmt.Errorf("save food: %w", err)
	}
	return stored, nil
}

// saveFoodInTx saves f in tx as saveFood does in mode, and returns it as
// stored, with the id of the save's event (uuid.Nil when it recorded
// none). f and a are valid.
func saveFoodInTx(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, f food.Food, mode saveMode, now time.Time) (food.Food, uuid.UUID, error) {
	nameKey, brandKey := food.Key(f.Name), food.Key(f.Brand)
	target, exists, err := saveTarget(ctx, tx, userID, f.ID, nameKey, brandKey, mode)
	if err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	f.ID = target
	if err := checkAliases(ctx, tx, userID, f); err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	if err := checkBarcodes(ctx, tx, userID, f); err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	var before *food.Food
	if exists {
		old, err := foodInTx(ctx, tx, userID, f.ID)
		if err != nil {
			return food.Food{}, uuid.Nil(), err
		}
		before = &old
	}
	if err := checkRecipeFood(before, f); err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	if exists {
		if err := updateFood(ctx, tx, userID, f, nameKey, brandKey, now); err != nil {
			return food.Food{}, uuid.Nil(), err
		}
	} else if err := insertFood(ctx, tx, userID, f, nameKey, brandKey, now); err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	if err := replaceFoodChildren(ctx, tx, userID, f); err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	stored, err := foodInTx(ctx, tx, userID, f.ID)
	if err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	id, err := record(ctx, tx, userID, a, event.FoodSave, f.ID.String(), before, stored, now)
	return stored, id, err
}

// newFoods is what saveNewFoods made of the new foods a meal brings: the
// id each is stored under, by the id it came with; the user's foods it
// kept instead, by their id; and the events it recorded, which are parts
// of the meal's change (markParts).
type newFoods struct {
	ids   map[uuid.UUID]uuid.UUID
	kept  map[uuid.UUID]food.Food
	parts []uuid.UUID
}

// saveNewFoods saves the foods of foods that items name in tx, each as
// SaveFood does, and returns the id each is stored under by the id it came
// with. A food of the same name and brand as one the user has is that
// food, as the user has it: its values are not changed (upsert_food
// corrects a food), but the barcodes it came with are added to it, as a
// scan of it is the food; kept holds it, by its id, for its items to be
// worked out of. A food no item names (one of an op found done already)
// is not saved.
func saveNewFoods(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, foods []food.Food, items []meal.Item,
	now time.Time,
) (newFoods, error) {
	named := make(map[uuid.UUID]bool, len(items))
	for i := range items {
		named[items[i].FoodID] = true
	}
	out := newFoods{ids: make(map[uuid.UUID]uuid.UUID, len(foods)), kept: map[uuid.UUID]food.Food{}}
	for i := range foods {
		f := &foods[i]
		if !named[f.ID] {
			continue
		}
		target, exists, err := saveTarget(ctx, tx, userID, f.ID, food.Key(f.Name), food.Key(f.Brand), saveOrCorrect)
		if err != nil {
			return newFoods{}, err
		}
		var write uuid.UUID
		if exists {
			had, brought, err := keptFood(ctx, tx, userID, a, target, now)
			if err != nil {
				return newFoods{}, err
			}
			had, linked, err := linkBarcodes(ctx, tx, userID, a, had, f.Barcodes, now)
			if err != nil {
				return newFoods{}, err
			}
			if linked != uuid.Nil() {
				out.parts = append(out.parts, linked)
			}
			out.ids[f.ID], out.kept[target], write = target, had, brought
		} else {
			stored, saved, err := saveFoodInTx(ctx, tx, userID, a, *f, saveOrCorrect, now)
			if err != nil {
				return newFoods{}, err
			}
			out.ids[f.ID], write = stored.ID, saved
		}
		if write != uuid.Nil() {
			out.parts = append(out.parts, write)
		}
	}
	return out, nil
}

// keptFood returns userID's food id, which a new food of its name and
// brand is, brought back if it was archived: the user eats it again, and
// a search must find it next time. Bringing it back is a's event, whose
// id it returns too (uuid.Nil when the food was in use already).
func keptFood(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, id uuid.UUID, now time.Time) (food.Food, uuid.UUID, error) {
	had, err := foodInTx(ctx, tx, userID, id)
	if err != nil || !had.Archived {
		return had, uuid.Nil(), err
	}
	if err := setArchived(ctx, tx, userID, id, false, now); err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	brought, err := record(ctx, tx, userID, a, event.FoodArchive, id.String(), archivedState{true}, archivedState{false}, now)
	if err != nil {
		return food.Food{}, uuid.Nil(), err
	}
	had.Archived = false
	return had, brought, nil
}

// linkBarcodes adds to userID's food had the barcodes of codes it lacks,
// as a's save of it, whose event's id it returns (uuid.Nil when it has
// them all). A barcode another food has is refused (ErrBarcodeTaken), as
// it is for a new food; past food.MaxBarcodes, the rest are left out.
func linkBarcodes(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, had food.Food, codes []string,
	now time.Time,
) (food.Food, uuid.UUID, error) {
	more := slices.Clone(had.Barcodes)
	for _, code := range codes {
		if !slices.Contains(more, code) && len(more) < food.MaxBarcodes {
			more = append(more, code)
		}
	}
	if len(more) == len(had.Barcodes) {
		return had, uuid.Nil(), nil
	}
	linked := had
	linked.Barcodes = more
	return saveFoodInTx(ctx, tx, userID, a, linked, saveOrCorrect, now)
}

// FoodByKey returns userID's food of name and brand, compared as SaveFood
// compares them (food.Key), archived too, or ErrNotFound: the food a new
// food of that name and brand sent with a meal is.
func (db *DB) FoodByKey(ctx context.Context, userID uuid.UUID, name, brand string) (food.Food, error) {
	var f food.Food
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var id string
		err := tx.QueryRowContext(ctx, "SELECT id FROM foods WHERE user_id = ? AND name_key = ? AND brand_key = ?",
			userID.String(), food.Key(name), food.Key(brand)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		fid, err := uuid.Parse(id)
		if err != nil {
			return fmt.Errorf("food id %q: %w", id, err)
		}
		f, err = foodInTx(ctx, tx, userID, fid)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return food.Food{}, err
	}
	if err != nil {
		return food.Food{}, fmt.Errorf("food by key: %w", err)
	}
	return f, nil
}

// validFoods reports the first of foods that breaks the rules.
func validFoods(foods []food.Food) error {
	for i := range foods {
		if err := foods[i].Validate(); err != nil {
			return fmt.Errorf("invalid food: %w", err)
		}
	}
	return nil
}

// saveTarget returns the id SaveFood writes to and whether that row
// exists; when correcting only, the row with id, or ErrNotFound; when
// creating only, a new row, or the food of the name and brand in the way.
func saveTarget(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID, nameKey, brandKey string, mode saveMode) (uuid.UUID, bool, error) {
	uid := userID.String()
	var one int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM foods WHERE user_id = ? AND id = ?", uid, id.String()).Scan(&one)
	switch {
	case err == nil && mode == createOnly:
		// A new food under the id of one the user has corrects nothing.
		return uuid.Nil(), false, conflict(ctx, tx, userID, ErrNameTaken, id.String(), "")
	case err == nil:
		var other string
		err := tx.QueryRowContext(ctx, "SELECT id FROM foods WHERE user_id = ? AND name_key = ? AND brand_key = ? AND id <> ?",
			uid, nameKey, brandKey, id.String()).Scan(&other)
		if err == nil {
			return uuid.Nil(), false, conflict(ctx, tx, userID, ErrNameTaken, other, "")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil(), false, err
		}
		return id, true, nil
	case !errors.Is(err, sql.ErrNoRows):
		return uuid.Nil(), false, err
	case mode == correctOnly:
		return uuid.Nil(), false, ErrNotFound
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
	if mode == createOnly {
		return uuid.Nil(), false, conflict(ctx, tx, userID, ErrNameTaken, same, "")
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
			return conflict(ctx, tx, userID, ErrAliasTaken, other, a)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

// checkBarcodes refuses a barcode of f's that another of userID's foods
// has.
func checkBarcodes(ctx context.Context, tx *sql.Tx, userID uuid.UUID, f food.Food) error {
	for _, code := range f.Barcodes {
		var other string
		err := tx.QueryRowContext(ctx, "SELECT food_id FROM food_barcodes WHERE user_id = ? AND code = ? AND food_id <> ?",
			userID.String(), code, f.ID.String()).Scan(&other)
		if err == nil {
			return conflict(ctx, tx, userID, ErrBarcodeTaken, other, code)
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

// replaceFoodChildren writes f's servings, aliases, barcodes and search
// row in place of the old ones.
func replaceFoodChildren(ctx context.Context, tx *sql.Tx, userID uuid.UUID, f food.Food) error {
	id, uid := f.ID.String(), userID.String()
	for _, q := range []string{
		"DELETE FROM food_servings WHERE food_id = ? AND user_id = ?",
		"DELETE FROM food_aliases WHERE food_id = ? AND user_id = ?",
		"DELETE FROM food_barcodes WHERE food_id = ? AND user_id = ?",
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
	for _, code := range f.Barcodes {
		if _, err := tx.ExecContext(ctx, "INSERT INTO food_barcodes (food_id, user_id, code) VALUES (?, ?, ?)",
			id, uid, code); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO foods_fts (food_id, user_id, name, brand, aliases) VALUES (?, ?, ?, ?, ?)",
		id, uid, searchText(f.Name), searchText(f.Brand), searchText(strings.Join(f.Aliases, "\n")))
	return err
}

// searchText is a text as foods_fts indexes it: the text, and with a đ
// in it, the text again with d and with dj in its place. The tokenizer's
// remove_diacritics leaves đ as it is, as no d and mark make it up, and
// Croatian is typed both ways without the stroke: "grožđe" is found as
// "grozde" and as "grozdje". Migration 0018 indexes the foods saved
// before it the same way.
func searchText(s string) string {
	if !strings.ContainsAny(s, "đĐ") {
		return s
	}
	d := strings.NewReplacer("đ", "d", "Đ", "D").Replace(s)
	dj := strings.NewReplacer("đ", "dj", "Đ", "Dj").Replace(s)
	return s + "\n" + d + "\n" + dj
}

// FoodByName returns userID's food with name and brand, compared by
// food.Key as a save compares them, archived or not, or ErrNotFound.
func (db *DB) FoodByName(ctx context.Context, userID uuid.UUID, name, brand string) (food.Food, error) {
	var f food.Food
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var id string
		err := tx.QueryRowContext(ctx, "SELECT id FROM foods WHERE user_id = ? AND name_key = ? AND brand_key = ?",
			userID.String(), food.Key(name), food.Key(brand)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		fid, err := uuid.Parse(id)
		if err != nil {
			return err
		}
		f, err = foodInTx(ctx, tx, userID, fid)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return food.Food{}, err
	}
	if err != nil {
		return food.Food{}, fmt.Errorf("food by name: %w", err)
	}
	return f, nil
}

// FoodByBarcode returns userID's food of the barcode code, in its
// canonical form (food.Barcode), archived or not, or ErrNotFound.
func (db *DB) FoodByBarcode(ctx context.Context, userID uuid.UUID, code string) (food.Food, error) {
	var f food.Food
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var id string
		err := tx.QueryRowContext(ctx, "SELECT food_id FROM food_barcodes WHERE user_id = ? AND code = ?",
			userID.String(), code).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		fid, err := uuid.Parse(id)
		if err != nil {
			return fmt.Errorf("food id %q: %w", id, err)
		}
		f, err = foodInTx(ctx, tx, userID, fid)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return food.Food{}, err
	}
	if err != nil {
		return food.Food{}, fmt.Errorf("food by barcode: %w", err)
	}
	return f, nil
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

// foodChildren reads f's servings, aliases and barcodes, in the order
// saved.
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
	if err := arows.Err(); err != nil {
		return err
	}
	brows, err := tx.QueryContext(ctx, "SELECT code FROM food_barcodes WHERE food_id = ? AND user_id = ? ORDER BY rowid",
		f.ID.String(), userID.String())
	if err != nil {
		return err
	}
	defer func() { _ = brows.Close() }()
	for brows.Next() {
		var code string
		if err := brows.Scan(&code); err != nil {
			return err
		}
		f.Barcodes = append(f.Barcodes, code)
	}
	return brows.Err()
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
// before, or as saved before. A food that e added goes, unless a meal or
// a recipe uses it, which keeps it, archived.
func takeBackFood(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	id, err := entityID(e)
	if err != nil {
		return err
	}
	switch {
	case e.Action == event.FoodArchive:
		var s archivedState
		if err := decode(e.Before, &s); err != nil {
			return err
		}
		return setArchived(ctx, tx, userID, id, s.Archived, now)
	case e.Before == nil:
		used, err := foodInUse(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		if used {
			return setArchived(ctx, tx, userID, id, true, now)
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM foods WHERE user_id = ? AND id = ?", userID.String(), id.String())
		return err
	}
	var f food.Food
	if err := decode(e.Before, &f); err != nil {
		return err
	}
	if err := checkFoodClash(ctx, tx, userID, f); err != nil {
		return err
	}
	if err := updateFood(ctx, tx, userID, f, food.Key(f.Name), food.Key(f.Brand), now); err != nil {
		return err
	}
	return replaceFoodChildren(ctx, tx, userID, f)
}

// checkFoodClash refuses to put food f back when another food has taken
// its name, an alias or a barcode since, a clash undo passes over.
func checkFoodClash(ctx context.Context, tx *sql.Tx, userID uuid.UUID, f food.Food) error {
	var other int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM foods WHERE user_id = ? AND name_key = ? AND brand_key = ? AND id <> ?",
		userID.String(), food.Key(f.Name), food.Key(f.Brand), f.ID.String()).Scan(&other); err != nil {
		return err
	}
	if other > 0 {
		return clash{UndoNameTaken}
	}
	if err := checkAliases(ctx, tx, userID, f); errors.Is(err, ErrAliasTaken) {
		return fmt.Errorf("%w: %w", clash{UndoAliasTaken}, err)
	} else if err != nil {
		return err
	}
	if err := checkBarcodes(ctx, tx, userID, f); errors.Is(err, ErrBarcodeTaken) {
		return fmt.Errorf("%w: %w", clash{UndoBarcodeTaken}, err)
	} else if err != nil {
		return err
	}
	return nil
}

// foodStateNow returns the state of e's food now, as e's kind of event
// records it: the food (nil when gone), or whether it is archived.
func foodStateNow(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event) (any, error) {
	id, err := entityID(e)
	if err != nil {
		return nil, err
	}
	f, err := foodInTx(ctx, tx, userID, id)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if e.Action == event.FoodArchive {
		return archivedState{f.Archived}, nil
	}
	return f, nil
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
	return db.findFoods(ctx, userID, query, limit, false)
}

// FindArchivedFoods is FindFoods among userID's archived foods alone.
func (db *DB) FindArchivedFoods(ctx context.Context, userID uuid.UUID, query string, limit int) ([]food.Food, error) {
	return db.findFoods(ctx, userID, query, limit, true)
}

// findFoods is FindFoods among userID's archived foods, or the others, as
// archived says.
func (db *DB) findFoods(ctx context.Context, userID uuid.UUID, query string, limit int, archived bool) ([]food.Food, error) {
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
			WHERE f.user_id = ? AND f.archived = ?
				AND f.id IN (SELECT food_id FROM foods_fts WHERE foods_fts MATCH ? AND user_id = ?)
			ORDER BY CASE
					WHEN f.id IN (SELECT food_id FROM foods_fts WHERE foods_fts MATCH ? AND user_id = ?) THEN 0
					WHEN f.id IN (SELECT food_id FROM foods_fts WHERE foods_fts MATCH ? AND user_id = ?) THEN 1
					ELSE 2
				END, length(f.name), f.name
			LIMIT ?`, uid, archived, match, uid, "{name} : ("+match+")", uid, "{name aliases} : ("+match+")", uid, limit)
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

// FoodOrder is the order of a list of foods.
type FoodOrder string

// The orders of ListFoods.
const (
	// ByName sorts by name, then brand, in any case (food.Key), by code
	// point: a letter with a diacritic after the plain ones.
	ByName FoodOrder = "name"
	// ByUse puts the food a meal had last first, and the foods no meal
	// has had after the others, by name.
	ByUse FoodOrder = "use"
	// ByFrequency puts the food the most of the user's latest 100 meals
	// had first, and then the others as ByUse.
	ByFrequency FoodOrder = "frequent"
)

// frequentMeals is how many of a user's latest meals ByFrequency counts.
const frequentMeals = 100

// MaxFoodList bounds a page of ListFoods.
const MaxFoodList = 200

// ListFoods returns a page of userID's foods, the archived ones or the
// others as archived says, and of source alone unless it is empty, in
// order, from offset and at most limit of them, with their servings,
// aliases and barcodes; and how many such foods there are.
func (db *DB) ListFoods(ctx context.Context, userID uuid.UUID, archived bool, source food.Source, order FoodOrder, limit, offset int) ([]food.Food, int, error) {
	by := "f.name_key, f.brand_key, f.id"
	lastUse := `(SELECT max(m.eaten_at) FROM meal_items AS i JOIN meals AS m ON m.id = i.meal_id AND m.user_id = i.user_id
				WHERE i.user_id = f.user_id AND i.food_id = f.id) DESC NULLS LAST, `
	switch order {
	case ByName:
	case ByUse:
		by = lastUse + by
	case ByFrequency:
		by = `(SELECT count(DISTINCT i.meal_id) FROM meal_items AS i
				WHERE i.user_id = f.user_id AND i.food_id = f.id AND i.meal_id IN (
					SELECT m.id FROM meals AS m WHERE m.user_id = f.user_id ORDER BY m.eaten_at DESC, m.id DESC LIMIT ` +
			fmt.Sprint(frequentMeals) + `)) DESC, ` + lastUse + by
	default:
		return nil, 0, fmt.Errorf("list foods: order %q: want name, use or frequent", order)
	}
	limit = min(max(limit, 0), MaxFoodList)
	offset = max(offset, 0)
	uid := userID.String()
	var out []food.Food
	var total int
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM foods WHERE user_id = ? AND archived = ? AND (? = '' OR source = ?)",
			uid, archived, source, source).Scan(&total); err != nil {
			return err
		}
		var err error
		out, err = queryFoods(ctx, tx, `
			SELECT `+foodColumns+` FROM foods AS f
			WHERE f.user_id = ? AND f.archived = ? AND (? = '' OR f.source = ?)
			ORDER BY `+by+`
			LIMIT ? OFFSET ?`, uid, archived, source, source, limit, offset)
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
		return nil, 0, fmt.Errorf("list foods: %w", err)
	}
	return out, total, nil
}

// queryFoods runs q, which selects foodColumns, and returns the foods,
// without their servings, aliases and barcodes.
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
	// A đ typed is looked for as dj, which searchText indexes too.
	dj := strings.NewReplacer("đ", "dj", "Đ", "Dj")
	for i, w := range words {
		words[i] = `"` + dj.Replace(w) + `"*`
	}
	return strings.Join(words, " ")
}
