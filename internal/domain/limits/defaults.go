package limits

import (
	"math"

	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// DefaultRate returns the rate, in % of the trend weight a week, a phase
// of type t starts with (docs/limits.md, "Default"): for a cut −0.5 below
// a BMI of 30 and −0.75 from 30 on, by bmi, the BMI when the phase
// starts; +0.25 for a bulk; −0.1 for a recomp; 0 for maintenance. A
// default never passes a limit: it is cut at l's, so at 160 kg a cut's
// −0.75 % (1.2 kg) stays, and at 210 kg it is cut to 1.5 kg a week.
// trendKg is the trend weight now, which turns the kg into %.
func DefaultRate(t plan.PhaseType, bmi, trendKg float64, l Limits) float64 {
	switch t {
	case plan.Cut:
		r, limit := -0.5, l.CutRate
		if bmi >= ObeseBMI {
			r, limit = -0.75, l.CutRateObese
		}
		// Without a trend weight the kg give −Inf %, which cuts nothing.
		return math.Max(math.Max(r, limit), plan.RatePctWeek(-l.LossKgWeek, trendKg))
	case plan.Bulk:
		return math.Min(0.25, l.BulkRate)
	case plan.Recomp:
		return math.Min(math.Max(-0.1, l.RecompMin), l.RecompMax)
	case plan.Maintain:
	}
	return 0
}

// DefaultProtein returns the protein, in g/kg of the reference mass refKg,
// a phase of type t starts with (docs/limits.md): 2.2 on a cut, 1.6 in
// maintenance, 1.8 on a bulk and 2.0 on a recomp, cut to l's floor and
// ceiling (a ceiling of 2.0 cuts a cut's 2.2), and raised to a floor in
// grams the user set; the ceiling wins over it.
func DefaultProtein(t plan.PhaseType, refKg float64, l Limits) float64 {
	var g float64
	switch t {
	case plan.Cut:
		g = 2.2
	case plan.Maintain:
		g = 1.6
	case plan.Bulk:
		g = 1.8
	case plan.Recomp:
		g = 2.0
	}
	g = math.Max(g, l.ProteinFloor)
	if refKg > 0 {
		g = math.Max(g, l.ProteinFloorGrams/refKg)
	}
	return math.Min(g, l.ProteinCeiling)
}
