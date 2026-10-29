package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// undo sends an undo under id, and reads what it took back.
func undo(t *testing.T, e mealsEnv, id uuid.UUID) (int, string, string) {
	t.Helper()
	rec := e.in.do(t, http.MethodPost, "/api/v1/undo", fmt.Sprintf(`{"id": %q}`, id), e.session, nil)
	var a struct {
		Undone struct {
			Action, Entity, Client string
		} `json:"undone"`
		Repeated bool `json:"repeated"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		got := a.Undone.Action + " " + a.Undone.Entity + " " + a.Undone.Client
		if a.Repeated {
			got += " (repeated)"
		}
		return rec.Code, got, rec.Body.String()
	}
	return rec.Code, "", rec.Body.String()
}

// undo_last takes back the latest change, through whatever client; sent
// again under its id, it takes back no other; with nothing left it is a
// 404.
func TestUndoLast(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	first, second := meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String()
	for _, id := range []string{first, second} {
		e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id))
	}
	request := clientid.NewAt(e.now)
	// In order: the second send is the repeated one.
	for _, step := range []struct{ name, want string }{{"an undo", ""}, {"the undo again", " (repeated)"}} {
		if code, got, body := undo(t, e, request); code != http.StatusOK || got != "meal.create "+second+" web"+step.want {
			t.Errorf("%s: %d %s", step.name, code, body)
		}
	}
	if rec := e.in.do(t, http.MethodGet, "/api/v1/days/today", "", e.session, nil); !strings.Contains(rec.Body.String(), first) || strings.Contains(rec.Body.String(), second) {
		t.Errorf("the day after the undo: %s", rec.Body.String())
	}
	if code, got, body := undo(t, e, clientid.NewAt(e.now)); code != http.StatusOK || got != "meal.create "+first+" web" {
		t.Errorf("another undo: %d %s", code, body)
	}
	// The foods of the meals' fixture, then nothing.
	for {
		code, _, body := undo(t, e, clientid.NewAt(e.now))
		if code == http.StatusNotFound {
			if !strings.Contains(body, "nothing to undo") {
				t.Errorf("nothing left: %s", body)
			}
			break
		}
		if code != http.StatusOK {
			t.Fatalf("an undo: %d %s", code, body)
		}
	}
}

// The answer says the days the change taken back was of, so a client
// showing another day can say where it went: both days of a meal moved,
// the day of one logged, none of a food.
func TestUndoLastSaysItsDays(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	upsert := `{"name": "Kefir", "basis": "100ml", "source": "label", "nutrients": {"kcal": 50, "protein": 3.4, "carbs": 4.7, "fat": 1.5}}`
	if rec := e.in.do(t, http.MethodPost, "/api/v1/foods", upsert, e.session, nil); rec.Code != http.StatusOK {
		t.Fatalf("a food: %d %s", rec.Code, rec.Body.String())
	}
	id := meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id))
	if rec := e.patch(t, id, fmt.Sprintf(`{"eaten_at": %q}`, e.now.Add(-24*time.Hour).Format(time.RFC3339))); rec.Code != http.StatusOK {
		t.Fatalf("the meal moved to the day before: %d %s", rec.Code, rec.Body.String())
	}
	today := day.DateOf(e.now)
	for _, want := range []string{
		fmt.Sprintf(`[%q,%q]`, today.AddDays(-1), today), // the move
		fmt.Sprintf(`[%q]`, today),                       // the meal logged
		`[]`,                                             // the food
	} {
		rec := e.in.do(t, http.MethodPost, "/api/v1/undo", fmt.Sprintf(`{"id": %q}`, clientid.NewAt(e.now)), e.session, nil)
		var a struct {
			Undone struct {
				Action string          `json:"action"`
				Days   json.RawMessage `json:"days"`
			} `json:"undone"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &a) != nil || string(a.Undone.Days) != want {
			t.Errorf("an undo: %d %s, want the days %s", rec.Code, rec.Body.String(), want)
		}
	}
}

// A change undo cannot take back is a 409 that says why, and is passed
// over: sent again under its id, the undo says so again; a new undo goes
// on to the change before.
func TestUndoLastConflict(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	upsert := `{"name": "Kefir", "basis": "100ml", "source": "label", "nutrients": {"kcal": 50, "protein": 3.4, "carbs": 4.7, "fat": 1.5}}`
	if rec := e.in.do(t, http.MethodPost, "/api/v1/foods", upsert, e.session, nil); rec.Code != http.StatusOK {
		t.Fatalf("a food: %d %s", rec.Code, rec.Body.String())
	}
	// An import corrects the food since.
	kefir, err := foodNamed(t, e.in.db, e.in.user.ID, "Kefir", "")
	if err != nil {
		t.Fatal(err)
	}
	kefir.Note = "the label's"
	if _, err := e.in.db.SaveFood(t.Context(), e.in.user.ID, event.Actor{Kind: event.Import, Client: "hae"}, kefir, e.now); err != nil {
		t.Fatal(err)
	}
	request := clientid.NewAt(e.now)
	for _, want := range []string{"it changed since", "this undo passed over it before"} {
		if code, _, body := undo(t, e, request); code != http.StatusConflict || !strings.Contains(body, "food.save of "+kefir.ID.String()+" cannot be undone: "+want) ||
			!strings.Contains(body, "an undo under a new id") {
			t.Errorf("an undo of the food: %d %s, want 409 saying %q", code, body, want)
		}
	}
	if code, got, body := undo(t, e, clientid.NewAt(e.now)); code != http.StatusOK || strings.Contains(got, kefir.ID.String()) {
		t.Errorf("a new undo: %d %s, want the change before the food's", code, body)
	}
}

// A new undo's id is a version 7 UUID made near the clock.
func TestUndoLastRefusesItsID(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	for name, tc := range map[string]struct{ id, want string }{
		"version 4":    {"f47ac10b-58cc-4372-a567-0e02b2c3d479", "not a version 7"},
		"two days old": {clientid.NewAt(e.now.Add(-48 * time.Hour)).String(), "more than a day before"},
		"no UUID":      {"undo-1", "id"},
		"nil":          {"00000000-0000-0000-0000-000000000000", "not a version 7"},
	} {
		rec := e.in.do(t, http.MethodPost, "/api/v1/undo", fmt.Sprintf(`{"id": %q}`, tc.id), e.session, nil)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s, want 422 saying %q", name, rec.Code, rec.Body.String(), tc.want)
		}
	}
}

// undo_last takes back a change made through any client, and says which.
func TestUndoLastSaysTheClient(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	tok, rec := e.in.createToken(t, e.session[0], `{"name":"Claude","scopes":["tools"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("a token: %d %s", rec.Code, rec.Body.String())
	}
	id := meal.NewIDAt(e.now).String()
	body := fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id)
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", body, nil, bearer(tok.Token)); rec.Code != http.StatusCreated {
		t.Fatalf("a meal through the API: %d %s", rec.Code, rec.Body.String())
	}
	if code, got, body := undo(t, e, clientid.NewAt(e.now)); code != http.StatusOK || got != "meal.create "+id+" api" {
		t.Errorf("an undo of it: %d %s, want the client api", code, body)
	}
}

// What kept a change from being taken back is in the log, not the answer.
func TestUndoLastLogsWhy(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, meal.NewIDAt(e.now)))
	e.in.exec(t, "UPDATE events SET entity = 'not-an-id' WHERE operation = 'meal.create'")
	code, _, body := undo(t, e, clientid.NewAt(e.now))
	if code != http.StatusConflict || strings.Contains(body, "invalid") {
		t.Errorf("an undo of a change that does not read: %d %s", code, body)
	}
	if logs := e.in.logs.String(); !strings.Contains(logs, `entity \"not-an-id\"`) {
		t.Errorf("the log has no cause: %s", logs)
	}
}

// An undo that answered there is nothing to undo, sent again after a
// change since, answers so again: it takes back no change made after it.
func TestUndoLastOfNothingSentAgain(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	e := mealsEnv{in: in, session: []*http.Cookie{sessionOf(t, in)}, now: in.clock.Now()}
	request := clientid.NewAt(e.now)
	if code, _, body := undo(t, e, request); code != http.StatusNotFound {
		t.Fatalf("an undo of nothing: %d %s", code, body)
	}
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, meal.NewIDAt(e.now)))
	if code, _, body := undo(t, e, request); code != http.StatusNotFound || !strings.Contains(body, "nothing to undo") {
		t.Errorf("the undo of nothing again: %d %s, want 404", code, body)
	}
}

// An undo without an id, from a client that cannot make one, takes back
// the latest change under one the server makes; sent again, it is
// another undo, which takes back the change before.
func TestUndoLastWithoutAnID(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	first, second := meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String()
	for _, id := range []string{first, second} {
		e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id))
	}
	for _, want := range []string{second, first} {
		rec := e.in.do(t, http.MethodPost, "/api/v1/undo", `{}`, e.session, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) || strings.Contains(rec.Body.String(), "repeated") {
			t.Errorf("an undo without an id: %d %s, want the create of %s", rec.Code, rec.Body.String(), want)
		}
	}
}

// The notes a coach sets through MCP are the client mcp's, which an undo
// says and takes back.
func TestUndoLastTakesBackTheCoachsNotes(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	tok, rec := e.in.createToken(t, e.session[0], `{"name":"Claude","scopes":["tools"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("a token: %d %s", rec.Code, rec.Body.String())
	}
	cs := mcpSession(t, e.in, tok.Token)
	if text, isErr := callTool(t, cs, "update_profile_notes", map[string]any{"notes": "Knees: no running."}); isErr {
		t.Fatalf("the notes: %s", text)
	}
	if text, _ := callTool(t, cs, "get_profile", nil); !strings.Contains(text, "Knees: no running.") {
		t.Errorf("the profile without the notes: %s", text)
	}
	if code, got, body := undo(t, e, clientid.NewAt(e.now)); code != http.StatusOK || got != "notes.set notes mcp" {
		t.Errorf("an undo of them: %d %s, want notes.set of the client mcp", code, body)
	}
	if text, _ := callTool(t, cs, "get_profile", nil); strings.Contains(text, "Knees") {
		t.Errorf("the notes after the undo: %s", text)
	}
}

// An undo that names its change (last, the web app's) takes it back only
// while it is the latest: after another, of any client, it takes back
// nothing and says so (412). MCP neither shows nor takes last.
func TestUndoOfTheLatest(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	first, second := meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String()
	for _, id := range []string{first, second} {
		e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id))
	}
	of := func(id string) string {
		return fmt.Sprintf(`{"id": %q, "last": {"action": "meal.create", "entity": %q}}`, clientid.NewAt(e.now), id)
	}
	rec := e.in.do(t, http.MethodPost, "/api/v1/undo", of(first), e.session, nil)
	if rec.Code != http.StatusPreconditionFailed || !strings.Contains(rec.Body.String(), "meal.create of "+second) {
		t.Errorf("undo of the first meal: %d %s, want 412 naming the second", rec.Code, rec.Body.String())
	}
	if rec := e.in.do(t, http.MethodGet, "/api/v1/days/today", "", e.session, nil); !strings.Contains(rec.Body.String(), first) || !strings.Contains(rec.Body.String(), second) {
		t.Errorf("the day after the undo of another: %s, want both meals", rec.Body.String())
	}
	rec = e.in.do(t, http.MethodPost, "/api/v1/undo", of(second), e.session, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), second) {
		t.Errorf("undo of the second meal, the latest: %d %s", rec.Code, rec.Body.String())
	}

	cs := mcpSession(t, e.in, mcpToken(t, e.in, e.session[0], `"tools"`))
	last := map[string]any{"action": "meal.create", "entity": first}
	if text, isErr := callTool(t, cs, "undo_last", map[string]any{"last": last}); !isErr || !strings.Contains(text, `no argument "last"`) {
		t.Errorf("undo through MCP, naming its change: %s", text)
	}
}

// An estimate saved as a food in a meal's change (the web's Save as a
// food) is one change with it: its undo, named as the web's toast names
// it, takes the food back too, and leaves no undo of the food alone.
func TestUndoOfAMealChangeTakesBackItsNewFood(t *testing.T) {
	t.Parallel()

	e := newMealsEnv(t)
	id, soup := meal.NewIDAt(e.now).String(), meal.NewIDAt(e.now).String()
	e.logged(t, fmt.Sprintf(`{"id": %q, "items": [{"id": %q, "name": "Canteen soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}]}`, id, soup))
	before := e.mealOf(t, id)
	asFood := fmt.Sprintf(`{"ops": [{"op": "replace", "item_id": %q, "item": {"food": {"name": "Canteen soup", "basis": "portion", "source": "estimate",
		"nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}, "portions": 1}}]}`, soup)
	if rec := e.patch(t, id, asFood); rec.Code != http.StatusOK {
		t.Fatalf("save the estimate as a food: %d %s", rec.Code, rec.Body.String())
	}
	saved, err := foodNamed(t, e.in.db, e.in.user.ID, "Canteen soup", "")
	if err != nil {
		t.Fatalf("the food saved: %v", err)
	}

	rec := e.in.do(t, http.MethodPost, "/api/v1/undo",
		fmt.Sprintf(`{"id": %q, "last": {"action": "meal.replace", "entity": %q}}`, clientid.NewAt(e.now), id), e.session, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"action":"meal.replace"`) {
		t.Fatalf("undo of the change: %d %s", rec.Code, rec.Body.String())
	}
	if got := e.mealOf(t, id); !reflect.DeepEqual(got, before) {
		t.Errorf("the meal after the undo = %+v, want it as before, %+v", got, before)
	}
	if f, err := e.in.db.FoodByID(t.Context(), e.in.user.ID, saved.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the food after the undo = %+v, %v; want it gone with the change", f, err)
	}
	// The next undo takes back the meal's log: the food's save is no
	// change of its own.
	if code, got, body := undo(t, e, clientid.NewAt(e.now)); code != http.StatusOK || got != "meal.create "+id+" web" {
		t.Errorf("the next undo: %d %s, want the meal's log", code, body)
	}
}
