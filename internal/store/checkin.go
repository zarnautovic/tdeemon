package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
)

// DayCheckIn is what the user says of a day: the check-in (mood, hunger
// and note), the toggle of the day's type (nil when the plan decides) and
// the reason the day is out of calibration ("" when it is in). A
// check-in's event records it whole.
type DayCheckIn struct {
	Mood           int    `json:"mood,omitempty"`
	Hunger         int    `json:"hunger,omitempty"`
	Note           string `json:"note,omitempty"`
	TrainingDay    *bool  `json:"training_day,omitempty"`
	ExcludedReason string `json:"excluded_reason,omitempty"`
}

// validate checks c as a day can hold it.
func (c DayCheckIn) validate() error {
	err := day.CheckIn{Mood: c.Mood, Hunger: c.Hunger, Note: c.Note}.Validate()
	if c.ExcludedReason != "" {
		err = errors.Join(err, day.ValidateExclusion(day.Exclusion{Reason: c.ExcludedReason}))
	}
	return err
}

// CheckInOf returns what userID says of day d; a day they said nothing of
// is the zero DayCheckIn.
func (db *DB) CheckInOf(ctx context.Context, userID uuid.UUID, d day.Date) (DayCheckIn, error) {
	var c DayCheckIn
	err := db.readTx(ctx, func(tx *sql.Tx) error {
		var err error
		c, err = checkInOf(ctx, tx, userID, d)
		return err
	})
	if err != nil {
		return DayCheckIn{}, fmt.Errorf("check-in: %w", err)
	}
	return c, nil
}

// CheckIn stores in place of what userID says of day d what change makes
// of it, and returns it as stored. change gets the day as it is in the
// transaction that stores its answer, so check-ins sent at once each
// change the other's result. The change is recorded as a's one event,
// undone at once; a change that leaves the day as it was records nothing
// (record).
func (db *DB) CheckIn(ctx context.Context, userID uuid.UUID, a event.Actor, d day.Date,
	change func(DayCheckIn) DayCheckIn, now time.Time,
) (DayCheckIn, error) {
	if d.IsZero() {
		return DayCheckIn{}, errors.New("invalid check-in: the day is missing")
	}
	if err := a.Validate(); err != nil {
		return DayCheckIn{}, fmt.Errorf("invalid actor: %w", err)
	}
	var after DayCheckIn
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		before, err := checkInOf(ctx, tx, userID, d)
		if err != nil {
			return err
		}
		after = change(before)
		if err := after.validate(); err != nil {
			return fmt.Errorf("invalid check-in: %w", err)
		}
		if err := putCheckIn(ctx, tx, userID, d, after, now); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.DayCheckIn, d.String(), before, after, now)
		return err
	})
	if err != nil {
		return DayCheckIn{}, fmt.Errorf("check in: %w", err)
	}
	return after, nil
}

func checkInOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date) (DayCheckIn, error) {
	var mood, hunger sql.NullInt64
	var note, reason sql.NullString
	var training sql.NullBool
	err := tx.QueryRowContext(ctx, "SELECT mood, hunger, note, training_day, excluded_reason FROM days WHERE user_id = ? AND day = ?",
		userID.String(), d.String()).Scan(&mood, &hunger, &note, &training, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return DayCheckIn{}, nil
	}
	if err != nil {
		return DayCheckIn{}, err
	}
	c := DayCheckIn{Mood: int(mood.Int64), Hunger: int(hunger.Int64), Note: note.String, ExcludedReason: reason.String}
	if training.Valid {
		c.TrainingDay = &training.Bool
	}
	return c, nil
}

// putCheckIn stores c as what userID says of day d.
func putCheckIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date, c DayCheckIn, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO days (user_id, day, mood, hunger, note, training_day, excluded_reason, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, day) DO UPDATE SET mood = excluded.mood, hunger = excluded.hunger, note = excluded.note,
			training_day = excluded.training_day, excluded_reason = excluded.excluded_reason, updated_at = excluded.updated_at`,
		userID.String(), d.String(), nullIfZero(c.Mood), nullIfZero(c.Hunger), nullIfEmpty(c.Note), c.TrainingDay,
		nullIfEmpty(c.ExcludedReason), formatTime(now))
	return err
}

func nullIfZero(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func takeBackCheckIn(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	d, err := entityDay(e.Entity)
	if err != nil {
		return err
	}
	var c DayCheckIn
	if e.Before != nil {
		if err := decode(e.Before, &c); err != nil {
			return err
		}
	}
	return putCheckIn(ctx, tx, userID, d, c, now)
}
