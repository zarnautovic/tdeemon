package plan

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func weekdays(tb testing.TB, ds ...time.Weekday) Weekdays {
	tb.Helper()
	w, err := WeekdaysOf(ds...)
	if err != nil {
		tb.Fatalf("WeekdaysOf(%v): %v", ds, err)
	}
	return w
}

func TestWeekdays(t *testing.T) {
	t.Parallel()

	w := weekdays(t, time.Monday, time.Wednesday, time.Friday)
	if w.Count() != 3 {
		t.Errorf("Count() = %d, want 3", w.Count())
	}
	if !w.Has(time.Wednesday) || w.Has(time.Tuesday) || w.Has(time.Sunday) {
		t.Errorf("Has is wrong for %v", w.Days())
	}
	if got, want := w.Days(), []time.Weekday{time.Monday, time.Wednesday, time.Friday}; !slices.Equal(got, want) {
		t.Errorf("Days() = %v, want %v", got, want)
	}
	if got := weekdays(t, time.Monday, time.Monday); got != weekdays(t, time.Monday) {
		t.Errorf("a day given twice = %#b, want it once", got)
	}
	all := weekdays(t, time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday)
	if all.Count() != 7 || len(all.Days()) != 7 {
		t.Errorf("every day: Count() = %d, Days() = %v", all.Count(), all.Days())
	}
	if Weekdays(0).Count() != 0 || Weekdays(0).Days() != nil {
		t.Error("no day is not empty")
	}
}

// A number that is no day of the week, as a request could send, is
// refused, and is in no set.
func TestWeekdaysOfNoDay(t *testing.T) {
	t.Parallel()

	for _, d := range []time.Weekday{-1, 7, 8, 255} {
		t.Run(strconv.Itoa(int(d)), func(t *testing.T) {
			t.Parallel()
			if w, err := WeekdaysOf(time.Monday, d); err == nil || w != 0 || !strings.Contains(err.Error(), "want 0 (Sunday) to 6 (Saturday)") {
				t.Errorf("WeekdaysOf(Monday, %d) = %#b, %v; want an error", d, w, err)
			}
			if Weekdays(255).Has(d) {
				t.Errorf("Has(%d) = true", d)
			}
		})
	}
}

func TestWeekdaysValidate(t *testing.T) {
	t.Parallel()

	w := weekdays(t, time.Monday, time.Wednesday, time.Friday)
	all := Weekdays(1<<7 - 1)
	for _, ok := range []Weekdays{0, w, all} {
		if err := ok.Validate(); err != nil {
			t.Errorf("Validate(%#b) = %v", ok, err)
		}
	}
	// The eighth bit is no day: a value read from outside may have it.
	bad := Weekdays(1<<7) | w
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "Sunday (0) to Saturday (6)") {
		t.Errorf("Validate(%#b) = %v, want an error", bad, err)
	}
	if bad.Count() != 3 {
		t.Errorf("Count() of %#b = %d, want the 3 days only", bad, bad.Count())
	}
}

func TestTypeOf(t *testing.T) {
	t.Parallel()

	plan := weekdays(t, time.Monday, time.Wednesday, time.Friday)
	monday, tuesday := date(t, "2026-10-19"), date(t, "2026-10-20")
	yes, no := true, false
	for _, tc := range []struct {
		name    string
		on      string
		session bool
		toggle  *bool
		want    DayType
	}{
		{"a planned training day", "monday", false, nil, Training},
		{"a planned rest day", "tuesday", false, nil, Rest},
		{"a session on a rest day makes it a training day", "tuesday", true, nil, Training},
		{"a session on a training day", "monday", true, nil, Training},
		{"the toggle turns a training day into rest", "monday", false, &no, Rest},
		{"the toggle turns a rest day into training", "tuesday", false, &yes, Training},
		{"the toggle beats a session", "tuesday", true, &no, Rest},
		{"the toggle agreeing with the plan", "monday", false, &yes, Training},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := monday
			if tc.on == "tuesday" {
				d = tuesday
			}
			if got := TypeOf(d, plan, tc.session, tc.toggle); got != tc.want {
				t.Errorf("TypeOf = %s, want %s", got, tc.want)
			}
		})
	}
	if got := TypeOf(monday, 0, false, nil); got != Rest {
		t.Errorf("TypeOf with no training weekdays = %s, want rest", got)
	}
}
