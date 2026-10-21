package metric

import (
	"math"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	for _, v := range []Value{
		{Weight, 82.6, Manual},
		{Weight, 20, Import},
		{Weight, 400, Import},
		{Steps, 0, Import},
		{Sleep, 24, Import},
		{BodyFat, 31.5, Import},
		{BasalKcal, 1890.25, Import},
		{Steps, 300000, Import},     // an ultra day
		{ExerciseMin, 1500, Import}, // a 25-hour day
	} {
		if err := v.Validate(); err != nil {
			t.Errorf("%+v: %v", v, err)
		}
	}
	for _, tc := range []struct {
		v    Value
		want string
	}{
		{Value{"mood", 3, Manual}, `metric "mood"`},
		{Value{Weight, 80, "phone"}, `source "phone"`},
		{Value{Weight, 19.9, Manual}, "want 20 to 400 kg"},
		{Value{Weight, 401, Manual}, "want 20 to 400 kg"},
		{Value{Sleep, 450, Import}, "want 0 to 24 h"}, // minutes for hours
		{Value{BodyFat, 76, Import}, "want 1 to 75 %"},
		{Value{Steps, -1, Import}, "steps -1"},
		{Value{RestingHR, math.NaN(), Import}, "rhr NaN"},
		{Value{RestingHR, math.Inf(1), Import}, "rhr +Inf"},
	} {
		if err := tc.v.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Validate(%+v) = %v, want an error with %q", tc.v, err, tc.want)
		}
	}
}

func TestValidateDay(t *testing.T) {
	t.Parallel()

	if err := ValidateDay([]Value{{Weight, 80, Manual}, {Steps, 5000, Import}}); err != nil {
		t.Errorf("a day: %v", err)
	}
	if err := ValidateDay([]Value{{Steps, 5000, Import}, {Steps, 6000, Import}}); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("a kind twice = %v", err)
	}
	if err := ValidateDay([]Value{{Steps, -5, Import}}); err == nil {
		t.Error("an invalid value passed")
	}
}

func TestKinds(t *testing.T) {
	t.Parallel()

	for _, k := range Kinds() {
		if k.Unit() == "" || !k.Known() {
			t.Errorf("%s has no spec", k)
		}
	}
	if len(Kinds()) != len(specs) {
		t.Errorf("%d kinds listed, %d specified", len(Kinds()), len(specs))
	}
	if lo, hi := Weight.Bounds(); lo != 20 || hi != 400 {
		t.Errorf("Weight.Bounds() = %v, %v", lo, hi)
	}
	if Kind("mood").Known() || Kind("mood").Unit() != "" {
		t.Error("an unknown kind is known")
	}
	for k, want := range map[Kind]bool{Steps: true, ActiveKcal: true, BasalKcal: true, ExerciseMin: true, Sleep: true, Weight: false, HRV: false, RestingHR: false} {
		if k.Grows() != want {
			t.Errorf("%s.Grows() = %v, want %v", k, !want, want)
		}
	}
	for k, want := range map[Kind]bool{Weight: true, BodyFat: true, LeanMass: true, Steps: false, HRV: false, VO2Max: false} {
		if k.KeepsFirst() != want {
			t.Errorf("%s.KeepsFirst() = %v, want %v", k, !want, want)
		}
	}
}

// The predecessor's rules (health-api, internal/vault ApplyMetrics), with
// two changes on purpose: a manual value beats an import of every kind,
// not only weight, and body fat and lean mass keep the first weighing as
// weight does.
func TestMerge(t *testing.T) {
	t.Parallel()

	v := func(k Kind, x float64, s Source) *Value { return &Value{k, x, s} }
	for _, tc := range []struct {
		name        string
		current     *Value
		incoming    Value
		want        Value
		wantChanged bool
	}{
		{"nothing yet", nil, Value{Weight, 83.1, Import}, Value{Weight, 83.1, Import}, true},
		{"a manual weight beats an import", v(Weight, 82.4, Manual), Value{Weight, 83.1, Import}, Value{Weight, 82.4, Manual}, false},
		{"a manual weight corrects a manual one", v(Weight, 82.4, Manual), Value{Weight, 82.0, Manual}, Value{Weight, 82.0, Manual}, true},
		{"a manual weight replaces an import", v(Weight, 83.1, Import), Value{Weight, 82.4, Manual}, Value{Weight, 82.4, Manual}, true},
		{"the first weighing stays", v(Weight, 82.2, Import), Value{Weight, 83.5, Import}, Value{Weight, 82.2, Import}, false},
		{"the first body fat stays", v(BodyFat, 31.2, Import), Value{BodyFat, 30.8, Import}, Value{BodyFat, 31.2, Import}, false},
		{"a manual weight over an imported one", v(Weight, 83.5, Import), Value{Weight, 82.2, Manual}, Value{Weight, 82.2, Manual}, true},
		{"sleep never shrinks on an import", v(Sleep, 7.6, Import), Value{Sleep, 3.9, Import}, Value{Sleep, 7.6, Import}, false},
		{"a longer sleep applies", v(Sleep, 7.6, Import), Value{Sleep, 7.9, Import}, Value{Sleep, 7.9, Import}, true},
		{"steps never shrink on an import", v(Steps, 5120, Import), Value{Steps, 310, Import}, Value{Steps, 5120, Import}, false},
		{"basal kcal never shrink", v(BasalKcal, 1890.25, Import), Value{BasalKcal, 612.3, Import}, Value{BasalKcal, 1890.25, Import}, false},
		{"more steps apply", v(Steps, 5120, Import), Value{Steps, 5480, Import}, Value{Steps, 5480, Import}, true},
		{"the same total again", v(Steps, 5000, Import), Value{Steps, 5000, Import}, Value{Steps, 5000, Import}, false},
		{"a manual correction of steps, lower", v(Steps, 9000, Import), Value{Steps, 6500, Manual}, Value{Steps, 6500, Manual}, true},
		{"an import after manual steps", v(Steps, 6500, Manual), Value{Steps, 9000, Import}, Value{Steps, 6500, Manual}, false},
		{"the same manual value again", v(HRV, 42, Manual), Value{HRV, 42, Manual}, Value{HRV, 42, Manual}, false},
		{"an import of a kind that does not grow, lower", v(HRV, 48, Import), Value{HRV, 41, Import}, Value{HRV, 41, Import}, true},
		{"an import of a kind that does not grow, higher", v(RestingHR, 52, Import), Value{RestingHR, 55, Import}, Value{RestingHR, 55, Import}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, changed := Merge(tc.current, tc.incoming)
			if got != tc.want || changed != tc.wantChanged {
				t.Errorf("Merge = %+v, %v; want %+v, %v", got, changed, tc.want, tc.wantChanged)
			}
		})
	}
}
