package store_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/store"
)

var checkInDay = day.Date{Year: 2026, Month: 10, Day: 21}

// A check-in stores what the user says of a day; a check-in that changes
// part of it keeps the rest; it is one event; and another user's day is
// not touched.
func TestCheckIn(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	full := store.DayCheckIn{Mood: 4, Hunger: 2, Note: "slept well", TrainingDay: new(true), ExcludedReason: "restaurant"}
	got, err := db.CheckIn(t.Context(), ana, testActor, checkInDay, func(store.DayCheckIn) store.DayCheckIn { return full }, t0)
	if err != nil || !reflect.DeepEqual(got, full) {
		t.Fatalf("CheckIn = %+v, %v; want %+v", got, err, full)
	}
	if read, err := db.CheckInOf(t.Context(), ana, checkInDay); err != nil || !reflect.DeepEqual(read, full) {
		t.Errorf("CheckInOf = %+v, %v; want %+v", read, err, full)
	}
	if other, err := db.CheckInOf(t.Context(), ivo, checkInDay); err != nil || !reflect.DeepEqual(other, store.DayCheckIn{}) {
		t.Errorf("the other user's day = %+v, %v; want nothing said", other, err)
	}
	// A change of the mood alone, and of nothing.
	moodOnly := func(c store.DayCheckIn) store.DayCheckIn { c.Mood = 2; return c }
	if _, err := db.CheckIn(t.Context(), ana, testActor, checkInDay, moodOnly, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CheckIn(t.Context(), ana, testActor, checkInDay, moodOnly, t0); err != nil {
		t.Fatal(err)
	}
	es, err := db.Events(t.Context(), ana, 10)
	if err != nil || len(es) != 2 || es[0].Action != event.DayCheckIn || es[0].Entity != checkInDay.String() {
		t.Fatalf("events = %+v, %v; want two check-ins, the second change recording nothing", es, err)
	}
	want := full
	want.Mood = 2
	if read, _ := db.CheckInOf(t.Context(), ana, checkInDay); !reflect.DeepEqual(read, want) {
		t.Errorf("after a change of the mood = %+v, want %+v", read, want)
	}
}

// One undo takes a check-in back whole: the mood of the second; the
// whole first, again.
func TestCheckInUndone(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	full := store.DayCheckIn{Mood: 4, Hunger: 2, Note: "slept well", TrainingDay: new(true), ExcludedReason: "restaurant"}
	for _, change := range []func(store.DayCheckIn) store.DayCheckIn{
		func(store.DayCheckIn) store.DayCheckIn { return full },
		func(c store.DayCheckIn) store.DayCheckIn { c.Mood = 2; return c },
	} {
		if _, err := db.CheckIn(t.Context(), ana, testActor, checkInDay, change, t0); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []store.DayCheckIn{full, {}} {
		if _, err := db.UndoLast(t.Context(), ana, testActor, t0); err != nil {
			t.Fatal(err)
		}
		if read, _ := db.CheckInOf(t.Context(), ana, checkInDay); !reflect.DeepEqual(read, want) {
			t.Errorf("undone = %+v, want %+v", read, want)
		}
	}
}

// A check-in out of bounds is refused, and stores nothing.
func TestCheckInRefuses(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	for _, tc := range []struct {
		name string
		c    store.DayCheckIn
		want string
	}{
		{"a mood off the scale", store.DayCheckIn{Mood: 6}, "mood 6"},
		{"a reason too long", store.DayCheckIn{ExcludedReason: strings.Repeat("a", day.MaxReasonLen+1)}, "reason"},
	} {
		if _, err := db.CheckIn(t.Context(), ana, testActor, checkInDay, func(store.DayCheckIn) store.DayCheckIn { return tc.c }, t0); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
	same := func(c store.DayCheckIn) store.DayCheckIn { c.Mood = 3; return c }
	if _, err := db.CheckIn(t.Context(), ana, testActor, day.Date{}, same, t0); err == nil || !strings.Contains(err.Error(), "the day is missing") {
		t.Errorf("a check-in of no day: %v", err)
	}
	if _, err := db.CheckIn(t.Context(), ana, event.Actor{}, checkInDay, same, t0); err == nil || !strings.Contains(err.Error(), "invalid actor") {
		t.Errorf("a check-in of no one: %v", err)
	}
	if read, err := db.CheckInOf(t.Context(), ana, checkInDay); err != nil || !reflect.DeepEqual(read, store.DayCheckIn{}) {
		t.Errorf("after the refusals = %+v, %v; want nothing", read, err)
	}
}

// Check-ins sent at once are each made to the other's result.
func TestCheckInsAtOnce(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	var wg sync.WaitGroup
	for _, change := range []func(store.DayCheckIn) store.DayCheckIn{
		func(c store.DayCheckIn) store.DayCheckIn { c.Mood = 3; return c },
		func(c store.DayCheckIn) store.DayCheckIn { c.Hunger = 4; return c },
		func(c store.DayCheckIn) store.DayCheckIn { c.Note = "busy"; return c },
	} {
		wg.Go(func() {
			if _, err := db.CheckIn(t.Context(), ana, testActor, checkInDay, change, t0); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	want := store.DayCheckIn{Mood: 3, Hunger: 4, Note: "busy"}
	if read, _ := db.CheckInOf(t.Context(), ana, checkInDay); !reflect.DeepEqual(read, want) {
		t.Errorf("after check-ins at once = %+v, want %+v", read, want)
	}
}

// A day a check-in excluded was decided: an import leaves it as it is.
func TestDayExclusionRecordedByACheckIn(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	other := checkInDay.AddDays(1)
	for d, reason := range map[day.Date]string{checkInDay: "sick", other: ""} {
		if _, err := db.CheckIn(t.Context(), ana, testActor, d, func(c store.DayCheckIn) store.DayCheckIn {
			c.Mood, c.ExcludedReason = 2, reason
			return c
		}, t0); err != nil {
			t.Fatal(err)
		}
	}
	for d, want := range map[day.Date]bool{checkInDay: true, other: false} {
		if got, err := db.DayExclusionRecorded(t.Context(), ana, d); err != nil || got != want {
			t.Errorf("DayExclusionRecorded(%s) = %v, %v; want %v", d, got, err, want)
		}
	}
}

// A check-in that changes nothing writes nothing: no empty row of a day.
func TestCheckInOfNothing(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	if _, err := db.CheckIn(t.Context(), ana, testActor, checkInDay, func(c store.DayCheckIn) store.DayCheckIn { return c }, t0); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.Read().QueryRowContext(t.Context(), "SELECT count(*) FROM days WHERE user_id = ?", ana.String()).Scan(&n); err != nil || n != 0 {
		t.Errorf("days after a check-in of nothing = %d, %v; want none", n, err)
	}
}
