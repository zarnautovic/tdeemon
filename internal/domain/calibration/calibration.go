// Package calibration measures the TDEE from what a user ate and how
// their weight moved (DESIGN.md §7): a smoothed trend weight, a linear
// regression of the weigh-ins over a window of days, and the intake over
// the same days, with an interval that says how far the estimate can be
// trusted. It reads the days it is given and nothing else; loading them
// is the caller's.
package calibration

import (
	"math"
	"slices"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// Doubts: why an estimate is missing or not trusted, machine-readable so
// a summary can say what would settle it.
const (
	// DoubtFewDays: the window holds fewer than Params.MinDays once the
	// first days of the current phase are left out, and the window before
	// the phase gave no estimate either.
	DoubtFewDays = "few_days"
	// DoubtFewWeighIns: fewer than three weigh-ins, too few for a slope
	// with an error, or fewer a week than Params.WeighIns.
	DoubtFewWeighIns = "few_weigh_ins"
	// DoubtFewLoggedDays: fewer than two logged days, too few for an
	// intake with an error, or fewer a week than Params.LoggedDays.
	DoubtFewLoggedDays = "few_logged_days"
	// DoubtWideInterval: half the interval is wider than
	// Params.TrustedKcal.
	DoubtWideInterval = "wide_interval"
)

// The fewest weigh-ins and logged days an estimate needs at all: a slope
// with a residual to measure its error by, and an intake with a spread.
const (
	minWeighIns   = 3
	minLoggedDays = 2
)

// maxAutocorrelation bounds the autocorrelation of the residuals that
// widens the slope's error: at the bound, about one weigh-in in twenty
// counts as independent, and the error stays finite.
const maxAutocorrelation = 0.9

// Params are calibration's settings. They are the same for every user
// until the autopilot comes (P1), which makes them a user's within the
// limits.
type Params struct {
	// WindowDays is how many days, up to the last, an estimate reads.
	WindowDays int
	// MinDays is the fewest days a window needs once the first days of a
	// phase are left out.
	MinDays int
	// WaterDays is how many days at the start of every phase calibration
	// leaves out: glycogen and water move the weight then, not the energy
	// balance (a bulk's start refills them too).
	WaterDays int
	// TrendAlpha is how much of a weigh-in's difference from the trend
	// weight the trend takes, from 0 to 1.
	TrendAlpha float64
	// TrustedKcal is the widest half of the interval, in kcal a day, of an
	// estimate that is trusted.
	TrustedKcal float64
	// LoggedDays and WeighIns are the fewest logged days and weigh-ins a
	// week, on average over the window, of an estimate that is trusted.
	LoggedDays float64
	WeighIns   float64
}

// DefaultParams returns the settings of DESIGN.md §7: a window of 42
// days with at least 14 left after a phase's first 14, a trend that takes
// a tenth of each weigh-in, and trust at an interval of ±200 kcal or
// narrower with the data an autopilot step needs (docs/limits.md). A
// window of 28 days swung with water the weight held for weeks, as after
// a change of training.
func DefaultParams() Params {
	l := limits.UserDefaults()
	return Params{
		WindowDays: 42, MinDays: 14, WaterDays: 14, TrendAlpha: 0.1,
		TrustedKcal: 200, LoggedDays: l.LoggedDays, WeighIns: l.WeighIns,
	}
}

// Day is what calibration reads of one day.
type Day struct {
	// Status is the day's. Only a complete day's intake counts: an
	// excluded day's is not to be trusted, and an open day is not over.
	Status day.Status
	// Meals is how many meals the day has, and Kcal what they add up to.
	// A day without a meal was not logged: its intake is unknown, not 0.
	Meals int
	Kcal  float64
	// WeightKg is the day's weigh-in, 0 for none. An excluded day's counts
	// too: the day's energy stays in the weight whatever was logged.
	WeightKg float64
}

// Estimate is the TDEE measured over a span of days, the window.
type Estimate struct {
	// From and To are the window's first and last day; From is the day
	// after To when the current phase's first days reach past To.
	From, To day.Date
	// BeforePhase is true when the window ends the day before the current
	// phase starts: the phase's own days are too few yet, its first
	// Params.WaterDays and then Params.MinDays more. The estimate measures
	// the phase before, so it says nothing of the current one's rate.
	BeforePhase bool
	// Days is how many days the window has; LoggedDays how many of them
	// are complete and have a meal, whose intake counts; WeighIns how many
	// have a weigh-in.
	Days, LoggedDays, WeighIns int
	// TrendKg is the trend weight on To, from the weigh-ins up to it; 0
	// before the first.
	TrendKg float64
	// Measured is false when the window holds too little for an estimate:
	// only the fields above are set, and Doubts say what is missing.
	Measured bool
	// TDEE is the estimate, in kcal a day, and HalfWidth half its 80 %
	// interval.
	TDEE      float64
	HalfWidth float64
	// IntakeKcal is the average intake of the logged days.
	IntakeKcal float64
	// KgPerWeek is the slope of the weigh-ins, a minus being a loss, and
	// KgPerWeekSE its standard error, widened by the residuals'
	// autocorrelation.
	KgPerWeek   float64
	KgPerWeekSE float64
	// Trusted is true when the estimate is measured, its interval narrow
	// enough and its data enough for a verdict; Doubts say why not.
	Trusted bool
	Doubts  []string
}

// Calibrate measures the TDEE over the Params.WindowDays up to last, the
// day before today for a summary. days holds the days by date; a date it
// lacks is a day with nothing logged. phases is the user's timeline, in
// the order they start (plan.ValidateTimeline): the first
// Params.WaterDays of the phase last lies in, or of the last to start
// before it, are left out. When the days left are fewer than
// Params.MinDays, the estimate is the one up to the day before the phase
// started (BeforePhase), if that window gives one.
func Calibrate(days map[day.Date]Day, phases []plan.Phase, last day.Date, p Params) Estimate {
	from, start := windowFrom(phases, last, p)
	e := measure(days, from, last, p)
	if e.Days >= p.MinDays || start.IsZero() {
		return e
	}
	before := start.AddDays(-1)
	bf, _ := windowFrom(phases, before, p)
	if b := measure(days, bf, before, p); b.Measured {
		b.BeforePhase = true
		return b
	}
	return e
}

// windowFrom returns the first day of the window that ends on last, and
// the start of the phase last lies in or follows (the zero Date for
// none), whose first Params.WaterDays the window leaves out.
func windowFrom(phases []plan.Phase, last day.Date, p Params) (from, start day.Date) {
	from = last.AddDays(1 - p.WindowDays)
	for _, ph := range slices.Backward(phases) {
		if !last.Before(ph.Start) {
			start = ph.Start
			break
		}
	}
	if !start.IsZero() {
		if w := start.AddDays(p.WaterDays); from.Before(w) {
			from = w
		}
	}
	if last.Before(from) {
		from = last.AddDays(1)
	}
	return from, start
}

// measure estimates the TDEE over the days from to to.
func measure(days map[day.Date]Day, from, to day.Date, p Params) Estimate {
	e := Estimate{From: from, To: to, Days: to.DaysSince(from) + 1, TrendKg: trendOn(days, to, p.TrendAlpha)}
	var xs, kgs, kcal []float64
	for i := range e.Days {
		d := days[from.AddDays(i)]
		if d.WeightKg > 0 {
			xs, kgs = append(xs, float64(i)), append(kgs, d.WeightKg)
		}
		if d.Status == day.Complete && d.Meals > 0 {
			kcal = append(kcal, d.Kcal)
		}
	}
	e.WeighIns, e.LoggedDays = len(xs), len(kcal)
	e.Measured = e.Days >= p.MinDays && e.WeighIns >= minWeighIns && e.LoggedDays >= minLoggedDays
	if e.Measured {
		f := regress(xs, kgs)
		e.IntakeKcal = mean(kcal)
		e.KgPerWeek, e.KgPerWeekSE = 7*f.slope, 7*f.se
		// The weight's slope is the energy balance: what was eaten less
		// what was spent, KcalPerKg a kg.
		e.TDEE = e.IntakeKcal - f.slope*plan.KcalPerKg
		e.HalfWidth = tQuantile(f.df) * math.Sqrt(meanVariance(kcal, e.Days)+square(f.se*plan.KcalPerKg))
	}
	e.Doubts = doubts(&e, p)
	e.Trusted = e.Measured && len(e.Doubts) == 0
	return e
}

// doubts lists why e is missing or not trusted.
func doubts(e *Estimate, p Params) []string {
	var out []string
	if e.Days < p.MinDays {
		out = append(out, DoubtFewDays)
	}
	if e.WeighIns < minWeighIns || perWeek(e.WeighIns, e.Days) < p.WeighIns {
		out = append(out, DoubtFewWeighIns)
	}
	if e.LoggedDays < minLoggedDays || perWeek(e.LoggedDays, e.Days) < p.LoggedDays {
		out = append(out, DoubtFewLoggedDays)
	}
	if e.Measured && e.HalfWidth > p.TrustedKcal {
		out = append(out, DoubtWideInterval)
	}
	return out
}

// perWeek returns n of days days as a number a week; 0 for no days.
func perWeek(n, days int) float64 {
	if days <= 0 {
		return 0
	}
	return 7 * float64(n) / float64(days)
}

// fit is a regression line's slope, its standard error and the degrees
// of freedom the error has.
type fit struct {
	slope, se float64
	df        int
}

// regress fits a line to the weigh-ins kgs on the days xs, which are
// distinct and in order, at least minWeighIns of them. The error of the
// slope assumes independent residuals, which weigh-ins on following days
// are not (water and salt carry over), so it is widened by their lag-1
// autocorrelation, and the independent weigh-ins it counts are fewer.
// Without that, an 80 % interval covers the TDEE in 39 to 77 % of the
// tests' simulated windows, the less the more water carries over.
func regress(xs, kgs []float64) fit {
	n := float64(len(xs))
	mx, mk := mean(xs), mean(kgs)
	var sxx, sxk float64
	for i, x := range xs {
		sxx += square(x - mx)
		sxk += (x - mx) * (kgs[i] - mk)
	}
	slope := sxk / sxx
	residuals := make([]float64, len(xs))
	var sse float64
	for i, x := range xs {
		residuals[i] = kgs[i] - mk - slope*(x-mx)
		sse += square(residuals[i])
	}
	rho := autocorrelation(residuals, sse)
	inflation := (1 + rho) / (1 - rho)
	return fit{
		slope: slope,
		se:    math.Sqrt(sse / (n - 2) / sxx * inflation),
		df:    max(1, int(n/inflation)-2),
	}
}

// autocorrelation returns the lag-1 autocorrelation of the residuals,
// whose squares add up to sse, from 0 to maxAutocorrelation: a negative
// one would narrow the error, which the data of a few weeks cannot tell.
// The estimate from a short series falls short of the true one, more so
// once a mean and a slope are taken out, so it is raised by
// (2 + 4ρ) / n; the tests' simulation finds the interval then covers
// about 80 %, for water that carries over from not at all to strongly.
func autocorrelation(residuals []float64, sse float64) float64 {
	if sse == 0 {
		return 0
	}
	var lagged float64
	for i := 1; i < len(residuals); i++ {
		lagged += residuals[i] * residuals[i-1]
	}
	rho := lagged / sse
	rho += (2 + 4*rho) / float64(len(residuals))
	return min(max(rho, 0), maxAutocorrelation)
}

// meanVariance returns the variance of the average of kcal as the
// average intake of all the window's days: the days not logged are
// unknown, so the fewer of the days logged, the wider it is, and with
// every day logged it is 0 (a sample without replacement).
func meanVariance(kcal []float64, days int) float64 {
	m := len(kcal)
	if m >= days {
		return 0
	}
	avg := mean(kcal)
	var ss float64
	for _, k := range kcal {
		ss += square(k - avg)
	}
	return ss / float64(m-1) / float64(m) * float64(days-m) / float64(days-1)
}

// tQuantiles are Student's t at 0.9, the bound of an 80 % interval, for 1
// to 30 degrees of freedom; tNormal is the normal distribution's, which
// more approach.
var tQuantiles = [...]float64{
	3.078, 1.886, 1.638, 1.533, 1.476, 1.440, 1.415, 1.397, 1.383, 1.372,
	1.363, 1.356, 1.350, 1.345, 1.341, 1.337, 1.333, 1.330, 1.328, 1.325,
	1.323, 1.321, 1.319, 1.318, 1.316, 1.315, 1.314, 1.313, 1.311, 1.310,
}

const tNormal = 1.2816

// tQuantile returns the bound of an 80 % interval with df degrees of
// freedom, 1 or more.
func tQuantile(df int) float64 {
	if df > len(tQuantiles) {
		return tNormal
	}
	return tQuantiles[df-1]
}

func mean(vs []float64) float64 {
	var sum float64
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}

func square(v float64) float64 { return v * v }
