package api

import (
	"context"
	"math"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/calibration"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
)

// planDays is how far back get_plan reaches: the phases and the targets
// in force within the last 365 days, and what came of them.
const planDays = 365

// planRevisions is the most revisions get_plan gives.
const planRevisions = 10

type planOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         planBody
}

// planBody is the plan as a coach reads it (DESIGN.md §8): the goal
// timeline, the targets with what each came to, the limits a change must
// keep, and the proposals, waiting and applied.
type planBody struct {
	Today            string             `json:"today" format:"date" doc:"The user's day now. The outcomes are of the days up to yesterday, as today is open."`
	Phases           []phaseBody        `json:"phases" nullable:"false" doc:"The goal timeline, in order: the phases in force within the last 365 days, and those to come."`
	TrainingWeekdays []string           `json:"training_weekdays" nullable:"false" enum:"sunday,monday,tuesday,wednesday,thursday,friday,saturday" doc:"The days of the week the plan makes training days."`
	Targets          []targetPeriodBody `json:"targets" nullable:"false" doc:"The targets, in order: those in force within the last 365 days, and those to come, each with what its days came to."`
	Limits           map[string]float64 `json:"limits" doc:"The limits a change of the plan must keep, by name: for each, the stricter of the user's and the instance's (docs/limits.md). The server refuses a change past them; a proposal may change the user's, up to the instance's, which the user accepts in the app."`
	Pending          *proposalBody      `json:"pending,omitempty" doc:"The proposal that waits for the user's consent; absent when none does."`
	PendingPreview   *previewBody       `json:"pending_preview,omitempty" doc:"What the pending proposal would come to, by the data and the limits now, as propose_changes previews it; with what the limits would refuse, if they changed since."`
	Revisions        []proposalBody     `json:"revisions" nullable:"false" doc:"The plan's latest revisions, the proposals applied (reverted ones too), newest first, up to 10."`
}

// targetPeriodBody is a target, from its first day to the day before the
// next, with what its days came to.
type targetPeriodBody struct {
	EffectiveFrom string        `json:"effective_from" format:"date" doc:"The first day the targets hold."`
	To            *string       `json:"to,omitempty" format:"date" doc:"The last day they hold, the day before the next takes over; absent for the last."`
	Training      dayTargetBody `json:"training" doc:"On a training day."`
	Rest          dayTargetBody `json:"rest" doc:"On a rest day."`
	Outcome       *outcomeBody  `json:"outcome,omitempty" doc:"What its days up to yesterday, of the last 365, came to; absent for a target still to come."`
}

// outcomeBody is what a target's days came to: how they were eaten and
// what calibration measures of them.
type outcomeBody struct {
	From            string   `json:"from" format:"date" doc:"The first day counted: the target's, or 365 days before today."`
	To              string   `json:"to" format:"date" doc:"The last day counted: the target's, or yesterday."`
	Days            int      `json:"days"`
	LoggedDays      int      `json:"logged_days" doc:"The complete days with a meal, not excluded."`
	WeighIns        int      `json:"weigh_ins" doc:"The weigh-ins of the days not excluded."`
	IntakeKcal      *float64 `json:"intake_kcal,omitempty" doc:"The average intake of the logged days."`
	TargetKcal      *float64 `json:"target_kcal,omitempty" doc:"The average target of the logged days, each day's type's: what the intake is held to."`
	IntakeProtein   *float64 `json:"intake_protein,omitempty" doc:"The average grams of protein of the logged days."`
	TargetProtein   *float64 `json:"target_protein,omitempty" doc:"The average target of protein of the logged days."`
	Phase           string   `json:"phase,omitempty" enum:"cut,maintain,bulk,recomp" doc:"The phase of the last day counted; absent without one."`
	PlannedPctWeek  *float64 `json:"planned_pct_week,omitempty" doc:"That phase's target rate, in % of the trend weight a week, a minus being a loss."`
	MeasuredFrom    string   `json:"measured_from" format:"date" doc:"The first day measured: the first 14 days after a change of intake (a phase's start that changes it by more than a tenth) are left out, as water and glycogen move the weight then; the day after to when no day is left."`
	Measured        bool     `json:"measured" doc:"Whether the days measured held enough for an estimate; doubts say what is missing."`
	TDEE            *float64 `json:"tdee,omitempty" doc:"The TDEE measured over them, in kcal a day."`
	HalfWidth       *float64 `json:"half_width,omitempty" doc:"Half its 80 % interval."`
	Trusted         bool     `json:"trusted" doc:"Whether its interval and data are enough to trust it."`
	KgPerWeek       *float64 `json:"kg_per_week,omitempty" doc:"The weight's slope over them, a minus being a loss."`
	MeasuredPctWeek *float64 `json:"measured_pct_week,omitempty" doc:"The measured rate, in % of the trend weight a week: against planned_pct_week, what the target came to."`
	Doubts          []string `json:"doubts" nullable:"false" doc:"Why the estimate is missing or not trusted: few_days, few_weigh_ins, few_logged_days, wide_interval."`
}

// proposalBody is a proposed change of the plan, and where it is in its
// life.
type proposalBody struct {
	ID         string      `json:"id" format:"uuid"`
	Status     string      `json:"status" enum:"pending,applied,rejected,expired,superseded,reverted" doc:"pending waits for the user's consent, for a week; superseded is one the plan changed under, or a newer one replaced; reverted is an applied one a later one took back."`
	Rationale  string      `json:"rationale" doc:"Why it was proposed: the proposer's words, or a code of the app, which it says in the user's language: changed in the app (the user's own change there), taken back in the app (a revision the user took back there)."`
	By         actorBody   `json:"by" doc:"Who proposed it."`
	Window     *windowBody `json:"window,omitempty" doc:"The days it was drawn from."`
	Changes    changesBody `json:"changes"`
	CreatedAt  time.Time   `json:"created_at"`
	ExpiresAt  time.Time   `json:"expires_at" doc:"When it expires, if it still waits then."`
	DecidedAt  *time.Time  `json:"decided_at,omitempty" doc:"When it stopped waiting: applied, rejected, or superseded."`
	DecidedBy  *actorBody  `json:"decided_by,omitempty" doc:"Who applied it, rejected it, or changed the plan under it."`
	Reverts    *string     `json:"reverts,omitempty" format:"uuid" doc:"The applied proposal it takes back."`
	RevertedBy *string     `json:"reverted_by,omitempty" format:"uuid" doc:"The proposal that took it back."`
}

// actorBody is who made a proposal or decided it: the client, and the
// model that acted for the user.
type actorBody struct {
	Client string `json:"client" doc:"What it came through: mcp, web, telegram."`
	Model  string `json:"model,omitempty" doc:"The AI model that acted for the user."`
}

type windowBody struct {
	From string `json:"from" format:"date"`
	To   string `json:"to" format:"date"`
}

// changesBody is what a proposal changes, each change with the plan
// before it and after.
type changesBody struct {
	Phases           []phaseChangeBody   `json:"phases" nullable:"false" doc:"The phases it changes, by their start."`
	Targets          []targetChangeBody  `json:"targets" nullable:"false" doc:"The targets it changes, by their first day."`
	TrainingWeekdays *weekdaysChangeBody `json:"training_weekdays,omitempty" doc:"The training weekdays, when it changes them."`
	Limits           []limitChangeBody   `json:"limits" nullable:"false" doc:"The user's limits it changes, by name: a proposal with one applies only in the app."`
}

type limitChangeBody struct {
	Name    string  `json:"name" doc:"The limit's name (docs/limits.md)."`
	Before  float64 `json:"before"`
	After   float64 `json:"after"`
	Loosens bool    `json:"loosens" doc:"Whether after is looser than before: applying it takes the user's confirmation."`
}

type phaseChangeBody struct {
	Start  string     `json:"start" format:"date"`
	Before *phaseBody `json:"before,omitempty" doc:"The phase before; absent for a new one."`
	After  *phaseBody `json:"after,omitempty" doc:"The phase after; absent for a delete."`
}

type targetValuesBody struct {
	Training dayTargetBody `json:"training"`
	Rest     dayTargetBody `json:"rest"`
}

type targetChangeBody struct {
	From   string            `json:"from" format:"date"`
	Before *targetValuesBody `json:"before,omitempty" doc:"The target before; absent for a new one."`
	After  *targetValuesBody `json:"after,omitempty" doc:"The target after; absent for a delete."`
}

type weekdaysChangeBody struct {
	Before []string `json:"before" nullable:"false" enum:"sunday,monday,tuesday,wednesday,thursday,friday,saturday"`
	After  []string `json:"after" nullable:"false" enum:"sunday,monday,tuesday,wednesday,thursday,friday,saturday"`
}

func (s *sessions) getPlan(ctx context.Context, _ *struct{}) (*planOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	cal, today, err := userDay(p, now)
	if err != nil {
		return nil, internalError("reading the plan failed", err)
	}
	yesterday, since := today.AddDays(-1), today.AddDays(-planDays)
	// An outcome from since reads the intake before it too.
	lead := calibration.LeadDays(calibration.DefaultParams())
	data, err := s.readDays(ctx, p.User.ID, cal, since.AddDays(-lead), yesterday, yesterday, now)
	if err != nil {
		return nil, internalError("reading the plan failed", err)
	}
	pending, err := s.db.Proposals(ctx, p.User.ID, 1, now, proposal.Pending)
	if err != nil {
		return nil, internalError("reading the plan failed", err)
	}
	revisions, err := s.db.Proposals(ctx, p.User.ID, planRevisions, now, proposal.Applied, proposal.Reverted)
	if err != nil {
		return nil, internalError("reading the plan failed", err)
	}
	b := planBody{
		Today: today.String(), Phases: planPhases(data.phases, since), TrainingWeekdays: weekdayNames(data.training),
		Targets: data.targetPeriods(since, yesterday), Limits: limitsMap(data.lim, true), Revisions: []proposalBody{},
	}
	loc := cal.Location
	if len(pending) > 0 && pending[0].Status == proposal.Pending {
		b.Pending = new(newProposalBody(pending[0], loc))
		cs, err := s.coachState(ctx, p, now)
		if err != nil {
			return nil, internalError("reading the plan failed", err)
		}
		b.PendingPreview = new(cs.preview(pending[0].Changes.Check(cs.state, cs.levels, cs.phases, cs.targets)))
	}
	for i := range revisions {
		b.Revisions = append(b.Revisions, newProposalBody(revisions[i], loc))
	}
	return &planOutput{CacheControl: "no-store", Body: b}, nil
}

// planPhases returns the phases of the timeline phases in force on since
// or after.
func planPhases(phases []plan.Phase, since day.Date) []phaseBody {
	out := []phaseBody{}
	for i, ph := range phases {
		if end := phaseEnd(phases, i); end.IsZero() || !end.Before(since) {
			out = append(out, *newPhaseBody(ph))
		}
	}
	return out
}

// targetPeriods returns data's targets in force on since or after, each
// with the outcome of its days from since to yesterday.
func (data summaryData) targetPeriods(since, yesterday day.Date) []targetPeriodBody {
	out := []targetPeriodBody{}
	trend := calibration.Trend(data.calDays, data.params.TrendAlpha)
	for i, t := range data.targets {
		var end day.Date
		if i+1 < len(data.targets) {
			end = data.targets[i+1].EffectiveFrom.AddDays(-1)
		}
		if !end.IsZero() && end.Before(since) {
			continue
		}
		tb := targetPeriodBody{EffectiveFrom: t.EffectiveFrom.String(), Training: newDayTargetBody(t.Training), Rest: newDayTargetBody(t.Rest)}
		if !end.IsZero() {
			tb.To = new(end.String())
		}
		if !yesterday.Before(t.EffectiveFrom) {
			from, to := t.EffectiveFrom, yesterday
			if from.Before(since) {
				from = since
			}
			if !end.IsZero() && end.Before(to) {
				to = end
			}
			tb.Outcome = data.outcome(from, to, trend)
		}
		out = append(out, tb)
	}
	return out
}

// phaseEnd returns the last day of phases[i]: its planned end, or the day
// before the next starts; the zero Date for the last phase without an
// end.
func phaseEnd(phases []plan.Phase, i int) day.Date {
	if end := phases[i].End; !end.IsZero() || i+1 == len(phases) {
		return end
	}
	return phases[i+1].Start.AddDays(-1)
}

// outcome is what the days from to to of data came to, with the trend
// weight of trend.
func (data summaryData) outcome(from, to day.Date, trend []calibration.Point) *outcomeBody {
	o := &outcomeBody{From: from.String(), To: to.String(), Days: to.DaysSince(from) + 1, Doubts: []string{}}
	var kcal, protein, targetKcal, targetProtein float64
	for d := from; !to.Before(d); d = d.AddDays(1) {
		c, sd := data.calDays[d], data.days[d]
		if c.Weighed() {
			o.WeighIns++
		}
		if !c.Logged() {
			continue
		}
		o.LoggedDays++
		kcal, protein = kcal+c.Kcal, protein+sd.protein
		if t, ok := plan.DayTargetOn(data.targets, d, sd.dayType); ok {
			targetKcal, targetProtein = targetKcal+t.Kcal, targetProtein+t.Protein
		}
	}
	if n := float64(o.LoggedDays); n > 0 {
		o.IntakeKcal, o.IntakeProtein = new(math.Round(kcal/n)), new(roundGrams(protein/n))
		o.TargetKcal, o.TargetProtein = new(math.Round(targetKcal/n)), new(roundGrams(targetProtein/n))
	}
	if ph, ok := plan.PhaseOn(data.phases, to); ok {
		o.Phase, o.PlannedPctWeek = string(ph.Type), new(ph.RatePctWeek)
	}
	e := calibration.Measure(data.calDays, data.phases, from, to, data.params)
	o.MeasuredFrom, o.Measured, o.Trusted = e.From.String(), e.Measured, e.Trusted
	o.Doubts = append(o.Doubts, e.Doubts...)
	if e.Measured {
		o.TDEE, o.HalfWidth = new(noNegativeZero(math.Round(e.TDEE))), new(math.Round(e.HalfWidth))
		o.KgPerWeek = new(round2(e.KgPerWeek))
		if kg, _ := calibration.TrendAt(trend, e.To); kg > 0 {
			o.MeasuredPctWeek = new(round2(plan.RatePctWeek(e.KgPerWeek, kg)))
		}
	}
	return o
}

// weekdayNames returns the days of w by their names in lower case, from
// Sunday.
func weekdayNames(w plan.Weekdays) []string {
	out := []string{}
	for _, d := range w.Days() {
		out = append(out, strings.ToLower(d.String()))
	}
	return out
}

// newProposalBody is p, its times on the clock of loc.
func newProposalBody(p proposal.Proposal, loc *time.Location) proposalBody {
	b := proposalBody{
		ID: p.ID.String(), Status: string(p.Status), Rationale: p.Rationale, By: newActorBody(p.By),
		Changes: newChangesBody(p.Changes), CreatedAt: p.CreatedAt.In(loc), ExpiresAt: p.ExpiresAt.In(loc),
	}
	if w := p.Window; w != nil {
		b.Window = &windowBody{From: w.From.String(), To: w.To.String()}
	}
	if !p.DecidedAt.IsZero() {
		b.DecidedAt = new(p.DecidedAt.In(loc))
	}
	if p.DecidedBy.Client != "" {
		b.DecidedBy = new(newActorBody(p.DecidedBy))
	}
	if p.Reverts != uuid.Nil() {
		b.Reverts = new(p.Reverts.String())
	}
	if p.RevertedBy != uuid.Nil() {
		b.RevertedBy = new(p.RevertedBy.String())
	}
	return b
}

func newActorBody(a event.Actor) actorBody { return actorBody{Client: a.Client, Model: a.Model} }

func newChangesBody(c proposal.Changes) changesBody {
	b := changesBody{Phases: []phaseChangeBody{}, Targets: []targetChangeBody{}, Limits: []limitChangeBody{}}
	phase := func(p *plan.Phase) *phaseBody {
		if p == nil {
			return nil
		}
		return newPhaseBody(*p)
	}
	target := func(t *plan.Target) *targetValuesBody {
		if t == nil {
			return nil
		}
		return &targetValuesBody{Training: newDayTargetBody(t.Training), Rest: newDayTargetBody(t.Rest)}
	}
	for _, ch := range c.Phases {
		b.Phases = append(b.Phases, phaseChangeBody{Start: ch.Start.String(), Before: phase(ch.Before), After: phase(ch.After)})
	}
	for _, ch := range c.Targets {
		b.Targets = append(b.Targets, targetChangeBody{From: ch.From.String(), Before: target(ch.Before), After: target(ch.After)})
	}
	if w := c.TrainingWeekdays; w != nil {
		b.TrainingWeekdays = &weekdaysChangeBody{Before: weekdayNames(w.Before), After: weekdayNames(w.After)}
	}
	loosened := c.Loosened()
	for _, ch := range c.Limits {
		b.Limits = append(b.Limits, limitChangeBody{Name: ch.Name, Before: ch.Before, After: ch.After, Loosens: slices.Contains(loosened, ch.Name)})
	}
	return b
}
