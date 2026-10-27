package store_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// limitsDraft proposes a protein floor of 1.8 g/kg and a faster cut's
// limit, named in no order.
func limitsDraft() proposal.Draft {
	return proposal.Draft{
		Rationale: "A faster cut, with more protein to keep the muscle.",
		Changes: proposal.Changes{Limits: []proposal.LimitChange{
			{Name: "protein_floor", After: 1.8},
			{Name: "cut_rate", After: -1.0},
		}},
	}
}

// userLimits returns userID's limits, failing t on an error.
func userLimits(t *testing.T, db *store.DB, userID uuid.UUID) limits.Limits {
	t.Helper()
	l, err := db.UserLimits(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// A change of a limit has the limit as it is before it, the changes in
// the order of the limits.
func TestCreateProposalOfLimits(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, limitsDraft(), t0)
	want := []proposal.LimitChange{{Name: "cut_rate", Before: -0.7, After: -1.0}, {Name: "protein_floor", Before: 1.6, After: 1.8}}
	if !reflect.DeepEqual(p.Changes.Limits, want) {
		t.Errorf("the changes of the limits = %+v, want %+v", p.Changes.Limits, want)
	}
	if got := getProposal(t, db, ana, p.ID, t0); !reflect.DeepEqual(got.Changes, p.Changes) {
		t.Errorf("read back: %+v, want %+v", got.Changes, p.Changes)
	}
	if got := userLimits(t, db, ana); got != limits.UserDefaults() {
		t.Errorf("the limits = %+v, want them as they were", got)
	}
}

func TestCreateProposalOfLimitsRefuses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		changes []proposal.LimitChange
		want    string
	}{
		{"a limit of no name", []proposal.LimitChange{{Name: "speed", After: 1}}, `no limit "speed"`},
		{"a limit as it is", []proposal.LimitChange{{Name: "cut_rate", After: -0.7}}, "the limit cut_rate is already so"},
		{"limits that break their rules", []proposal.LimitChange{{Name: "recomp_min", After: 0.5}}, "the limits it leaves: recomp_min 0.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, ana, _ := withPlans(t)
			d := limitsDraft()
			d.Changes.Limits = tc.changes
			_, err := db.CreateProposal(t.Context(), ana, coachActor, d, day.DateOf(t0), t0, nil)
			if !errors.Is(err, store.ErrInvalidProposal) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("CreateProposal = %v, want %q", err, tc.want)
			}
		})
	}
}

// Applied, a proposal of the limits makes them the user's, as the
// actor's change of them; undo leaves them.
func TestApplyProposalOfLimits(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withPlans(t)
	p := propose(t, db, ana, limitsDraft(), t0)
	apply(t, db, ana, p.ID, t0)
	want := limits.UserDefaults()
	want.CutRate, want.ProteinFloor = -1.0, 1.8
	if got := userLimits(t, db, ana); got != want {
		t.Errorf("the limits = %+v, want %+v", got, want)
	}
	if got := userLimits(t, db, ivo); got != limits.UserDefaults() {
		t.Errorf("Ivo's limits = %+v, want them as they were", got)
	}
	e := lastEvent(t, db, ana)
	if e.Action != event.LimitsSet || e.Actor != testActor {
		t.Errorf("the last event = %+v, want the limits set by %+v", e, testActor)
	}
	if got := eventActions(t, db, ana, 2); !reflect.DeepEqual(got, []event.Action{event.LimitsSet, event.ProposalStatus}) {
		t.Errorf("the events = %v", got)
	}
}

// A revert of a proposal of the limits puts each back, once the limits
// are as it left them.
func TestRevertProposalOfLimits(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, limitsDraft(), t0)
	applied := apply(t, db, ana, p.ID, t0)
	later := t0.Add(time.Hour)
	revert := revertOf(t, db, ana, p.ID, later)
	if !reflect.DeepEqual(revert.Changes, applied.Changes.Inverse()) {
		t.Errorf("the revert = %+v, want %+v", revert.Changes, applied.Changes.Inverse())
	}
	apply(t, db, ana, revert.ID, later)
	if got := userLimits(t, db, ana); got != limits.UserDefaults() {
		t.Errorf("the limits = %+v, want them as before", got)
	}
	wantStatus(t, db, ana, p.ID, later, proposal.Reverted)
}

func TestRevertProposalOfLimitsChangedSince(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, limitsDraft(), t0)
	apply(t, db, ana, p.ID, t0)
	l := userLimits(t, db, ana)
	l.CutRate = -0.9
	if err := db.SetUserLimits(t.Context(), ana, testActor, l, limits.Instance(), t0); err != nil {
		t.Fatal(err)
	}
	_, err := db.RevertProposal(t.Context(), ana, coachActor, p.ID, "Back.", day.DateOf(t0), t0, nil)
	if !errors.Is(err, store.ErrInvalidProposal) || !strings.Contains(err.Error(), "the limit cut_rate changed since the proposal") {
		t.Errorf("RevertProposal = %v", err)
	}
}

// A change of the limits supersedes a pending proposal of them, whose
// limits before hold no more; one of the plan alone waits on, its
// preview read against the limits now.
func TestAChangeOfTheLimitsSupersedesAProposalOfThem(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withPlans(t)
	p := propose(t, db, ana, limitsDraft(), t0)
	ivos := propose(t, db, ivo, limitsDraft(), t0)
	// The limits as they are change nothing.
	if err := db.SetUserLimits(t.Context(), ana, testActor, limits.UserDefaults(), limits.Instance(), t0); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, db, ana, p.ID, t0, proposal.Pending)
	l := limits.UserDefaults()
	l.DriftPct = 10
	if err := db.SetUserLimits(t.Context(), ana, testActor, l, limits.Instance(), t0); err != nil {
		t.Fatal(err)
	}
	wantSuperseded(t, db, ana, p.ID, testActor, t0)
	wantStatus(t, db, ivo, ivos.ID, t0, proposal.Pending)
}

// Limits changed under a proposal, as an import of the database could,
// supersede it when it applies.
func TestApplyProposalOfLimitsUnderChangedLimits(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, limitsDraft(), t0)
	exec(t, db, `UPDATE plan_settings SET limits = '{"cut_rate": -0.8}' WHERE user_id = ?`, ana.String())
	_, err := db.ApplyProposal(t.Context(), ana, testActor, p.ID, day.DateOf(t0), t0, nil)
	wantClosed(t, err, proposal.Superseded)
	if got := userLimits(t, db, ana).ProteinFloor; got != 1.6 {
		t.Errorf("the protein floor = %v, want it as it was", got)
	}
}

// Limits it does not change, changed since so that those it leaves break
// their rules, refuse a proposal when it applies.
func TestApplyProposalOfLimitsThatBreakTheirRules(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	d := limitsDraft()
	d.Changes.Limits = []proposal.LimitChange{{Name: "recomp_min", After: -0.3}}
	p := propose(t, db, ana, d, t0)
	exec(t, db, `UPDATE plan_settings SET limits = '{"recomp_max": -0.4}' WHERE user_id = ?`, ana.String())
	_, err := db.ApplyProposal(t.Context(), ana, testActor, p.ID, day.DateOf(t0), t0, nil)
	if !errors.Is(err, store.ErrInvalidProposal) || !strings.Contains(err.Error(), "recomp_min -0.3: want at most recomp_max -0.4") {
		t.Errorf("ApplyProposal = %v", err)
	}
	wantStatus(t, db, ana, p.ID, t0, proposal.Pending)
}
