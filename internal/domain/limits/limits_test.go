package limits

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The defaults are the table of docs/limits.md.
func TestDefaults(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		l    Limits
	}{{"the instance's", Instance()}, {"a user's", UserDefaults()}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.l.Validate(); err != nil {
				t.Errorf("Validate = %v", err)
			}
		})
	}
	if err := ValidateInstance(Instance()); err != nil {
		t.Errorf("ValidateInstance(Instance()) = %v", err)
	}
	// A user starts inside the instance's limits.
	if got := Beyond(UserDefaults(), Instance()); got != nil {
		t.Errorf("Beyond(UserDefaults(), Instance()) = %v", got)
	}
	if got := Effective(UserDefaults(), Instance()); got != UserDefaults() {
		t.Errorf("Effective(UserDefaults(), Instance()) = %+v, want the user's", got)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		change func(*Limits)
		want   string
	}{
		{"a rate that is no number", func(l *Limits) { l.CutRate = math.NaN() }, "cut_rate NaN: want a number"},
		{"an infinite step", func(l *Limits) { l.StepKcal = math.Inf(1) }, "step_kcal +Inf"},
		{"a part of a day", func(l *Limits) { l.IntervalDays = 6.5 }, "interval_days 6.5: want a whole number of days"},
		{"a cut that gains", func(l *Limits) { l.CutRate = 0.1 }, "cut_rate 0.1: want a loss"},
		{"an obese cut that gains", func(l *Limits) { l.CutRateObese = 0.1 }, "cut_rate_obese 0.1"},
		{"a negative kg a week", func(l *Limits) { l.LossKgWeek = -1 }, "loss_kg_week -1"},
		{"a bulk that loses", func(l *Limits) { l.BulkRate = -0.1 }, "bulk_rate -0.1: want a gain"},
		{"a recomp range upside down", func(l *Limits) { l.RecompMin = 0.3 }, "recomp_min 0.3: want at most recomp_max 0.25"},
		{"protein under the code's floor", func(l *Limits) { l.ProteinFloor = 0.79 }, "protein_floor 0.79 g/kg: want at least 0.8"},
		{"a protein range upside down", func(l *Limits) { l.ProteinCeiling = 1.5 }, "protein_floor 1.6: want at most protein_ceiling 1.5"},
		{"negative grams", func(l *Limits) { l.ProteinFloorGrams = -1 }, "protein_floor_grams -1"},
		{"800 kcal for a man", func(l *Limits) { l.KcalFloorMale = 800 }, "kcal_floor_male 800: want more than 800"},
		{"800 kcal for a woman", func(l *Limits) { l.KcalFloorFemale = 800 }, "kcal_floor_female 800"},
		{"a share over 100", func(l *Limits) { l.FatFloorPct = 101 }, "fat_floor_pct 101: want 0 to 100 %"},
		{"a negative share", func(l *Limits) { l.StepPct = -1 }, "step_pct -1"},
		{"a drift over 100", func(l *Limits) { l.DriftPct = 100.5 }, "drift_pct 100.5"},
		{"a negative step", func(l *Limits) { l.StepKcal = -1 }, "step_kcal -1"},
		{"a negative wait", func(l *Limits) { l.FirstStepDays = -1 }, "first_step_days -1"},
		{"more days than a week", func(l *Limits) { l.LoggedDays = 8 }, "logged_days 8: want 0 to 7"},
		{"negative weigh-ins", func(l *Limits) { l.WeighIns = -1 }, "weigh_ins -1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := UserDefaults()
			tc.change(&l)
			if err := l.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error with %q", err, tc.want)
			}
		})
	}

	// A value that is no number is reported once, not also for its sign.
	l0 := UserDefaults()
	l0.CutRate = math.NaN()
	if err := l0.Validate(); err == nil || strings.Contains(err.Error(), "want a loss") {
		t.Errorf("Validate = %v, want only that it is no number", err)
	}

	// The edges are allowed.
	l := UserDefaults()
	l.KcalFloorMale, l.ProteinFloor, l.FatFloorPct, l.LoggedDays, l.CutRate, l.BulkRate = 801, 0.8, 100, 7, 0, 0
	if err := l.Validate(); err != nil {
		t.Errorf("Validate at the edges = %v", err)
	}
}

func TestValidateInstance(t *testing.T) {
	t.Parallel()

	// The admin may tighten anything, and loosen down to the code's floor
	// a limit that has its own hard value.
	ok := Instance()
	ok.KcalFloorFemale, ok.KcalFloorMale, ok.CutRate, ok.IntervalDays, ok.ProteinCeiling = 1000, 1000, -1.2, 10, 2.5
	if err := ValidateInstance(ok); err != nil {
		t.Errorf("ValidateInstance = %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Limits)
		want   string
	}{
		{"what no level may have", func(l *Limits) { l.KcalFloorMale = 700 }, "kcal_floor_male 700"},
		{"a floor in grams", func(l *Limits) { l.ProteinFloorGrams = 100 }, "protein_floor_grams: a user's limit"},
		{"a protein ceiling loosened", func(l *Limits) { l.ProteinCeiling = 3.5 }, "protein_ceiling 3.5: the instance may only tighten it from 3"},
		{"a shorter interval", func(l *Limits) { l.IntervalDays = 6 }, "interval_days 6"},
		{"a shorter first wait", func(l *Limits) { l.FirstStepDays = 13 }, "first_step_days 13"},
		{"fewer logged days", func(l *Limits) { l.LoggedDays = 3 }, "logged_days 3"},
		{"fewer weigh-ins", func(l *Limits) { l.WeighIns = 1 }, "weigh_ins 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := Instance()
			tc.change(&l)
			if err := ValidateInstance(l); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ValidateInstance = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func TestEffectiveBeyondLoosened(t *testing.T) {
	t.Parallel()

	inst := Instance()
	inst.KcalFloorFemale = 1300 // the admin tightened it since
	user := UserDefaults()
	user.CutRate = -1.2       // looser than the instance's −1.0
	user.ProteinCeiling = 2.5 // tighter
	user.ProteinFloorGrams = 150

	eff := Effective(user, inst)
	want := user
	want.CutRate, want.KcalFloorFemale = -1.0, 1300
	if eff != want {
		t.Errorf("Effective = %+v, want %+v", eff, want)
	}
	if got := Beyond(user, inst); !slices.Equal(got, []string{"cut_rate", "kcal_floor_female"}) {
		t.Errorf("Beyond = %v", got)
	}
	// Loosening needs a confirmation; tightening does not.
	if got := Loosened(UserDefaults(), user); !slices.Equal(got, []string{"cut_rate"}) {
		t.Errorf("Loosened = %v, want cut_rate", got)
	}
	if got := Loosened(user, UserDefaults()); !slices.Equal(got, []string{"protein_ceiling", "protein_floor_grams"}) {
		t.Errorf("Loosened back = %v", got)
	}
	if got := Loosened(user, user); got != nil {
		t.Errorf("Loosened(same) = %v", got)
	}
}

func TestFields(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	var l Limits
	for i, f := range Fields() {
		if seen[f.Name] {
			t.Errorf("%s twice", f.Name)
		}
		seen[f.Name] = true
		f.Set(&l, float64(i+1))
		if f.Get(l) != float64(i+1) {
			t.Errorf("%s: Get after Set = %v", f.Name, f.Get(l))
		}
	}
	// Every field of Limits has its Field: setting them all left none at
	// zero.
	for i, f := range Fields() {
		if f.Get(l) != float64(i+1) {
			t.Errorf("%s shares its value with another field", f.Name)
		}
	}
	if n, want := len(Fields()), reflect.TypeFor[Limits]().NumField(); n != want {
		t.Errorf("%d fields, want the %d of Limits", n, want)
	}
}
