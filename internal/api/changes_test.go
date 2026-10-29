package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// changesSaid is each change of an answer of list-changes in a line:
// what it did, through what, by whom, of what and which days, and
// whether it is undone or next.
func changesSaid(t *testing.T, body []byte) []string {
	t.Helper()
	var a struct {
		Changes []struct {
			ID, Action, Entity, Client, Model, Name, Brand, Metric, Status string
			Days, Items                                                    []string
			Archived                                                       *bool
			Undone, Next                                                   bool
		} `json:"changes"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		t.Fatalf("the changes: %v: %s", err, body)
	}
	said := make([]string, 0, len(a.Changes))
	for i := range a.Changes {
		c := &a.Changes[i]
		if _, err := uuid.Parse(c.ID); err != nil {
			t.Errorf("a change's id %q: %v", c.ID, err)
		}
		s := c.Action + " via " + c.Client
		if c.Model != "" {
			s += " (" + c.Model + ")"
		}
		for _, part := range []string{c.Name, c.Brand, strings.Join(c.Items, "+"), c.Metric, c.Status, strings.Join(c.Days, ",")} {
			if part != "" {
				s += " · " + part
			}
		}
		if c.Archived != nil {
			s += fmt.Sprintf(" · archived %t", *c.Archived)
		}
		if c.Undone {
			s += " · undone"
		}
		if c.Next {
			s += " · next"
		}
		said = append(said, s)
	}
	return said
}

// saveChanges gives in's user a change of each kind, through the web and
// an AI coach, and a device's import: a food saved and archived, a meal
// the coach logged, a weigh-in, steps imported, and a snack logged and
// deleted, the delete undone. It returns the snack's id.
func saveChanges(t *testing.T, in *instance) uuid.UUID {
	t.Helper()
	ctx, now := t.Context(), in.clock.Now()
	today := day.DateOf(now)
	web := event.Actor{Kind: event.User, Client: "web"}
	coach := event.Actor{Kind: event.User, Client: "mcp", Model: "claude-opus-5-5"}

	skyr := food.Food{
		ID: uuid.NewV7(), Name: "Skyr", Brand: "Dukat", Basis: food.Per100g, Source: food.Label,
		Nutrients: food.Nutrients{Kcal: 63, Protein: 11, Carbs: 4, Fat: 0.2},
	}
	if _, err := in.db.SaveFood(ctx, in.user.ID, web, skyr, now); err != nil {
		t.Fatal(err)
	}
	if err := in.db.SetFoodArchived(ctx, in.user.ID, web, skyr.ID, true, now); err != nil {
		t.Fatal(err)
	}
	estimate := func(name string) meal.Item {
		it, err := meal.Estimate(name, food.Nutrients{Kcal: 200, Protein: 10, Carbs: 20, Fat: 8}, "")
		if err != nil {
			t.Fatal(err)
		}
		return it
	}
	lunch := meal.Meal{
		ID: meal.NewIDAt(now), EatenAt: now, Day: today, Description: "Lunch", Client: meal.ClientMCP,
		Items: []meal.Item{estimate("Soup"), estimate("Bread")},
	}
	if _, _, err := in.db.CreateMeal(ctx, in.user.ID, coach, lunch, now); err != nil {
		t.Fatal(err)
	}
	if _, err := in.db.SetMetric(ctx, in.user.ID, web, today, metric.Value{Kind: metric.Weight, Value: 72.5, Source: metric.Manual}, now); err != nil {
		t.Fatal(err)
	}
	// A device's import, which undo leaves, and is not listed.
	if _, err := in.db.MergeMetrics(ctx, in.user.ID, event.Actor{Kind: event.Import, Client: "hae"}, today,
		[]metric.Value{{Kind: metric.Steps, Value: 4000, Source: metric.Import}}, now); err != nil {
		t.Fatal(err)
	}
	snack := meal.Meal{ID: meal.NewIDAt(now), EatenAt: now, Day: today, Client: meal.ClientWeb, Items: []meal.Item{estimate("Apple")}}
	if _, _, err := in.db.CreateMeal(ctx, in.user.ID, web, snack, now); err != nil {
		t.Fatal(err)
	}
	if err := in.db.DeleteMeal(ctx, in.user.ID, web, snack.ID, now); err != nil {
		t.Fatal(err)
	}
	// The delete taken back: the snack's log is next.
	if _, err := in.db.UndoLast(ctx, in.user.ID, web, now); err != nil {
		t.Fatal(err)
	}
	return snack.ID
}

// list-changes is the user's latest changes, the latest first, through
// any client, each with the names of what it was of: a meal's as the
// change left it (as it was, for a delete), a food's; whether an undo
// took it back, and the one undo takes back next. The undos and an
// import's writes are not among them.
func TestListChanges(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	snack := saveChanges(t, in)

	rec := in.do(t, http.MethodGet, "/api/v1/changes", "", session, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	d := day.DateOf(in.clock.Now()).String()
	want := []string{
		"meal.delete via web · Apple · " + d + " · undone",
		"meal.create via web · Apple · " + d + " · next",
		"metric.set via web · weight · " + d,
		"meal.create via mcp (claude-opus-5-5) · Lunch · Soup+Bread · " + d,
		"food.archive via web · Skyr · Dukat · archived true",
		"food.save via web · Skyr · Dukat",
	}
	if got := changesSaid(t, rec.Body.Bytes()); !slices.Equal(got, want) {
		t.Errorf("the changes:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	rec = in.do(t, http.MethodGet, "/api/v1/changes?limit=2", "", session, nil)
	if got := changesSaid(t, rec.Body.Bytes()); rec.Code != http.StatusOK || !slices.Equal(got, want[:2]) {
		t.Errorf("the latest two: %d %s", rec.Code, rec.Body.String())
	}
	// Undo the latest named, as the list's Undo does: the snack's log goes,
	// and the measurement is next.
	undo := fmt.Sprintf(`{"last": {"action": "meal.create", "entity": %q}}`, snack)
	if rec := in.do(t, http.MethodPost, "/api/v1/undo", undo, session, nil); rec.Code != http.StatusOK {
		t.Fatalf("the undo: %d %s", rec.Code, rec.Body.String())
	}
	rec = in.do(t, http.MethodGet, "/api/v1/changes?limit=3", "", session, nil)
	want = []string{
		"meal.delete via web · Apple · " + d + " · undone",
		"meal.create via web · Apple · " + d + " · undone",
		"metric.set via web · weight · " + d + " · next",
	}
	if got := changesSaid(t, rec.Body.Bytes()); !slices.Equal(got, want) {
		t.Errorf("after the undo:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The plan's changes are listed too, which undo leaves: a proposal says
// its status after the change, and none of them is next.
func TestListChangesOfThePlan(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	saveIsolationPlan(t, in.db, in.user, in.clock.Now())

	rec := in.do(t, http.MethodGet, "/api/v1/changes?limit=50", "", session, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	got := changesSaid(t, rec.Body.Bytes())
	today := day.DateOf(in.clock.Now()).String()
	if len(got) == 0 || got[0] != "day.checkin via web · "+today+" · next" {
		t.Errorf("the latest, the check-in, next: %q", got)
	}
	for _, want := range []string{"proposal.status via web · applied", "proposal.create via web", "limits.set via web", "notes.set via web"} {
		if !slices.Contains(got, want) {
			t.Errorf("no %q among the changes: %q", want, got)
		}
	}
	if n := len(slices.DeleteFunc(slices.Clone(got), func(s string) bool { return !strings.HasSuffix(s, " · next") })); n != 1 {
		t.Errorf("%d changes next, want one: %q", n, got)
	}
}

// With nothing to undo, no change is next; with no change, the list is
// empty, not null.
func TestListChangesNoneNext(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	if rec := in.do(t, http.MethodGet, "/api/v1/changes", "", session, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"changes":[]`) {
		t.Errorf("no changes: %d %s", rec.Code, rec.Body.String())
	}
	if err := in.db.SetProfileNotes(t.Context(), in.user.ID, event.Actor{Kind: event.User, Client: "web"}, "No fish.", in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := in.db.UndoLast(t.Context(), in.user.ID, event.Actor{Kind: event.User, Client: "web"}, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	rec := in.do(t, http.MethodGet, "/api/v1/changes", "", session, nil)
	if got := changesSaid(t, rec.Body.Bytes()); !slices.Equal(got, []string{"notes.set via web · undone"}) {
		t.Errorf("the notes taken back: %q", got)
	}
}
