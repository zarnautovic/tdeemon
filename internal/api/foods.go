package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// actorOf is who writes for the request of p: the user, through the web
// app with a session, or through the API with a token.
func actorOf(p auth.Principal) event.Actor {
	if p.Via == auth.ViaSession {
		return webActor
	}
	return event.Actor{Kind: event.User, Client: "api"}
}

// servingBody is a named amount of a food, in its unit.
type servingBody struct {
	Name   string  `json:"name" minLength:"1" maxLength:"200" example:"slice"`
	Amount float64 `json:"amount" exclusiveMinimum:"0" doc:"In the food's unit: grams or ml."`
}

// foodNutrientsBody is a food's nutrients as they are stored, per 100 g,
// 100 ml or portion: not rounded, so a food read and saved again keeps
// its values.
type foodNutrientsBody struct {
	Kcal         float64  `json:"kcal" minimum:"0"`
	Protein      float64  `json:"protein" minimum:"0"`
	Carbs        float64  `json:"carbs" minimum:"0" doc:"Polyols are part of them, as on an EU label."`
	Fat          float64  `json:"fat" minimum:"0"`
	Fiber        *float64 `json:"fiber,omitempty" minimum:"0"`
	SaturatedFat *float64 `json:"saturated_fat,omitempty" minimum:"0"`
	Sugars       *float64 `json:"sugars,omitempty" minimum:"0"`
	Salt         *float64 `json:"salt,omitempty" minimum:"0"`
	Polyols      *float64 `json:"polyols,omitempty" minimum:"0"`
	Alcohol      *float64 `json:"alcohol,omitempty" minimum:"0"`
}

func newFoodNutrientsBody(n food.Nutrients) foodNutrientsBody {
	return foodNutrientsBody{
		Kcal: n.Kcal, Protein: n.Protein, Carbs: n.Carbs, Fat: n.Fat,
		Fiber: n.Fiber, SaturatedFat: n.SaturatedFat, Sugars: n.Sugars, Salt: n.Salt, Polyols: n.Polyols, Alcohol: n.Alcohol,
	}
}

func (n foodNutrientsBody) nutrients() food.Nutrients {
	return food.Nutrients{
		Kcal: n.Kcal, Protein: n.Protein, Carbs: n.Carbs, Fat: n.Fat,
		Fiber: n.Fiber, SaturatedFat: n.SaturatedFat, Sugars: n.Sugars, Salt: n.Salt, Polyols: n.Polyols, Alcohol: n.Alcohol,
	}
}

// FoodFields are a food's fields, as upsert_food takes them and an answer
// shows them. It is exported so foodBody can embed it: huma describes no
// field of an unexported embedded struct.
type FoodFields struct {
	Name         string            `json:"name" minLength:"1" maxLength:"200" example:"Greek yoghurt 2 %"`
	Brand        string            `json:"brand,omitempty" maxLength:"100" doc:"The same name and brand are the same food."`
	Basis        string            `json:"basis" enum:"100g,100ml,portion" doc:"What the nutrients are for: 100 g, 100 ml, or a portion."`
	Nutrients    foodNutrientsBody `json:"nutrients"`
	PortionGrams float64           `json:"portion_grams,omitempty" minimum:"0" doc:"The weight of a portion of a food per portion, when known: it can then be logged in grams."`
	Servings     []servingBody     `json:"servings,omitempty" nullable:"false" doc:"Named amounts: a piece, a slice."`
	Aliases      []string          `json:"aliases,omitempty" nullable:"false" doc:"Other names the user calls the food by; search finds them."`
	Source       string            `json:"source" enum:"label,estimate,user,dish" doc:"Where the values come from: a label is exact, an estimate a judgement."`
	Note         string            `json:"note,omitempty" maxLength:"2000"`
}

// lastUseBody is the last time a meal used a food: what search offers as
// the usual amount.
type lastUseBody struct {
	At       time.Time    `json:"at"`
	MealID   string       `json:"meal_id" format:"uuid"`
	Quantity quantityBody `json:"quantity" doc:"How much, as it was given."`
	Amount   float64      `json:"amount" doc:"How much, in unit."`
	Unit     string       `json:"unit" enum:"g,ml,portion"`
}

// foodBody is a food as stored.
type foodBody struct {
	ID string `json:"id" format:"uuid"`
	FoodFields
	Archived bool         `json:"archived" doc:"An archived food stays out of a search, and keeps its name and aliases."`
	LastUse  *lastUseBody `json:"last_use,omitempty" doc:"The last time a meal used the food; absent when none has."`
}

func newFoodBody(f *food.Food) foodBody {
	b := foodBody{
		ID: f.ID.String(), Name: f.Name, Brand: f.Brand, Basis: string(f.Basis), Nutrients: newFoodNutrientsBody(f.Nutrients),
		PortionGrams: f.PortionGrams, Aliases: f.Aliases, Source: string(f.Source), Note: f.Note, Archived: f.Archived,
	}
	for _, s := range f.Servings {
		b.Servings = append(b.Servings, servingBody{Name: s.Name, Amount: s.Amount})
	}
	return b
}

type foodsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Foods []foodBody `json:"foods" nullable:"false"`
	}
}

type findFoodsInput struct {
	Query string `query:"query" required:"true" minLength:"1" maxLength:"200" example:"greek yog" doc:"Words the name, the brand or an alias starts with, in any case and without the diacritics."`
	Limit int    `query:"limit" default:"10" minimum:"1" maximum:"50"`
}

func (s *sessions) findFoods(ctx context.Context, in *findFoodsInput) (*foodsOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	foods, err := s.db.FindFoods(ctx, p.User.ID, in.Query, in.Limit)
	if err != nil {
		return nil, internalError("finding foods failed", err)
	}
	ids := make([]uuid.UUID, len(foods))
	for i := range foods {
		ids[i] = foods[i].ID
	}
	uses, err := s.db.LastUses(ctx, p.User.ID, ids)
	if err != nil {
		return nil, internalError("finding foods failed", err)
	}
	out := &foodsOutput{CacheControl: "no-store"}
	out.Body.Foods = make([]foodBody, 0, len(foods))
	for i := range foods {
		b := newFoodBody(&foods[i])
		if u, ok := uses[foods[i].ID]; ok {
			b.LastUse = &lastUseBody{
				At: u.At, MealID: u.MealID.String(), Amount: u.Amount, Unit: u.Unit,
				Quantity: quantityBody{
					Amount: u.Quantity.Amount, Unit: u.Quantity.Unit,
					Servings: u.Quantity.Servings, Serving: u.Quantity.Serving, Portions: u.Quantity.Portions,
				},
			}
		}
		out.Body.Foods = append(out.Body.Foods, b)
	}
	return out, nil
}

type upsertFoodInput struct {
	Body struct {
		ID string `json:"id,omitempty" format:"uuid" doc:"The food to correct, by its id; without one, the food of the same name and brand, or a new one."`
		FoodFields
	}
}

type foodOutput struct {
	Body foodBody
}

func (s *sessions) upsertFood(ctx context.Context, in *upsertFoodInput) (*foodOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	b := in.Body
	f := food.Food{
		ID: uuid.NewV7(), Name: b.Name, Brand: b.Brand, Basis: food.Basis(b.Basis), Nutrients: b.Nutrients.nutrients(),
		PortionGrams: b.PortionGrams, Aliases: b.Aliases, Source: food.Source(b.Source), Note: b.Note,
	}
	if b.ID != "" {
		if f.ID, err = uuid.Parse(b.ID); err != nil {
			return nil, huma.Error422UnprocessableEntity("the id: " + err.Error())
		}
	}
	for _, sv := range b.Servings {
		f.Servings = append(f.Servings, food.Serving{Name: sv.Name, Amount: sv.Amount})
	}
	if err := f.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity(strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	saved, err := s.db.SaveFood(ctx, p.User.ID, actorOf(p), f, s.now())
	var conflict *store.FoodConflictError
	switch {
	case errors.As(err, &conflict):
		return nil, huma.Error409Conflict(conflictDetail(conflict))
	case err != nil:
		return nil, internalError("saving the food failed", err)
	}
	return &foodOutput{Body: newFoodBody(&saved)}, nil
}

// conflictDetail says which food is in the way of a save, so a client can
// correct that one, rename it or take its alias away: an archived food is
// in no search.
func conflictDetail(c *store.FoodConflictError) string {
	what := "the name and brand are taken by food "
	if c.Alias != "" {
		what = fmt.Sprintf("the alias %q is taken by food ", c.Alias)
	}
	archived := ""
	if c.Food.Archived {
		archived = ", archived"
	}
	return fmt.Sprintf("%s%s (%q%s): correct that food, or save this one under another name or without the alias",
		what, c.Food.ID, c.Food.Name, archived)
}
