// Package history imports a user's history from the predecessor, once
// (DESIGN.md §10): its database, an index of the vault's daily notes,
// gives the days' metrics and meals. The import is idempotent: the ids
// of its meals come from the day and the meal's place in it, and the
// metrics merge as an import, so running it again changes nothing.
package history

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/text"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/ingest"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// Actor is who the import writes as.
var Actor = event.Actor{Kind: event.Import, Client: "history"}

// RestName names the estimate that brings a day's meals up to the day's
// total, where the note's total was more than its meals.
const RestName = "Rest of the day (history import)"

// rolloverHour is when the predecessor closed a day's note: until then a
// meal went into the note of the day before, so a meal before it is past
// midnight.
const rolloverHour = 5

// metricColumns maps the predecessor's columns of a day to the kinds of
// metrics. A day's mood, hunger and status are left out; its remark
// excludes it (Day.Remark).
var metricColumns = []struct{ column, kind string }{
	{"kilaza", "weight"},
	{"masnoca", "body_fat"},
	{"misicna_masa", "lean_mass"},
	{"koraci", "steps"},
	{"aktivne_kalorije", "active_kcal"},
	{"bazalne_kalorije", "basal_kcal"},
	{"san", "sleep_h"},
	{"rhr", "rhr"},
	{"hrv", "hrv"},
	{"vo2max", "vo2max"},
	{"trening_min", "exercise_min"},
}

// Day is a day of the predecessor's.
type Day struct {
	Date    string
	Metrics map[string]float64 // by the names of metric.Kinds
	// Total is the day's intake as its note gave it; nil when it gave
	// none.
	Total *food.Nutrients
	Meals []Meal
	// Remark is the note's remark, a restaurant, an illness, a trip: the
	// predecessor's calibration left such a day out, and the import
	// excludes it.
	Remark string
}

// Meal is a meal of the predecessor's: a line of its day's note.
type Meal struct {
	// Time is the wall clock it was eaten at, "13:05", or "".
	Time        string
	Description string
	Nutrients   food.Nutrients
}

// Read returns the days of the predecessor's database at path, opened
// read-only, in order.
func Read(ctx context.Context, path string) ([]Day, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no database at %s", path)
	}
	// A URI, so that a path with # or ? in it stays the path, of the
	// absolute path, as a relative one would read as the URI's authority;
	// read-only, so nothing of the predecessor's changes.
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("the predecessor's database: %w", err)
	}
	uri := url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro"}
	old, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("the predecessor's database: %w", err)
	}
	defer func() { _ = old.Close() }()
	days, byDate, err := readDays(ctx, old)
	if err != nil {
		return nil, fmt.Errorf("the predecessor's days: %w", err)
	}
	if days, err = readMeals(ctx, old, days, byDate); err != nil {
		return nil, fmt.Errorf("the predecessor's meals: %w", err)
	}
	return days, nil
}

func readDays(ctx context.Context, old *sql.DB) ([]Day, map[string]int, error) {
	cols := []string{"date", "kalorije", "proteini", "ugljikohidrati", "masti"}
	for _, c := range metricColumns {
		cols = append(cols, c.column)
	}
	rows, err := old.QueryContext(ctx, "SELECT "+strings.Join(cols, ", ")+", coalesce(napomena, '') FROM days ORDER BY date") //nolint:gosec // G202: the columns are the constants above
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	var days []Day
	byDate := map[string]int{}
	for rows.Next() {
		var date, remark string
		vals := make([]sql.NullFloat64, len(cols)-1)
		dest := []any{&date}
		for i := range vals {
			dest = append(dest, &vals[i])
		}
		if err := rows.Scan(append(dest, &remark)...); err != nil {
			return nil, nil, err
		}
		d := Day{Date: date, Metrics: map[string]float64{}, Remark: strings.TrimSpace(remark)}
		if vals[0].Valid {
			d.Total = &food.Nutrients{Kcal: vals[0].Float64, Protein: vals[1].Float64, Carbs: vals[2].Float64, Fat: vals[3].Float64}
		}
		for i, c := range metricColumns {
			if v := vals[4+i]; v.Valid {
				d.Metrics[c.kind] = v.Float64
			}
		}
		byDate[date] = len(days)
		days = append(days, d)
	}
	return days, byDate, rows.Err()
}

func readMeals(ctx context.Context, old *sql.DB, days []Day, byDate map[string]int) ([]Day, error) {
	rows, err := old.QueryContext(ctx, `SELECT date, coalesce(time, ''), coalesce(description, ''),
		coalesce(kcal, 0), coalesce(proteini, 0), coalesce(ugljikohidrati, 0), coalesce(masti, 0)
		FROM meals ORDER BY date, rowid`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var date string
		var m Meal
		n := &m.Nutrients
		if err := rows.Scan(&date, &m.Time, &m.Description, &n.Kcal, &n.Protein, &n.Carbs, &n.Fat); err != nil {
			return nil, err
		}
		i, ok := byDate[date]
		if !ok {
			// A note whose front matter did not read still has its meals.
			i = len(days)
			byDate[date] = i
			days = append(days, Day{Date: date, Metrics: map[string]float64{}})
		}
		days[i].Meals = append(days[i].Meals, m)
	}
	return days, rows.Err()
}

// Report is what an import did, or with DryRun would do.
type Report struct {
	Days    int
	Metrics int
	// Meals counts the meals the history holds, created now or by a run
	// before; Rests the estimates of a day's rest among them.
	Meals, Rests int
	// Excluded are the days excluded, each with why: a remark, or no
	// intake.
	Excluded []string
	// Rejected are the values left out, by their rule.
	Rejected []ingest.Rejected
	// Weighed are the weigh-ins the days bring, by their day, the ones
	// left out not among them: a dry run holds a plan to them, as it
	// writes no days.
	Weighed map[day.Date]float64
	// Over are the days whose meals have more kcal than the day's total;
	// their meals stay, with no rest.
	Over []string
	// Renamed are the meals whose description was cut or replaced to
	// meet the rules of a name.
	Renamed []string
	// Skipped are the days that did not read, the meals deleted since an
	// import before, which stay deleted, and the meals of an import before
	// that differ now (the note, the time zone or the day's start
	// changed), which stay as they are.
	Skipped []string
}

// Import writes days into u's history as an import, or with dryRun only
// counts what it would write. It stops at the first write that fails;
// running it again picks up where it stopped.
func Import(ctx context.Context, db *store.DB, u user.User, days []Day, now time.Time, dryRun bool) (Report, error) {
	cal, err := day.NewCalendar(u.TimeZone, u.DayStartHour)
	if err != nil {
		return Report{}, fmt.Errorf("the user's calendar: %w", err)
	}
	var r Report
	metrics := map[string]map[string]float64{}
	for _, d := range days {
		if _, err := day.ParseDate(d.Date); err != nil {
			r.Skipped = append(r.Skipped, fmt.Sprintf("%q: %v", d.Date, err))
			continue
		}
		r.Days++
		if len(d.Metrics) > 0 {
			metrics[d.Date] = d.Metrics
		}
	}
	res, err := ingest.FromDays(metrics)
	if err != nil {
		return r, err
	}
	r.Rejected = res.Rejected
	r.Weighed = map[day.Date]float64{}
	for _, d := range res.Days {
		r.Metrics += len(d.Values)
		for _, v := range d.Values {
			if v.Kind == metric.Weight {
				r.Weighed[d.Date] = v.Value
			}
		}
		if dryRun {
			continue
		}
		if _, err := db.MergeMetrics(ctx, u.ID, Actor, d.Date, d.Values, now); err != nil {
			return r, fmt.Errorf("the metrics of %s: %w", d.Date, err)
		}
	}
	for _, d := range days {
		date, err := day.ParseDate(d.Date)
		if err != nil {
			continue
		}
		meals, err := importMeals(ctx, db, u.ID, cal, date, d, now, dryRun, &r)
		if err != nil {
			return r, err
		}
		if err := exclude(ctx, db, u.ID, cal, date, d, meals, now, dryRun, &r); err != nil {
			return r, err
		}
	}
	return r, nil
}

// exclude excludes date for its remark, or as a day without intake, if
// it is one of those.
func exclude(ctx context.Context, db *store.DB, userID uuid.UUID, cal day.Calendar, date day.Date, d Day, meals int, now time.Time,
	dryRun bool, r *Report,
) error {
	reason, err := exclusionReason(ctx, db, userID, cal, date, d, meals, now)
	if err != nil || reason == "" {
		return err
	}
	// A day excluded before, by a run before or by the user, and included
	// since or not, stays as it is.
	decided, err := db.DayExclusionRecorded(ctx, userID, date)
	if err != nil {
		return err
	}
	if decided {
		ex, err := db.Exclusions(ctx, userID, date, date)
		if err != nil {
			return err
		}
		if ex[date].Reason != reason {
			r.Skipped = append(r.Skipped, fmt.Sprintf("%s: in or out of calibration as decided before, which stays", date))
		}
		return nil
	}
	fitted, replaced := fitName(reason, day.MaxReasonLen, "Remark of the predecessor's note")
	if replaced && fitted == "Remark of the predecessor's note" {
		r.Renamed = append(r.Renamed, d.Date+": a remark that is no reason, replaced")
	}
	r.Excluded = append(r.Excluded, d.Date+": "+fitted)
	if dryRun {
		return nil
	}
	if err := db.ExcludeDay(ctx, userID, Actor, date, day.Exclusion{Reason: fitted}, now); err != nil {
		return fmt.Errorf("excluding %s: %w", date, err)
	}
	return nil
}

// Why a day is excluded that has no intake: none in the note, or none
// that could be imported.
const (
	NoIntakeReason     = "No intake logged in the predecessor"
	UnimportableReason = "Intake of the predecessor not importable"
)

// exclusionReason returns why date is out of calibration, or "": its
// remark, its spaces joined; else, when the day has no meal after the
// import (meals), it is over, and the user logged none of their own in
// tdeemon, no intake.
func exclusionReason(ctx context.Context, db *store.DB, userID uuid.UUID, cal day.Calendar, date day.Date, d Day, meals int,
	now time.Time,
) (string, error) {
	if remark := strings.Join(strings.Fields(d.Remark), " "); remark != "" {
		return remark, nil
	}
	if meals > 0 || now.Before(cal.End(date)) {
		return "", nil
	}
	own, err := db.MealsOfDay(ctx, userID, date)
	if err != nil || len(own) > 0 {
		return "", err
	}
	if len(d.Meals) > 0 || d.Total != nil && d.Total.Kcal > 0 {
		return UnimportableReason, nil
	}
	return NoIntakeReason, nil
}

// importMeals writes a day's meals, and the estimate of its rest when its
// total was more than its meals, and returns how many it gives the day:
// written now, by a run before, or deleted since by the user, who decided
// the day then.
func importMeals(ctx context.Context, db *store.DB, userID uuid.UUID, cal day.Calendar, date day.Date, d Day, now time.Time,
	dryRun bool, r *Report,
) (int, error) {
	ms := buildMeals(userID, cal, date, d, r)
	if d.Total != nil && !slices.ContainsFunc(ms, func(m meal.Meal) bool { return m.Description == RestName }) && !dryRun {
		// A rest a run before wrote stays, though the meals reach the
		// total now.
		if _, err := db.MealByID(ctx, userID, mealID(userID, date, "rest")); err == nil {
			r.Skipped = append(r.Skipped, fmt.Sprintf("%s, %s: of a run before, stays though the meals reach the total now", d.Date, RestName))
		}
	}
	for i := range ms {
		m := &ms[i]
		r.Meals++
		if dryRun {
			continue
		}
		_, _, err := db.CreateMeal(ctx, userID, Actor, *m, now)
		switch {
		case errors.Is(err, store.ErrMealDeleted):
			r.Meals--
			r.Skipped = append(r.Skipped, fmt.Sprintf("%s, %s: deleted since an import before", d.Date, m.Description))
			continue
		case errors.Is(err, store.ErrMealConflict):
			r.Skipped = append(r.Skipped, fmt.Sprintf("%s, %s: differs from an import before, which stays as it is", d.Date, m.Description))
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("a meal of %s: %w", d.Date, err)
		}
	}
	return len(ms), nil
}

// buildMeals returns the meals of a day's note, and the estimate of its
// rest when its total was more than its meals.
func buildMeals(userID uuid.UUID, cal day.Calendar, date day.Date, d Day, r *Report) []meal.Meal {
	var sum food.Nutrients
	ms := make([]meal.Meal, 0, len(d.Meals)+1)
	for i, m := range d.Meals {
		desc, renamed := fitName(m.Description, meal.MaxNameLen, "Meal")
		if renamed {
			r.Renamed = append(r.Renamed, d.Date+": "+desc)
		}
		item, err := meal.Estimate(desc, m.Nutrients, "")
		if err != nil {
			r.Skipped = append(r.Skipped, fmt.Sprintf("%s, %s: %v", d.Date, desc, err))
			continue
		}
		sum = add(sum, m.Nutrients)
		ms = append(ms, meal.Meal{
			ID: mealID(userID, date, strconv.Itoa(i)), EatenAt: eatenAt(cal, date, m.Time), Day: date,
			Description: desc, Client: meal.ClientImport, Items: []meal.Item{item},
		})
	}
	if d.Total == nil {
		return ms
	}
	rest, over := restOf(*d.Total, sum)
	if over {
		r.Over = append(r.Over, d.Date)
	}
	if rest == nil {
		return ms
	}
	item, err := meal.Estimate(RestName, *rest, "")
	if err != nil {
		r.Skipped = append(r.Skipped, fmt.Sprintf("%s, %s: %v", d.Date, RestName, err))
		return ms
	}
	r.Rests++
	return append(ms, meal.Meal{
		ID: mealID(userID, date, "rest"), EatenAt: cal.End(date).Add(-time.Minute), Day: date,
		Description: RestName, Client: meal.ClientImport, Items: []meal.Item{item},
	})
}

// restOf returns what total holds beyond sum, each part at least 0 (a
// part the meals pass, or the note left empty, is 0), or nil when its kcal
// are not a kcal more, as grams without their kcal would be no food; and
// whether the meals have a kcal more than total.
func restOf(total, sum food.Nutrients) (*food.Nutrients, bool) {
	diff := food.Nutrients{Kcal: total.Kcal - sum.Kcal, Protein: total.Protein - sum.Protein, Carbs: total.Carbs - sum.Carbs, Fat: total.Fat - sum.Fat}
	if diff.Kcal < 1 {
		return nil, diff.Kcal <= -1
	}
	rest := food.Nutrients{Kcal: diff.Kcal, Protein: math.Max(diff.Protein, 0), Carbs: math.Max(diff.Carbs, 0), Fat: math.Max(diff.Fat, 0)}
	return &rest, false
}

func add(a, b food.Nutrients) food.Nutrients {
	return food.Nutrients{Kcal: a.Kcal + b.Kcal, Protein: a.Protein + b.Protein, Carbs: a.Carbs + b.Carbs, Fat: a.Fat + b.Fat}
}

// eatenAt returns when a meal of date's note was eaten: at clock on the
// user's wall clock, the next calendar day when clock comes before the
// predecessor's rollover (a meal after midnight); at noon when the note
// gave no time, or one that does not read. It stays within date's day
// (cal), which the meal belongs to as its note did: a meal past the
// day's end, as one between the user's day start and the rollover is, or
// in an hour summer time skips, takes the day's first or last minute.
func eatenAt(cal day.Calendar, date day.Date, clock string) time.Time {
	at := time.Date(date.Year, date.Month, date.Day, 12, 0, 0, 0, cal.Location)
	if t, err := time.Parse("15:04", clock); err == nil {
		d := date
		if t.Hour() < rolloverHour {
			d = date.AddDays(1)
		}
		at = time.Date(d.Year, d.Month, d.Day, t.Hour(), t.Minute(), 0, 0, cal.Location)
	}
	switch start, end := cal.Start(date), cal.End(date); {
	case at.Before(start):
		return start
	case !at.Before(end):
		return end.Add(-time.Minute)
	}
	return at
}

// fitName returns s as a name of at most maxLen characters: trimmed, cut
// to maxLen, or fallback when it is no name; and whether it changed beyond
// the trim.
func fitName(s string, maxLen int, fallback string) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxLen {
		s = strings.TrimSpace(string([]rune(s)[:maxLen-1])) + "…"
		if text.Check("name", s, maxLen, true) == nil {
			return s, true
		}
	}
	if text.Check("name", s, maxLen, true) != nil {
		return fallback, true
	}
	return s, false
}

// mealID returns the id of the meal named key, its place or "rest", on
// date of userID's history: the same on every run, so a run again
// changes nothing. It is a UUIDv8 of a SHA-256.
func mealID(userID uuid.UUID, date day.Date, key string) uuid.UUID {
	h := sha256.Sum256([]byte("tdeemon history meal " + userID.String() + " " + date.String() + " " + key))
	var id uuid.UUID
	copy(id[:], h[:16])
	id[6] = id[6]&0x0f | 0x80
	id[8] = id[8]&0x3f | 0x80
	return id
}
