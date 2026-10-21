package plan

import (
	"fmt"
	"math/bits"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// DayType is the type of a day a target is given for.
type DayType string

// Day types.
const (
	Training DayType = "training"
	Rest     DayType = "rest"
)

// Weekdays is a set of days of the week: bit i holds time.Weekday(i),
// Sunday being 0. The plan's training weekdays give each day its type
// ahead of time (DESIGN.md §7).
type Weekdays uint8

// allWeekdays holds every day of the week.
const allWeekdays Weekdays = 1<<7 - 1

// WeekdaysOf returns the set of the days ds, or an error for a value that
// is no day of the week (Sunday, 0, to Saturday, 6), as a number read
// from a request could be.
func WeekdaysOf(ds ...time.Weekday) (Weekdays, error) {
	var w Weekdays
	for _, d := range ds {
		if !isWeekday(d) {
			return 0, fmt.Errorf("weekday %d: want 0 (Sunday) to 6 (Saturday)", int(d))
		}
		w |= 1 << d
	}
	return w, nil
}

// Has reports whether d is in w; a value that is no day of the week is
// in no set.
func (w Weekdays) Has(d time.Weekday) bool { return isWeekday(d) && w&(1<<d) != 0 }

func isWeekday(d time.Weekday) bool { return d >= time.Sunday && d <= time.Saturday }

// Count returns how many days w holds, 0 to 7.
func (w Weekdays) Count() int { return bits.OnesCount8(uint8(w & allWeekdays)) }

// Days lists the days in w, from Sunday.
func (w Weekdays) Days() []time.Weekday {
	var out []time.Weekday
	for d := time.Sunday; d <= time.Saturday; d++ {
		if w.Has(d) {
			out = append(out, d)
		}
	}
	return out
}

// Validate reports a set with a bit that is no day of the week, as a
// value read from outside could have.
func (w Weekdays) Validate() error {
	if w&^allWeekdays != 0 {
		return fmt.Errorf("weekdays %#b: want only the bits of Sunday (0) to Saturday (6)", uint8(w))
	}
	return nil
}

// TypeOf returns the type of day d: a training day when it is one of the
// plan's training weekdays, else a rest day, ahead of time; a training
// day once a source recorded a training session on it (session); and the
// type the user set in the check-in when they set one (toggle), which
// has the last word. A source that recorded no session leaves the
// plan's type: it may simply not have synced. Calibration counts the
// type corrected this way.
func TypeOf(d day.Date, training Weekdays, session bool, toggle *bool) DayType {
	if toggle != nil {
		if *toggle {
			return Training
		}
		return Rest
	}
	if session || training.Has(d.Weekday()) {
		return Training
	}
	return Rest
}
