package coaching_test

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/docs"
	"github.com/zarnautovic/tdeemon/internal/coaching"
)

func TestSessions(t *testing.T) {
	t.Parallel()
	want := []string{"monthly_review", "plateau_check", "new_phase"}
	if got := coaching.Names(); !slices.Equal(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	titles := map[string]string{"monthly_review": "Monthly review", "plateau_check": "Plateau check", "new_phase": "Plan a new phase"}
	for _, s := range coaching.Sessions() {
		if s.Title != titles[s.Name] {
			t.Errorf("%s: title %q, want %q", s.Name, s.Title, titles[s.Name])
		}
		if s.Description == "" {
			t.Errorf("%s: no description", s.Name)
		}
		if !strings.HasPrefix(s.Steps, "# "+s.Title+"\n") || !strings.Contains(s.Steps, "1. ") {
			t.Errorf("%s: steps without their title or a first step:\n%s", s.Name, s.Steps)
		}
		// Every session ends in the user's language and leaves a change
		// to the user's yes.
		for _, must := range []string{"user's language", "`apply_proposal` only after the user's yes"} {
			if !strings.Contains(strings.Join(strings.Fields(s.Steps), " "), must) {
				t.Errorf("%s: its steps do not say %q", s.Name, must)
			}
		}
	}
}

func TestSessionsAreACopy(t *testing.T) {
	t.Parallel()
	got := coaching.Sessions()
	got[0].Steps = "changed"
	if again := coaching.Sessions(); again[0].Steps == "changed" {
		t.Error("Sessions() shares its slice with the package")
	}
}

func TestFind(t *testing.T) {
	t.Parallel()
	s, ok := coaching.Find("plateau_check")
	if !ok || s.Title != "Plateau check" {
		t.Errorf(`Find("plateau_check") = %+v, %v`, s, ok)
	}
	if s, ok := coaching.Find("weekly_review"); ok {
		t.Errorf(`Find("weekly_review") = %+v, want none`, s)
	}
}

func TestGuide(t *testing.T) {
	t.Parallel()
	s, _ := coaching.Find("monthly_review")
	g := s.Guide()
	if !strings.HasPrefix(g, s.Steps) || !strings.HasSuffix(g, docs.Methodology) {
		t.Errorf("the guide is not the steps, then the methodology:\n%s", g)
	}
	if !strings.Contains(docs.Methodology, "# How tdeemon reads the numbers") {
		t.Error("the methodology is not docs/methodology.md")
	}
}

// TestEveryStepsFileIsASession fails for a file of steps no session
// names, which nothing would serve.
func TestEveryStepsFileIsASession(t *testing.T) {
	t.Parallel()
	files, err := fs.Glob(os.DirFS("sessions"), "*.md")
	if err != nil {
		t.Fatal(err)
	}
	names := coaching.Names()
	for _, f := range files {
		if !slices.Contains(names, strings.TrimSuffix(f, ".md")) {
			t.Errorf("sessions/%s is no session's", f)
		}
	}
	if len(files) != len(names) {
		t.Errorf("%d files of steps for %d sessions", len(files), len(names))
	}
}
