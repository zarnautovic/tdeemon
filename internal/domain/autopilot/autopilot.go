// Package autopilot decides the autopilot's weekly review (DESIGN.md §7,
// §8): from calibration's verdict, the plan, the user's limits and the
// autopilot's own history, whether the kcal target takes a step, holds,
// or waits, and why. A step is the only change it applies itself: the
// target's weekly average shifted by the verdict's step
// (plan.Target.ShiftKcal), which limits.Validate holds to the limits of
// a step. Everything here is pure; the store writes what Decide decides.
package autopilot

import (
	"fmt"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// Mode is how far the user lets the autopilot go.
type Mode string

// Modes.
const (
	// Propose (the default): every step is a proposal the user accepts
	// or rejects.
	Propose Mode = "propose"
	// Auto applies a step by itself, within the user's limits, and
	// proposes one past the drift limit.
	Auto Mode = "autopilot"
)

// DefaultWeekday is the review's weekday until the user picks another.
const DefaultWeekday = time.Monday

// Validate reports a mode that is neither Propose nor Auto.
func (m Mode) Validate() error {
	if m != Propose && m != Auto {
		return fmt.Errorf("mode %q: want %s or %s", m, Propose, Auto)
	}
	return nil
}

// Settings are the user's settings of the autopilot: the mode and the
// weekday of the review.
type Settings struct {
	Mode    Mode
	Weekday time.Weekday
}

// DefaultSettings are a user's settings until they change them.
func DefaultSettings() Settings { return Settings{Mode: Propose, Weekday: DefaultWeekday} }

// Validate reports a mode or a weekday that is none.
func (s Settings) Validate() error {
	if err := s.Mode.Validate(); err != nil {
		return err
	}
	if s.Weekday < time.Sunday || s.Weekday > time.Saturday {
		return fmt.Errorf("weekday %d: want 0 (Sunday) to 6", s.Weekday)
	}
	return nil
}

// NextOn returns the first day on or after d that falls on weekday: the
// review's day, from d.
func NextOn(d day.Date, weekday time.Weekday) day.Date {
	return d.AddDays((int(weekday) - int(d.Weekday()) + 7) % 7)
}
