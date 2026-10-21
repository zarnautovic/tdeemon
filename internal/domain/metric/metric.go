// Package metric defines the measurements of a day (weight, steps,
// sleep, …) and the rules for merging a value that arrives into the one
// stored, carried over from the predecessor: a value the user gave beats
// an imported one, the first weighing of the day stays, and sleep and the
// day's running totals never shrink on a partial import.
package metric

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Kind is what a value measures.
type Kind string

// Kinds.
const (
	Weight      Kind = "weight"       // kg
	BodyFat     Kind = "body_fat"     // %
	LeanMass    Kind = "lean_mass"    // kg
	Steps       Kind = "steps"        // count
	ActiveKcal  Kind = "active_kcal"  // kcal
	BasalKcal   Kind = "basal_kcal"   // kcal
	Sleep       Kind = "sleep_h"      // hours
	RestingHR   Kind = "rhr"          // beats per minute
	HRV         Kind = "hrv"          // ms
	VO2Max      Kind = "vo2max"       // ml/kg/min
	ExerciseMin Kind = "exercise_min" // minutes
)

// How an import of a kind merges into an import of it the day holds.
type rule int

const (
	// replace: the later import is the day's value (resting heart rate,
	// HRV: the device's latest reading).
	replace rule = iota
	// grows: the value only grows over a day (a running total, the night's
	// sleep), so an import of less is a part of the day, not a correction.
	grows
	// first: the day's first import stays (a weighing: the morning's, as
	// the predecessor kept it, not the one after dinner; body fat and lean
	// mass come from the same scale).
	first
)

type spec struct {
	unit     string
	min, max float64
	rule     rule
}

// specs bounds each kind to values a person can have: a typo, or minutes
// sent for hours of sleep, is refused. (A unit that stays in bounds, lb
// for kg, cannot be told apart here; ingest converts by the units the
// source names.)
var specs = map[Kind]spec{
	Weight:      {"kg", 20, 400, first},
	BodyFat:     {"%", 1, 75, first},
	LeanMass:    {"kg", 10, 300, first},
	Steps:       {"steps", 0, 300000, grows},
	ActiveKcal:  {"kcal", 0, 15000, grows},
	BasalKcal:   {"kcal", 0, 10000, grows},
	Sleep:       {"h", 0, 24, grows},
	RestingHR:   {"bpm", 20, 250, replace},
	HRV:         {"ms", 1, 500, replace},
	VO2Max:      {"ml/kg/min", 5, 100, replace},
	ExerciseMin: {"min", 0, 1500, grows}, // a day of summer time's end has 25 hours
}

// Kinds lists every kind, in a fixed order.
func Kinds() []Kind {
	return []Kind{Weight, BodyFat, LeanMass, Steps, ActiveKcal, BasalKcal, Sleep, RestingHR, HRV, VO2Max, ExerciseMin}
}

// Unit returns the unit k's values are in, or "" for an unknown kind.
func (k Kind) Unit() string { return specs[k].unit }

// Bounds returns the least and the most a value of k may be.
func (k Kind) Bounds() (least, most float64) { return specs[k].min, specs[k].max }

// Source is where a value comes from.
type Source string

// Sources.
const (
	// Manual is a value the user gave, through any client: it is the one
	// that counts.
	Manual Source = "manual"
	// Import is a value a device or an app sent (Health Auto Export).
	Import Source = "import"
)

// Value is one measurement of a day.
type Value struct {
	Kind   Kind
	Value  float64
	Source Source
}

// Validate reports a value of an unknown kind or source, or out of its
// kind's bounds.
func (v Value) Validate() error {
	s, ok := specs[v.Kind]
	var errs []error
	if !ok {
		errs = append(errs, fmt.Errorf("metric %q: want one of %v", v.Kind, Kinds()))
	}
	if v.Source != Manual && v.Source != Import {
		errs = append(errs, fmt.Errorf("source %q: want manual or import", v.Source))
	}
	if ok && (math.IsNaN(v.Value) || v.Value < s.min || v.Value > s.max) {
		errs = append(errs, fmt.Errorf("%s %v: want %v to %v %s", v.Kind, v.Value, s.min, s.max, s.unit))
	}
	return errors.Join(errs...)
}

// ValidateDay checks the values that arrive for one day together: each
// valid, and no kind twice.
func ValidateDay(vs []Value) error {
	var errs []error
	seen := map[Kind]bool{}
	for _, v := range vs {
		if err := v.Validate(); err != nil {
			errs = append(errs, err)
		}
		if seen[v.Kind] {
			errs = append(errs, fmt.Errorf("metric %s is given twice", v.Kind))
		}
		seen[v.Kind] = true
	}
	return errors.Join(errs...)
}

// Merge returns the value to keep when incoming arrives for a day that
// holds current (nil when the day has none of its kind), and whether it
// differs from current:
//   - a manual value is kept over an import;
//   - an import of a weighing (weight, body fat, lean mass) is kept only
//     when the day has none: the first weighing of the day stays;
//   - an import of a kind that only grows over a day (steps, active and
//     basal kcal, exercise minutes, sleep) is kept only when it is more:
//     a sync of part of the day must not undo the rest;
//   - otherwise the incoming value replaces current, a manual one
//     correcting a manual one.
func Merge(current *Value, incoming Value) (Value, bool) {
	r := specs[incoming.Kind].rule
	switch {
	case current == nil:
		return incoming, true
	case current.Source == Manual && incoming.Source == Import:
		return *current, false
	case current.Source == Import && incoming.Source == Import && r == first:
		return *current, false
	case current.Source == Import && incoming.Source == Import && r == grows && incoming.Value <= current.Value:
		return *current, false
	}
	return incoming, incoming != *current
}

// Grows reports whether k only grows over a day.
func (k Kind) Grows() bool { return specs[k].rule == grows }

// KeepsFirst reports whether the day's first import of k stays.
func (k Kind) KeepsFirst() bool { return specs[k].rule == first }

// Known reports whether k is a kind of metric.
func (k Kind) Known() bool { return slices.Contains(Kinds(), k) }
