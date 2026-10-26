package proposal

import (
	"slices"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

// checkState is a man of 85 kg at 180 cm, in a cut since 2026-09-07,
// with a target of 2300 kcal from then and a trusted TDEE of 2600; and
// the timeline of his targets, with one more from 2026-11-02.
func checkState(t *testing.T) (limits.State, []plan.Target) {
	t.Helper()
	tg := *target(t, "2026-09-07", 2300)
	s := limits.State{
		Today:   date(t, "2026-10-26"),
		Profile: profile.Profile{Sex: profile.Male, BirthDate: date(t, "1990-01-01"), HeightCm: 180},
		TrendKg: 85, Phase: cut(t, "2026-09-07", "", -0.5), PhaseStartTrendKg: 88,
		Target: &tg, TrainingDays: 0b0101010, TDEE: 2600, TDEETrusted: true,
	}
	return s, []plan.Target{tg, *target(t, "2026-11-02", 2200)}
}

func levels() limits.Levels {
	return limits.Levels{Instance: limits.Instance(), User: limits.UserDefaults()}
}

// rules returns the rules of the violations of cs, by what each is of.
func rules(cs []Checked) map[string][]string {
	out := map[string][]string{}
	for _, c := range cs {
		key := string(c.Of) + " " + c.Day.String()
		out[key] = []string{}
		for _, v := range c.Violations {
			out[key] = append(out[key], v.Rule)
		}
	}
	return out
}

func TestChangesCheck(t *testing.T) {
	t.Parallel()

	s, targets := checkState(t)
	tomorrow := date(t, "2026-10-27")
	newDays := plan.Weekdays(0b1010101)
	for _, tc := range []struct {
		name string
		c    Changes
		want map[string][]string
	}{
		{
			"a target under the floor",
			Changes{Targets: []TargetChange{{From: tomorrow, After: target(t, "2026-10-27", 1400)}}},
			map[string][]string{"target 2026-10-27": {limits.RuleKcalFloor, limits.RuleImpliedLossRate}},
		},
		{
			"a target within the limits",
			Changes{Targets: []TargetChange{{From: tomorrow, After: target(t, "2026-10-27", 2200)}}},
			map[string][]string{"target 2026-10-27": {}},
		},
		{
			"a cut too fast",
			Changes{Phases: []PhaseChange{{Start: s.Phase.Start, Before: s.Phase, After: cut(t, "2026-09-07", "", -1.2)}}},
			map[string][]string{"phase 2026-09-07": {limits.RuleCutRate}},
		},
		{
			"a phase ahead",
			Changes{Phases: []PhaseChange{{Start: date(t, "2027-01-04"), After: &plan.Phase{Type: plan.Bulk, Start: date(t, "2027-01-04"), RatePctWeek: 0.25}}}},
			map[string][]string{"phase 2027-01-04": {}},
		},
		{
			"a phase deleted",
			Changes{Phases: []PhaseChange{{Start: date(t, "2027-01-04"), Before: &plan.Phase{Type: plan.Bulk, Start: date(t, "2027-01-04")}}}},
			map[string][]string{},
		},
		{
			"the training weekdays alone",
			Changes{TrainingWeekdays: &WeekdaysChange{Before: s.TrainingDays, After: newDays}},
			map[string][]string{"training_weekdays 2026-10-26": {}, "training_weekdays 2026-11-02": {}},
		},
		{"a target from today, with the training weekdays", Changes{
			Targets:          []TargetChange{{From: s.Today, After: target(t, "2026-10-26", 2200)}},
			TrainingWeekdays: &WeekdaysChange{Before: s.TrainingDays, After: newDays},
		}, map[string][]string{"target 2026-10-26": {}, "training_weekdays 2026-11-02": {}}},
		{
			"a target ahead deleted",
			Changes{Targets: []TargetChange{{From: date(t, "2026-11-02"), Before: &targets[1]}}},
			map[string][]string{"target 2026-11-02": {}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := rules(tc.c.Check(s, levels(), targets)); !mapsEqual(got, tc.want) {
				t.Errorf("Check = %v, want %v", got, tc.want)
			}
		})
	}
}

func mapsEqual(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		w, ok := b[k]
		if !ok || !slices.Equal(v, w) {
			return false
		}
	}
	return true
}

// A check of a target holds the target in force as the changes leave
// it, with the training weekdays then, against the one in force before.
func TestChangesCheckTheTargetInForce(t *testing.T) {
	t.Parallel()

	s, targets := checkState(t)
	newDays := plan.Weekdays(0b1010101)
	// The target from 2026-11-02 deleted: the one from 2026-09-07 holds
	// on, with the new training weekdays.
	cs := Changes{
		Targets:          []TargetChange{{From: date(t, "2026-11-02"), Before: &targets[1]}},
		TrainingWeekdays: &WeekdaysChange{Before: s.TrainingDays, After: newDays},
	}.Check(s, levels(), targets)
	if len(cs) != 2 {
		t.Fatalf("Check = %+v, want the target and the training weekdays today", cs)
	}
	for _, c := range cs {
		if c.Target == nil || *c.Target != targets[0] || c.TrainingDays != newDays {
			t.Errorf("the check of %s %s holds %+v with %#b, want the target from 2026-09-07 and the new weekdays", c.Of, c.Day, c.Target, c.TrainingDays)
		}
	}
	// A check of a phase holds no target, and the weekdays as they are.
	cs = Changes{Phases: []PhaseChange{{Start: s.Phase.Start, Before: s.Phase, After: cut(t, "2026-09-07", "", -0.6)}}}.Check(s, levels(), targets)
	if len(cs) != 1 || cs[0].Target != nil || cs[0].TrainingDays != s.TrainingDays {
		t.Errorf("the check of a phase = %+v", cs)
	}
}

// Below a BMI of 18.5 a lower target is refused, by the floor in the
// code, whatever the limits.
func TestChangesCheckUnderweight(t *testing.T) {
	t.Parallel()

	s, targets := checkState(t)
	s.TrendKg, s.PhaseStartTrendKg = 59, 60
	cs := Changes{Targets: []TargetChange{{From: date(t, "2026-10-27"), After: target(t, "2026-10-27", 2100)}}}.Check(s, levels(), targets)
	if len(cs) != 1 || !slices.ContainsFunc(cs[0].Violations, func(v limits.Violation) bool {
		return v.Rule == limits.RuleUnderweightLoss && v.Level == limits.LevelCode
	}) {
		t.Errorf("Check below a BMI of 18.5 = %+v", cs)
	}
}

func TestChangesTargetTimeline(t *testing.T) {
	t.Parallel()

	_, targets := checkState(t)
	from := date(t, "2026-10-27")
	c := Changes{Targets: []TargetChange{
		{From: targets[1].EffectiveFrom, Before: &targets[1]},
		{From: from, After: target(t, "2026-10-27", 2250)},
	}}
	if got, want := c.TargetTimeline(targets), []plan.Target{targets[0], *target(t, "2026-10-27", 2250)}; !slices.Equal(got, want) {
		t.Errorf("TargetTimeline = %+v, want %+v", got, want)
	}
	if got := (Changes{}).TargetTimeline(nil); got != nil {
		t.Errorf("the timeline of nothing = %+v", got)
	}
}

// Training weekdays with no target to move check nothing.
func TestChangesCheckWeekdaysWithoutATarget(t *testing.T) {
	t.Parallel()

	s, _ := checkState(t)
	s.Target = nil
	if cs := (Changes{TrainingWeekdays: &WeekdaysChange{After: 1}}).Check(s, levels(), nil); len(cs) != 0 {
		t.Errorf("Check = %+v, want none", cs)
	}
}

// New training weekdays move the weekly average of every split target to
// come: each is checked with them.
func TestChangesCheckTheTargetsToCome(t *testing.T) {
	t.Parallel()

	s, targets := checkState(t)
	// A split target from 2026-11-02: 3000 kcal on a training day, 1600
	// on a rest day, of -0.43 % a week with three training days, of -1.07
	// with none.
	targets[1].Training.Kcal, targets[1].Rest.Kcal = 3000, 1600
	cs := Changes{TrainingWeekdays: &WeekdaysChange{Before: s.TrainingDays, After: 0}}.Check(s, levels(), targets)
	want := map[string][]string{"training_weekdays 2026-10-26": {}, "training_weekdays 2026-11-02": {limits.RuleImpliedLossRate}}
	if got := rules(cs); !mapsEqual(got, want) {
		t.Errorf("Check = %v, want %v", got, want)
	}
}
