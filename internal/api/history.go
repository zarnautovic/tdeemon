package api

import (
	"context"
	"maps"
	"math"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/domain/calibration"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// The span of a history: by default the 90 days up to today, at most a
// year and a day, and of no day before historySince.
const (
	historyDays    = 90
	maxHistoryDays = 366
)

// historySince is the first day a history can be of: no one's days are
// older, and a date before year 1 has no RFC 3339 form.
var historySince = day.Date{Year: 1900, Month: 1, Day: 1}

type historyInput struct {
	From        string `query:"from" format:"date" doc:"The first day; left out, 89 days before to."`
	To          string `query:"to" format:"date" doc:"The last day, today at the latest; left out, today."`
	Granularity string `query:"granularity" enum:"day,week" default:"day" doc:"day: a row a day; week: one for each 7 days ending on to, the first maybe starting before from. A week of today leaves today's steps out."`
}

type historyOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         historyBody
}

// historyBody is a user's days, or weeks, as series: what was eaten
// against the target, the measurements, the trend weight and the TDEE
// measured up to each day, as the summary computes them.
type historyBody struct {
	From  string           `json:"from" format:"date"`
	To    string           `json:"to" format:"date"`
	Days  []historyDayBody `json:"days" nullable:"false" doc:"For granularity day, every day from from to to, in order; else empty."`
	Weeks []weekBody       `json:"weeks" nullable:"false" doc:"For granularity week, the weeks, the latest last; else empty."`
}

type historyDayBody struct {
	Date           string         `json:"date" format:"date"`
	Status         string         `json:"status" enum:"open,complete,excluded"`
	ExcludedReason string         `json:"excluded_reason,omitempty" doc:"Why the user left the day out of calibration."`
	Type           string         `json:"type" enum:"training,rest"`
	Meals          int            `json:"meals"`
	Intake         *dayTotalBody  `json:"intake,omitempty" doc:"What the meals add up to; absent for a day without a meal, whose intake is unknown, not 0."`
	Target         *dayTargetBody `json:"target,omitempty" doc:"The target of the day's type; absent before the first target."`
	WeightKg       *float64       `json:"weight_kg,omitempty"`
	TrendKg        *float64       `json:"trend_kg,omitempty" doc:"The trend weight on the day, from the weigh-ins of the days over up to it: today's is the summary's, and takes today's weigh-in once the day is over; absent before the first."`
	BodyFatPct     *float64       `json:"body_fat_pct,omitempty"`
	LeanMassKg     *float64       `json:"lean_mass_kg,omitempty"`
	Steps          *float64       `json:"steps,omitempty"`
	ActiveKcal     *float64       `json:"active_kcal,omitempty"`
	SleepH         *float64       `json:"sleep_h,omitempty" doc:"The night that ended on the day."`
	RestingHR      *float64       `json:"resting_hr,omitempty"`
	HRV            *float64       `json:"hrv,omitempty"`
	TDEE           *dayTDEEBody   `json:"tdee,omitempty" doc:"The TDEE calibration measures over its window up to the day; of today, which is not over, up to yesterday, the summary's. Absent when it measures none."`
}

// dayTotalBody is what a day's meals add up to, rounded as shown.
type dayTotalBody struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Carbs   float64 `json:"carbs"`
	Fat     float64 `json:"fat"`
}

type dayTDEEBody struct {
	Kcal        float64 `json:"kcal" doc:"In kcal a day."`
	HalfWidth   float64 `json:"half_width" doc:"Half its 80 % interval."`
	Trusted     bool    `json:"trusted" doc:"Whether its interval and data are enough to trust it."`
	BeforePhase bool    `json:"before_phase" doc:"The estimate is of the days before the phase the day is in, whose own are too few yet: the same estimate for some weeks after the intake changed."`
	To          string  `json:"to" format:"date" doc:"The last day of its window."`
}

func (s *sessions) getHistory(ctx context.Context, in *historyInput) (*historyOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	cal, today, err := userDay(p, now)
	if err != nil {
		return nil, internalError("reading the history failed", err)
	}
	from, to, err := historySpan(in, today)
	if err != nil {
		return nil, err
	}
	// Calibration reads the days before the first in full too.
	full := calibration.FullDays(calibration.DefaultParams())
	data, err := s.readDays(ctx, p.User.ID, cal, from.AddDays(-full), to, to, now)
	if err != nil {
		return nil, internalError("reading the history failed", err)
	}
	b := historyBody{From: from.String(), To: to.String(), Days: []historyDayBody{}, Weeks: []weekBody{}}
	// Calibration reads the days that are over, as the summary does
	// (DESIGN.md §7): a window that ended on today would count its
	// weigh-in and not its intake. So today's trend weight and TDEE are
	// the summary's, up to yesterday, and its weigh-in joins the trend
	// once the day is over.
	over := maps.Clone(data.calDays)
	delete(over, today)
	trend := calibration.Trend(over, data.params.TrendAlpha)
	if in.Granularity == "week" {
		for end := to; !end.Before(from); end = end.AddDays(-7) {
			b.Weeks = append(b.Weeks, data.week(end.AddDays(-6), end, trend))
		}
		for i, j := 0, len(b.Weeks)-1; i < j; i, j = i+1, j-1 {
			b.Weeks[i], b.Weeks[j] = b.Weeks[j], b.Weeks[i]
		}
	} else {
		for d := from; !to.Before(d); d = d.AddDays(1) {
			b.Days = append(b.Days, data.historyDay(d, over, trend, today))
		}
	}
	return &historyOutput{CacheControl: "no-store", Body: b}, nil
}

// historySpan is the days a history asks for, up to today.
func historySpan(in *historyInput, today day.Date) (from, to day.Date, err error) {
	to = today
	if in.To != "" {
		if to, err = day.ParseDate(in.To); err != nil {
			return from, to, huma.Error422UnprocessableEntity("to: " + err.Error())
		}
	}
	from = to.AddDays(1 - historyDays)
	if in.From != "" {
		if from, err = day.ParseDate(in.From); err != nil {
			return from, to, huma.Error422UnprocessableEntity("from: " + err.Error())
		}
	}
	switch {
	case today.Before(to):
		return from, to, huma.Error422UnprocessableEntity("to: " + to.String() + " is a day to come; a history is of today or the days before")
	case to.Before(from):
		return from, to, huma.Error422UnprocessableEntity("from: " + from.String() + " is after to, " + to.String())
	case from.Before(historySince):
		return from, to, huma.Error422UnprocessableEntity("from: a history is of " + historySince.String() + " or after")
	case to.DaysSince(from) >= maxHistoryDays:
		return from, to, huma.Error422UnprocessableEntity("from: a history is of at most 366 days")
	}
	return from, to, nil
}

// historyDay is day d of data, with the trend weight of trend and the TDEE
// calibration measures of the days over up to it: of today, up to
// yesterday.
func (data summaryData) historyDay(d day.Date, over map[day.Date]calibration.Day, trend []calibration.Point, today day.Date) historyDayBody {
	c, sd := data.calDays[d], data.days[d]
	b := historyDayBody{Date: d.String(), Status: string(c.Status), Type: string(sd.dayType), Meals: c.Meals}
	if ex, ok := data.exclusions[d]; ok {
		b.ExcludedReason = ex.Reason
	}
	if c.Meals > 0 {
		n := food.Nutrients{Kcal: c.Kcal, Protein: sd.protein, Carbs: sd.carbs, Fat: sd.fat}.Rounded()
		b.Intake = &dayTotalBody{Kcal: n.Kcal, Protein: n.Protein, Carbs: n.Carbs, Fat: n.Fat}
	}
	if t, ok := plan.DayTargetOn(data.targets, d, sd.dayType); ok {
		b.Target = new(newDayTargetBody(t))
	}
	if kg, _ := calibration.TrendAt(trend, d); kg > 0 {
		b.TrendKg = new(round2(kg))
	}
	b.measure(data.values[d])
	last := d
	if !d.Before(today) {
		last = today.AddDays(-1)
	}
	if e := calibration.Calibrate(over, data.phases, last, data.params); e.Measured {
		b.TDEE = &dayTDEEBody{
			Kcal: noNegativeZero(math.Round(e.TDEE)), HalfWidth: math.Round(e.HalfWidth), Trusted: e.Trusted,
			BeforePhase: e.BeforePhase, To: e.To.String(),
		}
	}
	return b
}

// measure puts the day's measurements in b, rounded as they are shown; a
// value of 0 is none, but steps'.
func (b *historyDayBody) measure(values []metric.Value) {
	for _, v := range values {
		if v.Value <= 0 && v.Kind != metric.Steps {
			continue
		}
		switch v.Kind {
		case metric.Weight:
			b.WeightKg = new(round2(v.Value))
		case metric.BodyFat:
			b.BodyFatPct = new(round2(v.Value))
		case metric.LeanMass:
			b.LeanMassKg = new(round2(v.Value))
		case metric.Steps:
			b.Steps = new(math.Round(v.Value))
		case metric.ActiveKcal:
			b.ActiveKcal = new(math.Round(v.Value))
		case metric.Sleep:
			b.SleepH = new(round2(v.Value))
		case metric.RestingHR:
			b.RestingHR = new(math.Round(v.Value))
		case metric.HRV:
			b.HRV = new(math.Round(v.Value))
		default:
		}
	}
}
