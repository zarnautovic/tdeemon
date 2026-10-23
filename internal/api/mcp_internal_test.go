package api

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
)

// A tool's definition comes from the document whole, or not at all: a
// $ref to nothing, a body not of JSON, a name both a parameter and a
// property of the body, an operation the document lacks; and a tool
// with no brief description for a model.
func TestMCPToolOfABadDocument(t *testing.T) {
	t.Parallel()

	tl := tool{name: "a_tool", scopes: []auth.Scope{auth.ScopeTools}, op: huma.Operation{Method: http.MethodPut, Path: "/x/{id}"}, brief: "A tool."}
	param := `"parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}]`
	for name, tc := range map[string]struct{ doc, want string }{
		"a $ref to nothing": {`{"paths": {"/x/{id}": {"put": {` + param + `, "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Gone"}}}}}}}}`, "a $ref to nothing"},
		"a body of text":    {`{"paths": {"/x/{id}": {"put": {` + param + `, "requestBody": {"content": {"text/plain": {"schema": {"type": "string"}}}}}}}}`, "not JSON"},
		"a name twice": {`{"paths": {"/x/{id}": {"put": {` + param + `, "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/B"}}}}}}},
			"components": {"schemas": {"B": {"type": "object", "properties": {"id": {"type": "string"}}}}}}`, "both a parameter and a property"},
		"a schema of itself": {`{"paths": {"/x/{id}": {"put": {` + param + `, "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/B"}}}}}}},
			"components": {"schemas": {"B": {"type": "object", "properties": {"b": {"$ref": "#/components/schemas/B"}}}}}}`, "too deep"},
		"no operation": {`{"paths": {}}`, "no PUT /x/{id}"},
	} {
		if _, err := newMCPServer([]tool{tl}, []byte(tc.doc), nil, ServerInfo{}, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
	if _, err := newMCPServer([]tool{tl}, []byte(`not json`), nil, ServerInfo{}, nil); err == nil {
		t.Error("a document that is no JSON was taken")
	}
	tl.brief = ""
	doc := `{"paths": {"/x/{id}": {"put": {` + param + `}}}}`
	if _, err := newMCPServer([]tool{tl}, []byte(doc), nil, ServerInfo{}, nil); err == nil || !strings.Contains(err.Error(), "no brief") {
		t.Errorf("a tool without a brief: %v, want an error", err)
	}
}

// A tool whose operation panics is an error of the tool: the panic does
// not take the server down with it, as the SDK runs a call on a goroutine
// of its own.
func TestMCPCallThatPanics(t *testing.T) {
	t.Parallel()

	var log strings.Builder
	m := &mcpServer{routes: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }), logger: slog.New(slog.NewTextHandler(&log, nil))}
	tl := &mcpTool{tool: tool{name: "a_tool", op: huma.Operation{Method: http.MethodGet, Path: "/x"}}}
	if res := m.call(t.Context(), auth.Principal{}, tl, nil); !res.IsError {
		t.Errorf("a call that panics = %+v, want an error of the tool", res)
	}
	// The log says where, as net/http's does for a panic of a request.
	if !strings.Contains(log.String(), "boom") || !strings.Contains(log.String(), "goroutine") {
		t.Errorf("the log of the panic: %s", log.String())
	}
}
