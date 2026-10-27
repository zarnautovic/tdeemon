package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
)

// ErrInvalidProposal reports a proposal that breaks a rule of its own (a
// rationale, a window, a change of the past, a change of nothing), one
// whose timeline overlaps, or the revert of a proposal that is not
// applied.
var ErrInvalidProposal = errors.New("invalid proposal")

// ProposalCheck checks a proposal's changes, with the plan before each,
// against the limits (proposal.Changes.Check). It runs in the
// transaction once the changes keep their own rules, before anything is
// written: its error is returned as it is, and nothing is written. The
// transaction holds the one write connection, so what the check reads
// through the store's reads (the limits, the profile, the data) no write
// changes until it ends. nil checks nothing.
type ProposalCheck func(proposal.Changes) error

// ProposalClosedError reports a proposal that waits no more: Status says
// what became of it.
type ProposalClosedError struct {
	Status proposal.Status
}

func (e *ProposalClosedError) Error() string { return fmt.Sprintf("the proposal is %s", e.Status) }

// The states of a proposal have JSON names of their own, as the plan's
// do: the column of its changes holds them, and its events record them.

// phaseChangeState is a change of the phase that starts on Start.
type phaseChangeState struct {
	Start  string      `json:"start"`
	Before *phaseState `json:"before"`
	After  *phaseState `json:"after"`
}

// targetValues are a target's values, without the proposal that set it.
type targetValues struct {
	Training dayTargetState `json:"training"`
	Rest     dayTargetState `json:"rest"`
}

// targetChangeState is a change of the target from day From.
type targetChangeState struct {
	From   string        `json:"from"`
	Before *targetValues `json:"before"`
	After  *targetValues `json:"after"`
}

// weekdaysChangeState is a change of the training weekdays.
type weekdaysChangeState struct {
	Before int `json:"before"`
	After  int `json:"after"`
}

// limitChangeState is a change of the user's limit Name.
type limitChangeState struct {
	Name   string  `json:"name"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
}

// changesState is what a proposal changes. A proposal made before the
// limits could be proposed has no "limits".
type changesState struct {
	Phases           []phaseChangeState   `json:"phases"`
	Targets          []targetChangeState  `json:"targets"`
	TrainingWeekdays *weekdaysChangeState `json:"training_weekdays"`
	Limits           []limitChangeState   `json:"limits"`
}

func changesStateOf(c proposal.Changes) changesState {
	phase := func(p *plan.Phase) *phaseState {
		if p == nil {
			return nil
		}
		return new(phaseStateOf(*p))
	}
	target := func(t *plan.Target) *targetValues {
		if t == nil {
			return nil
		}
		return &targetValues{Training: dayTargetStateOf(t.Training), Rest: dayTargetStateOf(t.Rest)}
	}
	s := changesState{Phases: []phaseChangeState{}, Targets: []targetChangeState{}, Limits: []limitChangeState{}}
	for _, ch := range c.Phases {
		s.Phases = append(s.Phases, phaseChangeState{Start: ch.Start.String(), Before: phase(ch.Before), After: phase(ch.After)})
	}
	for _, ch := range c.Targets {
		s.Targets = append(s.Targets, targetChangeState{From: ch.From.String(), Before: target(ch.Before), After: target(ch.After)})
	}
	if w := c.TrainingWeekdays; w != nil {
		s.TrainingWeekdays = &weekdaysChangeState{Before: int(w.Before), After: int(w.After)}
	}
	for _, ch := range c.Limits {
		s.Limits = append(s.Limits, limitChangeState{Name: ch.Name, Before: ch.Before, After: ch.After})
	}
	return s
}

func (s changesState) changes() (proposal.Changes, error) {
	var c proposal.Changes
	for _, ch := range s.Phases {
		start, err := day.ParseDate(ch.Start)
		if err != nil {
			return proposal.Changes{}, err
		}
		phase := func(ps *phaseState) (*plan.Phase, error) {
			if ps == nil {
				return nil, nil
			}
			p, err := ps.phase(start)
			return &p, err
		}
		pc := proposal.PhaseChange{Start: start}
		if pc.Before, err = phase(ch.Before); err != nil {
			return proposal.Changes{}, err
		}
		if pc.After, err = phase(ch.After); err != nil {
			return proposal.Changes{}, err
		}
		c.Phases = append(c.Phases, pc)
	}
	for _, ch := range s.Targets {
		from, err := day.ParseDate(ch.From)
		if err != nil {
			return proposal.Changes{}, err
		}
		target := func(v *targetValues) *plan.Target {
			if v == nil {
				return nil
			}
			return new(targetState{Training: v.Training, Rest: v.Rest}.target(from))
		}
		c.Targets = append(c.Targets, proposal.TargetChange{From: from, Before: target(ch.Before), After: target(ch.After)})
	}
	if w := s.TrainingWeekdays; w != nil {
		c.TrainingWeekdays = &proposal.WeekdaysChange{
			Before: plan.Weekdays(w.Before), //nolint:gosec // G115: a check of the shape refuses what is no set of weekdays
			After:  plan.Weekdays(w.After),  //nolint:gosec // G115: as above
		}
	}
	for _, ch := range s.Limits {
		c.Limits = append(c.Limits, proposal.LimitChange{Name: ch.Name, Before: ch.Before, After: ch.After})
	}
	return c, nil
}

// proposalState is a proposal's row, and the state its events record.
type proposalState struct {
	Status        string          `json:"status"`
	Client        string          `json:"client"`
	Model         string          `json:"model"`
	Rationale     string          `json:"rationale"`
	WindowFrom    *string         `json:"window_from"`
	WindowTo      *string         `json:"window_to"`
	Changes       json.RawMessage `json:"changes"`
	Reverts       *string         `json:"reverts"`
	CreatedAt     string          `json:"created_at"`
	ExpiresAt     string          `json:"expires_at"`
	DecidedAt     *string         `json:"decided_at"`
	DecidedActor  *string         `json:"decided_actor"`
	DecidedClient *string         `json:"decided_client"`
	DecidedModel  *string         `json:"decided_model"`
	RevertedBy    *string         `json:"reverted_by"`
}

const proposalColumns = `id, status, client, model, rationale, window_from, window_to, changes, reverts,
	created_at, expires_at, decided_at, decided_actor, decided_client, decided_model, reverted_by`

func scanProposal(row interface{ Scan(...any) error }) (string, proposalState, error) {
	var id, changes string
	var s proposalState
	if err := row.Scan(&id, &s.Status, &s.Client, &s.Model, &s.Rationale, &s.WindowFrom, &s.WindowTo, &changes, &s.Reverts,
		&s.CreatedAt, &s.ExpiresAt, &s.DecidedAt, &s.DecidedActor, &s.DecidedClient, &s.DecidedModel, &s.RevertedBy); err != nil {
		return "", proposalState{}, err
	}
	s.Changes = json.RawMessage(changes)
	return id, s, nil
}

// proposal returns s as the proposal id, with its status at now
// (proposal.StatusAt).
func (s proposalState) proposal(id string, now time.Time) (proposal.Proposal, error) {
	p := proposal.Proposal{
		Status: proposal.Status(s.Status), Rationale: s.Rationale,
		By: event.Actor{Kind: event.User, Client: s.Client, Model: s.Model},
	}
	var err error
	if p.ID, err = uuid.Parse(id); err != nil {
		return proposal.Proposal{}, fmt.Errorf("proposal id %q: %w", id, err)
	}
	if p.Window, err = windowOf(s.WindowFrom, s.WindowTo); err != nil {
		return proposal.Proposal{}, err
	}
	var cs changesState
	if err := json.Unmarshal(s.Changes, &cs); err != nil {
		return proposal.Proposal{}, fmt.Errorf("the changes of proposal %s: %w", id, err)
	}
	if p.Changes, err = cs.changes(); err != nil {
		return proposal.Proposal{}, fmt.Errorf("the changes of proposal %s: %w", id, err)
	}
	if p.Reverts, err = proposalRef(s.Reverts); err != nil {
		return proposal.Proposal{}, err
	}
	if p.RevertedBy, err = proposalRef(s.RevertedBy); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.decided(&p); err != nil {
		return proposal.Proposal{}, err
	}
	p.Status = p.StatusAt(now)
	return p, nil
}

// decided sets p's times, and who decided it.
func (s proposalState) decided(p *proposal.Proposal) error {
	var err error
	if p.CreatedAt, err = parseTime(s.CreatedAt); err != nil {
		return err
	}
	if p.ExpiresAt, err = parseTime(s.ExpiresAt); err != nil {
		return err
	}
	if s.DecidedAt == nil {
		return nil
	}
	if p.DecidedAt, err = parseTime(*s.DecidedAt); err != nil {
		return err
	}
	if s.DecidedActor != nil && s.DecidedClient != nil && s.DecidedModel != nil {
		p.DecidedBy = event.Actor{Kind: event.ActorKind(*s.DecidedActor), Client: *s.DecidedClient, Model: *s.DecidedModel}
	}
	return nil
}

// windowOf returns the window of the days from and to, nil for none.
func windowOf(from, to *string) (*proposal.Window, error) {
	if from == nil || to == nil {
		return nil, nil
	}
	var w proposal.Window
	var err error
	if w.From, err = day.ParseDate(*from); err != nil {
		return nil, err
	}
	if w.To, err = day.ParseDate(*to); err != nil {
		return nil, err
	}
	return &w, nil
}

// proposalRef returns the proposal id s names, uuid.Nil for none.
func proposalRef(s *string) (uuid.UUID, error) {
	if s == nil {
		return uuid.Nil(), nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("a proposal %q: %w", *s, err)
	}
	return id, nil
}

// proposalOf returns the state of userID's proposal id, nil for none.
func proposalOf(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID) (*proposalState, error) {
	_, s, err := scanProposal(tx.QueryRowContext(ctx, "SELECT "+proposalColumns+" FROM proposals WHERE user_id = ? AND id = ?",
		userID.String(), id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// readProposal returns userID's proposal id at now, or ErrNotFound.
func readProposal(ctx context.Context, tx *sql.Tx, userID, id uuid.UUID, now time.Time) (proposal.Proposal, error) {
	s, err := proposalOf(ctx, tx, userID, id)
	if err != nil {
		return proposal.Proposal{}, err
	}
	if s == nil {
		return proposal.Proposal{}, ErrNotFound
	}
	return s.proposal(id.String(), now)
}

// Proposal returns userID's proposal id, with its status at now (an
// expired one is pending no more); ErrNotFound when there is none.
func (db *DB) Proposal(ctx context.Context, userID, id uuid.UUID, now time.Time) (proposal.Proposal, error) {
	var p proposal.Proposal
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		p, err = readProposal(ctx, tx, userID, id, now)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return proposal.Proposal{}, err
	}
	if err != nil {
		return proposal.Proposal{}, fmt.Errorf("proposal: %w", err)
	}
	return p, nil
}

// Proposals returns up to limit of userID's latest proposals, newest
// first, each with its status at now: what waits, and the plan's
// revisions; none for a limit of 0 or less. Given statuses, it returns
// those stored in one of them alone: an expired proposal is stored
// pending, and comes back expired.
func (db *DB) Proposals(ctx context.Context, userID uuid.UUID, limit int, now time.Time, statuses ...proposal.Status) ([]proposal.Proposal, error) {
	out := []proposal.Proposal{}
	if limit <= 0 {
		return out, nil
	}
	query, args := "SELECT "+proposalColumns+" FROM proposals WHERE user_id = ?", []any{userID.String()}
	if len(statuses) > 0 {
		query += " AND status IN (?" + strings.Repeat(", ?", len(statuses)-1) + ")"
		for _, st := range statuses {
			args = append(args, string(st))
		}
	}
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query+" ORDER BY rowid DESC LIMIT ?", append(args, limit)...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			id, s, err := scanProposal(rows)
			if err != nil {
				return err
			}
			p, err := s.proposal(id, now)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("proposals: %w", err)
	}
	return out, nil
}

// CreateProposal makes d userID's pending proposal, a's, today being the
// user's day now, and returns it. Each change gets the plan as it is
// before it (Before); the changes must keep their rules
// (proposal.Changes.Validate) and leave a timeline without an overlap,
// or ErrInvalidProposal; check then holds them to the limits. The
// proposal supersedes the user's pending ones: what they showed is what
// it changes now. It waits proposal.TTL for the user's consent.
func (db *DB) CreateProposal(ctx context.Context, userID uuid.UUID, a event.Actor, d proposal.Draft, today day.Date, now time.Time,
	check ProposalCheck,
) (proposal.Proposal, error) {
	if err := checkDraft(a, d, today); err != nil {
		return proposal.Proposal{}, err
	}
	var p proposal.Proposal
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		c, err := planBefore(ctx, tx, userID, d.Changes)
		if err != nil {
			return err
		}
		p, err = createProposalIn(ctx, tx, userID, a, d, c, uuid.Nil(), today, now, check)
		return err
	})
	if err != nil {
		return proposal.Proposal{}, proposalError("create proposal", err)
	}
	return p, nil
}

// RevertProposal makes the proposal that takes userID's applied proposal
// id back, a's, with why, today being the user's day now, and returns
// it, pending as any other: each change of id put back as it was before
// (proposal.Changes.Inverse); ErrNotFound when there is none. It needs an
// applied proposal, the plan it
// changed as it left it (a later change would be lost), and changes that
// keep the past as it was (a change that took effect is moved on by a
// proposal of its own), or ErrInvalidProposal. check holds the inverse to
// the limits. Applied, it marks id reverted.
func (db *DB) RevertProposal(ctx context.Context, userID uuid.UUID, a event.Actor, id uuid.UUID, rationale string, today day.Date, now time.Time,
	check ProposalCheck,
) (proposal.Proposal, error) {
	d := proposal.Draft{Rationale: rationale}
	if err := checkDraft(a, d, today); err != nil {
		return proposal.Proposal{}, err
	}
	var p proposal.Proposal
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		s, err := proposalOf(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		if s == nil {
			return ErrNotFound
		}
		if proposal.Status(s.Status) != proposal.Applied {
			return fmt.Errorf("%w: the proposal is %s, not applied", ErrInvalidProposal, s.Status)
		}
		applied, err := s.proposal(id.String(), now)
		if err != nil {
			return err
		}
		back := applied.Changes.Inverse()
		c, err := planBefore(ctx, tx, userID, back)
		if err != nil {
			return err
		}
		if err := changedSince(c, back); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidProposal, err)
		}
		p, err = createProposalIn(ctx, tx, userID, a, d, c, id, today, now, check)
		return err
	})
	if err != nil {
		return proposal.Proposal{}, proposalError("revert proposal", err)
	}
	return p, nil
}

// checkDraft reports an actor that is not the user, or a rationale or a
// window of d that breaks its rules (ErrInvalidProposal).
func checkDraft(a event.Actor, d proposal.Draft, today day.Date) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	if a.Kind != event.User {
		return fmt.Errorf("invalid actor: a proposal is the user's, not %s's", a.Kind)
	}
	if err := proposal.ValidateRationale(d.Rationale); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidProposal, err)
	}
	if d.Window != nil {
		if err := d.Window.Validate(today); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidProposal, err)
		}
	}
	return nil
}

// changedSince reports the first entity whose state now (c's Before) is
// not the state the revert back takes it from: the plan changed since the
// proposal it reverts.
func changedSince(c, back proposal.Changes) error {
	for i, ch := range c.Phases {
		if !reflect.DeepEqual(ch.Before, back.Phases[i].Before) {
			return fmt.Errorf("the phase from %s changed since the proposal", ch.Start)
		}
	}
	for i, ch := range c.Targets {
		if !reflect.DeepEqual(ch.Before, back.Targets[i].Before) {
			return fmt.Errorf("the target from %s changed since the proposal", ch.From)
		}
	}
	if w := c.TrainingWeekdays; w != nil && w.Before != back.TrainingWeekdays.Before {
		return errors.New("the training weekdays changed since the proposal")
	}
	for i, ch := range c.Limits {
		if ch.Before != back.Limits[i].Before {
			return fmt.Errorf("the limit %s changed since the proposal", ch.Name)
		}
	}
	return nil
}

// createProposalIn stores d as userID's new pending proposal in tx, a's,
// with c, its changes with the plan before each, and reverts, the
// proposal it takes back or uuid.Nil, and returns it; ErrInvalidProposal
// for changes that break their rules or leave a timeline that overlaps,
// or check's error. It supersedes the user's pending proposals.
func createProposalIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, d proposal.Draft, c proposal.Changes,
	reverts uuid.UUID, today day.Date, now time.Time, check ProposalCheck,
) (proposal.Proposal, error) {
	phases, err := phasesIn(ctx, tx, userID)
	if err != nil {
		return proposal.Proposal{}, err
	}
	user, err := limitsOf(ctx, tx, userID)
	if err != nil {
		return proposal.Proposal{}, err
	}
	if err := c.Validate(today, phases, user.limits()); err != nil {
		return proposal.Proposal{}, fmt.Errorf("%w: %w", ErrInvalidProposal, err)
	}
	if err := plan.ValidateTimeline(c.Timeline(phases)); err != nil {
		return proposal.Proposal{}, fmt.Errorf("%w: the phases it leaves: %w", ErrInvalidProposal, err)
	}
	if check != nil {
		if err := check(c); err != nil {
			return proposal.Proposal{}, err
		}
	}
	if err := supersedeProposals(ctx, tx, userID, a, now); err != nil {
		return proposal.Proposal{}, err
	}
	changes, err := json.Marshal(changesStateOf(c))
	if err != nil {
		return proposal.Proposal{}, err
	}
	id := uuid.NewV7()
	var from, to, revertsID any
	if w := d.Window; w != nil {
		from, to = w.From.String(), w.To.String()
	}
	if reverts != uuid.Nil() {
		revertsID = reverts.String()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO proposals (user_id, id, status, client, model, rationale,
		window_from, window_to, changes, reverts, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID.String(), id.String(), string(proposal.Pending), a.Client, a.Model, d.Rationale,
		from, to, string(changes), revertsID, formatTime(now), formatTime(now.Add(proposal.TTL))); err != nil {
		return proposal.Proposal{}, err
	}
	after, err := proposalOf(ctx, tx, userID, id)
	if err != nil {
		return proposal.Proposal{}, err
	}
	if _, err := record(ctx, tx, userID, a, event.ProposalCreate, id.String(), nil, after, now); err != nil {
		return proposal.Proposal{}, err
	}
	return after.proposal(id.String(), now)
}

// planBefore returns c, in the order of each change's day and of the
// limits (limits.Fields), with the plan as it is before each change:
// userID's phase, target, training weekdays and limits in tx.
func planBefore(ctx context.Context, tx *sql.Tx, userID uuid.UUID, c proposal.Changes) (proposal.Changes, error) {
	var out proposal.Changes
	for _, ch := range c.Phases {
		s, err := phaseOf(ctx, tx, userID, ch.Start)
		if err != nil {
			return proposal.Changes{}, err
		}
		pc := proposal.PhaseChange{Start: ch.Start}
		if s != nil {
			p, err := s.phase(ch.Start)
			if err != nil {
				return proposal.Changes{}, err
			}
			pc.Before = &p
		}
		if ch.After != nil {
			pc.After = new(*ch.After)
		}
		out.Phases = append(out.Phases, pc)
	}
	slices.SortStableFunc(out.Phases, func(a, b proposal.PhaseChange) int { return a.Start.Compare(b.Start) })
	for _, ch := range c.Targets {
		s, err := targetOf(ctx, tx, userID, ch.From)
		if err != nil {
			return proposal.Changes{}, err
		}
		tc := proposal.TargetChange{From: ch.From}
		if s != nil {
			tc.Before = new(s.target(ch.From))
		}
		if ch.After != nil {
			tc.After = new(*ch.After)
		}
		out.Targets = append(out.Targets, tc)
	}
	slices.SortStableFunc(out.Targets, func(a, b proposal.TargetChange) int { return a.From.Compare(b.From) })
	if w := c.TrainingWeekdays; w != nil {
		s, err := weekdaysOf(ctx, tx, userID)
		if err != nil {
			return proposal.Changes{}, err
		}
		out.TrainingWeekdays = &proposal.WeekdaysChange{
			Before: plan.Weekdays(s.TrainingWeekdays), //nolint:gosec // G115: the table holds 0 to 127
			After:  w.After,
		}
	}
	if len(c.Limits) > 0 {
		s, err := limitsOf(ctx, tx, userID)
		if err != nil {
			return proposal.Changes{}, err
		}
		out.Limits = limitsBefore(c.Limits, s.limits())
	}
	return out, nil
}

// limitsBefore returns changes, in the order of the limits, each with the
// limit as it is in user before it; a name of no limit, which Validate
// refuses, comes last, before 0.
func limitsBefore(changes []proposal.LimitChange, user limits.Limits) []proposal.LimitChange {
	order := map[string]int{}
	for i, f := range limits.Fields() {
		order[f.Name] = i + 1
	}
	rank := func(name string) int {
		if i, ok := order[name]; ok {
			return i
		}
		return len(order) + 1
	}
	out := make([]proposal.LimitChange, 0, len(changes))
	for _, ch := range changes {
		lc := proposal.LimitChange{Name: ch.Name, After: ch.After}
		if f, ok := limits.FieldNamed(ch.Name); ok {
			lc.Before = f.Get(user)
		}
		out = append(out, lc)
	}
	slices.SortStableFunc(out, func(a, b proposal.LimitChange) int { return rank(a.Name) - rank(b.Name) })
	return out
}

// RejectProposal rejects userID's pending proposal id, as a's decision,
// and returns it; a rejected one again is returned as it is, as a retry
// would ask. ErrNotFound when there is none, and a *ProposalClosedError
// when it waits no more.
func (db *DB) RejectProposal(ctx context.Context, userID uuid.UUID, a event.Actor, id uuid.UUID, now time.Time) (proposal.Proposal, error) {
	if err := a.Validate(); err != nil {
		return proposal.Proposal{}, fmt.Errorf("invalid actor: %w", err)
	}
	var p proposal.Proposal
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		if p, err = readProposal(ctx, tx, userID, id, now); err != nil {
			return err
		}
		switch p.Status {
		case proposal.Rejected:
			return nil
		case proposal.Pending:
		default:
			return &ProposalClosedError{Status: p.Status}
		}
		if err := closeProposal(ctx, tx, userID, a, id, proposal.Rejected, now); err != nil {
			return err
		}
		p, err = readProposal(ctx, tx, userID, id, now)
		return err
	})
	if err != nil {
		return proposal.Proposal{}, proposalError("reject proposal", err)
	}
	return p, nil
}

// ApplyProposal applies userID's pending proposal id, as a's consent,
// today being the user's day now, and returns it: each change is written
// as a's change of the plan, a target names the proposal that set it, and
// a revert marks the proposal it takes back reverted. An applied one
// again is returned as it is, as a retry would ask. ErrNotFound when
// there is none; a *ProposalClosedError when it waits no more, or the
// plan it changes is no longer as it was (the proposal is superseded);
// ErrInvalidProposal when a change would now change the past (a day
// went by, or the next phase started). check holds the changes to the
// limits again, as the data or the limits may have changed since.
func (db *DB) ApplyProposal(ctx context.Context, userID uuid.UUID, a event.Actor, id uuid.UUID, today day.Date, now time.Time,
	check ProposalCheck,
) (proposal.Proposal, error) {
	if err := a.Validate(); err != nil {
		return proposal.Proposal{}, fmt.Errorf("invalid actor: %w", err)
	}
	var p proposal.Proposal
	var changed bool
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		if p, err = readProposal(ctx, tx, userID, id, now); err != nil {
			return err
		}
		switch p.Status {
		case proposal.Applied:
			return nil
		case proposal.Pending:
		default:
			return &ProposalClosedError{Status: p.Status}
		}
		if changed, err = changedUnder(ctx, tx, userID, p, today); err != nil || changed {
			if changed {
				err = closeProposal(ctx, tx, userID, a, id, proposal.Superseded, now)
			}
			return err
		}
		if p, err = applyIn(ctx, tx, userID, a, p, now, check); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return proposal.Proposal{}, proposalError("apply proposal", err)
	}
	if changed {
		return proposal.Proposal{}, &ProposalClosedError{Status: proposal.Superseded}
	}
	return p, nil
}

// applyIn applies userID's pending proposal p in tx, as a's consent,
// once check passes its changes, and returns it as it then is.
func applyIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, p proposal.Proposal, now time.Time, check ProposalCheck) (proposal.Proposal, error) {
	if check != nil {
		if err := check(p.Changes); err != nil {
			return proposal.Proposal{}, err
		}
	}
	if err := closeProposal(ctx, tx, userID, a, p.ID, proposal.Applied, now); err != nil {
		return proposal.Proposal{}, err
	}
	if p.Reverts != uuid.Nil() {
		if err := revertProposal(ctx, tx, userID, a, p.Reverts, p.ID, now); err != nil {
			return proposal.Proposal{}, err
		}
	}
	if err := applyChanges(ctx, tx, userID, a, p.ID, p.Changes, now); err != nil {
		return proposal.Proposal{}, err
	}
	return readProposal(ctx, tx, userID, p.ID, now)
}

// changedUnder reports whether userID's plan in tx is no longer as p
// found it: a write of the plan supersedes a proposal, and one the store
// did not make, as an import of the database could, is found here. It
// returns ErrInvalidProposal for changes that would now change the past
// (a day went by, or the next phase started).
func changedUnder(ctx context.Context, tx *sql.Tx, userID uuid.UUID, p proposal.Proposal, today day.Date) (bool, error) {
	fresh, err := planBefore(ctx, tx, userID, p.Changes)
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(fresh, p.Changes) {
		return true, nil
	}
	phases, err := phasesIn(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	user, err := limitsOf(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	if err := p.Changes.Validate(today, phases, user.limits()); err != nil {
		return false, fmt.Errorf("%w: %w", ErrInvalidProposal, err)
	}
	return false, nil
}

// applyChanges writes c, the changes of the proposal id, as a's changes
// of the plan in tx, and checks the timeline they leave.
func applyChanges(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, id uuid.UUID, c proposal.Changes, now time.Time) error {
	for _, ch := range c.Phases {
		var err error
		if ch.After == nil {
			err = deletePhaseIn(ctx, tx, userID, a, ch.Start, now)
		} else {
			err = setPhaseIn(ctx, tx, userID, a, *ch.After, now)
		}
		if err != nil {
			return err
		}
	}
	for _, ch := range c.Targets {
		var err error
		if ch.After == nil {
			err = deleteTargetIn(ctx, tx, userID, a, ch.From, now)
		} else {
			err = setTargetIn(ctx, tx, userID, a, *ch.After, new(id.String()), now)
		}
		if err != nil {
			return err
		}
	}
	if w := c.TrainingWeekdays; w != nil {
		if err := setWeekdaysIn(ctx, tx, userID, a, w.After, now); err != nil {
			return err
		}
	}
	if len(c.Limits) > 0 {
		before, err := limitsOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if err := setLimitsIn(ctx, tx, userID, a, before, c.LimitsOf(before.limits()), now); err != nil {
			return err
		}
	}
	err := checkTimeline(ctx, tx, userID)
	if errors.Is(err, ErrPhaseOverlap) {
		return fmt.Errorf("%w: %w", ErrInvalidProposal, err)
	}
	return err
}

// proposalError wraps err of what was being done, but for the errors a
// caller tells apart.
func proposalError(doing string, err error) error {
	var closed *ProposalClosedError
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidProposal) || errors.As(err, &closed) {
		return err
	}
	return fmt.Errorf("%s: %w", doing, err)
}

// closeProposal sets userID's pending proposal id to status, decided by
// a at now, as a's event.
func closeProposal(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, id uuid.UUID, status proposal.Status, now time.Time) error {
	return setProposal(ctx, tx, userID, a, id, now, `UPDATE proposals SET status = ?, decided_at = ?, decided_actor = ?,
		decided_client = ?, decided_model = ? WHERE user_id = ? AND id = ?`,
		string(status), formatTime(now), string(a.Kind), a.Client, a.Model, userID.String(), id.String())
}

// revertProposal marks userID's applied proposal id reverted by the
// proposal by, as a's event.
func revertProposal(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, id, by uuid.UUID, now time.Time) error {
	s, err := proposalOf(ctx, tx, userID, id)
	if err != nil {
		return err
	}
	if s == nil || proposal.Status(s.Status) != proposal.Applied {
		return fmt.Errorf("%w: no applied proposal %s to revert", ErrInvalidProposal, id)
	}
	return setProposal(ctx, tx, userID, a, id, now, "UPDATE proposals SET status = ?, reverted_by = ? WHERE user_id = ? AND id = ?",
		string(proposal.Reverted), by.String(), userID.String(), id.String())
}

// setProposal runs query, an update of userID's proposal id, with args,
// and records it as a's event.
func setProposal(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, id uuid.UUID, now time.Time, query string, args ...any) error {
	before, err := proposalOf(ctx, tx, userID, id)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	after, err := proposalOf(ctx, tx, userID, id)
	if err != nil {
		return err
	}
	_, err = record(ctx, tx, userID, a, event.ProposalStatus, id.String(), before, after, now)
	return err
}

// supersedeProposals marks userID's pending proposals superseded by a at
// now, but for the expired ones, which are expired already.
func supersedeProposals(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, now time.Time) error {
	ids, err := pendingProposals(ctx, tx, userID, now)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := closeProposal(ctx, tx, userID, a, id, proposal.Superseded, now); err != nil {
			return err
		}
	}
	return nil
}

// supersedeLimitsProposals marks userID's pending proposals that change
// the limits superseded by a at now: the limits before them hold no more.
func supersedeLimitsProposals(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, now time.Time) error {
	ids, err := pendingProposals(ctx, tx, userID, now)
	if err != nil {
		return err
	}
	for _, id := range ids {
		s, err := proposalOf(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		var cs changesState
		if err := json.Unmarshal(s.Changes, &cs); err != nil {
			return fmt.Errorf("the changes of proposal %s: %w", id, err)
		}
		if len(cs.Limits) == 0 {
			continue
		}
		if err := closeProposal(ctx, tx, userID, a, id, proposal.Superseded, now); err != nil {
			return err
		}
	}
	return nil
}

// pendingProposals returns the ids of userID's proposals that wait at
// now.
func pendingProposals(ctx context.Context, tx *sql.Tx, userID uuid.UUID, now time.Time) ([]uuid.UUID, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM proposals WHERE user_id = ? AND status = ? AND expires_at > ?",
		userID.String(), string(proposal.Pending), formatTime(now))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []uuid.UUID
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("proposal id %q: %w", s, err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// recordPlan records a's change of the plan, as record does, and when it
// changed something, supersedes userID's pending proposals: the plan they
// showed holds no more.
func recordPlan(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, action event.Action, entity string,
	before, after any, now time.Time,
) error {
	id, err := record(ctx, tx, userID, a, action, entity, before, after, now)
	if err != nil || id == uuid.Nil() {
		return err
	}
	return supersedeProposals(ctx, tx, userID, a, now)
}
