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
	dry, err := history.ImportPlan(t.Context(), db, u, f, limits.Instance(), now, true)
	want := []string{"profile", "training weekdays", "phase from 2026-07-01", "target from 2026-07-01", "target from 2026-10-01", "notes"}
	if err != nil || !slices.Equal(dry.Written, want) {
		t.Fatalf("the dry run = %+v, %v", dry, err)
	}
	if ts, _ := db.Targets(t.Context(), u.ID); len(ts) != 0 {
		t.Fatal("the dry run wrote")
	}
	r, err := history.ImportPlan(t.Context(), db, u, f, limits.Instance(), now, false)
	if err != nil || !slices.Equal(r.Written, want) {
		t.Fatalf("report = %+v, %v", r, err)
	}
	checkPlan(t, db, u)

	again, err := history.ImportPlan(t.Context(), db, u, f, limits.Instance(), now, false)
	if err != nil || len(again.Written) != 0 || !slices.Equal(again.Kept, []string{"plan", "profile", "notes"}) {
		t.Errorf("the run again = %+v, %v", again, err)
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
			_, err := history.ImportPlan(t.Context(), db, u, readPlan(t, c.edit), limits.Instance(), now, false)
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
	r, err := history.ImportPlan(t.Context(), db, u, readPlan(t, nil), limits.Instance(), now, false)
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
	r, err := history.ImportPlan(t.Context(), db, u, readPlan(t, nil), limits.Instance(), now, false)
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
	} {
		if _, err := history.ImportPlan(t.Context(), db, u, readPlan(t, edit), limits.Instance(), now, false); err == nil || !strings.Contains(err.Error(), "the plan file") {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := u
	bad.TimeZone = "Nowhere/Bogus"
	if _, err := history.ImportPlan(t.Context(), db, bad, readPlan(t, nil), limits.Instance(), now, false); err == nil {
		t.Error("ImportPlan with a calendar that does not read succeeded")
	}
	if ts, _ := db.Targets(t.Context(), u.ID); len(ts) != 0 {
		t.Error("a plan of a bad shape wrote")
	}
}
