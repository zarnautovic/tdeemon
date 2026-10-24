package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	switch {
	case p.Via == auth.ViaSession:
		return webActor
	case p.Client == auth.ClientMCP:
		return event.Actor{Kind: event.User, Client: auth.ClientMCP}
	}
	return event.Actor{Kind: event.User, Client: "api"}
}

// servingBody is a named amount of a food, in its unit.
type servingBody struct {
	Name   string  `json:"name" minLength:"1" maxLength:"50" example:"slice"`
	Amount float64 `json:"amount" exclusiveMinimum:"0" maximum:"10000" doc:"In g, or ml for a food by 100ml."`
}

// foodNutrientsBody is a food's nutrients as they are stored, per 100 g,
// 100 ml or portion: not rounded, so a food read and saved again keeps
// its values.
type foodNutrientsBody struct {
	Kcal         float64  `json:"kcal" minimum:"0"`
	Protein      float64  `json:"protein" minimum:"0"`
	Carbs        float64  `json:"carbs" minimum:"0" doc:"With polyols, as on an EU label."`
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
	Brand        string            `json:"brand,omitempty" maxLength:"100" doc:"The same name and brand are one food."`
	Basis        string            `json:"basis" enum:"100g,100ml,portion" doc:"What the nutrients are for."`
	Nutrients    foodNutrientsBody `json:"nutrients"`
	PortionGrams float64           `json:"portion_grams,omitempty" minimum:"0" doc:"For basis portion only: the grams of a portion, when known."`
	Servings     []servingBody     `json:"servings,omitempty" nullable:"false" maxItems:"20" doc:"Named amounts: a piece, a slice."`
	Aliases      []string          `json:"aliases,omitempty" nullable:"false" maxItems:"20" minLength:"1" maxLength:"100" doc:"Other names search finds."`
	Source       string            `json:"source" enum:"label,estimate,user,dish" doc:"Where the values come from; dish is an earlier client's, for no new food."`
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
	Queries  []string     `json:"queries,omitempty" nullable:"false" doc:"The queries of find_foods that found the food, of those it was sent."`
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
		Foods   []foodBody `json:"foods" nullable:"false"`
		Unfound []string   `json:"unfound" nullable:"false" doc:"The queries that found no food."`
	}
}

// findFoodsInput's maxItems is store.MaxFoodQueries, and its limit's
// maximum store.MaxFoodResults.
type findFoodsInput struct {
	Query []string `query:"query,explode" required:"true" nullable:"false" minItems:"1" maxItems:"20" minLength:"1" maxLength:"200" example:"greek yog" doc:"The start of words of the name, brand or an alias, in any case, without diacritics; one a food."`
	Limit int      `query:"limit" default:"10" minimum:"1" maximum:"50" doc:"The most foods of each query."`
}

func (s *sessions) findFoods(ctx context.Context, in *findFoodsInput) (*foodsOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	// The foods of each query, in the order of the queries; a food two
	// find, once, with both. A query sent twice, in any case, is one.
	var foods []food.Food
	var queries [][]string
	unfound := []string{}
	at := map[uuid.UUID]int{}
	asked := map[string]bool{}
	for _, q := range in.Query {
		key := strings.ToLower(strings.TrimSpace(q))
		if asked[key] {
			continue
		}
		asked[key] = true
		found, err := s.db.FindFoods(ctx, p.User.ID, q, in.Limit)
		if err != nil {
			return nil, internalError("finding foods failed", err)
		}
		if len(found) == 0 && key != "" {
			unfound = append(unfound, strings.TrimSpace(q))
		}
		for i := range found {
			if j, ok := at[found[i].ID]; ok {
				queries[j] = append(queries[j], q)
				continue
			}
			at[found[i].ID] = len(foods)
			foods, queries = append(foods, found[i]), append(queries, []string{q})
		}
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
	out.Body.Foods, out.Body.Unfound = make([]foodBody, 0, len(foods)), unfound
	for i := range foods {
		b := newFoodBody(&foods[i])
		b.Queries = queries[i]
		if u, ok := uses[foods[i].ID]; ok {
			b.LastUse = newLastUseBody(u)
		}
		out.Body.Foods = append(out.Body.Foods, b)
	}
	return out, nil
}

func newLastUseBody(u store.LastUse) *lastUseBody {
	return &lastUseBody{
		At: u.At, MealID: u.MealID.String(), Amount: u.Amount, Unit: u.Unit,
		Quantity: quantityBody{
			Amount: u.Quantity.Amount, Unit: u.Quantity.Unit,
			Servings: u.Quantity.Servings, Serving: u.Quantity.Serving, Portions: u.Quantity.Portions,
		},
	}
}

// listFoodsInput's limit's maximum is store.MaxFoodList, and its order's
// values store.FoodOrder's.
type listFoodsInput struct {
	Archived bool   `query:"archived" doc:"The archived foods instead of the others."`
	Order    string `query:"order" enum:"name,use" default:"name" doc:"name: by name, then brand; use: the food a meal had last first, and those no meal had after the others, by name."`
	Limit    int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"The most foods of the page."`
	Offset   int    `query:"offset" default:"0" minimum:"0" maximum:"1000000" doc:"How many foods of the list come before the page."`
}

type foodListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Foods []foodBody `json:"foods" nullable:"false"`
		Total int        `json:"total" doc:"How many foods the whole list has."`
	}
}

func (s *sessions) listFoods(ctx context.Context, in *listFoodsInput) (*foodListOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	foods, total, err := s.db.ListFoods(ctx, p.User.ID, in.Archived, store.FoodOrder(in.Order), in.Limit, in.Offset)
	if err != nil {
		return nil, internalError("listing the foods failed", err)
	}
	ids := make([]uuid.UUID, len(foods))
	for i := range foods {
		ids[i] = foods[i].ID
	}
	uses, err := s.db.LastUses(ctx, p.User.ID, ids)
	if err != nil {
		return nil, internalError("listing the foods failed", err)
	}
	out := &foodListOutput{CacheControl: "no-store"}
	out.Body.Total = total
	out.Body.Foods = make([]foodBody, 0, len(foods))
	for i := range foods {
		b := newFoodBody(&foods[i])
		if u, ok := uses[foods[i].ID]; ok {
			b.LastUse = newLastUseBody(u)
		}
		out.Body.Foods = append(out.Body.Foods, b)
	}
	return out, nil
}

func (s *sessions) registerFoods(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "list-foods",
		Method:      http.MethodGet,
		Path:        Prefix + "/foods/list",
		Summary:     "List your foods",
		Description: "Needs a web session. A page of your foods, the archived ones or the others, by name or by their last use, " +
			"with how many there are; find_foods searches them.",
		Tags:   []string{"foods"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.listFoods)
	huma.Register(a, huma.Operation{
		OperationID: "create-food",
		Method:      http.MethodPost,
		Path:        Prefix + "/foods/new",
		Summary:     "Make a new food",
		Description: "Needs a web session. Saves a new food: a food you have of the name and brand, archived too, is refused (409), naming it, " +
			"and so is an alias another food has; upsert_food corrects a food instead.",
		Tags:          []string{"foods"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity},
	}, s.createFood)
}

type getFoodInput struct {
	ID string `path:"id" format:"uuid"`
}

type foodOutputNoStore struct {
	CacheControl string `header:"Cache-Control"`
	Body         foodBody
}

func (s *sessions) getFood(ctx context.Context, in *getFoodInput) (*foodOutputNoStore, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, huma.Error404NotFound("no such food")
	}
	f, err := s.db.FoodByID(ctx, p.User.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such food")
	} else if err != nil {
		return nil, internalError("reading the food failed", err)
	}
	return &foodOutputNoStore{CacheControl: "no-store", Body: newFoodBody(&f)}, nil
}

type upsertFoodInput struct {
	Body struct {
		ID string `json:"id,omitempty" format:"uuid" doc:"The food to correct; without it, the food of the name and brand, or a new one."`
		FoodFields
		Archived bool `json:"archived,omitempty" doc:"Archives the food (out of search, its names kept); left out, a save brings it back."`
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
	f := b.food()
	f.Archived = b.Archived
	if b.ID != "" {
		// An id corrects a food the user has; a new food comes without
		// one, so a mistyped or made-up id makes no duplicate.
		if f.ID, err = uuid.Parse(b.ID); err != nil {
			return nil, huma.Error422UnprocessableEntity("the id: " + err.Error())
		}
	}
	if err := f.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity(strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	save := s.db.SaveFood
	if b.ID != "" {
		save = s.db.CorrectFood
	}
	saved, err := save(ctx, p.User.ID, actorOf(p), f, s.now())
	var conflict *store.FoodConflictError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, huma.Error404NotFound("no such food: a new food comes without an id")
	case errors.As(err, &conflict):
		return nil, huma.Error409Conflict(conflictDetail(conflict))
	case err != nil:
		return nil, internalError("saving the food failed", err)
	}
	return &foodOutput{Body: newFoodBody(&saved)}, nil
}

// food is a new food of b's fields, under a new id.
func (b *FoodFields) food() food.Food {
	f := food.Food{
		ID: uuid.NewV7(), Name: b.Name, Brand: b.Brand, Basis: food.Basis(b.Basis), Nutrients: b.Nutrients.nutrients(),
		PortionGrams: b.PortionGrams, Aliases: b.Aliases, Source: food.Source(b.Source), Note: b.Note,
	}
	for _, sv := range b.Servings {
		f.Servings = append(f.Servings, food.Serving{Name: sv.Name, Amount: sv.Amount})
	}
	return f
}

type createFoodInput struct {
	Body FoodFields
}

type createdFoodOutput struct {
	Body foodBody
}

// createFood saves a new food, and only that: a food the user has of the
// name and brand, archived too, is in the way (409), where upsert_food
// would correct it. The web app's "Add a food" makes a new food.
func (s *sessions) createFood(ctx context.Context, in *createFoodInput) (*createdFoodOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	f := in.Body.food()
	if err := f.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity(strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	saved, err := s.db.CreateFood(ctx, p.User.ID, actorOf(p), f, s.now())
	var conflict *store.FoodConflictError
	switch {
	case errors.As(err, &conflict):
		return nil, huma.Error409Conflict(conflictDetail(conflict))
	case err != nil:
		return nil, internalError("saving the food failed", err)
	}
	return &createdFoodOutput{Body: newFoodBody(&saved)}, nil
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
