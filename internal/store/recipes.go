package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/recipe"
)

// Why a recipe, or a food of one, cannot be saved as asked.
var (
	// ErrInvalidRecipe: the recipe breaks a rule of package recipe, as
	// its ingredients came out once their new foods were saved.
	ErrInvalidRecipe = errors.New("invalid recipe")
	// ErrRecipeCycle: an ingredient is the recipe, through the recipes
	// it is made of.
	ErrRecipeCycle = errors.New("the recipe is an ingredient of one of its ingredients")
	// ErrRecipeFood: a save of a food would change a recipe's food, whose
	// values change only with its recipe (SaveRecipe), or make a food of
	// source recipe that has no recipe.
	ErrRecipeFood = errors.New("a recipe's food changes only with its recipe")
)

// SaveRecipe stores r, a recipe of userID's, worked out of its
// ingredients (recipe.Make), and returns it as stored. As SaveFood does
// for a food, it corrects the recipe of r's id, or else the one of r's
// name, or adds it; a food of the name that is no recipe is in the way, a
// *FoodConflictError of ErrNameTaken, and so is a food of an alias of
// r's (ErrAliasTaken). foods are new foods its ingredients name by their
// ids, saved first as a meal's are (CreateMealWithFoods): a food of the
// name and brand of one the user has is that food, and the ingredient is
// worked out of it. The recipe is saved whole: its food and ingredients
// replace the ones before, each ingredient with the values its food
// brings now. It is recorded as a's event, with the recipe before (none
// for a new one) and after, the new foods' writes parts of it.
func (db *DB) SaveRecipe(ctx context.Context, userID uuid.UUID, a event.Actor, r recipe.Recipe, foods []food.Food, now time.Time) (recipe.Recipe, error) {
	return db.saveRecipe(ctx, userID, a, r, foods, saveOrCorrect, now)
}

// CreateRecipe is SaveRecipe of a new recipe only: a food of its name,
// archived too, is in the way, a *FoodConflictError of ErrNameTaken.
func (db *DB) CreateRecipe(ctx context.Context, userID uuid.UUID, a event.Actor, r recipe.Recipe, foods []food.Food, now time.Time) (recipe.Recipe, error) {
	return db.saveRecipe(ctx, userID, a, r, foods, createOnly, now)
}

// CorrectRecipe is SaveRecipe of userID's recipe of r's id, which must
// be one of theirs: ErrNotFound otherwise.
func (db *DB) CorrectRecipe(ctx context.Context, userID uuid.UUID, a event.Actor, r recipe.Recipe, foods []food.Food, now time.Time) (recipe.Recipe, error) {
	return db.saveRecipe(ctx, userID, a, r, foods, correctOnly, now)
}

func (db *DB) saveRecipe(ctx context.Context, userID uuid.UUID, a event.Actor, r recipe.Recipe, foods []food.Food, mode saveMode,
	now time.Time,
) (recipe.Recipe, error) {
	if err := r.Validate(); err != nil {
		return recipe.Recipe{}, fmt.Errorf("%w: %w", ErrInvalidRecipe, err)
	}
	if err := a.Validate(); err != nil {
		return recipe.Recipe{}, fmt.Errorf("invalid actor: %w", err)
	}
	if err := validFoods(foods); err != nil {
		return recipe.Recipe{}, err
	}
	var stored recipe.Recipe
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		stored, err = saveRecipeInTx(ctx, tx, userID, a, r, foods, mode, now)
		return err
	})
	switch {
	case errors.Is(err, ErrNotFound) || refusedFood(err) || errors.Is(err, ErrInvalidRecipe) || errors.Is(err, ErrRecipeCycle):
		return recipe.Recipe{}, err
	case err != nil:
		if _, ok := errors.AsType[*FoodItemError](err); ok {
			return recipe.Recipe{}, err
		}
		return recipe.Recipe{}, fmt.Errorf("save recipe: %w", err)
	}
	return stored, nil
}

func saveRecipeInTx(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, r recipe.Recipe, foods []food.Food, mode saveMode,
	now time.Time,
) (recipe.Recipe, error) {
	nameKey := food.Key(r.Food.Name)
	target, exists, err := saveTarget(ctx, tx, userID, r.Food.ID, nameKey, "", mode)
	if err != nil {
		return recipe.Recipe{}, err
	}
	var before *recipe.Recipe
	if exists {
		had, err := recipeInTx(ctx, tx, userID, target)
		if errors.Is(err, ErrNotFound) {
			// A food of the name or id, but no recipe.
			return recipe.Recipe{}, conflict(ctx, tx, userID, ErrNameTaken, target.String(), "")
		}
		if err != nil {
			return recipe.Recipe{}, err
		}
		before = &had
	}
	var parts []uuid.UUID
	if r.Items, parts, err = ingredients(ctx, tx, userID, a, r.Items, foods, now); err != nil {
		return recipe.Recipe{}, err
	}
	r.Food.ID = target
	if err := checkCycle(ctx, tx, userID, target, r.Items); err != nil {
		return recipe.Recipe{}, err
	}
	made, err := recipe.Make(r)
	if err != nil {
		return recipe.Recipe{}, fmt.Errorf("%w: %w", ErrInvalidRecipe, err)
	}
	if err := errors.Join(checkAliases(ctx, tx, userID, made.Food), checkBarcodes(ctx, tx, userID, made.Food)); err != nil {
		return recipe.Recipe{}, err
	}
	if exists {
		err = updateFood(ctx, tx, userID, made.Food, nameKey, "", now)
	} else {
		err = insertFood(ctx, tx, userID, made.Food, nameKey, "", now)
	}
	if err != nil {
		return recipe.Recipe{}, err
	}
	if err := writeRecipe(ctx, tx, userID, made); err != nil {
		return recipe.Recipe{}, err
	}
	stored, err := recipeInTx(ctx, tx, userID, target)
	if err != nil {
		return recipe.Recipe{}, err
	}
	saved, err := record(ctx, tx, userID, a, event.RecipeSave, target.String(), before, stored, now)
	if err != nil {
		return recipe.Recipe{}, err
	}
	return stored, markParts(ctx, tx, userID, saved, parts)
}

// ingredients saves the new foods of foods that items name, as a meal's
// are saved, and returns items of the foods they are stored under, each
// worked out of its food as it is now (freshItems), with the events of
// the foods' writes.
func ingredients(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, items []meal.Item, foods []food.Food,
	now time.Time,
) ([]meal.Item, []uuid.UUID, error) {
	brought, err := saveNewFoods(ctx, tx, userID, a, foods, items, now)
	if err != nil {
		return nil, nil, err
	}
	if items, err = itemsOf(items, brought.ids, brought.kept); err != nil {
		return nil, nil, err
	}
	items, err = freshItems(ctx, tx, userID, items)
	return items, brought.parts, err
}

// freshItems returns items worked out of their foods as userID has them
// now, in the quantity each was given: a recipe saved takes its
// ingredients' values as they are then, whatever values the client sent.
// An ingredient of no food of the user's, or in a quantity its food no
// longer takes (a serving taken away), is ErrInvalidRecipe.
func freshItems(ctx context.Context, tx *sql.Tx, userID uuid.UUID, items []meal.Item) ([]meal.Item, error) {
	out := make([]meal.Item, len(items))
	for i := range items {
		f, err := foodInTx(ctx, tx, userID, items[i].FoodID)
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: ingredient %q: no such food", ErrInvalidRecipe, items[i].Name)
		}
		if err != nil {
			return nil, err
		}
		if out[i], err = meal.FromFood(f, items[i].Quantity, items[i].Note); err != nil {
			return nil, fmt.Errorf("%w: ingredient %w", ErrInvalidRecipe, err)
		}
	}
	return out, nil
}

// checkCycle refuses items of the recipe id when one of them is the
// recipe, or a recipe made of it, at any depth (ErrRecipeCycle).
func checkCycle(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID, items []meal.Item) error {
	for i := range items {
		if items[i].FoodID == id {
			return fmt.Errorf("%w: %q", ErrRecipeCycle, items[i].Name)
		}
		var one int
		err := tx.QueryRowContext(ctx, `WITH RECURSIVE reach (id) AS (
				SELECT ?
				UNION
				SELECT ri.food_id FROM recipe_items AS ri JOIN reach ON ri.recipe_id = reach.id WHERE ri.user_id = ?
			)
			SELECT 1 FROM reach WHERE id = ? LIMIT 1`,
			items[i].FoodID.String(), userID.String(), id.String()).Scan(&one)
		if err == nil {
			return fmt.Errorf("%w: %q", ErrRecipeCycle, items[i].Name)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

// writeRecipe writes r's food's servings, aliases and search row, and
// its recipe row and ingredients, in place of the ones before; its foods
// row is written.
func writeRecipe(ctx context.Context, tx *sql.Tx, userID uuid.UUID, r recipe.Recipe) error {
	if err := replaceFoodChildren(ctx, tx, userID, r.Food); err != nil {
		return err
	}
	id, uid := r.Food.ID.String(), userID.String()
	if _, err := tx.ExecContext(ctx, "DELETE FROM recipes WHERE food_id = ? AND user_id = ?", id, uid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO recipes (food_id, user_id, yield_g, portions, pot_g) VALUES (?, ?, ?, ?, ?)",
		id, uid, positive(r.Yield), positive(float64(r.Portions)), positive(r.Pot)); err != nil {
		return err
	}
	for i := range r.Items {
		it := &r.Items[i]
		n := it.Nutrients
		o := optionalNutrients(&n)
		q := it.Quantity
		if _, err := tx.ExecContext(ctx, `INSERT INTO recipe_items (recipe_id, user_id, position, food_id, name, brand,
			quantity_amount, quantity_unit, quantity_servings, quantity_serving, quantity_portions, amount, unit,
			kcal, protein, carbs, fat, fiber, saturated_fat, sugars, salt, polyols, alcohol, source, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, uid, i, it.FoodID.String(), it.Name, it.Brand,
			q.Amount, q.Unit, q.Servings, q.Serving, q.Portions, it.Amount, it.Unit,
			n.Kcal, n.Protein, n.Carbs, n.Fat, nullable(*o[0]), nullable(*o[1]), nullable(*o[2]), nullable(*o[3]),
			nullable(*o[4]), nullable(*o[5]), string(it.Source), it.Note); err != nil {
			return err
		}
	}
	return nil
}

// positive is v for a column that is NULL for none: NULL for 0.
func positive(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

// RecipeByID returns userID's recipe of the food with id, archived or
// not, or ErrNotFound: for a food that is no recipe too.
func (db *DB) RecipeByID(ctx context.Context, userID, id uuid.UUID) (recipe.Recipe, error) {
	var r recipe.Recipe
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		r, err = recipeInTx(ctx, tx, userID, id)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return recipe.Recipe{}, err
	}
	if err != nil {
		return recipe.Recipe{}, fmt.Errorf("recipe: %w", err)
	}
	return r, nil
}

// recipeInTx reads userID's recipe id in tx, its food and ingredients, or
// returns ErrNotFound.
func recipeInTx(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID) (recipe.Recipe, error) {
	var yield, pot sql.NullFloat64
	var portions sql.NullInt64
	err := tx.QueryRowContext(ctx, "SELECT yield_g, portions, pot_g FROM recipes WHERE user_id = ? AND food_id = ?",
		userID.String(), id.String()).Scan(&yield, &portions, &pot)
	if errors.Is(err, sql.ErrNoRows) {
		return recipe.Recipe{}, ErrNotFound
	}
	if err != nil {
		return recipe.Recipe{}, err
	}
	r := recipe.Recipe{Yield: yield.Float64, Portions: int(portions.Int64), Pot: pot.Float64}
	if r.Food, err = foodInTx(ctx, tx, userID, id); err != nil {
		return recipe.Recipe{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT food_id, name, brand, quantity_amount, quantity_unit, quantity_servings,
		quantity_serving, quantity_portions, amount, unit, kcal, protein, carbs, fat,
		fiber, saturated_fat, sugars, salt, polyols, alcohol, source, note
		FROM recipe_items WHERE user_id = ? AND recipe_id = ? ORDER BY position`, userID.String(), id.String())
	if err != nil {
		return recipe.Recipe{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var it meal.Item
		var foodID, source string
		var opt [6]sql.NullFloat64
		q, n := &it.Quantity, &it.Nutrients
		if err := rows.Scan(&foodID, &it.Name, &it.Brand, &q.Amount, &q.Unit, &q.Servings, &q.Serving, &q.Portions,
			&it.Amount, &it.Unit, &n.Kcal, &n.Protein, &n.Carbs, &n.Fat,
			&opt[0], &opt[1], &opt[2], &opt[3], &opt[4], &opt[5], &source, &it.Note); err != nil {
			return recipe.Recipe{}, err
		}
		if it.FoodID, err = uuid.Parse(foodID); err != nil {
			return recipe.Recipe{}, fmt.Errorf("ingredient food id %q: %w", foodID, err)
		}
		for i, p := range optionalNutrients(n) {
			if opt[i].Valid {
				v := opt[i].Float64
				*p = &v
			}
		}
		it.Source = food.Source(source)
		r.Items = append(r.Items, it)
	}
	return r, rows.Err()
}

// RecipesOf returns the recipes of userID's foods among ids, by id, with
// their food and ingredients: the foods of a search or a list that are
// recipes.
func (db *DB) RecipesOf(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]recipe.Recipe, error) {
	out := map[uuid.UUID]recipe.Recipe{}
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			r, err := recipeInTx(ctx, tx, userID, id)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			out[id] = r
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("recipes: %w", err)
	}
	return out, nil
}

// checkRecipeFood refuses a save of food f, which was before (nil for a
// new one), when it is or makes a recipe's food: only the recipe changes
// that, but for barcodes added to it (linkBarcodes) and whether it is
// archived, which leave its values as they are.
func checkRecipeFood(before *food.Food, f food.Food) error {
	switch {
	case before == nil && f.Source == food.Recipe:
		return ErrRecipeFood
	case before == nil || (before.Source != food.Recipe && f.Source != food.Recipe):
		return nil
	}
	had, now := *before, f
	had.Barcodes, now.Barcodes, had.Archived, now.Archived = nil, nil, false, false
	if !reflect.DeepEqual(had, now) {
		return ErrRecipeFood
	}
	return nil
}

// foodInUse reports whether a meal's item or a recipe's ingredient is of
// userID's food id, which then stays, archived, when undo would take it
// away.
func foodInUse(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID) (bool, error) {
	var used int
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM meal_items WHERE user_id = ?1 AND food_id = ?2)
		OR EXISTS (SELECT 1 FROM recipe_items WHERE user_id = ?1 AND food_id = ?2)`,
		userID.String(), id.String()).Scan(&used)
	return used != 0, err
}

// takeBackRecipe puts the recipe of e back as it was before: its food
// and ingredients, or, for one e added, none: its food goes, unless a
// meal or another recipe uses it, which keeps it, archived, with its
// ingredients.
func takeBackRecipe(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	id, err := entityID(e)
	if err != nil {
		return err
	}
	if e.Before == nil {
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
	var r recipe.Recipe
	if err := decode(e.Before, &r); err != nil {
		return err
	}
	if err := checkFoodClash(ctx, tx, userID, r.Food); err != nil {
		return err
	}
	// An ingredient may be made of the recipe since (an import's save,
	// which undo leaves).
	if err := checkCycle(ctx, tx, userID, r.Food.ID, r.Items); errors.Is(err, ErrRecipeCycle) {
		return fmt.Errorf("%w: %w", clash{UndoStuck}, err)
	} else if err != nil {
		return err
	}
	if err := updateFood(ctx, tx, userID, r.Food, food.Key(r.Food.Name), food.Key(r.Food.Brand), now); err != nil {
		return err
	}
	return writeRecipe(ctx, tx, userID, r)
}

// recipeStateNow returns the recipe of e now, as e records it, or nil
// when it is gone.
func recipeStateNow(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event) (any, error) {
	id, err := entityID(e)
	if err != nil {
		return nil, err
	}
	r, err := recipeInTx(ctx, tx, userID, id)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}
