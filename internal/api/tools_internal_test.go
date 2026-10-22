package api

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
)

// A tool names the scopes of the tokens that may call it: one that does
// not cannot be registered, so a tool that writes is never open to read
// tokens by default.
func TestRegisterToolsNeedsTheScopes(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("a tool with no scopes was registered")
		}
	}()
	registerTools(newAPI(http.NewServeMux(), slog.New(slog.DiscardHandler)), []tool{
		newTool("log_nothing", nil, huma.Operation{OperationID: "log-nothing", Method: http.MethodPost, Path: "/nothing"},
			func(context.Context, *struct{}) (*struct{}, error) { return nil, nil }), //nolint:nilnil // no output
	})
}

// registerTools keeps the Metadata a tool's operation has, and adds its
// name and scopes.
func TestRegisterToolsKeepsTheMetadata(t *testing.T) {
	t.Parallel()

	a := newAPI(http.NewServeMux(), slog.New(slog.DiscardHandler))
	registerTools(a, []tool{
		newTool("get_thing", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "get-thing", Method: http.MethodGet, Path: "/thing", Metadata: map[string]any{"own": 1},
		}, func(context.Context, *struct{}) (*struct{}, error) { return nil, nil }), //nolint:nilnil // no output
	})
	md := a.OpenAPI().Paths["/thing"].Get.Metadata
	if scopes, _ := md[tokenScopeKey].([]auth.Scope); md["own"] != 1 || md[toolKey] != "get_thing" || len(scopes) != 1 || scopes[0] != auth.ScopeTools {
		t.Errorf("Metadata = %v", md)
	}
}
