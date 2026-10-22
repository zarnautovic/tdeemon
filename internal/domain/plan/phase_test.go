package plan

import (
	"math"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

func date(tb testing.TB, s string) day.Date {
	tb.Helper()
	d, err := day.ParseDate(s)
	if err != nil {
		tb.Fatalf("ParseDate(%s): %v", s, err)
	}
	return d
}

func TestPhaseValidate(t *testing.T) {
	t.Parallel()

	start := date(t, "2026-11-02")
	for _, p := range []Phase{
		{Type: Cut, Start: start, RatePctWeek: -0.5},
		{Type: Cut, Start: start, RatePctWeek: 0}, // a slower rate is always allowed
		{Type: Maintain, Start: start},
		{Type: Bulk, Start: start, RatePctWeek: 0.25},
		{Type: Bulk, Start: start},
		{Type: Recomp, Start: start, RatePctWeek: -0.1},
		{Type: Recomp, Start: start, RatePctWeek: 0.2},
		{Type: Cut, Start: start, End: start, RatePctWeek: -0.5},                        // a phase of one day
		{Type: Maintain, Start: start, End: date(t, "2026-11-15")},                      // a diet break
		{Type: Cut, Start: start, RatePctWeek: -0.5, GoalWeight: new(72.0)},             // below the weight now
		{Type: Bulk, Start: start, RatePctWeek: 0.2, MilestoneWeight: new(85.0)},        // above it
		{Type: Cut, Start: start, RatePctWeek: -0.7, GoalWeight: new(20.0)},             // the least a weight is
		{Type: Cut, Start: start, RatePctWeek: -0.7, MilestoneWeight: new(400.0)},       // the most
		{Type: Recomp, Start: start, GoalWeight: new(80.5), MilestoneWeight: new(79.0)}, // both
	} {
		if err := p.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", p, err)
		}
	}
	for _, tc := range []struct {
		name string
		p    Phase
		want string
	}{
		{"an unknown type", Phase{Type: "diet", Start: start}, `phase type "diet"`},
		{"no type", Phase{Start: start}, `phase type ""`},
		{"no start", Phase{Type: Maintain}, "needs a start"},
		{"an end before the start", Phase{Type: Maintain, Start: start, End: date(t, "2026-11-01")}, "before it starts"},
		{"a cut that gains", Phase{Type: Cut, Start: start, RatePctWeek: 0.1}, "a cut's is 0 or a loss"},
		{"a bulk that loses", Phase{Type: Bulk, Start: start, RatePctWeek: -0.1}, "a bulk's is 0 or a gain"},
		{"maintenance that loses", Phase{Type: Maintain, Start: start, RatePctWeek: -0.1}, "maintenance's is 0"},
		{"maintenance that gains", Phase{Type: Maintain, Start: start, RatePctWeek: 0.05}, "maintenance's is 0"},
		{"a rate that is no number", Phase{Type: Recomp, Start: start, RatePctWeek: math.NaN()}, "rate NaN"},
		{"an infinite rate", Phase{Type: Cut, Start: start, RatePctWeek: math.Inf(-1)}, "rate -Inf"},
		{"a goal too low", Phase{Type: Cut, Start: start, GoalWeight: new(19.9)}, "goal weight 19.9: want 20 to 400 kg"},
		{"a milestone too high", Phase{Type: Bulk, Start: start, MilestoneWeight: new(401.0)}, "milestone weight 401"},
		{"a goal that is no number", Phase{Type: Bulk, Start: start, GoalWeight: new(math.NaN())}, "goal weight NaN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.p.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error with %q", err, tc.want)
			}
		})
	}

	// An unknown type is not also reported for its rate's sign.
	if err := (Phase{Type: "diet", Start: start, RatePctWeek: 1}).Validate(); err == nil || strings.Contains(err.Error(), "rate") {
		t.Errorf("Validate of an unknown type = %v, want only the type", err)
	}
	// Every rule broken is reported.
	err := Phase{Type: Cut, End: start, RatePctWeek: 1, GoalWeight: new(1.0), MilestoneWeight: new(1.0)}.Validate()
	for _, want := range []string{"needs a start", "a cut's", "goal weight", "milestone weight"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate = %v, want it to say %q", err, want)
		}
	}
}

func TestPhaseOn(t *testing.T) {
	t.Parallel()

	open := Phase{Type: Cut, Start: date(t, "2026-11-02"), RatePctWeek: -0.5}
	closed := Phase{Type: Maintain, Start: date(t, "2026-11-02"), End: date(t, "2026-11-08")}
	for _, tc := range []struct {
		name string
		p    Phase
		d    string
		want bool
	}{
		{"the day before the start", open, "2026-11-01", false},
		{"the start", open, "2026-11-02", true},
		{"long after, with no end", open, "2027-06-30", true},
		{"the end", closed, "2026-11-08", true},
		{"the day after the end", closed, "2026-11-09", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.On(date(t, tc.d)); got != tc.want {
				t.Errorf("On(%s) = %v, want %v", tc.d, got, tc.want)
			}
		})
	}
}

// A cut, a diet break of a week, the cut again, a gap, then a bulk with
// no end.
func timeline(tb testing.TB) []Phase {
	tb.Helper()
	return []Phase{
		{Type: Cut, Start: date(tb, "2026-09-07"), RatePctWeek: -0.5},
		{Type: Maintain, Start: date(tb, "2026-11-02"), End: date(tb, "2026-11-08")},
		{Type: Cut, Start: date(tb, "2026-11-09"), End: date(tb, "2026-12-20"), RatePctWeek: -0.5},
		{Type: Bulk, Start: date(tb, "2027-01-04"), RatePctWeek: 0.25},
	}
}

func TestValidateTimeline(t *testing.T) {
	t.Parallel()

	if err := ValidateTimeline(timeline(t)); err != nil {
		t.Errorf("ValidateTimeline = %v", err)
	}
	if err := ValidateTimeline(nil); err != nil {
		t.Errorf("ValidateTimeline(nil) = %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func([]Phase)
		want   string
	}{
		{"a phase that breaks a rule", func(ps []Phase) { ps[1].RatePctWeek = 1 }, "phase 2: rate 1"},
		{"two starting on one day", func(ps []Phase) { ps[1].Start = ps[0].Start }, "phase 2 starts on 2026-09-07, not after phase 1's start"},
		{"out of order", func(ps []Phase) { ps[3].Start = date(t, "2026-11-01") }, "phase 4 starts on 2026-11-01, not after phase 3's"},
		{"starting on the end of the one before", func(ps []Phase) { ps[2].Start = ps[1].End }, "phase 3 starts on 2026-11-08, while phase 2 runs to 2026-11-08"},
		{"starting inside the one before", func(ps []Phase) { ps[3].Start = date(t, "2026-12-01") }, "while phase 3 runs to 2026-12-20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps := timeline(t)
			tc.change(ps)
			if err := ValidateTimeline(ps); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ValidateTimeline = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func TestPhaseOnATimeline(t *testing.T) {
	t.Parallel()

	ps := timeline(t)
	for _, tc := range []struct {
		d    string
		want int // the phase's index, or -1 for none
	}{
		{"2026-09-06", -1},
		{"2026-09-07", 0},
		{"2026-11-01", 0}, // the open cut lasts until the break starts
		{"2026-11-02", 1},
		{"2026-11-08", 1},
		{"2026-11-09", 2},
		{"2026-12-20", 2},
		{"2026-12-21", -1}, // between the cut's end and the bulk
		{"2027-01-03", -1},
		{"2027-01-04", 3},
		{"2030-01-01", 3},
	} {
		t.Run(tc.d, func(t *testing.T) {
			t.Parallel()
			got, ok := PhaseOn(ps, date(t, tc.d))
			switch {
			case tc.want < 0 && (ok || got != (Phase{})):
				t.Errorf("PhaseOn(%s) = %+v, %v; want none", tc.d, got, ok)
			case tc.want >= 0 && (!ok || got.Start != ps[tc.want].Start):
				t.Errorf("PhaseOn(%s) = %+v, %v; want phase %d", tc.d, got, ok, tc.want+1)
			}
		})
	}
	if _, ok := PhaseOn(nil, date(t, "2026-10-21")); ok {
		t.Error("PhaseOn found a phase in no timeline")
	}
}
