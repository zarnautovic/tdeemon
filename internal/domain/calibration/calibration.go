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
	// first days of the current phase, and the days before the user's
	// first data, are left out, and no window before the phase gave an
	// estimate either. It is then the only doubt: the days to come settle
	// it, not more weigh-ins.
	DoubtFewDays = "few_days"
	// DoubtFewWeighIns: fewer than six weigh-ins over a week or more, too
	// few for a slope to say anything, fewer a week than Params.WeighIns,
	// over the window's whole weeks or in its last 7 days, or a whole week
	// without one.
	DoubtFewWeighIns = "few_weigh_ins"
	// DoubtFewLoggedDays: fewer than a week's logged days between the
	// first and the last weigh-in, too few for the intake's spread, or
	// fewer a week than Params.LoggedDays, over the window's whole weeks
	// or in its last 7 days.
	DoubtFewLoggedDays = "few_logged_days"
	// DoubtWideInterval: half the interval is wider than
	// Params.TrustedKcal.
	DoubtWideInterval = "wide_interval"
)

// What an estimate needs at all: six weigh-ins, two weeks of someone who
// weighs in thrice a week, from first to last at least a week apart, and
// a week of logged days. Three weigh-ins over two days would give a slope
// of water, and two logged days a spread of nothing.
const (
	minWeighIns     = 6
	minWeighInsSpan = 7
	minLoggedDays   = 7
)

// week is the end of a window whose data a trusted estimate needs too,
// as an autopilot step does (docs/limits.md): a slope that ends weeks
// before its last day says little of the weight now.
const week = 7

// scaleStep is the step, in kg, most scales show a weight in. Rounding to
// it puts a floor under the residuals' variance, of a step's square over
// 12, so weigh-ins on a straight line do not make the slope certain.
const scaleStep = 0.1

// maxAutocorrelation bounds the autocorrelation of the residuals that
// widens the slope's error: at the bound, about one weigh-in in twenty
// counts as independent, and the error stays finite.
const maxAutocorrelation = 0.9

// Params are calibration's settings. They are the same for every user
// until the autopilot comes (P1), which makes them a user's within the
// limits, and checks them. Until then a Params is DefaultParams, or one
// changed from it: a field left at 0 is no setting but a bound of 0 (no
// tolerance, no change of steps, no drift allowed).
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
	// weight the trend takes a day, from 0 to 1.
	TrendAlpha float64
	// TrustedKcal is the widest half of the interval, in kcal a day, of an
	// estimate that is trusted.
	TrustedKcal float64
	// LoggedDays and WeighIns are the fewest logged days and weigh-ins a
	// week of an estimate that is trusted, over the window's whole weeks
	// and in its last 7 days. They are the autopilot's data limits, which
	// a user may tighten: a caller takes them from the user's limits.
	LoggedDays float64
	WeighIns   float64
	// TolerancePctWeek is how far, in % of the trend weight a week, the
	// measured rate may lie from the phase's target and be on it.
	TolerancePctWeek float64
	// StepsChange is the share by which the last 7 days' steps may differ
	// from the days before them before the verdict holds the kcal: the
	// change is the activity's, not the metabolism's.
	StepsChange float64
	// LeanLossKgWeek is the loss of lean mass, in kg a week, at which a
	// cut or a recomp raises protein or calories instead of cutting.
	LeanLossKgWeek float64
	// DriftShare is how far the trend weight may stray from the one a
	// maintenance phase started with, as a share of it, before the
	// summary flags the phase drifting.
	DriftShare float64
}

// The settings are checked when they become a user's (P1); until then
// they are DefaultParams.

// DefaultParams returns the settings of DESIGN.md §7: a window of 42
// days with at least 14 left after a phase's first 14, a trend that takes
// a tenth a day, and trust at an interval of ±200 kcal or narrower with
// the data an autopilot step needs (docs/limits.md); a verdict that
// tolerates ±0.1 % a week around the target, holds at steps 30 % off,
// and turns at a lean loss of 0.5 kg a week; and a maintenance phase
// that drifts at 3 %. A window of 28 days
// swung with water the weight held for weeks, as after a change of
// training.
func DefaultParams() Params {
	l := limits.UserDefaults()
	return Params{
		WindowDays: 42, MinDays: 14, WaterDays: 14, TrendAlpha: 0.1,
		TrustedKcal: 200, LoggedDays: l.LoggedDays, WeighIns: l.WeighIns,
		TolerancePctWeek: 0.1, StepsChange: 0.3, LeanLossKgWeek: 0.5, DriftShare: 0.03,
	}
}

// Day is what calibration reads of one day.
type Day struct {
	// Status is the day's. Only a complete day's intake counts: an open
	// day is not over. An excluded day counts for nothing, its weigh-in
	// neither: the user took it out, being sick, on a trip, at a
	// restaurant.
	Status day.Status
	// Meals is how many meals the day has, and Kcal what they add up to.
	// A day without a meal was not logged: its intake is unknown, not 0.
	Meals int
	Kcal  float64
	// WeightKg is the day's weigh-in, 0 for none. A value that is no
	// number, or an infinite one, counts as none, as does such a Kcal.
	WeightKg float64
	// Steps, BodyFatPct, LeanMassKg and ActiveKcal are the day's other
	// measurements the verdict's guards and the limits read, 0 for none,
	// with the same rules.
	Steps, BodyFatPct, LeanMassKg, ActiveKcal float64
}

// Estimate is the TDEE measured over a span of days, the window.
type Estimate struct {
	// From and To are the window's first and last day; From is the day
	// after To when the current phase's first days reach past To.
	From, To day.Date
	// BeforePhase is true when the window ends before the intake last
	// changed, as the current phase started (or the last one ended): the
	// days since are too few yet, their first Params.WaterDays and then
	// Params.MinDays more. The estimate measures a phase before, so it
	// says nothing of the current one's rate, and its doubts are of its
	// own window.
	BeforePhase bool
	// Days is how many days the window has; LoggedDays how many of them
	// are complete and have a meal, whose intake counts; WeighIns how many
	// have a weigh-in. RecentLoggedDays and RecentWeighIns count the same
	// in the window's last 7 days.
	Days, LoggedDays, WeighIns       int
	RecentLoggedDays, RecentWeighIns int
	// TrendKg is the trend weight on the last day Calibrate was asked
	// for, from the weigh-ins up to it, also when the window ends before
	// the phase; 0 before the first weigh-in. TrendDate is the day of the
	// last weigh-in it took, which says how old it is.
	TrendKg   float64
	TrendDate day.Date
	// PhaseStartTrendKg is the trend weight the current phase started
	// with: on the day before it, from a weigh-in at most 14 days older,
	// else the phase's first weigh-in, up to the last day asked for, as a
	// trend starts at it; 0 without a phase or a weigh-in of either.
	PhaseStartTrendKg float64
	// Body is what the window says of the body besides the weight, for
	// the verdict's guards, and Now what the 7 days up to the last day
	// asked for say, whatever the window, for the limits.
	Body Body
	Now  Now
	// Measured is false when the window holds too little for an estimate:
	// only the fields above are set, and Doubts say what is missing.
	Measured bool
	// TDEE is the estimate, in kcal a day, and HalfWidth half its 80 %
	// interval.
	TDEE      float64
	HalfWidth float64
	// IntakeKcal is the average intake of the logged days between the
	// first and the last weigh-in: the days whose energy the slope
	// measures.
	IntakeKcal float64
	// KgPerWeek is the slope of the weigh-ins, a minus being a loss, and
	// KgPerWeekSE its standard error, widened by the residuals'
	// autocorrelation.
	KgPerWeek   float64
	KgPerWeekSE float64
	// Trusted is true when the estimate is measured, its interval narrow
	// enough and its data enough; Doubts say why not. A verdict on the
	// current phase needs it not to be BeforePhase too.
	Trusted bool
	Doubts  []string
}

// Calibrate measures the TDEE over the Params.WindowDays up to last, the
// day before today for a summary: a change of intake on the day after
// last, today, counts as one already, so the estimate on a phase's first
// day is the one before it.
//
// days holds the days by date; a date it lacks is a day with nothing
// logged, though no window starts before the first day with a meal: the
// intake before it is unknown, as the user had not begun to log, even
// with weigh-ins imported from before. It should
// hold the user's history up to last: the trend weight starts at the
// first weigh-in it has, and an estimate from before a phase reads the
// Params.WindowDays before the phase started.
//
// phases is the user's timeline, in the order they start
// (plan.ValidateTimeline). The intake changes when a phase starts, and on
// the day after a phase's planned end when no phase starts then; the
// first Params.WaterDays after the latest such change are left out. When
// the days left are fewer than Params.MinDays, the estimate is the latest
// one before that change (BeforePhase). It goes back past a change too
// recent to give one, as a short diet break, but not past days enough
// that held too little, nor to a window that ends Params.WindowDays or
// more before last: an estimate from before those would be too old.
//
// Days not logged and days excluded are taken to be like the logged ones,
// give or take the spread of a day; an excluded day's weigh-in does not
// count either. Days left out for being unlike them,
// as big days one does not log, bias the TDEE low by what they would have
// added; logging an estimate of such a day keeps it whole.
func Calibrate(days map[day.Date]Day, phases []plan.Phase, last day.Date, p Params) Estimate {
	first := firstData(days)
	changes := intakeChanges(phases)
	change := latestChange(changes, last.AddDays(1))
	e := measure(days, windowFrom(last, change, first, p), last, p)
	for e.Days < p.MinDays && !change.IsZero() {
		before := change.AddDays(-1)
		if last.DaysSince(before) >= p.WindowDays {
			break
		}
		change = latestChange(changes, before)
		if b := measure(days, windowFrom(before, change, first, p), before, p); b.Days >= p.MinDays {
			if b.Measured {
				b.BeforePhase = true
				e = b
			}
			break
		}
	}
	trend := Trend(days, p.TrendAlpha)
	e.TrendKg, e.TrendDate = TrendAt(trend, last)
	if ph, ok := plan.PhaseOn(phases, last.AddDays(1)); ok {
		e.PhaseStartTrendKg = startTrend(days, trend, ph.Start, last)
	}
	e.Body = bodyOf(days, e.From, e.To)
	e.Now = nowOf(days, last)
	return e
}

// firstData returns the first day of days with a meal, the zero Date for
// none.
func firstData(days map[day.Date]Day) day.Date {
	var first day.Date
	for d, v := range days {
		if v.Meals > 0 && (first.IsZero() || d.Before(first)) {
			first = d
		}
	}
	return first
}

// startGap is how old, in days, the last weigh-in before a phase may be
// for the trend on its day before to be the one it started with.
const startGap = 14

// startTrend returns the trend weight a phase that starts on start
// started with, of the trend of days, in the order of its dates, up to
// last: the one on the day before it, when a weigh-in at most startGap
// days older made it, else the phase's first weigh-in, as a trend started
// there, without what an older weigh-in leaves in the trend; 0 for
// neither.
func startTrend(days map[day.Date]Day, trend []Point, start, last day.Date) float64 {
	// The phase on today starts on today at the latest, so the day before
	// it is last at the latest.
	before := start.AddDays(-1)
	if kg, on := TrendAt(trend, before); kg > 0 && before.DaysSince(on) <= startGap {
		return kg
	}
	for d := start; !last.Before(d); d = d.AddDays(1) {
		if v := days[d]; weighed(v) {
			return v.WeightKg
		}
	}
	return 0
}

// intakeChanges returns the days the intake changes on, in order: each
// phase's start, and the day after a phase's planned end when no phase
// starts then.
func intakeChanges(phases []plan.Phase) []day.Date {
	var out []day.Date
	for i, ph := range phases {
		out = append(out, ph.Start)
		if after := ph.End.AddDays(1); !ph.End.IsZero() && (i == len(phases)-1 || phases[i+1].Start != after) {
			out = append(out, after)
		}
	}
	return out
}

// latestChange returns the latest of changes on or before d, the zero
// Date for none.
func latestChange(changes []day.Date, d day.Date) day.Date {
	for _, c := range slices.Backward(changes) {
		if !d.Before(c) {
			return c
		}
	}
	return day.Date{}
}

// windowFrom returns the first day of the window that ends on last, whose
// intake last changed on change (the zero Date for never): the window
// leaves out the first Params.WaterDays from it, and the days before
// first, the user's first day with a meal.
func windowFrom(last, change, first day.Date, p Params) day.Date {
	from := last.AddDays(1 - p.WindowDays)
	if !change.IsZero() {
		if w := change.AddDays(p.WaterDays); from.Before(w) {
			from = w
		}
	}
	if from.Before(first) {
		from = first
	}
	if last.Before(from) {
		from = last.AddDays(1)
	}
	return from
}

// data is what a window holds for its doubts. The per-week limits hold
// for its whole weeks, counted back from its last day: a window of 15
// days would put an eighth day of someone who weighs in thrice a week on
// one side of the limit or the other.
type data struct {
	// weekDays is how many days the whole weeks have, and weighIns and
	// logged their weigh-ins and logged days; emptyWeeks counts the whole
	// weeks without a weigh-in, which a slope cannot see into.
	weekDays, weighIns, logged, emptyWeeks int
	// spread is whether the weigh-ins are enough for a slope, and
	// spanLogged how many days between the first and the last are logged,
	// or in the window when they are not enough.
	spread     bool
	spanLogged int
}

// measure estimates the TDEE over the days from to to.
func measure(days map[day.Date]Day, from, to day.Date, p Params) Estimate {
	e := Estimate{From: from, To: to, Days: to.DaysSince(from) + 1}
	// A window shorter than a week is its own whole week.
	dt := data{weekDays: week * (e.Days / week)}
	if dt.weekDays == 0 {
		dt.weekDays = e.Days
	}
	weighedWeeks := map[int]bool{}
	var xs, kgs []float64
	kcal := make([]float64, e.Days) // a logged day's kcal, NaN for a day not logged
	for i := range e.Days {
		d := days[from.AddDays(i)]
		recent, inWeeks := i >= e.Days-week, i >= e.Days-dt.weekDays
		kcal[i] = math.NaN()
		if weighed(d) {
			xs, kgs = append(xs, float64(i)), append(kgs, d.WeightKg)
			e.RecentWeighIns += btoi(recent)
			dt.weighIns += btoi(inWeeks)
			if inWeeks {
				weighedWeeks[(e.Days-1-i)/week] = true
			}
		}
		if loggedDay(d) {
			kcal[i] = d.Kcal
			e.LoggedDays++
			e.RecentLoggedDays += btoi(recent)
			dt.logged += btoi(inWeeks)
		}
	}
	e.WeighIns = len(xs)
	dt.emptyWeeks = max(dt.weekDays/week, 1) - len(weighedWeeks)
	dt.spread = spread(xs)
	// Without a slope there is no span: the window's logged days are
	// what the doubts can judge.
	dt.spanLogged = e.LoggedDays
	var span []float64
	if dt.spread {
		span = loggedKcal(kcal[int(xs[0]) : int(xs[len(xs)-1])+1])
		dt.spanLogged = len(span)
	}
	e.Measured = e.Days >= p.MinDays && dt.spread && dt.spanLogged >= minLoggedDays
	if e.Measured {
		f := regress(xs, kgs)
		e.IntakeKcal = mean(span)
		e.KgPerWeek, e.KgPerWeekSE = 7*f.slope, 7*f.se
		// The weight's slope is the energy balance: what was eaten less
		// what was spent, KcalPerKg a kg, over the days it spans.
		e.TDEE = e.IntakeKcal - f.slope*plan.KcalPerKg
		spanDays := int(xs[len(xs)-1]-xs[0]) + 1
		slopeVar, intakeVar := square(f.se*plan.KcalPerKg), intakeVariance(span, spanDays)
		df := combinedDF(slopeVar, f.df, intakeVar, len(span)-1)
		e.HalfWidth = tQuantile(df) * math.Sqrt(slopeVar+intakeVar)
	}
	e.Doubts = doubts(e, dt, p)
	e.Trusted = e.Measured && len(e.Doubts) == 0
	return e
}

// loggedKcal returns the kcal of the logged days of kcal, leaving out the
// NaN of a day not logged.
func loggedKcal(kcal []float64) []float64 {
	var out []float64
	for _, k := range kcal {
		if !math.IsNaN(k) {
			out = append(out, k)
		}
	}
	return out
}

// spread reports whether the weigh-ins on the days xs, in order, are
// enough for a slope: minWeighIns of them, minWeighInsSpan days apart.
func spread(xs []float64) bool {
	return len(xs) >= minWeighIns && xs[len(xs)-1]-xs[0] >= minWeighInsSpan
}

// loggedDay reports whether d's intake counts: a complete day with a meal
// and kcal that are a number.
func loggedDay(d Day) bool {
	return d.Status == day.Complete && d.Meals > 0 && finite(d.Kcal) && d.Kcal >= 0
}

// weighed reports whether d has a weigh-in that counts: a number, on a
// day not excluded.
func weighed(d Day) bool { return d.Status != day.Excluded && d.WeightKg > 0 && finite(d.WeightKg) }

// doubts lists why e is missing or not trusted, with the data dt of its
// window. A window too short has only that doubt: its counts say nothing
// of how often one weighs in.
func doubts(e Estimate, dt data, p Params) []string {
	if e.Days < p.MinDays {
		return []string{DoubtFewDays}
	}
	var out []string
	if perWeek(dt.weighIns, dt.weekDays) < p.WeighIns || !dt.spread || dt.emptyWeeks > 0 || float64(e.RecentWeighIns) < p.WeighIns {
		out = append(out, DoubtFewWeighIns)
	}
	if perWeek(dt.logged, dt.weekDays) < p.LoggedDays || dt.spanLogged < minLoggedDays || float64(e.RecentLoggedDays) < p.LoggedDays {
		out = append(out, DoubtFewLoggedDays)
	}
	// Not "wider than": an interval that is no number is not narrow.
	if e.Measured && !(e.HalfWidth <= p.TrustedKcal) {
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

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
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
	// Residuals under the scale's step are the rounding of float
	// arithmetic, whose autocorrelation means nothing.
	floor := (n - 2) * square(scaleStep) / 12
	var rho float64
	if sse > floor {
		rho = autocorrelation(residuals, sse)
	}
	inflation := (1 + rho) / (1 - rho)
	sse = max(sse, floor)
	return fit{
		slope: slope,
		se:    math.Sqrt(sse / (n - 2) / sxx * inflation),
		df:    max(1, int(n/inflation)-2),
	}
}

// autocorrelation returns the lag-1 autocorrelation of the residuals,
// whose squares add up to sse, more than 0, from 0 to
// maxAutocorrelation: a negative one would narrow the error, which the
// data of a few weeks cannot tell.
// The estimate from a short series falls short of the true one, more so
// once a mean and a slope are taken out, so it is raised by
// (2 + 4ρ) / n; the tests' simulation finds the interval then covers
// about 80 %, for water that carries over from not at all to strongly.
func autocorrelation(residuals []float64, sse float64) float64 {
	var lagged float64
	for i := 1; i < len(residuals); i++ {
		lagged += residuals[i] * residuals[i-1]
	}
	rho := lagged / sse
	rho += (2 + 4*rho) / float64(len(residuals))
	return min(max(rho, 0), maxAutocorrelation)
}

// intakeVariance returns the variance of the average of the logged days'
// kcal as the average intake of all the window's days. The days not
// logged are unknown, and taken to differ from the logged ones together,
// as the weekends of someone who logs only on weekdays would, by as much
// as one day spreads: the error is that spread times the share of the
// days not logged, 0 when every day is. It is wider than if they were a
// random sample, whose differences would cancel out. The spread is at
// least minSpread of the average: someone who eats the same on every day
// they log may not on the days they do not.
func intakeVariance(kcal []float64, days int) float64 {
	m := len(kcal)
	if m >= days {
		return 0
	}
	avg := mean(kcal)
	var ss float64
	for _, k := range kcal {
		ss += square(k - avg)
	}
	spread := max(ss/float64(m-1), square(minSpread*avg))
	return square(float64(days-m)/float64(days)) * spread
}

// minSpread is the least spread of a day's intake, as a share of the
// average, that the days not logged are taken to differ by: a tenth, about
// what logging itself misses by.
const minSpread = 0.1

// combinedDF returns the degrees of freedom of the sum of two variances
// with df1 and df2 of their own (Welch and Satterthwaite): near the larger
// part's, so a few logged days of a nearly even intake do not widen the
// slope's bound.
func combinedDF(v1 float64, df1 int, v2 float64, df2 int) int {
	if v2 == 0 {
		return df1
	}
	df := square(v1+v2) / (square(v1)/float64(df1) + square(v2)/float64(df2))
	return max(1, int(df))
}

// tQuantiles are Student's t at 0.9, the bound of an 80 % interval, for 1
// to 30 degrees of freedom; past them, the bound comes from the normal
// distribution's, tNormal, with the first term of its expansion in 1/df.
var tQuantiles = [...]float64{
	3.078, 1.886, 1.638, 1.533, 1.476, 1.440, 1.415, 1.397, 1.383, 1.372,
	1.363, 1.356, 1.350, 1.345, 1.341, 1.337, 1.333, 1.330, 1.328, 1.325,
	1.323, 1.321, 1.319, 1.318, 1.316, 1.315, 1.314, 1.313, 1.311, 1.310,
}

const tNormal = 1.2816

// tQuantile returns the bound of an 80 % interval with df degrees of
// freedom, 1 or more: within 0.001 of Student's t past the table.
func tQuantile(df int) float64 {
	if df > len(tQuantiles) {
		z := tNormal
		return z + (z*z*z+z)/(4*float64(df))
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

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
