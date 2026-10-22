package api

import (
	"context"
	"errors"
	"strings"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/store"
)

// undoneBody is a change undo took back.
type undoneBody struct {
	Action string    `json:"action" doc:"What the change did: food.save, food.archive, meal.create, meal.replace, meal.delete, day.exclude, day.include, day.training, day.checkin, metric.set, metric.delete, metrics.merge or notes.set."`
	Entity string    `json:"entity" doc:"What it changed: a food's or a meal's id, a day (YYYY-MM-DD), a day and a metric (2026-10-21/weight), or notes."`
	At     time.Time `json:"at" doc:"When the change was made."`
	Client string    `json:"client" doc:"What it came through: web, api, mcp."`
	Model  string    `json:"model,omitempty" doc:"The AI model that made it for the user, when one did."`
}

type undoInput struct {
	Body struct {
		ID string `json:"id" format:"uuid" doc:"A UUIDv7 the client makes for the undo, no more than a day before and five minutes ahead of the clock: the same undo sent again under it takes back no other change."`
	}
}

type undoOutput struct {
	Body struct {
		Undone undoneBody `json:"undone"`
	}
}

func (s *sessions) undoLast(ctx context.Context, in *undoInput) (*undoOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	request, err := uuid.Parse(in.Body.ID)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("the undo's id: " + err.Error())
	}
	e, _, err := s.db.Undo(ctx, p.User.ID, actorOf(p), request, s.now())
	var conflict *store.UndoConflictError
	switch {
	case errors.Is(err, store.ErrUndoID):
		return nil, huma.Error422UnprocessableEntity(strings.TrimPrefix(err.Error(), store.ErrUndoID.Error()+": "))
	case errors.Is(err, store.ErrNothingToUndo):
		return nil, huma.Error404NotFound("nothing to undo: every change of the user's is undone already, or one undo leaves (the plan, an import's)")
	case errors.As(err, &conflict):
		return nil, huma.Error409Conflict(conflict.Error() + "; it is passed over, so undo again takes back the change before it")
	case err != nil:
		return nil, internalError("undoing failed", err)
	}
	out := &undoOutput{}
	out.Body.Undone = undoneBody{Action: string(e.Action), Entity: e.Entity, At: e.At.UTC(), Client: e.Actor.Client, Model: e.Actor.Model}
	return out, nil
}
