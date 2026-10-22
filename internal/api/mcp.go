package api

import (
	"bytes"
	"context"
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

	"github.com/zarnautovic/tdeemon/internal/auth"
)

// MCPPath is where MCP clients reach the tools (DESIGN.md §6): Streamable
// HTTP, with an API token.
const MCPPath = "/mcp"

// mcpTool is a tool as MCP serves it: its definition, from its
// operation's part of the OpenAPI document, and how its arguments make a
// request of the operation.
type mcpTool struct {
	tool tool
	def  *mcp.Tool
	// params are the operation's parameters, by name: "path" or "query".
	params map[string]string
	// body says the operation takes a JSON body, of the arguments that
	// are not parameters.
	body bool
}

// mcpServer serves the tools of the registry over MCP. A call runs the
// tool's operation in process, as a request with the caller's principal,
// so MCP and REST share the validation, the token gate and the answers.
type mcpServer struct {
	tools  []mcpTool
	routes http.Handler
	info   ServerInfo
	logger *slog.Logger
}

// newMCPServer is the MCP server of tools, described by spec, the OpenAPI
// document, whose calls routes serves.
func newMCPServer(tools []tool, spec []byte, routes http.Handler, info ServerInfo, logger *slog.Logger) (*mcpServer, error) {
	var doc openAPIDoc
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("mcp: the OpenAPI document: %w", err)
	}
	m := &mcpServer{routes: routes, info: info, logger: logger}
	for i := range tools {
		mt, err := doc.mcpTool(tools[i])
		if err != nil {
			return nil, fmt.Errorf("mcp: tool %s: %w", tools[i].name, err)
		}
		m.tools = append(m.tools, mt)
	}
	return m, nil
}

// handler answers MCP requests made with an API token that opens a tool.
// The session's cookie is no way in: a browser would send it to any
// site's request.
func (m *mcpServer) handler() http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		p, _ := auth.PrincipalFrom(r.Context())
		return m.serverFor(p)
	}, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, Logger: m.logger,
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
	s := mcp.NewServer(&mcp.Implementation{Name: m.info.Name, Version: m.info.Version}, &mcp.ServerOptions{Logger: m.logger})
	p.Client = auth.ClientMCP
	for i := range m.tools {
		t := &m.tools[i]
		if opens(p, t.tool) {
			s.AddTool(t.def, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return m.call(ctx, p, t, req.Params.Arguments), nil
			})
		}
	}
	return s
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
				Message  string `json:"message"`
				Location string `json:"location"`
			} `json:"errors"`
		}
		if json.Unmarshal(body, &problem) != nil || problem.Detail == "" {
			return toolError(fmt.Sprintf("%d %s", w.status, http.StatusText(w.status)))
		}
		var msg strings.Builder
		msg.WriteString(problem.Detail)
		for _, e := range problem.Errors {
			msg.WriteString("; " + strings.TrimPrefix(e.Location+": ", ": ") + e.Message)
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
		where, isParam := t.params[name]
		if !isParam {
			if !t.body {
				return "", nil, nil, fmt.Errorf("no argument %q: %s takes %s", name, t.tool.name, strings.Join(slices.Sorted(maps.Keys(t.params)), ", "))
			}
			body[name] = raw
			continue
		}
		v, err := paramValue(raw)
		if err != nil {
			return "", nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		if where == "path" {
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
	mt := mcpTool{tool: t, params: map[string]string{}}
	props := map[string]any{}
	required, err := d.params(op, &mt, props)
	if err != nil {
		return mcpTool{}, err
	}
	if op.RequestBody != nil {
		req, err := d.body(op, props)
		if err != nil {
			return mcpTool{}, err
		}
		required = append(required, req...)
		mt.body = true
	}
	slices.Sort(required)
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
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
		Name: t.name, Title: t.op.Summary, Description: t.op.Description, InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			Title: t.op.Summary, ReadOnlyHint: readOnly, DestructiveHint: new(t.destructive),
			IdempotentHint: !t.writesAgain, OpenWorldHint: new(false),
		},
	}
	return mt, nil
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
		mt.params[prm.Name] = prm.In
		props[prm.Name] = s
		// The day of a tool is today when left out.
		if prm.Required && prm.Name != "date" {
			required = append(required, prm.Name)
		}
	}
	return required, nil
}

// body adds the properties of op's JSON body to props, and returns those
// required.
func (d openAPIDoc) body(op openAPIOp, props map[string]any) ([]string, error) {
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
			return d.inline(s, depth+1)
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
