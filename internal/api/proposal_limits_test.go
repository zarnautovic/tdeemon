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
)

// fasterCutBody proposes a faster cut than the user's limit allows, of
// savePlan's cut from 60 days ago, with the limit that allows it, and a
// higher protein floor.
func fasterCutBody(today day.Date) string {
	return fmt.Sprintf(`{"rationale": "A faster cut, with more protein.", "model": "claude-opus-5-5",
		"limits": {"protein_floor": 1.8, "cut_rate": -1.0},
		"phases": [{"start": %q, "type": "cut", "rate_pct_week": -0.9}]}`, today.AddDays(-60))
}

// getLimits returns the user's own limits, as the web app reads them.
func getLimits(t *testing.T, in *instance, session *http.Cookie) map[string]float64 {
	t.Helper()
	rec := in.do(t, http.MethodGet, "/api/v1/limits", "", []*http.Cookie{session}, nil)
	var b struct {
		Limits map[string]float64 `json:"limits"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &b) != nil {
		t.Fatalf("GET /api/v1/limits: %d %s", rec.Code, rec.Body.String())
	}
	return b.Limits
}

// A proposal changes the user's limits by name: each with the limit
// before it and whether it loosens it, and the rest of it held to the
// limits as it leaves them.
func TestProposeLimits(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveCoach(t, in)
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/proposals", fasterCutBody(today), &a); code != http.StatusCreated {
		t.Fatalf("propose: %d %+v", code, a)
	}
	want := []planLimitChange{
		{Name: "cut_rate", Before: -0.7, After: -1.0, Loosens: true},
		{Name: "protein_floor", Before: 1.6, After: 1.8},
	}
	if got := a.Proposal.Changes.Limits; !slices.Equal(got, want) {
		t.Errorf("the changes of the limits = %+v, want %+v", got, want)
	}
	var ofs []string
	for _, c := range a.Preview.Checks {
		ofs = append(ofs, c.Of+" "+c.Day)
	}
	if want := []string{"limits " + today.String(), "phase " + today.AddDays(-60).String()}; !slices.Equal(ofs, want) {
		t.Errorf("the checks = %v, want %v", ofs, want)
	}
	// The limits are as they were until it applies.
	if got := getLimits(t, in, session)["cut_rate"]; got != -0.7 {
		t.Errorf("cut_rate = %v, want it as it was", got)
	}
}

// A limit past the instance's is refused as a violation of the
// instance's level, which only the admin loosens.
func TestProposeLimitsPastTheInstances(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	saveCoach(t, in)
	var pr problem
	body := `{"rationale": "Faster.", "limits": {"cut_rate": -1.2}}`
	if code := post(t, in, session, "/api/v1/proposals", body, &pr); code != http.StatusUnprocessableEntity || len(pr.Errors) != 1 {
		t.Fatalf("propose: %d %+v", code, pr)
	}
	var v struct {
		Rule, Level     string
		Limit, Proposed float64
	}
	if e := pr.Errors[0]; e.Location != "limits" || json.Unmarshal(e.Value, &v) != nil ||
		v != (struct {
			Rule, Level     string
			Limit, Proposed float64
		}{"cut_rate", string(limits.LevelInstance), -1.0, -1.2}) {
		t.Errorf("the violation = %+v (%+v)", pr.Errors[0], v)
	}
}

func TestProposeLimitsRefusesItsShape(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	saveCoach(t, in)
	for name, tc := range map[string]struct{ limits, want string }{
		"a limit of no name": {`{"speed": 1}`, `no limit "speed"`},
		"a limit as it is":   {`{"cut_rate": -0.7}`, "the limit cut_rate is already so"},
		"a minimum past its maximum": {
			`{"recomp_min": 0.5}`, "recomp_min 0.5: want at most recomp_max 0.25",
		},
	} {
		var pr problem
		body := `{"rationale": "Why.", "limits": ` + tc.limits + `}`
		if code := post(t, in, session, "/api/v1/proposals", body, &pr); code != http.StatusUnprocessableEntity || !strings.Contains(pr.Detail, tc.want) {
			t.Errorf("%s: %d %+v, want 422 %q", name, code, pr, tc.want)
		}
	}
}

// proposeFasterCut proposes fasterCutBody with a tools token, and returns
// the token and the proposal's id.
func proposeFasterCut(t *testing.T, in *instance, session *http.Cookie, today day.Date) (string, string) {
	t.Helper()
	tools := mcpToken(t, in, session, `"tools"`)
	rec := in.do(t, http.MethodPost, "/api/v1/proposals", fasterCutBody(today), nil, bearer(tools))
	var a proposeAnswer
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &a) != nil {
		t.Fatalf("propose with a tools token: %d %s", rec.Code, rec.Body.String())
	}
	return tools, a.Proposal.ID
}

// A proposal of the limits applies only in a web session: not with a
// token, nor through MCP, however the user agreed, and MCP takes no
// confirmation, the web app's alone.
func TestApplyProposalOfLimitsNeedsTheApp(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tools, id := proposeFasterCut(t, in, session, saveCoach(t, in))
	if rec := in.do(t, http.MethodPost, "/api/v1/proposals/"+id+"/apply", `{"confirm": true}`, nil, bearer(tools)); rec.Code != http.StatusForbidden ||
		!strings.Contains(rec.Body.String(), "only in the app") {
		t.Errorf("apply with a token: %d %s, want 403", rec.Code, rec.Body.String())
	}
	cs := mcpSession(t, in, tools)
	if text, isErr := callTool(t, cs, "apply_proposal", map[string]any{"id": id}); !isErr || !strings.Contains(text, "only in the app") {
		t.Errorf("apply through MCP: %s", text)
	}
	if text, isErr := callTool(t, cs, "apply_proposal", map[string]any{"id": id, "confirm": true}); !isErr || !strings.Contains(text, `no argument "confirm"`) {
		t.Errorf("apply through MCP, confirmed: %s", text)
	}
	if got := getLimits(t, in, session)["cut_rate"]; got != -0.7 {
		t.Errorf("cut_rate = %v after the refusals, want it as it was", got)
	}
}

// In the app, one that loosens a limit applies with the user's
// confirmation alone.
func TestApplyProposalOfLimits(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	_, id := proposeFasterCut(t, in, session, saveCoach(t, in))
	apply := "/api/v1/proposals/" + id + "/apply"
	var pr problem
	if code := post(t, in, session, apply, "", &pr); code != http.StatusConflict || !strings.Contains(pr.Detail, "loosening cut_rate needs confirm: true") {
		t.Errorf("apply in the app without confirm: %d %+v", code, pr)
	}
	var applied struct{ Proposal planProposal }
	if code := post(t, in, session, apply, `{"confirm": true}`, &applied); code != http.StatusOK || applied.Proposal.Status != "applied" {
		t.Fatalf("apply in the app, confirmed: %d %+v", code, applied)
	}
	got := getLimits(t, in, session)
	if got["cut_rate"] != -1.0 || got["protein_floor"] != 1.8 {
		t.Errorf("the limits = %v, want cut_rate -1 and protein_floor 1.8", got)
	}
	if b := getPlan(t, in, []*http.Cookie{session}, nil); b.Phases[0].RatePctWeek != -0.9 || b.Limits["cut_rate"] != -1.0 {
		t.Errorf("the plan = %+v", b)
	}
}

// One that only tightens applies in the app without a confirmation.
func TestApplyProposalThatTightensTheLimits(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	saveCoach(t, in)
	var a proposeAnswer
	if code := post(t, in, session, "/api/v1/proposals", `{"rationale": "More protein.", "limits": {"protein_floor": 1.8}}`, &a); code != http.StatusCreated {
		t.Fatalf("propose: %d %+v", code, a)
	}
	if code := post(t, in, session, "/api/v1/proposals/"+a.Proposal.ID+"/apply", "", nil); code != http.StatusOK {
		t.Errorf("apply: %d", code)
	}
	if got := getLimits(t, in, session)["protein_floor"]; got != 1.8 {
		t.Errorf("protein_floor = %v, want 1.8", got)
	}
}

// MCP proposes a change of the limits, and has no confirmation to give:
// the web app's alone.
func TestMCPToolsOfTheLimits(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	res, err := mcpSession(t, in, mcpToken(t, in, session, `"tools"`)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	props := map[string]map[string]any{}
	for _, tl := range res.Tools {
		var s struct {
			Properties map[string]any `json:"properties"`
		}
		b, _ := json.Marshal(tl.InputSchema)
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatal(err)
		}
		props[tl.Name] = s.Properties
	}
	if _, ok := props["propose_changes"]["limits"]; !ok {
		t.Error("propose_changes takes no limits")
	}
	if _, ok := props["apply_proposal"]["confirm"]; ok {
		t.Error("apply_proposal takes confirm, the web app's")
	}
}
