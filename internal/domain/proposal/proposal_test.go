package proposal

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

func date(tb testing.TB, s string) day.Date {
	tb.Helper()
	d, err := day.ParseDate(s)
	if err != nil {
		tb.Fatalf("ParseDate(%s): %v", s, err)
	}
	return d
}

func cut(tb testing.TB, start, end string, rate float64) *plan.Phase {
	tb.Helper()
	p := &plan.Phase{Type: plan.Cut, Start: date(tb, start), RatePctWeek: rate}
	if end != "" {
		p.End = date(tb, end)
	}
	return p
}

func target(tb testing.TB, from string, kcal float64) *plan.Target {
	tb.Helper()
	d := plan.DayTarget{Kcal: kcal, Protein: 150, Fat: 60}
	return &plan.Target{EffectiveFrom: date(tb, from), Training: d, Rest: d}
}

func TestChangesValidate(t *testing.T) {
	t.Parallel()

	today := date(t, "2026-10-26")
	// The current phase started before today and runs on.
	current := cut(t, "2026-09-07", "", -0.5)
	faster := cut(t, "2026-09-07", "", -0.7)
	endedYesterday := cut(t, "2026-09-07", "2026-10-25", -0.5)
	bulk := &plan.Phase{Type: plan.Bulk, Start: date(t, "2027-01-04"), RatePctWeek: 0.25}
	timeline := []plan.Phase{*current, *bulk}
	for _, tc := range []struct {
		name string
		c    Changes
	}{
		{"a new phase ahead", Changes{Phases: []PhaseChange{{Start: bulk.Start, After: bulk}}}},
		{"a new phase today", Changes{Phases: []PhaseChange{{Start: today, After: cut(t, "2026-10-26", "", -0.5)}}}},
		{"the current phase's rate", Changes{Phases: []PhaseChange{{Start: current.Start, Before: current, After: faster}}}},
		{"the current phase's rate and the next one's", Changes{Phases: []PhaseChange{
			{Start: current.Start, Before: current, After: faster},
			{Start: bulk.Start, Before: bulk, After: &plan.Phase{Type: plan.Bulk, Start: bulk.Start, RatePctWeek: 0.2}},
		}}},
		{"the current phase ended yesterday", Changes{Phases: []PhaseChange{
			{Start: current.Start, Before: current, After: endedYesterday},
			{Start: today, After: &plan.Phase{Type: plan.Maintain, Start: today}},
		}}},
		{"a phase ahead deleted", Changes{Phases: []PhaseChange{{Start: bulk.Start, Before: bulk}}}},
		{"a phase from today deleted", Changes{Phases: []PhaseChange{{Start: today, Before: cut(t, "2026-10-26", "", -0.5)}}}},
		{"a target from today", Changes{Targets: []TargetChange{{From: today, After: target(t, "2026-10-26", 2100)}}}},
		{"a target ahead changed", Changes{Targets: []TargetChange{
			{From: date(t, "2026-11-02"), Before: target(t, "2026-11-02", 2100), After: target(t, "2026-11-02", 2000)},
		}}},
		{"a target ahead deleted", Changes{Targets: []TargetChange{{From: date(t, "2026-11-02"), Before: target(t, "2026-11-02", 2100)}}}},
		{"the training weekdays", Changes{TrainingWeekdays: &WeekdaysChange{Before: 0b0101010, After: 0b0010101}}},
		{"none", Changes{TrainingWeekdays: &WeekdaysChange{Before: 0b0101010, After: 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.c.Validate(today, timeline); err != nil {
				t.Errorf("Validate = %v", err)
			}
		})
	}
}

func TestChangesValidateRefuses(t *testing.T) {
	t.Parallel()

	today := date(t, "2026-10-26")
	current := cut(t, "2026-09-07", "", -0.5)
	past := cut(t, "2026-08-03", "2026-09-06", -0.5)
	// An open phase the past one ended.
	open := cut(t, "2026-06-01", "", -0.5)
	maintain := &plan.Phase{Type: plan.Maintain, Start: current.Start}
	tomorrow := date(t, "2026-10-27")
	timeline := []plan.Phase{*open, *past, *current}
	for _, tc := range []struct {
		name string
		c    Changes
		want string
	}{
		{"nothing", Changes{}, "changes nothing"},
		{"a phase twice", Changes{Phases: []PhaseChange{
			{Start: tomorrow, After: cut(t, "2026-10-27", "", -0.5)},
			{Start: tomorrow, After: cut(t, "2026-10-27", "", -0.7)},
		}}, "the phase from 2026-10-27 twice"},
		{
			"a phase under another start",
			Changes{Phases: []PhaseChange{{Start: tomorrow, After: cut(t, "2026-10-28", "", -0.5)}}},
			"the phase from 2026-10-27 starts on 2026-10-28",
		},
		{
			"a phase of the wrong shape",
			Changes{Phases: []PhaseChange{{Start: tomorrow, After: &plan.Phase{Type: plan.Bulk, Start: tomorrow, RatePctWeek: -0.5}}}},
			"a bulk's is 0 or a gain",
		},
		{"a phase without a start", Changes{Phases: []PhaseChange{{After: &plan.Phase{Type: plan.Maintain}}}}, "a phase needs a start"},
		{"a delete of no phase", Changes{Phases: []PhaseChange{{Start: tomorrow}}}, "no phase from 2026-10-27 to delete"},
		{
			"a phase as it is",
			Changes{Phases: []PhaseChange{{Start: current.Start, Before: current, After: cut(t, "2026-09-07", "", -0.5)}}},
			"the phase from 2026-09-07 is already so",
		},
		{
			"a new phase in the past",
			Changes{Phases: []PhaseChange{{Start: date(t, "2026-10-25"), After: cut(t, "2026-10-25", "", -0.5)}}},
			"starts on 2026-10-25, before today",
		},
		{"the current phase deleted", Changes{Phases: []PhaseChange{{Start: current.Start, Before: current}}}, "the phase from 2026-09-07 has started"},
		{
			"the current phase of another type",
			Changes{Phases: []PhaseChange{{Start: current.Start, Before: current, After: maintain}}},
			"a maintain phase from 2026-09-07 for the cut",
		},
		{"the current phase ended before yesterday", Changes{Phases: []PhaseChange{
			{Start: current.Start, Before: current, After: cut(t, "2026-09-07", "2026-10-24", -0.5)},
		}}, "ends on 2026-10-24, before yesterday"},
		{
			"a past phase",
			Changes{Phases: []PhaseChange{{Start: past.Start, Before: past, After: cut(t, "2026-08-03", "2026-09-06", -0.7)}}},
			"the phase from 2026-08-03 ended on 2026-09-06",
		},
		{
			"a phase the next one ended",
			Changes{Phases: []PhaseChange{{Start: open.Start, Before: open, After: cut(t, "2026-06-01", "", -0.7)}}},
			"the phase from 2026-06-01 ended on 2026-08-02",
		},
		{"a target twice", Changes{Targets: []TargetChange{
			{From: tomorrow, After: target(t, "2026-10-27", 2100)},
			{From: tomorrow, After: target(t, "2026-10-27", 2000)},
		}}, "the target from 2026-10-27 twice"},
		{
			"a target under another day",
			Changes{Targets: []TargetChange{{From: tomorrow, After: target(t, "2026-10-28", 2100)}}},
			"the target from 2026-10-27 takes effect on 2026-10-28",
		},
		{"a target of the wrong shape", Changes{Targets: []TargetChange{{From: tomorrow, After: target(t, "2026-10-27", -1)}}}, "kcal -1"},
		{"a target without a day", Changes{Targets: []TargetChange{{After: &plan.Target{}}}}, "a target needs the day it takes effect"},
		{"a delete of no target", Changes{Targets: []TargetChange{{From: tomorrow}}}, "no target from 2026-10-27 to delete"},
		{
			"a target as it is",
			Changes{Targets: []TargetChange{{From: tomorrow, Before: target(t, "2026-10-27", 2100), After: target(t, "2026-10-27", 2100)}}},
			"the target from 2026-10-27 is already so",
		},
		{
			"a target in the past",
			Changes{Targets: []TargetChange{{From: date(t, "2026-10-25"), After: target(t, "2026-10-25", 2100)}}},
			"the target from 2026-10-25, before today",
		},
		{
			"a past target deleted",
			Changes{Targets: []TargetChange{{From: date(t, "2026-10-25"), Before: target(t, "2026-10-25", 2100)}}},
			"the target from 2026-10-25, before today",
		},
		{"weekdays of no week", Changes{TrainingWeekdays: &WeekdaysChange{After: 0b10000000}}, "want only the bits"},
		{"the weekdays as they are", Changes{TrainingWeekdays: &WeekdaysChange{Before: 0b0101010, After: 0b0101010}}, "the training weekdays are already so"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.c.Validate(today, timeline)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want %q", err, tc.want)
			}
		})
	}
}

// The current phase is over once the next starts, ended or not.
func TestChangesValidateAsTheNextPhaseStarts(t *testing.T) {
	t.Parallel()

	current := cut(t, "2026-09-07", "", -0.5)
	next := &plan.Phase{Type: plan.Maintain, Start: date(t, "2026-10-27")}
	c := Changes{Phases: []PhaseChange{{Start: current.Start, Before: current, After: cut(t, "2026-09-07", "", -0.7)}}}
	timeline := []plan.Phase{*current, *next}
	if err := c.Validate(date(t, "2026-10-26"), timeline); err != nil {
		t.Errorf("Validate the day before = %v", err)
	}
	if err := c.Validate(date(t, "2026-10-27"), timeline); err == nil || !strings.Contains(err.Error(), "ended on 2026-10-26") {
		t.Errorf("Validate as the next starts = %v", err)
	}
}

func TestChangesInverse(t *testing.T) {
	t.Parallel()

	current, faster := cut(t, "2026-09-07", "", -0.5), cut(t, "2026-09-07", "", -0.7)
	bulk := &plan.Phase{Type: plan.Bulk, Start: date(t, "2027-01-04"), RatePctWeek: 0.25}
	from := date(t, "2026-11-02")
	c := Changes{
		Phases:           []PhaseChange{{Start: current.Start, Before: current, After: faster}, {Start: bulk.Start, After: bulk}},
		Targets:          []TargetChange{{From: from, Before: target(t, "2026-11-02", 2100)}},
		TrainingWeekdays: &WeekdaysChange{Before: 0b0101010, After: 0b0010101},
	}
	want := Changes{
		Phases:           []PhaseChange{{Start: current.Start, Before: faster, After: current}, {Start: bulk.Start, Before: bulk}},
		Targets:          []TargetChange{{From: from, After: target(t, "2026-11-02", 2100)}},
		TrainingWeekdays: &WeekdaysChange{Before: 0b0010101, After: 0b0101010},
	}
	if got := c.Inverse(); !reflect.DeepEqual(got, want) {
		t.Errorf("Inverse = %+v, want %+v", got, want)
	}
	if got := (Changes{}).Inverse(); !reflect.DeepEqual(got, Changes{}) {
		t.Errorf("the inverse of nothing = %+v", got)
	}
}

func TestChangesTimeline(t *testing.T) {
	t.Parallel()

	current := cut(t, "2026-09-07", "", -0.5)
	bulk := &plan.Phase{Type: plan.Bulk, Start: date(t, "2027-01-04"), RatePctWeek: 0.25}
	brk := &plan.Phase{Type: plan.Maintain, Start: date(t, "2026-12-21"), End: date(t, "2027-01-03")}
	ended := cut(t, "2026-09-07", "2026-12-20", -0.5)
	c := Changes{Phases: []PhaseChange{
		{Start: bulk.Start, Before: bulk},
		{Start: brk.Start, After: brk},
		{Start: current.Start, Before: current, After: ended},
	}}
	got := c.Timeline([]plan.Phase{*current, *bulk})
	if want := []plan.Phase{*ended, *brk}; !reflect.DeepEqual(got, want) {
		t.Errorf("Timeline = %+v, want %+v", got, want)
	}
	if got := (Changes{}).Timeline(nil); got != nil {
		t.Errorf("the timeline of nothing = %+v, want none", got)
	}
}

func TestWindowValidate(t *testing.T) {
	t.Parallel()

	today := date(t, "2026-10-26")
	if err := (Window{From: date(t, "2026-09-26"), To: today}).Validate(today); err != nil {
		t.Errorf("a month up to today = %v", err)
	}
	if err := (Window{From: today, To: today}).Validate(today); err != nil {
		t.Errorf("today alone = %v", err)
	}
	for _, tc := range []struct {
		name string
		w    Window
		want string
	}{
		{"no start", Window{To: today}, "a window needs its first and last day"},
		{"no end", Window{From: today}, "a window needs its first and last day"},
		{"backwards", Window{From: today, To: date(t, "2026-10-25")}, "ends on 2026-10-25, before it starts on 2026-10-26"},
		{"ahead", Window{From: today, To: date(t, "2026-10-27")}, "ends on 2026-10-27, after today"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.w.Validate(today); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateRationale(t *testing.T) {
	t.Parallel()

	if err := ValidateRationale("The trend fell 0.4 % a week, under the cut's 0.5 %.\nA step of 100 kcal."); err != nil {
		t.Errorf("a rationale of two lines = %v", err)
	}
	for _, r := range []string{"", " ", strings.Repeat("x", MaxRationaleLen+1)} {
		if err := ValidateRationale(r); err == nil {
			t.Errorf("ValidateRationale(%.10q) = nil, want an error", r)
		}
	}
}

func TestStatusAt(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	p := Proposal{Status: Pending, CreatedAt: created, ExpiresAt: created.Add(TTL)}
	for _, tc := range []struct {
		name   string
		status Status
		at     time.Time
		want   Status
	}{
		{"pending, a day on", Pending, created.Add(24 * time.Hour), Pending},
		{"pending, a second before it expires", Pending, created.Add(TTL - time.Second), Pending},
		{"pending, as it expires", Pending, created.Add(TTL), Expired},
		{"pending, after", Pending, created.Add(30 * 24 * time.Hour), Expired},
		{"applied, after", Applied, created.Add(30 * 24 * time.Hour), Applied},
		{"rejected, after", Rejected, created.Add(30 * 24 * time.Hour), Rejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := p
			p.Status = tc.status
			if got := p.StatusAt(tc.at); got != tc.want {
				t.Errorf("StatusAt = %s, want %s", got, tc.want)
			}
		})
	}
}
