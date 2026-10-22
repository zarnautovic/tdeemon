// Package day defines the logical day meals and measurements belong to,
// and its status. A day starts at the user's day start hour, not at
// midnight: a snack at 01:30 belongs to the day before.
package day

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/text"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
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

// Weekday returns the day of the week d falls on.
func (d Date) Weekday() time.Weekday { return d.at(time.UTC).Weekday() }

// Before reports whether d comes before e.
func (d Date) Before(e Date) bool { return d.at(time.UTC).Before(e.at(time.UTC)) }

// DaysSince returns how many days d comes after e: below 0 when it comes
// before, 0 on the same date. It counts in seconds of the Unix epoch, so
// dates centuries apart do not overflow a time.Duration.
func (d Date) DaysSince(e Date) int {
	return int((d.at(time.UTC).Unix() - e.at(time.UTC).Unix()) / secondsPerDay)
}

const secondsPerDay = 24 * 60 * 60

// Compare returns -1 when d comes before e, 1 when after, 0 on the same
// date: the order slices.SortFunc takes.
func (d Date) Compare(e Date) int {
	return cmp.Or(cmp.Compare(d.Year, e.Year), cmp.Compare(d.Month, e.Month), cmp.Compare(d.Day, e.Day))
}

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
	// The user's own rule: "" and "Local" would be this machine's zone.
	if err := user.ValidateTimeZone(tz); err != nil {
		return Calendar{}, err
	}
	loc, _ := time.LoadLocation(tz) // ValidateTimeZone has loaded it
	return Calendar{Location: loc, StartHour: startHour}, nil
}

// DayOf returns the day an instant belongs to: its date on the user's
// wall clock, or the date before when it comes before that date's start.
// It is defined by Start, so the instant lies in [Start, End) of its day
// across summer time as zones change their clocks today. (Some zones
// once turned the clock back across midnight; an instant in such an hour
// can get a day that has already ended.)
func (c Calendar) DayOf(t time.Time) Date {
	d := DateOf(t.In(c.Location))
	if t.Before(c.Start(d)) {
		return d.AddDays(-1)
	}
	return d
}

// Start returns the instant d starts: the first instant whose wall clock
// shows StartHour on d's date, or later. When the clock skips that hour
// (summer time starts), the day starts once the clock jumps; when it
// shows the hour twice (summer time ends), at the first of them.
func (c Calendar) Start(d Date) time.Time {
	want := time.Date(d.Year, d.Month, d.Day, c.StartHour, 0, 0, 0, time.UTC) // the wall clock, as a comparable value
	t := time.Date(d.Year, d.Month, d.Day, c.StartHour, 0, 0, 0, c.Location)
	if wallClock(t).Before(want) {
		// time.Date went back across a gap: the day starts when the gap
		// ends, the start of the next zone period.
		_, end := t.ZoneBounds()
		return end
	}
	start, _ := t.ZoneBounds()
	// time.Date may also go forward across a gap that does not begin at
	// the hour (02:45 to 03:45 in Chatham, for a day at 03:00): when the
	// clock jumped past the hour as t's period began, the day starts then.
	if !start.IsZero() && wallClock(start.Add(-time.Nanosecond)).Before(want) && !wallClock(start).Before(want) {
		return start
	}
	// The clock went back to show this wall clock twice when the instant
	// that shows it at the period before's offset still lies in that
	// period; it is the earlier of the two.
	if !start.IsZero() {
		_, prevOffset := start.Add(-time.Nanosecond).Zone()
		_, offset := t.Zone()
		if earlier := t.Add(time.Duration(offset-prevOffset) * time.Second); earlier.Before(start) {
			return earlier
		}
	}
	return t
}

// wallClock returns t's wall clock in its location, as a UTC time to
// compare.
func wallClock(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

// End returns the instant d ends: the start of the next day.
func (c Calendar) End(d Date) time.Time { return c.Start(d.AddDays(1)) }

// Status is where a day stands.
type Status string

// Statuses. A day is open until its end, then complete; excluded is the
// user's choice, kept with a reason, and drops the day's intake from
// calibration (its weigh-in counts).
const (
	Open     Status = "open"
	Complete Status = "complete"
	Excluded Status = "excluded"
)

// MaxReasonLen bounds an exclusion's reason, in characters.
const MaxReasonLen = 200

// Exclusion is why the user took a day's intake out of calibration: a
// restaurant meal they could not weigh, being sick, a day they forgot to
// log.
type Exclusion struct {
	Reason string
}

// ValidateExclusion checks the reason for excluding a day: some text, at
// most MaxReasonLen characters.
func ValidateExclusion(e Exclusion) error {
	if strings.TrimSpace(e.Reason) == "" {
		return errors.New("excluding a day needs a reason, such as \"restaurant\" or \"sick\"")
	}
	return text.Check("the reason", e.Reason, MaxReasonLen, true)
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
