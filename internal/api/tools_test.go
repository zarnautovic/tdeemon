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
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
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
	for name, op := range map[string]string{
		"get_profile": "get-me", "get_day": "get-day", "find_foods": "find-foods", "upsert_food": "upsert-food",
	} {
		if got[name] != op {
			t.Errorf("tool %s is operation %q, want %q", name, got[name], op)
		}
	}
}

// A tool that reads takes a token with the read scope or the tools
// scope; one that writes, only the tools scope. Neither takes one with
// only ingest.
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
	for _, op := range document().ops {
		name, ok := op.Metadata[api.ToolKey].(string)
		if !ok {
			continue
		}
		path := strings.ReplaceAll(op.Path, "{date}", "today")
		writes, body := op.Method != http.MethodGet, ""
		if writes {
			body = "{}" // refused for what it lacks, past the gate
		}
		for scope, token := range tokens {
			rec := in.do(t, op.Method, path, body, nil, bearer(token))
			if refused := rec.Code == http.StatusForbidden; refused != (scope == "ingest" || (writes && scope == "read")) {
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

	// Two slices of bread (80 g) and an estimate, rounded as they are
	// shown: kcal to a whole number, grams to a tenth.
	toast := `{"kcal": 207, "protein": 6.8, "carbs": 38.4, "fat": 2.6, "fiber": 4.8}`
	total := `{"kcal": 387, "protein": 12.8, "carbs": 58.4, "fat": 10.6, "fiber": 4.8}`
	todays := fmt.Sprintf(`{
		"date": %[1]q, "status": "open", "type": "training", "training_toggle": true,
		"meals": [{
			"id": "*", "eaten_at": %[2]q, "description": "Ana's lunch", "client": "mcp",
			"items": [
				{"id": "*", "food_id": %[3]q, "name": "Ana's rye bread", "quantity": {"servings": 2, "serving": "slice"},
				 "amount": 80, "unit": "g", "nutrients": %[4]s, "source": "user", "note": "Ana's toast"},
				{"id": "*", "name": "Ana's canteen soup", "quantity": {"portions": 1}, "amount": 1, "unit": "portion",
				 "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}, "source": "estimate"}
			],
			"total": %[5]s
		}],
		"total": %[5]s,
		"target": {"kcal": 2100, "protein": 150, "fat": 60, "carbs": 240},
		"remaining": {"kcal": 1713, "protein": 137.2, "fat": 49.4, "carbs": 181.6},
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

// Today is the user's day: at 03:30 in Zagreb, with the day starting at
// 04:00, it is still the day before by the calendar.
func TestToolsTodayIsTheUsers(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	in.clock.Set(time.Date(2026, 10, 21, 1, 30, 0, 0, time.UTC))
	if rec := in.do(t, http.MethodGet, "/api/v1/me", "", session, nil); !strings.Contains(rec.Body.String(), `"today":"2026-10-20"`) {
		t.Errorf("get_profile: %s, want today 2026-10-20", rec.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/days/today", "", session, nil); !strings.Contains(rec.Body.String(), `"date":"2026-10-20"`) {
		t.Errorf("get_day: %s, want 2026-10-20", rec.Body.String())
	}
}

// A day's target is its type's: a rest day's on a rest day, a training
// day's on a training day, and the check-in's toggle has the last word
// on the type.
func TestToolsTakeTheTargetOfTheDaysType(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	now := in.clock.Now() // a Wednesday
	today := day.DateOf(now)
	ctx := t.Context()
	tg := plan.Target{
		EffectiveFrom: today.AddDays(-7),
		Training:      plan.DayTarget{Kcal: 2300.4, Protein: 160.04, Fat: 70.06},
		Rest:          plan.DayTarget{Kcal: 1800.6, Protein: 150, Fat: 60},
	}
	if err := in.db.SetTarget(ctx, in.user.ID, isolationActor, tg, now); err != nil {
		t.Fatalf("SetTarget: %v", err)
	}
	// Monday, Wednesday and Friday train.
	if err := in.db.SetTrainingWeekdays(ctx, in.user.ID, isolationActor, plan.Weekdays(0b0101010), now); err != nil {
		t.Fatalf("SetTrainingWeekdays: %v", err)
	}
	// Today, a training weekday, the check-in makes a rest day; Thursday,
	// a rest weekday, a training day.
	if err := in.db.SetTrainingDay(ctx, in.user.ID, isolationActor, today, new(false), now); err != nil {
		t.Fatalf("SetTrainingDay: %v", err)
	}
	if err := in.db.SetTrainingDay(ctx, in.user.ID, isolationActor, today.AddDays(1), new(true), now); err != nil {
		t.Fatalf("SetTrainingDay: %v", err)
	}
	// Shown rounded: kcal to a whole number, grams to a tenth.
	training := `{"kcal": 2300, "protein": 160, "fat": 70.1, "carbs": 257.4}`
	rest := `{"kcal": 1801, "protein": 150, "fat": 60, "carbs": 165.1}`
	// Tuesday's fry-up goes past the rest day's target: what remains is
	// below 0 there.
	// What remains comes from the values as they are, rounded once:
	// 1800.6 - 1900.4 is -100, where the rounded 1801 - 1900 would be -99.
	fryUp, err := meal.Estimate("Fry-up", food.Nutrients{Kcal: 1900.4, Protein: 50, Carbs: 150, Fat: 120}, "")
	if err != nil {
		t.Fatal(err)
	}
	tuesday := today.AddDays(-1)
	m := meal.Meal{ID: uuid.NewV7(), EatenAt: now.Add(-24 * time.Hour), Day: tuesday, Description: "Breakfast", Client: meal.ClientWeb, Items: []meal.Item{fryUp}}
	if _, _, err := in.db.CreateMeal(ctx, in.user.ID, isolationActor, m, now); err != nil {
		t.Fatalf("CreateMeal: %v", err)
	}
	none := `"meals": [], "total": {"kcal": 0, "protein": 0, "carbs": 0, "fat": 0}`
	for _, tc := range []struct {
		date                   day.Date
		typ, target, remaining string
		toggle, meals          string
	}{
		{today.AddDays(-2), "training", training, training, "", none}, // Monday
		{tuesday, "rest", rest, `{"kcal": -100, "protein": 100, "fat": -60, "carbs": 15.1}`, "", `"meals": "*",
			"total": {"kcal": 1900, "protein": 50, "carbs": 150, "fat": 120}`},
		{today, "rest", rest, rest, `"training_toggle": false,`, none},
		{today.AddDays(1), "training", training, training, `"training_toggle": true,`, none},
	} {
		rec := in.do(t, http.MethodGet, "/api/v1/days/"+tc.date.String(), "", session, nil)
		checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{
			"date": %q, "status": "*", "type": %q, %s %s,
			"target": %s, "remaining": %s, "metrics": []
		}`, tc.date, tc.typ, tc.toggle, tc.meals, tc.target, tc.remaining))
	}
	rec := in.do(t, http.MethodGet, "/api/v1/me", "", session, nil)
	want := `"today_type":"rest"`
	if !strings.Contains(rec.Body.String(), want) ||
		!strings.Contains(rec.Body.String(), `"training":{"kcal":2300,"protein":160,"fat":70.1,"carbs":257.4},"rest":{"kcal":1801,`) {
		t.Errorf("get_profile: %s, want a rest day today and both targets", rec.Body.String())
	}
}

// An item keeps its quantity as it was given: grams of a food per portion
// come back as the grams, next to the portions they are.
func TestGetDayKeepsTheQuantityAsGiven(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	now := in.clock.Now()
	stew := food.Food{
		ID: uuid.NewV7(), Name: "Canteen stew", Basis: food.PerPortion, Source: food.Estimate,
		Nutrients: food.Nutrients{Kcal: 780, Protein: 38, Carbs: 85, Fat: 28}, PortionGrams: 450,
	}
	if _, err := in.db.SaveFood(t.Context(), in.user.ID, isolationActor, stew, now); err != nil {
		t.Fatalf("SaveFood: %v", err)
	}
	half, err := meal.FromFood(stew, meal.Quantity{Amount: 225, Unit: "g"}, "")
	if err != nil {
		t.Fatal(err)
	}
	m := meal.Meal{ID: uuid.NewV7(), EatenAt: now, Day: day.DateOf(now), Description: "Lunch", Client: meal.ClientWeb, Items: []meal.Item{half}}
	if _, _, err := in.db.CreateMeal(t.Context(), in.user.ID, isolationActor, m, now); err != nil {
		t.Fatalf("CreateMeal: %v", err)
	}
	rec := in.do(t, http.MethodGet, "/api/v1/days/today", "", []*http.Cookie{sessionOf(t, in)}, nil)
	if !strings.Contains(rec.Body.String(), `"quantity":{"amount":225,"unit":"g"},"amount":0.5,"unit":"portion"`) {
		t.Errorf("%s, want 225 g given, half a portion", rec.Body.String())
	}
}

// Every nutrient a label may give comes back, rounded as it is shown
// (salt to a hundredth), and so does a meal's note.
func TestGetDayShowsEveryNutrient(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	now := in.clock.Now()
	n := food.Nutrients{
		Kcal: 250.4, Protein: 10.04, Carbs: 30.06, Fat: 8.02,
		Fiber: new(3.33), SaturatedFat: new(1.27), Sugars: new(4.44), Salt: new(0.083), Polyols: new(2.22), Alcohol: new(1.16),
	}
	item, err := meal.Estimate("Cake", n, "")
	if err != nil {
		t.Fatal(err)
	}
	m := meal.Meal{ID: uuid.NewV7(), EatenAt: now, Day: day.DateOf(now), Description: "Tea", Client: meal.ClientChat, Note: "At the office", Items: []meal.Item{item}}
	if _, _, err := in.db.CreateMeal(t.Context(), in.user.ID, isolationActor, m, now); err != nil {
		t.Fatalf("CreateMeal: %v", err)
	}
	rec := in.do(t, http.MethodGet, "/api/v1/days/today", "", []*http.Cookie{sessionOf(t, in)}, nil)
	nutrients := `{"kcal": 250, "protein": 10, "carbs": 30.1, "fat": 8, "fiber": 3.3, "saturated_fat": 1.3, "sugars": 4.4, "salt": 0.08, "polyols": 2.2, "alcohol": 1.2}`
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{
		"date": %q, "status": "open", "type": "rest", "metrics": [],
		"meals": [{
			"id": "*", "eaten_at": "*", "description": "Tea", "client": "chat", "note": "At the office",
			"items": [{"id": "*", "name": "Cake", "quantity": {"portions": 1}, "amount": 1, "unit": "portion", "nutrients": %[2]s, "source": "estimate"}],
			"total": %[2]s
		}],
		"total": %[2]s
	}`, day.DateOf(now), nutrients))
}

// find_foods finds the user's foods by name, brand or alias, the name's
// matches first, with how much was eaten last; archived foods stay out.
func TestFindFoods(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	now := in.clock.Now()
	bread := saveIsolationFoods(t, in.db, in.user, now) // and an archived stew
	saveIsolationMeals(t, in.db, in.user, bread, now)
	roll := food.Food{
		ID: uuid.NewV7(), Name: "Rye roll", Brand: "Bakery", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 251.3, Protein: 8.04, Carbs: 46.07, Fat: 2.9, Salt: new(1.234)},
	}
	if _, err := in.db.SaveFood(t.Context(), in.user.ID, isolationActor, roll, now); err != nil {
		t.Fatal(err)
	}
	// A roll and a half, by weight.
	item, err := meal.FromFood(roll, meal.Quantity{Amount: 120, Unit: "g"}, "")
	if err != nil {
		t.Fatal(err)
	}
	m := meal.Meal{ID: uuid.NewV7(), EatenAt: now, Day: day.DateOf(now), Description: "Snack", Client: meal.ClientWeb, Items: []meal.Item{item}}
	if _, _, err := in.db.CreateMeal(t.Context(), in.user.ID, isolationActor, m, now); err != nil {
		t.Fatal(err)
	}
	rec := in.do(t, http.MethodGet, "/api/v1/foods?query=rye&limit=5", "", session, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	// The shorter name first, the values as they are stored, and the
	// bread's last use: two slices at lunch.
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{"foods": [
		{"id": %[1]q, "name": "Rye roll", "brand": "Bakery", "basis": "100g", "source": "label", "archived": false,
		 "nutrients": {"kcal": 251.3, "protein": 8.04, "carbs": 46.07, "fat": 2.9, "salt": 1.234},
		 "last_use": {"at": %[3]q, "meal_id": %[4]q, "quantity": {"amount": 120, "unit": "g"}, "amount": 120, "unit": "g"}},
		{"id": %[2]q, "name": "Ana's rye bread", "basis": "100g", "source": "user", "archived": false,
		 "nutrients": {"kcal": 259, "protein": 8.5, "carbs": 48, "fat": 3.3, "fiber": 6},
		 "servings": [{"name": "slice", "amount": 40}], "aliases": ["Ana's loaf"],
		 "last_use": {"at": %[3]q, "meal_id": "*", "quantity": {"servings": 2, "serving": "slice"}, "amount": 80, "unit": "g"}}
	]}`, roll.ID, bread.ID, now.Format(time.RFC3339), m.ID))
	// By an alias; an archived food not at all; at most limit.
	for query, want := range map[string]string{"loaf": `"name":"Ana's rye bread"`, "stew": `{"foods":[]}`} {
		if rec := in.do(t, http.MethodGet, "/api/v1/foods?query="+query, "", session, nil); !strings.Contains(rec.Body.String(), want) {
			t.Errorf("query %s: %s, want %s", query, rec.Body.String(), want)
		}
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/foods?query=rye&limit=1", "", session, nil); strings.Count(rec.Body.String(), `"id"`) != 1 {
		t.Errorf("limit 1: %s", rec.Body.String())
	}
	for _, q := range []string{"", "?query=rye&limit=0", "?query=rye&limit=51"} {
		if rec := in.do(t, http.MethodGet, "/api/v1/foods"+q, "", session, nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("GET /api/v1/foods%s: %d, want 422", q, rec.Code)
		}
	}
}

// upsert_food saves a food: a new one, or one of the same name and brand
// corrected in place, keeping its id; a food in the way of a name or an
// alias is named, archived or not.
func TestUpsertFood(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	const yoghurt = `{"name": "Greek yoghurt 2 %%", "brand": "Dukat", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 73, "protein": 9.5, "carbs": 3.6, "fat": 2, "saturated_fat": 1.3},
		"servings": [{"name": "cup", "amount": 150}], "aliases": ["%s"], "note": "from the lid"}`
	rec := in.do(t, http.MethodPost, "/api/v1/foods", fmt.Sprintf(yoghurt, "my yoghurt"), session, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("a new food: %d %s", rec.Code, rec.Body.String())
	}
	saved := decodeJSON[struct {
		ID string `json:"id"`
	}](t, rec.Body.Bytes())
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{
		"id": %q, "name": "Greek yoghurt 2 %%", "brand": "Dukat", "basis": "100g", "source": "label", "archived": false,
		"nutrients": {"kcal": 73, "protein": 9.5, "carbs": 3.6, "fat": 2, "saturated_fat": 1.3},
		"servings": [{"name": "cup", "amount": 150}], "aliases": ["my yoghurt"], "note": "from the lid"
	}`, saved.ID))
	// The same name and brand, under another id, is the same food.
	rec = in.do(t, http.MethodPost, "/api/v1/foods", `{"id": "`+uuid.NewV7().String()+`", `+fmt.Sprintf(yoghurt, "my yoghurt")[1:], session, nil)
	if !strings.Contains(rec.Body.String(), `"id":"`+saved.ID+`"`) {
		t.Errorf("the same food again: %d %s, want its id kept", rec.Code, rec.Body.String())
	}
	// An archived food keeps its alias; a new food that wants it is told
	// which food has it.
	id, err := uuid.Parse(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.db.SetFoodArchived(t.Context(), in.user.ID, isolationActor, id, true, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	rec = in.do(t, http.MethodPost, "/api/v1/foods", strings.Replace(fmt.Sprintf(yoghurt, "my yoghurt"), "Dukat", "Vindija", 1), session, nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), saved.ID) ||
		!strings.Contains(rec.Body.String(), `the alias \"my yoghurt\" is taken by food `) || !strings.Contains(rec.Body.String(), "archived") {
		t.Errorf("a taken alias: %d %s", rec.Code, rec.Body.String())
	}
	// The archived food, corrected without its alias, stays archived when
	// the save says so: out of a search.
	body := fmt.Sprintf(`{"id": %q, "archived": true, `, saved.ID) + strings.Replace(fmt.Sprintf(yoghurt, "x")[1:], `"aliases": ["x"], `, "", 1)
	if rec := in.do(t, http.MethodPost, "/api/v1/foods", body, session, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"archived":true`) ||
		strings.Contains(rec.Body.String(), "aliases") {
		t.Errorf("the archived food corrected: %d %s", rec.Code, rec.Body.String())
	}
	// A food that breaks the rules.
	rec = in.do(t, http.MethodPost, "/api/v1/foods", `{"name": "Air", "basis": "100g", "source": "label", "nutrients": {"kcal": 950, "protein": 0, "carbs": 0, "fat": 0}}`, session, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an invalid food: %d %s", rec.Code, rec.Body.String())
	}
}

// A new food takes the id the client chose, and its event says who wrote
// it: the web app with a session, the API with a token.
func TestUpsertFoodIDAndActor(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	chosen := uuid.NewV7()
	body := fmt.Sprintf(`{"id": %q, "name": "Skyr", "basis": "100g", "source": "label", "nutrients": {"kcal": 63, "protein": 11, "carbs": 4, "fat": 0.2}}`, chosen)
	if rec := in.do(t, http.MethodPost, "/api/v1/foods", body, session, nil); !strings.Contains(rec.Body.String(), `"id":"`+chosen.String()+`"`) {
		t.Errorf("a new food under its id: %d %s", rec.Code, rec.Body.String())
	}
	tok, _ := in.createToken(t, session[0], `{"name":"agent","scopes":["tools"]}`)
	in.do(t, http.MethodPost, "/api/v1/foods", `{"name": "Kefir", "basis": "100ml", "source": "label", "nutrients": {"kcal": 50, "protein": 3.4, "carbs": 4.7, "fat": 1.5}}`, nil, bearer(tok.Token))
	if es, err := in.db.Events(t.Context(), in.user.ID, 2); err != nil || len(es) != 2 || es[0].Actor.Client != "api" || es[1].Actor.Client != "web" {
		t.Errorf("the events = %+v, %v; want the API's, then the web app's", es, err)
	}
}

// A food's id, name and aliases are the user's: another user's food
// under the same id, or of the same name, brand and alias, is theirs, and
// no conflict; saving makes one of the user's own.
func TestUpsertFoodIDsArePerUser(t *testing.T) {
	t.Parallel()

	e := newIsolationEnv(t, false)
	b := e.owner.bread
	body := fmt.Sprintf(`{"id": %q, "name": "Crackers", "basis": "100g", "source": "user", "nutrients": {"kcal": 420, "protein": 10, "carbs": 70, "fat": 10}}`, b.ID)
	rec := e.in.do(t, http.MethodPost, "/api/v1/foods", body, nil, bearer(e.other.token.Token))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Crackers"`) {
		t.Fatalf("the other's food: %d %s", rec.Code, rec.Body.String())
	}
	body = fmt.Sprintf(`{"name": %q, "brand": %q, "aliases": [%q], "basis": "100g", "source": "user", "nutrients": {"kcal": 420, "protein": 10, "carbs": 70, "fat": 10}}`,
		b.Name, b.Brand, b.Aliases[0])
	rec = e.in.do(t, http.MethodPost, "/api/v1/foods", body, nil, bearer(e.other.token.Token))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), b.ID.String()) {
		t.Fatalf("the other's food of the owner's name and alias: %d %s", rec.Code, rec.Body.String())
	}
	f, err := e.in.db.FoodByID(t.Context(), e.owner.user.ID, e.owner.bread.ID)
	if err != nil || f.Name != e.owner.bread.Name {
		t.Errorf("the owner's food = %+v, %v; want it as it was", f, err)
	}
}
