package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// list-foods pages the user's foods, the archived ones apart, by name, by
// their last use or by the meals that had them, with how many there are,
// for a web session only.
// find_foods leaves the archived foods out, and finds them alone when
// asked for them.
func TestFindArchivedFoods(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	stew := e.in.user.Name + "'s canteen stew"
	for _, tc := range []struct{ path, want, not string }{
		{"/api/v1/foods?query=stew", `"unfound":["stew"]`, stew},
		{"/api/v1/foods?query=stew&archived=true", stew, `"unfound":["stew"]`},
		{"/api/v1/foods?query=rye&archived=true", `"unfound":["rye"]`, "rye bread"},
	} {
		rec := e.in.do(t, http.MethodGet, tc.path, "", e.session, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) || strings.Contains(rec.Body.String(), tc.not) {
			t.Errorf("%s: %d %s, want %q and not %q", tc.path, rec.Code, rec.Body.String(), tc.want, tc.not)
		}
	}
}

func TestListFoods(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := sessionOf(t, in)
	cookies := []*http.Cookie{session}
	now := in.clock.Now()
	bread := saveIsolationFoods(t, in.db, in.user, now) // and an archived stew
	apple := food.Food{
		ID: uuid.NewV7(), Name: "Apple", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 52, Protein: 0.3, Carbs: 14, Fat: 0.2},
	}
	if _, err := in.db.SaveFood(t.Context(), in.user.ID, isolationActor, apple, now); err != nil {
		t.Fatal(err)
	}
	// Bread eaten, the apple never.
	item, err := meal.FromFood(bread, meal.Quantity{Amount: 60, Unit: "g"}, "")
	if err != nil {
		t.Fatal(err)
	}
	m := meal.Meal{ID: uuid.NewV7(), EatenAt: now, Day: day.DateOf(now), Client: meal.ClientWeb, Items: []meal.Item{item}}
	if _, _, err := in.db.CreateMeal(t.Context(), in.user.ID, isolationActor, m, now); err != nil {
		t.Fatal(err)
	}

	rec := in.do(t, http.MethodGet, "/api/v1/foods/list", "", cookies, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{"total": 2, "foods": [
		{"id": %[1]q, "name": "Ana's rye bread", "basis": "100g", "source": "user", "archived": false,
		 "nutrients": {"kcal": 259, "protein": 8.5, "carbs": 48, "fat": 3.3, "fiber": 6},
		 "servings": [{"name": "slice", "amount": 40}], "aliases": ["Ana's loaf"],
		 "last_use": {"at": %[3]q, "day": "2026-10-21", "meal_id": %[4]q, "quantity": {"amount": 60, "unit": "g"}, "amount": 60, "unit": "g"}},
		{"id": %[2]q, "name": "Apple", "basis": "100g", "source": "label", "archived": false,
		 "nutrients": {"kcal": 52, "protein": 0.3, "carbs": 14, "fat": 0.2}}
	]}`, bread.ID, apple.ID, now.Format(time.RFC3339), m.ID))

	for query, want := range map[string]string{
		"?order=name&limit=1":          `"name":"Ana's rye bread"`,
		"?order=name&limit=1&offset=1": `"name":"Apple"`,
		"?archived=true":               `"name":"Ana's canteen stew"`,
		"?order=use&offset=5":          `{"foods":[],"total":2}`,
		"?order=frequent&limit=1":      `"name":"Ana's rye bread"`,
	} {
		rec := in.do(t, http.MethodGet, "/api/v1/foods/list"+query, "", cookies, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET /api/v1/foods/list%s: %d %s, want %s", query, rec.Code, rec.Body.String(), want)
		}
	}
	if got := in.do(t, http.MethodGet, "/api/v1/foods/list?order=name&limit=1", "", cookies, nil).Body.String(); strings.Count(got, `"id"`) != 1 {
		t.Errorf("limit 1: %s", got)
	}
	for _, q := range []string{"?limit=0", "?limit=201", "?offset=-1", "?order=kcal"} {
		if rec := in.do(t, http.MethodGet, "/api/v1/foods/list"+q, "", cookies, nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("GET /api/v1/foods/list%s: %d, want 422", q, rec.Code)
		}
	}
	// Not with an API token, nor without anyone.
	tok, created := in.createToken(t, session, `{"name":"phone","scopes":["read","tools"]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("a token: %d %s", created.Code, created.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/foods/list", "", nil, bearer(tok.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("with a token: %d, want 403", rec.Code)
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/foods/list", "", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("without anyone: %d, want 401", rec.Code)
	}
}

// create-food makes a new food only, and names the food in the way of
// its name and brand, archived too, which upsert-food would correct.
func TestCreateFood(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := sessionOf(t, in)
	cookies := []*http.Cookie{session}
	saveIsolationFoods(t, in.db, in.user, in.clock.Now()) // and an archived stew
	body := `{"name": "Kefir", "basis": "100ml", "source": "label", "nutrients": {"kcal": 41, "protein": 3.4, "carbs": 4, "fat": 1.5}}`
	rec := in.do(t, http.MethodPost, "/api/v1/foods/new", body, cookies, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"name":"Kefir"`) {
		t.Fatalf("a new food: %d %s", rec.Code, rec.Body.String())
	}
	for name, sent := range map[string]string{
		"the same again": body,
		"the archived stew's name": `{"name": "ana's CANTEEN stew", "basis": "portion", "source": "estimate",
			"nutrients": {"kcal": 700, "protein": 30, "carbs": 80, "fat": 25}}`,
	} {
		rec := in.do(t, http.MethodPost, "/api/v1/foods/new", sent, cookies, nil)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "the name and brand are taken by food") ||
			!strings.Contains(rec.Body.String(), `"location":"body.name"`) {
			t.Errorf("%s: %d %s, want 409 naming the food", name, rec.Code, rec.Body.String())
		}
	}
	if rec := in.do(t, http.MethodPost, "/api/v1/foods/new", `{"name": "Bad", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 900, "protein": 90, "carbs": 90, "fat": 90}}`, cookies, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a food no food can be: %d %s, want 422", rec.Code, rec.Body.String())
	}
	tok, created := in.createToken(t, session, `{"name":"phone","scopes":["read","tools"]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("a token: %d %s", created.Code, created.Body.String())
	}
	if rec := in.do(t, http.MethodPost, "/api/v1/foods/new", body, nil, bearer(tok.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("with a token: %d, want 403", rec.Code)
	}
}
