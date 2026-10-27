package store_test

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// An undo sent again under its id is the same undo: it answers with the
// change it took back, and takes back no other.
func TestUndoSentAgain(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar())
	first := createMeal(t, db, ana, breakfast(t, b, t0))
	second := createMeal(t, db, ana, breakfast(t, b, t0))
	request := clientid.NewAt(t0)
	for i, wantAgain := range []bool{false, true, true} {
		e, again, err := db.Undo(t.Context(), ana, testActor, request, t0)
		if err != nil || again != wantAgain || e.Action != event.MealCreate || e.Entity != second.ID.String() {
			t.Fatalf("undo %d = %+v, %v, %v; want the second meal's create, again %v", i+1, e, again, err, wantAgain)
		}
	}
	if _, err := db.MealByID(t.Context(), ana, first.ID); err != nil {
		t.Errorf("the first meal after the undo sent again: %v, want it kept", err)
	}
	// Another undo takes back the change before.
	if e, again, err := db.Undo(t.Context(), ana, testActor, clientid.NewAt(t0), t0); err != nil || again || e.Entity != first.ID.String() {
		t.Errorf("another undo = %+v, %v, %v; want the first meal's create", e, again, err)
	}
}

// An undo that passed over a change it could not take back, sent again,
// says so again, and takes back no other.
func TestUndoPassedOverSentAgain(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	granola := bar()
	granola.Name, granola.Aliases = "Granola bar", nil
	f := saveFood(t, db, ana, granola)
	f.Note = "the label's"
	// An import corrects the food the user saved last.
	if _, err := db.SaveFood(t.Context(), ana, importer, f, t0); err != nil {
		t.Fatal(err)
	}
	request := clientid.NewAt(t0)
	for i, want := range []store.UndoReason{store.UndoChanged, store.UndoPassedOver} {
		var conflict *store.UndoConflictError
		if _, _, err := db.Undo(t.Context(), ana, testActor, request, t0); !errors.As(err, &conflict) || conflict.Reason != want {
			t.Fatalf("undo %d = %v, want %q", i+1, err, want)
		}
	}
	if _, err := db.MealByID(t.Context(), ana, m.ID); err != nil {
		t.Errorf("the meal after the undo sent again: %v, want it kept", err)
	}
}

// A new undo's id is a version 7 id made near the clock; ids are per
// user.
func TestUndoID(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	ivos := createMeal(t, db, ivo, breakfast(t, saveFood(t, db, ivo, bar()), t0))
	for name, id := range map[string]uuid.UUID{
		"version 4":       uuid.NewV4(),
		"two days old":    clientid.NewAt(t0.Add(-48 * time.Hour)),
		"ahead the clock": clientid.NewAt(t0.Add(time.Hour)),
		"nil":             uuid.Nil(),
	} {
		if _, _, err := db.Undo(t.Context(), ana, testActor, id, t0); !errors.Is(err, store.ErrUndoID) {
			t.Errorf("%s: %v, want ErrUndoID", name, err)
		}
	}
	request := clientid.NewAt(t0)
	if _, _, err := db.Undo(t.Context(), ana, testActor, request, t0); err != nil {
		t.Fatal(err)
	}
	if e, again, err := db.Undo(t.Context(), ivo, testActor, request, t0); err != nil || again || e.Entity != ivos.ID.String() {
		t.Errorf("Ivo's undo under Ana's undo's id = %+v, %v, %v; want his meal's create", e, again, err)
	}
}

// The same undo sent many times at once takes back one change.
func TestUndoSentAtOnce(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar())
	for range 3 {
		createMeal(t, db, ana, breakfast(t, b, t0))
	}
	request := clientid.NewAt(t0)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, _, err := db.Undo(t.Context(), ana, testActor, request, t0); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	es, err := db.Events(t.Context(), ana, 20)
	if err != nil {
		t.Fatal(err)
	}
	undos := 0
	for _, e := range es {
		if e.Action == event.Undo {
			undos++
		}
	}
	if undos != 1 {
		t.Errorf("%d undos, want 1", undos)
	}
}

// An undo that found nothing, sent again after a change since, finds
// nothing again: it takes back no change made after it.
func TestUndoOfNothingSentAgain(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	request := clientid.NewAt(t0)
	if _, _, err := db.Undo(t.Context(), ana, testActor, request, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Fatalf("an undo of nothing = %v, want ErrNothingToUndo", err)
	}
	m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	if _, _, err := db.Undo(t.Context(), ana, testActor, request, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("the undo of nothing again = %v, want ErrNothingToUndo", err)
	}
	if _, err := db.MealByID(t.Context(), ana, m.ID); err != nil {
		t.Errorf("the meal after the undo of nothing again: %v, want it kept", err)
	}
	// UndoLast, under no id, records nothing of an undo of nothing.
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); err != nil {
		t.Fatal(err)
	}
	before, _ := db.Events(t.Context(), ana, store.MaxEvents)
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Fatalf("UndoLast of nothing = %v", err)
	}
	if after, _ := db.Events(t.Context(), ana, store.MaxEvents); len(after) != len(before) {
		t.Errorf("UndoLast of nothing recorded %d events", len(after)-len(before))
	}
}

// A failure to record an undo that found nothing is a failure, not
// "nothing to undo": the undo sent again must not take back a change
// made since as if it were new.
func TestUndoOfNothingNotRecorded(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	exec := func(q string) {
		t.Helper()
		if err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
			_, err := tx.ExecContext(t.Context(), q)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TRIGGER no_nothing BEFORE INSERT ON events WHEN NEW.entity = 'nothing'
		BEGIN SELECT RAISE(ABORT, 'the disk is full'); END`)
	request := clientid.NewAt(t0)
	if _, _, err := db.Undo(t.Context(), ana, testActor, request, t0); err == nil || errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("an undo of nothing that could not be recorded = %v, want a failure", err)
	}
	exec("DROP TRIGGER no_nothing")
	m := createMeal(t, db, ana, breakfast(t, saveFood(t, db, ana, bar()), t0))
	// The failure kept nothing: the undo sent again is a new one, and
	// takes back the meal, which is what a client that saw the failure
	// asks for.
	if e, again, err := db.Undo(t.Context(), ana, testActor, request, t0); err != nil || again || e.Entity != m.ID.String() {
		t.Errorf("the undo again = %+v, %v, %v", e, again, err)
	}
}

// An undo that names the change it means takes it back only while it is
// the latest: after another change, of any client, it takes back
// nothing, and the change stays for an undo that names it once it is
// the latest again.
func TestUndoOfTheLatest(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	b := saveFood(t, db, ana, bar())
	first := createMeal(t, db, ana, breakfast(t, b, t0))
	second := createMeal(t, db, ana, breakfast(t, b, t0))
	ofFirst := store.Change{Action: event.MealCreate, Entity: first.ID.String()}
	request := clientid.NewAt(t0)
	var other *store.NotLatestError
	if _, _, err := db.UndoOf(t.Context(), ana, testActor, request, ofFirst, t0); !errors.As(err, &other) ||
		other.Latest.Action != event.MealCreate || other.Latest.Entity != second.ID.String() {
		t.Fatalf("undo of the first meal = %v, want the second meal named as the latest", err)
	}
	for _, m := range []meal.Meal{first, second} {
		if _, err := db.MealByID(t.Context(), ana, m.ID); err != nil {
			t.Errorf("meal %s after the undo of another: %v, want it kept", m.ID, err)
		}
	}
	// The second taken back, the first is the latest, and the same
	// request takes it back.
	if _, _, err := db.Undo(t.Context(), ana, testActor, clientid.NewAt(t0), t0); err != nil {
		t.Fatal(err)
	}
	if e, again, err := db.UndoOf(t.Context(), ana, testActor, request, ofFirst, t0); err != nil || again || e.Entity != first.ID.String() {
		t.Errorf("undo of the first meal, the latest = %+v, %v, %v; want it taken back", e, again, err)
	}
	// Taken back, it is no longer the latest: the food is.
	if _, _, err := db.UndoOf(t.Context(), ana, testActor, clientid.NewAt(t0), ofFirst, t0); !errors.As(err, &other) || other.Latest.Action != event.FoodSave {
		t.Errorf("undo of the first meal again = %v, want the food named as the latest", err)
	}
}
