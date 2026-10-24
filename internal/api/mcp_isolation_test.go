package api_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// mcpIsolationCase calls a tool as the other party, through cs, their
// MCP session.
type mcpIsolationCase func(t *testing.T, e isolationEnv, cs *mcp.ClientSession)

// mcpWorks is the case of a tool the other calls on what is theirs: it
// must not fail, and must show what shows returns of the other's own.
//
//nolint:thelper // A case is a test, not a helper: a failure should point into it.
func mcpWorks(name string, args func(e isolationEnv) map[string]any, shows func(e isolationEnv) []string) mcpIsolationCase {
	return func(t *testing.T, e isolationEnv, cs *mcp.ClientSession) {
		var a map[string]any
		if args != nil {
			a = args(e)
		}
		text, isErr := callTool(t, cs, name, a)
		if isErr {
			t.Fatalf("%s of their own: %s", name, text)
		}
		if shows != nil {
			for _, s := range shows(e) {
				if !strings.Contains(text, s) {
					t.Errorf("%s shows none of %q: %s", name, s, text)
				}
			}
		}
	}
}

// mcpOnTheirs is the case of a tool that names an entity by its id: the
// owner's is the error of one that does not exist, word for word, and
// the other's own works.
//
//nolint:thelper // A case is a test, not a helper: a failure should point into it.
func mcpOnTheirs(name string, owners, own func(e isolationEnv) string, args map[string]any) mcpIsolationCase {
	return func(t *testing.T, e isolationEnv, cs *mcp.ClientSession) {
		with := func(id string) map[string]any {
			a := map[string]any{"id": id}
			maps.Copy(a, args)
			return a
		}
		theirs, theirsErr := callTool(t, cs, name, with(owners(e)))
		none, noneErr := callTool(t, cs, name, with(uuid.NewV7().String()))
		if !theirsErr || theirs != none || !noneErr {
			t.Errorf("%s of the owner's: %s; of none: %s; want the same error", name, theirs, none)
		}
		if text, isErr := callTool(t, cs, name, with(own(e))); isErr {
			t.Errorf("%s of their own: %s", name, text)
		}
	}
}

// mcpIsolationCases has a case for every tool MCP serves:
// TestMCPIsolation runs each as the other party, a user and an admin,
// and checks the owner was left as they were; TestMCPIsolationCovers-
// EveryTool fails for a tool without a case.
func mcpIsolationCases() map[string]mcpIsolationCase {
	name := func(e isolationEnv) []string { return []string{e.other.user.Name} }
	ownersBread := func(e isolationEnv) string { return e.owner.bread.ID.String() }
	ownBread := func(e isolationEnv) string { return e.other.bread.ID.String() }
	ownersMeal := func(e isolationEnv) string { return e.owner.meal.String() }
	ownMeal := func(e isolationEnv) string { return e.other.meal.String() }
	return map[string]mcpIsolationCase{
		"get_profile": mcpWorks("get_profile", nil, name),
		"get_day":     mcpWorks("get_day", nil, func(e isolationEnv) []string { return []string{e.other.user.Name + "'s lunch"} }),
		"get_summary": mcpWorks("get_summary", nil, func(e isolationEnv) []string {
			return []string{fmt.Sprintf(`"weight_kg":%v`, isolationWeight(e.other.user)), fmt.Sprintf(`"kcal":%v`, isolationKcal(e.other.user))}
		}),
		"get_history": mcpWorks("get_history", nil, func(e isolationEnv) []string {
			return []string{fmt.Sprintf(`"weight_kg":%v`, isolationWeight(e.other.user))}
		}),
		"update_profile_notes": mcpWorks("update_profile_notes", func(e isolationEnv) map[string]any {
			return map[string]any{"notes": e.other.user.Name + " avoids nuts."}
		}, func(e isolationEnv) []string { return []string{e.other.user.Name + " avoids nuts."} }),
		"find_foods": mcpWorks("find_foods", func(isolationEnv) map[string]any { return map[string]any{"query": "rye"} },
			func(e isolationEnv) []string { return []string{e.other.user.Name + "'s rye bread"} }),
		"get_food": mcpOnTheirs("get_food", ownersBread, ownBread, nil),
		"upsert_food": mcpWorks("upsert_food", func(e isolationEnv) map[string]any {
			return map[string]any{
				"name": e.other.user.Name + "'s crackers", "basis": "100g", "source": "user",
				"nutrients": map[string]any{"kcal": 420, "protein": 10, "carbs": 70, "fat": 10},
			}
		}, nil),
		"log_meal": mcpWorks("log_meal", func(e isolationEnv) map[string]any {
			return map[string]any{
				"id": meal.NewIDAt(e.in.clock.Now()).String(), "description": e.other.user.Name + "'s dinner",
				"items": []any{map[string]any{"food_id": e.other.bread.ID.String(), "servings": 1, "serving": "slice"}},
			}
		}, nil),
		"update_meal": mcpOnTheirs("update_meal", ownersMeal, ownMeal, map[string]any{"note": "seen"}),
		"delete_meal": mcpOnTheirs("delete_meal", ownersMeal, ownMeal, nil),
		"log_metric": mcpWorks("log_metric", func(isolationEnv) map[string]any {
			return map[string]any{"kind": "weight", "value": 77.3}
		}, nil),
		"delete_metric": mcpWorks("delete_metric", func(isolationEnv) map[string]any { return map[string]any{"kind": "weight"} }, nil),
		"undo_last": mcpWorks("undo_last", func(e isolationEnv) map[string]any {
			return map[string]any{"id": clientid.NewAt(e.in.clock.Now()).String()}
		}, nil),
		"checkin": mcpWorks("checkin", func(isolationEnv) map[string]any {
			return map[string]any{"mood": 2, "note": "a quiet day", "training_day": "rest", "excluded_reason": "sick"}
		}, nil),
	}
}

func TestMCPIsolation(t *testing.T) {
	t.Parallel()

	for who, adminActs := range map[string]bool{"a user acts": false, "an admin acts": true} {
		t.Run(who, func(t *testing.T) {
			t.Parallel()
			for tool, run := range mcpIsolationCases() {
				t.Run(tool, func(t *testing.T) {
					t.Parallel()
					e := newIsolationEnv(t, adminActs)
					e.isolated(t, func(t *testing.T, e isolationEnv) {
						t.Helper()
						run(t, e, mcpSession(t, e.in, e.other.token.Token))
					}, func() {})
				})
			}
		})
	}
}

// Every tool MCP serves has an isolation case, and every case a tool.
func TestMCPIsolationCoversEveryTool(t *testing.T) {
	t.Parallel()

	e := newIsolationEnv(t, false)
	res, err := mcpSession(t, e.in, e.other.token.Token).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := mcpIsolationCases()
	var tools []string
	for _, tl := range res.Tools {
		tools = append(tools, tl.Name)
		if _, ok := cases[tl.Name]; !ok {
			t.Errorf("the tool %s has no case in mcpIsolationCases", tl.Name)
		}
	}
	for name := range cases {
		if !slices.Contains(tools, name) {
			t.Errorf("mcpIsolationCases has a case for %s, which MCP does not serve", name)
		}
	}
	if len(tools) == 0 {
		t.Error("MCP serves no tool to a token of the tools scope")
	}
}
