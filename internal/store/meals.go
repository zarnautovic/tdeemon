package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// ErrMealConflict reports a meal created under the id of another of the
// user's meals: not a retry of it.
var ErrMealConflict = errors.New("a meal with this id holds something else")

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
	i.fiber, i.saturated_fat, i.sugars, i.salt, i.polyols, i.alcohol, i.source, i.note`

// CreateMeal stores m, a new meal of userID's, and returns it as stored,
// each item with a new id. Creating is idempotent on m's id: the same
// meal sent again (the same meal.Meal.Fingerprint) returns the stored
// one and false, and nothing changes; another meal under the id is
// ErrMealConflict. Ids are per user: another user's meal under the id is
// no meal of this user's. The time eaten is kept to the second.
func (db *DB) CreateMeal(ctx context.Context, userID uuid.UUID, m meal.Meal, now time.Time) (meal.Meal, bool, error) {
	if err := m.Validate(); err != nil {
		return meal.Meal{}, false, fmt.Errorf("invalid meal: %w", err)
	}
	created := false
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var fingerprint string
		err := tx.QueryRowContext(ctx, "SELECT fingerprint FROM meals WHERE user_id = ? AND id = ?",
			userID.String(), m.ID.String()).Scan(&fingerprint)
		switch {
		case err == nil && fingerprint != m.Fingerprint():
			return ErrMealConflict
		case err == nil:
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		created = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO meals
			(id, user_id, eaten_at, day, description, client, note, fingerprint, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID.String(), userID.String(), formatTime(m.EatenAt), m.Day.String(), m.Description, string(m.Client),
			m.Note, m.Fingerprint(), formatTime(now), formatTime(now)); err != nil {
			return err
		}
		return insertItems(ctx, tx, userID, m, nil)
	})
	if errors.Is(err, ErrMealConflict) {
		return meal.Meal{}, false, err
	}
	if err != nil {
		return meal.Meal{}, false, fmt.Errorf("create meal: %w", err)
	}
	stored, err := db.MealByID(ctx, userID, m.ID)
	return stored, created, err
}

// ReplaceMeal stores m in place of userID's meal with m's id (an edit:
// another time, items added, changed or removed) and returns it as
// stored. An item that carries the id of one of the meal's items keeps
// it; any other item gets a new id. A meal the user does not have is
// ErrNotFound.
func (db *DB) ReplaceMeal(ctx context.Context, userID uuid.UUID, m meal.Meal, now time.Time) (meal.Meal, error) {
	if err := m.Validate(); err != nil {
		return meal.Meal{}, fmt.Errorf("invalid meal: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE meals SET eaten_at = ?, day = ?, description = ?, client = ?, note = ?,
			fingerprint = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
			formatTime(m.EatenAt), m.Day.String(), m.Description, string(m.Client), m.Note, m.Fingerprint(),
			formatTime(now), m.ID.String(), userID.String())
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrNotFound
		}
		kept, err := itemIDs(ctx, tx, userID, m.ID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM meal_items WHERE meal_id = ? AND user_id = ?",
			m.ID.String(), userID.String()); err != nil {
			return err
		}
		return insertItems(ctx, tx, userID, m, kept)
	})
	if errors.Is(err, ErrNotFound) {
		return meal.Meal{}, err
	}
	if err != nil {
		return meal.Meal{}, fmt.Errorf("replace meal: %w", err)
	}
	return db.MealByID(ctx, userID, m.ID)
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

// insertItems writes m's items, in order. An item keeps its id when kept
// holds it (the meal had it before an edit); any other gets a new one,
// so item ids are the store's to give. The foods the items name must be
// userID's: the foreign key on the food and the user refuses any other.
func insertItems(ctx context.Context, tx *sql.Tx, userID uuid.UUID, m meal.Meal, kept map[uuid.UUID]bool) error {
	for i := range m.Items {
		it := &m.Items[i]
		id := it.ID
		if !kept[id] {
			id = uuid.NewV7()
		}
		var foodID any
		if it.FoodID != uuid.Nil() {
			foodID = it.FoodID.String()
		}
		n := it.Nutrients
		o := optionalNutrients(&n)
		q := it.Quantity
		if _, err := tx.ExecContext(ctx, `INSERT INTO meal_items (id, meal_id, user_id, position, food_id, name,
			quantity_amount, quantity_unit, quantity_servings, quantity_serving, quantity_portions, amount, unit,
			kcal, protein, carbs, fat, fiber, saturated_fat, sugars, salt, polyols, alcohol, source, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.String(), m.ID.String(), userID.String(), i, foodID, it.Name,
			q.Amount, q.Unit, q.Servings, q.Serving, q.Portions, it.Amount, it.Unit,
			n.Kcal, n.Protein, n.Carbs, n.Fat, nullable(*o[0]), nullable(*o[1]), nullable(*o[2]), nullable(*o[3]),
			nullable(*o[4]), nullable(*o[5]), string(it.Source), it.Note); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMeal removes userID's meal with id and its items, or returns
// ErrNotFound.
func (db *DB) DeleteMeal(ctx context.Context, userID, id uuid.UUID) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM meals WHERE id = ? AND user_id = ?", id.String(), userID.String())
		if err != nil {
			return fmt.Errorf("delete meal: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("delete meal: %w", err)
		} else if n == 0 {
			return ErrNotFound
		}
		return nil
	})
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
	var foodID sql.NullString
	var it meal.Item
	var opt [6]sql.NullFloat64
	q, n := &it.Quantity, &it.Nutrients
	if err := rows.Scan(&id, &foodID, &it.Name, &q.Amount, &q.Unit, &q.Servings, &q.Serving, &q.Portions,
		&it.Amount, &it.Unit, &n.Kcal, &n.Protein, &n.Carbs, &n.Fat,
		&opt[0], &opt[1], &opt[2], &opt[3], &opt[4], &opt[5], &source, &it.Note); err != nil {
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
			ORDER BY m.eaten_at DESC, i.position DESC LIMIT 1`, userID.String(), id.String()).
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
// reason, or changes the reason of a day already out.
func (db *DB) ExcludeDay(ctx context.Context, userID uuid.UUID, d day.Date, e day.Exclusion, now time.Time) error {
	if err := day.ValidateExclusion(e); err != nil {
		return fmt.Errorf("invalid exclusion: %w", err)
	}
	if d.IsZero() {
		return errors.New("invalid exclusion: the day is missing")
	}
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO days (user_id, day, excluded_reason, updated_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id, day) DO UPDATE SET excluded_reason = excluded.excluded_reason, updated_at = excluded.updated_at`,
			userID.String(), d.String(), e.Reason, formatTime(now))
		if err != nil {
			return fmt.Errorf("exclude day: %w", err)
		}
		return nil
	})
}

// IncludeDay brings userID's logical day d back into calibration. A day
// not excluded stays as it is.
func (db *DB) IncludeDay(ctx context.Context, userID uuid.UUID, d day.Date, now time.Time) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE days SET excluded_reason = NULL, updated_at = ? WHERE user_id = ? AND day = ? AND excluded_reason IS NOT NULL",
			formatTime(now), userID.String(), d.String())
		if err != nil {
			return fmt.Errorf("include day: %w", err)
		}
		return nil
	})
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
