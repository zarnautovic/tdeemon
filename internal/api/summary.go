package api

import (
	"context"
	"math"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/calibration"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/recovery"
)

// summaryHistory is how many days up to yesterday the summary reads:
// calibration's window, its look-back before a phase and the trend's
// start all lie well within it.
const summaryHistory = 180

// incompleteShare is the share of its target under which a complete day
// with meals is possibly incomplete (DESIGN.md §4).
const incompleteShare = 0.5

type summaryInput struct {
	Weeks int `query:"weeks" minimum:"1" maximum:"12" default:"4" doc:"How many weeks, up to yesterday, the weekly averages cover."`
}

type summaryOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         summaryBody
}

// summaryBody is calibration's summary (DESIGN.md §5 and §7): what the
// server computes, for a model to explain, not to compute again.
type summaryBody struct {
	Date           string       `json:"date" format:"date" doc:"Today, the user's day now. The summary is of the days up to yesterday; today is open."`
	Weeks          []weekBody   `json:"weeks" nullable:"false" doc:"The last weeks up to yesterday, each of 7 days, the latest last."`
	Trend          *trendBody   `json:"trend,omitempty" doc:"The trend weight; absent before the first weigh-in."`
	TDEE           tdeeBody     `json:"tdee"`
	Verdict        verdictBody  `json:"verdict"`
	Recovery       recoveryBody `json:"recovery"`
	IncompleteDays []string     `json:"incomplete_days" nullable:"false" doc:"Complete days of the weeks whose meals come under half their target: possibly not all logged. The user may log the rest, or exclude the day."`
}

type weekBody struct {
	From         string   `json:"from" format:"date"`
	To           string   `json:"to" format:"date"`
	LoggedDays   int      `json:"logged_days" doc:"The complete days with a meal, not excluded."`
	Kcal         *float64 `json:"kcal,omitempty" doc:"The average intake of the logged days."`
	Protein      *float64 `json:"protein,omitempty" doc:"The average grams of protein of the logged days."`
	WeighIns     int      `json:"weigh_ins"`
	WeightKg     *float64 `json:"weight_kg,omitempty" doc:"The average weigh-in."`
	TrendKg      *float64 `json:"trend_kg,omitempty" doc:"The trend weight on the week's last day."`
	Steps        *float64 `json:"steps,omitempty" doc:"The average steps of the days that have them."`
	SleepH       *float64 `json:"sleep_h,omitempty" doc:"The average hours of sleep of the days that have them."`
	TrainingDays int      `json:"training_days" doc:"The days of the training type, as each day's type is."`
}

type trendBody struct {
	Kg   float64 `json:"kg" doc:"The trend weight: smoothed, without the day-to-day swing of water."`
	Date string  `json:"date" format:"date" doc:"The day of the last weigh-in it took: a trend weeks old is not the weight now."`
}

type tdeeBody struct {
	Measured    bool     `json:"measured" doc:"Whether the window held enough for an estimate; doubts say what is missing."`
	Kcal        *float64 `json:"kcal,omitempty" doc:"The measured TDEE, in kcal a day."`
	HalfWidth   *float64 `json:"half_width,omitempty" doc:"Half its 80 % interval."`
	Trusted     bool     `json:"trusted" doc:"Whether its interval and data are enough for a verdict."`
	BeforePhase bool     `json:"before_phase" doc:"The estimate is of the days before the current phase, whose own are too few yet: no verdict on the phase."`
	From        string   `json:"from" format:"date" doc:"The window's first day."`
	To          string   `json:"to" format:"date" doc:"The window's last day."`
	LoggedDays  int      `json:"logged_days"`
	WeighIns    int      `json:"weigh_ins"`
	KgPerWeek   *float64 `json:"kg_per_week,omitempty" doc:"The weight's slope over the window, a minus being a loss."`
	Doubts      []string `json:"doubts" nullable:"false" doc:"Why the estimate is missing or not trusted: few_days, few_weigh_ins, few_logged_days, wide_interval."`
}

type verdictBody struct {
	Rate             string   `json:"rate" enum:"none,on_target,below,above" doc:"The measured rate against the phase's target: below is a faster loss or a slower gain than planned."`
	Reasons          []string `json:"reasons" nullable:"false" doc:"Why there is no verdict: no_phase, no_target, no_estimate, before_phase, not_trusted."`
	MeasuredPctWeek  *float64 `json:"measured_pct_week,omitempty" doc:"The measured rate, in % of the trend weight a week."`
	TargetPctWeek    *float64 `json:"target_pct_week,omitempty" doc:"The phase's target rate."`
	TolerancePctWeek *float64 `json:"tolerance_pct_week,omitempty" doc:"How far apart they may be."`
	Action           string   `json:"action" enum:"none,hold,raise_kcal,lower_kcal,raise_protein_or_kcal,shrink_surplus,maintain" doc:"What the verdict proposes; nothing applies it."`
	Guards           []string `json:"guards" nullable:"false" doc:"The guards that turned or held the action: underweight, lean_mass, fat_gain, activity."`
	GapKcal          float64  `json:"gap_kcal" doc:"The change of the target's weekly average the action asks for, in kcal a day, more than 0 to eat more: a coach's to propose."`
	StepKcal         float64  `json:"step_kcal" doc:"The step toward it that the autopilot's limits allow."`
	IntakeKcal       *float64 `json:"intake_kcal,omitempty" doc:"What was eaten on average over the window."`
	TargetKcal       *float64 `json:"target_kcal,omitempty" doc:"The target's weekly average now: a rate that misses with a target that holds is the intake's."`
	Flags            []string `json:"flags" nullable:"false" doc:"What to show beside it: underweight, no_height, maintenance_drift, no_steps, no_lean_mass, no_body_fat."`
}

type recoveryBody struct {
	SleepH        *float64 `json:"sleep_h,omitempty" doc:"The last 7 days' average sleep, in hours."`
	RestingHR     *float64 `json:"resting_hr,omitempty" doc:"The last 7 days' average resting heart rate."`
	RestingHRBase *float64 `json:"resting_hr_base,omitempty" doc:"The 21 days' before them."`
	HRV           *float64 `json:"hrv,omitempty" doc:"The last 7 days' average HRV."`
	HRVBase       *float64 `json:"hrv_base,omitempty" doc:"The 21 days' before them."`
	Flags         []string `json:"flags" nullable:"false" doc:"What falls short: short_sleep (under 6.5 hours), high_resting_hr (5 % over its base), low_hrv (15 % under its base)."`
}

// summaryDay is what the summary reads of one of the user's days.
type summaryDay struct {
	status        day.Status
	meals         int
	kcal, protein float64
	values        []metric.Value
	dayType       plan.DayType
}

// summaryData is what the summary reads of the user.
type summaryData struct {
	days     map[day.Date]summaryDay
	values   map[day.Date][]metric.Value
	phases   []plan.Phase
	targets  []plan.Target
	training plan.Weekdays
	params   calibration.Params
	lim      limits.Limits
}

func (s *sessions) getSummary(ctx context.Context, in *summaryInput) (*summaryOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	cal, today, err := userDay(p, now)
	if err != nil {
		return nil, internalError("reading the summary failed", err)
	}
	last := today.AddDays(-1)
	data, err := s.summaryData(ctx, p.User.ID, cal, last.AddDays(1-summaryHistory), last, now)
	if err != nil {
		return nil, internalError("reading the summary failed", err)
	}
	pr, err := s.db.Profile(ctx, p.User.ID)
	if err != nil {
		return nil, internalError("reading the summary failed", err)
	}
	days := map[day.Date]calibration.Day{}
	for d, sd := range data.days {
		days[d] = sd.calibrationDay()
	}
	e := calibration.Calibrate(days, data.phases, last, data.params)
	var ph *plan.Phase
	if got, ok := plan.PhaseOn(data.phases, today); ok {
		ph = &got
	}
	var t *plan.Target
	if got, ok := plan.TargetOn(data.targets, today); ok {
		t = &got
	}
	v := calibration.Judge(e, ph, t, data.training, pr, data.lim, data.params)
	b := summaryBody{
		Date: today.String(), Weeks: []weekBody{}, TDEE: newTDEEBody(e), Verdict: newVerdictBody(v),
		Recovery: newRecoveryBody(recovery.Of(data.values, last)), IncompleteDays: []string{},
	}
	if e.TrendKg > 0 {
		b.Trend = &trendBody{Kg: round2(e.TrendKg), Date: e.TrendDate.String()}
	}
	trend := calibration.Trend(days, data.params.TrendAlpha)
	for w := in.Weeks - 1; w >= 0; w-- {
		to := last.AddDays(-7 * w)
		b.Weeks = append(b.Weeks, data.week(to.AddDays(-6), to, trend))
	}
	b.IncompleteDays = data.incomplete(last.AddDays(1-7*in.Weeks), last)
	return &summaryOutput{CacheControl: "no-store", Body: b}, nil
}

// summaryData reads the user's days from from to to, as the calendar cal
// splits them at now, and their plan.
func (s *sessions) summaryData(ctx context.Context, userID uuid.UUID, cal day.Calendar, from, to day.Date, now time.Time) (summaryData, error) {
	var data summaryData
	meals, err := s.db.MealsBetween(ctx, userID, cal.Start(from), cal.End(to))
	if err != nil {
		return data, err
	}
	if data.values, err = s.db.Metrics(ctx, userID, from, to); err != nil {
		return data, err
	}
	exclusions, err := s.db.Exclusions(ctx, userID, from, to)
	if err != nil {
		return data, err
	}
	if data.phases, err = s.db.Phases(ctx, userID); err != nil {
		return data, err
	}
	if data.targets, err = s.db.Targets(ctx, userID); err != nil {
		return data, err
	}
	if data.training, err = s.db.TrainingWeekdays(ctx, userID); err != nil {
		return data, err
	}
	toggles, err := s.db.TrainingDayToggles(ctx, userID, from, to)
	if err != nil {
		return data, err
	}
	workouts, err := s.db.WorkoutDays(ctx, userID, from, to)
	if err != nil {
		return data, err
	}
	user, err := s.db.UserLimits(ctx, userID)
	if err != nil {
		return data, err
	}
	// The data an estimate needs to be trusted are the autopilot's
	// limits, which the user may have tightened.
	data.lim = limits.Effective(user, s.instance)
	data.params = calibration.DefaultParams()
	data.params.LoggedDays, data.params.WeighIns = data.lim.LoggedDays, data.lim.WeighIns
	data.days = map[day.Date]summaryDay{}
	for d := from; !to.Before(d); d = d.AddDays(1) {
		_, excluded := exclusions[d]
		var toggle *bool
		if v, ok := toggles[d]; ok {
			toggle = &v
		}
		data.days[d] = summaryDay{
			status: cal.StatusOf(d, now, excluded), values: data.values[d],
			dayType: plan.TypeOf(d, data.training, workouts[d], toggle),
		}
	}
	for i := range meals {
		m := &meals[i]
		sd, ok := data.days[m.Day]
		if !ok {
			continue
		}
		n := m.Total()
		sd.meals, sd.kcal, sd.protein = sd.meals+1, sd.kcal+n.Kcal, sd.protein+n.Protein
		data.days[m.Day] = sd
	}
	return data, nil
}

// calibrationDay is the day as calibration reads it.
func (d summaryDay) calibrationDay() calibration.Day {
	c := calibration.Day{Status: d.status, Meals: d.meals, Kcal: d.kcal}
	for _, v := range d.values {
		switch v.Kind {
		case metric.Weight:
			c.WeightKg = v.Value
		case metric.BodyFat:
			c.BodyFatPct = v.Value
		case metric.LeanMass:
			c.LeanMassKg = v.Value
		case metric.Steps:
			c.Steps = v.Value
		case metric.ActiveKcal:
			c.ActiveKcal = v.Value
		default:
		}
	}
	return c
}

// logged reports whether d's intake counts: complete, not excluded, with
// a meal.
func (d summaryDay) logged() bool { return d.status == day.Complete && d.meals > 0 }

// week returns the week from from to to, with the trend weight of trend.
func (data summaryData) week(from, to day.Date, trend []calibration.Point) weekBody {
	w := weekBody{From: from.String(), To: to.String()}
	var kcal, protein, weights, steps, sleep []float64
	for d := from; !to.Before(d); d = d.AddDays(1) {
		sd := data.days[d]
		if sd.logged() {
			kcal, protein = append(kcal, sd.kcal), append(protein, sd.protein)
		}
		if sd.dayType == plan.Training {
			w.TrainingDays++
		}
		for _, v := range sd.values {
			switch v.Kind {
			case metric.Weight:
				weights = append(weights, v.Value)
			case metric.Steps:
				steps = append(steps, v.Value)
			case metric.Sleep:
				sleep = append(sleep, v.Value)
			default:
			}
		}
	}
	w.LoggedDays, w.WeighIns = len(kcal), len(weights)
	w.Kcal, w.Protein = averageOf(kcal, math.Round), averageOf(protein, roundGrams)
	w.WeightKg, w.Steps, w.SleepH = averageOf(weights, round2), averageOf(steps, math.Round), averageOf(sleep, round2)
	for _, pt := range trend {
		if to.Before(pt.Date) {
			break
		}
		w.TrendKg = new(round2(pt.Kg))
	}
	return w
}

// incomplete returns the complete days from from to to with meals that
// come under incompleteShare of their type's target.
func (data summaryData) incomplete(from, to day.Date) []string {
	out := []string{}
	for d := from; !to.Before(d); d = d.AddDays(1) {
		sd := data.days[d]
		if t, ok := plan.DayTargetOn(data.targets, d, sd.dayType); ok && sd.logged() && sd.kcal < incompleteShare*t.Kcal {
			out = append(out, d.String())
		}
	}
	return out
}

func newTDEEBody(e calibration.Estimate) tdeeBody {
	b := tdeeBody{
		Measured: e.Measured, Trusted: e.Trusted, BeforePhase: e.BeforePhase,
		From: e.From.String(), To: e.To.String(), LoggedDays: e.LoggedDays, WeighIns: e.WeighIns,
		Doubts: append([]string{}, e.Doubts...),
	}
	if e.Measured {
		b.Kcal, b.HalfWidth, b.KgPerWeek = new(math.Round(e.TDEE)), new(math.Round(e.HalfWidth)), new(round2(e.KgPerWeek))
	}
	return b
}

func newVerdictBody(v calibration.Verdict) verdictBody {
	b := verdictBody{
		Rate: v.Rate, Reasons: append([]string{}, v.Reasons...), Action: v.Action,
		Guards: append([]string{}, v.Guards...), Flags: append([]string{}, v.Flags...),
		GapKcal: noNegativeZero(math.Round(v.GapKcal)), StepKcal: noNegativeZero(math.Round(v.StepKcal)),
	}
	if v.Rate != calibration.RateNone {
		b.MeasuredPctWeek, b.TargetPctWeek, b.TolerancePctWeek = new(round2(v.MeasuredPctWeek)), new(v.TargetPctWeek), new(v.TolerancePctWeek)
		b.IntakeKcal, b.TargetKcal = new(math.Round(v.IntakeKcal)), new(math.Round(v.TargetKcal))
	}
	return b
}

func newRecoveryBody(r recovery.Recovery) recoveryBody {
	return recoveryBody{
		SleepH: positive(r.SleepH, round2), RestingHR: positive(r.RestingHR, math.Round), RestingHRBase: positive(r.RestingHRBase, math.Round),
		HRV: positive(r.HRV, math.Round), HRVBase: positive(r.HRVBase, math.Round), Flags: append([]string{}, r.Flags...),
	}
}

// averageOf returns the rounded average of vs, nil for none.
func averageOf(vs []float64, round func(float64) float64) *float64 {
	if len(vs) == 0 {
		return nil
	}
	var sum float64
	for _, v := range vs {
		sum += v
	}
	return new(round(sum / float64(len(vs))))
}

// positive returns v rounded, nil for 0, which is none.
func positive(v float64, round func(float64) float64) *float64 {
	if v <= 0 {
		return nil
	}
	return new(round(v))
}

// round2 rounds to a hundredth: kg, hours and rates as they are shown.
func round2(v float64) float64 { return noNegativeZero(math.Round(v*100) / 100) }
