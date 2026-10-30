package api

import (
	"context"
	"errors"
	"strings"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// undoneBody is a change undo took back.
type undoneBody struct {
	Action string    `json:"action" doc:"What the change did: food.save, food.archive, recipe.save, meal.create, meal.replace, meal.delete, day.exclude, day.include, day.training, day.checkin, metric.set, metric.delete, metrics.merge or notes.set."`
	Entity string    `json:"entity" doc:"What it changed: a food's or a meal's id, a day (YYYY-MM-DD), a day and a metric (2026-10-21/weight), or notes."`
	At     time.Time `json:"at" doc:"When the change was made."`
	Client string    `json:"client" doc:"What it came through: web, api, mcp."`
	Model  string    `json:"model,omitempty" doc:"The AI model that made it for the user, when one did."`
	Days   []string  `json:"days" nullable:"false" doc:"The days (YYYY-MM-DD) it was of, in order: a meal's before and after it (two for a meal moved to another day), or a check-in's, a measurement's or a day's; none for a food or the notes. get_day shows each after the undo."`
}

// lastChange is the change a client means to take back, one it made, as
// undone names it.
type lastChange struct {
	Action string `json:"action" minLength:"1" maxLength:"50" doc:"What the change did, as undone's action: meal.create, meal.delete, metric.set…"`
	Entity string `json:"entity" minLength:"1" maxLength:"100" doc:"What it changed, as undone's entity: a meal's id, a day, a day and a metric (2026-10-21/weight)."`
}

type undoInput struct {
	Body struct {
		ID   string      `json:"id,omitempty" format:"uuid" doc:"A UUIDv7 made now (a day back to five minutes ahead): the undo sent again under it takes back nothing more. Left out (not empty), the server makes one, and an undo sent again takes back another change."`
		Last *lastChange `json:"last,omitempty" doc:"The change to take back, which the web app's undo of a change it just made names: taken back only while it is the latest. After another, made since through any client, nothing is taken back (412), and the same id may be sent again."`
	}
}

type undoOutput struct {
	Body struct {
		Undone   undoneBody `json:"undone"`
		Repeated bool       `json:"repeated,omitempty" doc:"The undo was sent before under its id, and this is its answer again: no other change was taken back. To go further back, undo under a new id."`
	}
}

func (s *sessions) undoLast(ctx context.Context, in *undoInput) (*undoOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	// A client that cannot make an id (a model without a clock) leaves it
	// out: the server makes one now.
	request := clientid.NewAt(s.now())
	if in.Body.ID != "" {
		var err error
		if request, err = uuid.Parse(in.Body.ID); err != nil {
			return nil, huma.Error422UnprocessableEntity("the undo's id: " + err.Error())
		}
	}
	var e event.Event
	var again bool
	if last := in.Body.Last; last != nil {
		e, again, err = s.db.UndoOf(ctx, p.User.ID, actorOf(p), request, store.Change{Action: event.Action(last.Action), Entity: last.Entity}, s.now())
	} else {
		e, again, err = s.db.Undo(ctx, p.User.ID, actorOf(p), request, s.now())
	}
	var conflict *store.UndoConflictError
	var other *store.NotLatestError
	switch {
	case errors.Is(err, store.ErrUndoID):
		return nil, huma.Error422UnprocessableEntity(strings.TrimPrefix(err.Error(), store.ErrUndoID.Error()+": "))
	case errors.Is(err, store.ErrNothingToUndo):
		return nil, huma.Error404NotFound("nothing to undo: every change of the user's is undone already, or one undo leaves (the plan, an import's)")
	case errors.As(err, &other):
		return nil, huma.Error412PreconditionFailed(other.Error() + ": nothing was taken back")
	case errors.As(err, &conflict):
		if conflict.Err != nil {
			s.logger.DebugContext(ctx, "a change could not be undone", "event", conflict.Event.ID.String(), "error", conflict.Err)
		}
		return nil, huma.Error409Conflict(conflict.Error() + "; it is passed over, so an undo under a new id takes back the change before it, and this id sent again answers the same")
	case err != nil:
		return nil, internalError("undoing failed", err)
	}
	out := &undoOutput{}
	out.Body.Repeated = again
	out.Body.Undone = undoneBody{Action: string(e.Action), Entity: e.Entity, At: e.At.UTC(), Client: e.Actor.Client, Model: e.Actor.Model, Days: []string{}}
	for _, d := range store.DaysOf(e) {
		out.Body.Undone.Days = append(out.Body.Undone.Days, d.String())
	}
	return out, nil
}
