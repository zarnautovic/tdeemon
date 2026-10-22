// Package profile defines what a plan needs to know about the person:
// the sex the energy equations take, the birth date and the height, and
// what follows from them with a weight: the age on a day, the BMI, the
// resting energy by the Mifflin–St Jeor equation and the reference mass
// protein is counted against. Logging works without a profile; a plan
// does not (docs/limits.md, "A plan needs the profile").
package profile

import (
	"errors"
	"fmt"
	"math"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// Sex is the sex the energy equations take: they have a term for each.
type Sex string

// Sexes.
const (
	Male   Sex = "male"
	Female Sex = "female"
)

// Bounds of the fields: a height a person can have, in centimetres, so a
// height typed in metres (1.8) or inches is refused; and an age.
const (
	MinHeightCm = 50
	MaxHeightCm = 275
	MaxAge      = 130
)

// ProteinMassBMI is the BMI whose weight caps the reference mass for
// protein: above it, the actual weight overstates the need (Weijs 2025;
// docs/limits.md, "Protein").
const ProteinMassBMI = 30

// Profile is the person behind a plan. Each field is optional for
// logging, so its zero value means it is not given; a plan needs them all
// (Missing).
type Profile struct {
	Sex       Sex
	BirthDate day.Date
	// HeightCm is the height in centimetres.
	HeightCm float64
}

// Field names, as Missing reports them.
const (
	FieldSex       = "sex"
	FieldBirthDate = "birth_date"
	FieldHeight    = "height"
)

// Missing lists the fields a plan needs that p does not give, in a fixed
// order; none for a complete profile.
func (p Profile) Missing() []string {
	var out []string
	if p.Sex == "" {
		out = append(out, FieldSex)
	}
	if p.BirthDate.IsZero() {
		out = append(out, FieldBirthDate)
	}
	if p.HeightCm == 0 {
		out = append(out, FieldHeight)
	}
	return out
}

// Complete reports whether p gives every field a plan needs.
func (p Profile) Complete() bool { return len(p.Missing()) == 0 }

// Validate reports every field that is given and breaks its rule, joined
// into one error: a sex other than Male or Female, a birth date after
// today or one that makes the age today, in whole years, more than
// MaxAge, and a height outside MinHeightCm to MaxHeightCm. A field that is not given is not an error here; Missing
// says what a plan still needs.
func (p Profile) Validate(today day.Date) error {
	var errs []error
	if p.Sex != "" && p.Sex != Male && p.Sex != Female {
		errs = append(errs, fmt.Errorf("sex %q: want %s or %s", p.Sex, Male, Female))
	}
	if !p.BirthDate.IsZero() {
		switch {
		case today.Before(p.BirthDate):
			errs = append(errs, fmt.Errorf("birth date %s: after today, %s", p.BirthDate, today))
		case AgeOn(p.BirthDate, today) > MaxAge:
			errs = append(errs, fmt.Errorf("birth date %s: want an age of at most %d", p.BirthDate, MaxAge))
		}
	}
	if p.HeightCm != 0 && (!finite(p.HeightCm) || p.HeightCm < MinHeightCm || p.HeightCm > MaxHeightCm) {
		errs = append(errs, fmt.Errorf("height %v: want %d to %d cm", p.HeightCm, MinHeightCm, MaxHeightCm))
	}
	return errors.Join(errs...)
}

// AgeOn returns the age in whole years of someone born on birth, on day
// on: the years since, less one before the birthday of on's year. Someone
// born on 29 February turns a year older on 1 March in a common year.
func AgeOn(birth, on day.Date) int {
	age := on.Year - birth.Year
	if on.Month < birth.Month || (on.Month == birth.Month && on.Day < birth.Day) {
		age--
	}
	return age
}

// AgeOn returns p's age on day d.
func (p Profile) AgeOn(d day.Date) int { return AgeOn(p.BirthDate, d) }

// BMI returns the body mass index of weightKg at p's height: the weight
// over the square of the height in metres. p must give the height.
func (p Profile) BMI(weightKg float64) float64 {
	m := p.HeightCm / 100
	return weightKg / (m * m)
}

// WeightAt returns the weight, in kg, at which p's BMI is bmi. p must
// give the height.
func (p Profile) WeightAt(bmi float64) float64 {
	m := p.HeightCm / 100
	return bmi * m * m
}

// BMR returns the resting energy, in kcal a day, by the Mifflin–St Jeor
// equation at weightKg on day d: 10 kcal per kg, 6.25 per cm, less 5 per
// year of age, plus 5 for a man or less 161 for a woman. It is the best
// of the equations for most people (Frankenfield et al. 2005), within 10
// % of the measured rate; tdeemon warns below it and does not refuse
// (docs/limits.md, "Warnings, not limits"). p must be complete.
func (p Profile) BMR(weightKg float64, d day.Date) float64 {
	s := 5.0
	if p.Sex == Female {
		s = -161
	}
	return 10*weightKg + 6.25*p.HeightCm - 5*float64(p.AgeOn(d)) + s
}

// ProteinReferenceMass returns the mass, in kg, protein is counted
// against: the trend weight, but at most the weight at a BMI of
// ProteinMassBMI, as the sources' values are per kg of body weight and
// the actual weight overstates the need above it. The fat-free mass is
// not used: its values per kg are others (docs/limits.md, "Protein").
// p must give the height.
func (p Profile) ProteinReferenceMass(trendKg float64) float64 {
	return math.Min(trendKg, p.WeightAt(ProteinMassBMI))
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
