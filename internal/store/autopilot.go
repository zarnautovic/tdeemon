package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/autopilot"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
)

// The entity of the autopilot's settings' events, and the prefix of a
// review's ("review/2026-11-02").
const (
	autopilotEntity = "autopilot"
	reviewEntity    = "review/"
)

// AutopilotActor is who the autopilot's writes are by: its steps, its
// proposals and its reviews, which undo leaves.
var AutopilotActor = event.Actor{Kind: event.Autopilot, Client: event.ClientAutopilot}

// ErrReviewed reports a review of a day that has one: the job ran for it
// already, before a crash kept its run from being recorded.
var ErrReviewed = errors.New("the day has its review")

// autopilotState is the state of a user's settings of the autopilot.
type autopilotState struct {
	Mode    string `json:"mode"`
	Weekday int    `json:"weekday"`
}

func autopilotOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (autopilotState, error) {
	def := autopilot.DefaultSettings()
	s := autopilotState{Mode: string(def.Mode), Weekday: int(def.Weekday)}
	err := tx.QueryRowContext(ctx, "SELECT autonomy, review_weekday FROM plan_settings WHERE user_id = ?",
		userID.String()).Scan(&s.Mode, &s.Weekday)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return autopilotState{}, err
	}
	return s, nil
}

func (s autopilotState) settings() autopilot.Settings {
	return autopilot.Settings{Mode: autopilot.Mode(s.Mode), Weekday: time.Weekday(s.Weekday)}
}

// AutopilotSettings returns userID's settings of the autopilot: the
// defaults until the user changes them.
func (db *DB) AutopilotSettings(ctx context.Context, userID uuid.UUID) (autopilot.Settings, error) {
	var s autopilotState
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		s, err = autopilotOf(ctx, tx, userID)
		return err
	})
	if err != nil {
		return autopilot.Settings{}, fmt.Errorf("autopilot settings: %w", err)
	}
	return s.settings(), nil
}

// SetAutopilotSettings stores s as userID's settings of the autopilot, as
// a's event. Only a web session sets them, as the limits, and undo leaves
// them.
func (db *DB) SetAutopilotSettings(ctx context.Context, userID uuid.UUID, a event.Actor, s autopilot.Settings, now time.Time) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("invalid autopilot settings: %w", err)
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := autopilotOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO plan_settings (user_id, autonomy, review_weekday, updated_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET autonomy = excluded.autonomy, review_weekday = excluded.review_weekday,
				updated_at = excluded.updated_at`,
			userID.String(), string(s.Mode), int(s.Weekday), formatTime(now)); err != nil {
			return err
		}
		after := autopilotState{Mode: string(s.Mode), Weekday: int(s.Weekday)}
		_, err = record(ctx, tx, userID, a, event.AutopilotSet, autopilotEntity, before, after, now)
		return err
	})
	if err != nil {
		return fmt.Errorf("set autopilot settings: %w", err)
	}
	return nil
}

// reviewState is a review's numbers, as its row and its event keep them.
type reviewState struct {
	Outcome          string  `json:"outcome"`
	Reason           string  `json:"reason"`
	Rule             string  `json:"rule,omitempty"`
	StepKcal         float64 `json:"step_kcal"`
	GapKcal          float64 `json:"gap_kcal"`
	PastDrift        bool    `json:"past_drift"`
	MeasuredPctWeek  float64 `json:"measured_pct_week"`
	TargetPctWeek    float64 `json:"target_pct_week"`
	TolerancePctWeek float64 `json:"tolerance_pct_week"`
	TDEE             float64 `json:"tdee"`
	HalfWidth        float64 `json:"half_width"`
	IntakeKcal       float64 `json:"intake_kcal"`
	TargetKcal       float64 `json:"target_kcal"`
	AfterKcal        float64 `json:"after_kcal"`
	LoggedDays       int     `json:"logged_days"`
	WeighIns         int     `json:"weigh_ins"`
	NeedsLoggedDays  int     `json:"needs_logged_days"`
	NeedsWeighIns    int     `json:"needs_weigh_ins"`
	StepsChangePct   float64 `json:"steps_change_pct"`
	From             string  `json:"from,omitempty"`
	Expected         string  `json:"expected,omitempty"`
	CarbsG           float64 `json:"carbs_g"`
	DayType          string  `json:"day_type,omitempty"`
	BMI              float64 `json:"bmi"`
	DriftKcal        float64 `json:"drift_kcal"`
	DriftLimitKcal   float64 `json:"drift_limit_kcal"`
	ProposalID       string  `json:"proposal_id,omitempty"`
}

func dateString(d day.Date) string {
	if d.IsZero() {
		return ""
	}
	return d.String()
}

func reviewStateOf(r autopilot.Review, proposalID uuid.UUID) reviewState {
	s := reviewState{
		Outcome: string(r.Outcome), Reason: string(r.Reason), Rule: r.Rule,
		StepKcal: r.StepKcal + 0, GapKcal: r.GapKcal + 0, PastDrift: r.PastDrift,
		MeasuredPctWeek: r.MeasuredPctWeek + 0, TargetPctWeek: r.TargetPctWeek + 0, TolerancePctWeek: r.TolerancePctWeek + 0,
		TDEE: r.TDEE + 0, HalfWidth: r.HalfWidth + 0, IntakeKcal: r.IntakeKcal + 0, TargetKcal: r.TargetKcal + 0, AfterKcal: r.AfterKcal + 0,
		LoggedDays: r.LoggedDays, WeighIns: r.WeighIns, NeedsLoggedDays: r.NeedsLoggedDays, NeedsWeighIns: r.NeedsWeighIns,
		StepsChangePct: r.StepsChangePct + 0, From: dateString(r.From), Expected: dateString(r.Expected),
		CarbsG: r.CarbsG + 0, DayType: string(r.DayType), BMI: r.BMI + 0,
		DriftKcal: r.DriftKcal + 0, DriftLimitKcal: r.DriftLimitKcal + 0,
	}
	if proposalID != uuid.Nil() {
		s.ProposalID = proposalID.String()
	}
	return s
}

func optionalDate(s string) (day.Date, error) {
	if s == "" {
		return day.Date{}, nil
	}
	return day.ParseDate(s)
}

func (s reviewState) review(date day.Date) (StoredReview, error) {
	r := autopilot.Review{
		Date: date, Outcome: autopilot.Outcome(s.Outcome), Reason: autopilot.Reason(s.Reason), Rule: s.Rule,
		StepKcal: s.StepKcal, GapKcal: s.GapKcal, PastDrift: s.PastDrift,
		MeasuredPctWeek: s.MeasuredPctWeek, TargetPctWeek: s.TargetPctWeek, TolerancePctWeek: s.TolerancePctWeek,
		TDEE: s.TDEE, HalfWidth: s.HalfWidth, IntakeKcal: s.IntakeKcal, TargetKcal: s.TargetKcal, AfterKcal: s.AfterKcal,
		LoggedDays: s.LoggedDays, WeighIns: s.WeighIns, NeedsLoggedDays: s.NeedsLoggedDays, NeedsWeighIns: s.NeedsWeighIns,
		StepsChangePct: s.StepsChangePct, CarbsG: s.CarbsG, DayType: plan.DayType(s.DayType), BMI: s.BMI,
		DriftKcal: s.DriftKcal, DriftLimitKcal: s.DriftLimitKcal,
	}
	var err error
	if r.From, err = optionalDate(s.From); err != nil {
		return StoredReview{}, fmt.Errorf("review of %s: %w", date, err)
	}
	if r.Expected, err = optionalDate(s.Expected); err != nil {
		return StoredReview{}, fmt.Errorf("review of %s: %w", date, err)
	}
	out := StoredReview{Review: r}
	if s.ProposalID != "" {
		if out.ProposalID, err = uuid.Parse(s.ProposalID); err != nil {
			return StoredReview{}, fmt.Errorf("review of %s: %w", date, err)
		}
	}
	return out, nil
}

// StoredReview is a review as the store keeps it: the review, and the
// proposal it made or applied, or uuid.Nil.
type StoredReview struct {
	Review     autopilot.Review
	ProposalID uuid.UUID
}

// SaveReview records r, the autopilot's review of userID's day r.Date,
// as the autopilot's, today being that day, and returns it as stored:
// with the step it applied (Applied: a proposal of the autopilot's,
// applied at once, which keeps tdee as the TDEE it was taken at) or the
// proposal it made (Proposed, which supersedes the user's pending ones,
// as any). check holds the changes to the limits again, in the
// transaction that writes them. A day that has its review already is
// ErrReviewed, and nothing is written: the job ran for it before.
func (db *DB) SaveReview(ctx context.Context, userID uuid.UUID, r autopilot.Review, tdee float64, now time.Time,
	check ProposalCheck,
) (StoredReview, error) {
	a := AutopilotActor
	var out StoredReview
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM autopilot_reviews WHERE user_id = ? AND day = ?",
			userID.String(), r.Date.String()).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrReviewed
		}
		id := uuid.Nil()
		if r.Outcome == autopilot.Applied || r.Outcome == autopilot.Proposed {
			p, err := reviewProposal(ctx, tx, userID, r, tdee, now, check)
			if err != nil {
				return err
			}
			id = p.ID
		}
		s := reviewStateOf(r, id)
		b, err := json.Marshal(s)
		if err != nil {
			return err
		}
		var pid any
		if id != uuid.Nil() {
			pid = id.String()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO autopilot_reviews (user_id, day, outcome, reason, review, proposal_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, userID.String(), r.Date.String(), s.Outcome, s.Reason, string(b), pid, formatTime(now)); err != nil {
			return err
		}
		if _, err := record(ctx, tx, userID, a, event.AutopilotReview, reviewEntity+r.Date.String(), nil, s, now); err != nil {
			return err
		}
		out, err = s.review(r.Date)
		return err
	})
	if errors.Is(err, ErrReviewed) {
		return StoredReview{}, err
	}
	if err != nil {
		return StoredReview{}, proposalError("save review", err)
	}
	return out, nil
}

// reviewProposal makes the autopilot's proposal of r's changes in tx, and
// applies it at once for a step it takes itself.
func reviewProposal(ctx context.Context, tx *sql.Tx, userID uuid.UUID, r autopilot.Review, tdee float64, now time.Time,
	check ProposalCheck,
) (proposal.Proposal, error) {
	a := AutopilotActor
	d := proposal.Draft{Rationale: proposal.Autopilot, Changes: r.Changes}
	c, err := planBefore(ctx, tx, userID, d.Changes)
	if err != nil {
		return proposal.Proposal{}, err
	}
	p, checked, err := createProposalIn(ctx, tx, userID, a, d, c, uuid.Nil(), r.Date, now, check)
	if err != nil || r.Outcome != autopilot.Applied {
		return p, err
	}
	if checked > 0 {
		// The TDEE of the check, in this transaction.
		tdee = checked
	}
	// check held the changes a moment ago, in this transaction.
	return applyIn(ctx, tx, userID, a, p, now, nil, tdee)
}

// Reviews returns up to limit of userID's latest reviews, newest first.
func (db *DB) Reviews(ctx context.Context, userID uuid.UUID, limit int) ([]StoredReview, error) {
	out := []StoredReview{}
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT day, review FROM autopilot_reviews WHERE user_id = ? ORDER BY day DESC LIMIT ?",
			userID.String(), limit)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			r, err := scanReview(rows)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("reviews: %w", err)
	}
	return out, nil
}

// ReviewsOf returns userID's reviews that made the proposals ids, by the
// proposal's id; a proposal no review made is left out.
func (db *DB) ReviewsOf(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]StoredReview, error) {
	out := map[uuid.UUID]StoredReview{}
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			r, err := scanReview(tx.QueryRowContext(ctx, "SELECT day, review FROM autopilot_reviews WHERE user_id = ? AND proposal_id = ?",
				userID.String(), id.String()))
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			out[id] = r
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reviews of proposals: %w", err)
	}
	return out, nil
}

func scanReview(row interface{ Scan(...any) error }) (StoredReview, error) {
	var d, raw string
	if err := row.Scan(&d, &raw); err != nil {
		return StoredReview{}, err
	}
	date, err := day.ParseDate(d)
	if err != nil {
		return StoredReview{}, fmt.Errorf("review of %q: %w", d, err)
	}
	var s reviewState
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return StoredReview{}, fmt.Errorf("review of %s: %w", d, err)
	}
	return s.review(date)
}
