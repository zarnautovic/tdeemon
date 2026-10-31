package api

import (
	"context"
	"net/http"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// changeBody is one of the user's latest changes, as its event recorded
// it, with what it was of by the names the user's data has: data a client
// says in the user's language, never a sentence of the server's.
type changeBody struct {
	ID     string    `json:"id" format:"uuid" doc:"The change's own id, its event's."`
	Action string    `json:"action" doc:"What the change did: food.save, food.archive, recipe.save, meal.create, meal.replace, meal.delete, day.exclude, day.include, day.training, day.checkin, metric.set, metric.delete, metrics.merge, notes.set, or a change of the plan: profile.set, limits.set, phase.set, phase.delete, target.set, target.delete, plan.weekdays, proposal.create, proposal.status, autopilot.set (the autopilot's settings)."`
	Entity string    `json:"entity" doc:"What it changed, as undo's last names it: a food's, a meal's or a proposal's id, a day (YYYY-MM-DD), a day and a metric (2026-10-21/weight), profile, limits, plan or notes."`
	At     time.Time `json:"at" doc:"When the change was made."`
	Client string    `json:"client" doc:"What it came through: web, api, mcp, or autopilot for the autopilot's own step or proposal, which undo never takes back."`
	Model  string    `json:"model,omitempty" doc:"The AI model that made it for the user, when one said so."`
	Days   []string  `json:"days" nullable:"false" doc:"The days (YYYY-MM-DD) it was of, as undo's answer says them: a meal's before and after it, a check-in's, a measurement's or a day's; none for a food, the notes or the plan."`
	Name   string    `json:"name,omitempty" doc:"A meal's description, or a food's or a recipe's name: as the change left it (as it was, for a meal deleted; as it is now, for a food archived or brought back)."`
	Brand  string    `json:"brand,omitempty" doc:"A food's brand, beside its name."`
	Items  []string  `json:"items" nullable:"false" doc:"A meal's items by their names, as the change left it (as it was, for a delete); none for anything else."`
	Metric string    `json:"metric,omitempty" doc:"A measurement's kind, of metric.set and metric.delete."`
	// A pointer, so a food brought back says false.
	Archived *bool  `json:"archived,omitempty" doc:"Of food.archive: true when it archived the food, false when it brought it back in use."`
	Status   string `json:"status,omitempty" doc:"Of proposal.status: the proposal's status after it (applied, rejected, superseded, reverted)."`
	Undone   bool   `json:"undone,omitempty" doc:"An undo took it back."`
	Next     bool   `json:"next,omitempty" doc:"Undo takes it back next: POST /api/v1/undo naming it (last) takes it back while it stays the latest. At most one change is next, and none when undo has nothing to take back."`
}

// changesInput's limit's maximum is store.MaxRecentChanges.
type changesInput struct {
	Limit int `query:"limit" default:"10" minimum:"1" maximum:"50" doc:"The most changes."`
}

type changesOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Changes []changeBody `json:"changes" nullable:"false" doc:"The latest first."`
	}
}

func (s *sessions) listChanges(ctx context.Context, in *changesInput) (*changesOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	changes, err := s.db.RecentChanges(ctx, p.User.ID, in.Limit)
	if err != nil {
		return nil, internalError("listing the changes failed", err)
	}
	out := &changesOutput{CacheControl: "no-store"}
	out.Body.Changes = make([]changeBody, 0, len(changes))
	for i := range changes {
		out.Body.Changes = append(out.Body.Changes, newChangeBody(&changes[i]))
	}
	return out, nil
}

func newChangeBody(c *store.RecentChange) changeBody {
	e := c.Event
	b := changeBody{
		ID: e.ID.String(), Action: string(e.Action), Entity: e.Entity, At: e.At.UTC(), Client: e.Actor.Client, Model: e.Actor.Model,
		Days: []string{}, Name: c.Name, Brand: c.Brand, Items: []string{}, Metric: string(c.Metric), Status: c.Status,
		Undone: e.UndoneBy != uuid.Nil(), Next: c.Next,
	}
	for _, d := range store.DaysOf(e) {
		b.Days = append(b.Days, d.String())
	}
	b.Items = append(b.Items, c.Items...)
	if e.Action == event.FoodArchive {
		b.Archived = &c.Archived
	}
	return b
}

func (s *sessions) registerChanges(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "list-changes",
		Method:      http.MethodGet,
		Path:        Prefix + "/changes",
		Summary:     "List your recent changes",
		Description: "Needs a web session. Your latest changes, the latest first, made through any client (you in the app, your AI coach, " +
			"an app with an access key): what each did and to what, when, through what, the names of what it was of, whether an undo took it back, " +
			"and the one undo takes back next. The undos themselves are not among them, nor an import's writes, which undo leaves, " +
			"nor the new foods a meal brought, which are part of the meal's change.",
		Tags:   []string{"changes"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.listChanges)
}
