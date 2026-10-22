package api

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// dayOf is the day a path names for p's user: a date, or today, the
// user's day at now.
func dayOf(p auth.Principal, date string, now time.Time) (day.Date, error) {
	_, today, err := userDay(p, now)
	if err != nil {
		return day.Date{}, internalError("reading the day failed", err)
	}
	if date == "today" {
		return today, nil
	}
	d, err := day.ParseDate(date)
	if err != nil {
		return day.Date{}, huma.Error422UnprocessableEntity("the date: " + err.Error())
	}
	return d, nil
}

// dayChangeOutput is the answer of a tool that changes a day: the day as
// it stands now.
type dayChangeOutput struct {
	Body dayBody
}

// changedDay answers with day d of p's user as it stands at now; failed
// is what the tool says when that fails.
func (s *sessions) changedDay(ctx context.Context, p auth.Principal, d day.Date, now time.Time, failed string) (*dayChangeOutput, error) {
	b, err := s.readDay(ctx, p, d, now)
	if err != nil {
		return nil, internalError(failed, err)
	}
	return &dayChangeOutput{Body: b}, nil
}

type metricInput struct {
	Date string `path:"date" pattern:"^(today|[0-9]{4}-[0-9]{2}-[0-9]{2})$" example:"today" doc:"The day, YYYY-MM-DD, or today: the user's day now, by their day's start hour."`
	Kind string `path:"kind" enum:"weight,body_fat,lean_mass,steps,active_kcal,basal_kcal,sleep_h,rhr,hrv,vo2max,exercise_min"`
}

type logMetricInput struct {
	Date string `path:"date" pattern:"^(today|[0-9]{4}-[0-9]{2}-[0-9]{2})$" example:"today" doc:"The day, YYYY-MM-DD, or today: the user's day now, by their day's start hour."`
	Kind string `path:"kind" enum:"weight,body_fat,lean_mass,steps,active_kcal,basal_kcal,sleep_h,rhr,hrv,vo2max,exercise_min"`
	Body struct {
		Value float64 `json:"value" doc:"In the kind's unit: weight and lean_mass kg, body_fat %, steps a count, active_kcal and basal_kcal kcal, sleep_h hours, rhr bpm, hrv ms, vo2max ml/kg/min, exercise_min minutes."`
	}
}

func (s *sessions) logMetric(ctx context.Context, in *logMetricInput) (*dayChangeOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	d, err := dayOf(p, in.Date, now)
	if err != nil {
		return nil, err
	}
	if _, today, err := userDay(p, now); err != nil {
		return nil, internalError("logging the metric failed", err)
	} else if today.Before(d) {
		return nil, huma.Error422UnprocessableEntity("the date: " + d.String() + " is a day to come; a measurement is of today or a day before")
	}
	v := metric.Value{Kind: metric.Kind(in.Kind), Value: in.Body.Value, Source: metric.Manual}
	if err := v.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity(oneLine(err))
	}
	if _, err := s.db.SetMetric(ctx, p.User.ID, actorOf(p), d, v, now); err != nil {
		return nil, internalError("logging the metric failed", err)
	}
	return s.changedDay(ctx, p, d, now, "logging the metric failed")
}

func (s *sessions) deleteMetric(ctx context.Context, in *metricInput) (*dayChangeOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	d, err := dayOf(p, in.Date, now)
	if err != nil {
		return nil, err
	}
	_, err = s.db.DeleteMetric(ctx, p.User.ID, actorOf(p), d, metric.Kind(in.Kind), now)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no " + in.Kind + " on " + d.String())
	}
	if err != nil {
		return nil, internalError("deleting the metric failed", err)
	}
	return s.changedDay(ctx, p, d, now, "deleting the metric failed")
}

type checkInInput struct {
	Date string `path:"date" pattern:"^(today|[0-9]{4}-[0-9]{2}-[0-9]{2})$" example:"today" doc:"The day, YYYY-MM-DD, or today: the user's day now, by their day's start hour."`
	Body checkInBody
}

// checkInBody is what check-in changes of a day: each field given, and
// only those.
type checkInBody struct {
	Mood           *int    `json:"mood,omitempty" minimum:"0" maximum:"5" doc:"The user's mood, 1 (low) to 5 (high); 0 clears it."`
	Hunger         *int    `json:"hunger,omitempty" minimum:"0" maximum:"5" doc:"How hungry the user was, 1 (not at all) to 5 (very); 0 clears it."`
	Note           *string `json:"note,omitempty" maxLength:"2000" doc:"What the user says of the day; empty clears it."`
	TrainingDay    *string `json:"training_day,omitempty" enum:"training,rest,plan" doc:"training or rest sets the day's type, which picks its target; plan leaves it to the training weekdays."`
	ExcludedReason *string `json:"excluded_reason,omitempty" maxLength:"200" doc:"Takes the day out of calibration for this reason (restaurant, sick, forgot to log); empty brings it back."`
}

func (s *sessions) checkIn(ctx context.Context, in *checkInInput) (*dayChangeOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	d, err := dayOf(p, in.Date, now)
	if err != nil {
		return nil, err
	}
	b := &in.Body
	if err := checkCheckIn(b); err != nil {
		return nil, err
	}
	// A day to come takes its type and its exclusion, planned; what the
	// user feels is of a day begun, so a date sent for it is not taken.
	if _, today, err := userDay(p, now); err != nil {
		return nil, internalError("checking in failed", err)
	} else if today.Before(d) && (b.Mood != nil || b.Hunger != nil || b.Note != nil) {
		return nil, huma.Error422UnprocessableEntity("the date: " + d.String() + " is a day to come; a mood, hunger or note is of today or a day before")
	}
	// The change is made to the day as it is when it is stored, so
	// check-ins sent at once each keep what they say.
	if _, err := s.db.CheckIn(ctx, p.User.ID, actorOf(p), d, func(c store.DayCheckIn) store.DayCheckIn {
		return b.applyTo(c)
	}, now); err != nil {
		return nil, internalError("checking in failed", err)
	}
	return s.changedDay(ctx, p, d, now, "checking in failed")
}

// checkCheckIn refuses what b says that a day cannot hold.
func checkCheckIn(b *checkInBody) error {
	var errs []error
	if b.Note != nil {
		errs = append(errs, day.CheckIn{Note: *b.Note}.Validate())
	}
	if b.ExcludedReason != nil && *b.ExcludedReason != "" {
		errs = append(errs, day.ValidateExclusion(day.Exclusion{Reason: *b.ExcludedReason}))
	}
	if err := errors.Join(errs...); err != nil {
		return huma.Error422UnprocessableEntity(oneLine(err))
	}
	return nil
}

// applyTo is c with what b says in place of what c says.
func (b *checkInBody) applyTo(c store.DayCheckIn) store.DayCheckIn {
	if b.Mood != nil {
		c.Mood = *b.Mood
	}
	if b.Hunger != nil {
		c.Hunger = *b.Hunger
	}
	if b.Note != nil {
		c.Note = *b.Note
	}
	if b.TrainingDay != nil {
		switch *b.TrainingDay {
		case "training":
			c.TrainingDay = new(true)
		case "rest":
			c.TrainingDay = new(false)
		default:
			c.TrainingDay = nil
		}
	}
	if b.ExcludedReason != nil {
		c.ExcludedReason = *b.ExcludedReason
	}
	return c
}
