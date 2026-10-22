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
// typo would leave a value out, and so is anything after the plan.
func ReadPlan(r io.Reader) (PlanFile, error) {
	var f PlanFile
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return PlanFile{}, fmt.Errorf("the plan file: %w", err)
	}
	var more json.RawMessage
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		return PlanFile{}, errors.New("the plan file: something after the plan")
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
// and targets in order: a timeline and a history as the store keeps them.
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
		p.phases = append(p.phases, ph)
	}
	for _, x := range f.Targets {
		t := plan.Target{
			Training: plan.DayTarget{Kcal: x.Training.Kcal, Protein: x.Training.Protein, Fat: x.Training.Fat},
			Rest:     plan.DayTarget{Kcal: x.Rest.Kcal, Protein: x.Rest.Protein, Fat: x.Rest.Fat},
		}
		var err error
		t.EffectiveFrom, err = day.ParseDate(x.EffectiveFrom)
		errs = append(errs, err)
		p.targets = append(p.targets, t)
	}
	slices.SortFunc(p.phases, func(a, b plan.Phase) int { return strings.Compare(a.Start.String(), b.Start.String()) })
	slices.SortFunc(p.targets, func(a, b plan.Target) int { return strings.Compare(a.EffectiveFrom.String(), b.EffectiveFrom.String()) })
	errs = append(errs, plan.ValidateTimeline(p.phases), plan.ValidateHistory(p.targets))
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
	// Kept names what the user had already, which stays: their own plan
	// (a phase or a target no import before wrote), training weekdays, a
	// complete profile, notes.
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

// PlanLimits are what ImportPlan holds a plan to: the instance's limits,
// and weigh-ins the days of a dry run would bring, by their day, which
// the store does not have yet.
type PlanLimits struct {
	Instance limits.Limits
	Weighed  map[day.Date]float64
}

// ImportPlan writes f into u's plan, or with dryRun says what it would
// write. What the user has stays theirs: a plan of their own (a phase or
// a target an import before did not write), training weekdays, the
// fields of their profile (the file fills the others), notes; a plan an
// import before wrote in part is written whole. Every phase and target
// from the current ones (the ones today falls in) on, the future ones
// too, are held to the limits first (the instance's, and the user's own
// from the store), by the profile and the trend weight of the last week
// of weigh-ins, each target against the one before it; a violation
// refuses the whole plan, and nothing is written. The phases and targets
// before the current ones are history, written as they were.
func ImportPlan(ctx context.Context, db *store.DB, u user.User, f PlanFile, lim PlanLimits, now time.Time, dryRun bool) (PlanReport, error) {
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
	have, err := existing(ctx, db, u, p, &r)
	if err != nil {
		return r, err
	}
	if !have.plan {
		if err := hold(ctx, db, u, p, have, today, lim, &r); err != nil {
			return r, err
		}
	}
	r.Written = written(p, have)
	if dryRun {
		return r, nil
	}
	return r, write(ctx, db, u, p, have, today, now)
}

// had is what the user has already, and the profile the file's fills.
type had struct {
	plan, weekdays, notes bool
	profile, merged       profile.Profile
	// phases and targets are the stored ones, of an import before.
	phases  []plan.Phase
	targets []plan.Target
}

func existing(ctx context.Context, db *store.DB, u user.User, p thePlan, r *PlanReport) (had, error) {
	var h had
	phases, err := db.Phases(ctx, u.ID)
	if err != nil {
		return h, err
	}
	targets, err := db.Targets(ctx, u.ID)
	if err != nil {
		return h, err
	}
	w, err := db.TrainingWeekdays(ctx, u.ID)
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
	h.plan = !inPlan(phases, targets, p)
	h.phases, h.targets = phases, targets
	h.weekdays, h.notes = w != 0, notes != ""
	h.merged = h.profile
	if p.profile != nil {
		h.merged = fill(h.profile, *p.profile)
	}
	for _, k := range []struct {
		kept bool
		name string
	}{{h.plan, "plan"}, {h.weekdays, "training weekdays"}, {h.profile.Complete(), "profile"}, {h.notes, "notes"}} {
		if k.kept {
			r.Kept = append(r.Kept, k.name)
		}
	}
	return h, nil
}

// inPlan reports whether every phase and target stored is one of p's, as
// an import of p before wrote them: none, or a part of p.
func inPlan(phases []plan.Phase, targets []plan.Target, p thePlan) bool {
	for _, ph := range phases {
		if !slices.ContainsFunc(p.phases, func(x plan.Phase) bool { return samePhase(x, ph) }) {
			return false
		}
	}
	for _, t := range targets {
		if !slices.Contains(p.targets, t) {
			return false
		}
	}
	return true
}

func samePhase(a, b plan.Phase) bool {
	same := func(x, y *float64) bool { return x == nil && y == nil || x != nil && y != nil && *x == *y }
	return a.Type == b.Type && a.Start == b.Start && a.End == b.End && a.RatePctWeek == b.RatePctWeek &&
		same(a.GoalWeight, b.GoalWeight) && same(a.MilestoneWeight, b.MilestoneWeight)
}

// fill returns mine with the fields it lacks from the file's.
func fill(mine, file profile.Profile) profile.Profile {
	if mine.Sex == "" {
		mine.Sex = file.Sex
	}
	if mine.BirthDate.IsZero() {
		mine.BirthDate = file.BirthDate
	}
	if mine.HeightCm == 0 {
		mine.HeightCm = file.HeightCm
	}
	return mine
}

// hold holds the phases and targets from the current ones on, and the
// training weekdays, to the limits.
func hold(ctx context.Context, db *store.DB, u user.User, p thePlan, h had, today day.Date, lim PlanLimits, r *PlanReport) error {
	trend, err := trendKg(ctx, db, u, today, lim.Weighed)
	if err != nil {
		return err
	}
	userLimits, err := db.UserLimits(ctx, u.ID)
	if err != nil {
		return err
	}
	levels := limits.Levels{Instance: lim.Instance, User: userLimits}
	s := limits.State{Today: today, Profile: h.merged, TrendKg: trend}
	weekdays := p.weekdays
	if h.weekdays || h.plan {
		weekdays = nil
	}
	if weekdays != nil {
		s.TrainingDays = *weekdays
	}
	var changes []limits.Change
	for _, ph := range fromCurrent(p.phases, today, func(ph plan.Phase) day.Date { return ph.Start }, func(ph plan.Phase) bool { return ph.On(today) }) {
		changes = append(changes, limits.Change{Phase: &ph, By: limits.ByPlan})
	}
	targets := fromCurrent(p.targets, today, func(t plan.Target) day.Date { return t.EffectiveFrom },
		func(t plan.Target) bool { cur, ok := plan.TargetOn(p.targets, today); return ok && cur == t })
	var violations []limits.Violation
	for i, t := range targets {
		c := limits.Change{Target: &t, By: limits.ByPlan}
		st := s
		if i == 0 {
			c.TrainingDays = weekdays
		}
		if before := slices.Index(p.targets, t); before > 0 {
			st.Target = &p.targets[before-1]
		}
		res := limits.Validate(c, st, levels)
		violations = append(violations, res.Violations...)
		r.Warnings = append(r.Warnings, res.Warnings...)
	}
	for _, c := range changes {
		res := limits.Validate(c, s, levels)
		violations = append(violations, res.Violations...)
		r.Warnings = append(r.Warnings, res.Warnings...)
	}
	if len(violations) > 0 {
		return &PlanRefusedError{Violations: violations}
	}
	return nil
}

// fromCurrent returns the entries of xs from the current one (the one
// current reports) on, or those after today when none is current: the
// ones the limits hold.
func fromCurrent[T any](xs []T, today day.Date, start func(T) day.Date, current func(T) bool) []T {
	for i, x := range xs {
		if current(x) || today.Before(start(x)) {
			return xs[i:]
		}
	}
	return nil
}

// trendKg stands in for the trend weight until calibration has its own:
// the mean of the weigh-ins of the last week up to today that has any,
// within the last 30 days, of the store's and the ones of weighed on days
// the store has none of; 0 for none.
func trendKg(ctx context.Context, db *store.DB, u user.User, today day.Date, weighed map[day.Date]float64) (float64, error) {
	ms, err := db.Metrics(ctx, u.ID, today.AddDays(-30), today)
	if err != nil {
		return 0, err
	}
	kg := map[day.Date]float64{}
	for d, w := range weighed {
		if !d.Before(today.AddDays(-30)) && !today.Before(d) {
			kg[d] = w
		}
	}
	for d, vs := range ms {
		for _, v := range vs {
			if v.Kind == metric.Weight {
				kg[d] = v.Value
			}
		}
	}
	var last day.Date
	for d := range kg {
		if last.Before(d) {
			last = d
		}
	}
	var sum float64
	var n int
	for d, w := range kg {
		if !d.Before(last.AddDays(-6)) {
			sum += w
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return sum / float64(n), nil
}

// written names what a write of p writes, by what the user has: the
// training weekdays only with a plan, as they move its weekly average,
// and of the plan what an import before has not written.
func written(p thePlan, h had) []string {
	var out []string
	if h.merged != h.profile {
		out = append(out, "profile")
	}
	if p.weekdays != nil && !h.weekdays && !h.plan {
		out = append(out, "training weekdays")
	}
	if !h.plan {
		for _, ph := range p.phases {
			if !slices.ContainsFunc(h.phases, func(x plan.Phase) bool { return samePhase(x, ph) }) {
				out = append(out, "phase from "+ph.Start.String())
			}
		}
		for _, t := range p.targets {
			if !slices.Contains(h.targets, t) {
				out = append(out, "target from "+t.EffectiveFrom.String())
			}
		}
	}
	if p.notes != "" && !h.notes {
		out = append(out, "notes")
	}
	return out
}

func write(ctx context.Context, db *store.DB, u user.User, p thePlan, h had, today day.Date, now time.Time) error {
	if h.merged != h.profile {
		if err := db.SetProfile(ctx, u.ID, Actor, h.merged, today, now); err != nil {
			return fmt.Errorf("the profile: %w", err)
		}
	}
	if p.weekdays != nil && !h.weekdays && !h.plan {
		if err := db.SetTrainingWeekdays(ctx, u.ID, Actor, *p.weekdays, now); err != nil {
			return fmt.Errorf("the training weekdays: %w", err)
		}
	}
	if !h.plan {
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
