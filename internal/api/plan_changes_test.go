package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
)

// changeBody changes the target from day d to kcal on both types of day.
func changeBody(d day.Date, kcal int) string {
	return fmt.Sprintf(`{"targets": [{"from": %q, "training": {"kcal": %d, "protein": 160, "fat": 60}}]}`, d, kcal)
}

// changePlan sends the user's own change body, and returns the answer.
func changePlan(t *testing.T, in *instance, session *http.Cookie, body string) proposeAnswer {
	t.Helper()
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/plan/changes", body, &a); code != http.StatusOK {
		t.Fatalf("change: %d %+v", code, a)
	}
	return a
}

// The user's own change applies at once: a revision of the user's in the
// app, whose rationale is a code, with what the limits checked.
func TestChangePlan(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	a := changePlan(t, in, session, changeBody(today, 1950))
	p := a.Proposal
	if p.Status != "applied" || p.By.Client != "web" || p.By.Model != "" || p.Rationale != proposal.ChangedInTheApp ||
		p.DecidedBy == nil || p.DecidedBy.Client != "web" || p.Window != nil {
		t.Errorf("the revision = %+v", p)
	}
	// The target in force before it was from another day: it is a new one.
	if len(p.Changes.Targets) != 1 || p.Changes.Targets[0].Before != nil || p.Changes.Targets[0].After.Training.Kcal != 1950 {
		t.Errorf("its target = %+v", p.Changes.Targets)
	}
	if len(a.Preview.Checks) != 1 || a.Preview.Checks[0].Of != "target" || !nearPtr(a.Preview.Checks[0].WeeklyKcal, 1950, 0) {
		t.Errorf("the checks = %+v", a.Preview.Checks)
	}
}

// The change is the plan's latest revision, and the proposal that waited
// is superseded: the plan changed under it.
func TestChangePlanIsARevision(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	waited := getPlan(t, in, []*http.Cookie{session}, nil).Pending
	p := changePlan(t, in, session, changeBody(today, 1950)).Proposal
	b := getPlan(t, in, []*http.Cookie{session}, nil)
	if b.Pending != nil || len(b.Revisions) == 0 || b.Revisions[0].ID != p.ID {
		t.Errorf("pending = %+v, revisions = %+v; want none waiting, and the change the latest revision", b.Pending, b.Revisions)
	}
	if !slices.ContainsFunc(b.Targets, func(tg planTarget) bool { return tg.EffectiveFrom == today.String() && tg.Training.Kcal == 1950 }) {
		t.Errorf("the targets = %+v, want one of 1950 from today", b.Targets)
	}
	if code := post(t, in, session, "/api/v1/proposals/"+waited.ID+"/apply", "", nil); code != http.StatusConflict {
		t.Errorf("the proposal that waited, applied: %d, want 409", code)
	}
}

// Changed again the same day, the target from today is edited; the app
// takes the change back with a code of its own.
func TestChangePlanAgainAndBack(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	changePlan(t, in, session, changeBody(today, 1950))
	again := changePlan(t, in, session, changeBody(today, 2000)).Proposal
	if before := again.Changes.Targets[0].Before; before == nil || before.Training.Kcal != 1950 {
		t.Errorf("changed again: %+v, want the target of 1950 before it", again.Changes)
	}
	var back proposeAnswer
	if code := post(t, in, session, "/api/v1/proposals/"+again.ID+"/revert", `{"rationale": "taken back in the app"}`, &back); code != http.StatusCreated ||
		back.Proposal.Rationale != "taken back in the app" {
		t.Fatalf("revert: %d %+v", code, back)
	}
	if code := post(t, in, session, "/api/v1/proposals/"+back.Proposal.ID+"/apply", "", nil); code != http.StatusOK {
		t.Errorf("apply the revert: %d", code)
	}
	b := getPlan(t, in, []*http.Cookie{session}, nil)
	if !slices.ContainsFunc(b.Targets, func(tg planTarget) bool { return tg.EffectiveFrom == today.String() && tg.Training.Kcal == 1950 }) {
		t.Errorf("the targets = %+v, want the one of 1950 back", b.Targets)
	}
}

// A phase is changed from the app too: the one now, and the next.
func TestChangePlanPhases(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	cut := getPlan(t, in, []*http.Cookie{session}, nil).Phases[0]
	body := fmt.Sprintf(`{"phases": [
		{"start": %q, "type": "cut", "rate_pct_week": -0.6, "end": %q, "goal_weight": 78},
		{"start": %q, "type": "maintain"}]}`, cut.Start, today.AddDays(13), today.AddDays(14))
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/plan/changes", body, &a); code != http.StatusOK || len(a.Proposal.Changes.Phases) != 2 {
		t.Fatalf("change: %d %+v", code, a)
	}
	b := getPlan(t, in, []*http.Cookie{session}, nil)
	if len(b.Phases) != 2 || b.Phases[0].RatePctWeek != -0.6 || b.Phases[1].Type != "maintain" || b.Phases[1].Start != today.AddDays(14).String() {
		t.Errorf("the phases = %+v", b.Phases)
	}
}

// What the limits refuse, or the change's own rules, changes nothing: the
// proposal that waited waits on. Only a web session changes the plan so.
func TestChangePlanRefuses(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	waited := getPlan(t, in, []*http.Cookie{session}, nil).Pending
	var pr problem
	if code := post(t, in, session, "/api/v1/plan/changes", changeBody(today, 1300), &pr); code != http.StatusUnprocessableEntity {
		t.Fatalf("change to 1300 kcal: %d %+v", code, pr)
	}
	found := false
	for _, e := range pr.Errors {
		var v struct {
			Rule, Level     string
			Limit, Proposed float64
		}
		if err := json.Unmarshal(e.Value, &v); err == nil && v.Rule == limits.RuleKcalFloor {
			found = v.Limit == 1500 && v.Proposed == 1300 && v.Level == string(limits.LevelUser) && e.Location == "target "+today.String()
		}
	}
	if !found {
		t.Errorf("the refusal = %+v, want kcal_floor, 1500, 1300, of the user's level", pr)
	}
	for name, tc := range map[string]struct{ body, want string }{
		"a target in the past": {changeBody(today.AddDays(-1), 1900), "before today"},
		"nothing":              {`{}`, "changes nothing"},
		"a phase type that has started": {
			fmt.Sprintf(`{"phases": [{"start": %q, "type": "bulk", "rate_pct_week": 0.2}]}`, today.AddDays(-60)),
			"has started",
		},
		"a target without its values": {fmt.Sprintf(`{"targets": [{"from": %q}]}`, today), "training is required"},
	} {
		var pr problem
		if code := post(t, in, session, "/api/v1/plan/changes", tc.body, &pr); code != http.StatusUnprocessableEntity || !strings.Contains(pr.Detail, tc.want) {
			t.Errorf("%s: %d %+v, want 422 %q", name, code, pr, tc.want)
		}
	}
	if after := getPlan(t, in, []*http.Cookie{session}, nil).Pending; after == nil || after.ID != waited.ID {
		t.Errorf("pending = %+v, want %+v", after, waited)
	}
	tools, _ := in.createToken(t, session, `{"name":"claude","scopes":["tools"]}`)
	if rec := in.do(t, http.MethodPost, "/api/v1/plan/changes", changeBody(today, 1900), nil, bearer(tools.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("with a tools token: %d %s, want 403", rec.Code, rec.Body.String())
	}
}
