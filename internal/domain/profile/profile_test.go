package profile

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

func date(t *testing.T, s string) day.Date {
	t.Helper()
	d, err := day.ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate(%s): %v", s, err)
	}
	return d
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestAgeOn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, birth, on string
		want            int
	}{
		{"the day before the birthday", "1990-06-15", "2026-06-14", 35},
		{"on the birthday", "1990-06-15", "2026-06-15", 36},
		{"after the birthday", "1990-06-15", "2026-12-31", 36},
		{"a month before", "1990-06-15", "2026-05-20", 35},
		{"the day of birth", "2026-10-21", "2026-10-21", 0},
		{"29 February, on 28 February of a common year", "2008-02-29", "2026-02-28", 17},
		{"29 February, on 1 March of a common year", "2008-02-29", "2026-03-01", 18},
		{"29 February, on 29 February of a leap year", "2008-02-29", "2028-02-29", 20},
		{"the day before turning 18", "2008-10-22", "2026-10-21", 17},
		{"turning 18", "2008-10-21", "2026-10-21", 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := AgeOn(date(t, tc.birth), date(t, tc.on)); got != tc.want {
				t.Errorf("AgeOn(%s, %s) = %d, want %d", tc.birth, tc.on, got, tc.want)
			}
			p := Profile{BirthDate: date(t, tc.birth)}
			if got := p.AgeOn(date(t, tc.on)); got != tc.want {
				t.Errorf("Profile.AgeOn = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMissing(t *testing.T) {
	t.Parallel()

	full := Profile{Sex: Female, BirthDate: date(t, "1990-06-15"), HeightCm: 168}
	for _, tc := range []struct {
		name string
		p    Profile
		want []string
	}{
		{"complete", full, nil},
		{"empty", Profile{}, []string{FieldSex, FieldBirthDate, FieldHeight}},
		{"no sex", Profile{BirthDate: full.BirthDate, HeightCm: 168}, []string{FieldSex}},
		{"no birth date", Profile{Sex: Male, HeightCm: 168}, []string{FieldBirthDate}},
		{"no height", Profile{Sex: Male, BirthDate: full.BirthDate}, []string{FieldHeight}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.Missing(); !slices.Equal(got, tc.want) {
				t.Errorf("Missing() = %v, want %v", got, tc.want)
			}
			if got := tc.p.Complete(); got != (tc.want == nil) {
				t.Errorf("Complete() = %v, want %v", got, tc.want == nil)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	today := date(t, "2026-10-21")
	for _, p := range []Profile{
		{},
		{Sex: Male},
		{Sex: Female, BirthDate: date(t, "1990-06-15"), HeightCm: 168.5},
		{BirthDate: today}, // born today: a profile, if no plan
		{BirthDate: date(t, "1896-10-21")},
		{HeightCm: MinHeightCm},
		{HeightCm: MaxHeightCm},
	} {
		if err := p.Validate(today); err != nil {
			t.Errorf("Validate(%+v) = %v", p, err)
		}
	}
	for _, tc := range []struct {
		name string
		p    Profile
		want string
	}{
		{"an unknown sex", Profile{Sex: "other"}, `sex "other": want male or female`},
		{"a sex in capitals", Profile{Sex: "Male"}, `sex "Male"`},
		{"born tomorrow", Profile{BirthDate: date(t, "2026-10-22")}, "after today"},
		{"older than the oldest", Profile{BirthDate: date(t, "1895-10-21")}, "at most 130"},
		{"a height in metres", Profile{HeightCm: 1.8}, "height 1.8: want 50 to 275 cm"},
		{"too short", Profile{HeightCm: 49.9}, "height 49.9"},
		{"too tall", Profile{HeightCm: 275.1}, "height 275.1"},
		{"a negative height", Profile{HeightCm: -170}, "height -170"},
		{"a height that is no number", Profile{HeightCm: math.NaN()}, "height NaN"},
		{"an infinite height", Profile{HeightCm: math.Inf(1)}, "height +Inf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.p.Validate(today); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error with %q", err, tc.want)
			}
		})
	}

	// Every field that breaks a rule is reported.
	err := Profile{Sex: "x", BirthDate: date(t, "2030-01-01"), HeightCm: 1}.Validate(today)
	for _, want := range []string{"sex", "birth date", "height"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate = %v, want it to name the %s", err, want)
		}
	}
}

func TestBMI(t *testing.T) {
	t.Parallel()

	p := Profile{HeightCm: 180}
	for _, tc := range []struct {
		name        string
		weight, bmi float64
	}{
		{"a BMI of 25", 81, 25},
		{"a BMI of 30", 97.2, 30},
		{"a BMI of 18.5", 59.94, 18.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := p.BMI(tc.weight); !near(got, tc.bmi) {
				t.Errorf("BMI(%v kg at 180 cm) = %v, want %v", tc.weight, got, tc.bmi)
			}
			if got := p.WeightAt(tc.bmi); !near(got, tc.weight) {
				t.Errorf("WeightAt(%v at 180 cm) = %v, want %v", tc.bmi, got, tc.weight)
			}
		})
	}
}

func TestBMR(t *testing.T) {
	t.Parallel()

	on := date(t, "2026-10-21")
	for _, tc := range []struct {
		name   string
		p      Profile
		weight float64
		want   float64
	}{
		// 10 × 80 + 6.25 × 180 − 5 × 30 + 5
		{"a man", Profile{Male, date(t, "1996-01-10"), 180}, 80, 1780},
		// 10 × 60 + 6.25 × 165 − 5 × 40 − 161
		{"a woman", Profile{Female, date(t, "1986-03-02"), 165}, 60, 1270.25},
		// The age is the one on the day: 29 until the birthday.
		{"a man the day before his birthday", Profile{Male, date(t, "1996-10-22"), 180}, 80, 1785},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.BMR(tc.weight, on); !near(got, tc.want) {
				t.Errorf("BMR = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProteinReferenceMass(t *testing.T) {
	t.Parallel()

	p := Profile{HeightCm: 180} // a BMI of 30 is 97.2 kg
	for _, tc := range []struct {
		name         string
		trend, wantM float64
	}{
		{"below a BMI of 30, the trend weight", 80, 80},
		{"at a BMI of 30", 97.2, 97.2},
		{"above it, the weight at a BMI of 30", 120, 97.2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := p.ProteinReferenceMass(tc.trend); !near(got, tc.wantM) {
				t.Errorf("ProteinReferenceMass(%v) = %v, want %v", tc.trend, got, tc.wantM)
			}
		})
	}
}

func TestValidateNotes(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"", "No fish.\nTrains at 6.", strings.Repeat("a", 4000)} {
		if err := ValidateNotes(ok); err != nil {
			t.Errorf("ValidateNotes(%.20q) = %v", ok, err)
		}
	}
	for _, bad := range []string{strings.Repeat("a", 4001), " ", "a\u200bb"} {
		if err := ValidateNotes(bad); err == nil {
			t.Errorf("ValidateNotes(%.20q) passed", bad)
		}
	}
}
