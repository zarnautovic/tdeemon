package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/autopilot"
	"github.com/zarnautovic/tdeemon/internal/domain/calibration"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/notify"
	"github.com/zarnautovic/tdeemon/internal/scheduler"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// reviewProposals is how many of a user's latest proposals the review
// reads its history from: a phase of a year has some 50 weekly steps,
// and as many proposals of a coach's.
const reviewProposals = 500

// errReviewChanged reports a review whose decision the data or the plan
// changed between the review and its write: the job tries again.
var errReviewChanged = errors.New("the review's decision changed before it was stored")

// AutopilotReviewJob is the autopilot's weekly review, the scheduler's
// job: each morning at 09:00 on each user's clock, before the morning
// summary, (or up to 5 hours later, a server down then catching up), on
// the user's review weekday, it decides the review (autopilot.Decide),
// applies its step or makes its proposal and records it
// (store.DB.SaveReview), and tells the user's channels, but for a
// review that repeats the last one. A day reviewed already is left as it
// is: the job ran for it before a crash kept its run from being
// recorded.
func AutopilotReviewJob(cfg Config, bot *telegram.Bot) scheduler.Job {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &sessions{db: cfg.DB, logger: cfg.Logger, now: cfg.Now, instance: cfg.Limits}
	channels := notify.Channels{DB: cfg.DB, Bot: bot, Push: cfg.Push, Logger: cfg.Logger}
	return scheduler.Job{
		Name: "autopilot_review",
		Due:  scheduler.DailyAt(9*time.Hour, 5*time.Hour),
		Run: func(ctx context.Context, u user.User, occurrence string, now time.Time) error {
			d, err := day.ParseDate(occurrence)
			if err != nil {
				return err
			}
			settings, err := s.db.AutopilotSettings(ctx, u.ID)
			if err != nil || d.Weekday() != settings.Weekday {
				return err
			}
			r, last, err := s.runReview(ctx, u, d, now)
			if errors.Is(err, store.ErrReviewed) {
				return nil
			}
			if err != nil {
				return err
			}
			s.logger.InfoContext(ctx, "reviewed the plan", "user_id", u.ID.String(),
				"outcome", string(r.Review.Outcome), "reason", string(r.Review.Reason))
			if last != nil && r.Review.Repeats(last.Review) {
				return nil
			}
			if ok, err := channels.Has(ctx, u); err != nil || !ok {
				return err
			}
			if _, err := channels.Send(ctx, u, reviewNote(r, settings, u.Locale), now); err != nil {
				// The review is done and stored; its message is not sent
				// again, as the plan's card says it.
				s.logger.WarnContext(ctx, "the weekly review's message failed", "user_id", u.ID.String(), "error", err)
			}
			return nil
		},
	}
}

// reviewInput is what the review of p's user reads at now, on the day
// today: the data, the plan and the limits as the proposals' check reads
// them, the settings, and the autopilot's history and proposals.
func (s *sessions) reviewInput(ctx context.Context, p auth.Principal, settings autopilot.Settings, now time.Time) (autopilot.Input, error) {
	cs, err := s.coachState(ctx, p, now)
	if err != nil {
		return autopilot.Input{}, err
	}
	cal, today, err := userDay(p, now)
	if err != nil {
		return autopilot.Input{}, err
	}
	st := cs.state
	in := autopilot.Input{
		Today: today, Settings: settings, Estimate: cs.estimate, Expected: cs.expected,
		State: st, Levels: cs.levels, Phases: cs.phases, Targets: cs.targets,
	}
	in.Verdict = calibration.Judge(cs.estimate, st.Phase, st.Target, st.TrainingDays, st.Profile,
		limits.Effective(cs.levels.User, cs.levels.Instance), cs.params)
	ps, err := s.db.Proposals(ctx, p.User.ID, reviewProposals, now)
	if err != nil {
		return autopilot.Input{}, err
	}
	if st.Phase != nil {
		in.History = autopilot.HistoryOf(autopilot.RevisionsOf(ps, cal, st.TrainingDays, cs.targets), *st.Phase)
	}
	if in.Rejected, err = s.rejected(ctx, p.User.ID, ps); err != nil {
		return autopilot.Input{}, err
	}
	for i := range ps {
		if pr := &ps[i]; pr.Status == proposal.Pending {
			in.Waiting = autopilot.WaitsOther
			if pr.By.Kind == event.Autopilot {
				in.Waiting = autopilot.WaitsOwn
			}
			break
		}
	}
	return in, nil
}

// rejected returns the autopilot's last proposal the user rejected, of
// ps (newest first), when no proposal was applied since; nil for none.
func (s *sessions) rejected(ctx context.Context, userID uuid.UUID, ps []proposal.Proposal) (*autopilot.Rejected, error) {
	for i := range ps {
		switch p := &ps[i]; {
		case p.Status == proposal.Applied || p.Status == proposal.Reverted:
			return nil, nil
		case p.Status == proposal.Rejected && p.By.Kind == event.Autopilot:
			reviews, err := s.db.ReviewsOf(ctx, userID, []uuid.UUID{p.ID})
			if err != nil {
				return nil, err
			}
			r, ok := reviews[p.ID]
			if !ok {
				return nil, nil
			}
			return &autopilot.Rejected{Reason: r.Review.Reason, GapKcal: r.Review.GapKcal}, nil
		}
	}
	return nil, nil
}

// runReview decides u's review of today at now and stores it, with its
// step or proposal, and returns it, with the review before it (nil for
// none). The decision is taken again in the transaction that stores it,
// and must come out the same, so nothing that changed meanwhile slips
// past the limits.
func (s *sessions) runReview(ctx context.Context, u user.User, today day.Date, now time.Time) (store.StoredReview, *store.StoredReview, error) {
	p := auth.Principal{User: u}
	settings, err := s.db.AutopilotSettings(ctx, u.ID)
	if err != nil {
		return store.StoredReview{}, nil, err
	}
	before, err := s.db.Reviews(ctx, u.ID, 1)
	if err != nil {
		return store.StoredReview{}, nil, err
	}
	in, err := s.reviewInput(ctx, p, settings, now)
	if err != nil {
		return store.StoredReview{}, nil, err
	}
	if in.Today != today {
		return store.StoredReview{}, nil, fmt.Errorf("review of %s on %s", today, in.Today)
	}
	r := autopilot.Decide(in)
	stored, err := s.db.SaveReview(ctx, u.ID, r, trusted(in.Estimate), now, func(proposal.Changes) (float64, error) {
		again, err := s.reviewInput(ctx, p, settings, now)
		if err != nil {
			return 0, err
		}
		if a := autopilot.Decide(again); a.Outcome != r.Outcome || !reflect.DeepEqual(a.Changes, r.Changes) {
			return 0, errReviewChanged
		}
		return trusted(again.Estimate), nil
	})
	if err != nil {
		return store.StoredReview{}, nil, err
	}
	var last *store.StoredReview
	if len(before) > 0 {
		last = &before[0]
	}
	return stored, last, nil
}

// trusted returns the TDEE of e when calibration trusts it on the
// phase's own days, else 0.
func trusted(e calibration.Estimate) float64 {
	if e.Measured && e.Trusted && !e.BeforePhase && e.TDEE > 0 {
		return e.TDEE
	}
	return 0
}

// weekdayName is the name of a weekday as the API says it.
func weekdayName(d time.Weekday) string { return weekdayNames(plan.Weekdays(1 << d))[0] }

// reviewBody is a weekly review, with the numbers the app and the
// messages say it by.
type reviewBody struct {
	Date      string  `json:"date" format:"date" doc:"The review's day."`
	Outcome   string  `json:"outcome" enum:"applied,proposed,hold,skipped" doc:"applied: a step it took; proposed: a proposal that waits; hold: the target stays; skipped: no step, for reason."`
	Reason    string  `json:"reason" enum:"cut_slow,cut_fast,bulk_slow,bulk_fast,maintain_loss,maintain_gain,bulk_fat,lean_loss,underweight,on_target,intake,steps_guard,rejected,no_plan,not_trusted,first_wait,interval,few_data,pending,few_carbs,limit" doc:"Why (DESIGN.md §8)."`
	Rule      string  `json:"rule,omitempty" doc:"The limit's rule, for limit."`
	StepKcal  float64 `json:"step_kcal" doc:"The step applied or proposed, kcal a day of the weekly average, a minus being down; 0 for none."`
	GapKcal   float64 `json:"gap_kcal" doc:"The change of the weekly average the verdict asked for."`
	PastDrift bool    `json:"past_drift" doc:"Whether it proposes only because the step would take the drift past its limit."`
	// ProposalID is the proposal it made or applied.
	ProposalID       *string  `json:"proposal_id,omitempty" format:"uuid" doc:"The proposal it made or applied."`
	MeasuredPctWeek  float64  `json:"measured_pct_week" doc:"The measured rate, % of the trend weight a week."`
	TargetPctWeek    float64  `json:"target_pct_week" doc:"The phase's rate."`
	TolerancePctWeek float64  `json:"tolerance_pct_week"`
	TDEEKcal         float64  `json:"tdee_kcal" doc:"The TDEE, 0 for none."`
	TDEEHalfWidth    float64  `json:"tdee_half_width" doc:"Half its 80 % interval."`
	IntakeKcal       float64  `json:"intake_kcal" doc:"The window's average intake."`
	TargetKcal       float64  `json:"target_kcal" doc:"The weekly average of the target before."`
	AfterKcal        float64  `json:"after_kcal" doc:"The weekly average after the step, or as it stays."`
	LoggedDays       int      `json:"logged_days" doc:"The logged days of the last 7, an excluded day not counted."`
	WeighIns         int      `json:"weigh_ins"`
	NeedsLoggedDays  int      `json:"needs_logged_days"`
	NeedsWeighIns    int      `json:"needs_weigh_ins"`
	StepsChangePct   float64  `json:"steps_change_pct" doc:"How the last 7 days' steps differ from the weeks before, in %."`
	From             *string  `json:"from,omitempty" format:"date" doc:"The first review that may step, for a wait."`
	Expected         *string  `json:"expected,omitempty" format:"date" doc:"About when calibration expects a first estimate."`
	CarbsG           float64  `json:"carbs_g" doc:"For few_carbs: the carbs a step down would leave."`
	DayType          string   `json:"day_type,omitempty" enum:"training,rest" doc:"The type of day with those carbs; absent for an unsplit target."`
	CarbsFloorG      float64  `json:"carbs_floor_g" doc:"The carbs a step down may not go under."`
	BMI              float64  `json:"bmi" doc:"The BMI by the trend weight, 0 without a height."`
	DriftKcal        float64  `json:"drift_kcal" doc:"The drift as the review leaves it."`
	DriftLimitKcal   *float64 `json:"drift_limit_kcal,omitempty" doc:"Its limit; absent until there is a base."`
	Answer           string   `json:"answer,omitempty" enum:"accepted,rejected" doc:"The user's answer to its proposal, once given."`
}

func dateOrNil(d day.Date) *string {
	if d.IsZero() {
		return nil
	}
	return new(d.String())
}

func newReviewBody(sr store.StoredReview) reviewBody {
	r := sr.Review
	b := reviewBody{
		Date: r.Date.String(), Outcome: string(r.Outcome), Reason: string(r.Reason), Rule: r.Rule,
		StepKcal: r.StepKcal, GapKcal: round2(r.GapKcal), PastDrift: r.PastDrift,
		MeasuredPctWeek: round2(r.MeasuredPctWeek), TargetPctWeek: round2(r.TargetPctWeek), TolerancePctWeek: round2(r.TolerancePctWeek),
		TDEEKcal: round2(r.TDEE), TDEEHalfWidth: round2(r.HalfWidth), IntakeKcal: round2(r.IntakeKcal),
		TargetKcal: round2(r.TargetKcal), AfterKcal: round2(r.AfterKcal),
		LoggedDays: r.LoggedDays, WeighIns: r.WeighIns, NeedsLoggedDays: r.NeedsLoggedDays, NeedsWeighIns: r.NeedsWeighIns,
		StepsChangePct: round2(r.StepsChangePct), From: dateOrNil(r.From), Expected: dateOrNil(r.Expected),
		CarbsG: round2(r.CarbsG), DayType: string(r.DayType), CarbsFloorG: limits.CarbsRDA, BMI: round2(r.BMI),
		DriftKcal: round2(r.DriftKcal),
	}
	if r.DriftLimitKcal > 0 {
		b.DriftLimitKcal = new(round2(r.DriftLimitKcal))
	}
	if sr.ProposalID != uuid.Nil() {
		b.ProposalID = new(sr.ProposalID.String())
	}
	return b
}

// driftBody is the autopilot's drift from the last target the user
// agreed to.
type driftBody struct {
	UsedKcal   float64  `json:"used_kcal" doc:"The net change of the weekly average by its steps since, a minus being down."`
	LimitPct   float64  `json:"limit_pct" doc:"Its limit, in % of the base TDEE (the user's drift_pct)."`
	LimitKcal  *float64 `json:"limit_kcal,omitempty" doc:"The limit in kcal; absent until there is a base, from the first step."`
	BaseKcal   *float64 `json:"base_kcal,omitempty" doc:"The TDEE the limit is a share of: the one trusted when the user agreed to the target, else the one at the first step since."`
	BaseFrom   *string  `json:"base_from,omitempty" format:"date" doc:"The day it was taken."`
	AgreedOn   string   `json:"agreed_on" format:"date" doc:"The day of the last kcal target the user agreed to: the phase's start, or a change they applied."`
	AgreedKcal *float64 `json:"agreed_kcal,omitempty" doc:"The weekly average of the target in force; absent without one."`
}

// autopilotBody is the user's autopilot: its settings, when it reviews
// next, its step and drift, and its last review.
type autopilotBody struct {
	Mode      string      `json:"mode" enum:"propose,autopilot" doc:"propose: every step is a proposal; autopilot: a step within the limits applies by itself, past the drift limit it is proposed."`
	Weekday   string      `json:"weekday" enum:"sunday,monday,tuesday,wednesday,thursday,friday,saturday" doc:"The review's day of the week."`
	Next      string      `json:"next" format:"date" doc:"The next review."`
	FirstStep *string     `json:"first_step,omitempty" format:"date" doc:"The first review that may step, after the waits and the day calibration expects an estimate; absent without a phase."`
	StepKcal  float64     `json:"step_kcal" doc:"The largest step the limits allow now, kcal a day of the weekly average; 0 without a target."`
	Drift     driftBody   `json:"drift"`
	Last      *reviewBody `json:"last,omitempty" doc:"The last review; absent before the first."`
}

type autopilotOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         autopilotBody
}

type setAutopilotInput struct {
	Body struct {
		Mode    string `json:"mode,omitempty" enum:"propose,autopilot" doc:"Absent: as it is."`
		Weekday string `json:"weekday,omitempty" enum:"sunday,monday,tuesday,wednesday,thursday,friday,saturday" doc:"Absent: as it is."`
	}
}

func (s *sessions) registerAutopilot(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "get-autopilot",
		Method:      http.MethodGet,
		Path:        Prefix + "/autopilot",
		Summary:     "Get your autopilot",
		Description: "Needs a web session. The autopilot's settings, the next review and the first that may step, the step and the drift its limits allow, and its last review.",
		Tags:        []string{"plan"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.getAutopilot)
	huma.Register(a, huma.Operation{
		OperationID: "set-autopilot",
		Method:      http.MethodPut,
		Path:        Prefix + "/autopilot",
		Summary:     "Change your autopilot",
		Description: "Needs a web session, never an API token: how far the autopilot may go is the user's, as the limits are. " +
			"Sets the mode and the review's weekday; what is left out stays. Undo leaves it.",
		Tags:   []string{"plan"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, s.setAutopilot)
}

func (s *sessions) getAutopilot(ctx context.Context, _ *struct{}) (*autopilotOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	b, err := s.autopilot(ctx, p, s.now())
	if err != nil {
		return nil, internalError("reading the autopilot failed", err)
	}
	return &autopilotOutput{CacheControl: "no-store", Body: b}, nil
}

func (s *sessions) setAutopilot(ctx context.Context, in *setAutopilotInput) (*autopilotOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.db.AutopilotSettings(ctx, p.User.ID)
	if err != nil {
		return nil, internalError("reading the autopilot failed", err)
	}
	if in.Body.Mode != "" {
		settings.Mode = autopilot.Mode(in.Body.Mode)
	}
	if in.Body.Weekday != "" {
		w, ok := weekdayOf(in.Body.Weekday)
		if !ok {
			return nil, huma.Error422UnprocessableEntity("no such weekday: " + in.Body.Weekday)
		}
		settings.Weekday = w
	}
	if err := settings.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	now := s.now()
	if err := s.db.SetAutopilotSettings(ctx, p.User.ID, webActor, settings, now); err != nil {
		return nil, internalError("changing the autopilot failed", err)
	}
	s.logger.InfoContext(ctx, "changed the autopilot", "user_id", p.User.ID.String(), "mode", string(settings.Mode))
	b, err := s.autopilot(ctx, p, now)
	if err != nil {
		return nil, internalError("reading the autopilot failed", err)
	}
	return &autopilotOutput{CacheControl: "no-store", Body: b}, nil
}

// weekdayOf returns the weekday the API names name.
func weekdayOf(name string) (time.Weekday, bool) {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if weekdayName(d) == name {
			return d, true
		}
	}
	return 0, false
}

// autopilot is p's user's autopilot at now: the settings, the next
// review (today's while it has not run), the first that may step, the
// step and the drift, and the last review.
func (s *sessions) autopilot(ctx context.Context, p auth.Principal, now time.Time) (autopilotBody, error) {
	settings, err := s.db.AutopilotSettings(ctx, p.User.ID)
	if err != nil {
		return autopilotBody{}, err
	}
	in, err := s.reviewInput(ctx, p, settings, now)
	if err != nil {
		return autopilotBody{}, err
	}
	reviews, err := s.db.Reviews(ctx, p.User.ID, 1)
	if err != nil {
		return autopilotBody{}, err
	}
	lim := limits.Effective(in.Levels.User, in.Levels.Instance)
	next := autopilot.NextOn(in.Today, settings.Weekday)
	if len(reviews) > 0 && reviews[0].Review.Date == next {
		next = autopilot.NextOn(next.AddDays(1), settings.Weekday)
	}
	b := autopilotBody{
		Mode: string(settings.Mode), Weekday: weekdayName(settings.Weekday), Next: next.String(),
		Drift: driftBody{UsedKcal: round2(in.History.DriftKcal), LimitPct: lim.DriftPct},
	}
	if len(reviews) > 0 {
		b.Last = new(newReviewBody(reviews[0]))
	}
	if st := in.State; st.Target != nil {
		avg := st.Target.WeeklyAverage(st.TrainingDays.Count()).Kcal
		b.StepKcal = round2(lim.Step(avg))
		b.Drift.AgreedKcal = new(round2(avg - in.History.DriftKcal))
	}
	if in.State.Phase != nil {
		h := in.History
		b.Drift.AgreedOn = h.AgreedOn.String()
		b.FirstStep = new(autopilot.FirstStep(next, settings.Weekday, h, lim, trusted(in.Estimate) > 0, in.Expected).String())
		if h.BaseTDEE > 0 {
			b.Drift.BaseKcal = new(round2(h.BaseTDEE))
			b.Drift.BaseFrom = new(h.BaseFrom.String())
			b.Drift.LimitKcal = new(round2(lim.DriftPct / 100 * h.BaseTDEE))
		}
	} else {
		b.Drift.AgreedOn = in.Today.String()
	}
	return b, nil
}

// withCheckIns gives the autopilot's proposals among pending and
// revisions the reviews that made them, with the user's answer.
func (s *sessions) withCheckIns(ctx context.Context, userID uuid.UUID, pending *proposalBody, revisions []proposalBody) error {
	bodies := make([]*proposalBody, 0, len(revisions)+1)
	if pending != nil {
		bodies = append(bodies, pending)
	}
	for i := range revisions {
		bodies = append(bodies, &revisions[i])
	}
	var ids []uuid.UUID
	for _, b := range bodies {
		if b.By.Client == event.ClientAutopilot {
			if id, err := uuid.Parse(b.ID); err == nil {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	reviews, err := s.db.ReviewsOf(ctx, userID, ids)
	if err != nil {
		return err
	}
	for _, b := range bodies {
		id, err := uuid.Parse(b.ID)
		if err != nil {
			continue
		}
		r, ok := reviews[id]
		if !ok {
			continue
		}
		c := newReviewBody(r)
		c.Answer = answerOf(b)
		b.CheckIn = &c
	}
	return nil
}

// answerOf is the user's answer to the autopilot's proposal b: rejected,
// accepted (applied by someone else than the autopilot), or none.
func answerOf(b *proposalBody) string {
	switch {
	case b.Status == string(proposal.Rejected):
		return "rejected"
	case b.DecidedBy != nil && b.DecidedBy.Client != event.ClientAutopilot &&
		(b.Status == string(proposal.Applied) || b.Status == string(proposal.Reverted)):
		return "accepted"
	}
	return ""
}
