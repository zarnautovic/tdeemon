package store

import (
	"reflect"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

// 0018 indexes the foods saved before it as the store indexes a food
// saved now (searchText): a đ also as d and as dj, in the name, the brand
// and the aliases, and the rest as it was.
func TestMigrationOfTheFoodsSearchIndexesAsTheStore(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.migrate(t.Context(), upTo(t, "0017_telegram_chats.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0017: %v", err)
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
	for _, f := range []food.Food{
		{Name: "Bijelo grožđe", Brand: "Đakovo", Aliases: []string{"slatko", "Grožđica", "zeleno"}},
		{Name: "Ćevapi", Aliases: []string{"ćevčići"}},
		{Name: "Kruh"},
	} {
		f.ID = uuid.NewV7()
		f.Basis, f.Source = food.Per100g, food.Label
		f.Nutrients = food.Nutrients{Kcal: 70, Protein: 1, Carbs: 16, Fat: 0.2}
		if _, err := db.SaveFood(t.Context(), user, actor, f, now); err != nil {
			t.Fatal(err)
		}
	}
	want := searchRows(t, db)
	// As the store indexed them before: each text as it is.
	exec(`UPDATE foods_fts SET
		name = (SELECT f.name FROM foods AS f WHERE f.id = foods_fts.food_id),
		brand = (SELECT f.brand FROM foods AS f WHERE f.id = foods_fts.food_id),
		aliases = coalesce((SELECT group_concat(alias, char(10)) FROM
			(SELECT a.alias FROM food_aliases AS a WHERE a.food_id = foods_fts.food_id ORDER BY a.rowid)), '')`)
	if reflect.DeepEqual(searchRows(t, db), want) {
		t.Fatal("the rows as they were before are those of now")
	}

	if err := db.migrate(t.Context(), upTo(t, "0018_foods_search_dj.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0018: %v", err)
	}
	if got := searchRows(t, db); !reflect.DeepEqual(got, want) {
		t.Errorf("foods_fts after the migration:\n%q\nwant\n%q", got, want)
	}
	var found int
	if err := db.read.QueryRowContext(t.Context(),
		`SELECT count(*) FROM foods_fts WHERE foods_fts MATCH '"grozde"* "dakovo"* "grozdica"*'`).Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Errorf("found %d foods by grozde, dakovo and grozdica, want 1", found)
	}
}

// searchRows is foods_fts, row by row in the order of the foods' ids.
func searchRows(t *testing.T, db *DB) [][4]string {
	t.Helper()
	rows, err := db.read.QueryContext(t.Context(), "SELECT food_id, name, brand, aliases FROM foods_fts ORDER BY food_id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out [][4]string
	for rows.Next() {
		var r [4]string
		if err := rows.Scan(&r[0], &r[1], &r[2], &r[3]); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
