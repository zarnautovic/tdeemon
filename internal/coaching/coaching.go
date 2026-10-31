// Package coaching holds the coaching sessions a model runs with the
// user (DESIGN.md §8): for each, the steps to take, which tools to call
// and what to check, and the methodology (docs/methodology.md), so a
// model of any client reads the numbers alike. They are text: the tools
// give them, and MCP serves them as its prompts.
package coaching

import (
	"embed"
	"strings"

	"github.com/zarnautovic/tdeemon/docs"
)

// A Session is a coaching session: a review the user asks for, in steps.
type Session struct {
	// Name names it to the tools and to MCP: monthly_review.
	Name string
	// Title is its name to a person, its steps' heading.
	Title string
	// Description says when it is for, in a line.
	Description string
	// Steps are its steps, in Markdown, under its title.
	Steps string
}

// Guide is what a model reads to run s: its steps, then the
// methodology.
func (s Session) Guide() string {
	return s.Steps + "\n" + docs.Methodology
}

//go:embed sessions/*.md
var files embed.FS

// sessions are the sessions, in the order a client lists them.
var sessions = []Session{
	{Name: "monthly_review", Description: "Review the plan's last weeks: the pace, adherence, the autopilot, and whether to change the plan."},
	{Name: "plateau_check", Description: "The weight seems to have stalled: check whether it has, why, and what to do."},
	{Name: "new_phase", Description: "Plan a new phase, a cut, maintenance, a bulk or a recomp, with its pace and targets."},
}

func init() {
	for i := range sessions {
		s := &sessions[i]
		b, err := files.ReadFile("sessions/" + s.Name + ".md")
		if err != nil {
			panic("coaching: the steps of " + s.Name + ": " + err.Error())
		}
		s.Steps = string(b)
		heading, _, _ := strings.Cut(s.Steps, "\n")
		title, ok := strings.CutPrefix(heading, "# ")
		if !ok {
			panic("coaching: the steps of " + s.Name + " start with no title")
		}
		s.Title = title
	}
}

// Sessions are the coaching sessions, in the order a client lists them.
func Sessions() []Session {
	return append([]Session(nil), sessions...)
}

// Names are the sessions' names, in the order a client lists them.
func Names() []string {
	names := make([]string, len(sessions))
	for i, s := range sessions {
		names[i] = s.Name
	}
	return names
}

// Find is the session named name.
func Find(name string) (Session, bool) {
	for _, s := range sessions {
		if s.Name == name {
			return s, true
		}
	}
	return Session{}, false
}
