package autopilot

import (
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
)

func TestRevisionsOf(t *testing.T) {
	t.Parallel()

	cal, err := day.NewCalendar("Europe/Zagreb", 4)
	if err != nil {
		t.Fatal(err)
	}
	user := event.Actor{Kind: event.User, Client: "web"}
	auto := event.Actor{Kind: event.Autopilot, Client: event.ClientAutopilot}
	// 02:00 in Zagreb on the 22nd is still the 21st's day, which starts
	// at 04:00.
	at := func(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, cal.Location) }
	before := baseTarget
	lower := baseTarget.ShiftKcal(-100)
	cutPh := cut()
	faster := cutPh
	faster.RatePctWeek = -0.7
	ps := []proposal.Proposal{
		// Newest first, as the store lists them.
		{Status: proposal.Applied, By: auto, DecidedBy: user, DecidedAt: at(29, 9), TDEE: 2500,
			Changes: proposal.Changes{Targets: []proposal.TargetChange{{Before: &lower, After: &before}}}},
		{Status: proposal.Rejected, By: auto, DecidedBy: user, DecidedAt: at(28, 9),
			Changes: proposal.Changes{Targets: []proposal.TargetChange{{Before: &before, After: &lower}}}},
		{Status: proposal.Reverted, By: auto, DecidedBy: auto, DecidedAt: at(22, 2), TDEE: 2486,
			Changes: proposal.Changes{Targets: []proposal.TargetChange{{Before: &before, After: &lower}}}},
		{Status: proposal.Applied, By: user, DecidedBy: user, DecidedAt: at(14, 9),
			Changes: proposal.Changes{Phases: []proposal.PhaseChange{{Before: &cutPh, After: &faster}}}},
		{Status: proposal.Applied, By: user, DecidedBy: user, DecidedAt: at(10, 9),
			Changes: proposal.Changes{TrainingWeekdays: &proposal.WeekdaysChange{After: plan.Weekdays(0b1)}}},
		{Status: proposal.Pending, By: user, Changes: proposal.Changes{Targets: []proposal.TargetChange{{After: &lower}}}},
	}
	got := RevisionsOf(ps, cal, training)
	want := []Revision{
		{Day: day.Date{Year: 2026, Month: 9, Day: 10}, By: ByUser},
		{Day: day.Date{Year: 2026, Month: 9, Day: 14}, By: ByUser, ChangesPhase: true},
		{Day: day.Date{Year: 2026, Month: 9, Day: 21}, By: ByAutopilot, SetsKcal: true, DeltaKcal: -100, TDEE: 2486},
		{Day: day.Date{Year: 2026, Month: 9, Day: 29}, By: ByAccepted, SetsKcal: true, DeltaKcal: 100, TDEE: 2500},
	}
	if !equal(got, want) {
		t.Errorf("RevisionsOf =\n%+v, want\n%+v", got, want)
	}
}
