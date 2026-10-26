package api_test

import (
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
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
	TrainingWeekdays []string `json:"training_weekdays"`
	Targets          []struct {
		EffectiveFrom string `json:"effective_from"`
		To            string `json:"to"`
		Training      struct{ Kcal float64 }
		Outcome       *planOutcome `json:"outcome"`
	} `json:"targets"`
	Limits    map[string]float64 `json:"limits"`
	Pending   *planProposal      `json:"pending"`
	Revisions []planProposal     `json:"revisions"`
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
	}, today, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ApplyProposal(ctx, id, isolationActor, step.ID, today, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateProposal(ctx, id, coach, proposal.Draft{
		Rationale: "Train on Sundays too.",
		Changes:   proposal.Changes{TrainingWeekdays: &proposal.WeekdaysChange{After: 0b0101011}},
	}, today, now); err != nil {
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
