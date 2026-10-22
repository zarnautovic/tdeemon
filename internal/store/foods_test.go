package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/store"
)

func bar() food.Food {
	return food.Food{
		ID: uuid.NewV7(), Name: "Protein bar Caramel", Brand: "Barebells", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 362.5, Protein: 29, Carbs: 31, Fat: 16.5, Polyols: new(19.0), SaturatedFat: new(7.6)},
		Servings:  []food.Serving{{Name: "bar", Amount: 55}},
		Aliases:   []string{"caramel bar"},
		Note:      "from the label",
	}
}

func saveFood(t *testing.T, db *store.DB, userID uuid.UUID, f food.Food) food.Food {
	t.Helper()
	got, err := db.SaveFood(t.Context(), userID, testActor, f, t0)
	if err != nil {
		t.Fatalf("SaveFood(%s): %v", f.Name, err)
	}
	return got
}

func TestSaveFoodAndReadItBack(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	want := bar()
	got := saveFood(t, db, ana, want)
	// Every field, the optional nutrients' values and absence included.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read back %+v, want %+v", got, want)
	}
	again, err := db.FoodByID(t.Context(), ana, want.ID)
	if err != nil || again.Name != want.Name || len(again.Servings) != 1 {
		t.Errorf("FoodByID = %+v, %v", again, err)
	}
}

func TestSaveFoodPerPortion(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	stew := food.Food{
		ID: uuid.NewV7(), Name: "Canteen bean stew", Basis: food.PerPortion, Source: food.Estimate,
		Nutrients: food.Nutrients{Kcal: 850, Protein: 40, Carbs: 90, Fat: 30}, PortionGrams: 500,
	}
	if got := saveFood(t, db, ana, stew); got.Basis != food.PerPortion || got.PortionGrams != 500 {
		t.Errorf("read back %+v", got)
	}
	stew.ID, stew.Name, stew.PortionGrams = uuid.NewV7(), "Canteen goulash", 0
	if got := saveFood(t, db, ana, stew); got.PortionGrams != 0 {
		t.Errorf("unknown portion weight read back as %v", got.PortionGrams)
	}
}

// The same name and brand, in any letter case or spacing, are the same
// food: saving it again corrects it in place and keeps its id.
func TestSaveFoodSameNameIsTheSameFood(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	first := saveFood(t, db, ana, bar())
	corrected := bar() // a new id, as a client that did not search first would send
	corrected.Name, corrected.Brand = "protein BAR  caramel", "BAREBELLS"
	corrected.Nutrients.Kcal = 360
	corrected.Servings = []food.Serving{{Name: "bar", Amount: 55}, {Name: "half", Amount: 27.5}}
	corrected.Aliases = nil
	got := saveFood(t, db, ana, corrected)
	if got.ID != first.ID || got.Nutrients.Kcal != 360 || got.Name != "protein BAR  caramel" || len(got.Servings) != 2 || len(got.Aliases) != 0 {
		t.Errorf("after the correction: %+v; want the first food's id with the new values", got)
	}
	if n := countRows(t, db, "foods"); n != 1 {
		t.Errorf("%d foods, want 1", n)
	}
	// Another brand is another food.
	other := bar()
	other.Brand = "Other brand"
	other.Aliases = nil
	if got := saveFood(t, db, ana, other); got.ID == first.ID {
		t.Error("another brand was taken for the same food")
	}
}

// Saving by id updates that food, also under a new name, unless another
// food has it.
func TestSaveFoodByID(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	f := saveFood(t, db, ana, bar())
	f.Name = "Protein bar Salted caramel"
	if got := saveFood(t, db, ana, f); got.ID != f.ID || got.Name != f.Name {
		t.Errorf("rename: %+v", got)
	}
	other := bar()
	other.Name, other.Aliases = "Protein bar Cookies", nil
	saveFood(t, db, ana, other)
	f.Name = "PROTEIN BAR cookies"
	if _, err := db.SaveFood(t.Context(), ana, testActor, f, t0); !errors.Is(err, store.ErrNameTaken) {
		t.Errorf("rename onto another food = %v, want ErrNameTaken", err)
	}
}

func TestSaveFoodRefuses(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	saveFood(t, db, ana, bar())
	// An alias another food goes by.
	other := bar()
	other.Name, other.Aliases = "Protein bar Cookies", []string{"Caramel  BAR"}
	if _, err := db.SaveFood(t.Context(), ana, testActor, other, t0); !errors.Is(err, store.ErrAliasTaken) {
		t.Errorf("a taken alias = %v, want ErrAliasTaken", err)
	}
	// A food that breaks the rules.
	bad := bar()
	bad.Nutrients.Kcal = -1
	if _, err := db.SaveFood(t.Context(), ana, testActor, bad, t0); err == nil || !strings.Contains(err.Error(), "invalid food") {
		t.Errorf("an invalid food = %v", err)
	}
}

// Ids are per user: another user's food under the same id is no food of
// this user's, so the food is saved under the id it was given, exactly as
// under an id nobody has, and the other user's food is untouched.
func TestSaveFoodUnderAnotherUsersID(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	ivos := saveFood(t, db, ivo, food.Food{
		ID: uuid.NewV7(), Name: "Ivo's bread", Basis: food.Per100g, Source: food.User,
		Nutrients: food.Nutrients{Kcal: 250, Protein: 9, Carbs: 49, Fat: 3},
	})
	steal := bar()
	steal.ID = ivos.ID
	got := saveFood(t, db, ana, steal)
	if got.ID != ivos.ID || got.Name != steal.Name {
		t.Errorf("saved as %+v, want ana's bar under the id it was given", got)
	}
	if got, _ := db.FoodByID(t.Context(), ivo, ivos.ID); got.Name != "Ivo's bread" {
		t.Errorf("ivo's food became %q", got.Name)
	}
}

// Two users' foods under one id stay two foods all the way: search, and
// the deletion of one user, which leaves the other's search row.
func TestFoodsUnderOneIDStayApart(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	anas := saveFood(t, db, ana, bar())
	ivos := food.Food{
		ID: anas.ID, Name: "Raženi kruh", Basis: food.Per100g, Source: food.User,
		Nutrients: food.Nutrients{Kcal: 250, Protein: 9, Carbs: 49, Fat: 3}, Aliases: []string{"ivin kruh"},
	}
	saveFood(t, db, ivo, ivos)
	if got, _ := db.FindFoods(t.Context(), ana, "kruh", 10); len(got) != 0 {
		t.Errorf("ana finds ivo's words through the shared id: %v", names(got))
	}
	if got, _ := db.FindFoods(t.Context(), ivo, "caramel", 10); len(got) != 0 {
		t.Errorf("ivo finds ana's words through the shared id: %v", names(got))
	}
	exec(t, db, "DELETE FROM users WHERE id = '"+ana.String()+"'")
	if got, _ := db.FindFoods(t.Context(), ivo, "kruh", 10); len(got) != 1 || got[0].ID != anas.ID {
		t.Errorf("after ana's deletion ivo finds %v, want his bread", names(got))
	}
}

// Saving the name and brand of an archived food corrects it and brings it
// back into search.
func TestSaveFoodRevivesAnArchivedOne(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	old := saveFood(t, db, ana, bar())
	if err := db.SetFoodArchived(t.Context(), ana, testActor, old.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	again := bar()
	again.Nutrients.Kcal = 370
	got := saveFood(t, db, ana, again)
	if got.ID != old.ID || got.Archived || got.Nutrients.Kcal != 370 {
		t.Errorf("saved as %+v, want the old food, corrected and not archived", got)
	}
	if found, _ := db.FindFoods(t.Context(), ana, "caramel", 10); len(found) != 1 {
		t.Errorf("the revived food is not found: %v", names(found))
	}
}

// When a food goes, its search row goes with it, also when the food goes
// with its user.
func TestSearchRowsLeaveWithTheirFood(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	saveFood(t, db, ana, bar())
	saveFood(t, db, ivo, bar())
	secret := bar()
	secret.Name, secret.Aliases = "Ivozzzsecretbread", nil
	saveFood(t, db, ivo, secret)
	exec(t, db, "DELETE FROM users WHERE id = '"+ivo.String()+"'")
	// Secure-delete: the words are gone from the index's own pages too.
	var left int
	if err := db.Read().QueryRowContext(t.Context(),
		"SELECT count(*) FROM foods_fts_data WHERE instr(block, CAST('ivozzzsecretbread' AS BLOB)) > 0").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d index pages still hold the deleted user's word", left)
	}
	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM foods_fts WHERE user_id = ?", ivo.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 || countRows(t, db, "foods_fts") != 1 {
		t.Errorf("%d search rows of the deleted user left, %d in all; want 0 and ana's 1", n, countRows(t, db, "foods_fts"))
	}
}

// One user's foods are not another's: the same name is fine, and neither
// sees the other's.
func TestFoodsBelongToTheirUser(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	anas := saveFood(t, db, ana, bar())
	ivos := saveFood(t, db, ivo, bar())
	if anas.ID == ivos.ID {
		t.Fatal("the same id for both")
	}
	if _, err := db.FoodByID(t.Context(), ivo, anas.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ivo reading ana's food = %v, want ErrNotFound", err)
	}
	if err := db.SetFoodArchived(t.Context(), ivo, testActor, anas.ID, true, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ivo archiving ana's food = %v, want ErrNotFound", err)
	}
	found, err := db.FindFoods(t.Context(), ivo, "caramel", 10)
	if err != nil || len(found) != 1 || found[0].ID != ivos.ID {
		t.Errorf("ivo's search = %+v, %v; want only his bar", found, err)
	}
}

// A serving or an alias can only belong to a food of its own user: the
// foreign key names both.
func TestFoodChildrenCannotCrossUsers(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	anas := saveFood(t, db, ana, bar())
	for _, q := range []string{
		"INSERT INTO food_servings (food_id, user_id, name, name_key, amount) VALUES (?, ?, 'x', 'x', 1)",
		"INSERT INTO food_aliases (food_id, user_id, alias, alias_key) VALUES (?, ?, 'x', 'x')",
	} {
		err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
			_, err := tx.ExecContext(t.Context(), q, anas.ID.String(), ivo.String())
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
			t.Errorf("%s as ivo = %v, want a foreign key error", q, err)
		}
	}
}

func TestFindFoods(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	mk := func(name, brand string, aliases ...string) food.Food {
		return saveFood(t, db, ana, food.Food{
			ID: uuid.NewV7(), Name: name, Brand: brand, Basis: food.Per100g, Source: food.Estimate,
			Nutrients: food.Nutrients{Kcal: 100, Protein: 5, Carbs: 10, Fat: 3}, Aliases: aliases,
		})
	}
	cevapi := mk("Ćevapi", "")
	water := mk("H\u2082O voda", "")
	yoghurt := mk("Grčki jogurt 2 %", "Dukat")
	bread := mk("Proteinski kruh", "", "moj kruh")
	pecivo := mk("Pecivo sa sezamom", "Kruh i pecivo")
	gone := mk("Stari kruh", "")
	if err := db.SetFoodArchived(t.Context(), ana, testActor, gone.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		want  []uuid.UUID
	}{
		{"cevapi", []uuid.UUID{cevapi.ID}},         // without diacritics
		{"c\u030cevapi", []uuid.UUID{cevapi.ID}},   // decomposed, as pasted from a PDF
		{"h\u2082o", []uuid.UUID{water.ID}},        // a subscript digit is part of its word
		{"ĆEV", []uuid.UUID{cevapi.ID}},            // a prefix, any case
		{"grcki jog", []uuid.UUID{yoghurt.ID}},     // every word a prefix
		{"dukat", []uuid.UUID{yoghurt.ID}},         // by brand
		{"moj", []uuid.UUID{bread.ID}},             // by alias
		{"kruh", []uuid.UUID{bread.ID, pecivo.ID}}, // the name first, then the brand; not the archived one
		{"jogurt kruh", nil},                       // all words must match
		{"", nil},
		{"  ,;  ", nil},
		{`"kruh" OR stari`, nil}, // FTS syntax is only words
		{"kruh*", []uuid.UUID{bread.ID, pecivo.ID}},
		{"NEAR(kruh pecivo)", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			got, err := db.FindFoods(t.Context(), ana, tc.query, 10)
			if err != nil {
				t.Fatalf("FindFoods: %v", err)
			}
			var ids []uuid.UUID
			for _, f := range got {
				ids = append(ids, f.ID)
			}
			if len(ids) != len(tc.want) {
				t.Fatalf("FindFoods(%q) = %v, want %v", tc.query, names(got), tc.want)
			}
			for i := range ids {
				if ids[i] != tc.want[i] {
					t.Errorf("FindFoods(%q) = %v; want another order or set", tc.query, names(got))
				}
			}
		})
	}
	// Results carry their servings and aliases.
	if got, _ := db.FindFoods(t.Context(), ana, "moj kruh", 10); len(got) != 1 || len(got[0].Aliases) != 1 {
		t.Errorf("search result without its alias: %+v", got)
	}
	if got, _ := db.FindFoods(t.Context(), ana, "kruh", 1); len(got) != 1 {
		t.Errorf("limit 1 gave %d", len(got))
	}
	if got, _ := db.FindFoods(t.Context(), ana, "kruh", 0); got != nil {
		t.Errorf("limit 0 gave %d", len(got))
	}
}

// A match in the name ranks before one in an alias, and that before one
// in the brand, however long the name and however the names sort.
func TestFindFoodsRanksNamesFirst(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	mk := func(name, brand string, aliases ...string) uuid.UUID {
		return saveFood(t, db, ana, food.Food{
			ID: uuid.NewV7(), Name: name, Brand: brand, Basis: food.Per100g, Source: food.Estimate,
			Nutrients: food.Nutrients{Kcal: 100, Protein: 5, Carbs: 10, Fat: 3}, Aliases: aliases,
		}).ID
	}
	byBrand := mk("Ajvar blagi", "Kruh")
	byAlias := mk("Burek sa sirom", "", "kruh")
	byName := mk("Integralni kruh s pet vrsta sjemenki i zobenim pahuljicama", "")
	short := mk("Kruh", "")
	white := mk("Bijeli kruh", "")
	for _, n := range []string{"Ajvar ljuti", "Banana", "Cedevita", "Datulje", "Egg", "Feta", "Grah", "Hummus"} {
		mk(n, "")
	}
	got, err := db.FindFoods(t.Context(), ana, "kruh", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[0].ID != short || got[1].ID != white || got[2].ID != byName || got[3].ID != byAlias || got[4].ID != byBrand {
		t.Errorf("FindFoods(kruh) = %v, want the names (shorter first), the alias, then the brand", names(got))
	}
}

// The order of one user's results does not depend on another user's
// foods: a relevance score over the whole index would let one user learn
// what words another's foods hold.
func TestFindFoodsOrderIgnoresOtherUsers(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	for _, nb := range [][2]string{{"Mlijeko", "Sir"}, {"Sir", "Mlijeko"}, {"Mlijeko i sir", ""}} {
		saveFood(t, db, ana, food.Food{
			ID: uuid.NewV7(), Name: nb[0], Brand: nb[1], Basis: food.Per100g, Source: food.Estimate,
			Nutrients: food.Nutrients{Kcal: 100, Protein: 5, Carbs: 10, Fat: 3},
		})
	}
	find := func() []string {
		t.Helper()
		got, err := db.FindFoods(t.Context(), ana, "mlijeko sir", 10)
		if err != nil {
			t.Fatal(err)
		}
		return names(got)
	}
	before := find()
	for i := range 30 {
		saveFood(t, db, ivo, food.Food{
			ID: uuid.NewV7(), Name: fmt.Sprintf("Mlijeko %d", i), Basis: food.Per100g, Source: food.Estimate,
			Nutrients: food.Nutrients{Kcal: 60, Protein: 3, Carbs: 5, Fat: 3},
		})
	}
	if after := find(); !reflect.DeepEqual(before, after) || len(before) != 3 || before[0] != "Mlijeko i sir" {
		t.Errorf("ana's results %v became %v with ivo's foods; want the same, the name with both words first", before, after)
	}
}

// An archived food leaves search, and comes back with it.
func TestFindFoodsArchived(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	f := saveFood(t, db, ana, bar())
	if err := db.SetFoodArchived(t.Context(), ana, testActor, f.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.FindFoods(t.Context(), ana, "caramel", 10); len(got) != 0 {
		t.Errorf("an archived food is found: %v", names(got))
	}
	if got, err := db.FoodByID(t.Context(), ana, f.ID); err != nil || !got.Archived {
		t.Errorf("FoodByID of an archived food = %+v, %v", got, err)
	}
	if err := db.SetFoodArchived(t.Context(), ana, testActor, f.ID, false, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.FindFoods(t.Context(), ana, "caramel", 10); len(got) != 1 {
		t.Errorf("an unarchived food is not found: %v", names(got))
	}
}

// Renaming a food moves it in search, and its aliases with it.
func TestFindFoodsAfterACorrection(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	f := saveFood(t, db, ana, bar())
	f.Name, f.Aliases = "Protein bar Cookies", []string{"cookie bar"}
	saveFood(t, db, ana, f)
	if got, _ := db.FindFoods(t.Context(), ana, "caramel", 10); len(got) != 0 {
		t.Errorf("the old name is still found: %v", names(got))
	}
	if got, _ := db.FindFoods(t.Context(), ana, "cookie", 10); len(got) != 1 {
		t.Errorf("the new name or alias is not found: %v", names(got))
	}
	if n := countRows(t, db, "foods_fts"); n != 1 {
		t.Errorf("%d search rows, want 1", n)
	}
}

func TestFoodStoreFailures(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	f := saveFood(t, db, ana, bar())
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithoutCancel(t.Context())
	if _, err := db.SaveFood(ctx, ana, testActor, bar(), t0); err == nil {
		t.Error("SaveFood on a closed database succeeded")
	}
	if _, err := db.FoodByID(ctx, ana, f.ID); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Errorf("FoodByID on a closed database = %v", err)
	}
	if _, err := db.FindFoods(ctx, ana, "bar", 5); err == nil {
		t.Error("FindFoods on a closed database succeeded")
	}
	if err := db.SetFoodArchived(ctx, ana, testActor, f.ID, true, t0); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetFoodArchived on a closed database = %v", err)
	}
}

func names(fs []food.Food) []string {
	out := make([]string, len(fs))
	for i := range fs {
		out[i] = fs[i].Name
	}
	return out
}

func countRows(t *testing.T, db *store.DB, table string) int {
	t.Helper()
	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// A save another food is in the way of names that food, archived or
// not, so a client can deal with it: a search does not find an archived
// food.
func TestSaveFoodNamesTheFoodInTheWay(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	old := saveFood(t, db, ana, bar())
	if err := db.SetFoodArchived(t.Context(), ana, testActor, old.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	// Its alias, on a new food.
	other := bar()
	other.Name, other.Aliases = "Protein bar Cookies", []string{"caramel bar"}
	_, err := db.SaveFood(t.Context(), ana, testActor, other, t0)
	var conflict *store.FoodConflictError
	if !errors.As(err, &conflict) || !errors.Is(err, store.ErrAliasTaken) || conflict.Food.ID != old.ID || conflict.Food.Name != old.Name ||
		!conflict.Food.Archived || conflict.Alias != "caramel bar" {
		t.Errorf("a taken alias = %#v, want the archived food in the way", err)
	}
	// Its name, on a food renamed.
	other.Aliases = nil
	saved := saveFood(t, db, ana, other)
	saved.Name, saved.Brand = old.Name, old.Brand
	_, err = db.SaveFood(t.Context(), ana, testActor, saved, t0)
	if !errors.As(err, &conflict) || !errors.Is(err, store.ErrNameTaken) || conflict.Food.ID != old.ID || conflict.Alias != "" {
		t.Errorf("a taken name = %#v, want the archived food in the way", err)
	}
}

// CorrectFood saves a food the user has, by its id, in the save's own
// transaction: an id they have no food of is ErrNotFound, and saves
// nothing, rather than a new food or the food of the same name.
func TestCorrectFood(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	f := saveFood(t, db, ana, bar())
	f.Nutrients.Kcal = 360
	if got, err := db.CorrectFood(t.Context(), ana, testActor, f, t0); err != nil || got.ID != f.ID || got.Nutrients.Kcal != 360 {
		t.Errorf("a correction = %+v, %v", got, err)
	}
	// An id no food of ana's has, under the name of hers: nothing.
	stray := f
	stray.ID = uuid.NewV7()
	if _, err := db.CorrectFood(t.Context(), ana, testActor, stray, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an unknown id = %v, want ErrNotFound", err)
	}
	// Her id is no food of ivo's.
	if _, err := db.CorrectFood(t.Context(), ivo, testActor, f, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another user's id = %v, want ErrNotFound", err)
	}
	if got, err := db.FoodByID(t.Context(), ana, f.ID); err != nil || got.Nutrients.Kcal != 360 {
		t.Errorf("her food = %+v, %v; want her correction", got, err)
	}
	if foods, err := db.FindFoods(t.Context(), ivo, "caramel", 10); err != nil || len(foods) != 0 {
		t.Errorf("ivo's foods = %v, %v; want none", foods, err)
	}
}
