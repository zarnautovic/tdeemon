package history_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/history"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// The plan in testdata/plan.json and the weigh-ins below are made up.

func readPlan(t *testing.T, edit func(string) string) history.PlanFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if edit != nil {
		s = edit(s)
	}
	f, err := history.ReadPlan(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// weighed gives u a week of weigh-ins before today, about kg.
func weighed(t *testing.T, db *store.DB, u user.User, kg float64) {
	t.Helper()
	d := day.Date{Year: 2026, Month: 10, Day: 21}
	for i := range 7 {
		v := []metric.Value{{Kind: metric.Weight, Value: kg + float64(i%2)/10, Source: metric.Import}}
		if _, err := db.MergeMetrics(t.Context(), u.ID, history.Actor, d.AddDays(-i), v, now); err != nil {
			t.Fatal(err)
		}
	}
}

// A plan the limits allow is written whole, as the import's; a dry run
// writes nothing; a run again keeps what the user has.
func TestImportPlan(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 72)
	f := readPlan(t, nil)
	dry, err := history.ImportPlan(t.Context(), db, u, f, history.PlanLimits{Instance: limits.Instance()}, now, true)
	want := []string{"profile", "training weekdays", "phase from 2026-07-01", "target from 2026-07-01", "target from 2026-10-01", "notes"}
	if err != nil || !slices.Equal(dry.Written, want) {
		t.Fatalf("the dry run = %+v, %v", dry, err)
	}
	if ts, _ := db.Targets(t.Context(), u.ID); len(ts) != 0 {
		t.Fatal("the dry run wrote")
	}
	r, err := history.ImportPlan(t.Context(), db, u, f, history.PlanLimits{Instance: limits.Instance()}, now, false)
	if err != nil || !slices.Equal(r.Written, want) {
		t.Fatalf("report = %+v, %v", r, err)
	}
	checkPlan(t, db, u)

	// A run again writes nothing, keeps nothing of the user's (what it
	// wrote is the file's), and holds nothing: weeks later, without a
	// weigh-in, the limits would refuse the plan.
	later := now.AddDate(0, 0, 60)
	again, err := history.ImportPlan(t.Context(), db, u, f, history.PlanLimits{Instance: limits.Instance()}, later, false)
	if err != nil || len(again.Written) != 0 || len(again.Kept) != 0 {
		t.Errorf("the run again = %+v, %v", again, err)
	}
	// Training weekdays the user cleared since are written again, and move
	// the plan's average: the plan is held again.
	if err := db.SetTrainingWeekdays(t.Context(), u.ID, event.Actor{Kind: event.User, Client: "web"}, 0, later); err != nil {
		t.Fatal(err)
	}
	if _, err := history.ImportPlan(t.Context(), db, u, f, history.PlanLimits{Instance: limits.Instance()}, later, true); !refusedFor(err, limits.RuleWeightMissing, "") {
		t.Errorf("the weekdays again: %v", err)
	}
}

// checkPlan checks the plan of testdata/plan.json, as the import's.
func checkPlan(t *testing.T, db *store.DB, u user.User) {
	t.Helper()
	ts, _ := db.Targets(t.Context(), u.ID)
	phases, _ := db.Phases(t.Context(), u.ID)
	w, _ := db.TrainingWeekdays(t.Context(), u.ID)
	notes, _ := db.ProfileNotes(t.Context(), u.ID)
	pr, _ := db.Profile(t.Context(), u.ID)
	if len(ts) != 2 || ts[1].Rest.Kcal != 1750 || len(phases) != 1 || phases[0].RatePctWeek != -0.5 || *phases[0].GoalWeight != 64 ||
		w != plan.Weekdays(0b0101010) || notes != "No fish. Trains at dawn." || pr.HeightCm != 170 {
		t.Errorf("targets %+v, phases %+v, weekdays %b, notes %q, profile %+v", ts, phases, w, notes, pr)
	}
	if es, _ := db.Events(t.Context(), u.ID, 1); len(es) != 1 || es[0].Actor != history.Actor {
		t.Errorf("events = %+v", es)
	}
}

// The limits refuse a plan whose current phase loses too fast, or with no
// weigh-in to hold it to, and nothing is written.
func TestImportPlanRefused(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		kg   float64
		edit func(string) string
		rule string
	}{
		"too fast":     {72, func(s string) string { return strings.Replace(s, `"rate_pct_week": -0.5`, `"rate_pct_week": -3`, 1) }, limits.RuleCutRate},
		"no weigh-ins": {0, nil, limits.RuleWeightMissing},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, u := newUser(t)
			if c.kg > 0 {
				weighed(t, db, u, c.kg)
			}
			_, err := history.ImportPlan(t.Context(), db, u, readPlan(t, c.edit), history.PlanLimits{Instance: limits.Instance()}, now, false)
			var refused *history.PlanRefusedError
			if !errors.As(err, &refused) || !slices.ContainsFunc(refused.Violations, func(v limits.Violation) bool { return v.Rule == c.rule }) ||
				!strings.Contains(err.Error(), c.rule) {
				t.Fatalf("err = %v, want %s", err, c.rule)
			}
			if ts, _ := db.Targets(t.Context(), u.ID); len(ts) != 0 {
				t.Error("a refused plan wrote")
			}
		})
	}
}

// A user with a plan of their own keeps it, and their profile; what they
// have not got yet, notes here, is written.
func TestImportPlanKeepsTheUsers(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 72)
	mine := event.Actor{Kind: event.User, Client: "web"}
	own := profile.Profile{Sex: profile.Female, BirthDate: day.Date{Year: 1991, Month: 2, Day: 3}, HeightCm: 168}
	if err := db.SetProfile(t.Context(), u.ID, mine, own, day.DateOf(now), now); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTarget(t.Context(), u.ID, mine, plan.Target{
		EffectiveFrom: day.Date{Year: 2026, Month: 10, Day: 1}, Training: plan.DayTarget{Kcal: 2100, Protein: 120, Fat: 70}, Rest: plan.DayTarget{Kcal: 2100, Protein: 120, Fat: 70},
	}, now); err != nil {
		t.Fatal(err)
	}
	r, err := history.ImportPlan(t.Context(), db, u, readPlan(t, nil), history.PlanLimits{Instance: limits.Instance()}, now, false)
	if err != nil || !slices.Equal(r.Written, []string{"notes"}) || !slices.Equal(r.Kept, []string{"plan", "profile"}) {
		t.Errorf("report = %+v, %v", r, err)
	}
	if ts, _ := db.Targets(t.Context(), u.ID); len(ts) != 1 || ts[0].Training.Kcal != 2100 {
		t.Errorf("targets = %+v", ts)
	}
	if pr, _ := db.Profile(t.Context(), u.ID); pr != own {
		t.Errorf("profile = %+v, want the user's own", pr)
	}
}

// The user's notes stay theirs; and the limits hold the plan to the
// user's own profile, here too young for a plan, rather than the file's.
func TestImportPlanByTheUsersOwn(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 72)
	mine := event.Actor{Kind: event.User, Client: "web"}
	if err := db.SetProfileNotes(t.Context(), u.ID, mine, "My own notes.", now); err != nil {
		t.Fatal(err)
	}
	young := profile.Profile{Sex: profile.Female, BirthDate: day.Date{Year: 2012, Month: 2, Day: 3}, HeightCm: 160}
	if err := db.SetProfile(t.Context(), u.ID, mine, young, day.DateOf(now), now); err != nil {
		t.Fatal(err)
	}
	r, err := history.ImportPlan(t.Context(), db, u, readPlan(t, nil), history.PlanLimits{Instance: limits.Instance()}, now, false)
	if err == nil || !strings.Contains(err.Error(), limits.RuleMinAge) || !slices.Contains(r.Kept, "notes") {
		t.Errorf("by a profile too young: %+v, %v", r, err)
	}
	if notes, _ := db.ProfileNotes(t.Context(), u.ID); notes != "My own notes." {
		t.Errorf("notes = %q", notes)
	}
}

// What is no plan file, or a plan of a shape no plan has, writes nothing.
func TestImportPlanRefusesItsShape(t *testing.T) {
	t.Parallel()

	if _, err := history.ReadPlan(strings.NewReader(`{"phase": []}`)); err == nil {
		t.Error("ReadPlan took an unknown field")
	}
	db, u := newUser(t)
	weighed(t, db, u, 72)
	for name, edit := range map[string]func(string) string{
		"a birth date": func(s string) string { return strings.Replace(s, `"1990-05-01"`, `"1. 5. 1990."`, 1) },
		"a height":     func(s string) string { return strings.Replace(s, `"height_cm": 170`, `"height_cm": 17`, 1) },
		"a phase type": func(s string) string { return strings.Replace(s, `"cut"`, `"diet"`, 1) },
		"a phase's end": func(s string) string {
			return strings.Replace(s, `"type": "cut"`, `"end": "someday", "type": "cut"`, 1)
		},
		"a weekday": func(s string) string { return strings.Replace(s, `[1, 3, 5]`, `[1, 3, 9]`, 1) },
		"a target": func(s string) string {
			return strings.Replace(s, `"effective_from": "2026-10-01"`, `"effective_from": "October"`, 1)
		},
		"the notes": func(s string) string { return strings.Replace(s, `No fish. Trains`, `No fish.\tTrains`, 1) },
	} {
		if _, err := history.ImportPlan(t.Context(), db, u, readPlan(t, edit), history.PlanLimits{Instance: limits.Instance()}, now, false); err == nil || !strings.Contains(err.Error(), "the plan file") {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := u
	bad.TimeZone = "Nowhere/Bogus"
	if _, err := history.ImportPlan(t.Context(), db, bad, readPlan(t, nil), history.PlanLimits{Instance: limits.Instance()}, now, false); err == nil {
		t.Error("ImportPlan with a calendar that does not read succeeded")
	}
	if ts, _ := db.Targets(t.Context(), u.ID); len(ts) != 0 {
		t.Error("a plan of a bad shape wrote")
	}
	if pr, _ := db.Profile(t.Context(), u.ID); pr != (profile.Profile{}) {
		t.Errorf("a plan of a bad shape wrote the profile %+v", pr)
	}
}

// The limits hold the plan to the training weekdays the user has, not
// the file's: below a BMI of 18.5, a target whose week is higher than the
// one before by the user's seven training days, though lower by the
// file's three, passes; and the user's weekdays are reported kept.
func TestImportPlanByTheUsersWeekdays(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 50)
	if err := db.SetTrainingWeekdays(t.Context(), u.ID, event.Actor{Kind: event.User, Client: "web"}, plan.Weekdays(0b1111111), now); err != nil {
		t.Fatal(err)
	}
	f := readPlan(t, func(s string) string {
		return strings.NewReplacer(
			`"type": "cut", "rate_pct_week": -0.5, "goal_weight": 64, "milestone_weight": 68`, `"type": "maintain", "rate_pct_week": 0`,
			`"protein": 130`, `"protein": 90`,
			`"training": {"kcal": 1950, "protein": 130, "fat": 60}, "rest": {"kcal": 1750, "protein": 130, "fat": 60}}`,
			`"training": {"kcal": 2000, "protein": 90, "fat": 60}, "rest": {"kcal": 1500, "protein": 90, "fat": 60}},
    {"effective_from": "2026-11-01", "training": {"kcal": 2100, "protein": 90, "fat": 60}, "rest": {"kcal": 1300, "protein": 90, "fat": 60}}`,
		).Replace(s)
	})
	r, err := importPlan(t, db, u, f, history.PlanLimits{}, true)
	if err != nil || slices.Contains(r.Written, "training weekdays") || !slices.Equal(r.Kept, []string{"training weekdays"}) {
		t.Errorf("report = %+v, %v", r, err)
	}
}

// No training weekdays in the file are none given: a run again writes
// nothing, and holds nothing.
func TestImportPlanNoWeekdays(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 72)
	f := readPlan(t, func(s string) string { return strings.Replace(s, `[1, 3, 5]`, `[]`, 1) })
	if _, err := importPlan(t, db, u, f, history.PlanLimits{}, false); err != nil {
		t.Fatal(err)
	}
	again, err := history.ImportPlan(t.Context(), db, u, f, history.PlanLimits{Instance: limits.Instance()}, now.AddDate(0, 0, 60), false)
	if err != nil || len(again.Written) != 0 {
		t.Errorf("the run again = %+v, %v", again, err)
	}
}

func importPlan(t *testing.T, db *store.DB, u user.User, f history.PlanFile, lim history.PlanLimits, dryRun bool) (history.PlanReport, error) {
	t.Helper()
	if lim.Instance == (limits.Limits{}) {
		lim.Instance = limits.Instance()
	}
	return history.ImportPlan(t.Context(), db, u, f, lim, now, dryRun)
}

func refusedFor(err error, rule string, level limits.Level) bool {
	var refused *history.PlanRefusedError
	return errors.As(err, &refused) && slices.ContainsFunc(refused.Violations, func(v limits.Violation) bool {
		return v.Rule == rule && (level == "" || v.Level == level)
	})
}

// The phases and targets after today are held to the limits too, and a
// plan all in the future needs weigh-ins as one of today does.
func TestImportPlanFuture(t *testing.T) {
	t.Parallel()

	ends := func(s string) string {
		return strings.Replace(s, `"type": "cut", "rate_pct_week": -0.5`, `"end": "2026-10-31", "type": "cut", "rate_pct_week": -0.5`, 1)
	}
	for name, c := range map[string]struct {
		kg   float64
		edit func(string) string
		rule string
	}{
		"a phase after today too fast": {72, func(s string) string {
			return strings.Replace(ends(s), `"phases": [`, `"phases": [{"start": "2026-11-01", "type": "cut", "rate_pct_week": -3},`, 1)
		}, limits.RuleCutRate},
		"a target after today too low": {72, func(s string) string {
			return strings.Replace(s, `"targets": [`, `"targets": [{"effective_from": "2026-11-01", "training": {"kcal": 600, "protein": 100, "fat": 20}, "rest": {"kcal": 600, "protein": 100, "fat": 20}},`, 1)
		}, limits.RuleKcalFloor},
		"all in the future, no weigh-ins": {0, func(s string) string {
			return strings.NewReplacer(`"2026-07-01"`, `"2026-11-01"`, `"2026-10-01"`, `"2026-12-01"`).Replace(s)
		}, limits.RuleWeightMissing},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, u := newUser(t)
			if c.kg > 0 {
				weighed(t, db, u, c.kg)
			}
			_, err := importPlan(t, db, u, readPlan(t, c.edit), history.PlanLimits{}, false)
			if !refusedFor(err, c.rule, "") {
				t.Errorf("err = %v, want %s", err, c.rule)
			}
			if ts, _ := db.Targets(t.Context(), u.ID); len(ts) != 0 {
				t.Error("a refused plan wrote")
			}
		})
	}
}

// Phases that overlap, or two targets of a day, are no plan, refused
// before anything is written.
func TestImportPlanTimeline(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 72)
	for name, edit := range map[string]func(string) string{
		"phases that overlap": func(s string) string {
			s = strings.Replace(s, `"type": "cut", "rate_pct_week": -0.5`, `"end": "2026-10-31", "type": "cut", "rate_pct_week": -0.5`, 1)
			return strings.Replace(s, `"phases": [`, `"phases": [{"start": "2026-10-15", "type": "maintain", "rate_pct_week": 0},`, 1)
		},
		"two targets of a day": func(s string) string {
			return strings.Replace(s, `"effective_from": "2026-10-01"`, `"effective_from": "2026-07-01"`, 1)
		},
	} {
		if _, err := importPlan(t, db, u, readPlan(t, edit), history.PlanLimits{}, false); err == nil || !strings.Contains(err.Error(), "the plan file") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := history.ReadPlan(strings.NewReader(`{"notes": "a"} {"notes": "b"}`)); err == nil {
		t.Error("ReadPlan took something after the plan")
	}
	if pr, _ := db.Profile(t.Context(), u.ID); pr.HeightCm != 0 {
		t.Error("a refused plan wrote its profile")
	}
}

// A plan an import before wrote in part is written whole on a run again.
func TestImportPlanResumes(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 72)
	first := readPlan(t, nil)
	first.Targets = first.Targets[1:]
	if _, err := importPlan(t, db, u, first, history.PlanLimits{}, false); err != nil {
		t.Fatal(err)
	}
	r, err := importPlan(t, db, u, readPlan(t, nil), history.PlanLimits{}, false)
	if err != nil || !slices.Equal(r.Written, []string{"target from 2026-07-01"}) || slices.Contains(r.Kept, "plan") {
		t.Fatalf("the run again = %+v, %v", r, err)
	}
	checkPlan(t, db, u)
}

// The limits are the strictest of the user's own and the instance's.
func TestImportPlanLevels(t *testing.T) {
	t.Parallel()

	fast := func(s string) string { return strings.Replace(s, `"rate_pct_week": -0.5`, `"rate_pct_week": -0.9`, 1) }
	db, u := newUser(t)
	weighed(t, db, u, 72)
	if _, err := importPlan(t, db, u, readPlan(t, fast), history.PlanLimits{}, true); !refusedFor(err, limits.RuleCutRate, limits.LevelUser) {
		t.Errorf("past the user's limit: %v", err)
	}
	mine := limits.UserDefaults()
	mine.CutRate = -1.0
	if err := db.SetUserLimits(t.Context(), u.ID, event.Actor{Kind: event.User, Client: "web"}, mine, limits.Instance(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := importPlan(t, db, u, readPlan(t, fast), history.PlanLimits{}, true); err != nil {
		t.Errorf("within the user's loosened limit: %v", err)
	}
	tight := limits.Instance()
	tight.CutRate = -0.4
	if _, err := importPlan(t, db, u, readPlan(t, nil), history.PlanLimits{Instance: tight}, true); !refusedFor(err, limits.RuleCutRate, limits.LevelInstance) {
		t.Errorf("past the instance's limit: %v", err)
	}
}

// A dry run holds the plan to the weigh-ins the days would bring.
func TestImportPlanWeighedOfADryRun(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighedIn := map[day.Date]float64{}
	for i := range 7 {
		weighedIn[day.Date{Year: 2026, Month: 10, Day: 21}.AddDays(-i)] = 72
	}
	if _, err := importPlan(t, db, u, readPlan(t, nil), history.PlanLimits{Weighed: weighedIn}, true); err != nil {
		t.Errorf("with the days' weigh-ins: %v", err)
	}
	if r, err := importPlan(t, db, u, readPlan(t, nil), history.PlanLimits{}, true); !refusedFor(err, limits.RuleWeightMissing, "") || len(r.Written) != 0 {
		t.Errorf("without: %v", err)
	}
}

// The file fills the fields of the profile the user has not set.
func TestImportPlanFillsTheProfile(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 72)
	if err := db.SetProfile(t.Context(), u.ID, event.Actor{Kind: event.User, Client: "web"}, profile.Profile{HeightCm: 172}, day.DateOf(now), now); err != nil {
		t.Fatal(err)
	}
	r, err := importPlan(t, db, u, readPlan(t, nil), history.PlanLimits{}, false)
	pr, _ := db.Profile(t.Context(), u.ID)
	if err != nil || !slices.Contains(r.Written, "profile") || pr.HeightCm != 172 || pr.Sex != profile.Female || pr.BirthDate.Year != 1990 {
		t.Errorf("profile = %+v, report %+v, %v", pr, r, err)
	}
}

// The history before the current target vouches for nothing: the
// current one is held as a new one, whether it moves toward the limit
// from the one before or stays as it was.
func TestImportPlanHistoryVouchesForNothing(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 72)
	for name, current := range map[string]string{
		"toward the floor": `{"kcal": 500, "protein": 50, "fat": 6}`,
		"as it was":        `{"kcal": 400, "protein": 40, "fat": 2}`,
	} {
		f := readPlan(t, func(s string) string {
			return strings.NewReplacer(
				`{"kcal": 2000, "protein": 130, "fat": 65}`, `{"kcal": 400, "protein": 40, "fat": 2}`,
				`{"kcal": 1950, "protein": 130, "fat": 60}`, current, `{"kcal": 1750, "protein": 130, "fat": 60}`, current,
			).Replace(s)
		})
		if _, err := importPlan(t, db, u, f, history.PlanLimits{}, true); !refusedFor(err, limits.RuleKcalFloor, "") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A target after the current one is held against the one before it: below
// a BMI of 18.5, one that lowers the kcal is refused, though alone it
// would pass.
func TestImportPlanAgainstTheOneBefore(t *testing.T) {
	t.Parallel()

	db, u := newUser(t)
	weighed(t, db, u, 50)
	maintain := func(s string) string {
		return strings.NewReplacer(
			`"type": "cut", "rate_pct_week": -0.5, "goal_weight": 64, "milestone_weight": 68`, `"type": "maintain", "rate_pct_week": 0`,
			`"protein": 130`, `"protein": 90`,
		).Replace(s)
	}
	if _, err := importPlan(t, db, u, readPlan(t, maintain), history.PlanLimits{}, true); err != nil {
		t.Fatalf("the plan alone: %v", err)
	}
	f := readPlan(t, func(s string) string {
		return strings.Replace(maintain(s), `
  ],
  "notes"`, `,
    {"effective_from": "2026-11-01", "training": {"kcal": 1700, "protein": 90, "fat": 60}, "rest": {"kcal": 1500, "protein": 90, "fat": 60}}
  ],
  "notes"`, 1)
	})
	if _, err := importPlan(t, db, u, f, history.PlanLimits{}, true); !refusedFor(err, limits.RuleUnderweightLoss, limits.LevelCode) {
		t.Errorf("a target lower than the one before: %v", err)
	}
}

// A refusal names each rule, its day type if any, and its values to two
// decimals at most, as a protein in g/kg needs them.
func TestPlanRefusedError(t *testing.T) {
	t.Parallel()

	err := &history.PlanRefusedError{Violations: []limits.Violation{
		{Rule: limits.RuleProteinCeiling, DayType: plan.Rest, Proposed: 2.4166666, Limit: 2.2, Level: limits.LevelInstance},
		{Rule: limits.RuleWeightMissing},
	}}
	want := "the limits refuse the plan: protein_ceiling (rest day): 2.42, the limit 2.2 (instance); weight_missing"
	if err.Error() != want {
		t.Errorf("%q, want %q", err.Error(), want)
	}
}
