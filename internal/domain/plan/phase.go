// Package plan defines the user's plan: the phases of the goal timeline
// with their target rates, the targets by type of day with their weekly
// average, and the type of each day. It holds the rules of their shape
// and their arithmetic; the limits a plan must keep, and Validate, are
// another package's (docs/limits.md).
package plan

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// PhaseType is the kind of a phase. Nothing assumes a loss: each type has
// a target rate of its own sign.
type PhaseType string

// Phase types.
const (
	// Cut loses weight: a rate of 0 or less.
	Cut PhaseType = "cut"
	// Maintain holds the weight: a rate of 0. A diet break or a week of
	// maintenance in a deload is a short Maintain phase.
	Maintain PhaseType = "maintain"
	// Bulk gains weight: a rate of 0 or more.
	Bulk PhaseType = "bulk"
	// Recomp stays near the weight, to lose fat and gain lean mass: a
	// rate of either sign, in a range of its own (docs/limits.md).
	Recomp PhaseType = "recomp"
)

// PhaseTypes lists every phase type, in a fixed order.
func PhaseTypes() []PhaseType { return []PhaseType{Cut, Maintain, Bulk, Recomp} }

// Phase is one stretch of the user's goal timeline.
type Phase struct {
	Type PhaseType
	// Start is the phase's first day.
	Start day.Date
	// End is its last day, when it has a planned one; the zero Date leaves
	// it open, until the next phase starts.
	End day.Date
	// RatePctWeek is the target rate in % of the trend weight a week, with
	// its sign: a minus is a loss.
	RatePctWeek float64
	// GoalWeight and MilestoneWeight, in kg, are optional. They may lie
	// above or below the weight now, whatever the phase.
	GoalWeight      *float64
	MilestoneWeight *float64
}

// Validate reports every rule of its shape p breaks, joined into one
// error: a known type, a start, an end not before the start, a rate that
// is a number of the type's sign (0 for Maintain), and a goal and a
// milestone weight a person can have. How fast a rate may be is a limit,
// not a rule of the shape.
func (p Phase) Validate() error {
	var errs []error
	if !slices.Contains(PhaseTypes(), p.Type) {
		errs = append(errs, fmt.Errorf("phase type %q: want one of %v", p.Type, PhaseTypes()))
	}
	if p.Start.IsZero() {
		errs = append(errs, errors.New("a phase needs a start"))
	}
	if !p.End.IsZero() && p.End.Before(p.Start) {
		errs = append(errs, fmt.Errorf("the phase ends on %s, before it starts on %s", p.End, p.Start))
	}
	if err := p.validateRate(); err != nil {
		errs = append(errs, err)
	}
	least, most := metric.Weight.Bounds()
	for _, w := range []struct {
		name string
		v    *float64
	}{{"goal", p.GoalWeight}, {"milestone", p.MilestoneWeight}} {
		if w.v != nil && (!finite(*w.v) || *w.v < least || *w.v > most) {
			errs = append(errs, fmt.Errorf("%s weight %v: want %v to %v kg", w.name, *w.v, least, most))
		}
	}
	return errors.Join(errs...)
}

// validateRate checks the rate is a number of the type's sign; an
// unknown type has none.
func (p Phase) validateRate() error {
	switch r := p.RatePctWeek; {
	case !finite(r):
		return fmt.Errorf("rate %v: want a number", r)
	case p.Type == Cut && r > 0:
		return fmt.Errorf("rate %v %% a week: a cut's is 0 or a loss, less than 0", r)
	case p.Type == Bulk && r < 0:
		return fmt.Errorf("rate %v %% a week: a bulk's is 0 or a gain, more than 0", r)
	case p.Type == Maintain && r != 0:
		return fmt.Errorf("rate %v %% a week: maintenance's is 0; another rate is a cut, a bulk or a recomp", r)
	}
	return nil
}

// On reports whether day d lies in p: from its start to its end, both
// included, or on from its start when it has none.
func (p Phase) On(d day.Date) bool {
	return !d.Before(p.Start) && (p.End.IsZero() || !p.End.Before(d))
}

// ValidateTimeline checks a user's phases together: each valid, in the
// order they start, none starting on or before the day the one before
// starts, and none starting before the planned end of the one before. A
// phase without an end lasts until the next one starts.
func ValidateTimeline(phases []Phase) error {
	var errs []error
	for i, p := range phases {
		if err := p.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("phase %d: %w", i+1, err))
		}
		if i == 0 {
			continue
		}
		prev := phases[i-1]
		switch {
		case !prev.Start.Before(p.Start):
			errs = append(errs, fmt.Errorf("phase %d starts on %s, not after phase %d's start on %s", i+1, p.Start, i, prev.Start))
		case !prev.End.IsZero() && !prev.End.Before(p.Start):
			errs = append(errs, fmt.Errorf("phase %d starts on %s, while phase %d runs to %s", i+1, p.Start, i, prev.End))
		}
	}
	return errors.Join(errs...)
}

// PhaseOn returns the phase day d lies in, from a timeline that passes
// ValidateTimeline: the last to start on or before d, unless it ended
// before d. The second result is false when d lies in no phase.
func PhaseOn(phases []Phase, d day.Date) (Phase, bool) {
	for _, p := range slices.Backward(phases) {
		if !d.Before(p.Start) {
			if !p.On(d) {
				break
			}
			return p, true
		}
	}
	return Phase{}, false
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
