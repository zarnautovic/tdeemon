package api_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/docs"
	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/coaching"
)

// guideBody is get-coaching-guide's answer.
type guideBody struct {
	Session string `json:"session"`
	Title   string `json:"title"`
	Guide   string `json:"guide"`
}

func TestCoachingGuide(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	for _, s := range coaching.Sessions() {
		rec := in.do(t, http.MethodGet, "/api/v1/coaching/"+s.Name, "", []*http.Cookie{session}, nil)
		var got guideBody
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("%s: %d %s", s.Name, rec.Code, rec.Body.String())
		}
		if got.Session != s.Name || got.Title != s.Title || got.Guide != s.Guide() {
			t.Errorf("%s: session %q, title %q, guide %d bytes; want %q, %q, its guide", s.Name, got.Session, got.Title, len(got.Guide), s.Name, s.Title)
		}
		if !strings.Contains(got.Guide, docs.Methodology) {
			t.Errorf("%s: the guide lacks the methodology", s.Name)
		}
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/coaching/weekly_review", "", []*http.Cookie{session}, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a session that does not exist: %d %s, want 422", rec.Code, rec.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/coaching/monthly_review", "", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out: %d, want 401", rec.Code)
	}
}

// TestCoachingGuideReadToken: a token that only reads gets the guide,
// through REST and MCP.
func TestCoachingGuideReadToken(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	token := mcpToken(t, in, session, `"read"`)
	if rec := in.do(t, http.MethodGet, "/api/v1/coaching/new_phase", "", nil, bearer(token)); rec.Code != http.StatusOK {
		t.Errorf("REST with a read token: %d %s", rec.Code, rec.Body.String())
	}
	text, isErr := callTool(t, mcpSession(t, in, token), "get_coaching_guide", map[string]any{"session": "new_phase"})
	var got guideBody
	if isErr || json.Unmarshal([]byte(text), &got) != nil || got.Title != "Plan a new phase" {
		t.Errorf("MCP with a read token: %s", text)
	}
}

// TestCoachingSessionsInTheDocument holds the operation's enum to the
// sessions there are.
func TestCoachingSessionsInTheDocument(t *testing.T) {
	t.Parallel()

	spec, err := api.Spec()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name   string `json:"name"`
				Schema struct {
					Enum []string `json:"enum"`
				} `json:"schema"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	op, ok := doc.Paths["/api/v1/coaching/{session}"]["get"]
	if !ok || len(op.Parameters) != 1 || !slices.Equal(op.Parameters[0].Schema.Enum, coaching.Names()) {
		t.Errorf("the operation's sessions = %+v, want %v", op.Parameters, coaching.Names())
	}
}

// TestCoachingGuidesNameWhatThereIs fails for a name in backticks in a
// guide, the steps or the methodology, that is no tool, no field of a
// tool's input or answer, nor a value the document lists: a model would
// look for what is not there.
func TestCoachingGuidesNameWhatThereIs(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	res, err := mcpSession(t, in, mcpToken(t, in, session, `"tools"`)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, tl := range res.Tools {
		known[tl.Name] = true
		propertyNames(tl.InputSchema, known)
	}
	toolAnswerNames(t, known)
	spec, err := api.Spec()
	if err != nil {
		t.Fatal(err)
	}
	var whole any
	if err := json.Unmarshal(spec, &whole); err != nil {
		t.Fatal(err)
	}
	enumValues(whole, known)
	// The user's limits, by their names in docs/limits.md.
	for _, name := range []string{"logged_days", "weigh_ins"} {
		known[name] = true
	}
	// A value a field's words list, as a verdict's reasons.
	said := func(name string) bool {
		return regexp.MustCompile(`\b` + name + `\b`).Match(spec)
	}
	for _, s := range coaching.Sessions() {
		for _, m := range regexp.MustCompile("`([a-z][a-z_.]*)`").FindAllStringSubmatch(s.Guide(), -1) {
			for part := range strings.SplitSeq(m[1], ".") {
				if !known[part] && !said(part) {
					t.Errorf("%s's guide names %q, which is no tool, field or value", s.Name, m[1])
				}
			}
		}
	}
}

// enumValues adds to values every value an enum of schema lists.
func enumValues(schema any, values map[string]bool) {
	switch v := schema.(type) {
	case map[string]any:
		if enum, ok := v["enum"].([]any); ok {
			for _, e := range enum {
				if s, ok := e.(string); ok {
					values[s] = true
				}
			}
		}
		for _, e := range v {
			enumValues(e, values)
		}
	case []any:
		for _, e := range v {
			enumValues(e, values)
		}
	}
}
