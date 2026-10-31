package store_test

import (
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/autopilot"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

func TestAutopilotSettings(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	if got, err := db.AutopilotSettings(t.Context(), ana); err != nil || got != autopilot.DefaultSettings() {
		t.Fatalf("settings before any = %+v, %v; want the defaults", got, err)
	}
	s := autopilot.Settings{Mode: autopilot.Auto, Weekday: time.Saturday}
	if err := db.SetAutopilotSettings(t.Context(), ana, testActor, s, t0); err != nil {
		t.Fatal(err)
	}
	if got, err := db.AutopilotSettings(t.Context(), ana); err != nil || got != s {
		t.Errorf("settings = %+v, %v; want %+v", got, err, s)
	}
	if got, _ := db.AutopilotSettings(t.Context(), ivo); got != autopilot.DefaultSettings() {
		t.Errorf("Ivo's settings = %+v, want the defaults", got)
	}
	// The training weekdays and the limits share the row, and stay.
	withPlan(t, db, ana)
	if got, _ := db.AutopilotSettings(t.Context(), ana); got != s {
		t.Errorf("settings after the plan = %+v, want %+v", got, s)
	}
	if err := db.SetAutopilotSettings(t.Context(), ana, testActor, autopilot.Settings{Mode: "always"}, t0); err == nil {
		t.Error("an unknown mode was stored")
	}
	// Undo leaves the settings, as it leaves the limits.
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("UndoLast = %v, want nothing to undo", err)
	}
}

// step is a step of 100 kcal down from planValues' target, today.
func step(t *testing.T, outcome autopilot.Outcome) autopilot.Review {
	t.Helper()
	_, tg := planValues(t)
	today := day.DateOf(t0)
	after := tg.ShiftKcal(-100)
	after.EffectiveFrom = today
	return autopilot.Review{
		Date: today, Outcome: outcome, Reason: autopilot.ReasonCutSlow, StepKcal: -100, GapKcal: -300,
		TDEE: 2400, HalfWidth: 120, TargetKcal: 2129, AfterKcal: 2029, DriftKcal: -100, DriftLimitKcal: 360,
		From: today.AddDays(7), Changes: proposal.Changes{Targets: []proposal.TargetChange{{From: today, After: &after}}},
	}
}

func TestSaveReviewAppliesAStep(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	waits := propose(t, db, ana, coachDraft(t), t0)
	var checked proposal.Changes
	r := step(t, autopilot.Applied)
	got, err := db.SaveReview(t.Context(), ana, r, 2400, t0, func(c proposal.Changes) (float64, error) {
		checked = c
		return 2400, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(checked.Targets) != 1 {
		t.Errorf("checked %+v, want the step", checked)
	}
	p, err := db.Proposal(t.Context(), ana, got.ProposalID, t0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != proposal.Applied || p.By != store.AutopilotActor || p.DecidedBy != store.AutopilotActor ||
		p.Rationale != proposal.Autopilot || p.TDEE != 2400 {
		t.Errorf("the step's proposal %+v, want applied by the autopilot with a TDEE of 2400", p)
	}
	wantStatus(t, db, ana, waits.ID, t0, proposal.Superseded)
	targets, err := db.Targets(t.Context(), ana)
	if err != nil {
		t.Fatal(err)
	}
	if last := targets[len(targets)-1]; last.Rest.Kcal != 1900 || last.Training.Kcal != 2200 {
		t.Errorf("the target now %+v, want 100 kcal less on both types of day", last)
	}
	if got.Review.Reason != r.Reason || got.Review.From != r.From || got.Review.DriftLimitKcal != 360 {
		t.Errorf("stored %+v, want the review's numbers", got.Review)
	}
	wantTakenBackInHistory(t, db, ana, p.ID)
}

// wantTakenBackInHistory checks that undo leaves userID's step id, and
// a revert in the plan's history takes it back.
func wantTakenBackInHistory(t *testing.T, db *store.DB, userID, id uuid.UUID) {
	t.Helper()
	if _, err := db.UndoLast(t.Context(), userID, testActor, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("UndoLast = %v, want nothing to undo", err)
	}
	back, err := db.RevertProposal(t.Context(), userID, testActor, id, proposal.ChangedInTheApp, day.DateOf(t0), t0, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, db, userID, back.ID, t0)
	wantStatus(t, db, userID, id, t0, proposal.Reverted)
}

func TestSaveReviewProposesAndHolds(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withPlans(t)
	got, err := db.SaveReview(t.Context(), ana, step(t, autopilot.Proposed), 2400, t0, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.Proposal(t.Context(), ana, got.ProposalID, t0)
	if err != nil || p.Status != proposal.Pending || p.By.Kind != event.Autopilot || p.TDEE != 0 {
		t.Fatalf("the proposal %+v, %v; want one of the autopilot's that waits", p, err)
	}
	// A second review of the day changes nothing.
	n := countRows(t, db, "events")
	if _, err := db.SaveReview(t.Context(), ana, step(t, autopilot.Applied), 2400, t0, nil); !errors.Is(err, store.ErrReviewed) {
		t.Errorf("a second review = %v, want ErrReviewed", err)
	}
	if countRows(t, db, "events") != n {
		t.Error("a second review recorded an event")
	}
	hold := autopilot.Review{Date: day.DateOf(t0).AddDays(7), Outcome: autopilot.Hold, Reason: autopilot.ReasonOnTarget, MeasuredPctWeek: -0.47}
	if _, err := db.SaveReview(t.Context(), ana, hold, 0, t0.Add(7*24*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	wantReviews(t, db, ana, ivo, p.ID)
}

// wantReviews checks userID's two reviews, a hold and then the proposal
// id, which other does not see.
func wantReviews(t *testing.T, db *store.DB, ana, ivo, id uuid.UUID) {
	t.Helper()
	p := proposal.Proposal{ID: id}
	reviews, err := db.Reviews(t.Context(), ana, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 2 || reviews[0].Review.Outcome != autopilot.Hold || reviews[0].ProposalID != uuid.Nil() ||
		reviews[1].ProposalID != p.ID || reviews[0].Review.MeasuredPctWeek != -0.47 {
		t.Errorf("reviews %+v, want the hold and then the proposal", reviews)
	}
	of, err := db.ReviewsOf(t.Context(), ana, []uuid.UUID{p.ID, reviews[0].ProposalID})
	if err != nil || len(of) != 1 || of[p.ID].Review.Reason != autopilot.ReasonCutSlow {
		t.Errorf("ReviewsOf = %+v, %v; want the proposal's review alone", of, err)
	}
	if other, err := db.Reviews(t.Context(), ivo, 10); err != nil || len(other) != 0 {
		t.Errorf("Ivo's reviews = %+v, %v; want none", other, err)
	}
	if other, err := db.ReviewsOf(t.Context(), ivo, []uuid.UUID{p.ID}); err != nil || len(other) != 0 {
		t.Errorf("Ivo's review of Ana's proposal = %+v, %v; want none", other, err)
	}
}

func TestARefusedStepWritesNothing(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	n := countRows(t, db, "events")
	refuse := func(proposal.Changes) (float64, error) { return 0, errRefused }
	if _, err := db.SaveReview(t.Context(), ana, step(t, autopilot.Applied), 2400, t0, refuse); !errors.Is(err, errRefused) {
		t.Errorf("SaveReview refused = %v", err)
	}
	if countRows(t, db, "events") != n || countRows(t, db, "autopilot_reviews") != 0 {
		t.Error("a refused step wrote")
	}
}

func TestAnAppliedProposalKeepsTheTDEE(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, coachDraft(t), t0)
	applied, err := db.ApplyProposal(t.Context(), ana, testActor, p.ID, day.DateOf(t0), t0,
		func(proposal.Changes) (float64, error) { return 2450, nil })
	if err != nil {
		t.Fatal(err)
	}
	if applied.TDEE != 2450 {
		t.Errorf("TDEE %v, want 2450", applied.TDEE)
	}
	d := proposal.Draft{Rationale: proposal.ChangedInTheApp, Changes: proposal.Changes{Targets: []proposal.TargetChange{{
		From: day.DateOf(t0).AddDays(2), After: &plan.Target{EffectiveFrom: day.DateOf(t0).AddDays(2), Training: dayTarget(2100, 150, 60), Rest: dayTarget(2100, 150, 60)},
	}}}}
	changed, err := db.ChangePlan(t.Context(), ana, testActor, d, day.DateOf(t0), t0, func(proposal.Changes) (float64, error) { return 2500, nil })
	if err != nil || changed.TDEE != 2500 {
		t.Errorf("ChangePlan = %v, TDEE %v; want 2500", err, changed.TDEE)
	}
}

func TestRecentChangesListTheAutopilotsStep(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	if _, err := db.SaveReview(t.Context(), ana, step(t, autopilot.Applied), 2400, t0, nil); err != nil {
		t.Fatal(err)
	}
	changes, err := db.RecentChanges(t.Context(), ana, 10)
	if err != nil {
		t.Fatal(err)
	}
	var autopilots int
	for _, c := range changes {
		if c.Event.Action == event.AutopilotReview {
			t.Error("a review is listed")
		}
		if c.Event.Actor.Kind == event.Autopilot {
			autopilots++
			if c.Next {
				t.Error("undo would take a step back")
			}
		}
	}
	if autopilots == 0 {
		t.Errorf("changes %+v, want the step's", changes)
	}
}
