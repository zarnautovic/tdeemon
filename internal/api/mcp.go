package api

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/zarnautovic/tdeemon/docs"
	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/coaching"
)

// mcpInstructions is the policy of logging the server gives every MCP
// client (DESIGN.md §6): the same for each, rather than a skill of each
// client's.
//
//go:embed mcp_instructions.md
var mcpInstructions string

// MCPPath is where MCP clients reach the tools (DESIGN.md §6): Streamable
// HTTP, with an API token.
const MCPPath = "/mcp"

// mcpTool is a tool as MCP serves it: its definition, from its
// operation's part of the OpenAPI document, and how its arguments make a
// request of the operation.
type mcpTool struct {
	tool tool
	def  *mcp.Tool
	// params are the operation's parameters, by name.
	params map[string]param
	// body says the operation takes a JSON body, of the arguments that
	// are not parameters.
	body bool
}

// param is where a parameter goes, "path" or "query", and whether it
// takes a list, as a query parameter repeated.
type param struct {
	in   string
	list bool
	// text says it takes a string: a number sent for it is refused, as
	// one would lose its leading zeros (a barcode's).
	text bool
}

// mcpServer serves the tools of the registry over MCP. A call runs the
// tool's operation in process, as a request with the caller's principal,
// so MCP and REST share the validation, the token gate and the answers.
type mcpServer struct {
	tools  []mcpTool
	routes http.Handler
	info   ServerInfo
	logger *slog.Logger
	// sdkLogger is the SDK's: its info at debug, as a stateless server
	// opens and closes a session for each request.
	sdkLogger *slog.Logger
}

// newMCPServer is the MCP server of tools, described by spec, the OpenAPI
// document, whose calls routes serves.
func newMCPServer(tools []tool, spec []byte, routes http.Handler, info ServerInfo, logger *slog.Logger) (*mcpServer, error) {
	var doc openAPIDoc
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("mcp: the OpenAPI document: %w", err)
	}
	m := &mcpServer{routes: routes, info: info, logger: logger}
	if logger != nil {
		m.sdkLogger = slog.New(infoAtDebug{logger.Handler()})
	}
	for i := range tools {
		mt, err := doc.mcpTool(tools[i])
		if err != nil {
			return nil, fmt.Errorf("mcp: tool %s: %w", tools[i].name, err)
		}
		m.tools = append(m.tools, mt)
	}
	return m, nil
}

// infoAtDebug logs its handler's info at debug.
type infoAtDebug struct{ slog.Handler }

func (h infoAtDebug) Enabled(ctx context.Context, l slog.Level) bool {
	return h.Handler.Enabled(ctx, debugOfInfo(l))
}

func (h infoAtDebug) Handle(ctx context.Context, r slog.Record) error {
	r.Level = debugOfInfo(r.Level)
	return h.Handler.Handle(ctx, r)
}

func (h infoAtDebug) WithAttrs(as []slog.Attr) slog.Handler {
	return infoAtDebug{h.Handler.WithAttrs(as)}
}

func (h infoAtDebug) WithGroup(name string) slog.Handler {
	return infoAtDebug{h.Handler.WithGroup(name)}
}

func debugOfInfo(l slog.Level) slog.Level {
	if l == slog.LevelInfo {
		return slog.LevelDebug
	}
	return l
}

// handler answers MCP requests made with an API token that opens a tool.
// The session's cookie is no way in: a browser would send it to any
// site's request.
func (m *mcpServer) handler() http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		p, _ := auth.PrincipalFrom(r.Context())
		return m.serverFor(p)
	}, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, Logger: m.sdkLogger,
		// The SDK refuses a request to a loopback address whose Host is
		// not localhost, against DNS rebinding: that is every request
		// through a reverse proxy on the same host. A page that rebinds
		// a name cannot send the user's token, the only way in.
		DisableLocalhostProtection: true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok || p.Via != auth.ViaToken {
			w.Header().Set("WWW-Authenticate", `Bearer realm="tdeemon"`)
			writeProblem(w, http.StatusUnauthorized, "MCP takes an API token, as a Bearer token in the Authorization header")
			return
		}
		if !slices.ContainsFunc(m.tools, func(t mcpTool) bool { return opens(p, t.tool) }) {
			writeProblem(w, http.StatusForbidden, "the token opens no tool: MCP takes a token with the read or the tools scope")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// opens reports whether p's token may call t.
func opens(p auth.Principal, t tool) bool {
	return slices.ContainsFunc(t.scopes, p.Can)
}

// serverFor is the MCP server of the tools p's token opens.
func (m *mcpServer) serverFor(p auth.Principal) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: m.info.Name, Version: m.info.Version},
		&mcp.ServerOptions{Logger: m.sdkLogger, Instructions: mcpInstructions})
	p.Client = auth.ClientMCP
	for i := range m.tools {
		t := &m.tools[i]
		if opens(p, t.tool) {
			s.AddTool(t.def, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return m.call(ctx, p, t, req.Params.Arguments), nil
			})
		}
	}
	addCoaching(s)
	return s
}

// methodologyURI is the methodology's address as an MCP resource.
const methodologyURI = "tdeemon://methodology"

// addCoaching adds the coaching sessions to s as its prompts, which a
// client shows as commands, and the methodology as its resource
// (DESIGN.md §8). They are the text get_coaching_guide answers, for the
// clients that show prompts; every token MCP takes opens that tool.
func addCoaching(s *mcp.Server) {
	for _, session := range coaching.Sessions() {
		s.AddPrompt(&mcp.Prompt{
			Name: session.Name, Title: session.Title, Description: session.Description,
			Arguments: []*mcp.PromptArgument{{
				Name: "about", Title: "About", Description: "What the user wants from the session, in their words.",
			}},
		}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return sessionPrompt(session, req.Params.Arguments["about"]), nil
		})
	}
	s.AddResource(&mcp.Resource{
		URI: methodologyURI, Name: "methodology", Title: "How tdeemon reads the numbers", MIMEType: "text/markdown",
		Description: "How the trend weight, the measured TDEE, the verdict on a phase and the autopilot come about.",
		Size:        int64(len(docs.Methodology)),
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: methodologyURI, MIMEType: "text/markdown", Text: docs.Methodology}}}, nil
	})
}

// sessionPrompt is the user's message that starts session, with what
// the user wants from it, when they said.
func sessionPrompt(session coaching.Session, about string) *mcp.GetPromptResult {
	var text strings.Builder
	text.WriteString("Run tdeemon's " + session.Title + " with me: follow these steps.\n\n")
	text.WriteString(session.Guide())
	if about = strings.TrimSpace(about); about != "" {
		text.WriteString("\nWhat I want from it: " + about + "\n")
	}
	return &mcp.GetPromptResult{
		Description: session.Description,
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text.String()}}},
	}
}

// call runs t's operation for p with args, and answers what it answered:
// its body, or the problem as an error of the tool, which the model reads.
func (m *mcpServer) call(ctx context.Context, p auth.Principal, t *mcpTool, args json.RawMessage) (res *mcp.CallToolResult) {
	// The SDK runs a call on a goroutine of its own, where net/http
	// recovers no panic: one would take the server down.
	defer func() {
		if v := recover(); v != nil {
			m.logger.ErrorContext(ctx, "a tool panicked", "tool", t.tool.name, "panic", v, "stack", string(debug.Stack()))
			res = toolError("the tool failed")
		}
	}()
	r, err := t.request(auth.WithPrincipal(ctx, p), args)
	if err != nil {
		return toolError(err.Error())
	}
	w := &recorder{header: http.Header{}, status: http.StatusOK}
	m.routes.ServeHTTP(w, r)
	body := w.body.Bytes()
	if w.status < 200 || w.status > 299 {
		var problem struct {
			Detail string `json:"detail"`
			Errors []struct {
				Message  string          `json:"message"`
				Location string          `json:"location"`
				Value    json.RawMessage `json:"value"`
			} `json:"errors"`
		}
		if json.Unmarshal(body, &problem) != nil || problem.Detail == "" {
			return toolError(fmt.Sprintf("%d %s", w.status, http.StatusText(w.status)))
		}
		var msg strings.Builder
		msg.WriteString(problem.Detail)
		for _, e := range problem.Errors {
			msg.WriteString("; " + strings.TrimPrefix(e.Location+": ", ": ") + e.Message)
			// A value of the server's own that is data, as a violation of
			// the limits, goes with it as it is, for the model to read.
			if bytes.HasPrefix(e.Value, []byte("{")) {
				msg.WriteString(" " + string(e.Value))
			}
		}
		return toolError(msg.String())
	}
	var structured map[string]any
	if err := json.Unmarshal(body, &structured); err != nil {
		return toolError("the answer did not read")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}, StructuredContent: structured}
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// request is the request of t's operation with args: its parameters in
// the path and the query, the other arguments its JSON body.
func (t *mcpTool) request(ctx context.Context, args json.RawMessage) (*http.Request, error) {
	in := map[string]json.RawMessage{}
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, errors.New("the arguments are not an object")
		}
	}
	path, query, body, err := t.place(in)
	if err != nil {
		return nil, err
	}
	// A day left out is today (DESIGN.md §5).
	path = strings.Replace(path, "{date}", "today", 1)
	if missing := pathParam.FindAllStringSubmatch(path, -1); len(missing) > 0 {
		names := make([]string, len(missing))
		for i, m := range missing {
			names[i] = m[1]
		}
		return nil, fmt.Errorf("%s needs %s", t.tool.name, strings.Join(names, ", "))
	}
	target := path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var b []byte
	if t.body {
		if b, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	r, err := http.NewRequestWithContext(ctx, t.tool.op.Method, target, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if t.body {
		r.Header.Set("Content-Type", "application/json")
	}
	return r, nil
}

// place puts each argument of in where t's operation takes it: in its
// path, its query or its body. An argument of null is one left out, as a
// model may send it.
func (t *mcpTool) place(in map[string]json.RawMessage) (string, url.Values, map[string]json.RawMessage, error) {
	path, query := t.tool.op.Path, url.Values{}
	body := map[string]json.RawMessage{}
	for _, name := range slices.Sorted(maps.Keys(in)) {
		raw := in[name]
		if string(raw) == "null" {
			continue
		}
		prm, isParam := t.params[name]
		if !isParam {
			if !t.body || slices.Contains(t.tool.webOnly, name) {
				return "", nil, nil, fmt.Errorf("no argument %q: %s takes %s", name, t.tool.name, strings.Join(slices.Sorted(maps.Keys(t.params)), ", "))
			}
			body[name] = raw
			continue
		}
		// A list goes as the parameter repeated; a model may send it as
		// a string of JSON.
		var many []json.RawMessage
		if prm.list && (json.Unmarshal(raw, &many) == nil || listInString(raw, &many)) {
			for _, r := range many {
				v, err := paramValue(r)
				if err != nil {
					return "", nil, nil, fmt.Errorf("%s: %w", name, err)
				}
				query.Add(name, v)
			}
			continue
		}
		v, err := prm.value(raw)
		if err != nil {
			return "", nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		if prm.in == "path" {
			// An empty one is left out too: no path takes it.
			if v == "" {
				continue
			}
			path = strings.Replace(path, "{"+name+"}", url.PathEscape(v), 1)
		} else {
			query.Set(name, v)
		}
	}
	return path, query, body, nil
}

// pathParam is a parameter of an operation's path, not filled in.
var pathParam = regexp.MustCompile(`\{([a-z_]+)\}`)

// paramValue is the text of a parameter's value: a string as it is, a
// number or a boolean as JSON writes it.
func paramValue(raw json.RawMessage) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	switch v := v.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	}
	return "", errors.New("want a string, a number or a boolean")
}

// value is the text of prm's value raw, as paramValue makes it; a number
// for a string is refused.
func (prm param) value(raw json.RawMessage) (string, error) {
	if t := bytes.TrimSpace(raw); prm.text && len(t) > 0 && (t[0] == '-' || t[0] >= '0' && t[0] <= '9') {
		return "", errors.New("want a string, its digits quoted")
	}
	return paramValue(raw)
}

// listInString reports whether raw is a string of a JSON list, and puts
// the list in many.
func listInString(raw json.RawMessage, many *[]json.RawMessage) bool {
	var s string
	if json.Unmarshal(raw, &s) != nil || !strings.HasPrefix(strings.TrimSpace(s), "[") {
		return false
	}
	return json.Unmarshal([]byte(s), many) == nil
}

// recorder keeps what a handler answers a call of a tool.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *recorder) Header() http.Header         { return w.header }
func (w *recorder) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *recorder) WriteHeader(status int)      { w.status = status }

// openAPIDoc is the part of the OpenAPI document a tool's definition
// comes from.
type openAPIDoc struct {
	Paths      map[string]map[string]openAPIOp `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
	// tool is the tool whose schema is being made.
	tool string
}

// saidOnce are schemas two tools take, said in full in their own tool's
// alone: a model has each tool's schema with each request, so another
// that takes it names it in a word. The operation checks it in full.
var saidOnce = map[string]struct{ of, description string }{
	"FoodFields": {"upsert_food", "The fields upsert_food takes, but its id and archived; no unit: the item has it."},
	"ItemInput":  {"log_meal", "An item, as log_meal's items take it."},
}

type openAPIOp struct {
	Parameters []struct {
		Name        string          `json:"name"`
		In          string          `json:"in"`
		Required    bool            `json:"required"`
		Description string          `json:"description"`
		Schema      json.RawMessage `json:"schema"`
	} `json:"parameters"`
	RequestBody *struct {
		Content map[string]struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
}

// mcpTool is t as MCP serves it: its input schema the operation's
// parameters and the properties of its body, in one object, with every
// $ref of the document replaced by what it names, so the schema stands
// alone.
func (d openAPIDoc) mcpTool(t tool) (mcpTool, error) {
	op, ok := d.Paths[t.op.Path][strings.ToLower(t.op.Method)]
	if !ok {
		return mcpTool{}, fmt.Errorf("no %s %s in the document", t.op.Method, t.op.Path)
	}
	d.tool = t.name
	if t.brief == "" {
		return mcpTool{}, errors.New("no brief description")
	}
	mt := mcpTool{tool: t, params: map[string]param{}}
	props := map[string]any{}
	required, err := d.params(op, &mt, props)
	if err != nil {
		return mcpTool{}, err
	}
	if op.RequestBody != nil {
		req, err := d.body(op, props, t.webOnly)
		if err != nil {
			return mcpTool{}, err
		}
		required = append(required, req...)
		mt.body = true
	}
	slices.Sort(required)
	schema := map[string]any{"type": "object", "properties": forModel(props, true), "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	// A tool that reads changes nothing. A tool is safe to send again: a
	// create is under the client's id, any other write replaces what it
	// names; but log_meal, whose meal sent without an id is another
	// (tool.mayWriteAgain). One that replaces or deletes what is there
	// says so (tool.destroys).
	readOnly := t.op.Method == http.MethodGet
	mt.def = &mcp.Tool{
		Name: t.name, Title: t.op.Summary, Description: t.brief, InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			Title: t.op.Summary, ReadOnlyHint: readOnly, DestructiveHint: new(t.destructive),
			IdempotentHint: !t.writesAgain, OpenWorldHint: new(t.opensOut),
		},
	}
	return mt, nil
}

// A tool's schema goes with each request a model makes, so it carries
// what the model needs to call the tool: not the format of a number, nor
// examples, nor the bounds of a number, a string or a list, which the
// operation checks and answers in its error.
var (
	droppedKeywords = []string{"examples", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "minLength", "maxLength", "minItems", "maxItems"}
	keptFormats     = []string{"uuid", "date", "date-time"}
)

// forModel returns schema v without what a model does not need; names
// says v is a map of properties by name, whose keys are no keywords.
func forModel(v any, names bool) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			if f, isString := e.(string); !names && (slices.Contains(droppedKeywords, k) || k == "format" && (!isString || !slices.Contains(keptFormats, f))) {
				continue
			}
			out[k] = forModel(e, !names && k == "properties")
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = forModel(e, false)
		}
		return out
	}
	return v
}

// params adds op's path and query parameters to props and to mt, and
// returns those required.
func (d openAPIDoc) params(op openAPIOp, mt *mcpTool, props map[string]any) ([]string, error) {
	var required []string
	for _, prm := range op.Parameters {
		if prm.In != "path" && prm.In != "query" {
			continue
		}
		s, err := d.inline(prm.Schema, 0)
		if err != nil {
			return nil, err
		}
		typ, _ := s.(map[string]any)["type"].(string)
		mt.params[prm.Name] = param{in: prm.In, list: prm.In == "query" && typ == "array", text: typ == "string"}
		props[prm.Name] = s
		// The day of a tool is today when left out.
		if prm.Required && prm.Name != "date" {
			required = append(required, prm.Name)
		}
	}
	return required, nil
}

// body adds the properties of op's JSON body to props, but for those
// webOnly names, and returns those required.
func (d openAPIDoc) body(op openAPIOp, props map[string]any, webOnly []string) ([]string, error) {
	raw, ok := op.RequestBody.Content["application/json"]
	if !ok {
		return nil, errors.New("its body is not JSON")
	}
	s, err := d.inline(raw.Schema, 0)
	if err != nil {
		return nil, err
	}
	body, _ := s.(map[string]any)
	bodyProps, _ := body["properties"].(map[string]any)
	for name, p := range bodyProps {
		if slices.Contains(webOnly, name) {
			continue
		}
		if _, taken := props[name]; taken {
			return nil, fmt.Errorf("%q is both a parameter and a property of the body", name)
		}
		props[name] = p
	}
	var required []string
	if req, ok := body["required"].([]any); ok {
		for _, r := range req {
			required = append(required, fmt.Sprint(r))
		}
	}
	return required, nil
}

// maxRefDepth bounds how deep inline follows $refs: the document has no
// schema that holds itself.
const maxRefDepth = 20

// inline is the schema raw with every $ref replaced by the schema it
// names.
func (d openAPIDoc) inline(raw json.RawMessage, depth int) (any, error) {
	if depth > maxRefDepth {
		return nil, errors.New("a $ref too deep: a schema that holds itself?")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return d.inlineValue(v, depth)
}

func (d openAPIDoc) inlineValue(v any, depth int) (any, error) {
	switch v := v.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			name, found := strings.CutPrefix(ref, "#/components/schemas/")
			s, known := d.Components.Schemas[name]
			if !found || !known {
				return nil, fmt.Errorf("a $ref to nothing: %s", ref)
			}
			// A field's words stand by its $ref: they say what the field is
			// where the schema says what it holds.
			words, _ := v["description"].(string)
			if once, ok := saidOnce[name]; ok && once.of != d.tool {
				return map[string]any{"type": "object", "description": strings.TrimSpace(words + " " + once.description)}, nil
			}
			in, err := d.inline(s, depth+1)
			if m, isMap := in.(map[string]any); isMap && words != "" {
				m["description"] = words
			}
			return in, err
		}
		out := make(map[string]any, len(v))
		for k, e := range v {
			var err error
			if out[k], err = d.inlineValue(e, depth); err != nil {
				return nil, err
			}
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			var err error
			if out[i], err = d.inlineValue(e, depth); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return v, nil
}
