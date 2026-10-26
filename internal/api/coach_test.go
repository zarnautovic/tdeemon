package api_test

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
)

// planOutcome is what a target's days came to, as the tests read it.
type planOutcome struct {
	From            string   `json:"from"`
	To              string   `json:"to"`
	Days            int      `json:"days"`
	LoggedDays      int      `json:"logged_days"`
	WeighIns        int      `json:"weigh_ins"`
	IntakeKcal      *float64 `json:"intake_kcal"`
	TargetKcal      *float64 `json:"target_kcal"`
	IntakeProtein   *float64 `json:"intake_protein"`
	TargetProtein   *float64 `json:"target_protein"`
	Phase           string   `json:"phase"`
	PlannedPctWeek  *float64 `json:"planned_pct_week"`
	MeasuredFrom    string   `json:"measured_from"`
	Measured        bool     `json:"measured"`
	TDEE            *float64 `json:"tdee"`
	HalfWidth       *float64 `json:"half_width"`
	Trusted         bool     `json:"trusted"`
	KgPerWeek       *float64 `json:"kg_per_week"`
	MeasuredPctWeek *float64 `json:"measured_pct_week"`
	Doubts          []string `json:"doubts"`
}

// planAnswer is get_plan's answer, as the tests read it.
type planAnswer struct {
	Today  string `json:"today"`
	Phases []struct {
		Type        string  `json:"type"`
		Start       string  `json:"start"`
		RatePctWeek float64 `json:"rate_pct_week"`
	} `json:"phases"`
	TrainingWeekdays []string           `json:"training_weekdays"`
	Targets          []planTarget       `json:"targets"`
	Limits           map[string]float64 `json:"limits"`
	Pending          *planProposal      `json:"pending"`
	Revisions        []planProposal     `json:"revisions"`
}

// planTarget is a target of get_plan's, as the tests read it.
type planTarget struct {
	EffectiveFrom string `json:"effective_from"`
	To            string `json:"to"`
	Training      struct{ Kcal float64 }
	Outcome       *planOutcome `json:"outcome"`
}

// planProposal is a proposal of get_plan's, as the tests read it.
type planProposal struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Rationale string `json:"rationale"`
	By        struct {
		Client string `json:"client"`
		Model  string `json:"model"`
	} `json:"by"`
	Window *struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"window"`
	Changes struct {
		Phases  []json.RawMessage `json:"phases"`
		Targets []struct {
			From   string `json:"from"`
			Before *struct{ Training struct{ Kcal float64 } }
			After  *struct{ Training struct{ Kcal float64 } }
		} `json:"targets"`
		TrainingWeekdays *struct {
			Before []string `json:"before"`
			After  []string `json:"after"`
		} `json:"training_weekdays"`
	} `json:"changes"`
	DecidedBy *struct {
		Client string `json:"client"`
	} `json:"decided_by"`
	Reverts    *string `json:"reverts"`
	RevertedBy *string `json:"reverted_by"`
}

// savePlan gives the user of saveHistory a cut from the history's first
// day, a target of 2100 kcal from then and one of 2000 from 30 days
// before today, training weekdays, a revision (a target of 1900 from 3
// days on) and a pending proposal; and returns today.
func savePlan(t *testing.T, in *instance) day.Date {
	t.Helper()
	today := saveHistory(t, in)
	ctx, db, id, now := t.Context(), in.db, in.user.ID, in.clock.Now()
	if err := db.SetPhase(ctx, id, isolationActor, plan.Phase{Type: plan.Cut, Start: today.AddDays(-60), RatePctWeek: -0.5}, now); err != nil {
		t.Fatal(err)
	}
	for _, tg := range []struct {
		from int
		kcal float64
	}{{-60, 2100}, {-30, 2000}} {
		d := plan.DayTarget{Kcal: tg.kcal, Protein: 150, Fat: 60}
		if err := db.SetTarget(ctx, id, isolationActor, plan.Target{EffectiveFrom: today.AddDays(tg.from), Training: d, Rest: d}, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetTrainingWeekdays(ctx, id, isolationActor, plan.Weekdays(0b0101010), now); err != nil {
		t.Fatal(err)
	}
	coach := isolationActor
	coach.Client, coach.Model = "mcp", "claude-opus-5-5"
	d := plan.DayTarget{Kcal: 1900, Protein: 150, Fat: 60}
	step, err := db.CreateProposal(ctx, id, coach, proposal.Draft{
		Rationale: "A step down: the loss is slower than the cut's.",
		Window:    &proposal.Window{From: today.AddDays(-28), To: today.AddDays(-1)},
		Changes: proposal.Changes{Targets: []proposal.TargetChange{{
			From: today.AddDays(3), After: &plan.Target{EffectiveFrom: today.AddDays(3), Training: d, Rest: d},
		}}},
	}, today, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ApplyProposal(ctx, id, isolationActor, step.ID, today, now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateProposal(ctx, id, coach, proposal.Draft{
		Rationale: "Train on Sundays too.",
		Changes:   proposal.Changes{TrainingWeekdays: &proposal.WeekdaysChange{After: 0b0101011}},
	}, today, now, nil); err != nil {
		t.Fatal(err)
	}
	return today
}

func getPlan(t *testing.T, in *instance, cookies []*http.Cookie, header map[string]string) planAnswer {
	t.Helper()
	rec := in.do(t, http.MethodGet, "/api/v1/plan", "", cookies, header)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/plan: %d %s", rec.Code, rec.Body.String())
	}
	var b planAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func nearPtr(got *float64, want, tolerance float64) bool {
	return got != nil && math.Abs(*got-want) <= tolerance
}

func TestGetPlan(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := savePlan(t, in)
	b := getPlan(t, in, []*http.Cookie{session}, nil)

	if b.Today != today.String() || len(b.Phases) != 1 || b.Phases[0].Type != "cut" || b.Phases[0].Start != today.AddDays(-60).String() {
		t.Errorf("the plan = %+v", b)
	}
	if !slices.Equal(b.TrainingWeekdays, []string{"monday", "wednesday", "friday"}) {
		t.Errorf("training weekdays = %v", b.TrainingWeekdays)
	}
	if lim := limits.Effective(limits.UserDefaults(), limits.Instance()); b.Limits["cut_rate"] != lim.CutRate || len(b.Limits) != len(limits.Fields()) {
		t.Errorf("limits = %v", b.Limits)
	}
}

func TestGetPlanTargets(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := savePlan(t, in)
	ts := getPlan(t, in, []*http.Cookie{session}, nil).Targets
	if len(ts) != 3 {
		t.Fatalf("targets = %+v, want 3", ts)
	}
	// The first: its days up to the next, of them measured those after
	// the cut's first 14.
	first := ts[0]
	if first.EffectiveFrom != today.AddDays(-60).String() || first.To != today.AddDays(-31).String() || first.Outcome == nil {
		t.Fatalf("the first target = %+v", first)
	}
	checkOutcome(t, *first.Outcome, today.AddDays(-60), today.AddDays(-31), 2100)
	checkMeasured(t, *first.Outcome, today.AddDays(-46))
	// The one in force, to the day before the one to come: its days up to
	// yesterday, every one measured.
	now := ts[1]
	if now.EffectiveFrom != today.AddDays(-30).String() || now.To != today.AddDays(2).String() || now.Outcome == nil {
		t.Fatalf("the target in force = %+v", now)
	}
	checkOutcome(t, *now.Outcome, today.AddDays(-30), today.AddDays(-1), 2000)
	checkMeasured(t, *now.Outcome, today.AddDays(-30))
	// The last, to come, has no end and no outcome yet.
	if next := ts[2]; next.EffectiveFrom != today.AddDays(3).String() || next.To != "" || next.Training.Kcal != 1900 || next.Outcome != nil {
		t.Errorf("the target to come = %+v", next)
	}
}

// checkOutcome holds o to the days from to to of saveHistory, a day of
// 2000 kcal and 150 g of protein each, against a target of kcal, in the
// cut of savePlan.
func checkOutcome(t *testing.T, o planOutcome, from, to day.Date, kcal float64) {
	t.Helper()
	n := to.DaysSince(from) + 1
	if o.From != from.String() || o.To != to.String() || o.Days != n || o.LoggedDays != n || o.WeighIns != n {
		t.Errorf("the outcome's days = %+v", o)
	}
	if !nearPtr(o.IntakeKcal, 2000, 0) || !nearPtr(o.TargetKcal, kcal, 0) || !nearPtr(o.IntakeProtein, 150, 0) || !nearPtr(o.TargetProtein, 150, 0) {
		t.Errorf("the outcome's intake = %+v", o)
	}
	if o.Phase != "cut" || !nearPtr(o.PlannedPctWeek, -0.5, 0) {
		t.Errorf("the outcome's plan = %+v", o)
	}
}

// checkMeasured holds what o measures from measuredFrom to saveHistory's
// loss of 0.05 kg a day: 0.35 kg a week of a trend weight near 83 kg,
// from a TDEE of 2000 + 0.05 × 7700.
func checkMeasured(t *testing.T, o planOutcome, measuredFrom day.Date) {
	t.Helper()
	if o.MeasuredFrom != measuredFrom.String() || !o.Measured || !o.Trusted || len(o.Doubts) != 0 || o.HalfWidth == nil {
		t.Errorf("the outcome's measure = %+v", o)
	}
	if !nearPtr(o.TDEE, 2385, 1) || !nearPtr(o.KgPerWeek, -0.35, 0.005) || !nearPtr(o.MeasuredPctWeek, -0.42, 0.005) {
		t.Errorf("the outcome's measure = %+v", o)
	}
}

func TestGetPlanShowsThePending(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	savePlan(t, in)
	p := getPlan(t, in, []*http.Cookie{session}, nil).Pending
	if p == nil || p.Status != "pending" || p.Rationale != "Train on Sundays too." || p.By.Client != "mcp" || p.By.Model != "claude-opus-5-5" {
		t.Fatalf("pending = %+v", p)
	}
	if p.Window != nil || p.DecidedBy != nil || len(p.Changes.Phases) != 0 || len(p.Changes.Targets) != 0 {
		t.Errorf("pending = %+v", p)
	}
	if w := p.Changes.TrainingWeekdays; w == nil || !slices.Equal(w.Before, []string{"monday", "wednesday", "friday"}) ||
		!slices.Equal(w.After, []string{"sunday", "monday", "wednesday", "friday"}) {
		t.Errorf("pending training weekdays = %+v", w)
	}
}

func TestGetPlanShowsTheRevisions(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := savePlan(t, in)
	rs := getPlan(t, in, []*http.Cookie{session}, nil).Revisions
	if len(rs) != 1 {
		t.Fatalf("revisions = %+v, want 1", rs)
	}
	r := rs[0]
	if r.Status != "applied" || r.DecidedBy == nil || r.Window == nil || r.Window.From != today.AddDays(-28).String() ||
		len(r.Changes.Targets) != 1 || r.Changes.TrainingWeekdays != nil {
		t.Fatalf("the revision = %+v", r)
	}
	if c := r.Changes.Targets[0]; c.From != today.AddDays(3).String() || c.Before != nil || c.After == nil || c.After.Training.Kcal != 1900 {
		t.Errorf("the revision's target = %+v", c)
	}
}

func TestGetPlanOfNoPlan(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	b := getPlan(t, in, []*http.Cookie{session}, nil)
	if len(b.Phases) != 0 || len(b.Targets) != 0 || len(b.TrainingWeekdays) != 0 || b.Pending != nil || len(b.Revisions) != 0 || len(b.Limits) == 0 {
		t.Errorf("the plan of no plan = %+v", b)
	}
}

// The tool get_plan: a token that reads reads it, an ingest one not.
func TestGetPlanTakesAReadToken(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tok, created := in.createToken(t, session, `{"name":"claude","scopes":["read"]}`)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	getPlan(t, in, nil, bearer(tok.Token))
	phone, created := in.createToken(t, session, `{"name":"phone","scopes":["ingest"]}`)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/plan", "", nil, bearer(phone.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("with an ingest token: %d %s, want 403", rec.Code, rec.Body.String())
	}
}

// saveCoach is savePlan with the profile a plan needs, of a man of 180
// cm, so the limits can hold a change to it.
func saveCoach(t *testing.T, in *instance) day.Date {
	t.Helper()
	today := savePlan(t, in)
	pr := profile.Profile{Sex: profile.Male, BirthDate: day.Date{Year: 1990, Month: 6, Day: 15}, HeightCm: 180}
	if err := in.db.SetProfile(t.Context(), in.user.ID, isolationActor, pr, today, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	return today
}

// proposeAnswer is propose_changes' and revert_proposal's answer, as the
// tests read it.
type proposeAnswer struct {
	Proposal planProposal `json:"proposal"`
	Preview  struct {
		TrendKg *float64 `json:"trend_kg"`
		TDEE    *float64 `json:"tdee"`
		Checks  []struct {
			Of              string   `json:"of"`
			Day             string   `json:"day"`
			WeeklyKcal      *float64 `json:"weekly_kcal"`
			ExpectedPctWeek *float64 `json:"expected_pct_week"`
			ExpectedKgWeek  *float64 `json:"expected_kg_week"`
			Warnings        []struct {
				Rule string `json:"rule"`
			} `json:"warnings"`
		} `json:"checks"`
	} `json:"preview"`
}

// problem is an error's answer, as the tests read it.
type problem struct {
	Detail string `json:"detail"`
	Errors []struct {
		Location string          `json:"location"`
		Message  string          `json:"message"`
		Value    json.RawMessage `json:"value"`
	} `json:"errors"`
}

func post(t *testing.T, in *instance, session *http.Cookie, path, body string, out any) int {
	t.Helper()
	rec := in.do(t, http.MethodPost, path, body, []*http.Cookie{session}, nil)
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("POST %s: %d %s: %v", path, rec.Code, rec.Body.String(), err)
		}
	}
	return rec.Code
}

// stepBody proposes a target of kcal from day d, its rest days as its
// training days.
func stepBody(d day.Date, kcal int) string {
	return fmt.Sprintf(`{"rationale": "The loss is under the cut's rate.", "model": "claude-opus-5-5",
		"targets": [{"from": %q, "training": {"kcal": %d, "protein": 160, "fat": 60}}]}`, d, kcal)
}

func TestProposeChanges(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	tomorrow := today.AddDays(1)
	body := fmt.Sprintf(`{"rationale": "The loss is under the cut's rate.", "model": "claude-opus-5-5",
		"window": {"from": %q, "to": %q},
		"targets": [{"from": %q, "training": {"kcal": 1900, "protein": 160, "fat": 60}}]}`, today.AddDays(-28), today.AddDays(-1), tomorrow)
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/proposals", body, &a); code != http.StatusCreated {
		t.Fatalf("propose: %d %+v", code, a)
	}
	p := a.Proposal
	if p.Status != "pending" || p.By.Client != "web" || p.By.Model != "claude-opus-5-5" || p.Window == nil || len(p.Changes.Targets) != 1 {
		t.Fatalf("the proposal = %+v", p)
	}
	if c := p.Changes.Targets[0]; c.From != tomorrow.String() || c.Before != nil || c.After == nil || c.After.Training.Kcal != 1900 {
		t.Errorf("its target = %+v", c)
	}
	// It waits, in place of the one before.
	if b := getPlan(t, in, []*http.Cookie{session}, nil); b.Pending == nil || b.Pending.ID != p.ID {
		t.Errorf("pending = %+v, want %s", b.Pending, p.ID)
	}
}

// The preview of a target: the rate 1900 kcal a day implies with the TDEE
// of 2385, at a trend weight near 82.5 kg.
func TestProposeChangesPreview(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	tomorrow := today.AddDays(1)
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/proposals", stepBody(tomorrow, 1900), &a); code != http.StatusCreated {
		t.Fatalf("propose: %d %+v", code, a)
	}
	pv := a.Preview
	if !nearPtr(pv.TDEE, 2385, 1) || !nearPtr(pv.TrendKg, 82.5, 0.1) || len(pv.Checks) != 1 {
		t.Fatalf("the preview = %+v", pv)
	}
	if c := pv.Checks[0]; c.Of != "target" || c.Day != tomorrow.String() || !nearPtr(c.WeeklyKcal, 1900, 0) ||
		!nearPtr(c.ExpectedPctWeek, -0.53, 0.01) || !nearPtr(c.ExpectedKgWeek, -0.44, 0.01) {
		t.Errorf("the check = %+v", c)
	}
}

func TestProposeChangesRefusesPastTheLimits(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	before := getPlan(t, in, []*http.Cookie{session}, nil).Pending
	var pr problem
	if code := post(t, in, session, "/api/v1/proposals", stepBody(today.AddDays(1), 1300), &pr); code != http.StatusUnprocessableEntity {
		t.Fatalf("propose 1300 kcal: %d %+v", code, pr)
	}
	var floor *struct {
		Rule, Level     string
		Limit, Proposed float64
	}
	for _, e := range pr.Errors {
		var v struct {
			Rule, Level     string
			Limit, Proposed float64
		}
		if err := json.Unmarshal(e.Value, &v); err == nil && v.Rule == limits.RuleKcalFloor {
			floor = &v
			if e.Location != "target "+today.AddDays(1).String() || e.Message == "" {
				t.Errorf("the violation's detail = %+v", e)
			}
		}
	}
	if floor == nil || floor.Limit != 1500 || floor.Proposed != 1300 || floor.Level == "" {
		t.Errorf("the refusal = %+v, want kcal_floor, 1500, 1300", pr)
	}
	// Nothing was stored: the proposal before waits on.
	if after := getPlan(t, in, []*http.Cookie{session}, nil).Pending; after == nil || after.ID != before.ID {
		t.Errorf("pending = %+v, want %+v", after, before)
	}
}

func TestProposeChangesRefusesItsShape(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	for name, tc := range map[string]struct{ body, want string }{
		"a target in the past": {stepBody(today.AddDays(-1), 1900), "before today"},
		"a target without its values": {
			fmt.Sprintf(`{"rationale": "Why.", "targets": [{"from": %q}]}`, today.AddDays(1)),
			"training is required",
		},
		"nothing": {`{"rationale": "Why."}`, "changes nothing"},
		"a delete of no phase": {
			fmt.Sprintf(`{"rationale": "Why.", "phases": [{"start": %q, "delete": true}]}`, today.AddDays(9)),
			"no phase from",
		},
	} {
		var pr problem
		if code := post(t, in, session, "/api/v1/proposals", tc.body, &pr); code != http.StatusUnprocessableEntity || !strings.Contains(pr.Detail, tc.want) {
			t.Errorf("%s: %d %+v, want 422 %q", name, code, pr, tc.want)
		}
	}
}

// proposeStep proposes a target of kcal from tomorrow, and returns it.
func proposeStep(t *testing.T, in *instance, session *http.Cookie, today day.Date, kcal int) planProposal {
	t.Helper()
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/proposals", stepBody(today.AddDays(1), kcal), &a); code != http.StatusCreated {
		t.Fatalf("propose: %d %+v", code, a)
	}
	return a.Proposal
}

func TestApplyProposal(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	p := proposeStep(t, in, session, today, 1900)
	var a struct{ Proposal planProposal }
	if code := post(t, in, session, "/api/v1/proposals/"+p.ID+"/apply", "", &a); code != http.StatusOK ||
		a.Proposal.Status != "applied" || a.Proposal.DecidedBy == nil || a.Proposal.DecidedBy.Client != "web" {
		t.Fatalf("apply: %d %+v", code, a)
	}
	// The plan has the target, and the revision.
	b := getPlan(t, in, []*http.Cookie{session}, nil)
	if !slices.ContainsFunc(b.Targets, func(tg planTarget) bool {
		return tg.EffectiveFrom == today.AddDays(1).String() && tg.Training.Kcal == 1900
	}) ||
		len(b.Revisions) != 2 || b.Revisions[0].ID != p.ID {
		t.Errorf("the plan = %+v", b)
	}
	// Applied again, as a retry would, it is as it was.
	if code := post(t, in, session, "/api/v1/proposals/"+p.ID+"/apply", "", &a); code != http.StatusOK || a.Proposal.Status != "applied" {
		t.Errorf("apply again: %d %+v", code, a)
	}
}

func TestApplyProposalRefuses(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	// Within the limits when proposed, past them when applied: the user
	// raised their floor since.
	p := proposeStep(t, in, session, today, 1800)
	l := limits.UserDefaults()
	l.KcalFloorMale = 1850
	if err := in.db.SetUserLimits(t.Context(), in.user.ID, isolationActor, l, limits.Instance(), in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	var pr problem
	if code := post(t, in, session, "/api/v1/proposals/"+p.ID+"/apply", "", &pr); code != http.StatusUnprocessableEntity ||
		!strings.Contains(string(pr.Errors[0].Value), limits.RuleKcalFloor) {
		t.Errorf("apply past the limits: %d %+v", code, pr)
	}
	if b := getPlan(t, in, []*http.Cookie{session}, nil); b.Pending == nil || b.Pending.ID != p.ID {
		t.Errorf("pending = %+v, want it waiting", b.Pending)
	}
	// Rejected, it applies no more; one that does not exist is no one's.
	if code := post(t, in, session, "/api/v1/proposals/"+p.ID+"/reject", "", nil); code != http.StatusOK {
		t.Fatalf("reject: %d", code)
	}
	if code := post(t, in, session, "/api/v1/proposals/"+p.ID+"/apply", "", &pr); code != http.StatusConflict || !strings.Contains(pr.Detail, "rejected") {
		t.Errorf("apply a rejected one: %d %+v", code, pr)
	}
	if code := post(t, in, session, "/api/v1/proposals/"+uuid.NewV7().String()+"/apply", "", nil); code != http.StatusNotFound {
		t.Errorf("apply none: %d, want 404", code)
	}
}

func TestRejectProposal(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	p := proposeStep(t, in, session, today, 1900)
	var a struct{ Proposal planProposal }
	if code := post(t, in, session, "/api/v1/proposals/"+p.ID+"/reject", "", &a); code != http.StatusOK || a.Proposal.Status != "rejected" {
		t.Errorf("reject: %d %+v", code, a)
	}
	if b := getPlan(t, in, []*http.Cookie{session}, nil); b.Pending != nil {
		t.Errorf("pending = %+v, want none", b.Pending)
	}
	applied := proposeStep(t, in, session, today, 2000)
	post(t, in, session, "/api/v1/proposals/"+applied.ID+"/apply", "", nil)
	var pr problem
	if code := post(t, in, session, "/api/v1/proposals/"+applied.ID+"/reject", "", &pr); code != http.StatusConflict || !strings.Contains(pr.Detail, "applied") {
		t.Errorf("reject an applied one: %d %+v", code, pr)
	}
}

func TestRevertProposal(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	p := proposeStep(t, in, session, today, 1900)
	post(t, in, session, "/api/v1/proposals/"+p.ID+"/apply", "", nil)
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/proposals/"+p.ID+"/revert", `{"rationale": "Too hungry.", "model": "claude-opus-5-5"}`, &a); code != http.StatusCreated {
		t.Fatalf("revert: %d %+v", code, a)
	}
	r := a.Proposal
	if r.Status != "pending" || r.Reverts == nil || *r.Reverts != p.ID || len(r.Changes.Targets) != 1 || r.Changes.Targets[0].After != nil || len(a.Preview.Checks) != 1 {
		t.Fatalf("the revert = %+v", a)
	}
	post(t, in, session, "/api/v1/proposals/"+r.ID+"/apply", "", nil)
	b := getPlan(t, in, []*http.Cookie{session}, nil)
	if len(b.Revisions) < 2 || b.Revisions[0].ID != r.ID || b.Revisions[1].Status != "reverted" || b.Revisions[1].RevertedBy == nil {
		t.Errorf("the revisions = %+v", b.Revisions)
	}
	if code := post(t, in, session, "/api/v1/proposals/"+uuid.NewV7().String()+"/revert", `{"rationale": "Back."}`, nil); code != http.StatusNotFound {
		t.Errorf("revert none: %d, want 404", code)
	}
}

// The tools that write the plan take a tools token, not a read one.
func TestProposalsTakeAToolsToken(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	read, _ := in.createToken(t, session, `{"name":"reader","scopes":["read"]}`)
	if rec := in.do(t, http.MethodPost, "/api/v1/proposals", stepBody(today.AddDays(1), 1900), nil, bearer(read.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("with a read token: %d %s, want 403", rec.Code, rec.Body.String())
	}
	tools, _ := in.createToken(t, session, `{"name":"claude","scopes":["tools"]}`)
	rec := in.do(t, http.MethodPost, "/api/v1/proposals", stepBody(today.AddDays(1), 1900), nil, bearer(tools.Token))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"client":"api"`) {
		t.Errorf("with a tools token: %d %s", rec.Code, rec.Body.String())
	}
}

// Through MCP, a refusal of the limits holds each violation as data too.
func TestMCPRefusalHoldsTheViolations(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	cs := mcpSession(t, in, mcpToken(t, in, session, `"tools"`))
	text, isErr := callTool(t, cs, "propose_changes", map[string]any{
		"rationale": "Faster.",
		"targets":   []any{map[string]any{"from": today.AddDays(1).String(), "training": map[string]any{"kcal": 1300, "protein": 160, "fat": 60}}},
	})
	if !isErr || !strings.Contains(text, `"rule":"kcal_floor"`) || !strings.Contains(text, `"limit":1500`) || !strings.Contains(text, `"proposed":1300`) {
		t.Errorf("the refusal through MCP = %s", text)
	}
}

// On the first day, a weigh-in of today's gives the trend weight a plan
// needs.
func TestProposeOnTheFirstDay(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	now := in.clock.Now()
	today := day.DateOf(now)
	pr := profile.Profile{Sex: profile.Male, BirthDate: day.Date{Year: 1990, Month: 6, Day: 15}, HeightCm: 180}
	if err := in.db.SetProfile(t.Context(), in.user.ID, isolationActor, pr, today, now); err != nil {
		t.Fatal(err)
	}
	if _, err := in.db.SetMetric(t.Context(), in.user.ID, isolationActor, today, metric.Value{Kind: metric.Weight, Value: 90, Source: metric.Manual}, now); err != nil {
		t.Fatal(err)
	}
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/proposals", stepBody(today, 2200), &a); code != http.StatusCreated || !nearPtr(a.Preview.TrendKg, 90, 0) {
		t.Errorf("a plan on the first day: %d %+v", code, a)
	}
}
