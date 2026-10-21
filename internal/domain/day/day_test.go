package day

import (
	"strings"
	"testing"
	"time"
)

func zagreb(t *testing.T, startHour int) Calendar {
	t.Helper()
	c, err := NewCalendar("Europe/Zagreb", startHour)
	if err != nil {
		t.Fatalf("NewCalendar: %v", err)
	}
	return c
}

func at(t *testing.T, c Calendar, s string) time.Time {
	t.Helper()
	v, err := time.ParseInLocation("2006-01-02 15:04", s, c.Location)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return v
}

func TestDayOf(t *testing.T) {
	t.Parallel()

	c := zagreb(t, 4)
	for _, tc := range []struct{ name, at, want string }{
		{"a snack at 01:30 belongs to the day before", "2026-10-21 01:30", "2026-10-20"},
		{"03:59 is still the day before", "2026-10-21 03:59", "2026-10-20"},
		{"04:00 starts the day", "2026-10-21 04:00", "2026-10-21"},
		{"the evening", "2026-10-21 23:59", "2026-10-21"},
		{"across a new year", "2027-01-01 02:00", "2026-12-31"},
		{"across a month", "2026-11-01 00:10", "2026-10-31"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := c.DayOf(at(t, c, tc.at)).String(); got != tc.want {
				t.Errorf("DayOf(%s) = %s, want %s", tc.at, got, tc.want)
			}
		})
	}
}

// The instant is read on the user's wall clock, whatever zone it comes in.
func TestDayOfUsesTheUsersZone(t *testing.T) {
	t.Parallel()

	c := zagreb(t, 4)
	// 23:30 UTC on 20 October is 01:30 in Zagreb on the 21st: still the
	// 20th's day.
	if got := c.DayOf(time.Date(2026, 10, 20, 23, 30, 0, 0, time.UTC)).String(); got != "2026-10-20" {
		t.Errorf("DayOf = %s, want 2026-10-20", got)
	}
	// 02:30 UTC on the 21st is 04:30 in Zagreb: the 21st.
	if got := c.DayOf(time.Date(2026, 10, 21, 2, 30, 0, 0, time.UTC)).String(); got != "2026-10-21" {
		t.Errorf("DayOf = %s, want 2026-10-21", got)
	}
}

// A day starting at midnight is the calendar day.
func TestDayOfAtMidnight(t *testing.T) {
	t.Parallel()

	c := zagreb(t, 0)
	if got := c.DayOf(at(t, c, "2026-10-21 00:00")).String(); got != "2026-10-21" {
		t.Errorf("DayOf(00:00) = %s, want 2026-10-21", got)
	}
}

// Summer time ends on 25 October 2026 in Zagreb (03:00 back to 02:00) and
// starts on 29 March 2026 (02:00 forward to 03:00): days follow the wall
// clock, so with the day at 04:00 the day of 24 October lasts 25 hours
// and that of 28 March 23.
func TestDaysAcrossSummerTime(t *testing.T) {
	t.Parallel()

	c := zagreb(t, 4)
	for _, tc := range []struct {
		date string
		want time.Duration
	}{
		{"2026-10-24", 25 * time.Hour}, // from 04:00 on the 24th to 04:00 on the 25th, with the extra hour
		{"2026-10-25", 24 * time.Hour},
		{"2026-03-28", 23 * time.Hour},
	} {
		t.Run(tc.date, func(t *testing.T) {
			t.Parallel()
			d, err := ParseDate(tc.date)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.End(d).Sub(c.Start(d)); got != tc.want {
				t.Errorf("%s lasts %v, want %v", tc.date, got, tc.want)
			}
		})
	}
	// A day that starts at 02:00, an hour that does not exist on 29 March:
	// it starts when the clock jumps, at 03:00.
	c2 := zagreb(t, 2)
	d, _ := ParseDate("2026-03-29")
	if got := c2.Start(d).In(c2.Location).Hour(); got != 3 {
		t.Errorf("the day starts at %d:00, want 3:00", got)
	}
}

// The property everything rests on: an instant lies in [Start, End) of
// the day DayOf gives it, and the days tile time without gaps, in zones
// where the clock jumps forward and back at different hours, by an hour
// or by half an hour, for day start hours around the jumps.
func TestDaysTileTime(t *testing.T) {
	t.Parallel()

	for _, tz := range []string{
		"Europe/Zagreb", "America/New_York", "America/Santiago", "Australia/Lord_Howe",
		"America/Havana", "Asia/Beirut", "Pacific/Chatham", "Antarctica/Troll", "UTC",
	} {
		t.Run(tz, func(t *testing.T) {
			t.Parallel()
			for _, h := range []int{0, 1, 2, 3, 4, 23} {
				c, err := NewCalendar(tz, h)
				if err != nil {
					t.Fatal(err)
				}
				for d := (Date{2026, 1, 1}); d.Year == 2026; d = d.AddDays(1) {
					// Start is the first instant the wall clock shows the
					// hour on the date, or later.
					s, want := c.Start(d), time.Date(d.Year, d.Month, d.Day, h, 0, 0, 0, time.UTC)
					if wallClock(s).Before(want) || !wallClock(s.Add(-time.Second)).Before(want) {
						t.Fatalf("hour %d: %v starts at %v", h, d, s)
					}
				}
				from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				for inst := from; inst.Year() == 2026; inst = inst.Add(15 * time.Minute) {
					d := c.DayOf(inst)
					if s, e := c.Start(d), c.End(d); inst.Before(s) || !inst.Before(e) {
						t.Fatalf("hour %d: %v belongs to %v, which runs from %v to %v", h, inst, d, s, e)
					}
				}
			}
		})
	}
}

// A gap that does not begin at the hour: the day starts as it ends.
func TestStartInAGapOffTheHour(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		tz   string
		hour int
		date string
		want time.Time
	}{
		// Chatham, 27 September 2026: 02:45 +1245 jumps to 03:45 +1345.
		{"Pacific/Chatham", 3, "2026-09-27", time.Date(2026, 9, 26, 14, 0, 0, 0, time.UTC)},
		// Troll, 29 March 2026: 01:00 +00 jumps to 03:00 +02.
		{"Antarctica/Troll", 2, "2026-03-29", time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC)},
	} {
		c, err := NewCalendar(tc.tz, tc.hour)
		if err != nil {
			t.Fatal(err)
		}
		d, _ := ParseDate(tc.date)
		if got := c.Start(d); !got.Equal(tc.want) {
			t.Errorf("%s: Start(%s) = %v, want %v", tc.tz, d, got.UTC(), tc.want)
		}
	}
}

// When the clock shows the start hour twice, the day starts at the first.
func TestStartAtARepeatedHour(t *testing.T) {
	t.Parallel()

	// Zagreb, 25 October 2026: 02:00–02:59 CEST, then again in CET.
	c := zagreb(t, 2)
	d, _ := ParseDate("2026-10-25")
	if got, want := c.Start(d), time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC); !got.Equal(want) { // 02:00 CEST
		t.Errorf("Start = %v, want %v (02:00 CEST)", got, want)
	}
	// New York, 8 March 2026: 02:00 is skipped; the day starts at 03:00 EDT.
	ny, err := NewCalendar("America/New_York", 2)
	if err != nil {
		t.Fatal(err)
	}
	d, _ = ParseDate("2026-03-08")
	if got, want := ny.Start(d), time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC); !got.Equal(want) { // 03:00 EDT
		t.Errorf("Start = %v, want %v (03:00 EDT)", got, want)
	}
}

func TestStatusOf(t *testing.T) {
	t.Parallel()

	c := zagreb(t, 4)
	d, _ := ParseDate("2026-10-21")
	for _, tc := range []struct {
		name     string
		now      string
		excluded bool
		want     Status
	}{
		{"during the day", "2026-10-21 18:00", false, Open},
		{"after midnight, before the day ends", "2026-10-22 03:59", false, Open},
		{"the instant the day ends", "2026-10-22 04:00", false, Complete},
		{"days later", "2026-10-30 12:00", false, Complete},
		{"excluded while open", "2026-10-21 18:00", true, Excluded},
		{"excluded after it ended", "2026-10-30 12:00", true, Excluded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := c.StatusOf(d, at(t, c, tc.now), tc.excluded); got != tc.want {
				t.Errorf("StatusOf = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestNewCalendarRefuses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		tz   string
		hour int
		want string
	}{
		{"hour 24", "UTC", 24, "want 0 to 23"},
		{"a negative hour", "UTC", -1, "want 0 to 23"},
		{"an unknown zone", "Mars/Olympus", 4, "time zone"},
		{"no zone", "", 4, "want an IANA name"},
		{"the machine's zone", "Local", 4, "want an IANA name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewCalendar(tc.tz, tc.hour); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("NewCalendar = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func TestDate(t *testing.T) {
	t.Parallel()

	d, err := ParseDate("2026-02-28")
	if err != nil || d != (Date{2026, time.February, 28}) {
		t.Fatalf("ParseDate = %v, %v", d, err)
	}
	if got := d.AddDays(1).String(); got != "2026-03-01" {
		t.Errorf("AddDays(1) = %s, want 2026-03-01", got)
	}
	if got := d.AddDays(-59).String(); got != "2025-12-31" {
		t.Errorf("AddDays(-59) = %s, want 2025-12-31", got)
	}
	if !d.Before(d.AddDays(1)) || d.Before(d) || d.AddDays(1).Before(d) {
		t.Error("Before is wrong")
	}
	if d.IsZero() || !(Date{}).IsZero() {
		t.Error("IsZero is wrong")
	}
	for _, bad := range []string{"", "2026-2-28", "28.02.2026.", "2026-02-30", "2026-02-28T00:00"} {
		t.Run(bad, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseDate(bad); err == nil {
				t.Errorf("ParseDate(%q) succeeded", bad)
			}
		})
	}
}

func TestValidateExclusion(t *testing.T) {
	t.Parallel()

	if err := ValidateExclusion(Exclusion{Reason: "restaurant"}); err != nil {
		t.Errorf("a reason: %v", err)
	}
	for name, reason := range map[string]string{
		"no reason":    "",
		"only space":   "   ",
		"too long":     strings.Repeat("ž", MaxReasonLen+1),
		"a tab":        "sick\tday",
		"space around": " sick",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateExclusion(Exclusion{Reason: reason}); err == nil {
				t.Error("accepted")
			}
		})
	}
}
