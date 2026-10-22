// Package limits keeps a plan inside what is safe for a healthy adult,
// on three levels: a floor in the code nobody can lower, the instance's
// limits the admin sets within it, and the user's within those. Validate
// checks every change of a plan against all three and says which rule a
// change breaks. docs/limits.md is the contract: a limit here and its row
// there change together.
package limits

import (
	"errors"
	"fmt"
	"math"
)

// The floor in the code, which no configuration lowers (docs/limits.md,
// "Three levels").
const (
	// CodeKcalFloor: a target of this many kcal a day or less is refused,
	// 800 itself included.
	CodeKcalFloor = 800
	// CodeProteinFloor is the least protein, in g/kg of the reference
	// mass: the RDA (IOM 2005).
	CodeProteinFloor = 0.8
	// CodeLossRate and CodeGainRate bound every rate, in % of the trend
	// weight a week: a phase's and the one a kcal target implies, on every
	// level, so no configuration allows a plan past them (a convention).
	CodeLossRate = -1.5
	CodeGainRate = 1.0
	// MinAge is the age, in years, a plan needs.
	MinAge = 18
	// UnderweightBMI is the BMI, by the trend weight now, below which no
	// change starts or deepens a loss, and no goal or milestone weight
	// lies.
	UnderweightBMI = 18.5
	// ObeseBMI is the BMI, by the trend weight when the phase started,
	// from which a cut's row for BMI 30 or more applies.
	ObeseBMI = 30
)

// Limits are one level's limits: the instance's or a user's. Rates are in
// % of the trend weight a week, a minus being a loss; shares are in %.
type Limits struct {
	// CutRate is the fastest loss below a BMI of 30, and CutRateObese at
	// 30 or more (both 0 or less).
	CutRate      float64
	CutRateObese float64
	// LossKgWeek is the fastest loss in kg a week, whatever the BMI.
	LossKgWeek float64
	// BulkRate is the fastest gain.
	BulkRate float64
	// RecompMin and RecompMax bound a recomp's rate.
	RecompMin float64
	RecompMax float64
	// ProteinFloor and ProteinCeiling bound protein, in g/kg of the
	// reference mass.
	ProteinFloor   float64
	ProteinCeiling float64
	// ProteinFloorGrams is a floor of protein in grams a user may set,
	// which only narrows the floor in g/kg; 0 for none. The instance has
	// none.
	ProteinFloorGrams float64
	// KcalFloorMale and KcalFloorFemale are the least kcal of a day, by
	// the sex of the energy equations.
	KcalFloorMale   float64
	KcalFloorFemale float64
	// FatFloorPct is the least share of a day's kcal fat gives.
	FatFloorPct float64
	// StepKcal and StepPct bound an autopilot step of the weekly average:
	// the lesser of StepKcal and StepPct % of it.
	StepKcal float64
	StepPct  float64
	// IntervalDays is the least time between two autopilot steps, and
	// FirstStepDays before the first after a phase starts or changes.
	IntervalDays  float64
	FirstStepDays float64
	// LoggedDays and WeighIns are the least data of the last 7 days an
	// autopilot step needs.
	LoggedDays float64
	WeighIns   float64
	// DriftPct bounds the autopilot's net change of the weekly average
	// from the last kcal target the user agreed to, in % of the TDEE then.
	DriftPct float64
}

// Instance returns the instance's limits by default: the column "Hard
// limit" of docs/limits.md, or the default where it is empty.
func Instance() Limits {
	return Limits{
		CutRate: -1.0, CutRateObese: -1.25, LossKgWeek: 1.5, BulkRate: 0.5,
		RecompMin: -0.5, RecompMax: 0.25,
		ProteinFloor: CodeProteinFloor, ProteinCeiling: 3.0,
		KcalFloorMale: 1200, KcalFloorFemale: 1200, FatFloorPct: 15,
		StepKcal: 200, StepPct: 10, IntervalDays: 7, FirstStepDays: 14,
		LoggedDays: 4, WeighIns: 3, DriftPct: 25,
	}
}

// UserDefaults returns a user's limits at the start: the column
// "Default" of docs/limits.md. The 1.5 kg a week has no default of its
// own, so it is the instance's.
func UserDefaults() Limits {
	return Limits{
		CutRate: -0.7, CutRateObese: -1.0, LossKgWeek: 1.5, BulkRate: 0.25,
		RecompMin: -0.5, RecompMax: 0.25,
		ProteinFloor: 1.6, ProteinCeiling: 3.0,
		KcalFloorMale: 1500, KcalFloorFemale: 1200, FatFloorPct: 20,
		StepKcal: 100, StepPct: 5, IntervalDays: 7, FirstStepDays: 14,
		LoggedDays: 4, WeighIns: 3, DriftPct: 15,
	}
}

// Field describes one limit: its name, as the API and the configuration
// call it, and which way it is looser.
type Field struct {
	// Name is the limit's name in snake case, such as "kcal_floor_male";
	// the configuration reads it as TDEEMON_LIMIT_KCAL_FLOOR_MALE.
	Name string
	// LooserBelow is true when a lower value is looser (a floor, the
	// fastest loss, a time to wait), false when a higher one is (a
	// ceiling, the fastest gain, a step).
	LooserBelow bool
	// Whole is true for a count of days, which takes whole numbers.
	Whole bool
	// UserOnly is true for a limit only a user sets.
	UserOnly bool
	// TightenOnly is true for a limit whose instance value may only be
	// tightened from its default: an empty cell under "Hard limit".
	TightenOnly bool

	get func(*Limits) *float64
}

// Get returns f's value in l.
func (f Field) Get(l Limits) float64 { return *f.get(&l) }

// Set sets f's value in l to v.
func (f Field) Set(l *Limits, v float64) { *f.get(l) = v }

// looser reports whether a is looser than b.
func (f Field) looser(a, b float64) bool {
	if f.LooserBelow {
		return a < b
	}
	return a > b
}

// Fields lists every limit, in a fixed order.
func Fields() []Field {
	return []Field{
		{Name: "cut_rate", LooserBelow: true, get: func(l *Limits) *float64 { return &l.CutRate }},
		{Name: "cut_rate_obese", LooserBelow: true, get: func(l *Limits) *float64 { return &l.CutRateObese }},
		{Name: "loss_kg_week", get: func(l *Limits) *float64 { return &l.LossKgWeek }},
		{Name: "bulk_rate", get: func(l *Limits) *float64 { return &l.BulkRate }},
		{Name: "recomp_min", LooserBelow: true, get: func(l *Limits) *float64 { return &l.RecompMin }},
		{Name: "recomp_max", get: func(l *Limits) *float64 { return &l.RecompMax }},
		{Name: "protein_floor", LooserBelow: true, get: func(l *Limits) *float64 { return &l.ProteinFloor }},
		{Name: "protein_ceiling", TightenOnly: true, get: func(l *Limits) *float64 { return &l.ProteinCeiling }},
		{Name: "protein_floor_grams", LooserBelow: true, UserOnly: true, get: func(l *Limits) *float64 { return &l.ProteinFloorGrams }},
		{Name: "kcal_floor_male", LooserBelow: true, get: func(l *Limits) *float64 { return &l.KcalFloorMale }},
		{Name: "kcal_floor_female", LooserBelow: true, get: func(l *Limits) *float64 { return &l.KcalFloorFemale }},
		{Name: "fat_floor_pct", LooserBelow: true, get: func(l *Limits) *float64 { return &l.FatFloorPct }},
		{Name: "step_kcal", get: func(l *Limits) *float64 { return &l.StepKcal }},
		{Name: "step_pct", get: func(l *Limits) *float64 { return &l.StepPct }},
		{Name: "interval_days", LooserBelow: true, Whole: true, TightenOnly: true, get: func(l *Limits) *float64 { return &l.IntervalDays }},
		{Name: "first_step_days", LooserBelow: true, Whole: true, TightenOnly: true, get: func(l *Limits) *float64 { return &l.FirstStepDays }},
		{Name: "logged_days", LooserBelow: true, Whole: true, TightenOnly: true, get: func(l *Limits) *float64 { return &l.LoggedDays }},
		{Name: "weigh_ins", LooserBelow: true, Whole: true, TightenOnly: true, get: func(l *Limits) *float64 { return &l.WeighIns }},
		{Name: "drift_pct", get: func(l *Limits) *float64 { return &l.DriftPct }},
	}
}

// Validate reports every limit of l that no level may have, joined into
// one error: a value that is no number, a rate of the wrong sign, a
// share outside 0 to 100 %, a count of days that is not whole or more
// than 7 (the data of a week), a minimum above its maximum, and the
// floor in the code (kcal more than CodeKcalFloor, protein at least
// CodeProteinFloor).
func (l Limits) Validate() error {
	var errs []error
	for _, f := range Fields() {
		switch v := f.Get(l); {
		case !finite(v):
			errs = append(errs, fmt.Errorf("%s %v: want a number", f.Name, v))
		case f.Whole && v != math.Trunc(v):
			errs = append(errs, fmt.Errorf("%s %v: want a whole number of days", f.Name, v))
		}
	}
	if errs != nil {
		return errors.Join(errs...)
	}
	check := func(ok bool, format string, args ...any) {
		if !ok {
			errs = append(errs, fmt.Errorf(format, args...))
		}
	}
	check(l.CutRate <= 0, "cut_rate %v: want a loss, 0 or less", l.CutRate)
	check(l.CutRateObese <= 0, "cut_rate_obese %v: want a loss, 0 or less", l.CutRateObese)
	for _, r := range []struct {
		name string
		v    float64
	}{{"cut_rate", l.CutRate}, {"cut_rate_obese", l.CutRateObese}, {"recomp_min", l.RecompMin}} {
		check(r.v >= CodeLossRate, "%s %v: want at least %v, the floor in the code", r.name, r.v, CodeLossRate)
	}
	for _, r := range []struct {
		name string
		v    float64
	}{{"bulk_rate", l.BulkRate}, {"recomp_max", l.RecompMax}} {
		check(r.v <= CodeGainRate, "%s %v: want at most %v, the ceiling in the code", r.name, r.v, CodeGainRate)
	}
	check(l.LossKgWeek >= 0, "loss_kg_week %v: want 0 or more", l.LossKgWeek)
	check(l.BulkRate >= 0, "bulk_rate %v: want a gain, 0 or more", l.BulkRate)
	check(l.RecompMin <= l.RecompMax, "recomp_min %v: want at most recomp_max %v", l.RecompMin, l.RecompMax)
	check(l.ProteinFloor >= CodeProteinFloor, "protein_floor %v g/kg: want at least %v, the floor in the code", l.ProteinFloor, CodeProteinFloor)
	check(l.ProteinFloor <= l.ProteinCeiling, "protein_floor %v: want at most protein_ceiling %v", l.ProteinFloor, l.ProteinCeiling)
	check(l.ProteinFloorGrams >= 0, "protein_floor_grams %v: want 0 or more", l.ProteinFloorGrams)
	check(l.KcalFloorMale > CodeKcalFloor, "kcal_floor_male %v: want more than %d, the floor in the code", l.KcalFloorMale, CodeKcalFloor)
	check(l.KcalFloorFemale > CodeKcalFloor, "kcal_floor_female %v: want more than %d, the floor in the code", l.KcalFloorFemale, CodeKcalFloor)
	for _, p := range []struct {
		name string
		v    float64
	}{{"fat_floor_pct", l.FatFloorPct}, {"step_pct", l.StepPct}, {"drift_pct", l.DriftPct}} {
		check(p.v >= 0 && p.v <= 100, "%s %v: want 0 to 100 %%", p.name, p.v)
	}
	check(l.StepKcal >= 0, "step_kcal %v: want 0 or more", l.StepKcal)
	for _, d := range []struct {
		name string
		v    float64
	}{{"interval_days", l.IntervalDays}, {"first_step_days", l.FirstStepDays}} {
		check(d.v >= 0, "%s %v: want 0 or more", d.name, d.v)
	}
	for _, d := range []struct {
		name string
		v    float64
	}{{"logged_days", l.LoggedDays}, {"weigh_ins", l.WeighIns}} {
		check(d.v >= 0 && d.v <= 7, "%s %v: want 0 to 7, the days of a week", d.name, d.v)
	}
	return errors.Join(errs...)
}

// ValidateInstance reports what makes l no valid set of an instance's
// limits: anything Validate reports, a floor of protein in grams (a
// user's only), and a limit loosened from its default where the default
// is the instance's limit too (TightenOnly).
func ValidateInstance(l Limits) error {
	errs := []error{l.Validate()}
	def := Instance()
	for _, f := range Fields() {
		switch v := f.Get(l); {
		case f.UserOnly && v != 0:
			errs = append(errs, fmt.Errorf("%s: a user's limit, not the instance's", f.Name))
		case f.TightenOnly && f.looser(v, f.Get(def)):
			errs = append(errs, fmt.Errorf("%s %v: the instance may only tighten it from %v", f.Name, v, f.Get(def)))
		}
	}
	return errors.Join(errs...)
}

// Effective returns the limits that hold for a user: for each limit the
// stricter of the user's and the instance's, so a user's limit the admin
// tightened since holds no more than the instance's. A user's own limit,
// which the instance has at 0, is the user's.
func Effective(user, instance Limits) Limits {
	out := user
	for _, f := range Fields() {
		if v := f.Get(instance); f.looser(f.Get(user), v) {
			f.Set(&out, v)
		}
	}
	return out
}

// Beyond lists the user's limits that are looser than the instance's: a
// user may loosen a limit up to the instance's and no further.
func Beyond(user, instance Limits) []string {
	var out []string
	for _, f := range Fields() {
		if f.looser(f.Get(user), f.Get(instance)) {
			out = append(out, f.Name)
		}
	}
	return out
}

// Loosened lists the limits that are looser in to than in from: a change
// of a user's limits that loosens one takes an explicit confirmation in a
// web session, and one that only tightens does not.
func Loosened(from, to Limits) []string {
	var out []string
	for _, f := range Fields() {
		if f.looser(f.Get(to), f.Get(from)) {
			out = append(out, f.Name)
		}
	}
	return out
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
