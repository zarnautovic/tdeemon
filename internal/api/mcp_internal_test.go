package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
)

// A tool's definition comes from the document whole, or not at all: a
// $ref to nothing, a body not of JSON, a name both a parameter and a
// property of the body, an operation the document lacks.
func TestMCPToolOfABadDocument(t *testing.T) {
	t.Parallel()

	tl := tool{name: "a_tool", scopes: []auth.Scope{auth.ScopeTools}, op: huma.Operation{Method: http.MethodPut, Path: "/x/{id}"}}
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
}
