package store

import (
	"fmt"
	"reflect"
	"testing"
)

// 0011 rebuilds the meals table for the api client: every meal stays as
// it was, its items with it, and the items' foreign key to their meal
// still holds.
func TestMigrationOfTheMealClientKeepsTheMeals(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.migrate(t.Context(), upTo(t, "0010_plan.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0010: %v", err)
	}
	const user = "0199f0a0-0000-7000-8000-000000000001"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.write.ExecContext(t.Context(), q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, email, name, role, locale, timezone, day_start_hour, created_at, updated_at)
		VALUES (?, 'ana@example.com', 'Ana', 'user', 'en', 'UTC', 4, '2026-10-21T08:00:00Z', '2026-10-21T08:00:00Z')`, user)
	for i, id := range []string{"0199f0a0-0000-7000-8000-0000000000a1", "0199f0a0-0000-7000-8000-0000000000a2"} {
		exec(`INSERT INTO meals (id, user_id, eaten_at, day, description, client, note, fingerprint, created_at, updated_at)
			VALUES (?, ?, '2026-10-21T08:00:00Z', '2026-10-21', 'Lunch', 'mcp', 'a note', printf('%064d', ?), '2026-10-21T08:00:00Z', '2026-10-21T08:00:00Z')`, id, user, i)
		exec(`INSERT INTO meal_items (id, meal_id, user_id, position, name, amount, unit, kcal, protein, carbs, fat, source)
			VALUES (?, ?, ?, 0, 'Soup', 1, 'portion', 180, 6, 20, 8, 'estimate')`, id[:34]+"b"+id[35:], id, user)
	}
	// Rowids with gaps, as deletes leave them.
	exec("UPDATE meals SET rowid = rowid * 10 + 7")
	before := tableRows(t, db, "meals")
	items := tableRows(t, db, "meal_items")

	if err := db.migrate(t.Context(), upTo(t, "0011_meal_client_api.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0011: %v", err)
	}
	if after := tableRows(t, db, "meals"); !reflect.DeepEqual(after, before) {
		t.Errorf("meals after the migration:\n%q\nwant\n%q", after, before)
	}
	if after := tableRows(t, db, "meal_items"); !reflect.DeepEqual(after, items) {
		t.Errorf("items after the migration:\n%q\nwant\n%q", after, items)
	}
	// The api client goes in, and an item of no meal does not.
	exec(`INSERT INTO meals (id, user_id, eaten_at, day, client, fingerprint, created_at, updated_at)
		VALUES ('0199f0a0-0000-7000-8000-0000000000a3', ?, '2026-10-21T09:00:00Z', '2026-10-21', 'api', printf('%064d', 3), '2026-10-21T09:00:00Z', '2026-10-21T09:00:00Z')`, user)
	if _, err := db.write.ExecContext(t.Context(), `INSERT INTO meal_items (id, meal_id, user_id, position, name, amount, unit, kcal, protein, carbs, fat, source)
		VALUES ('0199f0a0-0000-7000-8000-0000000000c1', '0199f0a0-0000-7000-8000-0000000000ff', ?, 0, 'Soup', 1, 'portion', 180, 6, 20, 8, 'estimate')`, user); err == nil {
		t.Error("an item of no meal went in")
	}
	// Deleting a meal takes its items.
	exec("DELETE FROM meals WHERE id = '0199f0a0-0000-7000-8000-0000000000a1'")
	if left := tableRows(t, db, "meal_items"); len(left) != 1 {
		t.Errorf("items after deleting a meal: %q, want the other's", left)
	}
}

// tableRows returns every column of every row of table, in the order of
// rowid, as text.
func tableRows(t *testing.T, db *DB, table string) [][]string {
	t.Helper()
	rows, err := db.Read().QueryContext(t.Context(), `SELECT rowid, * FROM "`+table+`" ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		r := make([]string, len(vals))
		for i, v := range vals {
			r[i] = fmt.Sprint(v)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
