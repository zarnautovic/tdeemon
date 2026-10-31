package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"reflect"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/recipe"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// RecipeFields are a recipe's fields, as upsert_recipe takes them. It is
// exported so the inputs can embed it: huma describes no field of an
// unexported embedded struct. Its bounds are package recipe's.
type RecipeFields struct {
	Name     string      `json:"name" minLength:"1" maxLength:"200" example:"Chicken curry" doc:"A recipe's name is its food's: the same name is the same recipe."`
	Aliases  []string    `json:"aliases,omitempty" nullable:"false" maxItems:"20" minLength:"1" maxLength:"100" doc:"Other names search finds."`
	Items    []itemInput `json:"items" nullable:"false" minItems:"1" maxItems:"50" doc:"The ingredients, as log_meal's items but no estimates: a food_id or a new food, with its quantity; a new food is saved."`
	YieldG   float64     `json:"yield_g,omitempty" minimum:"0" maximum:"10000" doc:"The cooked dish's weight in g. Left out when not weighed: the raw ingredients' weight stands for it, so a share of the dish is exact and its grams not."`
	Portions int         `json:"portions,omitempty" minimum:"0" maximum:"100" doc:"How many portions the dish is divided into."`
	PotG     float64     `json:"pot_g,omitempty" minimum:"0" maximum:"10000" doc:"The empty pot's weight in g, kept for the next weighing."`
	Note     string      `json:"note,omitempty" maxLength:"2000"`
}

// recipeBody is a recipe of a food, beside the food's fields.
type recipeBody struct {
	Weight    float64          `json:"weight" doc:"The cooked dish's weight in g: yield_g, or the raw ingredients' when not weighed. The serving recipe is the whole dish, portion a portion of it."`
	YieldG    float64          `json:"yield_g,omitempty" doc:"The cooked dish's weight as weighed; absent when not weighed: a share of the dish is exact, its grams not."`
	RawWeight float64          `json:"raw_weight,omitempty" doc:"The raw ingredients' weight in g (ml as g); absent when one's is not known."`
	Portions  int              `json:"portions,omitempty" doc:"How many portions the dish is divided into."`
	PotG      float64          `json:"pot_g,omitempty" doc:"The empty pot's weight in g."`
	Total     nutrientsBody    `json:"total" doc:"The whole dish's."`
	Portion   *nutrientsBody   `json:"portion,omitempty" doc:"A portion's, when divided."`
	Items     []ingredientBody `json:"items,omitempty" nullable:"false" doc:"The ingredients, with the values each brought when the recipe was saved; get_food gives them."`
}

// ingredientBody is one of a recipe's ingredients.
type ingredientBody struct {
	FoodID    string        `json:"food_id" format:"uuid"`
	Name      string        `json:"name" doc:"The food's name when the recipe was saved."`
	Brand     string        `json:"brand,omitempty"`
	Quantity  quantityBody  `json:"quantity" doc:"How much, as it was given."`
	Amount    float64       `json:"amount" doc:"How much, in unit."`
	Unit      string        `json:"unit" enum:"g,ml,portion"`
	Nutrients nutrientsBody `json:"nutrients" doc:"What it brought when the recipe was saved."`
	Source    string        `json:"source" enum:"label,estimate,user,dish,off,recipe"`
	Note      string        `json:"note,omitempty"`
	Changed   bool          `json:"changed,omitempty" doc:"Its food's values give another amount now: the recipe saved again takes them."`
}

// newRecipeBody is r's body; with its ingredients when items, each marked
// changed when changed says so.
func newRecipeBody(r *recipe.Recipe, items bool, changed map[int]bool) *recipeBody {
	w, err := r.Weight()
	if err != nil {
		w = 0
	}
	total := r.Total()
	b := &recipeBody{Weight: w, YieldG: r.Yield, Portions: r.Portions, PotG: r.Pot, Total: newNutrientsBody(total)}
	if raw, ok := r.RawWeight(); ok {
		b.RawWeight = raw
	}
	if r.Portions > 0 {
		b.Portion = new(newNutrientsBody(total.Scale(1 / float64(r.Portions))))
	}
	if !items {
		return b
	}
	for i := range r.Items {
		it := &r.Items[i]
		b.Items = append(b.Items, ingredientBody{
			FoodID: it.FoodID.String(), Name: it.Name, Brand: it.Brand, Amount: it.Amount, Unit: it.Unit,
			Quantity: quantityBody{
				Amount: it.Quantity.Amount, Unit: it.Quantity.Unit,
				Servings: it.Quantity.Servings, Serving: it.Quantity.Serving, Portions: it.Quantity.Portions,
			},
			Nutrients: newNutrientsBody(it.Nutrients), Source: string(it.Source), Note: it.Note, Changed: changed[i],
		})
	}
	return b
}

// withRecipes gives each body of foods that is a recipe's its recipe,
// without its ingredients: what a search or a list says of it.
func (s *sessions) withRecipes(ctx context.Context, p auth.Principal, foods []food.Food, bodies []foodBody) error {
	var ids []uuid.UUID
	for i := range foods {
		if foods[i].Source == food.Recipe {
			ids = append(ids, foods[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rs, err := s.db.RecipesOf(ctx, p.User.ID, ids)
	if err != nil {
		return err
	}
	for i := range foods {
		if r, ok := rs[foods[i].ID]; ok {
			bodies[i].Recipe = newRecipeBody(&r, false, nil)
		}
	}
	return nil
}

// recipeFoodBody is the body of r's food, with r whole: each ingredient
// marked changed whose food gives another amount now.
func (s *sessions) recipeFoodBody(ctx context.Context, p auth.Principal, r *recipe.Recipe) (foodBody, error) {
	changed := map[int]bool{}
	for i := range r.Items {
		it := &r.Items[i]
		f, err := s.db.FoodByID(ctx, p.User.ID, it.FoodID)
		if err != nil {
			return foodBody{}, err
		}
		// A recipe as an ingredient reweighed brings as much, but weighs
		// another amount: the raw weight it adds changes, as it does for a
		// portion whose weight was corrected.
		now, err := meal.FromFood(f, it.Quantity, it.Note)
		changed[i] = err != nil || math.Abs(now.Amount-it.Amount) > 1e-9 || now.Unit != it.Unit ||
			(now.Unit == "portion" && now.PortionGrams != it.PortionGrams) ||
			!reflect.DeepEqual(now.Nutrients.Rounded(), it.Nutrients.Rounded())
	}
	b := newFoodBody(&r.Food)
	b.Recipe = newRecipeBody(r, true, changed)
	return b, nil
}

type upsertRecipeInput struct {
	Body struct {
		ID string `json:"id,omitempty" format:"uuid" doc:"The recipe to correct; without it, the recipe of the name, or a new one."`
		RecipeFields
		Archived bool `json:"archived,omitempty" doc:"Archives the recipe's food; left out, a save brings it back."`
	}
}

type createRecipeInput struct {
	Body RecipeFields
}

// recipeAnswerBody is the answer of a save of a recipe: its food, with
// the recipe whole, and warnings to look at.
type recipeAnswerBody struct {
	Food     foodBody            `json:"food" doc:"The recipe's food, with its recipe."`
	Warnings []recipeWarningBody `json:"warnings" nullable:"false"`
}

// recipeWarningBody is something about a saved recipe to look at, usually
// with the user: the recipe is saved anyway.
type recipeWarningBody struct {
	Code    string `json:"code" enum:"atwater,saturated_fat,sugars,polyols,food_kept,yield_low,yield_high" doc:"What kind of warning it is: of a new food's values, or yield_low and yield_high of a weight far off the raw ingredients'."`
	Message string `json:"message"`
}

type recipeOutput struct {
	Body recipeAnswerBody
}

type createdRecipeOutput struct {
	Body recipeAnswerBody
}

// upsertRecipe saves a recipe, or corrects one: by its id, or else its
// name; else a new one.
func (s *sessions) upsertRecipe(ctx context.Context, in *upsertRecipeInput) (*recipeOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	b := &in.Body
	r, foods, warnings, err := s.sentRecipe(ctx, p, &b.RecipeFields)
	if err != nil {
		return nil, err
	}
	r.Food.Archived = b.Archived
	save := s.db.SaveRecipe
	if b.ID != "" {
		// An id corrects a recipe the user has: a mistyped or made-up one
		// makes no duplicate.
		if r.Food.ID, err = uuid.Parse(b.ID); err != nil {
			return nil, huma.Error422UnprocessableEntity("the id: " + err.Error())
		}
		save = s.db.CorrectRecipe
	}
	saved, err := save(ctx, p.User.ID, actorOf(p), r, foods, s.now())
	if err != nil {
		return nil, recipeError(err, &b.RecipeFields)
	}
	body, err := s.recipeAnswer(ctx, p, &saved, warnings)
	if err != nil {
		return nil, internalError("saving the recipe failed", err)
	}
	return &recipeOutput{Body: body}, nil
}

// createRecipe saves a new recipe, and only that: a food of its name,
// archived too, is in the way (409). The web app's "New recipe" makes one.
func (s *sessions) createRecipe(ctx context.Context, in *createRecipeInput) (*createdRecipeOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	r, foods, warnings, err := s.sentRecipe(ctx, p, &in.Body)
	if err != nil {
		return nil, err
	}
	saved, err := s.db.CreateRecipe(ctx, p.User.ID, actorOf(p), r, foods, s.now())
	if err != nil {
		return nil, recipeError(err, &in.Body)
	}
	body, err := s.recipeAnswer(ctx, p, &saved, warnings)
	if err != nil {
		return nil, internalError("saving the recipe failed", err)
	}
	return &createdRecipeOutput{Body: body}, nil
}

// sentRecipe is the recipe b sends, under a new id, worked out of its
// ingredients as they are (the store works it out again in its save),
// with the new foods it brings and warnings about it and their values.
func (s *sessions) sentRecipe(ctx context.Context, p auth.Principal, b *RecipeFields) (recipe.Recipe, []food.Food, []meal.Warning, error) {
	sent, foods, warnings, err := sentItems(b.Items)
	if err != nil {
		return recipe.Recipe{}, nil, nil, err
	}
	for i := range sent {
		if sent[i].FoodID == uuid.Nil() && foods[i] == nil {
			return recipe.Recipe{}, nil, nil, itemError(i, huma.Error422UnprocessableEntity(
				"an ingredient is a food, by food_id or a new one, with its quantity: no estimate"))
		}
	}
	items, ws, err := s.items(ctx, p, sent, foods)
	if err != nil {
		return recipe.Recipe{}, nil, nil, err
	}
	r := recipe.Recipe{
		Food:  food.Food{ID: uuid.NewV7(), Name: b.Name, Aliases: b.Aliases, Note: b.Note},
		Items: items, Yield: b.YieldG, Portions: b.Portions, Pot: b.PotG,
	}
	made, err := recipe.Make(r)
	if err != nil {
		return recipe.Recipe{}, nil, nil, huma.Error422UnprocessableEntity("the recipe: " + oneLine(err))
	}
	warnings = append(append(warnings, ws...), made.Warnings()...)
	return r, newFoods(foods), warnings, nil
}

// recipeAnswer is the answer of a save of r, with warnings.
func (s *sessions) recipeAnswer(ctx context.Context, p auth.Principal, r *recipe.Recipe, warnings []meal.Warning) (recipeAnswerBody, error) {
	b, err := s.recipeFoodBody(ctx, p, r)
	if err != nil {
		return recipeAnswerBody{}, err
	}
	out := recipeAnswerBody{Food: b, Warnings: make([]recipeWarningBody, 0, len(warnings))}
	for _, w := range warnings {
		out.Warnings = append(out.Warnings, recipeWarningBody{Code: w.Code, Message: w.Message})
	}
	return out, nil
}

// recipeError is the answer to a save of the recipe sent's err.
func recipeError(err error, sent *RecipeFields) error {
	var conflict *store.FoodConflictError
	var fie *store.FoodItemError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return huma.Error404NotFound("no such recipe: a new recipe comes without an id")
	case errors.As(err, &fie):
		return foodItemError(fie)
	case errors.As(err, &conflict):
		return huma.Error409Conflict(conflictDetail(conflict), conflictError(conflict, &FoodFields{Name: sent.Name, Aliases: sent.Aliases}))
	case errors.Is(err, store.ErrRecipeCycle):
		return huma.Error422UnprocessableEntity("the recipe: " + oneLine(err) + ": a recipe is never its own ingredient")
	case errors.Is(err, store.ErrInvalidRecipe):
		return huma.Error422UnprocessableEntity("the recipe: " + oneLine(err))
	case errors.Is(err, store.ErrRecipeFood):
		return errRecipeFood
	}
	return internalError("saving the recipe failed", err)
}

// recipeTool is upsert_recipe, a tool.
func (s *sessions) recipeTool() tool {
	return newTool("upsert_recipe", []auth.Scope{auth.ScopeTools}, huma.Operation{
		OperationID: "upsert-recipe",
		Method:      http.MethodPost,
		Path:        Prefix + "/recipes",
		Summary:     "Save a recipe",
		Description: "Saves a recipe, a dish cooked again, or corrects one: the recipe with the id (404 for an id the user has no recipe of), or else the one of the name; else a new one. " +
			"A recipe is a food (source recipe, by weight), found and logged as any: its values per 100 g of the cooked dish come from its ingredients and the dish's weight (yield_g), " +
			"and its servings are recipe, the whole dish, and portion, one of its portions: a tenth of the pot is 0.1 of recipe. " +
			"Not weighed, the raw ingredients' weight stands for the dish's. A save replaces the whole recipe, its ingredients taking their foods' values now: " +
			"correct it from what get_food gave. A food of the name that is no recipe, or an alias another food has, is refused (409), naming it; " +
			"so is an ingredient that is the recipe, at any depth (422). The answer holds its food with the recipe, and warnings: a weight far off the raw ingredients', or values of a new food that do not add up.",
		Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
	}, s.upsertRecipe).
		briefly("Saves a recipe, or corrects one: by its id, or else its name; else a new one. Its ingredients are log_meal's items, no estimates; " +
			"a save replaces it whole, so correct from what get_food gave. A recipe is a food: its servings recipe (the whole dish) and portion. " +
			"Answers its food with the recipe, and warnings.").destroys()
}

func (s *sessions) registerRecipes(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "create-recipe",
		Method:      http.MethodPost,
		Path:        Prefix + "/recipes/new",
		Summary:     "Make a new recipe",
		Description: "Needs a web session. Saves a new recipe: a food you have of its name, archived too, is refused (409), naming it, " +
			"and so is an alias another food has; upsert_recipe corrects a recipe instead. Answers as upsert_recipe does.",
		Tags:          []string{"foods"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity},
	}, s.createRecipe)
}
