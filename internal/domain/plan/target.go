package plan

import (
	"errors"
	"fmt"
	"slices"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// Kcal a gram of protein, carbs and fat gives, the Atwater factors.
const (
	kcalPerGramProtein = 4
	kcalPerGramCarbs   = 4
	kcalPerGramFat     = 9
)

// MaxKcal bounds a day's target, a guard against a typo (25000 for
// 2500), not a limit of a plan: the most a person can eat in a day, as
// the metrics bound the energy a device reports.
const MaxKcal = 15000

// DayTarget is what to eat on one type of day: energy, in kcal, and
// grams of protein and fat. Carbs are what the energy leaves, so a step
// of the kcal moves the carbs and the share of fat, and keeps the grams
// of protein and fat (docs/limits.md: a kcal step sets the fat share).
type DayTarget struct {
	Kcal    float64
	Protein float64
	Fat     float64
}

// Carbs returns the grams of carbs the target leaves: the energy that is
// not protein's or fat's, at 4 kcal a gram.
func (t DayTarget) Carbs() float64 {
	return (t.Kcal - kcalPerGramProtein*t.Protein - kcalPerGramFat*t.Fat) / kcalPerGramCarbs
}

// FatShare returns the share of the day's energy fat gives, from 0 to 1.
func (t DayTarget) FatShare() float64 { return kcalPerGramFat * t.Fat / t.Kcal }

// Validate reports every rule of its shape t breaks, joined into one
// error: kcal more than 0 and at most MaxKcal, grams of protein and fat
// of 0 or more, and no more energy in protein and fat than the day's. The
// floors and ceilings of a plan are limits, not rules of the shape.
func (t DayTarget) Validate() error {
	var errs []error
	if !finite(t.Kcal) || t.Kcal <= 0 || t.Kcal > MaxKcal {
		errs = append(errs, fmt.Errorf("kcal %v: want more than 0 and at most %d", t.Kcal, MaxKcal))
	}
	for _, g := range []struct {
		name string
		v    float64
	}{{"protein", t.Protein}, {"fat", t.Fat}} {
		if !finite(g.v) || g.v < 0 {
			errs = append(errs, fmt.Errorf("%s %v g: want a number, zero or more", g.name, g.v))
		}
	}
	if len(errs) == 0 && t.Carbs() < 0 {
		errs = append(errs, fmt.Errorf("protein and fat give %v kcal, more than the day's %v",
			kcalPerGramProtein*t.Protein+kcalPerGramFat*t.Fat, t.Kcal))
	}
	return errors.Join(errs...)
}

// Target is the plan's target from a day on, until the next one takes
// over: one for training days and one for rest days. An unsplit target
// has the same for both.
type Target struct {
	EffectiveFrom day.Date
	Training      DayTarget
	Rest          DayTarget
}

// For returns the target of a day of type dt.
func (t Target) For(dt DayType) DayTarget {
	if dt == Training {
		return t.Training
	}
	return t.Rest
}

// WeeklyAverage returns the average day of a week with trainingDays
// training days and the rest rest days: each day type weighed by how
// many days of the week it has. trainingDays is cut to 0 to 7.
func (t Target) WeeklyAverage(trainingDays int) DayTarget {
	n := float64(min(max(trainingDays, 0), 7))
	avg := func(training, rest float64) float64 { return (n*training + (7-n)*rest) / 7 }
	return DayTarget{
		Kcal:    avg(t.Training.Kcal, t.Rest.Kcal),
		Protein: avg(t.Training.Protein, t.Rest.Protein),
		Fat:     avg(t.Training.Fat, t.Rest.Fat),
	}
}

// ShiftKcal returns t with the weekly average moved by delta kcal a day,
// whatever the number of training days: both day types move by delta, so
// the difference between them stays as it is (DESIGN.md §7). Protein and
// fat keep their grams; the carbs take the change.
func (t Target) ShiftKcal(delta float64) Target {
	t.Training.Kcal += delta
	t.Rest.Kcal += delta
	return t
}

// Validate reports a target without a day it takes effect, and every
// rule a day type's target breaks.
func (t Target) Validate() error {
	var errs []error
	if t.EffectiveFrom.IsZero() {
		errs = append(errs, errors.New("a target needs the day it takes effect"))
	}
	if err := t.Training.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("training days: %w", err))
	}
	if err := t.Rest.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("rest days: %w", err))
	}
	return errors.Join(errs...)
}

// ValidateHistory checks a user's targets together: each valid, and each
// taking effect after the one before.
func ValidateHistory(targets []Target) error {
	var errs []error
	for i, t := range targets {
		if err := t.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("target %d: %w", i+1, err))
		}
		if i > 0 && !targets[i-1].EffectiveFrom.Before(t.EffectiveFrom) {
			errs = append(errs, fmt.Errorf("target %d takes effect on %s, not after target %d's %s",
				i+1, t.EffectiveFrom, i, targets[i-1].EffectiveFrom))
		}
	}
	return errors.Join(errs...)
}

// TargetOn returns the target in effect on day d, from a history that
// passes ValidateHistory: the last to take effect on or before d. The
// second result is false before the first.
func TargetOn(targets []Target, d day.Date) (Target, bool) {
	for _, t := range slices.Backward(targets) {
		if !d.Before(t.EffectiveFrom) {
			return t, true
		}
	}
	return Target{}, false
}

// DayTargetOn returns the target of day d of type dt: the target in
// effect on d, for its type. The second result is false before the first
// target. With the type from TypeOf, a day's target is known from the
// plan alone.
func DayTargetOn(targets []Target, d day.Date, dt DayType) (DayTarget, bool) {
	t, ok := TargetOn(targets, d)
	if !ok {
		return DayTarget{}, false
	}
	return t.For(dt), true
}

// ProteinGrams returns the grams of protein perKg g/kg of refKg gives.
// The target is kept in grams; its limits are in g/kg of the reference
// mass (profile.Profile.ProteinReferenceMass).
func ProteinGrams(perKg, refKg float64) float64 { return perKg * refKg }

// ProteinPerKg returns the g/kg of refKg that grams of protein are.
func ProteinPerKg(grams, refKg float64) float64 { return grams / refKg }
