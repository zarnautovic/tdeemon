package plan

// KcalPerKg is the energy of a kg of body weight lost or gained, the
// convention calibration and the limits count with (DESIGN.md §7,
// docs/limits.md).
const KcalPerKg = 7700

// KgPerWeek turns a rate in % of the trend weight a week into kg a week,
// at the trend weight now; the sign stays.
func KgPerWeek(ratePctWeek, trendKg float64) float64 { return ratePctWeek / 100 * trendKg }

// RatePctWeek turns kg a week into a rate in % of the trend weight
// trendKg a week; the sign stays.
func RatePctWeek(kgPerWeek, trendKg float64) float64 { return kgPerWeek / trendKg * 100 }

// ImpliedRate returns the rate, in % of the trend weight a week, that
// eating weeklyAvgKcal a day implies with a TDEE of tdee: the weekly
// average less the TDEE, times 7, over KcalPerKg, as a share of the trend
// weight (docs/limits.md, "Reading the table"). A minus is a loss. It
// counts only once calibration trusts the measured TDEE, which is the
// caller's to know.
func ImpliedRate(weeklyAvgKcal, tdee, trendKg float64) float64 {
	return RatePctWeek((weeklyAvgKcal-tdee)*7/KcalPerKg, trendKg)
}

// KcalPerDay returns the kcal a day a rate of ratePctWeek % of the trend
// weight trendKg a week stands for: its kg a week, KcalPerKg a kg, over
// 7 days. A gap between two rates in kcal is the KcalPerDay of their
// difference; the sign stays.
func KcalPerDay(ratePctWeek, trendKg float64) float64 {
	return KgPerWeek(ratePctWeek, trendKg) * KcalPerKg / 7
}
