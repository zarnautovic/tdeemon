package history_test

import (
	"database/sql"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/history"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// The values below are made up.

const oldRows = `
INSERT INTO days (date, kalorije, proteini, ugljikohidrati, masti, kilaza, koraci, san, trening_min, osjecaj, status, napomena)
	VALUES ('2026-09-01', 2000, 150, 200, 60, 81.2, 9000, 7.5, 35, 4, 'zatvoren', 'a long day');
INSERT INTO days (date, kalorije, proteini, ugljikohidrati, masti, kilaza, san)
	VALUES ('2026-09-02', 1500, 100, 150, 50, 80.9, 30);
INSERT INTO days (date, kilaza) VALUES ('2026-09-03', 80.7);
INSERT INTO days (date, kalorije) VALUES ('1. 9. 2026.', 1800);
INSERT INTO meals VALUES ('2026-09-01', '08:10', 'Oats with milk', 500, 25, 70, 12);
INSERT INTO meals VALUES ('2026-09-01', '', 'Chicken and rice', 800, 60, 90, 18);
INSERT INTO meals VALUES ('2026-09-01', '01:30', 'A late sandwich', 400, 20, 40, 15);
INSERT INTO meals VALUES ('2026-09-02', '13:00', 'Pasta', 1200, 40, 160, 45);
INSERT INTO meals VALUES ('2026-09-02', '19:00', 'Yoghurt', 400, 30, 20, 10);
INSERT INTO meals VALUES ('2026-09-04', '12:00', '` + "  A meal whose note had no front matter  " + `', 600, 30, 60, 20);
`

// oldDB writes the predecessor's database with rows to a file and returns
// its path.
func oldDB(t *testing.T, rows string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "health.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	schema, err := os.ReadFile(filepath.Join("testdata", "predecessor.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), string(schema)+rows); err != nil {
		t.Fatal(err)
	}
	return path
}

// newUser returns a user of a fresh database, in Zagreb, whose day starts
// at 04:00.
func newUser(t *testing.T) (*store.DB, user.User) {
	t.Helper()
	db := storetest.Open(t)
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	u := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser,
		Locale: "hr", TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatal(err)
	}
	return db, u
}

var now = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

// kcal returns an estimate's values of n kcal, a tenth of them protein.
func kcal(n float64) food.Nutrients { return food.Nutrients{Kcal: n, Protein: n / 40} }

func TestRead(t *testing.T) {
	t.Parallel()

	days, err := history.Read(t.Context(), oldDB(t, oldRows))
	if err != nil {
		t.Fatal(err)
	}
	var dates []string
	for _, d := range days {
		dates = append(dates, d.Date)
	}
	if !slices.Equal(dates, []string{"1. 9. 2026.", "2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04"}) {
		t.Fatalf("dates = %v", dates)
	}
	first := days[1]
	if first.Total == nil || first.Total.Kcal != 2000 || first.Total.Fat != 60 || len(first.Meals) != 3 || first.Meals[2].Time != "01:30" {
		t.Errorf("the first day = %+v", first)
	}
	if want := map[string]float64{"weight": 81.2, "steps": 9000, "sleep_h": 7.5, "exercise_min": 35}; !maps.Equal(first.Metrics, want) {
		t.Errorf("metrics = %v, want %v", first.Metrics, want)
	}
	if days[3].Total != nil || len(days[3].Meals) != 0 || days[4].Total != nil || len(days[4].Meals) != 1 {
		t.Errorf("a day without a total %+v, a day of meals alone %+v", days[3], days[4])
	}
}

// A relative path reads as the path, not as a URI's authority.
func TestReadRelative(t *testing.T) {
	path := oldDB(t, oldRows)
	t.Chdir(filepath.Dir(path))
	for _, rel := range []string{"health.db", "./health.db"} {
		if days, err := history.Read(t.Context(), rel); err != nil || len(days) != 5 {
			t.Errorf("Read(%q) = %d days, %v", rel, len(days), err)
		}
	}
}

func TestReadRefuses(t *testing.T) {
	t.Parallel()

	if _, err := history.Read(t.Context(), filepath.Join(t.TempDir(), "none.db")); err == nil || !strings.Contains(err.Error(), "no database") {
		t.Errorf("Read of no file: %v", err)
	}
	// A database that is not the predecessor's, or lacks its meals.
	for name, c := range map[string]struct{ sql, want string }{
		"no days":  {"CREATE TABLE notes (x)", "the predecessor's days"},
		"no meals": {"CREATE TABLE days (date TEXT, kalorije, proteini, ugljikohidrati, masti, kilaza, masnoca, misicna_masa, koraci, aktivne_kalorije, bazalne_kalorije, san, rhr, hrv, vo2max, trening_min, napomena)", "the predecessor's meals"},
	} {
		path := filepath.Join(t.TempDir(), "other.db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), c.sql); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		if _, err := history.Read(t.Context(), path); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

// imported reads oldRows into a fresh user's history and returns the
// database, the user and the report.
func imported(t *testing.T) (*store.DB, user.User, history.Report) {
	t.Helper()
	db, u := newUser(t)
	days, err := history.Read(t.Context(), oldDB(t, oldRows))
	if err != nil {
		t.Fatal(err)
	}
	r, err := history.Import(t.Context(), db, u, days, now, false)
	if err != nil {
		t.Fatal(err)
	}
	return db, u, r
}

// The report counts what was written and names what was left out: a
// value out of its bounds, a day whose meals pass its total, a day that
// does not read.
func TestImportReport(t *testing.T) {
	t.Parallel()

	_, _, r := imported(t)
	if r.Days != 4 || r.Metrics != 6 || r.Meals != 7 || r.Rests != 1 || r.Excluded != 2 || len(r.Rejected) != 1 || r.Rejected[0].Kind != metric.Sleep ||
		!slices.Equal(r.Over, []string{"2026-09-02"}) || len(r.Skipped) != 1 || !strings.Contains(r.Skipped[0], "1. 9. 2026.") {
		t.Errorf("report = %+v", r)
	}
}

// The metrics merge as an import, by the names of the kinds.
func TestImportMetrics(t *testing.T) {
	t.Parallel()

	db, u, _ := imported(t)
	d1 := day.Date{Year: 2026, Month: 9, Day: 1}
	ms, err := db.Metrics(t.Context(), u.ID, d1, d1.AddDays(2))
	if err != nil || len(ms[d1]) != 4 || !slices.Contains(ms[d1], metric.Value{Kind: metric.ExerciseMin, Value: 35, Source: metric.Import}) ||
		len(ms[d1.AddDays(1)]) != 1 || len(ms[d1.AddDays(2)]) != 1 {
		t.Errorf("metrics = %+v, %v", ms, err)
	}
}

// Each meal is an estimate on the note's day, at its time on the user's
// clock, the next calendar day past midnight, at noon without a time; the
// rest comes last, and the meals then add up to the note's total.
func TestImportMeals(t *testing.T) {
	t.Parallel()

	db, u, _ := imported(t)
	d1 := day.Date{Year: 2026, Month: 9, Day: 1}
	meals, err := db.MealsOfDay(t.Context(), u.ID, d1)
	if err != nil || len(meals) != 4 {
		t.Fatalf("meals of the first day = %+v, %v", meals, err)
	}
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	at := map[string]time.Time{}
	var kcal float64
	for _, m := range meals {
		at[m.Description] = m.EatenAt.In(zagreb)
		kcal += m.Items[0].Nutrients.Kcal
		if m.Client != meal.ClientImport || m.Day != d1 || len(m.Items) != 1 || m.Items[0].FoodID != uuid.Nil() {
			t.Errorf("meal %+v", m)
		}
	}
	for desc, want := range map[string]string{
		"A late sandwich":  "09-02 01:30",
		"Chicken and rice": "09-01 12:00",
		history.RestName:   "09-02 03:59",
	} {
		if got := at[desc].Format("01-02 15:04"); got != want {
			t.Errorf("%s at %s, want %s", desc, got, want)
		}
	}
	if kcal != 2000 {
		t.Errorf("the first day's meals = %v kcal, want the note's total, 2000", kcal)
	}
	if v := meals[0].ID[6] >> 4; v != 8 {
		t.Errorf("a meal's id is a version %d UUID, want 8", v)
	}
}

// A meal before the predecessor's rollover, 05:00, is past midnight; one
// between the user's day start, 04:00, and the rollover is the end of the
// note's day; a time that does not read is noon. For a user whose day
// starts at 06:00, a meal at 05:30 is the start of the note's day.
func TestImportTimes(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	d := day.Date{Year: 2026, Month: 9, Day: 10}
	days := []history.Day{{Date: d.String(), Metrics: map[string]float64{}, Meals: []history.Meal{
		{Time: "05:00", Description: "At five", Nutrients: kcal(100)},
		{Time: "04:30", Description: "Before the rollover", Nutrients: kcal(100)},
		{Time: "03:59", Description: "Before the day's start", Nutrients: kcal(100)},
		{Time: "half past", Description: "Some time", Nutrients: kcal(100)},
	}}}
	if _, err := history.Import(t.Context(), db, u, days, now, false); err != nil {
		t.Fatal(err)
	}
	ms, err := db.MealsOfDay(t.Context(), u.ID, d)
	if err != nil || len(ms) != 4 {
		t.Fatalf("meals = %+v, %v", ms, err)
	}
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	for _, m := range ms {
		got := m.EatenAt.In(zagreb).Format("01-02 15:04")
		if want := map[string]string{
			"At five": "09-10 05:00", "Before the rollover": "09-11 03:59", "Before the day's start": "09-11 03:59", "Some time": "09-10 12:00",
		}[m.Description]; got != want {
			t.Errorf("%s at %s, want %s", m.Description, got, want)
		}
	}

	late := u
	late.DayStartHour = 6
	early := []history.Day{{Date: "2026-09-15", Metrics: map[string]float64{}, Meals: []history.Meal{{Time: "05:30", Description: "Early", Nutrients: kcal(100)}}}}
	if _, err := history.Import(t.Context(), db, late, early, now, false); err != nil {
		t.Fatal(err)
	}
	ms, _ = db.MealsOfDay(t.Context(), u.ID, day.Date{Year: 2026, Month: 9, Day: 15})
	if len(ms) != 1 || ms[0].EatenAt.In(zagreb).Format("01-02 15:04") != "09-15 06:00" {
		t.Errorf("a meal before a 06:00 start = %+v, want the day's first minute", ms)
	}

	// Every meal stays in its note's day: one without a time for a user
	// whose day starts at 14:00, and one in the hour summer time skips for
	// a day that starts at 02:00 (2026-03-29 jumps from 02:00 to 03:00).
	for _, c := range []struct {
		start      int
		date, time string
		want       string
	}{
		{14, "2026-09-20", "", "09-20 14:00"},
		{2, "2026-03-28", "02:30", "03-29 01:59"},
	} {
		v := u
		v.DayStartHour = c.start
		one := []history.Day{{Date: c.date, Metrics: map[string]float64{}, Meals: []history.Meal{{Time: c.time, Description: "Edge", Nutrients: kcal(100)}}}}
		if _, err := history.Import(t.Context(), db, v, one, now, false); err != nil {
			t.Fatal(err)
		}
		d, _ := day.ParseDate(c.date)
		ms, _ := db.MealsOfDay(t.Context(), u.ID, d)
		if len(ms) != 1 || ms[0].EatenAt.In(zagreb).Format("01-02 15:04") != c.want {
			t.Errorf("start %d, %s %q = %+v, want %s", c.start, c.date, c.time, ms, c.want)
		}
	}
}

// The rest takes each part of the total the meals do not reach, and none
// they pass.
func TestImportRest(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	days, err := history.Read(t.Context(), oldDB(t, `
INSERT INTO days (date, kalorije, proteini, ugljikohidrati, masti) VALUES ('2026-09-05', 1000, 80, 50, 40);
INSERT INTO meals VALUES ('2026-09-05', '09:00', 'Eggs', 600, 90, 10, 30);
INSERT INTO days (date, kalorije, proteini, ugljikohidrati, masti) VALUES ('2026-09-06', 1000.4, 79.6, 50, 40);
INSERT INTO meals VALUES ('2026-09-06', '09:00', 'All of it', 1000, 80, 50, 40);
INSERT INTO days (date, kalorije, proteini, ugljikohidrati, masti) VALUES ('2026-09-07', 900, 70, 60, 30);
INSERT INTO days (date, kalorije, proteini, ugljikohidrati, masti) VALUES ('2026-09-14', 999.6, 80, 50, 40);
INSERT INTO meals VALUES ('2026-09-14', '09:00', 'Under a kcal over', 1000, 80, 50, 40);
`))
	if err != nil {
		t.Fatal(err)
	}
	r, err := history.Import(t.Context(), db, u, days, now, false)
	if err != nil || r.Rests != 2 || len(r.Over) != 0 {
		t.Fatalf("report = %+v, %v", r, err)
	}
	rest := func(d day.Date) *meal.Meal {
		t.Helper()
		ms, err := db.MealsOfDay(t.Context(), u.ID, d)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			if m.Description == history.RestName {
				return &m
			}
		}
		return nil
	}
	for d, want := range map[int]*food.Nutrients{
		5: {Kcal: 400, Protein: 0, Carbs: 40, Fat: 10},  // no protein: the meals pass it
		6: nil,                                          // under a kcal
		7: {Kcal: 900, Protein: 70, Carbs: 60, Fat: 30}, // no meals: the whole total
	} {
		got := rest(day.Date{Year: 2026, Month: 9, Day: d})
		if (got == nil) != (want == nil) || got != nil && got.Items[0].Nutrients != *want {
			t.Errorf("the rest on %d September = %+v, want %+v", d, got, want)
		}
	}
}

// A dry run writes nothing; a run again adds nothing; a meal deleted
// since stays deleted.
func TestImportAgain(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	days, err := history.Read(t.Context(), oldDB(t, oldRows))
	if err != nil {
		t.Fatal(err)
	}
	dry, err := history.Import(t.Context(), db, u, days, now, true)
	if err != nil || dry.Meals != 7 || dry.Metrics != 6 {
		t.Fatalf("the dry run = %+v, %v", dry, err)
	}
	if es, _ := db.Events(t.Context(), u.ID, 10); len(es) != 0 {
		t.Fatalf("the dry run wrote %d events", len(es))
	}
	if _, err := history.Import(t.Context(), db, u, days, now, false); err != nil {
		t.Fatal(err)
	}
	d1 := day.Date{Year: 2026, Month: 9, Day: 1}
	meals, _ := db.MealsOfDay(t.Context(), u.ID, d1)
	if err := db.DeleteMeal(t.Context(), u.ID, history.Actor, meals[0].ID, now); err != nil {
		t.Fatal(err)
	}
	before, _ := db.Events(t.Context(), u.ID, store.MaxEvents)
	r, err := history.Import(t.Context(), db, u, days, now.Add(time.Hour), false)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := db.Events(t.Context(), u.ID, store.MaxEvents)
	if len(after) != len(before) || r.Meals != 6 || !slices.ContainsFunc(r.Skipped, func(s string) bool { return strings.Contains(s, "deleted") }) {
		t.Errorf("the run again: %d events, then %d; report %+v", len(before), len(after), r)
	}
	if meals, _ := db.MealsOfDay(t.Context(), u.ID, d1); len(meals) != 3 {
		t.Errorf("meals after the run again = %d, want the deleted one gone", len(meals))
	}
}

// A description is trimmed, cut to the longest name, or replaced when it
// is no name; a meal that breaks the rules of an estimate is skipped.
func TestImportDescriptions(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	long := strings.Repeat("ž", 250)
	days := []history.Day{{Date: "2026-09-08", Metrics: map[string]float64{}, Meals: []history.Meal{
		{Time: "09:00", Description: long, Nutrients: kcal(300)},
		{Time: "10:00", Description: "tab\there", Nutrients: kcal(200)},
		{Time: "11:00", Description: "A feast", Nutrients: kcal(20000)},
	}}}
	r, err := history.Import(t.Context(), db, u, days, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Renamed) != 2 || len(r.Skipped) != 1 || r.Meals != 2 {
		t.Fatalf("report = %+v", r)
	}
	ms, _ := db.MealsOfDay(t.Context(), u.ID, day.Date{Year: 2026, Month: 9, Day: 8})
	var descs []string
	for _, m := range ms {
		descs = append(descs, m.Description)
	}
	if len(ms) != 2 || !strings.HasSuffix(descs[0], "ž…") || len([]rune(descs[0])) != meal.MaxNameLen || descs[1] != "Meal" {
		t.Errorf("descriptions = %q", descs)
	}
}

func TestImportRefuses(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	bad := u
	bad.TimeZone = "Nowhere/Bogus"
	if _, err := history.Import(t.Context(), db, bad, nil, now, false); err == nil {
		t.Error("Import with a calendar that does not read succeeded")
	}
	// A user the database does not have: the first write fails.
	stranger := u
	stranger.ID = uuid.NewV7()
	days := []history.Day{{Date: "2026-09-09", Metrics: map[string]float64{"weight": 80}}}
	if _, err := history.Import(t.Context(), db, stranger, days, now, false); err == nil {
		t.Error("Import for no user succeeded")
	}
	days = []history.Day{{Date: "2026-09-09", Metrics: map[string]float64{}, Meals: []history.Meal{{Description: "Soup", Nutrients: kcal(200)}}}}
	if _, err := history.Import(t.Context(), db, stranger, days, now, false); err == nil || errors.Is(err, store.ErrMealDeleted) {
		t.Errorf("a meal for no user: %v", err)
	}
}

// A day with a remark is excluded for it, its spaces joined, a long one
// cut, one that is no reason replaced; a day without intake is excluded
// as one; a day with intake and no remark is not.
func TestImportExclusions(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	days := []history.Day{
		{Date: "2026-09-11", Metrics: map[string]float64{}, Remark: " Birthday\tdinner\n with  friends "},
		{Date: "2026-09-12", Metrics: map[string]float64{}, Remark: strings.Repeat("a", 250)},
		{Date: "2026-09-13", Metrics: map[string]float64{}},
		{Date: "2026-09-14", Metrics: map[string]float64{}, Remark: "\u200b"},
		{Date: "2026-09-15", Metrics: map[string]float64{}, Total: &food.Nutrients{Kcal: 0}},
		{Date: "2026-09-16", Metrics: map[string]float64{}, Meals: []history.Meal{{Time: "12:00", Description: "Soup", Nutrients: kcal(300)}}},
		{Date: "2026-09-17", Metrics: map[string]float64{}, Total: &food.Nutrients{Kcal: 1800}},
	}
	dry, err := history.Import(t.Context(), db, u, days, now, true)
	if err != nil || dry.Excluded != 5 {
		t.Fatalf("the dry run = %+v, %v", dry, err)
	}
	r, err := history.Import(t.Context(), db, u, days, now, false)
	if err != nil || r.Excluded != 5 || len(r.Renamed) != 1 || !strings.Contains(r.Renamed[0], "2026-09-14") {
		t.Fatalf("report = %+v, %v", r, err)
	}
	d := day.Date{Year: 2026, Month: 9, Day: 11}
	ex, err := db.Exclusions(t.Context(), u.ID, d, d.AddDays(6))
	want := map[day.Date]string{
		d:            "Birthday dinner with friends",
		d.AddDays(1): strings.Repeat("a", day.MaxReasonLen-1) + "…",
		d.AddDays(2): history.NoIntakeReason,
		d.AddDays(3): "Remark of the predecessor's note",
		d.AddDays(4): history.NoIntakeReason,
	}
	if err != nil || len(ex) != len(want) {
		t.Fatalf("exclusions = %+v, %v", ex, err)
	}
	for dd, reason := range want {
		if ex[dd].Reason != reason {
			t.Errorf("%s excluded for %q, want %q", dd, ex[dd].Reason, reason)
		}
	}
}

// A run again leaves a day as decided since: included by the user, or
// excluded for a reason of theirs, and says so; a day as the import left
// it adds nothing.
func TestImportExclusionsAgain(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	days := []history.Day{
		{Date: "2026-09-11", Metrics: map[string]float64{}, Remark: "Birthday dinner"},
		{Date: "2026-09-12", Metrics: map[string]float64{}, Remark: "Travel"},
		{Date: "2026-09-13", Metrics: map[string]float64{}, Remark: "Sick"},
	}
	if _, err := history.Import(t.Context(), db, u, days, now, false); err != nil {
		t.Fatal(err)
	}
	d := day.Date{Year: 2026, Month: 9, Day: 11}
	mine := event.Actor{Kind: event.User, Client: "web"}
	if err := db.IncludeDay(t.Context(), u.ID, mine, d, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ExcludeDay(t.Context(), u.ID, mine, d.AddDays(1), day.Exclusion{Reason: "A trip, but I logged it all"}, now); err != nil {
		t.Fatal(err)
	}
	before, _ := db.Events(t.Context(), u.ID, store.MaxEvents)
	r, err := history.Import(t.Context(), db, u, days, now, false)
	if err != nil || r.Excluded != 0 || len(r.Skipped) != 2 {
		t.Fatalf("the run again = %+v, %v", r, err)
	}
	after, _ := db.Events(t.Context(), u.ID, store.MaxEvents)
	ex, _ := db.Exclusions(t.Context(), u.ID, d, d.AddDays(2))
	if len(after) != len(before) || len(ex) != 2 || ex[d.AddDays(1)].Reason != "A trip, but I logged it all" || ex[d.AddDays(2)].Reason != "Sick" {
		t.Errorf("after the run again: %d events (before %d), exclusions %+v", len(after), len(before), ex)
	}
}

// A meal an import before wrote differently, as a note fixed since or
// another time zone makes it, stays as it is, and the import goes on.
func TestImportChangedSince(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	days, err := history.Read(t.Context(), oldDB(t, oldRows))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := history.Import(t.Context(), db, u, days, now, false); err != nil {
		t.Fatal(err)
	}
	u.TimeZone = "UTC"
	r, err := history.Import(t.Context(), db, u, days, now, false)
	if err != nil || r.Meals != 7 || !slices.ContainsFunc(r.Skipped, func(s string) bool { return strings.Contains(s, "differs from an import before") }) {
		t.Errorf("the run in another zone = %+v, %v", r, err)
	}
}
