package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// PlanFile is a user's plan as a file of theirs gives it (JSON), written
// from the predecessor's goals note, which is prose: the profile, the
// training weekdays, the phases, the targets with their history, and the
// notes for a coach.
type PlanFile struct {
	Profile *struct {
		Sex       string  `json:"sex"`
		BirthDate string  `json:"birth_date"`
		HeightCm  float64 `json:"height_cm"`
	} `json:"profile"`
	// TrainingWeekdays are days of the week, 0 Sunday to 6 Saturday.
	TrainingWeekdays []int `json:"training_weekdays"`
	Phases           []struct {
		Start           string   `json:"start"`
		End             string   `json:"end"`
		Type            string   `json:"type"`
		RatePctWeek     float64  `json:"rate_pct_week"`
		GoalWeight      *float64 `json:"goal_weight"`
		MilestoneWeight *float64 `json:"milestone_weight"`
	} `json:"phases"`
	Targets []struct {
		EffectiveFrom string        `json:"effective_from"`
		Training      planDayTarget `json:"training"`
		Rest          planDayTarget `json:"rest"`
	} `json:"targets"`
	Notes string `json:"notes"`
}

type planDayTarget struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
}

// ReadPlan reads a plan file; a field it does not know is an error, as a
// typo would leave a value out.
func ReadPlan(r io.Reader) (PlanFile, error) {
	var f PlanFile
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return PlanFile{}, fmt.Errorf("the plan file: %w", err)
	}
	return f, nil
}

// thePlan is a plan file as the domain's types.
type thePlan struct {
	profile  *profile.Profile
	weekdays *plan.Weekdays
	phases   []plan.Phase
	targets  []plan.Target
	notes    string
}

// parse checks f's shape and returns it as the domain's types, the phases
// and targets in order.
func (f PlanFile) parse(today day.Date) (thePlan, error) {
	var p thePlan
	var errs []error
	if f.Profile != nil {
		birth, err := day.ParseDate(f.Profile.BirthDate)
		pr := profile.Profile{Sex: profile.Sex(f.Profile.Sex), BirthDate: birth, HeightCm: f.Profile.HeightCm}
		errs = append(errs, err, pr.Validate(today))
		p.profile = &pr
	}
	if f.TrainingWeekdays != nil {
		ds := make([]time.Weekday, len(f.TrainingWeekdays))
		for i, d := range f.TrainingWeekdays {
			ds[i] = time.Weekday(d)
		}
		w, err := plan.WeekdaysOf(ds...)
		errs = append(errs, err)
		p.weekdays = &w
	}
	for _, x := range f.Phases {
		ph := plan.Phase{Type: plan.PhaseType(x.Type), RatePctWeek: x.RatePctWeek, GoalWeight: x.GoalWeight, MilestoneWeight: x.MilestoneWeight}
		var err error
		ph.Start, err = day.ParseDate(x.Start)
		errs = append(errs, err)
		if x.End != "" {
			ph.End, err = day.ParseDate(x.End)
			errs = append(errs, err)
		}
		errs = append(errs, ph.Validate())
		p.phases = append(p.phases, ph)
	}
	for _, x := range f.Targets {
		t := plan.Target{
			Training: plan.DayTarget{Kcal: x.Training.Kcal, Protein: x.Training.Protein, Fat: x.Training.Fat},
			Rest:     plan.DayTarget{Kcal: x.Rest.Kcal, Protein: x.Rest.Protein, Fat: x.Rest.Fat},
		}
		var err error
		t.EffectiveFrom, err = day.ParseDate(x.EffectiveFrom)
		errs = append(errs, err, t.Validate())
		p.targets = append(p.targets, t)
	}
	slices.SortFunc(p.phases, func(a, b plan.Phase) int { return strings.Compare(a.Start.String(), b.Start.String()) })
	slices.SortFunc(p.targets, func(a, b plan.Target) int { return strings.Compare(a.EffectiveFrom.String(), b.EffectiveFrom.String()) })
	p.notes = strings.TrimSpace(f.Notes)
	if err := errors.Join(errs...); err != nil {
		return thePlan{}, fmt.Errorf("the plan file: %w", err)
	}
	return p, nil
}

// PlanReport is what an import of a plan did, or would do.
type PlanReport struct {
	// Written names what was written: "profile", "training weekdays",
	// each phase and target by its first day, "notes".
	Written []string
	// Kept names what the user had already, which stays: the plan as a
	// whole (any phase or target), the profile, the notes.
	Kept []string
	// Warnings are what the limits warn of, without refusing.
	Warnings []limits.Warning
}

// PlanRefusedError reports a plan the limits refuse; nothing is written.
type PlanRefusedError struct{ Violations []limits.Violation }

func (e *PlanRefusedError) Error() string {
	var b strings.Builder
	b.WriteString("the limits refuse the plan:")
	for _, v := range e.Violations {
		fmt.Fprintf(&b, " %s", v.Rule)
		if v.DayType != "" {
			fmt.Fprintf(&b, " (%s day)", v.DayType)
		}
		if v.Level != "" {
			fmt.Fprintf(&b, ": %v, the limit %v (%s);", v.Proposed, v.Limit, v.Level)
		} else {
			b.WriteString(";")
		}
	}
	return strings.TrimSuffix(b.String(), ";")
}

// ImportPlan writes f into u's plan, or with dryRun says what it would
// write. A user who has a plan already (a phase or a target) keeps it,
// and the file's plan is left out; their profile and their notes stay
// theirs too. The current phase and target (the ones today falls in) and
// the training weekdays are held to the limits (instance, the instance's;
// the user's own from the store) first, by the profile and the trend
// weight of the last week of weigh-ins, and a violation refuses the whole
// plan; the phases and targets before them are history, and are written
// as they were.
func ImportPlan(ctx context.Context, db *store.DB, u user.User, f PlanFile, instance limits.Limits, now time.Time, dryRun bool) (PlanReport, error) {
	var r PlanReport
	cal, err := day.NewCalendar(u.TimeZone, u.DayStartHour)
	if err != nil {
		return r, fmt.Errorf("the user's calendar: %w", err)
	}
	today := cal.DayOf(now)
	p, err := f.parse(today)
	if err != nil {
		return r, err
	}
	have, err := existing(ctx, db, u, &r)
	if err != nil {
		return r, err
	}
	if !have.plan {
		if err := hold(ctx, db, u, p, have.profile, today, instance, &r); err != nil {
			return r, err
		}
	}
	if dryRun {
		r.Written = written(p, have)
		return r, nil
	}
	err = write(ctx, db, u, p, have, today, now, &r)
	return r, err
}

// what the user has already.
type had struct {
	plan, notes bool
	profile     profile.Profile
}

func existing(ctx context.Context, db *store.DB, u user.User, r *PlanReport) (had, error) {
	var h had
	phases, err := db.Phases(ctx, u.ID)
	if err != nil {
		return h, err
	}
	targets, err := db.Targets(ctx, u.ID)
	if err != nil {
		return h, err
	}
	if h.profile, err = db.Profile(ctx, u.ID); err != nil {
		return h, err
	}
	notes, err := db.ProfileNotes(ctx, u.ID)
	if err != nil {
		return h, err
	}
	h.plan, h.notes = len(phases) > 0 || len(targets) > 0, notes != ""
	if h.plan {
		r.Kept = append(r.Kept, "plan")
	}
	if h.profile.Complete() {
		r.Kept = append(r.Kept, "profile")
	}
	if h.notes {
		r.Kept = append(r.Kept, "notes")
	}
	return h, nil
}

// hold holds the current phase and target, and the training weekdays, to
// the limits.
func hold(ctx context.Context, db *store.DB, u user.User, p thePlan, mine profile.Profile, today day.Date, instance limits.Limits,
	r *PlanReport,
) error {
	pr := mine
	if !pr.Complete() && p.profile != nil {
		pr = *p.profile
	}
	trend, err := trendKg(ctx, db, u, today)
	if err != nil {
		return err
	}
	userLimits, err := db.UserLimits(ctx, u.ID)
	if err != nil {
		return err
	}
	c := limits.Change{By: limits.ByPlan, TrainingDays: p.weekdays}
	if ph, ok := plan.PhaseOn(p.phases, today); ok {
		c.Phase = &ph
	}
	if t, ok := plan.TargetOn(p.targets, today); ok {
		c.Target = &t
	}
	s := limits.State{Today: today, Profile: pr, TrendKg: trend}
	if p.weekdays != nil {
		s.TrainingDays = *p.weekdays
	}
	res := limits.Validate(c, s, limits.Levels{Instance: instance, User: userLimits})
	if !res.OK() {
		return &PlanRefusedError{Violations: res.Violations}
	}
	r.Warnings = res.Warnings
	return nil
}

// trendKg stands in for the trend weight until calibration has its own:
// the mean of the weigh-ins of the last week before today that has any,
// within the last 30 days; 0 for none.
func trendKg(ctx context.Context, db *store.DB, u user.User, today day.Date) (float64, error) {
	ms, err := db.Metrics(ctx, u.ID, today.AddDays(-30), today)
	if err != nil {
		return 0, err
	}
	var last day.Date
	for d, vs := range ms {
		if slices.ContainsFunc(vs, func(v metric.Value) bool { return v.Kind == metric.Weight }) && last.Before(d) {
			last = d
		}
	}
	var sum float64
	var n int
	for d, vs := range ms {
		if d.Before(last.AddDays(-6)) {
			continue
		}
		for _, v := range vs {
			if v.Kind == metric.Weight {
				sum += v.Value
				n++
			}
		}
	}
	if n == 0 {
		return 0, nil
	}
	return sum / float64(n), nil
}

// written names what a write of p would write, by what the user has.
func written(p thePlan, h had) []string {
	var out []string
	if p.profile != nil && !h.profile.Complete() {
		out = append(out, "profile")
	}
	if !h.plan {
		if p.weekdays != nil {
			out = append(out, "training weekdays")
		}
		for _, ph := range p.phases {
			out = append(out, "phase from "+ph.Start.String())
		}
		for _, t := range p.targets {
			out = append(out, "target from "+t.EffectiveFrom.String())
		}
	}
	if p.notes != "" && !h.notes {
		out = append(out, "notes")
	}
	return out
}

func write(ctx context.Context, db *store.DB, u user.User, p thePlan, h had, today day.Date, now time.Time, r *PlanReport) error {
	r.Written = written(p, h)
	if p.profile != nil && !h.profile.Complete() {
		if err := db.SetProfile(ctx, u.ID, Actor, *p.profile, today, now); err != nil {
			return fmt.Errorf("the profile: %w", err)
		}
	}
	if !h.plan {
		if p.weekdays != nil {
			if err := db.SetTrainingWeekdays(ctx, u.ID, Actor, *p.weekdays, now); err != nil {
				return fmt.Errorf("the training weekdays: %w", err)
			}
		}
		for _, ph := range p.phases {
			if err := db.SetPhase(ctx, u.ID, Actor, ph, now); err != nil {
				return fmt.Errorf("the phase from %s: %w", ph.Start, err)
			}
		}
		for _, t := range p.targets {
			if err := db.SetTarget(ctx, u.ID, Actor, t, now); err != nil {
				return fmt.Errorf("the target from %s: %w", t.EffectiveFrom, err)
			}
		}
	}
	if p.notes != "" && !h.notes {
		if err := db.SetProfileNotes(ctx, u.ID, Actor, p.notes, now); err != nil {
			return fmt.Errorf("the notes: %w", err)
		}
	}
	return nil
}
