package api_test

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

// toolName is the shape of a tool's name: snake_case, as DESIGN.md §5
// names them.
var toolName = regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)

// tools returns the operations of the document that are tools, by name.
func tools(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, op := range document().ops {
		name, ok := op.Metadata[api.ToolKey].(string)
		if !ok {
			continue
		}
		if prev, dup := out[name]; dup {
			t.Errorf("tool %s is both %s and %s", name, prev, op.OperationID)
		}
		out[name] = op.OperationID
	}
	return out
}

// The registry has the tools of DESIGN.md §5 that exist so far, each
// named as there.
func TestToolsAreNamed(t *testing.T) {
	t.Parallel()

	got := tools(t)
	for name := range got {
		if !toolName.MatchString(name) {
			t.Errorf("tool %q is not snake_case", name)
		}
	}
	for name, op := range map[string]string{"get_profile": "get-me", "get_day": "get-day"} {
		if got[name] != op {
			t.Errorf("tool %s is operation %q, want %q", name, got[name], op)
		}
	}
}

// A tool that reads takes a token with the read scope or the tools
// scope, and refuses one with only ingest.
func TestToolsTakeReadAndToolsTokens(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tokens := map[string]string{}
	for _, scope := range []string{"read", "tools", "ingest"} {
		created, rec := in.createToken(t, session, `{"name":"`+scope+`","scopes":["`+scope+`"]}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create a %s token: %d %s", scope, rec.Code, rec.Body.String())
		}
		tokens[scope] = created.Token
	}
	for name, opID := range tools(t) {
		var path string
		for p, op := range document().ops {
			if op.OperationID == opID {
				_, path, _ = strings.Cut(p, " ")
			}
		}
		path = strings.ReplaceAll(path, "{date}", "today")
		for scope, token := range tokens {
			rec := in.do(t, http.MethodGet, path, "", nil, bearer(token))
			if refused := rec.Code == http.StatusForbidden; refused != (scope == "ingest") {
				t.Errorf("%s with a %s token: %d %s", name, scope, rec.Code, rec.Body.String())
			}
		}
	}
}

// matchJSON returns where got, a decoded JSON answer, differs from want,
// decoded JSON too: numbers match within float arithmetic, and "*" in
// want matches any value (an ID the server gave).
func matchJSON(path string, want, got any) []string {
	switch w := want.(type) {
	case string:
		if w == "*" && got != nil {
			return nil
		}
	case float64:
		if g, ok := got.(float64); ok && near(w, g) {
			return nil
		}
		return []string{fmt.Sprintf("%s: %v, want %v", path, got, w)}
	case map[string]any:
		return matchObject(path, w, got)
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return []string{fmt.Sprintf("%s: %v, want %d values", path, got, len(w))}
		}
		var out []string
		for i := range w {
			out = append(out, matchJSON(fmt.Sprintf("%s[%d]", path, i), w[i], g[i])...)
		}
		return out
	}
	if !reflect.DeepEqual(want, got) {
		return []string{fmt.Sprintf("%s: %v, want %v", path, got, want)}
	}
	return nil
}

// matchObject is matchJSON of an object: got has the keys of want, and
// no others.
func matchObject(path string, want map[string]any, got any) []string {
	g, ok := got.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("%s: %v, want an object", path, got)}
	}
	var out []string
	for k := range g {
		if _, ok := want[k]; !ok {
			out = append(out, fmt.Sprintf("%s.%s: %v, want none", path, k, g[k]))
		}
	}
	for k, v := range want {
		out = append(out, matchJSON(path+"."+k, v, g[k])...)
	}
	return out
}

// checkJSON fails for every difference of body from want (matchJSON).
func checkJSON(t *testing.T, body []byte, want string) {
	t.Helper()
	var w, g any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want: %v\n%s", err, want)
	}
	if err := json.Unmarshal(body, &g); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	for _, d := range matchJSON("", w, g) {
		t.Error(d)
	}
}

// near reports whether a and b are equal but for float arithmetic.
func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// get_profile gives an agent who the user is and their plan today.
func TestGetProfile(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	now := in.clock.Now()
	saveIsolationPlan(t, in.db, in.user, now)
	today := day.DateOf(now)
	// The phase gets an end and a milestone.
	ph := plan.Phase{Type: plan.Cut, Start: today, End: today.AddDays(83), RatePctWeek: -0.5, GoalWeight: new(70.0), MilestoneWeight: new(75.0)}
	if err := in.db.SetPhase(t.Context(), in.user.ID, isolationActor, ph, now); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}
	rec := in.do(t, http.MethodGet, "/api/v1/me", "", []*http.Cookie{sessionOf(t, in)}, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	n := int(in.user.Name[0] % 20)
	birth := today.AddDays(-12000 - n)
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{
		"name": "Ana", "locale": "hr", "time_zone": "Europe/Zagreb", "day_start_hour": 4,
		"today": %[1]q, "today_type": "training", "training_weekdays": ["monday", "wednesday", "friday"],
		"profile": {"sex": "female", "birth_date": %[2]q, "age": %[3]d, "height_cm": %[4]d, "missing": []},
		"phase": {"type": "cut", "start": %[1]q, "end": %[5]q, "rate_pct_week": -0.5, "goal_weight": 70, "milestone_weight": 75},
		"target": {
			"effective_from": %[1]q,
			"training": {"kcal": 2100, "protein": 150, "fat": 60, "carbs": 240},
			"rest": {"kcal": 2100, "protein": 150, "fat": 60, "carbs": 240}
		},
		"notes": "Notes of Ana: no fish, trains at dawn.",
		"units": {"energy": "kcal", "nutrients": "g", "weight": "kg", "height": "cm"}
	}`, today, birth, profile.AgeOn(birth, today), 160+n, today.AddDays(83)))
}

// Before a plan, get_profile says what the profile lacks, and has no
// phase or target.
func TestGetProfileBeforeAPlan(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	rec := in.do(t, http.MethodGet, "/api/v1/me", "", []*http.Cookie{sessionOf(t, in)}, nil)
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{
		"name": "Ana", "locale": "hr", "time_zone": "Europe/Zagreb", "day_start_hour": 4,
		"today": %q, "today_type": "rest", "training_weekdays": [],
		"profile": {"missing": ["sex", "birth_date", "height_cm"]}, "notes": "",
		"units": {"energy": "kcal", "nutrients": "g", "weight": "kg", "height": "cm"}
	}`, day.DateOf(in.clock.Now())))
}

// get_profile needs someone signed in.
func TestGetProfileNeedsSomeone(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	if rec := in.do(t, http.MethodGet, "/api/v1/me", "", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("%d %s, want 401", rec.Code, rec.Body.String())
	}
}

// get_day gives a day's meals, totals against its target, what remains,
// its metrics and where it stands.
func TestGetDay(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	now := in.clock.Now()
	bread := saveIsolationFoods(t, in.db, in.user, now)
	saveIsolationMeals(t, in.db, in.user, bread, now)
	saveIsolationMetrics(t, in.db, in.user, now)
	saveIsolationPlan(t, in.db, in.user, now)
	today := day.DateOf(now)

	// Two slices of bread (80 g) and an estimate.
	toast := `{"kcal": 207.2, "protein": 6.8, "carbs": 38.4, "fat": 2.64, "fiber": 4.8}`
	total := `{"kcal": 387.2, "protein": 12.8, "carbs": 58.4, "fat": 10.64, "fiber": 4.8}`
	todays := fmt.Sprintf(`{
		"date": %[1]q, "status": "open", "type": "training", "training_toggle": true,
		"meals": [{
			"id": "*", "eaten_at": %[2]q, "description": "Ana's lunch", "client": "mcp",
			"items": [
				{"id": "*", "food_id": %[3]q, "name": "Ana's rye bread", "amount": 80, "unit": "g",
				 "servings": 2, "serving": "slice", "nutrients": %[4]s, "source": "user", "note": "Ana's toast"},
				{"id": "*", "name": "Ana's canteen soup", "amount": 1, "unit": "portion", "portions": 1,
				 "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}, "source": "estimate"}
			],
			"total": %[5]s
		}],
		"total": %[5]s,
		"target": {"kcal": 2100, "protein": 150, "fat": 60, "carbs": 240},
		"remaining": {"kcal": 1712.8, "protein": 137.2, "fat": 49.36, "carbs": 181.6},
		"metrics": [
			{"kind": "weight", "value": 81.5, "unit": "kg", "source": "manual"},
			{"kind": "steps", "value": 6400, "unit": "steps", "source": "import"}
		]
	}`, today, now.Format(time.RFC3339), bread.ID, toast, total)
	for _, path := range []string{"/api/v1/days/today", "/api/v1/days/" + today.String()} {
		rec := in.do(t, http.MethodGet, path, "", session, nil)
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
		}
		checkJSON(t, rec.Body.Bytes(), todays)
	}

	// The day before: excluded, a rest day by the plan, before the first
	// target, with nothing logged.
	rec := in.do(t, http.MethodGet, "/api/v1/days/"+today.AddDays(-1).String(), "", session, nil)
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{
		"date": %q, "status": "excluded", "excluded_reason": "Ana's birthday dinner", "type": "rest",
		"meals": [], "total": {"kcal": 0, "protein": 0, "carbs": 0, "fat": 0}, "metrics": []
	}`, today.AddDays(-1)))
	// Two days before: over, and not excluded.
	rec = in.do(t, http.MethodGet, "/api/v1/days/"+today.AddDays(-2).String(), "", session, nil)
	if !strings.Contains(rec.Body.String(), `"status":"complete"`) {
		t.Errorf("two days before: %d %s", rec.Code, rec.Body.String())
	}
}

// get_day takes a date or today, and refuses anything else.
func TestGetDayRefuses(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	for _, date := range []string{"yesterday", "2026-02-30", "21.10.2026"} {
		if rec := in.do(t, http.MethodGet, "/api/v1/days/"+date, "", session, nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s, want 422", date, rec.Code, rec.Body.String())
		}
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/days/today", "", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no one: %d %s, want 401", rec.Code, rec.Body.String())
	}
}
