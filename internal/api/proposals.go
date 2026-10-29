package api

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/calibration"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

type proposeInput struct {
	Body proposeInputBody
}

// proposeInputBody is a proposal as a client makes it.
type proposeInputBody struct {
	Rationale        string             `json:"rationale" minLength:"1" maxLength:"4000" doc:"Why: what the data show, what you agreed."`
	Model            string             `json:"model,omitempty" maxLength:"100" doc:"The AI model you are."`
	Window           *windowBody        `json:"window,omitempty" doc:"The days you drew it from."`
	Phases           []phaseChangeIn    `json:"phases,omitempty"`
	Targets          []targetChangeIn   `json:"targets,omitempty"`
	TrainingWeekdays []string           `json:"training_weekdays,omitempty" enum:"sunday,monday,tuesday,wednesday,thursday,friday,saturday" doc:"From now on; [] for none; left out, as they are."`
	Limits           map[string]float64 `json:"limits,omitempty" doc:"Limits to change: get_plan's names, new values. Only the app applies these."`
}

// phaseChangeIn is a phase a proposal sets, or deletes.
type phaseChangeIn struct {
	Start           string   `json:"start" format:"date" doc:"Edits the phase from it, or adds one."`
	Delete          bool     `json:"delete,omitempty"`
	Type            string   `json:"type,omitempty" enum:"cut,maintain,bulk,recomp"`
	End             string   `json:"end,omitempty" format:"date" doc:"Left out: until the next."`
	RatePctWeek     float64  `json:"rate_pct_week,omitempty" doc:"% of the trend weight a week, a minus a loss."`
	GoalWeight      *float64 `json:"goal_weight,omitempty"`
	MilestoneWeight *float64 `json:"milestone_weight,omitempty"`
}

// targetChangeIn is a target a proposal sets, or deletes.
type targetChangeIn struct {
	From     string       `json:"from" format:"date" doc:"Today or later."`
	Delete   bool         `json:"delete,omitempty"`
	Training *dayTargetIn `json:"training,omitempty" doc:"Required unless delete."`
	Rest     *dayTargetIn `json:"rest,omitempty" doc:"Left out: as training."`
}

// dayTargetIn is a day type's target: carbs are what the kcal leave.
type dayTargetIn struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
}

type changePlanInput struct {
	Body changePlanBody
}

// changePlanBody is the user's own change of the plan, in the app: the
// phases and the targets as a proposal sets them.
type changePlanBody struct {
	Phases  []phaseChangeIn  `json:"phases,omitempty"`
	Targets []targetChangeIn `json:"targets,omitempty"`
}

type proposalIDInput struct {
	ID string `path:"id" format:"uuid" doc:"The proposal's id, as get_plan or propose_changes gave it."`
}

type applyInput struct {
	ID   string     `path:"id" format:"uuid" doc:"The proposal's id, as get_plan or propose_changes gave it."`
	Body *applyBody `doc:"The web app's alone."`
}

// applyBody is the user's confirmation of the limits a proposal loosens,
// which the web app asks for, as it does for a change of the limits.
type applyBody struct {
	Confirm bool `json:"confirm,omitempty" doc:"Loosening a limit takes this: the user saw that the proposal allows a plan the limit refused."`
}

// errLimitsInTheApp refuses a proposal of the limits from anything but a
// web session: no tool or token changes the limits (DESIGN.md §8).
var errLimitsInTheApp = huma.Error403Forbidden("a proposal that changes the limits applies only in the app: " +
	"the user accepts it there, under Settings, Plan")

type revertInput struct {
	ID   string `path:"id" format:"uuid" doc:"The applied proposal to take back."`
	Body struct {
		Rationale string `json:"rationale" minLength:"1" maxLength:"4000"`
		Model     string `json:"model,omitempty" maxLength:"100"`
	}
}

type proposedOutput struct {
	Body proposedBody
}

// proposedBody is a proposal made, and what it would come to.
type proposedBody struct {
	Proposal proposalBody `json:"proposal"`
	Preview  previewBody  `json:"preview"`
}

type proposalOutput struct {
	Body struct {
		Proposal proposalBody `json:"proposal"`
	}
}

// previewBody is what a proposal would come to, by the data now: what the
// limits checked, and the rate each target implies.
type previewBody struct {
	TrendKg *float64    `json:"trend_kg,omitempty" doc:"The trend weight the limits and the rates take."`
	TDEE    *float64    `json:"tdee,omitempty" doc:"The measured TDEE the expected rates take; absent while calibration trusts none."`
	Checks  []checkBody `json:"checks" nullable:"false" doc:"What the limits checked: the user's limits it changes, against the instance's; each phase it sets, the target in force from each day it changes one from, and today's with new training weekdays, against the limits it leaves."`
}

type checkBody struct {
	Of              string             `json:"of" enum:"phase,target,training_weekdays,limits"`
	Day             string             `json:"day" format:"date" doc:"The phase's start, the target's day, or today for the training weekdays alone and the limits."`
	WeeklyKcal      *float64           `json:"weekly_kcal,omitempty" doc:"The target's average day of a week, over the training weekdays."`
	ExpectedPctWeek *float64           `json:"expected_pct_week,omitempty" doc:"The rate it implies with the measured TDEE, in % of the trend weight a week."`
	ExpectedKgWeek  *float64           `json:"expected_kg_week,omitempty" doc:"The same in kg a week."`
	Warnings        []limitWarningBody `json:"warnings" nullable:"false" doc:"What is past a threshold without refusing it: below_bmr, low_energy_availability, low_carbs."`
	Violations      []violationValue   `json:"violations" nullable:"false" doc:"What the limits refuse now: for a pending proposal, what would refuse it if applied."`
}

type limitWarningBody struct {
	Rule      string  `json:"rule" enum:"below_bmr,low_energy_availability,low_carbs"`
	DayType   string  `json:"day_type,omitempty" enum:"training,rest" doc:"Absent for an unsplit target."`
	Threshold float64 `json:"threshold"`
	Value     float64 `json:"value"`
}

// violationValue is a violation of the limits, as a refusal's error
// detail gives it (DESIGN.md §8): the rule, the type of day, the limit
// that holds for the user, the value proposed, in the rule's units, and
// the level that refuses it, so it is clear who could allow it.
type violationValue struct {
	Rule     string  `json:"rule"`
	DayType  string  `json:"day_type,omitempty"`
	Limit    float64 `json:"limit"`
	Proposed float64 `json:"proposed"`
	Level    string  `json:"level,omitempty"`
}

// errNoProposal answers an id the user has no proposal under.
var errNoProposal = huma.Error404NotFound("no proposal under this id")

func newViolationValue(v limits.Violation) violationValue {
	return violationValue{Rule: v.Rule, DayType: string(v.DayType), Limit: round2(v.Limit), Proposed: round2(v.Proposed), Level: string(v.Level)}
}

// refusedError is a proposal's changes past the limits: the checks that
// found violations.
type refusedError struct{ checks []proposal.Checked }

func (e *refusedError) Error() string { return "the proposal breaks the limits" }

// coachState is what the limits hold a change of a user's plan to:
// calibration's state, as the summary reads it but for the trend weight,
// which takes today's weigh-in too; the levels of the limits; and the
// phases and the targets.
type coachState struct {
	state   limits.State
	levels  limits.Levels
	phases  []plan.Phase
	targets []plan.Target
}

func (s *sessions) coachState(ctx context.Context, p auth.Principal, now time.Time) (coachState, error) {
	cal, today, err := userDay(p, now)
	if err != nil {
		return coachState{}, err
	}
	// The summary's days, and today: the TDEE is of the days up to
	// yesterday, as today is open, but a plan made on the day of a first
	// weigh-in has a weight to go by.
	yesterday := today.AddDays(-1)
	data, err := s.readDays(ctx, p.User.ID, cal, yesterday.AddDays(1-max(calibration.FullDays(calibration.DefaultParams()), 7*summaryWeeks)), today, today, now)
	if err != nil {
		return coachState{}, err
	}
	pr, err := s.db.Profile(ctx, p.User.ID)
	if err != nil {
		return coachState{}, err
	}
	user, err := s.db.UserLimits(ctx, p.User.ID)
	if err != nil {
		return coachState{}, err
	}
	e := calibration.Calibrate(data.calDays, data.phases, yesterday, data.params)
	var ph *plan.Phase
	if got, ok := plan.PhaseOn(data.phases, today); ok {
		ph = &got
	}
	var t *plan.Target
	if got, ok := plan.TargetOn(data.targets, today); ok {
		t = &got
	}
	st := calibration.StateOf(e, today, pr, ph, t, data.training)
	if kg, _ := calibration.TrendAt(calibration.Trend(data.calDays, data.params.TrendAlpha), today); kg > 0 {
		st.TrendKg = kg
	}
	return coachState{
		state:  st,
		levels: limits.Levels{Instance: s.instance, User: user}, phases: data.phases, targets: data.targets,
	}, nil
}

// check returns a check of a proposal's changes against the limits of p's
// user at now, which refuses a violation. It reads the state in the
// store's transaction (store.ProposalCheck), so a change of the limits,
// the profile or the data meanwhile cannot slip past it, and keeps it in
// *cs, and what it found in *found.
func (s *sessions) check(ctx context.Context, p auth.Principal, now time.Time, cs *coachState, found *[]proposal.Checked) store.ProposalCheck {
	return func(c proposal.Changes) error {
		var err error
		if *cs, err = s.coachState(ctx, p, now); err != nil {
			return err
		}
		*found = c.Check(cs.state, cs.levels, cs.phases, cs.targets)
		for _, ck := range *found {
			if !ck.OK() {
				return &refusedError{checks: *found}
			}
		}
		return nil
	}
}

func (s *sessions) proposeChanges(ctx context.Context, in *proposeInput) (*proposedOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	b := in.Body
	a, err := coachActor(p, b.Model)
	if err != nil {
		return nil, err
	}
	d, err := b.draft()
	if err != nil {
		return nil, err
	}
	now := s.now()
	cal, today, err := userDay(p, now)
	if err != nil {
		return nil, internalError("making the proposal failed", err)
	}
	var cs coachState
	var checks []proposal.Checked
	made, err := s.db.CreateProposal(ctx, p.User.ID, a, d, today, now, s.check(ctx, p, now, &cs, &checks))
	if err != nil {
		return nil, proposalProblem("making the proposal failed", err)
	}
	return &proposedOutput{Body: proposedBody{Proposal: newProposalBody(made, cal.Location), Preview: cs.preview(checks)}}, nil
}

// changePlan is the user's own change of the plan in the app: a proposal
// of theirs, with the code ChangedInTheApp for its rationale, which the
// store applies as it makes it, held to the limits as any proposal.
func (s *sessions) changePlan(ctx context.Context, in *changePlanInput) (*proposedOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	d, err := proposeInputBody{Rationale: proposal.ChangedInTheApp, Phases: in.Body.Phases, Targets: in.Body.Targets}.draft()
	if err != nil {
		return nil, err
	}
	now := s.now()
	cal, today, err := userDay(p, now)
	if err != nil {
		return nil, internalError("changing the plan failed", err)
	}
	var cs coachState
	var checks []proposal.Checked
	changed, err := s.db.ChangePlan(ctx, p.User.ID, actorOf(p), d, today, now, s.check(ctx, p, now, &cs, &checks))
	if err != nil {
		return nil, proposalProblem("changing the plan failed", err)
	}
	s.logger.InfoContext(ctx, "changed the plan", "user_id", p.User.ID.String(), "proposal_id", changed.ID.String())
	return &proposedOutput{Body: proposedBody{Proposal: newProposalBody(changed, cal.Location), Preview: cs.preview(checks)}}, nil
}

func (s *sessions) applyProposal(ctx context.Context, in *applyInput) (*proposalOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	cal, today, err := userDay(p, now)
	if err != nil {
		return nil, internalError("applying the proposal failed", err)
	}
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, errNoProposal
	}
	var cs coachState
	var checks []proposal.Checked
	confirm := in.Body != nil && in.Body.Confirm
	check := s.check(ctx, p, now, &cs, &checks)
	applied, err := s.db.ApplyProposal(ctx, p.User.ID, actorOf(p), id, today, now, func(c proposal.Changes) error {
		if len(c.Limits) > 0 && p.Via != auth.ViaSession {
			return errLimitsInTheApp
		}
		if loosened := c.Loosened(); loosened != nil && !confirm {
			return huma.Error409Conflict("loosening " + strings.Join(loosened, ", ") + " needs confirm: true")
		}
		return check(c)
	})
	if err != nil {
		return nil, proposalProblem("applying the proposal failed", err)
	}
	out := &proposalOutput{}
	out.Body.Proposal = newProposalBody(applied, cal.Location)
	return out, nil
}

func (s *sessions) rejectProposal(ctx context.Context, in *proposalIDInput) (*proposalOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	cal, _, err := userDay(p, now)
	if err != nil {
		return nil, internalError("rejecting the proposal failed", err)
	}
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, errNoProposal
	}
	rejected, err := s.db.RejectProposal(ctx, p.User.ID, actorOf(p), id, now)
	if err != nil {
		return nil, proposalProblem("rejecting the proposal failed", err)
	}
	out := &proposalOutput{}
	out.Body.Proposal = newProposalBody(rejected, cal.Location)
	return out, nil
}

func (s *sessions) revertProposal(ctx context.Context, in *revertInput) (*proposedOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	a, err := coachActor(p, in.Body.Model)
	if err != nil {
		return nil, err
	}
	now := s.now()
	cal, today, err := userDay(p, now)
	if err != nil {
		return nil, internalError("making the revert failed", err)
	}
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, errNoProposal
	}
	var cs coachState
	var checks []proposal.Checked
	made, err := s.db.RevertProposal(ctx, p.User.ID, a, id, in.Body.Rationale, today, now, s.check(ctx, p, now, &cs, &checks))
	if err != nil {
		return nil, proposalProblem("making the revert failed", err)
	}
	return &proposedOutput{Body: proposedBody{Proposal: newProposalBody(made, cal.Location), Preview: cs.preview(checks)}}, nil
}

// coachActor is who proposes for the request of p, with the model that
// says it acts for the user.
func coachActor(p auth.Principal, model string) (event.Actor, error) {
	a := actorOf(p)
	a.Model = model
	if err := a.Validate(); err != nil {
		return event.Actor{}, huma.Error422UnprocessableEntity("model: " + err.Error())
	}
	return a, nil
}

// proposalProblem is the answer to err of a proposal's store method: a
// refusal of the limits, with each violation as data; a check's own
// answer; a change that breaks its own rules; a proposal that waits no
// more; none; or doing's failure.
func proposalProblem(doing string, err error) error {
	var refused *refusedError
	var closed *store.ProposalClosedError
	var answer huma.StatusError
	switch {
	case errors.As(err, &refused):
		return limitsRefusal(refused.checks)
	case errors.As(err, &answer):
		return answer
	case errors.Is(err, store.ErrInvalidProposal):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.As(err, &closed):
		return huma.Error409Conflict(closed.Error() + ": it waits no more")
	case errors.Is(err, store.ErrNotFound):
		return errNoProposal
	}
	return internalError(doing, err)
}

// limitsRefusal is the answer to changes past the limits: a detail for each
// violation, where it is (the change it is of) and the violation as its
// value, for a model to explain it and propose what the limits allow.
func limitsRefusal(checks []proposal.Checked) error {
	var details []error
	var rules []string
	for _, ck := range checks {
		at := string(ck.Of) + " " + ck.Day.String()
		if ck.Of == proposal.OfTrainingWeekdays || ck.Of == proposal.OfLimits {
			at = string(ck.Of)
		}
		for _, v := range ck.Violations {
			rules = append(rules, v.Rule+" ("+at+")")
			details = append(details, &huma.ErrorDetail{
				Location: at,
				Message:  violationMessage(v),
				Value:    newViolationValue(v),
			})
		}
	}
	return huma.Error422UnprocessableEntity("the proposal breaks the limits: "+strings.Join(rules, ", ")+
		". A user's limit the user may loosen in the app; an instance's, only the admin; one in the code, nothing", details...)
}

// violationMessage says v in words.
func violationMessage(v limits.Violation) string {
	on := ""
	if v.DayType != "" {
		on = " on " + string(v.DayType) + " days"
	}
	switch v.Rule {
	case limits.RuleProfileMissing:
		return "a plan needs the profile: sex, birth date and height, which the user sets in the app"
	case limits.RuleWeightMissing:
		return "a plan needs a trend weight: a weigh-in"
	case limits.RuleInvalidChange:
		return "the change breaks a rule of its shape"
	}
	return fmt.Sprintf("%s%s: %v proposed, the limit %v (%s)", v.Rule, on, round2(v.Proposed), round2(v.Limit), v.Level)
}

// preview is what the checks found, and the rate each target implies
// with the TDEE cs's calibration trusts.
func (cs coachState) preview(checks []proposal.Checked) previewBody {
	st := cs.state
	b := previewBody{Checks: []checkBody{}}
	if st.TrendKg > 0 {
		b.TrendKg = new(round2(st.TrendKg))
	}
	trusted := st.TDEETrusted && st.TDEE > 0 && st.TrendKg > 0
	if trusted {
		b.TDEE = new(math.Round(st.TDEE))
	}
	for _, ck := range checks {
		c := checkBody{Of: string(ck.Of), Day: ck.Day.String(), Warnings: []limitWarningBody{}, Violations: []violationValue{}}
		for _, v := range ck.Violations {
			c.Violations = append(c.Violations, newViolationValue(v))
		}
		if ck.Target != nil {
			weekly := ck.Target.WeeklyAverage(ck.TrainingDays.Count()).Kcal
			c.WeeklyKcal = new(math.Round(weekly))
			if trusted {
				rate := plan.ImpliedRate(weekly, st.TDEE, st.TrendKg)
				c.ExpectedPctWeek, c.ExpectedKgWeek = new(round2(rate)), new(round2(plan.KgPerWeek(rate, st.TrendKg)))
			}
		}
		for _, w := range ck.Warnings {
			c.Warnings = append(c.Warnings, limitWarningBody{Rule: w.Rule, DayType: string(w.DayType), Threshold: round2(w.Threshold), Value: round2(w.Value)})
		}
		b.Checks = append(b.Checks, c)
	}
	return b
}

// draft is the proposal b makes, as the domain reads it; a 422 for a
// field that does not read.
func (b proposeInputBody) draft() (proposal.Draft, error) {
	d := proposal.Draft{Rationale: b.Rationale}
	if w := b.Window; w != nil {
		from, err := day.ParseDate(w.From)
		if err != nil {
			return d, huma.Error422UnprocessableEntity("window.from: " + err.Error())
		}
		to, err := day.ParseDate(w.To)
		if err != nil {
			return d, huma.Error422UnprocessableEntity("window.to: " + err.Error())
		}
		d.Window = &proposal.Window{From: from, To: to}
	}
	for i, ph := range b.Phases {
		ch, err := ph.change()
		if err != nil {
			return d, huma.Error422UnprocessableEntity(fmt.Sprintf("phases[%d].%s", i, err))
		}
		d.Changes.Phases = append(d.Changes.Phases, ch)
	}
	for i, tg := range b.Targets {
		ch, err := tg.change()
		if err != nil {
			return d, huma.Error422UnprocessableEntity(fmt.Sprintf("targets[%d].%s", i, err))
		}
		d.Changes.Targets = append(d.Changes.Targets, ch)
	}
	if b.TrainingWeekdays != nil {
		w, err := weekdaysOf(b.TrainingWeekdays)
		if err != nil {
			return d, huma.Error422UnprocessableEntity("training_weekdays: " + err.Error())
		}
		d.Changes.TrainingWeekdays = &proposal.WeekdaysChange{After: w}
	}
	for _, name := range slices.Sorted(maps.Keys(b.Limits)) {
		d.Changes.Limits = append(d.Changes.Limits, proposal.LimitChange{Name: name, After: b.Limits[name]})
	}
	return d, nil
}

func (in phaseChangeIn) change() (proposal.PhaseChange, error) {
	start, err := day.ParseDate(in.Start)
	if err != nil {
		return proposal.PhaseChange{}, fmt.Errorf("start: %w", err)
	}
	ch := proposal.PhaseChange{Start: start}
	if in.Delete {
		return ch, nil
	}
	ph := plan.Phase{
		Type: plan.PhaseType(in.Type), Start: start, RatePctWeek: in.RatePctWeek,
		GoalWeight: in.GoalWeight, MilestoneWeight: in.MilestoneWeight,
	}
	if in.End != "" {
		if ph.End, err = day.ParseDate(in.End); err != nil {
			return proposal.PhaseChange{}, fmt.Errorf("end: %w", err)
		}
	}
	ch.After = &ph
	return ch, nil
}

func (in targetChangeIn) change() (proposal.TargetChange, error) {
	from, err := day.ParseDate(in.From)
	if err != nil {
		return proposal.TargetChange{}, fmt.Errorf("from: %w", err)
	}
	ch := proposal.TargetChange{From: from}
	if in.Delete {
		return ch, nil
	}
	if in.Training == nil {
		return proposal.TargetChange{}, errors.New("training is required, unless delete")
	}
	t := plan.Target{EffectiveFrom: from, Training: in.Training.target(), Rest: in.Training.target()}
	if in.Rest != nil {
		t.Rest = in.Rest.target()
	}
	ch.After = &t
	return ch, nil
}

func (in dayTargetIn) target() plan.DayTarget {
	return plan.DayTarget{Kcal: in.Kcal, Protein: in.Protein, Fat: in.Fat}
}

// weekdaysOf returns the set of the days of the week names names, in
// lower case.
func weekdaysOf(names []string) (plan.Weekdays, error) {
	var days []time.Weekday
	for _, n := range names {
		found := false
		for d := time.Sunday; d <= time.Saturday; d++ {
			if strings.ToLower(d.String()) == n {
				days, found = append(days, d), true
			}
		}
		if !found {
			return 0, fmt.Errorf("%q is no day of the week", n)
		}
	}
	return plan.WeekdaysOf(days...)
}
