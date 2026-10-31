package event

import (
	"strings"
	"testing"
)

func TestActorValidate(t *testing.T) {
	t.Parallel()

	for _, a := range []Actor{
		{Kind: User, Client: "mcp", Model: "claude-opus-5-5"},
		{Kind: User, Client: "web"},
		{Kind: Import, Client: "hae"},
		{Kind: Autopilot, Client: ClientAutopilot},
	} {
		if err := a.Validate(); err != nil {
			t.Errorf("%+v: %v", a, err)
		}
	}
	for _, tc := range []struct {
		a    Actor
		want string
	}{
		{Actor{Kind: "robot", Client: "web"}, `actor "robot"`},
		{Actor{Kind: User}, "client is required"},
		{Actor{Kind: User, Client: "web", Model: " gpt"}, "model"},
		{Actor{Kind: User, Client: strings.Repeat("c", MaxClientLen+1)}, "at most 50"},
	} {
		if err := tc.a.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Validate(%+v) = %v, want an error with %q", tc.a, err, tc.want)
		}
	}
}
