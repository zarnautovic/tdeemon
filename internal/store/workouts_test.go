package store_test

import (
	"reflect"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/store"
)

var strongActor = event.Actor{Kind: event.Import, Client: "strong"}

// setWorkouts sets ana's days of workouts from Strong to ds and returns
// what it added and removed.
func setWorkouts(t *testing.T, db *store.DB, userID uuid.UUID, ds ...day.Date) ([]day.Date, []day.Date) {
	t.Helper()
	added, removed, err := db.SetWorkoutDays(t.Context(), userID, strongActor, store.Strong, ds, t0)
	if err != nil {
		t.Fatalf("SetWorkoutDays: %v", err)
	}
	return added, removed
}

// checkWorkoutsEvent checks that the user's last event is the import of
// Strong's days, with date among them.
func checkWorkoutsEvent(t *testing.T, db *store.DB, userID uuid.UUID, date string) {
	t.Helper()
	es, err := db.Events(t.Context(), userID, 1)
	if err != nil || len(es) != 1 || es[0].Action != event.WorkoutsSet || es[0].Entity != "strong" || es[0].Actor != strongActor ||
		!strings.Contains(string(es[0].After), date) {
		t.Errorf("the event = %+v, %v", es, err)
	}
}

// An import replaces the source's days: it says what it added and
// removed, and records one event; the same days again record nothing.
func TestWorkoutDays(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	added, removed := setWorkouts(t, db, ana, d.AddDays(-2), d, d.AddDays(-5), d)
	if !reflect.DeepEqual(added, []day.Date{d.AddDays(-5), d.AddDays(-2), d}) || removed != nil {
		t.Errorf("the first import added %v, removed %v", added, removed)
	}
	got, err := db.WorkoutDays(t.Context(), ana, d.AddDays(-30), d)
	if err != nil || !reflect.DeepEqual(got, map[day.Date]bool{d.AddDays(-5): true, d.AddDays(-2): true, d: true}) {
		t.Errorf("days = %v, %v", got, err)
	}
	checkWorkoutsEvent(t, db, ana, "2026-10-16")

	n := countRows(t, db, "events")
	if added, removed := setWorkouts(t, db, ana, d, d.AddDays(-2), d.AddDays(-5)); added != nil || removed != nil || countRows(t, db, "events") != n {
		t.Errorf("the same days again added %v, removed %v", added, removed)
	}
	added, removed = setWorkouts(t, db, ana, d.AddDays(-2), d.AddDays(-1))
	if !reflect.DeepEqual(added, []day.Date{d.AddDays(-1)}) || !reflect.DeepEqual(removed, []day.Date{d.AddDays(-5), d}) {
		t.Errorf("the newer export added %v, removed %v", added, removed)
	}
	// The days from one day to another, these included.
	got, err = db.WorkoutDays(t.Context(), ana, d.AddDays(-1), d)
	if err != nil || !reflect.DeepEqual(got, map[day.Date]bool{d.AddDays(-1): true}) {
		t.Errorf("the last two days = %v, %v", got, err)
	}
}

// The days of workouts leave the check-in's toggle and another user's
// days alone, and undo passes over them.
func TestWorkoutDaysLeaveTheRest(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	d := day.DateOf(t0)
	if err := db.SetTrainingDay(t.Context(), ana, testActor, d.AddDays(-2), new(false), t0); err != nil {
		t.Fatal(err)
	}
	setWorkouts(t, db, ivo, d.AddDays(-9))
	setWorkouts(t, db, ana, d.AddDays(-2), d)
	if toggles, _ := db.TrainingDayToggles(t.Context(), ana, d.AddDays(-7), d); !reflect.DeepEqual(toggles, map[day.Date]bool{d.AddDays(-2): false}) {
		t.Errorf("toggles = %v", toggles)
	}
	if got, _ := db.WorkoutDays(t.Context(), ivo, d.AddDays(-30), d); !reflect.DeepEqual(got, map[day.Date]bool{d.AddDays(-9): true}) {
		t.Errorf("Ivo's days = %v", got)
	}
	// Undo passes over the days of workouts, even when no import wrote
	// them.
	if _, _, err := db.SetWorkoutDays(t.Context(), ana, testActor, store.Strong, []day.Date{d}, t0); err != nil {
		t.Fatal(err)
	}
	if e := undo(t, db, ana); e.Action != event.TrainingDay {
		t.Errorf("undo took back %s, want the toggle", e.Action)
	}
	if got, _ := db.WorkoutDays(t.Context(), ana, d.AddDays(-30), d); len(got) != 1 {
		t.Errorf("days after undo = %v", got)
	}
}

func TestWorkoutDaysRefuses(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	for name, err := range map[string]error{
		"no actor":   second(db.SetWorkoutDays(t.Context(), ana, event.Actor{}, store.Strong, nil, t0)),
		"a source":   second(db.SetWorkoutDays(t.Context(), ana, strongActor, "garmin", nil, t0)),
		"a zero day": second(db.SetWorkoutDays(t.Context(), ana, strongActor, store.Strong, []day.Date{{}}, t0)),
	} {
		if err == nil {
			t.Errorf("%s: SetWorkoutDays succeeded", name)
		}
	}
	if countRows(t, db, "workout_days") != 0 || countRows(t, db, "events") != 0 {
		t.Error("a refused import wrote")
	}
	var nothing []day.Date
	if _, _, err := db.SetWorkoutDays(t.Context(), ana, strongActor, store.Strong, nothing, t0); err != nil || countRows(t, db, "events") != 0 {
		t.Errorf("no days on none: %v", err)
	}
}

func second(_, _ []day.Date, err error) error { return err }
