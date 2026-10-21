package event

import (
	"strings"
	"testing"
	"uuid"
)

func TestActorValidate(t *testing.T) {
	t.Parallel()

	for _, a := range []Actor{
		{Kind: User, Client: "mcp", Model: "claude-opus-5-5"},
		{Kind: User, Client: "web"},
		{Kind: Import, Client: "hae"},
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

func TestUndoable(t *testing.T) {
	t.Parallel()

	user := Actor{Kind: User, Client: "web"}
	for _, tc := range []struct {
		name string
		e    Event
		want bool
	}{
		{"the user's change", Event{Actor: user, Action: MealCreate}, true},
		{"an import", Event{Actor: Actor{Kind: Import, Client: "hae"}, Action: MetricsMerge}, false},
		{"an undo", Event{Actor: user, Action: Undo}, false},
		{"undone already", Event{Actor: user, Action: MealCreate, UndoneBy: uuid.NewV7()}, false},
	} {
		if got := tc.e.Undoable(); got != tc.want {
			t.Errorf("%s: Undoable = %v, want %v", tc.name, got, tc.want)
		}
	}
}
