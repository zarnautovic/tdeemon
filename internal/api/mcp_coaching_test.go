package api_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/zarnautovic/tdeemon/docs"
	"github.com/zarnautovic/tdeemon/internal/coaching"
)

// The coaching sessions are MCP's prompts, which a client shows as
// commands, for a token that reads as for one that uses the tools.
func TestMCPCoachingPrompts(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	for _, scopes := range []string{`"read"`, `"tools"`} {
		cs := mcpSession(t, in, mcpToken(t, in, session, scopes))
		res, err := cs.ListPrompts(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, p := range res.Prompts {
			names = append(names, p.Name)
			s, _ := coaching.Find(p.Name)
			if p.Title != s.Title || p.Description != s.Description || len(p.Arguments) != 1 || p.Arguments[0].Name != "about" || p.Arguments[0].Required {
				t.Errorf("%s with %s: %+v", p.Name, scopes, p)
			}
		}
		// The SDK lists them by name.
		if want := slices.Sorted(slices.Values(coaching.Names())); !slices.Equal(names, want) {
			t.Errorf("the prompts with %s = %v, want %v", scopes, names, want)
		}
	}
}

func TestMCPCoachingPrompt(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cs := mcpSession(t, in, mcpToken(t, in, session, `"read"`))
	s, _ := coaching.Find("plateau_check")
	text := func(args map[string]string) string {
		t.Helper()
		res, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: s.Name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Messages) != 1 || res.Messages[0].Role != "user" || res.Description != s.Description {
			t.Fatalf("the prompt: %+v", res)
		}
		c, ok := res.Messages[0].Content.(*mcp.TextContent)
		if !ok {
			t.Fatalf("the prompt's content: %T", res.Messages[0].Content)
		}
		return c.Text
	}
	plain := text(nil)
	if !strings.HasPrefix(plain, "Run tdeemon's Plateau check with me") || !strings.Contains(plain, s.Guide()) || strings.Contains(plain, "What I want") {
		t.Errorf("the prompt without what the user wants:\n%s", plain)
	}
	if about := text(map[string]string{"about": "  Three weeks at 82 kg.  "}); !strings.HasSuffix(about, "\nWhat I want from it: Three weeks at 82 kg.\n") {
		t.Errorf("the prompt with what the user wants ends:\n%s", about[max(0, len(about)-200):])
	}
	if blank := text(map[string]string{"about": " "}); blank != plain {
		t.Error("a blank about changed the prompt")
	}
	if _, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: "weekly_review"}); err == nil {
		t.Error("a prompt that does not exist answered")
	}
}

// The methodology is MCP's resource, as docs/methodology.md has it.
func TestMCPMethodologyResource(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cs := mcpSession(t, in, mcpToken(t, in, session, `"read"`))
	list, err := cs.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Resources) != 1 || list.Resources[0].URI != "tdeemon://methodology" || list.Resources[0].MIMEType != "text/markdown" ||
		list.Resources[0].Size != int64(len(docs.Methodology)) {
		t.Fatalf("the resources: %+v", list.Resources)
	}
	res, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "tdeemon://methodology"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 || res.Contents[0].Text != docs.Methodology || res.Contents[0].URI != "tdeemon://methodology" {
		t.Errorf("the methodology read: %+v", res.Contents)
	}
}
