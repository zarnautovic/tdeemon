package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// The workouts in testdata/strong are made up.

// utc4 is a calendar in UTC whose day starts at 04:00.
func utc4(t *testing.T) day.Calendar {
	t.Helper()
	c, err := day.NewCalendar("UTC", 4)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A workout belongs to the day of its start: one past midnight, before the
// day starts, to the day before, in the user's zone; a date alone, to
// itself.
func TestParseStrongDays(t *testing.T) {
	t.Parallel()

	zagreb, err := day.NewCalendar("Europe/Zagreb", 4)
	if err != nil {
		t.Fatal(err)
	}
	const header = "Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n"
	for start, want := range map[string]string{
		"2026-10-20 00:30:00": "2026-10-19",
		"2026-10-20 03:00:00": "2026-10-19", // on the wall clock in Zagreb, not in UTC
		"2026-10-20 04:00:00": "2026-10-20",
		"2026-10-20":          "2026-10-20",
		"2026-10-25 02:30:00": "2026-10-24", // the night summer time ends
	} {
		w, err := ParseStrong(strings.NewReader(header+start+",Day A,1h,Squat,1,90,5,0\n"), zagreb)
		if err != nil || len(w.Days) != 1 || w.Days[0].String() != want {
			t.Errorf("a start at %s gave %+v, %v; want %s", start, w, err, want)
		}
	}
}

// Two workouts on a day are one day; a rest timer is no set.
func TestParseStrong(t *testing.T) {
	t.Parallel()

	f, err := os.Open(filepath.Join("testdata", "strong", "export.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	w, err := ParseStrong(f, utc4(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []day.Date{{Year: 2026, Month: 10, Day: 19}, {Year: 2026, Month: 10, Day: 21}}
	if w.Count != 3 || len(w.Days) != 2 || w.Days[0] != want[0] || w.Days[1] != want[1] {
		t.Errorf("ParseStrong = %+v, want 3 workouts on %v", w, want)
	}
}

// A header with a byte-order mark and columns in another order reads;
// only rest timers, or no rows, are no workouts.
func TestParseStrongShapes(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		csv   string
		count int
	}{
		"a mark and another order": {"\ufeffWorkout Name,Date,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\nDay C,2026-10-23 07:00:00,45m,Row,1,50,10,0\n", 1},
		"rest timers alone":        {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n2026-10-23 07:00:00,Day C,45m,Row,Rest Timer,0,0,90\n", 0},
		"no rows":                  {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n", 0},
	} {
		w, err := ParseStrong(strings.NewReader(c.csv), utc4(t))
		if err != nil || w.Count != c.count || len(w.Days) != c.count {
			t.Errorf("%s: %+v, %v", name, w, err)
		}
	}
}

// What is no export of Strong's fails whole, so a change of its format
// shows.
func TestParseStrongRefuses(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct{ csv, want string }{
		"empty":                  {"", "empty"},
		"a column missing":       {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps\n", `no column "Seconds"`},
		"a short date":           {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n21.10.,Day A,1h,Squat,1,90,5,0\n", "line 2"},
		"a time in another form": {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n2026-10-21T06:55:30Z,Day A,1h,Squat,1,90,5,0\n", `date "2026-10-21T06:55:30Z"`},
		"a date that reads":      {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n2026-13-01 07:00:00,Day A,1h,Squat,1,90,5,0\n", `date "2026-13-01 07:00:00"`},
		"a broken row":           {"Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n2026-10-21 07:00:00,Day A\n", "wrong number of fields"},
		"a broken header":        {"\"Date,Workout\n", "extraneous"},
	} {
		if _, err := ParseStrong(strings.NewReader(c.csv), utc4(t)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}
