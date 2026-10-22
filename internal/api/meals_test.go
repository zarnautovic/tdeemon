package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// mealsEnv is an instance with Ana's rye bread and plan, and her session.
type mealsEnv struct {
	in      *instance
	session []*http.Cookie
	now     time.Time
	bread   string
}

func newMealsEnv(t *testing.T) mealsEnv {
	t.Helper()
	in := newInstance(t, "http://localhost", "")
	now := in.clock.Now()
	bread := saveIsolationFoods(t, in.db, in.user, now)
	saveIsolationPlan(t, in.db, in.user, now)
	return mealsEnv{in: in, session: []*http.Cookie{sessionOf(t, in)}, now: now, bread: bread.ID.String()}
}

// lunch is a meal of three items of the three kinds: a food of the user's
// by its id, a new food, and an estimate.
func (e mealsEnv) lunch(id string) string {
	return fmt.Sprintf(`{"id": %q, "description": "Lunch", "note": "at home", "items": [
		{"food_id": %q, "servings": 2, "serving": "slice", "note": "toasted"},
		{"food": {"name": "Greek yoghurt", "brand": "Dukat", "basis": "100g", "source": "label",
		  "nutrients": {"kcal": 73, "protein": 9.5, "carbs": 3.6, "fat": 2}}, "amount": 200, "unit": "g"},
		{"name": "Restaurant pasta", "nutrients": {"kcal": 850, "protein": 32, "carbs": 105, "fat": 30}}
	]}`, id, e.bread)
}

// log_meal stores a meal of each kind of item, under the client's id, at
// the time of the id, and answers with the meal, where the day stands and
// any warnings; the same meal sent again is a retry.
func TestLogMeal(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	rec := e.in.do(t, http.MethodPost, "/api/v1/meals", e.lunch(id), e.session, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	foods, err := e.in.db.FindFoods(t.Context(), e.in.user.ID, "greek yoghurt", 5)
	if err != nil || len(foods) != 1 {
		t.Fatalf("the new food: %v, %v", foods, err)
	}
	total := `{"kcal": 1203, "protein": 57.8, "carbs": 150.6, "fat": 36.6, "fiber": 4.8}`
	want := fmt.Sprintf(`{
		"meal": {
			"id": %[1]q, "eaten_at": %[2]q, "description": "Lunch", "client": "web", "note": "at home",
			"items": [
				{"id": "*", "food_id": %[3]q, "name": "Ana's rye bread", "quantity": {"servings": 2, "serving": "slice"}, "amount": 80, "unit": "g",
				 "nutrients": {"kcal": 207, "protein": 6.8, "carbs": 38.4, "fat": 2.6, "fiber": 4.8}, "source": "user", "note": "toasted"},
				{"id": "*", "food_id": %[4]q, "name": "Dukat Greek yoghurt", "quantity": {"amount": 200, "unit": "g"}, "amount": 200, "unit": "g",
				 "nutrients": {"kcal": 146, "protein": 19, "carbs": 7.2, "fat": 4}, "source": "label"},
				{"id": "*", "name": "Restaurant pasta", "quantity": {"portions": 1}, "amount": 1, "unit": "portion",
				 "nutrients": {"kcal": 850, "protein": 32, "carbs": 105, "fat": 30}, "source": "estimate"}
			],
			"total": %[5]s
		},
		"day": {
			"date": %[6]q, "status": "open", "type": "training", "training_toggle": true, "total": %[5]s,
			"target": {"kcal": 2100, "protein": 150, "fat": 60, "carbs": 240},
			"remaining": {"kcal": 897, "protein": 92.2, "fat": 23.4, "carbs": 89.4}
		},
		"warnings": []
	}`, id, e.now.Format(time.RFC3339), e.bread, foods[0].ID, total, day.DateOf(e.now))
	checkJSON(t, rec.Body.Bytes(), want)

	// Sent again: the same meal, which is not logged twice.
	rec = e.in.do(t, http.MethodPost, "/api/v1/meals", e.lunch(id), e.session, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("a retry: %d %s, want 200", rec.Code, rec.Body.String())
	}
	checkJSON(t, rec.Body.Bytes(), want)
	// A late retry, days on: the id is stored, so its age does not matter.
	e.in.clock.Set(e.now.Add(72 * time.Hour))
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", e.lunch(id), e.session, nil); rec.Code != http.StatusOK {
		t.Errorf("a retry days later: %d %s, want 200", rec.Code, rec.Body.String())
	}
	e.in.clock.Set(e.now)
	// Other content under the id is another meal.
	rec = e.in.do(t, http.MethodPost, "/api/v1/meals", strings.Replace(e.lunch(id), "Lunch", "Dinner", 1), e.session, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("another meal under the id: %d %s, want 409", rec.Code, rec.Body.String())
	}
}

// A meal logged with a token is the API's; its time, when given, is the
// user's, and it belongs to their logical day.
func TestLogMealThroughTheAPI(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	tok, _ := e.in.createToken(t, e.session[0], `{"name":"agent","scopes":["tools"]}`)
	// 03:00 in Zagreb, before the day's start at 04:00: the day before,
	// where the date in UTC is the day after.
	eaten := time.Date(2026, 10, 21, 1, 0, 0, 0, time.UTC)
	body := fmt.Sprintf(`{"id": %q, "eaten_at": %q, "items": [{"name": "Snack", "nutrients": {"kcal": 200, "protein": 5, "carbs": 30, "fat": 6}}]}`,
		meal.NewIDAt(e.now), eaten.Format(time.RFC3339))
	rec := e.in.do(t, http.MethodPost, "/api/v1/meals", body, nil, bearer(tok.Token))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"client":"api"`) ||
		!strings.Contains(rec.Body.String(), `"date":"2026-10-20"`) || !strings.Contains(rec.Body.String(), `"eaten_at":"2026-10-21T01:00:00Z"`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

// log_meal warns about values that do not add up, and about a meal that
// looks like one logged minutes before; it stores the meal anyway.
func TestLogMealWarns(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	body := `{"id": %q, "items": [{"name": "Burger", "nutrients": {"kcal": 300, "protein": 30, "carbs": 50, "fat": 30}}]}`
	rec := e.in.do(t, http.MethodPost, "/api/v1/meals", fmt.Sprintf(body, meal.NewIDAt(e.now)), e.session, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"code":"atwater"`) {
		t.Errorf("values that do not add up: %d %s", rec.Code, rec.Body.String())
	}
	rec = e.in.do(t, http.MethodPost, "/api/v1/meals", fmt.Sprintf(body, meal.NewIDAt(e.now.Add(time.Minute))), e.session, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"code":"duplicate_meal"`) {
		t.Errorf("a meal twice: %d %s", rec.Code, rec.Body.String())
	}
	// A new food's values that do not add up.
	cake := fmt.Sprintf(`{"id": %q, "items": [{"food": {"name": "Cake", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 400, "protein": 5, "carbs": 50, "fat": 20, "saturated_fat": 25}}, "amount": 100, "unit": "g"}]}`, meal.NewIDAt(e.now.Add(-time.Hour)))
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", cake, e.session, nil); rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"code":"saturated_fat"`) {
		t.Errorf("a new food's values: %d %s", rec.Code, rec.Body.String())
	}
	// An item an edit brings, with values that do not add up; and the
	// same edit again, a retry, says so again.
	id := meal.NewIDAt(e.now.Add(-2 * time.Hour)).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id))
	add := fmt.Sprintf(`{"ops": [{"op": "add", "item": {"id": %q, "name": "Burger", "nutrients": {"kcal": 300, "protein": 30, "carbs": 50, "fat": 30}}}]}`, meal.NewIDAt(e.now))
	for _, name := range []string{"an add", "the add again"} {
		if rec := e.patch(t, id, add); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":"atwater"`) {
			t.Errorf("%s of values that do not add up: %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

// log_meal refuses a meal it cannot take, saying why, and stores nothing.
func TestLogMealRefuses(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	estimate := `{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`
	for _, tc := range []struct {
		name, id, items, want string
	}{
		{"a version 4 id", "f47ac10b-58cc-4372-a567-0e02b2c3d479", estimate, "not a version 7"},
		{"an id from days ago", meal.NewIDAt(e.now.Add(-49 * time.Hour)).String(), estimate, "more than a day before"},
		{"an item of two kinds", meal.NewIDAt(e.now).String(), fmt.Sprintf(`{"food_id": %q, "portions": 1, "name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`, e.bread), "one of"},
		{"an estimate without its values", meal.NewIDAt(e.now).String(), `{"name": "Soup"}`, "one of"},
		{"an estimate without its name", meal.NewIDAt(e.now).String(), `{"nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`, "one of"},
		{"no kind", meal.NewIDAt(e.now).String(), `{"note": "something"}`, "one of"},
		{"a quantity on an estimate", meal.NewIDAt(e.now).String(), `{"name": "Soup", "portions": 2, "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`, "an estimate is the whole plate"},
		{"a food no one has", meal.NewIDAt(e.now).String(), fmt.Sprintf(`{"food_id": %q, "amount": 50, "unit": "g"}`, meal.NewIDAt(e.now)), "no such food"},
		{"ml of a food by weight", meal.NewIDAt(e.now).String(), fmt.Sprintf(`{"food_id": %q, "amount": 250, "unit": "ml"}`, e.bread), "ml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := e.in.do(t, http.MethodPost, "/api/v1/meals", fmt.Sprintf(`{"id": %q, "items": [%s]}`, tc.id, tc.items), e.session, nil)
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("%d %s, want 422 with %q", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
	if meals, err := e.in.db.MealsOfDay(t.Context(), e.in.user.ID, day.DateOf(e.now)); err != nil || len(meals) != 0 {
		t.Errorf("meals = %d, %v; want none", len(meals), err)
	}
}

// logged logs body and fails the test unless it is stored.
func (e mealsEnv) logged(t *testing.T, body string) {
	t.Helper()
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", body, e.session, nil); rec.Code != http.StatusCreated {
		t.Fatalf("log the meal: %d %s", rec.Code, rec.Body.String())
	}
}

// patch sends ops to the meal id.
func (e mealsEnv) patch(t *testing.T, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	return e.in.do(t, http.MethodPatch, "/api/v1/meals/"+id, body, e.session, nil)
}

// update_meal adds, replaces and removes items, and changes the time and
// the words; an item it does not touch keeps what it brought when logged,
// though its food was corrected since.
func TestUpdateMeal(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	toast, pasta, egg := meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "description": "Lunch", "items": [
		{"id": %q, "food_id": %q, "servings": 2, "serving": "slice"},
		{"id": %q, "name": "Pasta", "nutrients": {"kcal": 600, "protein": 20, "carbs": 90, "fat": 15}}
	]}`, id, toast, e.bread, pasta))
	// The bread, corrected since.
	f, err := e.in.db.FindFoods(t.Context(), e.in.user.ID, "rye", 1)
	if err != nil || len(f) != 1 {
		t.Fatalf("the bread: %v, %v", f, err)
	}
	f[0].Nutrients.Kcal = 300
	if _, err := e.in.db.SaveFood(t.Context(), e.in.user.ID, isolationActor, f[0], e.now); err != nil {
		t.Fatal(err)
	}

	add := fmt.Sprintf(`{"op": "add", "item": {"id": %q, "food_id": %q, "servings": 1, "serving": "slice"}}`, egg, e.bread)
	rec := e.patch(t, id, fmt.Sprintf(`{"description": "Late lunch", "note": "with tea", "ops": [
		%s,
		{"op": "replace", "item_id": %q, "item": {"name": "Pasta", "nutrients": {"kcal": 700, "protein": 22, "carbs": 100, "fat": 20}}}
	]}`, add, pasta))
	if rec.Code != http.StatusOK {
		t.Fatalf("an edit: %d %s", rec.Code, rec.Body.String())
	}
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{
		"meal": {"id": %[1]q, "eaten_at": "*", "description": "Late lunch", "note": "with tea", "client": "web", "total": "*", "items": [
			{"id": %[2]q, "food_id": %[3]q, "name": "Ana's rye bread", "quantity": {"servings": 2, "serving": "slice"}, "amount": 80, "unit": "g",
			 "nutrients": {"kcal": 207, "protein": 6.8, "carbs": 38.4, "fat": 2.6, "fiber": 4.8}, "source": "user"},
			{"id": %[4]q, "name": "Pasta", "quantity": {"portions": 1}, "amount": 1, "unit": "portion",
			 "nutrients": {"kcal": 700, "protein": 22, "carbs": 100, "fat": 20}, "source": "estimate"},
			{"id": %[5]q, "food_id": %[3]q, "name": "Ana's rye bread", "quantity": {"servings": 1, "serving": "slice"}, "amount": 40, "unit": "g",
			 "nutrients": {"kcal": 120, "protein": 3.4, "carbs": 19.2, "fat": 1.3, "fiber": 2.4}, "source": "user"}
		]},
		"day": "*", "warnings": []
	}`, id, toast, e.bread, pasta, egg))

	// The add sent again is a retry; another item under its id a conflict.
	if rec := e.patch(t, id, `{"ops": [`+add+`]}`); rec.Code != http.StatusOK || strings.Count(rec.Body.String(), `"name":"Ana's rye bread"`) != 2 {
		t.Errorf("the add again: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.patch(t, id, `{"ops": [`+strings.Replace(add, `"servings": 1`, `"servings": 3`, 1)+`]}`); rec.Code != http.StatusConflict {
		t.Errorf("another item under the add's id: %d %s, want 409", rec.Code, rec.Body.String())
	}
	// A remove, sent again, changes nothing more.
	remove := fmt.Sprintf(`{"ops": [{"op": "remove", "item_id": %q}]}`, egg)
	for range 2 {
		if rec := e.patch(t, id, remove); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), egg) {
			t.Errorf("a remove: %d %s", rec.Code, rec.Body.String())
		}
	}
	// Eaten the day before: the meal moves to that day.
	rec = e.patch(t, id, fmt.Sprintf(`{"eaten_at": %q}`, e.now.Add(-24*time.Hour).Format(time.RFC3339)))
	if before := day.DateOf(e.now).AddDays(-1).String(); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"date":"`+before+`"`) {
		t.Errorf("another day: %d %s, want %s", rec.Code, rec.Body.String(), before)
	}
}

// update_meal refuses what it cannot do, saying why, and changes nothing.
func TestUpdateMealRefuses(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	soup := meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"id": %q, "name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id, soup))
	estimate := `{"name": "Bread", "nutrients": {"kcal": 100, "protein": 3, "carbs": 20, "fat": 1}}`
	for _, tc := range []struct {
		name, body, want string
	}{
		{"a replace of no item", fmt.Sprintf(`{"ops": [{"op": "replace", "item_id": %q, "item": %s}]}`, meal.NewIDAt(e.now), estimate), "no item"},
		{"an add without an item", `{"ops": [{"op": "add"}]}`, "an add takes an item"},
		{"an add of an item without an id", `{"ops": [{"op": "add", "item": ` + estimate + `}]}`, "an add takes an item with its id"},
		{"a remove with an item", fmt.Sprintf(`{"ops": [{"op": "remove", "item_id": %q, "item": %s}]}`, soup, estimate), "a remove takes an item_id"},
		{"the last item removed", fmt.Sprintf(`{"ops": [{"op": "remove", "item_id": %q}]}`, soup), "item"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if rec := e.patch(t, id, tc.body); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("%d %s, want 422 with %q", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
	if rec := e.patch(t, meal.NewIDAt(e.now).String(), `{"description": "x"}`); rec.Code != http.StatusNotFound {
		t.Errorf("a meal that does not exist: %d %s, want 404", rec.Code, rec.Body.String())
	}
}

// delete_meal deletes a meal and says where its day stands; its id is not
// the id of a new meal.
func TestDeleteMeal(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	body := fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id)
	e.logged(t, body)
	rec := e.in.do(t, http.MethodDelete, "/api/v1/meals/"+id, "", e.session, nil)
	checkJSON(t, rec.Body.Bytes(), fmt.Sprintf(`{"day": {"date": %q, "status": "open", "type": "training", "training_toggle": true,
		"total": {"kcal": 0, "protein": 0, "carbs": 0, "fat": 0}, "target": "*", "remaining": {"kcal": 2100, "protein": 150, "fat": 60, "carbs": 240}}}`, day.DateOf(e.now)))
	if rec := e.in.do(t, http.MethodDelete, "/api/v1/meals/"+id, "", e.session, nil); rec.Code != http.StatusNotFound {
		t.Errorf("deleted again: %d %s, want 404", rec.Code, rec.Body.String())
	}
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", body, e.session, nil); rec.Code != http.StatusConflict {
		t.Errorf("logged again under its id: %d %s, want 409", rec.Code, rec.Body.String())
	}
	// Days on, an offline queue's late retry: still the deleted meal's id,
	// not a new meal under an old id.
	e.in.clock.Set(e.now.Add(48 * time.Hour))
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", body, e.session, nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "deleted") {
		t.Errorf("a late retry of a deleted meal: %d %s, want 409", rec.Code, rec.Body.String())
	}
}

// A meal's id is the user's: another user's meal under the same id is
// theirs, and logging under it logs one of the user's own.
func TestLogMealIDsArePerUser(t *testing.T) {
	t.Parallel()

	e := newIsolationEnv(t, false)
	body := fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, e.owner.meal)
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", body, nil, bearer(e.other.token.Token)); rec.Code != http.StatusCreated {
		t.Fatalf("the other's meal under the owner's id: %d %s, want 201", rec.Code, rec.Body.String())
	}
	m, err := e.in.db.MealByID(t.Context(), e.owner.user.ID, e.owner.meal)
	if err != nil || m.Description != e.owner.user.Name+"'s lunch" || len(m.Items) != 2 {
		t.Errorf("the owner's meal = %+v, %v; want it as it was", m, err)
	}
}

// A meal's item cannot take the id of another meal's item, and a new food
// cannot take an alias another food has: both are conflicts.
func TestMealConflicts(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id, other, item := meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String()
	soup := `{"id": %q, "name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [%s]}`, id, fmt.Sprintf(soup, item)))
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Tea", "nutrients": {"kcal": 2, "protein": 0, "carbs": 0.3, "fat": 0}}]}`, other))
	rec := e.patch(t, other, fmt.Sprintf(`{"ops": [{"op": "add", "item": %s}]}`, fmt.Sprintf(soup, item)))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "another meal") {
		t.Errorf("an item of another meal: %d %s, want 409", rec.Code, rec.Body.String())
	}
	body := fmt.Sprintf(`{"id": %q, "items": [{"food": {"name": "Loaf", "basis": "100g", "source": "label", "aliases": ["Ana's loaf"],
		"nutrients": {"kcal": 250, "protein": 8, "carbs": 46, "fat": 3}}, "amount": 50, "unit": "g"}]}`, meal.NewIDAt(e.now))
	rec = e.in.do(t, http.MethodPost, "/api/v1/meals", body, e.session, nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `the alias \"Ana's loaf\" is taken by food `+e.bread) {
		t.Errorf("a new food of a taken alias: %d %s, want 409 naming the bread", rec.Code, rec.Body.String())
	}
}

// Changes of a meal sent at once, as an agent's calls in parallel, are
// all kept.
func TestUpdateMealAtOnce(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id))
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			body := fmt.Sprintf(`{"ops": [{"op": "add", "item": {"id": %q, "name": "Tea %d", "nutrients": {"kcal": 2, "protein": 0, "carbs": 0.3, "fat": 0}}}]}`, meal.NewIDAt(e.now), i)
			if rec := e.patch(t, id, body); rec.Code != http.StatusOK {
				t.Errorf("an add: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	wg.Wait()
	mid, err := uuid.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := e.in.db.MealByID(t.Context(), e.in.user.ID, mid); err != nil || len(m.Items) != 11 {
		t.Errorf("the meal has %d items, %v; want 11", len(m.Items), err)
	}
}

// A new meal's item under the id of another meal's item is a conflict
// that says so, not one of the meal's id.
func TestLogMealItemOfAnotherMeal(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	item := meal.NewIDAt(e.now).String()
	soup := fmt.Sprintf(`{"id": %q, "name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`, item)
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [%s]}`, meal.NewIDAt(e.now), soup))
	rec := e.in.do(t, http.MethodPost, "/api/v1/meals", fmt.Sprintf(`{"id": %q, "items": [%s]}`, meal.NewIDAt(e.now), soup), e.session, nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "another meal's item") {
		t.Errorf("%d %s, want 409 about the item", rec.Code, rec.Body.String())
	}
}

// A meal sent again saves none of its new foods again: a food corrected
// since stays corrected. A meal refused saves none of them at all.
func TestLogMealSavesFoodsOnce(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	e.logged(t, e.lunch(id))
	yoghurt, err := foodNamed(t, e.in.db, e.in.user.ID, "Greek yoghurt", "Dukat")
	if err != nil {
		t.Fatal(err)
	}
	yoghurt.Nutrients.Kcal = 80
	if _, err := e.in.db.SaveFood(t.Context(), e.in.user.ID, isolationActor, yoghurt, e.now); err != nil {
		t.Fatal(err)
	}
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", e.lunch(id), e.session, nil); rec.Code != http.StatusOK {
		t.Fatalf("the retry: %d %s", rec.Code, rec.Body.String())
	}
	if got, err := foodNamed(t, e.in.db, e.in.user.ID, "Greek yoghurt", "Dukat"); err != nil || got.Nutrients.Kcal != 80 {
		t.Errorf("the yoghurt after the retry = %v kcal, %v; want the correction kept", got.Nutrients.Kcal, err)
	}
	// Refused for its second item, after a new food in its first.
	body := fmt.Sprintf(`{"id": %q, "items": [
		{"food": {"name": "Kefir", "basis": "100ml", "source": "label", "nutrients": {"kcal": 50, "protein": 3.4, "carbs": 4.7, "fat": 1.5}}, "amount": 250, "unit": "ml"},
		{"food_id": %q, "amount": 50, "unit": "g"}
	]}`, meal.NewIDAt(e.now), meal.NewIDAt(e.now))
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", body, e.session, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a meal refused: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := foodNamed(t, e.in.db, e.in.user.ID, "Kefir", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the kefir of a meal refused: %v, want none", err)
	}
}

// An edit saves its new foods once: none when it is refused, none again
// when an add of it is sent again.
func TestUpdateMealSavesFoodsOnce(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id))
	// Refused for its second op, after a new food in its first.
	refused := fmt.Sprintf(`{"ops": [
		{"op": "add", "item": {"id": "`+meal.NewIDAt(e.now).String()+`", "food": {"name": "Jam", "basis": "100g", "source": "label", "nutrients": {"kcal": 250, "protein": 0.5, "carbs": 60, "fat": 0.1}}, "amount": 20, "unit": "g"}},
		{"op": "replace", "item_id": %q, "item": {"name": "Tea", "nutrients": {"kcal": 2, "protein": 0, "carbs": 0.3, "fat": 0}}}
	]}`, meal.NewIDAt(e.now))
	if rec := e.patch(t, id, refused); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an edit refused: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := foodNamed(t, e.in.db, e.in.user.ID, "Jam", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the jam of an edit refused: %v, want none", err)
	}
	// An add sent again saves its food no more either.
	add := `{"ops": [{"op": "add", "item": {"id": "` + meal.NewIDAt(e.now).String() + `", "food": {"name": "Honey", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 304, "protein": 0.3, "carbs": 82, "fat": 0}}, "amount": 20, "unit": "g"}}]}`
	if rec := e.patch(t, id, add); rec.Code != http.StatusOK {
		t.Fatalf("an add: %d %s", rec.Code, rec.Body.String())
	}
	honey, err := foodNamed(t, e.in.db, e.in.user.ID, "Honey", "")
	if err != nil {
		t.Fatal(err)
	}
	honey.Nutrients.Kcal = 300
	if _, err := e.in.db.SaveFood(t.Context(), e.in.user.ID, isolationActor, honey, e.now); err != nil {
		t.Fatal(err)
	}
	if rec := e.patch(t, id, add); rec.Code != http.StatusOK {
		t.Fatalf("the add again: %d %s", rec.Code, rec.Body.String())
	}
	if got, err := foodNamed(t, e.in.db, e.in.user.ID, "Honey", ""); err != nil || got.Nutrients.Kcal != 300 {
		t.Errorf("the honey after the add again = %v kcal, %v; want the correction kept", got.Nutrients.Kcal, err)
	}
}

// A meal refused saves none of its new foods, whatever refuses it: a food
// in the way of another's alias, an item of another meal's id.
func TestMealsRefusedSaveNoFood(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	other, item := meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"id": %q, "name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, other, item))
	kefir := `{"food": {"name": "Kefir", "basis": "100ml", "source": "label", "nutrients": {"kcal": 50, "protein": 3.4, "carbs": 4.7, "fat": 1.5}}, "amount": 250, "unit": "ml"}`
	for name, second := range map[string]string{
		"a food of a taken alias": `{"food": {"name": "Loaf", "basis": "100g", "source": "label", "aliases": ["Ana's loaf"], "nutrients": {"kcal": 250, "protein": 8, "carbs": 46, "fat": 3}}, "amount": 50, "unit": "g"}`,
		"an item of another meal": fmt.Sprintf(`{"id": %q, "name": "Tea", "nutrients": {"kcal": 2, "protein": 0, "carbs": 0.3, "fat": 0}}`, item),
	} {
		rec := e.in.do(t, http.MethodPost, "/api/v1/meals", fmt.Sprintf(`{"id": %q, "items": [%s, %s]}`, meal.NewIDAt(e.now), kefir, second), e.session, nil)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: %d %s, want 409", name, rec.Code, rec.Body.String())
		}
		if _, err := foodNamed(t, e.in.db, e.in.user.ID, "Kefir", ""); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s: the kefir of a meal refused: %v, want none", name, err)
		}
	}
	// An add of a new food under another meal's item's id.
	add := fmt.Sprintf(`{"ops": [{"op": "add", "item": {"id": %q, "food": {"name": "Jam", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 250, "protein": 0.5, "carbs": 60, "fat": 0.1}}, "amount": 20, "unit": "g"}}]}`, item)
	id := meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Bread", "nutrients": {"kcal": 100, "protein": 3, "carbs": 20, "fat": 1}}]}`, id))
	if rec := e.patch(t, id, add); rec.Code != http.StatusConflict {
		t.Errorf("an add under another meal's item's id: %d %s, want 409", rec.Code, rec.Body.String())
	}
	if _, err := foodNamed(t, e.in.db, e.in.user.ID, "Jam", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the jam of an edit refused: %v, want none", err)
	}
}

// A change of a meal goes back with undo, as it was.
func TestUpdateMealUndone(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id, soup := meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"id": %q, "name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}},
		{"name": "Bread", "nutrients": {"kcal": 100, "protein": 3, "carbs": 20, "fat": 1}}]}`, id, soup))
	mid, err := uuid.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	before, err := e.in.db.MealByID(t.Context(), e.in.user.ID, mid)
	if err != nil {
		t.Fatal(err)
	}
	for name, ops := range map[string]string{
		"a replace": fmt.Sprintf(`{"op": "replace", "item_id": %q, "item": {"name": "Stew", "nutrients": {"kcal": 300, "protein": 15, "carbs": 30, "fat": 12}}}`, soup),
		"a remove":  fmt.Sprintf(`{"op": "remove", "item_id": %q}`, soup),
	} {
		if rec := e.patch(t, id, `{"ops": [`+ops+`]}`); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
		if _, err := e.in.db.UndoLast(t.Context(), e.in.user.ID, isolationActor, e.now); err != nil {
			t.Fatalf("%s: UndoLast: %v", name, err)
		}
		if got, err := e.in.db.MealByID(t.Context(), e.in.user.ID, mid); err != nil || !reflect.DeepEqual(got, before) {
			t.Errorf("%s undone: %+v, %v; want %+v", name, got, err, before)
		}
	}
}

// A request sent again saves no food again, and finds the foods it named
// though they were renamed since: a replace sent again, a meal sent again
// after its new food was renamed.
func TestRetriesFindTheirFoods(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	e.logged(t, e.lunch(id))
	yoghurt, err := foodNamed(t, e.in.db, e.in.user.ID, "Greek yoghurt", "Dukat")
	if err != nil {
		t.Fatal(err)
	}
	yoghurt.Name = "Greek yogurt"
	if _, err := e.in.db.SaveFood(t.Context(), e.in.user.ID, isolationActor, yoghurt, e.now); err != nil {
		t.Fatal(err)
	}
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", e.lunch(id), e.session, nil); rec.Code != http.StatusOK {
		t.Errorf("the meal again, its yoghurt renamed: %d %s, want 200", rec.Code, rec.Body.String())
	}
	if _, err := foodNamed(t, e.in.db, e.in.user.ID, "Greek yoghurt", "Dukat"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the yoghurt under its old name: %v, want none", err)
	}
	// A replace with a new food, sent again after the food was corrected.
	mid, err := uuid.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	m, err := e.in.db.MealByID(t.Context(), e.in.user.ID, mid)
	if err != nil {
		t.Fatal(err)
	}
	replace := fmt.Sprintf(`{"ops": [{"op": "replace", "item_id": %q, "item": {"food": {"name": "Honey", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 304, "protein": 0.3, "carbs": 82, "fat": 0}}, "amount": 20, "unit": "g"}}]}`, m.Items[2].ID)
	if rec := e.patch(t, id, replace); rec.Code != http.StatusOK {
		t.Fatalf("a replace: %d %s", rec.Code, rec.Body.String())
	}
	honey, err := foodNamed(t, e.in.db, e.in.user.ID, "Honey", "")
	if err != nil {
		t.Fatal(err)
	}
	honey.Nutrients.Kcal = 300
	if _, err := e.in.db.SaveFood(t.Context(), e.in.user.ID, isolationActor, honey, e.now); err != nil {
		t.Fatal(err)
	}
	if rec := e.patch(t, id, replace); rec.Code != http.StatusOK {
		t.Fatalf("the replace again: %d %s", rec.Code, rec.Body.String())
	}
	if got, err := foodNamed(t, e.in.db, e.in.user.ID, "Honey", ""); err != nil || got.Nutrients.Kcal != 300 {
		t.Errorf("the honey after the replace again = %v kcal, %v; want the correction kept", got.Nutrients.Kcal, err)
	}
}

// An add of a new food of the same name and brand as a food of the
// user's is that food: the item names it.
func TestUpdateMealAddsAFoodTheUserHas(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	e.logged(t, e.lunch(id))
	item := meal.NewIDAt(e.now)
	add := fmt.Sprintf(`{"ops": [{"op": "add", "item": {"id": %q, "food": {"name": "Greek yoghurt", "brand": "Dukat", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 73, "protein": 9.5, "carbs": 3.6, "fat": 2}}, "amount": 150, "unit": "g"}}]}`, item)
	if rec := e.patch(t, id, add); rec.Code != http.StatusOK {
		t.Fatalf("an add: %d %s", rec.Code, rec.Body.String())
	}
	yoghurt, err := foodNamed(t, e.in.db, e.in.user.ID, "Greek yoghurt", "Dukat")
	if err != nil {
		t.Fatal(err)
	}
	mid, err := uuid.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	m, err := e.in.db.MealByID(t.Context(), e.in.user.ID, mid)
	if err != nil || len(m.Items) != 4 || m.Items[3].ID != item || m.Items[3].FoodID != yoghurt.ID {
		t.Errorf("the meal = %+v, %v; want a fourth item, %v, of the yoghurt %v", m.Items, err, item, yoghurt.ID)
	}
}

// foodNamed is user's food of name and brand, found by search, or
// store.ErrNotFound.
func foodNamed(t *testing.T, db *store.DB, user uuid.UUID, name, brand string) (food.Food, error) {
	t.Helper()
	found, err := db.FindFoods(t.Context(), user, name, 50)
	if err != nil {
		t.Fatal(err)
	}
	for i := range found {
		if found[i].Name == name && found[i].Brand == brand {
			return found[i], nil
		}
	}
	return food.Food{}, store.ErrNotFound
}

// mealOf is the user's meal id as stored.
func (e mealsEnv) mealOf(t *testing.T, id string) meal.Meal {
	t.Helper()
	mid, err := uuid.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	m, err := e.in.db.MealByID(t.Context(), e.in.user.ID, mid)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// A replace of an item of a food of the user's by a new food puts the
// new food in its place, whatever its unit: it is another item, though
// the quantity is the same.
func TestReplaceByANewFood(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"food_id": %q, "amount": 50, "unit": "g"}]}`, id, e.bread))
	item := e.mealOf(t, id).Items[0].ID
	for _, nf := range []struct{ name, food, quantity string }{
		{"Spelt bread", `{"name": "Spelt bread", "brand": "Klara", "basis": "100g", "source": "label",
			"nutrients": {"kcal": 250, "protein": 9, "carbs": 45, "fat": 3}}`, `"amount": 50, "unit": "g"`},
		{"Kefir", `{"name": "Kefir", "basis": "100ml", "source": "label",
			"nutrients": {"kcal": 50, "protein": 3.4, "carbs": 4.7, "fat": 1.5}}`, `"amount": 250, "unit": "ml"`},
	} {
		replace := fmt.Sprintf(`{"ops": [{"op": "replace", "item_id": %q, "item": {"food": %s, %s}}]}`, item, nf.food, nf.quantity)
		if rec := e.patch(t, id, replace); rec.Code != http.StatusOK {
			t.Fatalf("a replace by %s: %d %s", nf.name, rec.Code, rec.Body.String())
		}
		got := e.mealOf(t, id).Items
		if len(got) != 1 || got[0].ID != item || !strings.HasSuffix(got[0].Name, nf.name) {
			t.Errorf("after a replace by %s: %+v", nf.name, got)
		}
		brand := ""
		if nf.name == "Spelt bread" {
			brand = "Klara"
		}
		if f, err := foodNamed(t, e.in.db, e.in.user.ID, nf.name, brand); err != nil || got[0].FoodID != f.ID {
			t.Errorf("the item names %v, want the %s saved, %+v (%v)", got[0].FoodID, nf.name, f, err)
		}
	}
}

// Another meal under a meal's id is a conflict, though it differs only by
// a new food in place of a food of the user's, of the same quantity.
func TestLogMealOtherFoodUnderTheID(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"food_id": %q, "amount": 50, "unit": "g"}]}`, id, e.bread))
	other := fmt.Sprintf(`{"id": %q, "items": [{"food": {"name": "Spelt bread", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 250, "protein": 9, "carbs": 45, "fat": 3}}, "amount": 50, "unit": "g"}]}`, id)
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", other, e.session, nil); rec.Code != http.StatusConflict {
		t.Errorf("another meal under the id: %d %s, want 409", rec.Code, rec.Body.String())
	}
	if _, err := foodNamed(t, e.in.db, e.in.user.ID, "Spelt bread", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the food of the meal refused: %v, want none", err)
	}
}

// The same request sent many times at once, with a new food, is one meal
// and one item: the first logs or adds it, the others are retries.
func TestSentAtOnce(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	const n = 8
	at := func(do func() int) []int {
		codes := make([]int, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() { codes[i] = do() })
		}
		wg.Wait()
		slices.Sort(codes)
		return codes
	}
	id := meal.NewIDAt(e.now).String()
	want := append([]int{http.StatusOK}, make([]int, n-1)...)
	for i := range want {
		want[i] = http.StatusOK
	}
	want[n-1] = http.StatusCreated
	if got := at(func() int { return e.in.do(t, http.MethodPost, "/api/v1/meals", e.lunch(id), e.session, nil).Code }); !slices.Equal(got, want) {
		t.Errorf("a meal sent at once: %v, want one 201 and the rest 200", got)
	}
	add := fmt.Sprintf(`{"ops": [{"op": "add", "item": {"id": %q, "food": {"name": "Honey", "basis": "100g", "source": "label",
		"nutrients": {"kcal": 304, "protein": 0.3, "carbs": 82, "fat": 0}}, "amount": 20, "unit": "g"}}]}`, meal.NewIDAt(e.now))
	if got := at(func() int { return e.patch(t, id, add).Code }); slices.ContainsFunc(got, func(c int) bool { return c != http.StatusOK }) {
		t.Errorf("an add sent at once: %v, want each 200", got)
	}
	if got := e.mealOf(t, id).Items; len(got) != 4 {
		t.Errorf("the meal has %d items, want 4", len(got))
	}
}

// A request sent again is the same though its food no longer takes its
// quantity: nothing of a retry is worked out again.
func TestRetryAfterTheFoodChanged(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id := meal.NewIDAt(e.now).String()
	muesli := `{"food": {"name": "Muesli", "basis": "100g", "source": "label", "servings": [{"name": "cup", "amount": 45}],
		"nutrients": {"kcal": 370, "protein": 10, "carbs": 60, "fat": 7}}, "servings": 1, "serving": "cup"}`
	logMuesli := fmt.Sprintf(`{"id": %q, "items": [%s]}`, id, muesli)
	e.logged(t, logMuesli)
	add := fmt.Sprintf(`{"ops": [{"op": "add", "item": {"id": %q, "food_id": %q, "servings": 1, "serving": "slice"}}]}`, meal.NewIDAt(e.now), e.bread)
	if rec := e.patch(t, id, add); rec.Code != http.StatusOK {
		t.Fatalf("an add: %d %s", rec.Code, rec.Body.String())
	}
	for _, name := range []string{"Muesli", "Ana's rye bread"} {
		f, err := foodNamed(t, e.in.db, e.in.user.ID, name, "")
		if err != nil {
			t.Fatal(err)
		}
		f.Servings = []food.Serving{{Name: "bowl", Amount: 60}}
		if _, err := e.in.db.SaveFood(t.Context(), e.in.user.ID, isolationActor, f, e.now); err != nil {
			t.Fatal(err)
		}
	}
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", logMuesli, e.session, nil); rec.Code != http.StatusOK {
		t.Errorf("the meal again: %d %s, want 200", rec.Code, rec.Body.String())
	}
	if rec := e.patch(t, id, add); rec.Code != http.StatusOK {
		t.Errorf("the add again: %d %s, want 200", rec.Code, rec.Body.String())
	}
}
