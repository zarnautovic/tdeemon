package api_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/zarnautovic/tdeemon/internal/api"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// bearerTransport sends every request with token.
type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession connects to in's MCP server with token, through in's
// handler as it is at each request (a test may wrap it).
func mcpSession(t *testing.T, in *instance, token string) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { in.h.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := c.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}}, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// mcpToken is a token of in's user with scopes.
func mcpToken(t *testing.T, in *instance, session *http.Cookie, scopes string) string {
	t.Helper()
	tok, rec := in.createToken(t, session, `{"name":"Claude `+strings.ReplaceAll(scopes, `"`, "")+`","scopes":[`+scopes+`]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("a token: %d %s", rec.Code, rec.Body.String())
	}
	return tok.Token
}

// callTool calls name with args, and returns the result's text and
// whether it is an error of the tool.
func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	var text []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text = append(text, tc.Text)
		}
	}
	return strings.Join(text, "\n"), res.IsError
}

// The tools a token opens are the tools MCP lists: all of them with the
// tools scope, those that read with read. Each one's input schema stands
// alone, and says whether it only reads.
func TestMCPListsTheTools(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	names := func(scopes string) []string {
		res, err := mcpSession(t, in, mcpToken(t, in, session, scopes)).ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, tl := range res.Tools {
			out = append(out, tl.Name)
			schema, _ := json.Marshal(tl.InputSchema)
			if strings.Contains(string(schema), "$ref") || !strings.Contains(string(schema), `"type":"object"`) {
				t.Errorf("%s's input schema: %s", tl.Name, schema)
			}
			readOnly := slices.Contains([]string{"get_profile", "get_day", "get_summary", "get_history", "find_foods", "get_food"}, tl.Name)
			destructive := slices.Contains([]string{"upsert_food", "update_meal", "delete_meal", "log_metric", "delete_metric", "checkin", "undo_last", "update_profile_notes"}, tl.Name)
			if a := tl.Annotations; a == nil || a.ReadOnlyHint != readOnly || a.DestructiveHint == nil || *a.DestructiveHint != destructive || tl.Description == "" {
				t.Errorf("%s: annotations %+v, description %q", tl.Name, tl.Annotations, tl.Description)
			}
		}
		slices.Sort(out)
		return out
	}
	all := names(`"tools"`)
	for _, want := range []string{"checkin", "delete_meal", "find_foods", "get_day", "get_food", "get_history", "get_profile", "get_summary", "log_meal", "update_profile_notes", "upsert_food"} {
		if !slices.Contains(all, want) {
			t.Errorf("the tools %v lack %s", all, want)
		}
	}
	if read := names(`"read"`); !slices.Equal(read, []string{"find_foods", "get_day", "get_food", "get_history", "get_profile", "get_summary"}) {
		t.Errorf("a read token's tools = %v", read)
	}
}

// A tool's input schema asks for what its operation needs: its required
// parameters, but a day, which is today when left out, and its body's
// required properties. A tool that may be sent again says so.
func TestMCPToolSchemas(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	res, err := mcpSession(t, in, mcpToken(t, in, session, `"tools"`)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range res.Tools {
		byName[tl.Name] = tl
	}
	for name, want := range map[string][]string{
		"get_day": nil, "get_summary": nil, "checkin": nil, "log_metric": {"kind", "value"}, "get_food": {"id"},
		"log_meal": {"items"}, "find_foods": {"query"},
	} {
		schema, _ := byName[name].InputSchema.(map[string]any)
		var got []string
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				got = append(got, fmt.Sprint(r))
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s requires %v, want %v", name, got, want)
		}
	}
	// A tool is safe to send again but log_meal and undo_last, whose meal
	// or undo sent without an id is another.
	for name, want := range map[string]bool{"log_metric": true, "delete_meal": true, "log_meal": false, "undo_last": false, "update_meal": true, "upsert_food": true, "get_day": true} {
		if got := byName[name].Annotations.IdempotentHint; got != want {
			t.Errorf("%s idempotent = %v, want %v", name, got, want)
		}
	}
	if d := byName["log_meal"].Description; !strings.Contains(d, "without an id") {
		t.Errorf("log_meal's description does not say a meal may come without an id: %s", d)
	}
	// What a model would get wrong without it: a replace with what an
	// item has changes nothing; nothing is redone.
	for name, want := range map[string]string{"update_meal": "remove the item and add it", "undo_last": "Nothing is redone"} {
		if d := byName[name].Description; !strings.Contains(d, want) {
			t.Errorf("%s's description does not say %q: %s", name, want, d)
		}
	}
}

// TestMCPDescriptionsAreBrief holds a tool's description in MCP to its
// brief, and the operation's in the OpenAPI document to the whole of it.
func TestMCPDescriptionsAreBrief(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	res, err := mcpSession(t, in, mcpToken(t, in, session, `"tools"`)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := os.ReadFile("../../api/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	// A tool's title is its operation's summary.
	bySummary := map[string]string{}
	for _, ops := range doc.Paths {
		for _, op := range ops {
			bySummary[op.Summary] = op.Description
		}
	}
	full := map[string]string{}
	for _, tl := range res.Tools {
		full[tl.Name] = bySummary[tl.Title]
		if f := full[tl.Name]; tl.Description == "" || len(tl.Description) >= len(f) {
			t.Errorf("%s: %d bytes in MCP, %d in the document; want a brief", tl.Name, len(tl.Description), len(f))
		}
	}
	if !strings.Contains(full["update_meal"], "A replace with what the item has already changes nothing") {
		t.Errorf("update_meal's operation lost its whole description: %s", full["update_meal"])
	}
}

// TestMCPSchemasCarryWhatAModelUses holds a tool's schema to what a model
// needs to call it, as it is sent with each request: no format of a
// number, no examples, no bounds of a number, a string or a list, which
// the operation checks; the enums, the required and the formats of a
// string stay.
func TestMCPSchemasCarryWhatAModelUses(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	res, err := mcpSession(t, in, mcpToken(t, in, session, `"tools"`)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	dropped := []string{"examples", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "minLength", "maxLength", "minItems", "maxItems"}
	var walk func(tool string, v any, names bool)
	walk = func(tool string, v any, names bool) {
		switch v := v.(type) {
		case map[string]any:
			for k, e := range v {
				if !names && slices.Contains(dropped, k) {
					t.Errorf("%s: a schema has %s", tool, k)
				}
				if f, _ := e.(string); !names && k == "format" && !slices.Contains([]string{"uuid", "date", "date-time"}, f) {
					t.Errorf("%s: a schema has the format %s", tool, f)
				}
				walk(tool, e, !names && k == "properties")
			}
		case []any:
			for _, e := range v {
				walk(tool, e, false)
			}
		}
	}
	enums := 0
	for _, tl := range res.Tools {
		walk(tl.Name, tl.InputSchema, false)
		b, err := json.Marshal(tl.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		enums += strings.Count(string(b), `"enum"`)
	}
	if enums == 0 {
		t.Error("no schema has an enum")
	}
}

// TestMCPFitsItsBudget holds the tools and the instructions, which a
// model is sent with each request, to a budget in bytes: ~7k tokens, as
// the eval measures them (DESIGN.md §6). A change that grows them past it
// needs a reason, and the eval run again. 13,500 bytes until October
// 2026, raised for the rules of a meal's parts, then for the coach's
// history and notes, then for an item's quantities and a search again.
func TestMCPFitsItsBudget(t *testing.T) {
	t.Parallel()

	const budget = 16500
	in, session := signedIn(t)
	cs := mcpSession(t, in, mcpToken(t, in, session, `"tools"`))
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	size := len(cs.InitializeResult().Instructions)
	for _, tl := range res.Tools {
		b, err := json.Marshal(tl.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		size += len(tl.Name) + len(tl.Description) + len(b)
	}
	if size > budget {
		t.Errorf("the tools and the instructions are %d bytes, want at most %d", size, budget)
	}
}

// TestMCPSchemasSayOnce holds a schema two tools take to the tool it is
// of: log_meal's new food is upsert_food's, update_meal's item
// log_meal's, each said in a word where the other tool takes it.
func TestMCPSchemasSayOnce(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	res, err := mcpSession(t, in, mcpToken(t, in, session, `"tools"`)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]map[string]any{}
	for _, tl := range res.Tools {
		byName[tl.Name], _ = tl.InputSchema.(map[string]any)
	}
	at := func(v any, path ...string) map[string]any {
		for _, p := range path {
			m, _ := v.(map[string]any)
			v = m[p]
		}
		m, _ := v.(map[string]any)
		return m
	}
	item := at(byName["log_meal"], "properties", "items", "items")
	if at(item, "properties", "food_id") == nil {
		t.Errorf("log_meal's item is not in full: %v", item)
	}
	for path, want := range map[string]string{"food": "is that food", "nutrients": "whole plate"} {
		if d, _ := at(item, "properties", path)["description"].(string); !strings.Contains(d, want) {
			t.Errorf("log_meal's %s says %q, want its field's words, %q", path, d, want)
		}
	}
	for _, tc := range []struct {
		name, of string
		schema   map[string]any
	}{
		{"log_meal's new food", "upsert_food", at(item, "properties", "food")},
		{"update_meal's item", "log_meal", at(byName["update_meal"], "properties", "ops", "items", "properties", "item")},
	} {
		d, _ := tc.schema["description"].(string)
		if tc.schema["properties"] != nil || tc.schema["type"] != "object" || !strings.Contains(d, tc.of) {
			t.Errorf("%s = %v, want an object said to be %s's", tc.name, tc.schema, tc.of)
		}
	}
}

// A tool called through MCP is its operation: its answer, as JSON, and a
// write recorded as the client mcp's. A day left out is today.
func TestMCPCallsATool(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cs := mcpSession(t, in, mcpToken(t, in, session, `"tools"`))
	id := meal.NewIDAt(in.clock.Now()).String()
	text, isErr := callTool(t, cs, "log_meal", map[string]any{
		"id": id, "items": []any{map[string]any{"name": "Soup", "nutrients": map[string]any{"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}},
	})
	if isErr || !strings.Contains(text, `"client":"mcp"`) {
		t.Fatalf("log_meal: %v %s", isErr, text)
	}
	es, err := in.db.Events(t.Context(), in.user.ID, 1)
	if err != nil || len(es) != 1 || es[0].Actor.Client != "mcp" {
		t.Errorf("the event of the meal = %+v, %v; want the client mcp's", es, err)
	}
	text, isErr = callTool(t, cs, "get_day", nil)
	if isErr || !strings.Contains(text, `"date":"`+day.DateOf(in.clock.Now()).String()+`"`) || !strings.Contains(text, id) {
		t.Errorf("get_day of no day: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, cs, "find_foods", map[string]any{"query": []any{"soup", "bread"}, "limit": 5}); isErr {
		t.Errorf("find_foods of two queries and a number: %s", text)
	}
	// An argument of null, or a parameter of "", is one left out, as a
	// model may send it.
	for _, date := range []any{nil, ""} {
		if text, isErr := callTool(t, cs, "get_day", map[string]any{"date": date}); isErr || !strings.Contains(text, id) {
			t.Errorf("get_day of the date %#v: %v %s", date, isErr, text)
		}
	}
	if text, isErr := callTool(t, cs, "find_foods", map[string]any{"query": "soup", "limit": nil}); isErr {
		t.Errorf("find_foods of a null limit: %s", text)
	}
}

// TestMCPListInAString takes a list a model sends as a string of JSON as
// the list, not as one query of its brackets and words.
func TestMCPListInAString(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	saveIsolationFoods(t, in.db, in.user, in.clock.Now())
	cs := mcpSession(t, in, mcpToken(t, in, session, `"tools"`))
	if text, isErr := callTool(t, cs, "find_foods", map[string]any{"query": `["rye", "nothing"]`}); isErr || !strings.Contains(text, `"queries":["rye"]`) {
		t.Errorf("find_foods of a list in a string: %v %s", isErr, text)
	}
}

// What the operation refuses, and arguments it does not take, are errors
// of the tool, which say why.
func TestMCPToolErrors(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cs := mcpSession(t, in, mcpToken(t, in, session, `"tools"`))
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"log_meal", map[string]any{"id": "not-an-id", "items": []any{}}, "id"},
		{"get_food", map[string]any{"id": "0199f0a0-0000-7000-8000-000000000001", "color": "red"}, `no argument "color"`},
		{"get_food", nil, "get_food needs id"},
		{"get_food", map[string]any{"id": ""}, "get_food needs id"},
		{"delete_metric", map[string]any{"date": "today"}, "delete_metric needs kind"},
		{"get_food", map[string]any{"id": []any{1}}, "id: want a string"},
		{"get_day", map[string]any{"date": "yesterday"}, "date"},
		// A list for a parameter of one value, and a list of what no
		// query holds.
		{"find_foods", map[string]any{"query": "rye", "limit": []any{1, 2}}, "limit: want a string"},
		{"find_foods", map[string]any{"query": []any{nil}}, "query: want a string"},
		{"find_foods", map[string]any{"query": []any{[]any{"rye"}}}, "query: want a string"},
	} {
		if text, isErr := callTool(t, cs, tc.name, tc.args); !isErr || !strings.Contains(text, tc.want) {
			t.Errorf("%s %v: %v %s, want an error saying %q", tc.name, tc.args, isErr, text, tc.want)
		}
	}
}

// MCP takes an API token that opens a tool: none, a session or a bad
// token is a 401, a token of the ingest alone a 403; a tool a token does
// not open is not there.
func TestMCPAuth(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	mcpHeaders := map[string]string{"Accept": "application/json, text/event-stream", "Content-Type": "application/json"}
	for name, tc := range map[string]struct {
		cookies []*http.Cookie
		token   string
		want    int
	}{
		"no token":         {nil, "", http.StatusUnauthorized},
		"a session":        {[]*http.Cookie{session}, "", http.StatusUnauthorized},
		"a bad token":      {nil, "tdm_nothing", http.StatusUnauthorized},
		"an ingest token":  {nil, mcpToken(t, in, session, `"ingest"`), http.StatusForbidden},
		"a token of tools": {nil, mcpToken(t, in, session, `"tools"`), http.StatusOK},
	} {
		h := maps.Clone(mcpHeaders)
		if tc.token != "" {
			h["Authorization"] = "Bearer " + tc.token
		}
		rec := in.do(t, http.MethodPost, "/mcp", initialize, tc.cookies, h)
		if rec.Code != tc.want {
			t.Errorf("%s: %d %s, want %d", name, rec.Code, rec.Body.String(), tc.want)
		}
		if tc.want == http.StatusUnauthorized && !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Bearer") && name != "a bad token" {
			t.Errorf("%s: no WWW-Authenticate", name)
		}
	}
	// A stateless server has no stream to open, nor session to end; the
	// web app does not take them.
	tools := bearer(mcpToken(t, in, session, `"tools"`))
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		if rec := in.do(t, method, "/mcp", "", nil, tools); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /mcp: %d %s, want 405", method, rec.Code, rec.Body.String())
		}
	}
	cs := mcpSession(t, in, mcpToken(t, in, session, `"read"`))
	if _, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "log_meal", Arguments: map[string]any{}}); err == nil {
		t.Error("a read token called log_meal")
	}
}

// Behind a reverse proxy on the same host, which keeps the public Host,
// MCP answers as the API does.
func TestMCPBehindALocalProxy(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	srv := httptest.NewServer(in.h)
	t.Cleanup(srv.Close)
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/mcp", strings.NewReader(initialize))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "tdeemon.example.com"
	req.Header.Set("Authorization", "Bearer "+mcpToken(t, in, session, `"tools"`))
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Errorf("initialize through a local proxy: %d", res.StatusCode)
	}
}

// The server gives every client its policy of logging, short, and naming
// only tools it serves and the properties of their requests and answers:
// a renamed tool, argument or field fails here rather than leaving the
// instructions wrong.
func TestMCPInstructions(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cs := mcpSession(t, in, mcpToken(t, in, session, `"tools"`))
	got := cs.InitializeResult().Instructions
	if !strings.Contains(got, "`log_meal`") || !strings.Contains(got, "never work out") {
		t.Fatalf("instructions = %q", got)
	}
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// The prefix a client reads before its first call, which every call
	// of a log sends again (cached, as a client caches a prompt): about
	// four bytes a token.
	list, err := json.Marshal(res.Tools)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got) + len(list); n > 28000 {
		t.Errorf("the instructions and the tools are %d bytes, want at most 28000", n)
	}
	known := map[string]bool{}
	for _, tl := range res.Tools {
		known[tl.Name] = true
		propertyNames(tl.InputSchema, known)
	}
	// The fields of the tools' answers, which the instructions name too.
	toolAnswerNames(t, known)
	// Every name of the kind tools and fields have, every word in
	// backticks, and the fields of a JSON fragment in them.
	for _, m := range regexp.MustCompile("[a-z]+_[a-z_]+|`([^`]+)`").FindAllStringSubmatch(got, -1) {
		names := []string{m[0]}
		var fragment map[string]any
		switch {
		case m[1] != "" && json.Unmarshal([]byte("{"+m[1]+"}"), &fragment) == nil:
			names = slices.Collect(maps.Keys(fragment))
		case m[1] != "":
			names = []string{m[1]}
		}
		for _, name := range names {
			if !known[name] {
				t.Errorf("the instructions name %q, which is no tool nor field of one", name)
			}
		}
	}
}

// toolAnswerNames adds to names the name of every property of the answers
// of the operations that are tools, following their $refs.
func toolAnswerNames(t *testing.T, names map[string]bool) {
	t.Helper()
	spec, err := api.Spec()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Tags      []string       `json:"tags"`
			Responses map[string]any `json:"responses"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				name := strings.TrimPrefix(ref, "#/components/schemas/")
				if !seen[name] {
					seen[name] = true
					walk(doc.Components.Schemas[name])
				}
				return
			}
			if props, ok := v["properties"].(map[string]any); ok {
				for name := range props {
					names[name] = true
				}
			}
			for _, e := range v {
				walk(e)
			}
		case []any:
			for _, e := range v {
				walk(e)
			}
		}
	}
	for _, methods := range doc.Paths {
		for _, op := range methods {
			if slices.Contains(op.Tags, "tools") {
				walk(op.Responses["200"])
				walk(op.Responses["201"])
			}
		}
	}
}

// propertyNames adds the name of every property of schema, at any depth,
// to names.
func propertyNames(schema any, names map[string]bool) {
	switch v := schema.(type) {
	case map[string]any:
		if props, ok := v["properties"].(map[string]any); ok {
			for name := range props {
				names[name] = true
			}
		}
		for _, e := range v {
			propertyNames(e, names)
		}
	case []any:
		for _, e := range v {
			propertyNames(e, names)
		}
	}
}

// TestMCPLogsSessionsAtDebug holds the SDK's word of each stateless
// request's session, three lines a call, to the debug level.
func TestMCPLogsSessionsAtDebug(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cs := mcpSession(t, in, mcpToken(t, in, session, `"tools"`))
	if text, isErr := callTool(t, cs, "get_profile", nil); isErr {
		t.Fatal(text)
	}
	found := 0
	sc := bufio.NewScanner(strings.NewReader(in.logs.String()))
	for sc.Scan() {
		var rec struct{ Level, Msg string }
		if json.Unmarshal(sc.Bytes(), &rec) != nil || !strings.HasPrefix(rec.Msg, "server ") {
			continue
		}
		found++
		if rec.Level != "DEBUG" {
			t.Errorf("%s at %s, want DEBUG", rec.Msg, rec.Level)
		}
	}
	if found == 0 {
		t.Errorf("no word of the SDK's in the log:\n%s", in.logs.String())
	}
}
