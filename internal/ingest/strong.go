package ingest

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// strongColumns are the columns of Strong's export ParseStrong needs: the
// ones that name a workout and its sets. A column missing fails the whole
// export, so a change of Strong's format shows as an error, not as wrong
// days. The training module (P3) reads the sets from the latest export
// kept (Archive.SaveLatest).
var strongColumns = []string{"Date", "Workout Name", "Duration", "Exercise Name", "Set Order", "Weight", "Reps", "Seconds"}

// Workouts is what a Strong export gives tdeemon until the training
// module: how many workouts it holds, and the days they were on, in
// order.
type Workouts struct {
	Count int
	Days  []day.Date
}

// ParseStrong reads the Strong app's CSV export, one row a set, which
// holds the whole history. A workout is its start, to the second, which
// Strong writes on the phone's wall clock; it belongs to the day of cal
// that start falls in, as a meal does, so a workout after midnight and
// before the day's start hour is the day before's. A start whose time
// does not read gives its calendar date. A row of a rest timer is no set.
func ParseStrong(r io.Reader, cal day.Calendar) (Workouts, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return Workouts{}, errors.New("strong export: empty")
	}
	if err != nil {
		return Workouts{}, fmt.Errorf("strong export: %w", err)
	}
	col := map[string]int{}
	for i, name := range header {
		col[strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))] = i
	}
	for _, name := range strongColumns {
		if _, ok := col[name]; !ok {
			return Workouts{}, fmt.Errorf("strong export: no column %q (has Strong's format changed?)", name)
		}
	}
	workouts := map[string]bool{}
	days := map[day.Date]bool{}
	for line := 2; ; line++ {
		row, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Workouts{}, fmt.Errorf("strong export: %w", err)
		}
		field := func(name string) string { return strings.TrimSpace(row[col[name]]) }
		if field("Set Order") == "Rest Timer" {
			continue
		}
		start := field("Date")
		d, err := strongDay(start, cal)
		if err != nil {
			return Workouts{}, fmt.Errorf("strong export, line %d: %w", line, err)
		}
		workouts[start] = true
		days[d] = true
	}
	out := Workouts{Count: len(workouts), Days: make([]day.Date, 0, len(days))}
	for d := range days {
		out.Days = append(out.Days, d)
	}
	slices.SortFunc(out.Days, func(a, b day.Date) int { return strings.Compare(a.String(), b.String()) })
	return out, nil
}

// strongTime is the form of a workout's start in Strong's export.
const strongTime = "2006-01-02 15:04:05"

// strongDay returns the day of cal a workout's start falls in.
func strongDay(s string, cal day.Calendar) (day.Date, error) {
	if t, err := time.ParseInLocation(strongTime, s, cal.Location); err == nil {
		return cal.DayOf(t), nil
	}
	if len(s) < 10 {
		return day.Date{}, fmt.Errorf("date %q: want 2006-01-02 15:04:05", s)
	}
	d, err := day.ParseDate(s[:10])
	if err != nil {
		return day.Date{}, fmt.Errorf("date %q: want 2006-01-02 15:04:05", s)
	}
	return d, nil
}
