package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

var (
	// ErrPhaseOverlap reports a phase that would start on or before the
	// start or the planned end of another (plan.ValidateTimeline).
	ErrPhaseOverlap = errors.New("the phase overlaps another")
	// ErrLimitsBeyond reports a user's limit looser than the instance's.
	ErrLimitsBeyond = errors.New("a limit is looser than the instance's")
)

// The entities of the plan's events that name no day.
const (
	profileEntity  = "profile"
	limitsEntity   = "limits"
	weekdaysEntity = "plan"
	notesEntity    = "notes"
)

// The states the plan's events record have JSON names of their own, so a
// field added later reads the states recorded before it as they were.

// profileState is a profile event's state: the profile's columns, nil
// where they are not given.
type profileState struct {
	Sex       *string  `json:"sex"`
	BirthDate *string  `json:"birth_date"`
	HeightCm  *float64 `json:"height_cm"`
}

func profileStateOf(p profile.Profile) profileState {
	var s profileState
	if p.Sex != "" {
		s.Sex = new(string(p.Sex))
	}
	if !p.BirthDate.IsZero() {
		s.BirthDate = new(p.BirthDate.String())
	}
	if p.HeightCm != 0 {
		s.HeightCm = new(p.HeightCm)
	}
	return s
}

func (s profileState) profile() (profile.Profile, error) {
	var p profile.Profile
	if s.Sex != nil {
		p.Sex = profile.Sex(*s.Sex)
	}
	if s.BirthDate != nil {
		d, err := day.ParseDate(*s.BirthDate)
		if err != nil {
			return profile.Profile{}, err
		}
		p.BirthDate = d
	}
	if s.HeightCm != nil {
		p.HeightCm = *s.HeightCm
	}
	return p, nil
}

func profileOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (profileState, error) {
	var sex, birth sql.NullString
	var height sql.NullFloat64
	err := tx.QueryRowContext(ctx, "SELECT sex, birth_date, height_cm FROM users WHERE id = ?", userID.String()).
		Scan(&sex, &birth, &height)
	if errors.Is(err, sql.ErrNoRows) {
		return profileState{}, ErrNotFound
	}
	if err != nil {
		return profileState{}, err
	}
	var s profileState
	if sex.Valid {
		s.Sex = &sex.String
	}
	if birth.Valid {
		s.BirthDate = &birth.String
	}
	if height.Valid {
		s.HeightCm = &height.Float64
	}
	return s, nil
}

func putProfile(ctx context.Context, tx *sql.Tx, userID uuid.UUID, s profileState, now time.Time) error {
	_, err := tx.ExecContext(ctx, "UPDATE users SET sex = ?, birth_date = ?, height_cm = ?, updated_at = ? WHERE id = ?",
		s.Sex, s.BirthDate, s.HeightCm, formatTime(now), userID.String())
	return err
}

// Profile returns userID's profile: what is not given is zero.
func (db *DB) Profile(ctx context.Context, userID uuid.UUID) (profile.Profile, error) {
	var p profile.Profile
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		s, err := profileOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		p, err = s.profile()
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return profile.Profile{}, err
	}
	if err != nil {
		return profile.Profile{}, fmt.Errorf("profile: %w", err)
	}
	return p, nil
}

// SetProfile stores p as userID's profile, today being the user's day
// now, as a's event. The limits of a plan depend on the profile, so only
// a web session sets it, and undo leaves it (UndoLast): the event is the
// audit trail.
func (db *DB) SetProfile(ctx context.Context, userID uuid.UUID, a event.Actor, p profile.Profile, today day.Date, now time.Time) error {
	if err := p.Validate(today); err != nil {
		return fmt.Errorf("invalid profile: %w", err)
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := profileOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if err := putProfile(ctx, tx, userID, profileStateOf(p), now); err != nil {
			return err
		}
		after, err := profileOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.ProfileSet, profileEntity, before, after, now)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("set profile: %w", err)
	}
	return nil
}

// phaseState is a phase event's state; the event's entity is the
// phase's start.
type phaseState struct {
	Type            string   `json:"type"`
	End             *string  `json:"end"`
	RatePctWeek     float64  `json:"rate_pct_week"`
	GoalWeight      *float64 `json:"goal_weight"`
	MilestoneWeight *float64 `json:"milestone_weight"`
}

func (s phaseState) phase(start day.Date) (plan.Phase, error) {
	p := plan.Phase{Type: plan.PhaseType(s.Type), Start: start, RatePctWeek: s.RatePctWeek, GoalWeight: s.GoalWeight, MilestoneWeight: s.MilestoneWeight}
	if s.End != nil {
		d, err := day.ParseDate(*s.End)
		if err != nil {
			return plan.Phase{}, err
		}
		p.End = d
	}
	return p, nil
}

func phaseStateOf(p plan.Phase) phaseState {
	s := phaseState{Type: string(p.Type), RatePctWeek: p.RatePctWeek, GoalWeight: p.GoalWeight, MilestoneWeight: p.MilestoneWeight}
	if !p.End.IsZero() {
		s.End = new(p.End.String())
	}
	return s
}

const phaseColumns = "start, type, end_day, rate_pct_week, goal_weight, milestone_weight"

func scanPhase(row interface{ Scan(...any) error }) (day.Date, phaseState, error) {
	var start string
	var s phaseState
	var end sql.NullString
	var goal, milestone sql.NullFloat64
	if err := row.Scan(&start, &s.Type, &end, &s.RatePctWeek, &goal, &milestone); err != nil {
		return day.Date{}, phaseState{}, err
	}
	d, err := day.ParseDate(start)
	if err != nil {
		return day.Date{}, phaseState{}, err
	}
	if end.Valid {
		s.End = &end.String
	}
	if goal.Valid {
		s.GoalWeight = &goal.Float64
	}
	if milestone.Valid {
		s.MilestoneWeight = &milestone.Float64
	}
	return d, s, nil
}

// phaseOf returns the state of userID's phase that starts on start, nil
// for none.
func phaseOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID, start day.Date) (*phaseState, error) {
	_, s, err := scanPhase(tx.QueryRowContext(ctx, "SELECT "+phaseColumns+" FROM phases WHERE user_id = ? AND start = ?",
		userID.String(), start.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// putPhase stores s as userID's phase that starts on start, or deletes
// it for nil.
func putPhase(ctx context.Context, tx *sql.Tx, userID uuid.UUID, start day.Date, s *phaseState, now time.Time) error {
	if s == nil {
		_, err := tx.ExecContext(ctx, "DELETE FROM phases WHERE user_id = ? AND start = ?", userID.String(), start.String())
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO phases (user_id, `+phaseColumns+`, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, start) DO UPDATE SET type = excluded.type, end_day = excluded.end_day,
			rate_pct_week = excluded.rate_pct_week, goal_weight = excluded.goal_weight,
			milestone_weight = excluded.milestone_weight, updated_at = excluded.updated_at`,
		userID.String(), start.String(), s.Type, s.End, s.RatePctWeek, s.GoalWeight, s.MilestoneWeight, formatTime(now))
	return err
}

func phasesIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID) ([]plan.Phase, error) {
	rows, err := tx.QueryContext(ctx, "SELECT "+phaseColumns+" FROM phases WHERE user_id = ? ORDER BY start", userID.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []plan.Phase
	for rows.Next() {
		start, s, err := scanPhase(rows)
		if err != nil {
			return nil, err
		}
		p, err := s.phase(start)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Phases returns userID's goal timeline, in the order the phases start.
func (db *DB) Phases(ctx context.Context, userID uuid.UUID) ([]plan.Phase, error) {
	var out []plan.Phase
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = phasesIn(ctx, tx, userID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("phases: %w", err)
	}
	return out, nil
}

// SetPhase stores p as userID's phase that starts on p.Start, a new one
// or an edit of the one that starts then, as a's event; a change
// supersedes the user's pending proposals (recordPlan). A phase that
// would overlap another is refused with ErrPhaseOverlap. The limits of a
// plan are Validate's, which runs before.
func (db *DB) SetPhase(ctx context.Context, userID uuid.UUID, a event.Actor, p plan.Phase, now time.Time) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("invalid phase: %w", err)
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := setPhaseIn(ctx, tx, userID, a, p, now); err != nil {
			return err
		}
		return checkTimeline(ctx, tx, userID)
	})
	if errors.Is(err, ErrPhaseOverlap) {
		return err
	}
	if err != nil {
		return fmt.Errorf("set phase: %w", err)
	}
	return nil
}

// setPhaseIn stores p as userID's phase that starts on p.Start in tx, as
// a's change of the plan (recordPlan); the timeline is the caller's to
// check.
func setPhaseIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, p plan.Phase, now time.Time) error {
	before, err := phaseOf(ctx, tx, userID, p.Start)
	if err != nil {
		return err
	}
	if err := putPhase(ctx, tx, userID, p.Start, new(phaseStateOf(p)), now); err != nil {
		return err
	}
	after, err := phaseOf(ctx, tx, userID, p.Start)
	if err != nil {
		return err
	}
	return recordPlan(ctx, tx, userID, a, event.PhaseSet, p.Start.String(), before, after, now)
}

// checkTimeline reports ErrPhaseOverlap when userID's phases overlap. A
// stored phase that breaks a rule of its shape is an error of its own,
// not an overlap.
func checkTimeline(ctx context.Context, tx *sql.Tx, userID uuid.UUID) error {
	ps, err := phasesIn(ctx, tx, userID)
	if err != nil {
		return err
	}
	for _, p := range ps {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("the stored phase from %s: %w", p.Start, err)
		}
	}
	if err := plan.ValidateTimeline(ps); err != nil {
		return fmt.Errorf("%w: %w", ErrPhaseOverlap, err)
	}
	return nil
}

// DeletePhase deletes userID's phase that starts on start, as a's event,
// superseding the user's pending proposals; ErrNotFound when there is
// none.
func (db *DB) DeletePhase(ctx context.Context, userID uuid.UUID, a event.Actor, start day.Date, now time.Time) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error { return deletePhaseIn(ctx, tx, userID, a, start, now) })
	if errors.Is(err, ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("delete phase: %w", err)
	}
	return nil
}

// deletePhaseIn deletes userID's phase that starts on start in tx, as a's
// change of the plan (recordPlan); ErrNotFound when there is none.
func deletePhaseIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, start day.Date, now time.Time) error {
	before, err := phaseOf(ctx, tx, userID, start)
	if err != nil {
		return err
	}
	if before == nil {
		return ErrNotFound
	}
	if err := putPhase(ctx, tx, userID, start, nil, now); err != nil {
		return err
	}
	return recordPlan(ctx, tx, userID, a, event.PhaseDelete, start.String(), before, nil, now)
}

// dayTargetState is a day type's part of a target's state.
type dayTargetState struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
}

// targetState is a target event's state; the event's entity is the
// target's first day.
type targetState struct {
	Training   dayTargetState `json:"training"`
	Rest       dayTargetState `json:"rest"`
	ProposalID *string        `json:"proposal_id"`
}

func dayTargetStateOf(t plan.DayTarget) dayTargetState {
	return dayTargetState{Kcal: t.Kcal, Protein: t.Protein, Fat: t.Fat}
}

func (s targetState) target(from day.Date) plan.Target {
	d := func(t dayTargetState) plan.DayTarget {
		return plan.DayTarget{Kcal: t.Kcal, Protein: t.Protein, Fat: t.Fat}
	}
	return plan.Target{EffectiveFrom: from, Training: d(s.Training), Rest: d(s.Rest)}
}

const targetColumns = `effective_from, training_kcal, training_protein, training_fat,
	rest_kcal, rest_protein, rest_fat, proposal_id`

func scanTarget(row interface{ Scan(...any) error }) (day.Date, targetState, error) {
	var from string
	var s targetState
	var proposal sql.NullString
	if err := row.Scan(&from, &s.Training.Kcal, &s.Training.Protein, &s.Training.Fat,
		&s.Rest.Kcal, &s.Rest.Protein, &s.Rest.Fat, &proposal); err != nil {
		return day.Date{}, targetState{}, err
	}
	d, err := day.ParseDate(from)
	if err != nil {
		return day.Date{}, targetState{}, err
	}
	if proposal.Valid {
		s.ProposalID = &proposal.String
	}
	return d, s, nil
}

// targetOf returns the state of userID's target from day from, nil for
// none.
func targetOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID, from day.Date) (*targetState, error) {
	_, s, err := scanTarget(tx.QueryRowContext(ctx, "SELECT "+targetColumns+" FROM targets WHERE user_id = ? AND effective_from = ?",
		userID.String(), from.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func putTarget(ctx context.Context, tx *sql.Tx, userID uuid.UUID, from day.Date, s *targetState, now time.Time) error {
	if s == nil {
		_, err := tx.ExecContext(ctx, "DELETE FROM targets WHERE user_id = ? AND effective_from = ?", userID.String(), from.String())
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO targets (user_id, `+targetColumns+`, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, effective_from) DO UPDATE SET training_kcal = excluded.training_kcal,
			training_protein = excluded.training_protein, training_fat = excluded.training_fat,
			rest_kcal = excluded.rest_kcal, rest_protein = excluded.rest_protein, rest_fat = excluded.rest_fat,
			proposal_id = excluded.proposal_id, updated_at = excluded.updated_at`,
		userID.String(), from.String(), s.Training.Kcal, s.Training.Protein, s.Training.Fat,
		s.Rest.Kcal, s.Rest.Protein, s.Rest.Fat, s.ProposalID, formatTime(now))
	return err
}

// Targets returns userID's targets, in the order they take effect.
func (db *DB) Targets(ctx context.Context, userID uuid.UUID) ([]plan.Target, error) {
	var out []plan.Target
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT "+targetColumns+" FROM targets WHERE user_id = ? ORDER BY effective_from", userID.String())
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			from, s, err := scanTarget(rows)
			if err != nil {
				return err
			}
			out = append(out, s.target(from))
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("targets: %w", err)
	}
	return out, nil
}

// SetTarget stores t as userID's target from t.EffectiveFrom, a new one
// or an edit of the one from that day, as a's event; a change supersedes
// the user's pending proposals. The limits of a plan are Validate's,
// which runs before.
func (db *DB) SetTarget(ctx context.Context, userID uuid.UUID, a event.Actor, t plan.Target, now time.Time) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("invalid target: %w", err)
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error { return setTargetIn(ctx, tx, userID, a, t, nil, now) })
	if err != nil {
		return fmt.Errorf("set target: %w", err)
	}
	return nil
}

// setTargetIn stores t as userID's target from t.EffectiveFrom in tx, set
// by the proposal proposalID or by none for nil, as a's change of the
// plan (recordPlan).
func setTargetIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, t plan.Target, proposalID *string, now time.Time) error {
	before, err := targetOf(ctx, tx, userID, t.EffectiveFrom)
	if err != nil {
		return err
	}
	s := targetState{Training: dayTargetStateOf(t.Training), Rest: dayTargetStateOf(t.Rest), ProposalID: proposalID}
	if err := putTarget(ctx, tx, userID, t.EffectiveFrom, &s, now); err != nil {
		return err
	}
	after, err := targetOf(ctx, tx, userID, t.EffectiveFrom)
	if err != nil {
		return err
	}
	return recordPlan(ctx, tx, userID, a, event.TargetSet, t.EffectiveFrom.String(), before, after, now)
}

// DeleteTarget deletes userID's target from day from, as a's event,
// superseding the user's pending proposals; ErrNotFound when there is
// none.
func (db *DB) DeleteTarget(ctx context.Context, userID uuid.UUID, a event.Actor, from day.Date, now time.Time) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error { return deleteTargetIn(ctx, tx, userID, a, from, now) })
	if errors.Is(err, ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("delete target: %w", err)
	}
	return nil
}

// deleteTargetIn deletes userID's target from day from in tx, as a's
// change of the plan (recordPlan); ErrNotFound when there is none.
func deleteTargetIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, from day.Date, now time.Time) error {
	before, err := targetOf(ctx, tx, userID, from)
	if err != nil {
		return err
	}
	if before == nil {
		return ErrNotFound
	}
	if err := putTarget(ctx, tx, userID, from, nil, now); err != nil {
		return err
	}
	return recordPlan(ctx, tx, userID, a, event.TargetDelete, from.String(), before, nil, now)
}

// trainingDayState is the state of a day's toggle of its type: nil when
// the user did not set it.
type trainingDayState struct {
	TrainingDay *bool `json:"training_day"`
}

func trainingDayOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date) (trainingDayState, error) {
	var v sql.NullBool
	err := tx.QueryRowContext(ctx, "SELECT training_day FROM days WHERE user_id = ? AND day = ?", userID.String(), d.String()).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return trainingDayState{}, err
	}
	if v.Valid {
		return trainingDayState{&v.Bool}, nil
	}
	return trainingDayState{}, nil
}

func putTrainingDay(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date, training *bool, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO days (user_id, day, training_day, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, day) DO UPDATE SET training_day = excluded.training_day, updated_at = excluded.updated_at`,
		userID.String(), d.String(), training, formatTime(now))
	return err
}

// SetTrainingDay sets the check-in's toggle of userID's day d: a training
// day for true, a rest day for false, and the plan's type again for nil
// (plan.TypeOf). It is recorded as a's event; a toggle left as it was
// records nothing.
func (db *DB) SetTrainingDay(ctx context.Context, userID uuid.UUID, a event.Actor, d day.Date, training *bool, now time.Time) error {
	if d.IsZero() {
		return errors.New("invalid training day: the day is missing")
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := trainingDayOf(ctx, tx, userID, d)
		// None on a day without one would only add an empty row.
		if err != nil || before.TrainingDay == nil && training == nil {
			return err
		}
		if err := putTrainingDay(ctx, tx, userID, d, training, now); err != nil {
			return err
		}
		after, err := trainingDayOf(ctx, tx, userID, d)
		if err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.TrainingDay, d.String(), before, after, now)
		return err
	})
	if err != nil {
		return fmt.Errorf("set training day: %w", err)
	}
	return nil
}

func takeBackTrainingDay(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	d, err := entityDay(e.Entity)
	if err != nil {
		return err
	}
	var s trainingDayState
	if e.Before != nil {
		if err := decode(e.Before, &s); err != nil {
			return err
		}
	}
	return putTrainingDay(ctx, tx, userID, d, s.TrainingDay, now)
}

// TrainingDayToggles returns the check-in's toggles of userID's days from
// from to to, both included: true for a training day, false for a rest
// day; a day the user did not set is not in it.
func (db *DB) TrainingDayToggles(ctx context.Context, userID uuid.UUID, from, to day.Date) (map[day.Date]bool, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT day, training_day FROM days
		WHERE user_id = ? AND day BETWEEN ? AND ? AND training_day IS NOT NULL ORDER BY day`,
		userID.String(), from.String(), to.String())
	if err != nil {
		return nil, fmt.Errorf("training days: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[day.Date]bool{}
	for rows.Next() {
		var d string
		var training bool
		if err := rows.Scan(&d, &training); err != nil {
			return nil, fmt.Errorf("training days: %w", err)
		}
		date, err := day.ParseDate(d)
		if err != nil {
			return nil, fmt.Errorf("training days: %w", err)
		}
		out[date] = training
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("training days: %w", err)
	}
	return out, nil
}

// weekdaysState is the state of the plan's training weekdays.
type weekdaysState struct {
	TrainingWeekdays int `json:"training_weekdays"`
}

func weekdaysOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (weekdaysState, error) {
	var w int
	err := tx.QueryRowContext(ctx, "SELECT training_weekdays FROM plan_settings WHERE user_id = ?", userID.String()).Scan(&w)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return weekdaysState{}, err
	}
	return weekdaysState{w}, nil
}

func putWeekdays(ctx context.Context, tx *sql.Tx, userID uuid.UUID, w int, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO plan_settings (user_id, training_weekdays, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET training_weekdays = excluded.training_weekdays, updated_at = excluded.updated_at`,
		userID.String(), w, formatTime(now))
	return err
}

// TrainingWeekdays returns the training weekdays of userID's plan, which
// give each day its type ahead of time; none until set.
func (db *DB) TrainingWeekdays(ctx context.Context, userID uuid.UUID) (plan.Weekdays, error) {
	var s weekdaysState
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		s, err = weekdaysOf(ctx, tx, userID)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("training weekdays: %w", err)
	}
	return plan.Weekdays(s.TrainingWeekdays), nil //nolint:gosec // G115: the table holds 0 to 127
}

// SetTrainingWeekdays stores w as the training weekdays of userID's plan,
// as a's event; a change supersedes the user's pending proposals.
func (db *DB) SetTrainingWeekdays(ctx context.Context, userID uuid.UUID, a event.Actor, w plan.Weekdays, now time.Time) error {
	if err := w.Validate(); err != nil {
		return fmt.Errorf("invalid training weekdays: %w", err)
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error { return setWeekdaysIn(ctx, tx, userID, a, w, now) })
	if err != nil {
		return fmt.Errorf("set training weekdays: %w", err)
	}
	return nil
}

// setWeekdaysIn stores w as the training weekdays of userID's plan in tx,
// as a's change of the plan (recordPlan).
func setWeekdaysIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, w plan.Weekdays, now time.Time) error {
	before, err := weekdaysOf(ctx, tx, userID)
	if err != nil {
		return err
	}
	if err := putWeekdays(ctx, tx, userID, int(w), now); err != nil {
		return err
	}
	return recordPlan(ctx, tx, userID, a, event.WeekdaysSet, weekdaysEntity, before, weekdaysState{int(w)}, now)
}

// limitsState is the state of a user's limits: each limit that is not at
// its default, by its name, or nil for none. A limit at its default
// follows the default as a later release changes it.
type limitsState map[string]float64

func limitsStateOf(l limits.Limits) limitsState {
	var s limitsState
	def := limits.UserDefaults()
	for _, f := range limits.Fields() {
		if v := f.Get(l); v != f.Get(def) {
			if s == nil {
				s = limitsState{}
			}
			s[f.Name] = v + 0
		}
	}
	return s
}

// limits returns the limits of s: the defaults, with each limit s names.
func (s limitsState) limits() limits.Limits {
	l := limits.UserDefaults()
	for _, f := range limits.Fields() {
		if v, ok := s[f.Name]; ok {
			f.Set(&l, v)
		}
	}
	return l
}

func limitsOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (limitsState, error) {
	var raw sql.NullString
	err := tx.QueryRowContext(ctx, "SELECT limits FROM plan_settings WHERE user_id = ?", userID.String()).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !raw.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s limitsState
	if err := json.Unmarshal([]byte(raw.String), &s); err != nil {
		return nil, fmt.Errorf("limits: %w", err)
	}
	return s, nil
}

func putLimits(ctx context.Context, tx *sql.Tx, userID uuid.UUID, s limitsState, now time.Time) error {
	var raw any
	if s != nil {
		b, err := json.Marshal(s)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO plan_settings (user_id, limits, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET limits = excluded.limits, updated_at = excluded.updated_at`,
		userID.String(), raw, formatTime(now))
	return err
}

// UserLimits returns userID's limits: the defaults until the user changes
// them. The limits that hold are the stricter of these and the
// instance's (limits.Effective).
func (db *DB) UserLimits(ctx context.Context, userID uuid.UUID) (limits.Limits, error) {
	var s limitsState
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		s, err = limitsOf(ctx, tx, userID)
		return err
	})
	if err != nil {
		return limits.Limits{}, fmt.Errorf("user limits: %w", err)
	}
	return s.limits(), nil
}

// SetUserLimits stores l as userID's limits, as a's event. A limit looser
// than the instance's is refused with ErrLimitsBeyond. Only a web session
// sets them, with a confirmation for a limit it loosens
// (limits.Loosened), and undo leaves them (UndoLast).
func (db *DB) SetUserLimits(ctx context.Context, userID uuid.UUID, a event.Actor, l, instance limits.Limits, now time.Time) error {
	_, err := db.UpdateUserLimits(ctx, userID, a, instance, func(limits.Limits) (limits.Limits, error) { return l, nil }, now)
	return err
}

// UpdateUserLimits stores the limits change returns as userID's, as a's
// event, and returns them. change gets the limits as they are in the
// transaction that stores its answer, so a check it makes against them (a
// loosening needs a confirmation) holds when it is stored, whatever another
// change did meanwhile. An error of change stores nothing, and is returned
// wrapped. What change returns meets SetUserLimits's rules.
func (db *DB) UpdateUserLimits(ctx context.Context, userID uuid.UUID, a event.Actor, instance limits.Limits,
	change func(current limits.Limits) (limits.Limits, error), now time.Time,
) (limits.Limits, error) {
	if err := a.Validate(); err != nil {
		return limits.Limits{}, fmt.Errorf("invalid actor: %w", err)
	}
	var next limits.Limits
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := limitsOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if next, err = change(before.limits()); err != nil {
			return err
		}
		if err := next.Validate(); err != nil {
			return fmt.Errorf("invalid limits: %w", err)
		}
		if names := limits.Beyond(next, instance); names != nil {
			return fmt.Errorf("%w: %s", ErrLimitsBeyond, strings.Join(names, ", "))
		}
		return setLimitsIn(ctx, tx, userID, a, before, next, now)
	})
	if err != nil {
		return limits.Limits{}, fmt.Errorf("set user limits: %w", err)
	}
	return next, nil
}

// setLimitsIn stores next, which keeps its rules, as userID's limits in
// tx, those before being before, as a's event, and when they changed,
// supersedes the user's pending proposals that change the limits.
func setLimitsIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a event.Actor, before limitsState, next limits.Limits, now time.Time) error {
	after := limitsStateOf(next)
	if err := putLimits(ctx, tx, userID, after, now); err != nil {
		return err
	}
	id, err := record(ctx, tx, userID, a, event.LimitsSet, limitsEntity, before, after, now)
	if err != nil || id == uuid.Nil() {
		return err
	}
	return supersedeLimitsProposals(ctx, tx, userID, a, now)
}

// notesState is the state of the notes for a coach, nil for none.
type notesState struct {
	Notes string `json:"notes"`
}

func notesOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (*notesState, error) {
	var s notesState
	err := tx.QueryRowContext(ctx, "SELECT notes FROM profile_notes WHERE user_id = ?", userID.String()).Scan(&s.Notes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func putNotes(ctx context.Context, tx *sql.Tx, userID uuid.UUID, s *notesState, now time.Time) error {
	if s == nil {
		_, err := tx.ExecContext(ctx, "DELETE FROM profile_notes WHERE user_id = ?", userID.String())
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO profile_notes (user_id, notes, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET notes = excluded.notes, updated_at = excluded.updated_at`,
		userID.String(), s.Notes, formatTime(now))
	return err
}

// ProfileNotes returns the notes userID keeps for a coach; "" for none.
func (db *DB) ProfileNotes(ctx context.Context, userID uuid.UUID) (string, error) {
	var s *notesState
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		s, err = notesOf(ctx, tx, userID)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("profile notes: %w", err)
	}
	if s == nil {
		return "", nil
	}
	return s.Notes, nil
}

// SetProfileNotes stores notes as the notes userID keeps for a coach,
// as a's event; "" deletes them.
func (db *DB) SetProfileNotes(ctx context.Context, userID uuid.UUID, a event.Actor, notes string, now time.Time) error {
	if err := profile.ValidateNotes(notes); err != nil {
		return fmt.Errorf("invalid notes: %w", err)
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	var after *notesState
	if notes != "" {
		after = &notesState{notes}
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := notesOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if err := putNotes(ctx, tx, userID, after, now); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.NotesSet, notesEntity, before, after, now)
		return err
	})
	if err != nil {
		return fmt.Errorf("set profile notes: %w", err)
	}
	return nil
}

func takeBackNotes(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	var s *notesState
	if e.Before != nil {
		if err := decode(e.Before, &s); err != nil {
			return err
		}
	}
	return putNotes(ctx, tx, userID, s, now)
}

// planStateNow returns the state of the plan's entity of e now, as e's
// kind of event records it: of the plan's kinds, undo takes back only
// the notes and a day's toggle (UndoLast).
func planStateNow(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event) (any, error) {
	if e.Action == event.NotesSet {
		return notesOf(ctx, tx, userID)
	}
	d, err := entityDay(e.Entity)
	if err != nil {
		return nil, err
	}
	return trainingDayOf(ctx, tx, userID, d)
}

// takeBackPlan puts the plan's entity of e back as it was before e.
func takeBackPlan(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	if e.Action == event.NotesSet {
		return takeBackNotes(ctx, tx, userID, e, now)
	}
	return takeBackTrainingDay(ctx, tx, userID, e, now)
}
