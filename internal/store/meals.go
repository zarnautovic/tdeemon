package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// ErrMealConflict reports a meal created under the id of another of the
// user's meals (not a retry of it), or an item under the id of an item of
// another of the user's meals.
var ErrMealConflict = errors.New("a meal with this id holds something else")

// ErrItemTaken reports an item under the id of another of the user's
// meals' items. It is an ErrMealConflict too.
var ErrItemTaken = errors.New("another meal's item has this id")

// ErrMealDeleted reports a meal created under the id of one of the
// user's meals that was deleted (or its create undone): a late retry of
// its create does not bring it back.
var ErrMealDeleted = errors.New("the meal under this id was deleted")

const mealColumns = "m.id, m.eaten_at, m.day, m.description, m.client, m.note"

// The meals of a user that queryMeals reads: by id, by day, by the time
// eaten.
const (
	mealsSelect  = "SELECT " + mealColumns + " FROM meals AS m WHERE m.user_id = ? AND "
	mealsOrder   = " ORDER BY m.eaten_at, m.id"
	mealByID     = mealsSelect + "m.id = ?" + mealsOrder
	mealsOfDay   = mealsSelect + "m.day = ?" + mealsOrder
	mealsBetween = mealsSelect + "m.eaten_at BETWEEN ? AND ?" + mealsOrder
)

const itemColumns = `i.id, i.food_id, i.name, i.quantity_amount, i.quantity_unit, i.quantity_servings,
	i.quantity_serving, i.quantity_portions, i.amount, i.unit, i.kcal, i.protein, i.carbs, i.fat,
	i.fiber, i.saturated_fat, i.sugars, i.salt, i.polyols, i.alcohol, i.source, i.note, i.sent_food`

// CreateMeal stores m, a new meal of userID's, and returns it as stored,
// each item under the id it carries (a client's UUIDv7) or a new one.
// Creating is idempotent on m's id: the same meal sent again (the same
// meal.Meal.Fingerprint as when it was created) returns the meal as it is
// now, edits included, and false, and nothing changes; another meal under
// the id is ErrMealConflict, and an id whose meal was deleted is
// ErrMealDeleted. Ids are per user: another user's meal under the id is
// no meal of this user's. The time eaten is kept to the second. A meal
// created is recorded as a's event.
func (db *DB) CreateMeal(ctx context.Context, userID uuid.UUID, a event.Actor, m meal.Meal, now time.Time) (meal.Meal, bool, error) {
	return db.CreateMealWithFoods(ctx, userID, a, m, nil, now)
}

// CreateMealWithFoods is CreateMeal of a meal that brings new foods: each
// is saved as SaveFood saves it, in the meal's own transaction, and an
// item naming a food's id takes the id the food is stored under (a food
// of the same name and brand as one the user has is that one); a food no
// item names is not saved. A meal refused saves none of them; one already
// under m's id (a retry, or another meal) saves none either.
func (db *DB) CreateMealWithFoods(ctx context.Context, userID uuid.UUID, a event.Actor, m meal.Meal, foods []food.Food, now time.Time) (meal.Meal, bool, error) {
	if err := m.Validate(); err != nil {
		return meal.Meal{}, false, fmt.Errorf("invalid meal: %w", err)
	}
	if err := a.Validate(); err != nil {
		return meal.Meal{}, false, fmt.Errorf("invalid actor: %w", err)
	}
	if err := validFoods(foods); err != nil {
		return meal.Meal{}, false, err
	}
	created := false
	var stored meal.Meal
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var fingerprint string
		err := tx.QueryRowContext(ctx, "SELECT fingerprint FROM meals WHERE user_id = ? AND id = ?",
			userID.String(), m.ID.String()).Scan(&fingerprint)
		switch {
		case err == nil && fingerprint != m.Fingerprint():
			return ErrMealConflict
		case err == nil:
		case !errors.Is(err, sql.ErrNoRows):
			return err
		default:
			created = true
			stored, err = insertNewMeal(ctx, tx, userID, a, m, foods, now)
			return err
		}
		stored, err = mealInTx(ctx, tx, userID, m.ID)
		return err
	})
	if errors.Is(err, ErrMealConflict) || errors.Is(err, ErrMealDeleted) || errors.Is(err, ErrNameTaken) || errors.Is(err, ErrAliasTaken) {
		return meal.Meal{}, false, err
	}
	if err != nil {
		return meal.Meal{}, false, fmt.Errorf("create meal: %w", err)
	}
	return stored, created, nil
}

// insertNewMeal inserts m, a meal under an id no meal has had, with the
// new foods it brings, and records it as a's event.
func insertNewMeal(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, m meal.Meal, foods []food.Food, now time.Time) (meal.Meal, error) {
	if err := checkNotDeleted(ctx, tx, userID, m.ID); err != nil {
		return meal.Meal{}, err
	}
	ids, kept, err := saveNewFoods(ctx, tx, userID, a, foods, m.Items, now)
	if err != nil {
		return meal.Meal{}, err
	}
	if m.Items, err = itemsOf(m.Items, ids, kept); err != nil {
		return meal.Meal{}, err
	}
	// The time stored, to the second, so the state recorded is the state
	// read back.
	s := mealState{Meal: m, Fingerprint: m.Fingerprint(), CreatedAt: now.UTC().Truncate(time.Second)}
	if err := insertMeal(ctx, tx, userID, s, nil, now); err != nil {
		return meal.Meal{}, err
	}
	if s.Meal, err = mealInTx(ctx, tx, userID, m.ID); err != nil {
		return meal.Meal{}, err
	}
	_, err = record(ctx, tx, userID, a, event.MealCreate, m.ID.String(), nil, s, now)
	return s.Meal, err
}

// mealState is a meal as an event records it: with the fingerprint and
// the time of its create, so a meal put back answers retries as before.
type mealState struct {
	Meal        meal.Meal
	Fingerprint string
	CreatedAt   time.Time
}

// stateOf returns the state of userID's meal id in tx.
func stateOf(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID) (mealState, error) {
	m, err := mealInTx(ctx, tx, userID, id)
	if err != nil {
		return mealState{}, err
	}
	s := mealState{Meal: m}
	var created string
	if err := tx.QueryRowContext(ctx, "SELECT fingerprint, created_at FROM meals WHERE user_id = ? AND id = ?",
		userID.String(), id.String()).Scan(&s.Fingerprint, &created); err != nil {
		return mealState{}, err
	}
	s.CreatedAt, err = parseTime(created)
	return s, err
}

// checkNotDeleted returns ErrMealDeleted when userID's events show a meal
// under id that is no longer there: the tombstone of a delete, or of an
// undone create.
func checkNotDeleted(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM events
		WHERE user_id = ? AND entity = ? AND operation IN ('meal.create', 'meal.delete')`,
		userID.String(), id.String()).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrMealDeleted
	}
	return nil
}

// insertMeal writes the meal of s, with its fingerprint and the time of
// its create, and its items; an item keeps its id when kept holds it.
func insertMeal(ctx context.Context, tx *sql.Tx, userID uuid.UUID, s mealState, kept map[uuid.UUID]bool, now time.Time) error {
	m := s.Meal
	if _, err := tx.ExecContext(ctx, `INSERT INTO meals
		(id, user_id, eaten_at, day, description, client, note, fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID.String(), userID.String(), formatTime(m.EatenAt), m.Day.String(), m.Description, string(m.Client),
		m.Note, s.Fingerprint, formatTime(s.CreatedAt), formatTime(now)); err != nil {
		return err
	}
	return insertItems(ctx, tx, userID, m, kept)
}

// takeBackMeal puts the meal of e back as it was: a meal e created goes
// (and its id stays a tombstone), an edited one is as before, a deleted
// one is back with its fingerprint, the time of its create and its item
// ids.
func takeBackMeal(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	if e.Action == event.MealCreate {
		_, err := tx.ExecContext(ctx, "DELETE FROM meals WHERE user_id = ? AND id = ?", userID.String(), e.Entity)
		return err
	}
	var s mealState
	if err := decode(e.Before, &s); err != nil {
		return err
	}
	if e.Action == event.MealReplace {
		if _, err := tx.ExecContext(ctx, "DELETE FROM meals WHERE user_id = ? AND id = ?", userID.String(), e.Entity); err != nil {
			return err
		}
	}
	// The meal's rows are gone, so its items' ids are free again and the
	// items keep them, unless another meal took one since.
	err := insertMeal(ctx, tx, userID, s, nil, now)
	if errors.Is(err, ErrMealConflict) {
		return fmt.Errorf("%w: %w", clash{UndoItemTaken}, err)
	}
	return err
}

// mealInTx reads userID's meal id, with its items, in tx.
func mealInTx(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID) (meal.Meal, error) {
	ms, err := scanMeals(ctx, tx, mealByID, userID.String(), id.String())
	if err != nil {
		return meal.Meal{}, err
	}
	if len(ms) == 0 {
		return meal.Meal{}, ErrNotFound
	}
	ms[0].Items, err = mealItems(ctx, tx, userID, id)
	return ms[0], err
}

// ReplaceMeal stores m in place of userID's meal with m's id (an edit:
// another time, items added, changed or removed) and returns it as
// stored. Items keep the ids they carry: one of the meal's, or a client's
// for a new item (an id of another of the user's meals' items is
// ErrMealConflict); an item without one gets a new id. The client the
// meal was logged through and its fingerprint stay: a late retry of the
// create returns the edited meal. A meal the user does not have is
// ErrNotFound. The edit is recorded as a's event, with the meal before
// and after.
func (db *DB) ReplaceMeal(ctx context.Context, userID uuid.UUID, a event.Actor, m meal.Meal, now time.Time) (meal.Meal, error) {
	if err := m.Validate(); err != nil {
		return meal.Meal{}, fmt.Errorf("invalid meal: %w", err)
	}
	if err := a.Validate(); err != nil {
		return meal.Meal{}, fmt.Errorf("invalid actor: %w", err)
	}
	var stored meal.Meal
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := stateOf(ctx, tx, userID, m.ID)
		if err != nil {
			return err
		}
		stored, err = replaceInTx(ctx, tx, userID, a, before, m, now)
		return err
	})
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrMealConflict) {
		return meal.Meal{}, err
	}
	if err != nil {
		return meal.Meal{}, fmt.Errorf("replace meal: %w", err)
	}
	return stored, nil
}

// UpdateMeal stores in place of userID's meal id what change makes of it,
// as ReplaceMeal does, and returns it as stored. change gets the meal as
// it is in the transaction that stores its answer, so changes made at
// once (an agent's calls in parallel) are each made to the other's
// result and none is lost. The foods the result names of foods are saved
// with it, as CreateMealWithFoods saves them. An error of change stores
// nothing, and is returned as it is; a meal the user does not have is
// ErrNotFound.
func (db *DB) UpdateMeal(ctx context.Context, userID uuid.UUID, a event.Actor, id uuid.UUID, foods []food.Food,
	change func(meal.Meal) (meal.Meal, error), now time.Time,
) (meal.Meal, error) {
	if err := a.Validate(); err != nil {
		return meal.Meal{}, fmt.Errorf("invalid actor: %w", err)
	}
	if err := validFoods(foods); err != nil {
		return meal.Meal{}, err
	}
	var stored meal.Meal
	var changeErr error
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := stateOf(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		// A copy: the meal before goes into the event, as it was.
		cur := before.Meal
		cur.Items = slices.Clone(cur.Items)
		m, err := change(cur)
		if err != nil {
			changeErr = err
			return err
		}
		ids, kept, err := saveNewFoods(ctx, tx, userID, a, foods, m.Items, now)
		if err != nil {
			return err
		}
		if m.Items, err = itemsOf(m.Items, ids, kept); err != nil {
			return err
		}
		m.ID = id
		if err := m.Validate(); err != nil {
			changeErr = fmt.Errorf("invalid meal: %w", err)
			return changeErr
		}
		stored, err = replaceInTx(ctx, tx, userID, a, before, m, now)
		return err
	})
	switch {
	case changeErr != nil:
		return meal.Meal{}, changeErr
	case errors.Is(err, ErrNotFound) || errors.Is(err, ErrMealConflict) || errors.Is(err, ErrNameTaken) || errors.Is(err, ErrAliasTaken):
		return meal.Meal{}, err
	case err != nil:
		return meal.Meal{}, fmt.Errorf("update meal: %w", err)
	}
	return stored, nil
}

// itemsOf returns items with each food id that ids holds as the id the
// food is stored under; an item of a food kept, one the user had, is
// worked out of it as the user has it, or is a *FoodItemError.
func itemsOf(items []meal.Item, ids map[uuid.UUID]uuid.UUID, kept map[uuid.UUID]food.Food) ([]meal.Item, error) {
	out := slices.Clone(items)
	for i := range out {
		id, ok := ids[out[i].FoodID]
		if !ok {
			continue
		}
		out[i].FoodID = id
		f, had := kept[id]
		if !had {
			continue
		}
		it, err := meal.FromFood(f, out[i].Quantity, out[i].Note)
		if err != nil {
			return nil, &FoodItemError{Food: f, Err: err}
		}
		it.ID, it.SentFood = out[i].ID, out[i].SentFood
		out[i] = it
	}
	return out, nil
}

// FoodItemError reports an item of a new food that is the user's Food,
// by its name and brand, which the item's quantity does not fit.
type FoodItemError struct {
	Food food.Food
	Err  error
}

func (e *FoodItemError) Error() string {
	return fmt.Sprintf("the user's food %q, which a new food of its name is: %v", e.Food.Name, e.Err)
}

func (e *FoodItemError) Unwrap() error { return e.Err }

// replaceInTx writes m in place of the meal of before, and records it as
// a's event.
func replaceInTx(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, before mealState, m meal.Meal, now time.Time) (meal.Meal, error) {
	{
		res, err := tx.ExecContext(ctx, `UPDATE meals SET eaten_at = ?, day = ?, description = ?, note = ?,
			updated_at = ? WHERE id = ? AND user_id = ?`,
			formatTime(m.EatenAt), m.Day.String(), m.Description, m.Note, formatTime(now), m.ID.String(), userID.String())
		if err != nil {
			return meal.Meal{}, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return meal.Meal{}, err
		} else if n == 0 {
			return meal.Meal{}, ErrNotFound
		}
	}
	kept, err := itemIDs(ctx, tx, userID, m.ID)
	if err != nil {
		return meal.Meal{}, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM meal_items WHERE meal_id = ? AND user_id = ?",
		m.ID.String(), userID.String()); err != nil {
		return meal.Meal{}, err
	}
	if err := insertItems(ctx, tx, userID, m, kept); err != nil {
		return meal.Meal{}, err
	}
	after, err := stateOf(ctx, tx, userID, m.ID)
	if err != nil {
		return meal.Meal{}, err
	}
	_, err = record(ctx, tx, userID, a, event.MealReplace, m.ID.String(), before, after, now)
	return after.Meal, err
}

// RetryMeal returns userID's meal with id when fingerprint is the
// meal.Meal.Fingerprint it was created with: a create sent again, told
// before anything of it is worked out, so nothing of it is read or saved
// again. Another meal under the id is ErrMealConflict, an id whose meal
// was deleted (or its create undone) ErrMealDeleted, and an id no meal of
// the user's has had ErrNotFound.
func (db *DB) RetryMeal(ctx context.Context, userID, id uuid.UUID, fingerprint string) (meal.Meal, error) {
	var stored meal.Meal
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var fp string
		err := tx.QueryRowContext(ctx, "SELECT fingerprint FROM meals WHERE user_id = ? AND id = ?",
			userID.String(), id.String()).Scan(&fp)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if err := checkNotDeleted(ctx, tx, userID, id); err != nil {
				return err
			}
			return ErrNotFound
		case err != nil:
			return err
		case fp != fingerprint:
			return ErrMealConflict
		}
		stored, err = mealInTx(ctx, tx, userID, id)
		return err
	})
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrMealConflict) || errors.Is(err, ErrMealDeleted) {
		return meal.Meal{}, err
	}
	if err != nil {
		return meal.Meal{}, fmt.Errorf("retry meal: %w", err)
	}
	return stored, nil
}

// itemIDs returns the ids of the items of userID's meal mealID.
func itemIDs(ctx context.Context, tx *sql.Tx, userID, mealID uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM meal_items WHERE meal_id = ? AND user_id = ?", mealID.String(), userID.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := map[uuid.UUID]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("item id %q: %w", s, err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// insertItems writes m's items, in order. An item keeps the id it
// carries when kept holds it (the meal had it before an edit) or no other
// of the user's items has it (a client's id for a new item, so a retried
// "add" adds once); an id of another meal's item is ErrMealConflict, and
// an item without an id gets a new one. The foods the items name must be
// userID's: the foreign key on the food and the user refuses any other.
func insertItems(ctx context.Context, tx *sql.Tx, userID uuid.UUID, m meal.Meal, kept map[uuid.UUID]bool) error {
	for i := range m.Items {
		it := &m.Items[i]
		id := it.ID
		switch {
		case id == uuid.Nil():
			id = uuid.NewV7()
		case kept[id]:
		default:
			var other string
			err := tx.QueryRowContext(ctx, "SELECT meal_id FROM meal_items WHERE user_id = ? AND id = ?",
				userID.String(), id.String()).Scan(&other)
			if err == nil {
				return fmt.Errorf("%w: %w: item %s", ErrMealConflict, ErrItemTaken, id)
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var foodID, sentFood any
		if it.FoodID != uuid.Nil() {
			foodID = it.FoodID.String()
		}
		if it.SentFood != "" {
			sentFood = it.SentFood
		}
		n := it.Nutrients
		o := optionalNutrients(&n)
		q := it.Quantity
		if _, err := tx.ExecContext(ctx, `INSERT INTO meal_items (id, meal_id, user_id, position, food_id, name,
			quantity_amount, quantity_unit, quantity_servings, quantity_serving, quantity_portions, amount, unit,
			kcal, protein, carbs, fat, fiber, saturated_fat, sugars, salt, polyols, alcohol, source, note, sent_food)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.String(), m.ID.String(), userID.String(), i, foodID, it.Name,
			q.Amount, q.Unit, q.Servings, q.Serving, q.Portions, it.Amount, it.Unit,
			n.Kcal, n.Protein, n.Carbs, n.Fat, nullable(*o[0]), nullable(*o[1]), nullable(*o[2]), nullable(*o[3]),
			nullable(*o[4]), nullable(*o[5]), string(it.Source), it.Note, sentFood); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMeal removes userID's meal with id and its items, or returns
// ErrNotFound. It is recorded as a's event, with the meal before: undo
// puts it back, and the event is the tombstone that keeps a late retry
// of its create from bringing it back (ErrMealDeleted).
func (db *DB) DeleteMeal(ctx context.Context, userID uuid.UUID, a event.Actor, id uuid.UUID, now time.Time) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := stateOf(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM meals WHERE id = ? AND user_id = ?", id.String(), userID.String()); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.MealDelete, id.String(), before, nil, now)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("delete meal: %w", err)
	}
	return nil
}

// MealByID returns userID's meal with id, with its items in order, or
// ErrNotFound.
func (db *DB) MealByID(ctx context.Context, userID, id uuid.UUID) (meal.Meal, error) {
	ms, err := db.queryMeals(ctx, userID, mealByID, id.String())
	if err != nil {
		return meal.Meal{}, fmt.Errorf("meal: %w", err)
	}
	if len(ms) == 0 {
		return meal.Meal{}, ErrNotFound
	}
	return ms[0], nil
}

// MealsOfDay returns userID's meals of logical day d, in the order eaten.
func (db *DB) MealsOfDay(ctx context.Context, userID uuid.UUID, d day.Date) ([]meal.Meal, error) {
	ms, err := db.queryMeals(ctx, userID, mealsOfDay, d.String())
	if err != nil {
		return nil, fmt.Errorf("meals of the day: %w", err)
	}
	return ms, nil
}

// MealsBetween returns userID's meals eaten from from to to, both
// included, in the order eaten: the meals a new one may repeat
// (meal.DuplicateOf).
func (db *DB) MealsBetween(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]meal.Meal, error) {
	ms, err := db.queryMeals(ctx, userID, mealsBetween, formatTime(from), formatTime(to))
	if err != nil {
		return nil, fmt.Errorf("meals between: %w", err)
	}
	return ms, nil
}

// Intake is a day's meals and the kcal and protein of their items.
type Intake struct {
	Meals         int
	Kcal, Protein float64
}

// IntakeOfDays returns userID's intake on each day from from to to, both
// included, that has a meal: by the day each meal is of, as MealsOfDay
// reads it, summed in one query.
func (db *DB) IntakeOfDays(ctx context.Context, userID uuid.UUID, from, to day.Date) (map[day.Date]Intake, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT m.day, COUNT(DISTINCT m.id), COALESCE(SUM(i.kcal), 0), COALESCE(SUM(i.protein), 0)
		FROM meals AS m LEFT JOIN meal_items AS i ON i.meal_id = m.id AND i.user_id = m.user_id
		WHERE m.user_id = ? AND m.day BETWEEN ? AND ? GROUP BY m.day ORDER BY m.day`,
		userID.String(), from.String(), to.String())
	if err != nil {
		return nil, fmt.Errorf("intake of days: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[day.Date]Intake{}
	for rows.Next() {
		var d string
		var in Intake
		if err := rows.Scan(&d, &in.Meals, &in.Kcal, &in.Protein); err != nil {
			return nil, fmt.Errorf("intake of days: %w", err)
		}
		date, err := day.ParseDate(d)
		if err != nil {
			return nil, fmt.Errorf("intake of days: %w", err)
		}
		out[date] = in
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("intake of days: %w", err)
	}
	return out, nil
}

// FirstMealDay returns the day of userID's first meal, and false for no
// meal.
func (db *DB) FirstMealDay(ctx context.Context, userID uuid.UUID) (day.Date, bool, error) {
	var d sql.NullString
	if err := db.read.QueryRowContext(ctx, "SELECT MIN(day) FROM meals WHERE user_id = ?", userID.String()).Scan(&d); err != nil {
		return day.Date{}, false, fmt.Errorf("first meal day: %w", err)
	}
	if !d.Valid {
		return day.Date{}, false, nil
	}
	date, err := day.ParseDate(d.String)
	if err != nil {
		return day.Date{}, false, fmt.Errorf("first meal day: %w", err)
	}
	return date, true, nil
}

// queryMeals returns userID's meals that q (mealByID, mealsOfDay or
// mealsBetween) selects with args, with their items, read in one
// snapshot.
func (db *DB) queryMeals(ctx context.Context, userID uuid.UUID, q string, args ...any) ([]meal.Meal, error) {
	var out []meal.Meal
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		if out, err = scanMeals(ctx, tx, q, append([]any{userID.String()}, args...)...); err != nil {
			return err
		}
		for i := range out {
			if out[i].Items, err = mealItems(ctx, tx, userID, out[i].ID); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func scanMeals(ctx context.Context, tx *sql.Tx, q string, args ...any) ([]meal.Meal, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []meal.Meal
	for rows.Next() {
		m, err := scanMeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanMeal(rows *sql.Rows) (meal.Meal, error) {
	var id, eatenAt, d, client string
	var m meal.Meal
	if err := rows.Scan(&id, &eatenAt, &d, &m.Description, &client, &m.Note); err != nil {
		return meal.Meal{}, err
	}
	var err error
	if m.ID, err = uuid.Parse(id); err != nil {
		return meal.Meal{}, fmt.Errorf("meal id %q: %w", id, err)
	}
	if m.EatenAt, err = parseTime(eatenAt); err != nil {
		return meal.Meal{}, err
	}
	if m.Day, err = day.ParseDate(d); err != nil {
		return meal.Meal{}, err
	}
	m.Client = meal.Client(client)
	return m, nil
}

// mealItems returns the items of userID's meal mealID, in order.
func mealItems(ctx context.Context, tx *sql.Tx, userID, mealID uuid.UUID) ([]meal.Item, error) {
	rows, err := tx.QueryContext(ctx, "SELECT "+itemColumns+
		" FROM meal_items AS i WHERE i.meal_id = ? AND i.user_id = ? ORDER BY i.position", mealID.String(), userID.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []meal.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func scanItem(rows *sql.Rows) (meal.Item, error) {
	var id, source string
	var foodID, sentFood sql.NullString
	var it meal.Item
	var opt [6]sql.NullFloat64
	q, n := &it.Quantity, &it.Nutrients
	if err := rows.Scan(&id, &foodID, &it.Name, &q.Amount, &q.Unit, &q.Servings, &q.Serving, &q.Portions,
		&it.Amount, &it.Unit, &n.Kcal, &n.Protein, &n.Carbs, &n.Fat,
		&opt[0], &opt[1], &opt[2], &opt[3], &opt[4], &opt[5], &source, &it.Note, &sentFood); err != nil {
		return meal.Item{}, err
	}
	var err error
	if it.ID, err = uuid.Parse(id); err != nil {
		return meal.Item{}, fmt.Errorf("item id %q: %w", id, err)
	}
	if foodID.Valid {
		if it.FoodID, err = uuid.Parse(foodID.String); err != nil {
			return meal.Item{}, fmt.Errorf("item food id %q: %w", foodID.String, err)
		}
	}
	for i, p := range optionalNutrients(n) {
		if opt[i].Valid {
			v := opt[i].Float64
			*p = &v
		}
	}
	it.Source = food.Source(source)
	it.SentFood = sentFood.String
	return it, nil
}

// LastUse is the last time a food was eaten, and how much.
type LastUse struct {
	At       time.Time
	MealID   uuid.UUID
	Quantity meal.Quantity
	// Amount and Unit are the quantity in the food's unit.
	Amount float64
	Unit   string
}

// LastUses returns, for each of userID's foods in ids that a meal has
// used, the last item that used it: what search offers as the usual
// amount. At most MaxFoodResults ids are looked up.
func (db *DB) LastUses(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]LastUse, error) {
	out := map[uuid.UUID]LastUse{}
	for _, id := range ids[:min(len(ids), MaxFoodResults)] {
		var u LastUse
		var at, mealID string
		q := &u.Quantity
		err := db.read.QueryRowContext(ctx, `
			SELECT m.eaten_at, m.id, i.quantity_amount, i.quantity_unit, i.quantity_servings, i.quantity_serving,
				i.quantity_portions, i.amount, i.unit
			FROM meal_items AS i JOIN meals AS m ON m.id = i.meal_id AND m.user_id = i.user_id
			WHERE i.user_id = ? AND i.food_id = ?
			ORDER BY m.eaten_at DESC, m.id DESC, i.position DESC LIMIT 1`, userID.String(), id.String()).
			Scan(&at, &mealID, &q.Amount, &q.Unit, &q.Servings, &q.Serving, &q.Portions, &u.Amount, &u.Unit)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("last uses: %w", err)
		}
		if u.At, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("last uses: %w", err)
		}
		if u.MealID, err = uuid.Parse(mealID); err != nil {
			return nil, fmt.Errorf("last uses: meal id %q: %w", mealID, err)
		}
		out[id] = u
	}
	return out, nil
}

// ExcludeDay takes userID's logical day d out of calibration, for e's
// reason, or changes the reason of a day already out. It is recorded as
// a's event.
func (db *DB) ExcludeDay(ctx context.Context, userID uuid.UUID, a event.Actor, d day.Date, e day.Exclusion, now time.Time) error {
	if err := day.ValidateExclusion(e); err != nil {
		return fmt.Errorf("invalid exclusion: %w", err)
	}
	if d.IsZero() {
		return errors.New("invalid exclusion: the day is missing")
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := exclusionOf(ctx, tx, userID, d)
		if err != nil {
			return err
		}
		if err := setExclusion(ctx, tx, userID, d, &e.Reason, now); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.DayExclude, d.String(), before, exclusionState{&e.Reason}, now)
		return err
	})
	if err != nil {
		return fmt.Errorf("exclude day: %w", err)
	}
	return nil
}

// IncludeDay brings userID's logical day d back into calibration, as a's
// event. A day not excluded stays as it is, and nothing is recorded.
func (db *DB) IncludeDay(ctx context.Context, userID uuid.UUID, a event.Actor, d day.Date, now time.Time) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := exclusionOf(ctx, tx, userID, d)
		if err != nil || before.Reason == nil {
			return err
		}
		if err := setExclusion(ctx, tx, userID, d, nil, now); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.DayInclude, d.String(), before, exclusionState{}, now)
		return err
	})
	if err != nil {
		return fmt.Errorf("include day: %w", err)
	}
	return nil
}

// DayExclusionRecorded reports whether userID's day d was ever excluded,
// by anyone, whatever came after (an inclusion, an undo): an import that
// excludes days leaves a day decided before as it is. A day is only ever
// included after it was excluded. A check-in that excluded it counts.
func (db *DB) DayExclusionRecorded(ctx context.Context, userID uuid.UUID, d day.Date) (bool, error) {
	var n int
	err := db.read.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE user_id = ? AND entity = ?
		AND (operation = ? OR operation = ? AND json_extract(after_state, '$.excluded_reason') IS NOT NULL)`,
		userID.String(), d.String(), string(event.DayExclude), string(event.DayCheckIn)).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("day exclusion recorded: %w", err)
	}
	return n > 0, nil
}

// exclusionState is a day event's state: the reason the day is out of
// calibration, or nil when it is in.
type exclusionState struct{ Reason *string }

func exclusionOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date) (exclusionState, error) {
	var reason sql.NullString
	err := tx.QueryRowContext(ctx, "SELECT excluded_reason FROM days WHERE user_id = ? AND day = ?",
		userID.String(), d.String()).Scan(&reason)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return exclusionState{}, err
	}
	if reason.Valid {
		return exclusionState{&reason.String}, nil
	}
	return exclusionState{}, nil
}

// setExclusion stores reason (nil: none) as userID's day d's.
func setExclusion(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date, reason *string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO days (user_id, day, excluded_reason, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, day) DO UPDATE SET excluded_reason = excluded.excluded_reason, updated_at = excluded.updated_at`,
		userID.String(), d.String(), reason, formatTime(now))
	return err
}

// takeBackDay puts the day of e back in or out of calibration as before.
func takeBackDay(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	d, err := entityDay(e.Entity)
	if err != nil {
		return err
	}
	var s exclusionState
	if e.Before != nil {
		if err := decode(e.Before, &s); err != nil {
			return err
		}
	}
	return setExclusion(ctx, tx, userID, d, s.Reason, now)
}

// Exclusions returns userID's excluded days from from to to, both
// included, with their reasons.
func (db *DB) Exclusions(ctx context.Context, userID uuid.UUID, from, to day.Date) (map[day.Date]day.Exclusion, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT day, excluded_reason FROM days
		WHERE user_id = ? AND day BETWEEN ? AND ? AND excluded_reason IS NOT NULL ORDER BY day`,
		userID.String(), from.String(), to.String())
	if err != nil {
		return nil, fmt.Errorf("exclusions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[day.Date]day.Exclusion{}
	for rows.Next() {
		var d, reason string
		if err := rows.Scan(&d, &reason); err != nil {
			return nil, fmt.Errorf("exclusions: %w", err)
		}
		date, err := day.ParseDate(d)
		if err != nil {
			return nil, fmt.Errorf("exclusions: %w", err)
		}
		out[date] = day.Exclusion{Reason: reason}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("exclusions: %w", err)
	}
	return out, nil
}
