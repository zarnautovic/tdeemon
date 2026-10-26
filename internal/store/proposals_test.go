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
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// coachActor is a model proposing through MCP.
var coachActor = event.Actor{Kind: event.User, Client: "mcp", Model: "claude-opus-5-5"}

// planValues are the phase and the target withPlan sets: a cut from
// 2026-09-07 on, and a target from that day.
func planValues(t *testing.T) (plan.Phase, plan.Target) {
	t.Helper()
	ph := cut(t, "2026-09-07", "", -0.5)
	return ph, plan.Target{EffectiveFrom: ph.Start, Training: dayTarget(2300, 150, 70), Rest: dayTarget(2000, 150, 60)}
}

// withPlan gives userID the phase and the target of planValues, and
// training weekdays.
func withPlan(t *testing.T, db *store.DB, userID uuid.UUID) {
	t.Helper()
	ph, tg := planValues(t)
	if err := db.SetPhase(t.Context(), userID, testActor, ph, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTarget(t.Context(), userID, testActor, tg, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTrainingWeekdays(t.Context(), userID, testActor, plan.Weekdays(0b0101010), t0); err != nil {
		t.Fatal(err)
	}
}

// withPlans returns a database with Ana and Ivo, each with the plan of
// withPlan.
func withPlans(t *testing.T) (*store.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, ana, ivo := withTwoUsers(t)
	withPlan(t, db, ana)
	withPlan(t, db, ivo)
	return db, ana, ivo
}

// coachDraft proposes a faster cut, a target from tomorrow and training
// weekdays of four days.
func coachDraft(t *testing.T) proposal.Draft {
	t.Helper()
	tomorrow := day.DateOf(t0).AddDays(1)
	return proposal.Draft{
		Rationale: "The trend fell 0.3 % a week, under the cut's 0.5 %.",
		Window:    &proposal.Window{From: day.DateOf(t0).AddDays(-28), To: day.DateOf(t0)},
		Changes: proposal.Changes{
			Phases: []proposal.PhaseChange{{Start: date(t, "2026-09-07"), After: new(cut(t, "2026-09-07", "", -0.7))}},
			Targets: []proposal.TargetChange{{From: tomorrow, After: &plan.Target{
				EffectiveFrom: tomorrow, Training: dayTarget(2200, 160, 70), Rest: dayTarget(1900, 160, 60),
			}}},
			TrainingWeekdays: &proposal.WeekdaysChange{After: 0b0101011},
		},
	}
}

func propose(t *testing.T, db *store.DB, userID uuid.UUID, d proposal.Draft, now time.Time) proposal.Proposal {
	t.Helper()
	p, err := db.CreateProposal(t.Context(), userID, coachActor, d, day.DateOf(now), now)
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	return p
}

func apply(t *testing.T, db *store.DB, userID, id uuid.UUID, now time.Time) proposal.Proposal {
	t.Helper()
	p, err := db.ApplyProposal(t.Context(), userID, testActor, id, day.DateOf(now), now)
	if err != nil {
		t.Fatalf("ApplyProposal: %v", err)
	}
	return p
}

func getProposal(t *testing.T, db *store.DB, userID, id uuid.UUID, now time.Time) proposal.Proposal {
	t.Helper()
	p, err := db.Proposal(t.Context(), userID, id, now)
	if err != nil {
		t.Fatalf("Proposal: %v", err)
	}
	return p
}

// wantStatus fails t unless userID's proposal id is in status at now.
func wantStatus(t *testing.T, db *store.DB, userID, id uuid.UUID, now time.Time, status proposal.Status) {
	t.Helper()
	if got := getProposal(t, db, userID, id, now).Status; got != status {
		t.Errorf("the proposal is %s, want it %s", got, status)
	}
}

// wantDecided fails t unless userID's proposal id is in status, decided
// by a at at.
func wantDecided(t *testing.T, db *store.DB, userID, id uuid.UUID, status proposal.Status, a event.Actor, at time.Time) {
	t.Helper()
	p := getProposal(t, db, userID, id, at)
	if p.Status != status || p.DecidedBy != a || !p.DecidedAt.Equal(at) {
		t.Errorf("the proposal is %s, decided by %+v at %s; want it %s, by %+v at %s", p.Status, p.DecidedBy, p.DecidedAt, status, a, at)
	}
}

// wantClosed fails t unless err reports a proposal in status.
func wantClosed(t *testing.T, err error, status proposal.Status) {
	t.Helper()
	var c *store.ProposalClosedError
	if !errors.As(err, &c) || c.Status != status || err.Error() != "the proposal is "+string(status) {
		t.Errorf("err = %v, want the proposal %s", err, status)
	}
}

// eventActions returns the actions of userID's latest n events, newest
// first.
func eventActions(t *testing.T, db *store.DB, userID uuid.UUID, n int) []event.Action {
	t.Helper()
	es, err := db.Events(t.Context(), userID, n)
	if err != nil {
		t.Fatal(err)
	}
	var out []event.Action
	for i := range es {
		out = append(out, es[i].Action)
	}
	return out
}

func TestCreateProposal(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	ph, _ := planValues(t)
	d := coachDraft(t)
	p := propose(t, db, ana, d, t0)

	// Each change has the plan as it is before it.
	changes := coachDraft(t).Changes
	changes.Phases[0].Before = &ph
	changes.TrainingWeekdays.Before = 0b0101010
	want := proposal.Proposal{
		ID: p.ID, Status: proposal.Pending, By: coachActor, Rationale: d.Rationale, Window: d.Window,
		Changes: changes, CreatedAt: t0, ExpiresAt: t0.Add(proposal.TTL),
	}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("CreateProposal = %+v, want %+v", p, want)
	}
	if got := getProposal(t, db, ana, p.ID, t0); !reflect.DeepEqual(got, want) {
		t.Errorf("Proposal = %+v, want %+v", got, want)
	}
	if got, err := db.Proposals(t.Context(), ana, 10, t0); err != nil || !reflect.DeepEqual(got, []proposal.Proposal{want}) {
		t.Errorf("Proposals = %+v, %v", got, err)
	}
	e := lastEvent(t, db, ana)
	if e.Action != event.ProposalCreate || e.Entity != p.ID.String() || e.Before != nil || e.After == nil || e.Actor != coachActor {
		t.Errorf("the event = %+v", e)
	}
	// The plan is as it was.
	if got, _ := db.Phases(t.Context(), ana); !reflect.DeepEqual(got, []plan.Phase{ph}) {
		t.Errorf("Phases = %+v", got)
	}
}

func TestProposalsAreTheUsers(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withPlans(t)
	p := propose(t, db, ana, coachDraft(t), t0)
	if _, err := db.Proposal(t.Context(), ivo, p.ID, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Ivo's Proposal = %v, want ErrNotFound", err)
	}
	if got, err := db.Proposals(t.Context(), ivo, 10, t0); err != nil || len(got) != 0 {
		t.Errorf("Ivo's Proposals = %+v, %v", got, err)
	}
	if _, err := db.ApplyProposal(t.Context(), ivo, testActor, p.ID, day.DateOf(t0), t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Ivo applying Ana's = %v, want ErrNotFound", err)
	}
	if _, err := db.RejectProposal(t.Context(), ivo, testActor, p.ID, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Ivo rejecting Ana's = %v, want ErrNotFound", err)
	}
	// Ivo's own proposal leaves Ana's waiting.
	propose(t, db, ivo, coachDraft(t), t0)
	wantStatus(t, db, ana, p.ID, t0, proposal.Pending)
}

func TestCreateProposalRefuses(t *testing.T) {
	t.Parallel()

	today := day.DateOf(t0)
	for _, tc := range []struct {
		name  string
		edit  func(d *proposal.Draft)
		want  string
		wrong error
	}{
		{"no rationale", func(d *proposal.Draft) { d.Rationale = "" }, "rationale is required", store.ErrInvalidProposal},
		{"a window ahead", func(d *proposal.Draft) { d.Window.To = today.AddDays(1) }, "after today", store.ErrInvalidProposal},
		{"nothing", func(d *proposal.Draft) { d.Changes = proposal.Changes{} }, "changes nothing", store.ErrInvalidProposal},
		{"a delete of no phase", func(d *proposal.Draft) {
			d.Changes = proposal.Changes{Phases: []proposal.PhaseChange{{Start: today.AddDays(5)}}}
		}, "no phase from 2026-10-26 to delete", store.ErrInvalidProposal},
		{"a target in the past", func(d *proposal.Draft) {
			d.Changes.Targets[0].From, d.Changes.Targets[0].After.EffectiveFrom = today.AddDays(-1), today.AddDays(-1)
		}, "before today", store.ErrInvalidProposal},
		{"a phase over the current one", func(d *proposal.Draft) {
			d.Changes.Phases[0].After.End = date(t, "2026-12-31")
			d.Changes.Phases = append(d.Changes.Phases, proposal.PhaseChange{Start: date(t, "2026-12-01"), After: new(cut(t, "2026-12-01", "", -0.5))})
		}, "phase 2 starts on 2026-12-01, while phase 1 runs to 2026-12-31", store.ErrInvalidProposal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, ana, _ := withPlans(t)
			pending := propose(t, db, ana, coachDraft(t), t0)
			d := coachDraft(t)
			tc.edit(&d)
			n := countRows(t, db, "events")
			_, err := db.CreateProposal(t.Context(), ana, coachActor, d, today, t0)
			if !errors.Is(err, tc.wrong) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("CreateProposal = %v, want %q", err, tc.want)
			}
			if countRows(t, db, "events") != n {
				t.Error("a refused proposal recorded an event")
			}
			// The pending proposal waits on.
			wantStatus(t, db, ana, pending.ID, t0, proposal.Pending)
		})
	}
}

func TestProposalsNeedTheUser(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	for _, a := range []event.Actor{{Kind: event.User}, {Kind: event.Import, Client: "history"}} {
		if _, err := db.CreateProposal(t.Context(), ana, a, coachDraft(t), day.DateOf(t0), t0); err == nil ||
			!strings.Contains(err.Error(), "invalid actor") {
			t.Errorf("CreateProposal of %+v = %v", a, err)
		}
	}
	p := propose(t, db, ana, coachDraft(t), t0)
	if _, err := db.ApplyProposal(t.Context(), ana, event.Actor{}, p.ID, day.DateOf(t0), t0); err == nil ||
		!strings.Contains(err.Error(), "invalid actor") {
		t.Errorf("ApplyProposal without an actor = %v", err)
	}
	if _, err := db.RejectProposal(t.Context(), ana, event.Actor{}, p.ID, t0); err == nil ||
		!strings.Contains(err.Error(), "invalid actor") {
		t.Errorf("RejectProposal without an actor = %v", err)
	}
}

func TestANewProposalSupersedesThePending(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	first := propose(t, db, ana, coachDraft(t), t0)
	later := t0.Add(time.Hour)
	second := propose(t, db, ana, coachDraft(t), later)

	wantDecided(t, db, ana, first.ID, proposal.Superseded, coachActor, later)
	wantStatus(t, db, ana, second.ID, later, proposal.Pending)
	if got := eventActions(t, db, ana, 2); !reflect.DeepEqual(got, []event.Action{event.ProposalCreate, event.ProposalStatus}) {
		t.Errorf("the events = %v", got)
	}
	// Newest first, up to the limit.
	ps, err := db.Proposals(t.Context(), ana, 10, later)
	if err != nil || len(ps) != 2 || ps[0].ID != second.ID || ps[1].ID != first.ID {
		t.Errorf("Proposals = %+v, %v; want the newest first", ps, err)
	}
	if ps, _ := db.Proposals(t.Context(), ana, 1, later); len(ps) != 1 || ps[0].ID != second.ID {
		t.Errorf("Proposals up to 1 = %+v", ps)
	}
	for _, limit := range []int{0, -1} {
		if ps, err := db.Proposals(t.Context(), ana, limit, later); err != nil || len(ps) != 0 {
			t.Errorf("Proposals up to %d = %+v, %v; want none", limit, ps, err)
		}
	}
}

// Proposals of the statuses asked for: an expired one is stored pending.
func TestProposalsByStatus(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	applied := propose(t, db, ana, coachDraft(t), t0)
	apply(t, db, ana, applied.ID, t0)
	superseded := propose(t, db, ana, proposal.Draft{Rationale: "Rest days lower.", Changes: proposal.Changes{
		TrainingWeekdays: &proposal.WeekdaysChange{After: 1},
	}}, t0)
	pending := propose(t, db, ana, proposal.Draft{Rationale: "Two days.", Changes: proposal.Changes{
		TrainingWeekdays: &proposal.WeekdaysChange{After: 3},
	}}, t0)
	ids := func(ps []proposal.Proposal) []uuid.UUID {
		var out []uuid.UUID
		for i := range ps {
			out = append(out, ps[i].ID)
		}
		return out
	}
	for _, tc := range []struct {
		name     string
		now      time.Time
		statuses []proposal.Status
		want     []uuid.UUID
	}{
		{"all", t0, nil, []uuid.UUID{pending.ID, superseded.ID, applied.ID}},
		{"pending", t0, []proposal.Status{proposal.Pending}, []uuid.UUID{pending.ID}},
		{"the revisions", t0, []proposal.Status{proposal.Applied, proposal.Reverted}, []uuid.UUID{applied.ID}},
		{"pending, expired", t0.Add(proposal.TTL), []proposal.Status{proposal.Pending}, []uuid.UUID{pending.ID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := db.Proposals(t.Context(), ana, 10, tc.now, tc.statuses...)
			if err != nil || !reflect.DeepEqual(ids(got), tc.want) {
				t.Errorf("Proposals = %v, %v; want %v", ids(got), err, tc.want)
			}
		})
	}
}

// An expired proposal stays expired, superseded by nothing.
func TestAnExpiredProposalIsNotSuperseded(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	old := propose(t, db, ana, coachDraft(t), t0)
	later := t0.Add(proposal.TTL)
	propose(t, db, ana, proposal.Draft{Rationale: "Rest days lower.", Changes: proposal.Changes{
		TrainingWeekdays: &proposal.WeekdaysChange{After: 1},
	}}, later)
	p := getProposal(t, db, ana, old.ID, later)
	if p.Status != proposal.Expired || !p.DecidedAt.IsZero() || p.DecidedBy != (event.Actor{}) {
		t.Errorf("the old proposal = %+v, want it expired", p)
	}
}

func TestAChangeOfThePlanSupersedesThePending(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		write func(t *testing.T, db *store.DB, userID uuid.UUID) error
		// stays: the write changes nothing of the plan.
		stays bool
	}{
		{"a phase", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			return db.SetPhase(t.Context(), userID, testActor, cut(t, "2027-01-04", "", -0.5), t0)
		}, false},
		{"a phase deleted", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			return db.DeletePhase(t.Context(), userID, testActor, date(t, "2026-09-07"), t0)
		}, false},
		{"a target", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			_, tg := planValues(t)
			tg.Rest.Kcal = 1950
			return db.SetTarget(t.Context(), userID, testActor, tg, t0)
		}, false},
		{"a target deleted", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			return db.DeleteTarget(t.Context(), userID, testActor, date(t, "2026-09-07"), t0)
		}, false},
		{"the training weekdays", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			return db.SetTrainingWeekdays(t.Context(), userID, testActor, 0b1010101, t0)
		}, false},
		{"the same target", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			_, tg := planValues(t)
			return db.SetTarget(t.Context(), userID, testActor, tg, t0)
		}, true},
		{"the profile", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			return db.SetProfile(t.Context(), userID, testActor, profile.Profile{Sex: profile.Male}, day.DateOf(t0), t0)
		}, true},
		{"the limits", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			l := limits.UserDefaults()
			l.CutRate = -0.5
			return db.SetUserLimits(t.Context(), userID, testActor, l, limits.Instance(), t0)
		}, true},
		{"a day's type", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			return db.SetTrainingDay(t.Context(), userID, testActor, day.DateOf(t0), new(true), t0)
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, ana, ivo := withPlans(t)
			p := propose(t, db, ana, coachDraft(t), t0)
			ivos := propose(t, db, ivo, coachDraft(t), t0)
			if err := tc.write(t, db, ana); err != nil {
				t.Fatal(err)
			}
			if tc.stays {
				wantStatus(t, db, ana, p.ID, t0, proposal.Pending)
			} else {
				wantDecided(t, db, ana, p.ID, proposal.Superseded, testActor, t0)
			}
			wantStatus(t, db, ivo, ivos.ID, t0, proposal.Pending)
		})
	}
}

// breakDraft is coachDraft with a break ahead, and the cut ending before
// it.
func breakDraft(t *testing.T) proposal.Draft {
	t.Helper()
	d := coachDraft(t)
	brk := plan.Phase{Type: plan.Maintain, Start: date(t, "2026-12-21"), End: date(t, "2027-01-03")}
	d.Changes.Phases[0].After.End = brk.Start.AddDays(-1)
	d.Changes.Phases = append([]proposal.PhaseChange{{Start: brk.Start, After: &brk}}, d.Changes.Phases...)
	return d
}

func TestApplyProposal(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	d := breakDraft(t)
	p := propose(t, db, ana, d, t0)
	later := t0.Add(time.Minute)
	applied := apply(t, db, ana, p.ID, later)

	want := p
	want.Status, want.DecidedAt, want.DecidedBy = proposal.Applied, later, testActor
	if !reflect.DeepEqual(applied, want) {
		t.Errorf("ApplyProposal = %+v, want %+v", applied, want)
	}
	if got := getProposal(t, db, ana, p.ID, later); !reflect.DeepEqual(got, want) {
		t.Errorf("Proposal = %+v, want %+v", got, want)
	}
	// The plan is as it leaves it.
	brk, faster := *d.Changes.Phases[0].After, *d.Changes.Phases[1].After
	if got, _ := db.Phases(t.Context(), ana); !reflect.DeepEqual(got, []plan.Phase{faster, brk}) {
		t.Errorf("Phases = %+v, want %+v", got, []plan.Phase{faster, brk})
	}
	_, tg := planValues(t)
	if got, _ := db.Targets(t.Context(), ana); !reflect.DeepEqual(got, []plan.Target{tg, *d.Changes.Targets[0].After}) {
		t.Errorf("Targets = %+v", got)
	}
	if got, _ := db.TrainingWeekdays(t.Context(), ana); got != 0b0101011 {
		t.Errorf("TrainingWeekdays = %#b", got)
	}
	// The target it set names it.
	var from string
	if err := db.Read().QueryRowContext(t.Context(), "SELECT effective_from FROM targets WHERE user_id = ? AND proposal_id = ?",
		ana.String(), p.ID.String()).Scan(&from); err != nil || from != d.Changes.Targets[0].From.String() {
		t.Errorf("the target of the proposal = %q, %v", from, err)
	}
}

func TestApplyProposalRecordsEachChange(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, breakDraft(t), t0)
	n := countRows(t, db, "events")
	apply(t, db, ana, p.ID, t0)
	if got := countRows(t, db, "events"); got != n+5 {
		t.Errorf("%d events, want %d", got, n+5)
	}
	// Its status and each change are events of whoever applied it.
	es, err := db.Events(t.Context(), ana, 5)
	if err != nil {
		t.Fatal(err)
	}
	var actions []event.Action
	for _, e := range es {
		if e.Actor != testActor {
			t.Errorf("the event %+v is not of whoever applied it", e)
		}
		actions = append(actions, e.Action)
	}
	want := []event.Action{event.WeekdaysSet, event.TargetSet, event.PhaseSet, event.PhaseSet, event.ProposalStatus}
	if !reflect.DeepEqual(actions, want) {
		t.Errorf("the events = %v, want %v", actions, want)
	}
	// Undo leaves a proposal and the plan it changed.
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("UndoLast = %v, want ErrNothingToUndo", err)
	}
}

// Applied again, as a retry would, a proposal is as it was.
func TestApplyProposalAgain(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, coachDraft(t), t0)
	applied := apply(t, db, ana, p.ID, t0)
	n := countRows(t, db, "events")
	if again := apply(t, db, ana, p.ID, t0.Add(time.Minute)); !reflect.DeepEqual(again, applied) {
		t.Errorf("ApplyProposal again = %+v, want %+v", again, applied)
	}
	if countRows(t, db, "events") != n {
		t.Error("ApplyProposal again recorded an event")
	}
}

func TestApplyProposalRefusesOneClosed(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	applyAt := func(id uuid.UUID, now time.Time) error {
		_, err := db.ApplyProposal(t.Context(), ana, testActor, id, day.DateOf(now), now)
		return err
	}
	rejected := propose(t, db, ana, coachDraft(t), t0)
	if _, err := db.RejectProposal(t.Context(), ana, testActor, rejected.ID, t0); err != nil {
		t.Fatal(err)
	}
	wantClosed(t, applyAt(rejected.ID, t0), proposal.Rejected)

	superseded := propose(t, db, ana, coachDraft(t), t0)
	propose(t, db, ana, coachDraft(t), t0)
	wantClosed(t, applyAt(superseded.ID, t0), proposal.Superseded)

	expired := propose(t, db, ana, coachDraft(t), t0)
	wantClosed(t, applyAt(expired.ID, t0.Add(proposal.TTL)), proposal.Expired)

	if err := applyAt(uuid.NewV7(), t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ApplyProposal of none = %v, want ErrNotFound", err)
	}
	// None of them changed the plan.
	if got, _ := db.Phases(t.Context(), ana); len(got) != 1 || got[0].RatePctWeek != -0.5 {
		t.Errorf("Phases = %+v, want them as they were", got)
	}
}

// A day on, a target from the day before is in the past: the proposal
// waits, for nothing.
func TestApplyProposalADayLate(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	today := day.DateOf(t0)
	d := coachDraft(t)
	d.Changes.Targets[0].From, d.Changes.Targets[0].After.EffectiveFrom = today, today
	p := propose(t, db, ana, d, t0)
	late := t0.Add(24 * time.Hour)
	if _, err := db.ApplyProposal(t.Context(), ana, testActor, p.ID, day.DateOf(late), late); !errors.Is(err, store.ErrInvalidProposal) ||
		!strings.Contains(err.Error(), "before today") {
		t.Errorf("ApplyProposal a day late = %v", err)
	}
	wantStatus(t, db, ana, p.ID, late, proposal.Pending)
}

// A phase the next one ended as the proposal waited is in the past.
func TestApplyProposalAsTheNextPhaseStarts(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	next := plan.Phase{Type: plan.Maintain, Start: day.DateOf(t0).AddDays(1)}
	if err := db.SetPhase(t.Context(), ana, testActor, next, t0); err != nil {
		t.Fatal(err)
	}
	d := coachDraft(t)
	d.Changes.Targets, d.Changes.TrainingWeekdays = nil, nil
	p := propose(t, db, ana, d, t0)
	late := t0.Add(48 * time.Hour)
	if _, err := db.ApplyProposal(t.Context(), ana, testActor, p.ID, day.DateOf(late), late); !errors.Is(err, store.ErrInvalidProposal) ||
		!strings.Contains(err.Error(), "the phase from 2026-09-07 ended on 2026-10-21") {
		t.Errorf("ApplyProposal after the next phase started = %v", err)
	}
}

// The plan changed under a proposal without a write of the store: it is
// superseded when applied, and changes nothing.
func TestApplyProposalUnderAChangedPlan(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, coachDraft(t), t0)
	exec(t, db, "UPDATE plan_settings SET training_weekdays = 1 WHERE user_id = ?", ana.String())
	_, err := db.ApplyProposal(t.Context(), ana, testActor, p.ID, day.DateOf(t0), t0)
	wantClosed(t, err, proposal.Superseded)
	wantDecided(t, db, ana, p.ID, proposal.Superseded, testActor, t0)
	if got, _ := db.Phases(t.Context(), ana); len(got) != 1 || got[0].RatePctWeek != -0.5 {
		t.Errorf("Phases = %+v, want them as they were", got)
	}
}

func TestRejectProposal(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, coachDraft(t), t0)
	later := t0.Add(time.Minute)
	got, err := db.RejectProposal(t.Context(), ana, testActor, p.ID, later)
	if err != nil {
		t.Fatal(err)
	}
	want := p
	want.Status, want.DecidedAt, want.DecidedBy = proposal.Rejected, later, testActor
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RejectProposal = %+v, want %+v", got, want)
	}
	if e := lastEvent(t, db, ana); e.Action != event.ProposalStatus || e.Entity != p.ID.String() || e.Before == nil || e.After == nil {
		t.Errorf("the event = %+v", e)
	}
	// Rejected again, as a retry would, it is as it was.
	n := countRows(t, db, "events")
	if again, err := db.RejectProposal(t.Context(), ana, testActor, p.ID, later.Add(time.Minute)); err != nil || !reflect.DeepEqual(again, want) {
		t.Errorf("RejectProposal again = %+v, %v", again, err)
	}
	if countRows(t, db, "events") != n {
		t.Error("RejectProposal again recorded an event")
	}
	// The plan is as it was.
	if got, _ := db.TrainingWeekdays(t.Context(), ana); got != 0b0101010 {
		t.Errorf("TrainingWeekdays = %#b", got)
	}
}

func TestRejectProposalRefusesOneClosed(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	applied := propose(t, db, ana, coachDraft(t), t0)
	apply(t, db, ana, applied.ID, t0)
	_, err := db.RejectProposal(t.Context(), ana, testActor, applied.ID, t0)
	wantClosed(t, err, proposal.Applied)

	expired := propose(t, db, ana, proposal.Draft{Rationale: "Rest days lower.", Changes: proposal.Changes{
		TrainingWeekdays: &proposal.WeekdaysChange{After: 1},
	}}, t0)
	_, err = db.RejectProposal(t.Context(), ana, testActor, expired.ID, t0.Add(proposal.TTL))
	wantClosed(t, err, proposal.Expired)
	wantStatus(t, db, ana, expired.ID, t0.Add(proposal.TTL), proposal.Expired)
}

func revertOf(t *testing.T, db *store.DB, userID, id uuid.UUID, now time.Time) proposal.Proposal {
	t.Helper()
	p, err := db.RevertProposal(t.Context(), userID, coachActor, id, "Too hungry.", day.DateOf(now), now)
	if err != nil {
		t.Fatalf("RevertProposal: %v", err)
	}
	return p
}

func TestRevertProposal(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, coachDraft(t), t0)
	applied := apply(t, db, ana, p.ID, t0)
	later := t0.Add(time.Hour)
	revert := revertOf(t, db, ana, p.ID, later)
	// Each change of the proposal, put back.
	if revert.Reverts != p.ID || revert.Rationale != "Too hungry." || !reflect.DeepEqual(revert.Changes, applied.Changes.Inverse()) {
		t.Errorf("the revert = %+v", revert)
	}
	// Until the revert applies, the proposal stays applied.
	wantStatus(t, db, ana, p.ID, later, proposal.Applied)
	apply(t, db, ana, revert.ID, later)

	got := getProposal(t, db, ana, p.ID, later)
	if got.Status != proposal.Reverted || got.RevertedBy != revert.ID || !got.DecidedAt.Equal(t0) {
		t.Errorf("the reverted proposal = %+v", got)
	}
	ph, tg := planValues(t)
	if got, _ := db.Phases(t.Context(), ana); !reflect.DeepEqual(got, []plan.Phase{ph}) {
		t.Errorf("Phases = %+v, want them as before", got)
	}
	if got, _ := db.Targets(t.Context(), ana); !reflect.DeepEqual(got, []plan.Target{tg}) {
		t.Errorf("Targets = %+v, want them as before", got)
	}
	if got, _ := db.TrainingWeekdays(t.Context(), ana); got != 0b0101010 {
		t.Errorf("TrainingWeekdays = %#b, want them as before", got)
	}
	// The revert applied, the proposal reverted, and each change put back.
	want := []event.Action{event.WeekdaysSet, event.TargetDelete, event.PhaseSet, event.ProposalStatus, event.ProposalStatus}
	if got := eventActions(t, db, ana, 5); !reflect.DeepEqual(got, want) {
		t.Errorf("the events = %v, want %v", got, want)
	}
}

func TestRevertProposalRefuses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// revert returns the proposal to revert, and when.
		revert func(t *testing.T, db *store.DB, userID uuid.UUID) (uuid.UUID, time.Time)
		want   string
	}{
		{"a pending proposal", func(t *testing.T, db *store.DB, userID uuid.UUID) (uuid.UUID, time.Time) {
			t.Helper()
			return propose(t, db, userID, coachDraft(t), t0).ID, t0
		}, "no applied proposal"},
		{"no proposal", func(t *testing.T, _ *store.DB, _ uuid.UUID) (uuid.UUID, time.Time) {
			t.Helper()
			return uuid.NewV7(), t0
		}, "no applied proposal"},
		{"a reverted proposal", func(t *testing.T, db *store.DB, userID uuid.UUID) (uuid.UUID, time.Time) {
			t.Helper()
			p := propose(t, db, userID, coachDraft(t), t0)
			apply(t, db, userID, p.ID, t0)
			apply(t, db, userID, revertOf(t, db, userID, p.ID, t0).ID, t0)
			return p.ID, t0
		}, "no applied proposal"},
		{"a target changed since", func(t *testing.T, db *store.DB, userID uuid.UUID) (uuid.UUID, time.Time) {
			t.Helper()
			p := propose(t, db, userID, coachDraft(t), t0)
			applied := apply(t, db, userID, p.ID, t0)
			tg := *applied.Changes.Targets[0].After
			tg.Rest.Kcal = 1800
			if err := db.SetTarget(t.Context(), userID, testActor, tg, t0); err != nil {
				t.Fatal(err)
			}
			return p.ID, t0
		}, "the target from 2026-10-22 changed since the proposal"},
		{"a target that took effect", func(t *testing.T, db *store.DB, userID uuid.UUID) (uuid.UUID, time.Time) {
			t.Helper()
			p := propose(t, db, userID, coachDraft(t), t0)
			apply(t, db, userID, p.ID, t0)
			return p.ID, t0.Add(48 * time.Hour)
		}, "the target from 2026-10-22, before today 2026-10-23"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, ana, _ := withPlans(t)
			id, now := tc.revert(t, db, ana)
			n := countRows(t, db, "events")
			_, err := db.RevertProposal(t.Context(), ana, coachActor, id, "Back.", day.DateOf(now), now)
			if !errors.Is(err, store.ErrInvalidProposal) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("RevertProposal = %v, want %q", err, tc.want)
			}
			if countRows(t, db, "events") != n {
				t.Error("a refused revert recorded an event")
			}
		})
	}
}

// A user's proposals go with the user, a revert and the one it took
// back too.
func TestProposalsGoWithTheUser(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withPlans(t)
	p := propose(t, db, ana, coachDraft(t), t0)
	apply(t, db, ana, p.ID, t0)
	apply(t, db, ana, revertOf(t, db, ana, p.ID, t0).ID, t0)
	propose(t, db, ivo, coachDraft(t), t0)
	exec(t, db, "DELETE FROM users WHERE id = ?", ana.String())
	if n := countRows(t, db, "proposals"); n != 1 {
		t.Errorf("%d proposals, want Ivo's alone", n)
	}
}

func TestProposalStoreFailures(t *testing.T) {
	t.Parallel()

	db, ana, _ := withPlans(t)
	p := propose(t, db, ana, coachDraft(t), t0)
	exec(t, db, "UPDATE proposals SET changes = '{\"phases\": 1}' WHERE user_id = ?", ana.String())
	if _, err := db.Proposal(t.Context(), ana, p.ID, t0); err == nil {
		t.Error("Proposal of changes that do not decode = nil")
	}
	if _, err := db.Proposals(t.Context(), ana, 10, t0); err == nil {
		t.Error("Proposals of changes that do not decode = nil")
	}
	if _, err := db.ApplyProposal(t.Context(), ana, testActor, p.ID, day.DateOf(t0), t0); err == nil {
		t.Error("ApplyProposal of changes that do not decode = nil")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateProposal(t.Context(), ana, coachActor, coachDraft(t), day.DateOf(t0), t0); err == nil {
		t.Error("CreateProposal on a closed database = nil")
	}
	if _, err := db.RejectProposal(t.Context(), ana, testActor, p.ID, t0); err == nil {
		t.Error("RejectProposal on a closed database = nil")
	}
}
