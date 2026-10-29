package store

import (
	"reflect"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

// 0020 keeps an item's brand apart from its name: an item logged before,
// the brand and a space before the name, is split where its food's brand
// still starts the name, its food renamed since too, and left as it was
// where the brand has changed since, or there is none.
func TestMigrationOfTheItemsBrandSplitsTheName(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.migrate(t.Context(), upTo(t, "0019_proposals.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0019: %v", err)
	}
	laterTables(t, db, "0022_food_barcodes.sql")
	user := uuid.MustParse("0199f0a0-0000-7000-8000-000000000001")
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.write.ExecContext(t.Context(), q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, email, name, role, locale, timezone, day_start_hour, created_at, updated_at)
		VALUES (?, 'ana@example.com', 'Ana', 'user', 'en', 'UTC', 4, '2026-10-21T08:00:00Z', '2026-10-21T08:00:00Z')`, user.String())
	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	actor := event.Actor{Kind: event.User, Client: "web"}
	ids := map[string]string{}
	for _, f := range []food.Food{{Name: "Tuna u vlastitom soku", Brand: "Eva"}, {Name: "Kruh"}} {
		f.ID = uuid.NewV7()
		f.Basis, f.Source = food.Per100g, food.Label
		f.Nutrients = food.Nutrients{Kcal: 116, Protein: 26, Carbs: 0, Fat: 1}
		if _, err := db.SaveFood(t.Context(), user, actor, f, now); err != nil {
			t.Fatal(err)
		}
		ids[f.Name] = f.ID.String()
	}
	const meal = "0199f0a0-0000-7000-8000-0000000000a1"
	exec(`INSERT INTO meals (id, user_id, eaten_at, day, description, client, note, fingerprint, created_at, updated_at)
		VALUES (?, ?, '2026-10-21T08:00:00Z', '2026-10-21', '', 'web', '', printf('%064d', 1), '2026-10-21T08:00:00Z', '2026-10-21T08:00:00Z')`,
		meal, user.String())
	tuna, bread := ids["Tuna u vlastitom soku"], ids["Kruh"]
	for i, it := range []struct {
		food         any
		name, source string
	}{
		{tuna, "Eva Tuna u vlastitom soku", "label"},
		// Its food renamed since, its brand the same.
		{tuna, "Eva Tuna", "label"},
		// Its food's brand changed since.
		{tuna, "Rio Mare Tuna u vlastitom soku", "label"},
		{bread, "Kruh", "label"},
		// An estimate, which no food's brand starts.
		{nil, "Eva salad", "estimate"},
	} {
		exec(`INSERT INTO meal_items (id, meal_id, user_id, position, food_id, name, amount, unit, kcal, protein, carbs, fat, source)
			VALUES (printf('0199f0a0-0000-7000-8000-0000000000b%d', ?), ?, ?, ?, ?, ?, 100, 'g', 116, 26, 0, 1, ?)`,
			i, meal, user.String(), i, it.food, it.name, it.source)
	}

	if err := db.migrate(t.Context(), upTo(t, "0020_meal_item_brand.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0020: %v", err)
	}
	rows, err := db.read.QueryContext(t.Context(), "SELECT name, brand FROM meal_items ORDER BY position")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got [][2]string
	for rows.Next() {
		var r [2]string
		if err := rows.Scan(&r[0], &r[1]); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{
		{"Tuna u vlastitom soku", "Eva"},
		{"Tuna", "Eva"},
		{"Rio Mare Tuna u vlastitom soku", ""},
		{"Kruh", ""},
		{"Eva salad", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("items after the migration: %q, want %q", got, want)
	}
}
