// Package day defines the logical day meals and measurements belong to,
// and its status. A day starts at the user's day start hour, not at
// midnight: a snack at 01:30 belongs to the day before.
package day

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Date is a calendar date, with no time and no time zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// layout is a Date's text form: ISO 8601, 2026-10-21.
const layout = "2006-01-02"

// ParseDate reads a date in the form 2026-10-21.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(layout, s)
	if err != nil {
		return Date{}, fmt.Errorf("date %q: want YYYY-MM-DD", s)
	}
	return DateOf(t), nil
}

// DateOf returns the calendar date of t in t's location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// String returns the date as 2026-10-21.
func (d Date) String() string { return d.at(time.UTC).Format(layout) }

// IsZero reports whether d is the zero Date.
func (d Date) IsZero() bool { return d == Date{} }

// AddDays returns the date n days after d (before, for a negative n).
func (d Date) AddDays(n int) Date {
	return DateOf(time.Date(d.Year, d.Month, d.Day+n, 12, 0, 0, 0, time.UTC))
}

// Before reports whether d comes before e.
func (d Date) Before(e Date) bool { return d.at(time.UTC).Before(e.at(time.UTC)) }

func (d Date) at(loc *time.Location) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, loc)
}

// Calendar is how one user splits time into days: their time zone and
// the hour their day starts.
type Calendar struct {
	Location *time.Location
	// StartHour is the hour (0–23) the day starts at, on the wall clock.
	StartHour int
}

// NewCalendar returns the calendar of a user with the IANA time zone tz
// and the day start hour startHour.
func NewCalendar(tz string, startHour int) (Calendar, error) {
	if startHour < 0 || startHour > 23 {
		return Calendar{}, fmt.Errorf("day start hour %d: want 0 to 23", startHour)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return Calendar{}, fmt.Errorf("time zone %q: %w", tz, err)
	}
	return Calendar{Location: loc, StartHour: startHour}, nil
}

// DayOf returns the day an instant belongs to: its date on the user's
// wall clock, or the date before when that is earlier than the day start
// hour. It goes by the wall clock, so a day stays a day across a change
// to or from summer time.
func (c Calendar) DayOf(t time.Time) Date {
	local := t.In(c.Location)
	if local.Hour() < c.StartHour {
		return DateOf(local).AddDays(-1)
	}
	return DateOf(local)
}

// Start returns the instant d starts: StartHour on its date. When that
// hour does not exist on the day (summer time starts then), the day
// starts once it does.
func (c Calendar) Start(d Date) time.Time {
	return time.Date(d.Year, d.Month, d.Day, c.StartHour, 0, 0, 0, c.Location)
}

// End returns the instant d ends: the start of the next day.
func (c Calendar) End(d Date) time.Time { return c.Start(d.AddDays(1)) }

// Status is where a day stands.
type Status string

// Statuses. A day is open until its end, then complete; excluded is the
// user's choice, kept with a reason, and drops the day from calibration.
const (
	Open     Status = "open"
	Complete Status = "complete"
	Excluded Status = "excluded"
)

// MaxReasonLen bounds an exclusion's reason, in characters.
const MaxReasonLen = 200

// Exclusion is why the user took a day out of calibration: a restaurant
// meal they could not weigh, being sick, a day they forgot to log.
type Exclusion struct {
	Reason string
}

// ValidateExclusion checks the reason for excluding a day: some text, at
// most MaxReasonLen characters.
func ValidateExclusion(e Exclusion) error {
	r := strings.TrimSpace(e.Reason)
	switch {
	case r == "":
		return errors.New("excluding a day needs a reason, such as \"restaurant\" or \"sick\"")
	case len([]rune(r)) > MaxReasonLen:
		return fmt.Errorf("the reason is %d characters long, want at most %d", len([]rune(r)), MaxReasonLen)
	}
	return nil
}

// StatusOf returns the status of day d at instant now: excluded when the
// user excluded it, complete once its end has passed, open before. Only
// an exclusion is stored; the rest follows from the clock, so no day has
// to be closed.
func (c Calendar) StatusOf(d Date, now time.Time, excluded bool) Status {
	switch {
	case excluded:
		return Excluded
	case !now.Before(c.End(d)):
		return Complete
	}
	return Open
}
