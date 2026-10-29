package store_test

import (
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// archivedBar saves ana a bar, through MCP, and archives it; ivo gets a
// bar of their own.
func archivedBar(t *testing.T, db *store.DB, ana, ivo uuid.UUID) food.Food {
	t.Helper()
	f, err := db.SaveFood(t.Context(), ana, viaMCP, bar(), t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetFoodArchived(t.Context(), ana, viaMCP, f.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveFood(t.Context(), ivo, viaMCP, bar(), t0); err != nil {
		t.Fatal(err)
	}
	return f
}

// RecentChanges are one user's latest changes, newest first, each with
// the names of what it was of, a food archived named as it is now; the
// latest is next to undo.
func TestRecentChanges(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	f := archivedBar(t, db, ana, ivo)

	cs, err := db.RecentChanges(t.Context(), ana, 10)
	if err != nil || len(cs) != 2 {
		t.Fatalf("RecentChanges = %+v, %v; want the archive and the save", cs, err)
	}
	if c := cs[0]; c.Event.Action != event.FoodArchive || c.Name != f.Name || c.Brand != f.Brand || !c.Archived || !c.Next {
		t.Errorf("the archive = %+v", c)
	}
	if c := cs[1]; c.Event.Action != event.FoodSave || c.Name != f.Name || c.Next {
		t.Errorf("the save = %+v", c)
	}
	if cs, err := db.RecentChanges(t.Context(), ana, 0); cs != nil || err != nil {
		t.Errorf("none asked for: %+v, %v", cs, err)
	}
}

// A food gone since (its save undone) names nothing; the undos are left
// out, and with nothing left to undo, none is next.
func TestRecentChangesOfAFoodGone(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	archivedBar(t, db, ana, ivo)
	undo(t, db, ana)
	undo(t, db, ana)

	cs, err := db.RecentChanges(t.Context(), ana, 1)
	if err != nil || len(cs) != 1 {
		t.Fatalf("RecentChanges = %+v, %v; want the latest", cs, err)
	}
	if c := cs[0]; c.Event.Action != event.FoodArchive || c.Name != "" || c.Event.UndoneBy == uuid.Nil() || c.Next {
		t.Errorf("the archive of a food gone = %+v", c)
	}
}

// A meal saved with a new food is one change, the meal's: the food's
// save is part of it, and undo takes it back with the meal.
func TestRecentChangesOfAMealWithItsNewFood(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar())
	k := kefir()
	m := breakfast(t, b, t0)
	m.Items = []meal.Item{itemOf(t, k, meal.Quantity{Amount: 250, Unit: "ml"})}
	if _, _, err := db.CreateMealWithFoods(t.Context(), ana, testActor, m, []food.Food{k}, t0); err != nil {
		t.Fatalf("CreateMealWithFoods: %v", err)
	}

	cs, err := db.RecentChanges(t.Context(), ana, 10)
	if err != nil || len(cs) != 2 {
		t.Fatalf("RecentChanges = %+v, %v; want the meal's create and the bar's save", cs, err)
	}
	if c := cs[0]; c.Event.Action != event.MealCreate || c.Event.Entity != m.ID.String() || !c.Next {
		t.Errorf("the meal = %+v", c)
	}
	if c := cs[1]; c.Event.Action != event.FoodSave || c.Event.Entity != b.ID.String() {
		t.Errorf("the change before the meal = %+v, want the bar's save", c)
	}
}
