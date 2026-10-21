package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/store"
)

var (
	viaMCP   = event.Actor{Kind: event.User, Client: "mcp", Model: "claude-opus-5-5"}
	phone    = event.Actor{Kind: event.Import, Client: "hae"}
	importer = event.Actor{Kind: event.Import, Client: "import"}
)

func undo(t *testing.T, db *store.DB, userID uuid.UUID) event.Event {
	t.Helper()
	e, err := db.UndoLast(t.Context(), userID, testActor, t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("UndoLast: %v", err)
	}
	return e
}

func TestWritesAreRecorded(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	f, err := db.SaveFood(t.Context(), ana, viaMCP, bar(), t0)
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := db.CreateMeal(t.Context(), ana, viaMCP, breakfast(t, f, t0), t0)
	if err != nil {
		t.Fatal(err)
	}
	es, err := db.Events(t.Context(), ana, 10)
	if err != nil || len(es) != 2 {
		t.Fatalf("Events = %d, %v; want 2", len(es), err)
	}
	// Newest first, with who, what and the states.
	created, saved := es[0], es[1]
	want := event.Event{ID: created.ID, At: t0, Actor: viaMCP, Action: event.MealCreate, Entity: m.ID.String(), After: created.After}
	if !reflect.DeepEqual(created, want) || created.After == nil {
		t.Errorf("the meal's event = %+v", created)
	}
	var got food.Food
	if err := json.Unmarshal(saved.After, &got); err != nil || !reflect.DeepEqual(got, f) {
		t.Errorf("the food's event after = %+v, %v; want %+v", got, err, f)
	}
	if saved.Action != event.FoodSave || saved.Before != nil {
		t.Errorf("the food's event = %+v", saved)
	}
}

func TestEventsLimit(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	saveFood(t, db, ana, bar())
	other := bar()
	other.Name, other.Aliases = "Protein bar Cookies", nil
	saveFood(t, db, ana, other)
	if es, _ := db.Events(t.Context(), ana, 1); len(es) != 1 || es[0].Entity != other.ID.String() {
		t.Errorf("Events(1) = %+v; want the newest", es)
	}
	if es, _ := db.Events(t.Context(), ana, 0); es != nil {
		t.Errorf("Events(0) = %d", len(es))
	}
}

func TestARetryRecordsNothing(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	m := breakfast(t, saveFood(t, db, ana, bar()), t0)
	createMeal(t, db, ana, m)
	if _, created, err := db.CreateMeal(t.Context(), ana, testActor, m, t0); err != nil || created {
		t.Fatalf("retry = %v, %v", created, err)
	}
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, day.DateOf(t0), []metric.Value{{Kind: metric.Steps, Value: 100, Source: metric.Import}}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, day.DateOf(t0), []metric.Value{{Kind: metric.Steps, Value: 50, Source: metric.Import}}, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.IncludeDay(t.Context(), ana, testActor, day.DateOf(t0), t0); err != nil {
		t.Fatal(err)
	}
	// The food, the meal and the first merge; not the retry, the merge
	// that changed nothing or the include of a day not excluded.
	if n := countRows(t, db, "events"); n != 3 {
		t.Errorf("%d events, want 3", n)
	}
}

func TestUndoFood(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	first := saveFood(t, db, ana, bar())
	edit := first
	edit.Name, edit.Aliases, edit.Nutrients.Kcal = "Protein bar Cookies", []string{"cookie bar"}, 370
	edit.Servings = []food.Serving{{Name: "bar", Amount: 55}, {Name: "half", Amount: 27.5}}
	saveFood(t, db, ana, edit)
	if e := undo(t, db, ana); e.Action != event.FoodSave || e.UndoneBy == uuid.Nil() {
		t.Errorf("undone %+v", e)
	}
	if got, _ := db.FoodByID(t.Context(), ana, first.ID); !reflect.DeepEqual(got, first) {
		t.Errorf("after undoing the edit the food is %+v, want %+v", got, first)
	}
	if got, _ := db.FindFoods(t.Context(), ana, "cookie", 10); len(got) != 0 {
		t.Errorf("the undone name is still found: %v", names(got))
	}
	if err := db.SetFoodArchived(t.Context(), ana, testActor, first.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	undo(t, db, ana)
	if got, _ := db.FoodByID(t.Context(), ana, first.ID); got.Archived {
		t.Error("undoing the archive left the food archived")
	}
	// Undoing the save that added it: the food goes.
	undo(t, db, ana)
	if _, err := db.FoodByID(t.Context(), ana, first.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("undoing the create left the food: %v", err)
	}
	if n := countRows(t, db, "foods_fts"); n != 0 {
		t.Errorf("%d search rows left", n)
	}
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("one undo too many = %v, want ErrNothingToUndo", err)
	}
}

// A food a meal uses stays when its create is undone: archived.
func TestUndoFoodInUse(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	f := saveFood(t, db, ana, bar())
	if _, _, err := db.CreateMeal(t.Context(), ana, importer, breakfast(t, f, t0), t0); err != nil {
		t.Fatal(err)
	}
	if e := undo(t, db, ana); e.Action != event.FoodSave {
		t.Fatalf("undone %s, want the food's save (the import's meal is not undone)", e.Action)
	}
	if got, err := db.FoodByID(t.Context(), ana, f.ID); err != nil || !got.Archived {
		t.Errorf("the food in use = %+v, %v; want it kept, archived", got, err)
	}
}

func TestUndoMeal(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	bar := saveFood(t, db, ana, bar())
	original := breakfast(t, bar, t0)
	created := createMeal(t, db, ana, original)
	edit := created
	edit.Note = "changed"
	added, _ := meal.FromFood(bar, meal.Quantity{Amount: 30, Unit: "g"}, "")
	edit.Items = append(append([]meal.Item(nil), created.Items...), added)
	if _, err := db.ReplaceMeal(t.Context(), ana, testActor, edit, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteMeal(t.Context(), ana, testActor, created.ID, t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Undo the delete: the edited meal is back, its item ids and its
	// fingerprint with it, so a late retry of the create still finds it.
	undo(t, db, ana)
	back, err := db.MealByID(t.Context(), ana, created.ID)
	if err != nil || back.Note != "changed" || len(back.Items) != 3 || back.Items[0].ID != created.Items[0].ID {
		t.Fatalf("after undoing the delete = %+v, %v", back, err)
	}
	if got, isNew, err := db.CreateMeal(t.Context(), ana, testActor, original, t0); err != nil || isNew || !reflect.DeepEqual(got, back) {
		t.Errorf("a late retry after the undo = %v, %v; want the meal", isNew, err)
	}
	// Undo the edit: as created, item ids included.
	undo(t, db, ana)
	if got, _ := db.MealByID(t.Context(), ana, created.ID); !reflect.DeepEqual(got, created) {
		t.Errorf("after undoing the edit = %+v, want %+v", got, created)
	}
	// Undo the create: the meal goes, and its id stays a tombstone.
	undo(t, db, ana)
	if _, err := db.MealByID(t.Context(), ana, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("after undoing the create = %v", err)
	}
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, original, t0); !errors.Is(err, store.ErrMealDeleted) {
		t.Errorf("a late retry of an undone create = %v, want ErrMealDeleted", err)
	}
}

// A deleted meal's id is a tombstone: a late retry of its create does
// not bring it back.
func TestDeletedMealStaysDeleted(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	m := breakfast(t, saveFood(t, db, ana, bar()), t0)
	createMeal(t, db, ana, m)
	if err := db.DeleteMeal(t.Context(), ana, testActor, m.ID, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, m, t0); !errors.Is(err, store.ErrMealDeleted) {
		t.Errorf("a late retry of a deleted meal = %v, want ErrMealDeleted", err)
	}
	// Another user's tombstone is not this one's.
	theirs := breakfast(t, saveFood(t, db, ivo, bar()), t0)
	theirs.ID = m.ID
	if _, created, err := db.CreateMeal(t.Context(), ivo, testActor, theirs, t0); err != nil || !created {
		t.Errorf("ivo under ana's deleted id = %v, %v; want his meal", created, err)
	}
}

func TestUndoDay(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	for _, step := range []func() error{
		func() error {
			return db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "restaurant"}, t0)
		},
		func() error {
			return db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "wedding"}, t0)
		},
		func() error { return db.IncludeDay(t.Context(), ana, testActor, d, t0) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"wedding", "restaurant", ""} {
		undo(t, db, ana)
		got, err := db.Exclusions(t.Context(), ana, d, d)
		if err != nil || got[d].Reason != want || (want == "" && len(got) != 0) {
			t.Errorf("after an undo = %v, %v; want %q", got, err, want)
		}
	}
}

func TestUndoMetrics(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	steps := metric.Value{Kind: metric.Steps, Value: 5120, Source: metric.Import}
	if _, err := db.MergeMetrics(t.Context(), ana, phone, d, []metric.Value{steps}, t0); err != nil {
		t.Fatal(err)
	}
	weight := metric.Value{Kind: metric.Weight, Value: 82.4, Source: metric.Manual}
	moreSteps := metric.Value{Kind: metric.Steps, Value: 6000, Source: metric.Manual}
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, d, []metric.Value{weight, moreSteps}, t0); err != nil {
		t.Fatal(err)
	}
	set := metric.Value{Kind: metric.Weight, Value: 81.9, Source: metric.Manual}
	if _, err := db.SetMetric(t.Context(), ana, testActor, d, set, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteMetric(t.Context(), ana, testActor, d, metric.Steps, t0); err != nil {
		t.Fatal(err)
	}
	held := func() []metric.Value {
		t.Helper()
		got, err := db.Metrics(t.Context(), ana, d, d)
		if err != nil {
			t.Fatal(err)
		}
		return got[d]
	}
	undo(t, db, ana) // the delete: the manual steps are back
	if got := held(); !reflect.DeepEqual(got, []metric.Value{set, moreSteps}) {
		t.Errorf("after undoing the delete = %+v", got)
	}
	undo(t, db, ana) // the set: the merged weight is back
	if got := held(); !reflect.DeepEqual(got, []metric.Value{weight, moreSteps}) {
		t.Errorf("after undoing the set = %+v", got)
	}
	undo(t, db, ana) // the merge: no weight, the phone's steps
	if got := held(); !reflect.DeepEqual(got, []metric.Value{steps}) {
		t.Errorf("after undoing the merge = %+v", got)
	}
	// The phone's import is not the user's to undo.
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("undoing an import = %v, want ErrNothingToUndo", err)
	}
}

// An undo is an event too, of the actor that asked for it, and it marks
// the event it took back.
func TestUndoIsRecorded(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	saveFood(t, db, ana, bar())
	e, err := db.UndoLast(t.Context(), ana, viaMCP, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	es, _ := db.Events(t.Context(), ana, 10)
	if len(es) != 2 {
		t.Fatalf("%d events, want the save and the undo", len(es))
	}
	u, saved := es[0], es[1]
	if u.Action != event.Undo || u.Entity != saved.ID.String() || u.Actor != viaMCP || u.ID != e.UndoneBy ||
		saved.UndoneBy != u.ID || !reflect.DeepEqual(u.Before, saved.After) || u.After != nil {
		t.Errorf("undo event %+v of %+v", u, saved)
	}
}

// One user's undo takes back their own change only.
func TestUndoIsPerUser(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	ivos := saveFood(t, db, ivo, bar())
	anas := saveFood(t, db, ana, bar()) // ana's change is the newest of all
	undo(t, db, ivo)
	if _, err := db.FoodByID(t.Context(), ivo, ivos.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ivo's undo left his food: %v", err)
	}
	if _, err := db.FoodByID(t.Context(), ana, anas.ID); err != nil {
		t.Errorf("ivo's undo took ana's food: %v", err)
	}
	if es, _ := db.Events(t.Context(), ana, 10); len(es) != 1 || es[0].UndoneBy != uuid.Nil() {
		t.Errorf("ana's events after ivo's undo = %+v", es)
	}
}

func TestWritesNeedAnActor(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	nobody := event.Actor{Kind: event.User}
	d := day.DateOf(t0)
	f := saveFood(t, db, ana, bar())
	m := createMeal(t, db, ana, breakfast(t, f, t0))
	w := metric.Value{Kind: metric.Weight, Value: 82, Source: metric.Manual}
	for name, err := range map[string]error{
		"SaveFood":        func() error { _, err := db.SaveFood(t.Context(), ana, nobody, bar(), t0); return err }(),
		"SetFoodArchived": db.SetFoodArchived(t.Context(), ana, nobody, f.ID, true, t0),
		"CreateMeal": func() error {
			_, _, err := db.CreateMeal(t.Context(), ana, nobody, breakfast(t, f, t0), t0)
			return err
		}(),
		"ReplaceMeal": func() error { _, err := db.ReplaceMeal(t.Context(), ana, nobody, m, t0); return err }(),
		"DeleteMeal":  db.DeleteMeal(t.Context(), ana, nobody, m.ID, t0),
		"ExcludeDay":  db.ExcludeDay(t.Context(), ana, nobody, d, day.Exclusion{Reason: "sick"}, t0),
		"IncludeDay":  db.IncludeDay(t.Context(), ana, nobody, d, t0),
		"MergeMetrics": func() error {
			_, err := db.MergeMetrics(t.Context(), ana, nobody, d, []metric.Value{w}, t0)
			return err
		}(),
		"SetMetric":    func() error { _, err := db.SetMetric(t.Context(), ana, nobody, d, w, t0); return err }(),
		"DeleteMetric": func() error { _, err := db.DeleteMetric(t.Context(), ana, nobody, d, metric.Weight, t0); return err }(),
		"UndoLast":     func() error { _, err := db.UndoLast(t.Context(), ana, nobody, t0); return err }(),
	} {
		if err == nil || !strings.Contains(err.Error(), "invalid actor") {
			t.Errorf("%s without a client = %v", name, err)
		}
	}
	if n := countRows(t, db, "events"); n != 2 {
		t.Errorf("%d events, want only the two writes with an actor", n)
	}
}

func TestEventStoreFailures(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	saveFood(t, db, ana, bar())
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithoutCancel(t.Context())
	if _, err := db.Events(ctx, ana, 10); err == nil {
		t.Error("Events on a closed database succeeded")
	}
	if _, err := db.UndoLast(ctx, ana, testActor, t0); err == nil || errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("UndoLast on a closed database = %v", err)
	}
}

// An import that wrote the entity since: undo would lose it, so it
// changes nothing, says so, and passes over that change from then on.
func TestUndoAfterAnImport(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	if err := db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "sick"}, t0); err != nil {
		t.Fatal(err)
	}
	f := saveFood(t, db, ana, bar())
	f.Note = "from the box"
	saveFood(t, db, ana, f) // the user's edit
	imported := f
	imported.Nutrients.Kcal = 380
	if _, err := db.SaveFood(t.Context(), ana, importer, imported, t0); err != nil {
		t.Fatal(err)
	}
	var conflict *store.UndoConflictError
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.As(err, &conflict) || conflict.Event.Action != event.FoodSave {
		t.Fatalf("undo after an import = %v, want an UndoConflictError for the food", err)
	}
	if got, _ := db.FoodByID(t.Context(), ana, f.ID); got.Nutrients.Kcal != 380 {
		t.Errorf("the import's kcal became %v", got.Nutrients.Kcal)
	}
	// Not stuck: the food's create changed since too, and then the day's
	// exclusion is taken back.
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.As(err, &conflict) {
		t.Errorf("the second undo = %v, want the food's create in conflict too", err)
	}
	if e := undo(t, db, ana); e.Action != event.DayExclude {
		t.Errorf("the third undo took back %s, want the exclusion", e.Action)
	}
}

// A food's old name taken by another food since: the rename cannot be
// undone, and undo does not get stuck on it.
func TestUndoRenameAfterTheNameWasTaken(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, day.DateOf(t0), []metric.Value{{Kind: metric.Weight, Value: 82, Source: metric.Manual}}, t0); err != nil {
		t.Fatal(err)
	}
	a := saveFood(t, db, ana, bar())
	renamed := a
	renamed.Name, renamed.Aliases = "Protein bar Salted", nil
	saveFood(t, db, ana, renamed)
	newer := bar() // the old name, a new food, without the old alias
	newer.Aliases = nil
	if _, err := db.SaveFood(t.Context(), ana, importer, newer, t0); err != nil {
		t.Fatal(err)
	}
	var conflict *store.UndoConflictError
	for range 2 { // the rename, then the create
		if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.As(err, &conflict) {
			t.Fatalf("undo = %v, want an UndoConflictError", err)
		}
	}
	if e := undo(t, db, ana); e.Action != event.MetricsMerge {
		t.Errorf("the next undo took back %s, want the weight", e.Action)
	}
	if got, _ := db.FoodByID(t.Context(), ana, a.ID); got.Name != "Protein bar Salted" {
		t.Errorf("the renamed food is %q", got.Name)
	}
}

// The phone's later steps are not lost to an undo of a delete.
func TestUndoMetricAfterAnImport(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	steps := func(n float64) []metric.Value {
		return []metric.Value{{Kind: metric.Steps, Value: n, Source: metric.Import}}
	}
	if _, err := db.MergeMetrics(t.Context(), ana, phone, d, steps(5000), t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteMetric(t.Context(), ana, testActor, d, metric.Steps, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MergeMetrics(t.Context(), ana, phone, d, steps(8000), t0); err != nil {
		t.Fatal(err)
	}
	var conflict *store.UndoConflictError
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.As(err, &conflict) {
		t.Errorf("undo = %v, want an UndoConflictError", err)
	}
	if got, _ := db.Metrics(t.Context(), ana, d, d); got[d][0].Value != 8000 {
		t.Errorf("the phone's steps became %+v", got[d])
	}
}

// A write that leaves the state as it was records nothing, so undo takes
// back a real change.
func TestNoChangeNoEvent(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	f := saveFood(t, db, ana, bar())
	m := createMeal(t, db, ana, breakfast(t, f, t0))
	w := metric.Value{Kind: metric.Weight, Value: 82, Source: metric.Manual}
	if err := db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "sick"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetMetric(t.Context(), ana, testActor, d, w, t0); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, db, "events")
	saveFood(t, db, ana, f)
	if _, err := db.ReplaceMeal(t.Context(), ana, testActor, m, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFoodArchived(t.Context(), ana, testActor, f.ID, false, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "sick"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetMetric(t.Context(), ana, testActor, d, w, t0); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, "events"); n != before {
		t.Errorf("%d events after writes that changed nothing, want %d", n, before)
	}
}

// An undone delete puts the meal back with the time of its create.
func TestUndoDeleteKeepsCreatedAt(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	if err := db.DeleteMeal(t.Context(), ana, testActor, m.ID, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var created string
	if err := db.Read().QueryRowContext(t.Context(), "SELECT created_at FROM meals WHERE user_id = ? AND id = ?",
		ana.String(), m.ID.String()).Scan(&created); err != nil || created != t0.Format(time.RFC3339) {
		t.Errorf("created_at = %q, %v; want the create's %s", created, err, t0.Format(time.RFC3339))
	}
}

// An undone set on a day without the kind leaves the day without it.
func TestUndoSetOnAnEmptyDay(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	if _, err := db.SetMetric(t.Context(), ana, testActor, d, metric.Value{Kind: metric.HRV, Value: 48, Source: metric.Manual}, t0); err != nil {
		t.Fatal(err)
	}
	undo(t, db, ana)
	if got, _ := db.Metrics(t.Context(), ana, d, d); len(got[d]) != 0 {
		t.Errorf("after undoing the set the day holds %+v", got[d])
	}
}

func TestEventsAtMost(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	for i := range store.MaxEvents + 1 {
		if _, err := db.SetMetric(t.Context(), ana, testActor, d, metric.Value{Kind: metric.Steps, Value: float64(i), Source: metric.Manual}, t0); err != nil {
			t.Fatal(err)
		}
	}
	if es, _ := db.Events(t.Context(), ana, store.MaxEvents+50); len(es) != store.MaxEvents {
		t.Errorf("Events = %d, want at most %d", len(es), store.MaxEvents)
	}
}

// An alias taken by another food since: the rename's undo is a conflict,
// not a failure that every later undo meets again.
func TestUndoAfterTheAliasWasTaken(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	if err := db.ExcludeDay(t.Context(), ana, testActor, day.DateOf(t0), day.Exclusion{Reason: "sick"}, t0); err != nil {
		t.Fatal(err)
	}
	a := saveFood(t, db, ana, bar())
	renamed := a
	renamed.Aliases = []string{"salted bar"}
	saveFood(t, db, ana, renamed)
	other := food.Food{
		ID: uuid.NewV7(), Name: "Caramel wafer", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 500, Protein: 6, Carbs: 60, Fat: 25}, Aliases: []string{"caramel bar"},
	}
	if _, err := db.SaveFood(t.Context(), ana, importer, other, t0); err != nil {
		t.Fatal(err)
	}
	var conflict *store.UndoConflictError
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.As(err, &conflict) || !strings.Contains(conflict.Reason, "alias") {
		t.Fatalf("undo = %v, want an UndoConflictError about the alias", err)
	}
	if got, _ := db.FoodByID(t.Context(), ana, a.ID); !reflect.DeepEqual(got.Aliases, []string{"salted bar"}) {
		t.Errorf("the food's aliases became %v", got.Aliases)
	}
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.As(err, &conflict) { // the create: the food changed since
		t.Fatalf("the second undo = %v", err)
	}
	if e := undo(t, db, ana); e.Action != event.DayExclude {
		t.Errorf("the third undo took back %s, want the exclusion", e.Action)
	}
}

// A real clock has nanoseconds and a zone: the state recorded is the
// state read back, so undo finds no false conflict.
func TestUndoWithARealClock(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	now := time.Date(2026, 10, 21, 10, 15, 30, 123456789, time.FixedZone("CEST", 2*3600))
	f, err := db.SaveFood(t.Context(), ana, testActor, bar(), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.CreateMeal(t.Context(), ana, testActor, breakfast(t, f, now), now); err != nil {
		t.Fatal(err)
	}
	d := day.DateOf(now)
	if _, err := db.SetMetric(t.Context(), ana, testActor, d, metric.Value{Kind: metric.Steps, Value: math.Copysign(0, -1), Source: metric.Manual}, now); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := db.UndoLast(t.Context(), ana, testActor, now); err != nil {
			t.Fatalf("undo with a real clock: %v", err)
		}
	}
}
