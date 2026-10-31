package api

import (
	"context"

	"github.com/zarnautovic/tdeemon/internal/coaching"
)

// coachingGuideInput names the session whose guide is asked for. Its enum
// is coaching.Names(), which a test holds it to.
type coachingGuideInput struct {
	Session string `path:"session" enum:"monthly_review,plateau_check,new_phase" doc:"Which session."`
}

type coachingGuideOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         coachingGuideBody
}

// coachingGuideBody is a coaching session as a model runs it.
type coachingGuideBody struct {
	Session string `json:"session" doc:"The session's name."`
	Title   string `json:"title" doc:"Its name to a person."`
	Guide   string `json:"guide" doc:"What to do, in Markdown: the session's steps, then how tdeemon's numbers come about (docs/methodology.md)."`
}

// getCoachingGuide answers a coaching session's steps and the methodology
// (DESIGN.md §8): text, the same for every user.
func (s *sessions) getCoachingGuide(ctx context.Context, in *coachingGuideInput) (*coachingGuideOutput, error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}
	// The enum lets no other name in.
	session, _ := coaching.Find(in.Session)
	return &coachingGuideOutput{
		CacheControl: "no-cache",
		Body:         coachingGuideBody{Session: session.Name, Title: session.Title, Guide: session.Guide()},
	}, nil
}
