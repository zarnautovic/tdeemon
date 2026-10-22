package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// clientOf is what a meal of the request of p is logged through: the web
// app with a session, the API with a token.
func clientOf(p auth.Principal) meal.Client {
	if p.Via == auth.ViaSession {
		return meal.ClientWeb
	}
	return meal.ClientAPI
}

// itemInput is one thing eaten, as a client gives it: a food of the
// user's by its id, or a new food, with how much; or a one-off estimate
// of a whole plate.
type itemInput struct {
	ID       string      `json:"id,omitempty" format:"uuid" doc:"The client's id for the item; a new one when left out."`
	FoodID   string      `json:"food_id,omitempty" format:"uuid" doc:"A food of the user's, from find_foods."`
	Food     *FoodFields `json:"food,omitempty" doc:"A new food, saved as it is logged: one of the same name and brand is that food, corrected."`
	Amount   float64     `json:"amount,omitempty" minimum:"0" doc:"How much of the food, in unit: the food's own (g or ml)."`
	Unit     string      `json:"unit,omitempty" enum:"g,ml"`
	Servings float64     `json:"servings,omitempty" minimum:"0" doc:"How many of the food's serving."`
	Serving  string      `json:"serving,omitempty" doc:"A serving the food names: a slice, a piece."`
	Portions float64     `json:"portions,omitempty" minimum:"0" doc:"How many portions of a food per portion."`
	// Name and Nutrients make an estimate.
	Name      string             `json:"name,omitempty" maxLength:"200" doc:"A one-off estimate of a whole plate: its name, with its nutrients."`
	Nutrients *foodNutrientsBody `json:"nutrients,omitempty" doc:"The estimate's values, for the whole plate."`
	Note      string             `json:"note,omitempty" maxLength:"2000"`
}

// errNotItem is the 422 of an item of no kind, or of two.
var errNotItem = errors.New("an item is one of: a food_id with its quantity, a food with its quantity, or an estimate (name and nutrients)")

// item works out the item in: its food (saved first when new) and the
// nutrients its quantity brings, or the estimate; with warnings about
// values a client stated that do not add up.
func (s *sessions) item(ctx context.Context, p auth.Principal, in *itemInput) (meal.Item, []meal.Warning, error) {
	kinds := 0
	for _, given := range []bool{in.FoodID != "", in.Food != nil, in.Name != "" || in.Nutrients != nil} {
		if given {
			kinds++
		}
	}
	// None at all fails as an estimate without its values.
	if kinds > 1 {
		return meal.Item{}, nil, huma.Error422UnprocessableEntity(errNotItem.Error())
	}
	q := meal.Quantity{Amount: in.Amount, Unit: in.Unit, Servings: in.Servings, Serving: in.Serving, Portions: in.Portions}
	var it meal.Item
	var warnings []meal.Warning
	var err error
	switch {
	case in.FoodID != "":
		it, err = s.foodItem(ctx, p, in, q)
	case in.Food != nil:
		it, warnings, err = s.newFoodItem(ctx, p, in, q)
	default:
		it, warnings, err = estimateItem(in, q)
	}
	if err != nil {
		return meal.Item{}, nil, err
	}
	if in.ID != "" {
		if it.ID, err = uuid.Parse(in.ID); err != nil {
			return meal.Item{}, nil, huma.Error422UnprocessableEntity("the item's id: " + err.Error())
		}
	}
	return it, warnings, nil
}

// estimateItem is the item of an estimate of a whole plate, with warnings
// about its values.
func estimateItem(in *itemInput, q meal.Quantity) (meal.Item, []meal.Warning, error) {
	if in.Name == "" || in.Nutrients == nil {
		return meal.Item{}, nil, huma.Error422UnprocessableEntity(errNotItem.Error())
	}
	if q != (meal.Quantity{}) {
		return meal.Item{}, nil, huma.Error422UnprocessableEntity("an estimate is the whole plate: give its values, without a quantity")
	}
	n := in.Nutrients.nutrients()
	it, err := meal.Estimate(in.Name, n, in.Note)
	if err != nil {
		return meal.Item{}, nil, huma.Error422UnprocessableEntity(oneLine(err))
	}
	return it, meal.CheckNutrients(in.Name, n), nil
}

// foodItem is the item of quantity q of the user's food in.FoodID.
func (s *sessions) foodItem(ctx context.Context, p auth.Principal, in *itemInput, q meal.Quantity) (meal.Item, error) {
	f, err := s.foodOf(ctx, p, in.FoodID)
	if err != nil {
		return meal.Item{}, err
	}
	it, err := meal.FromFood(f, q, in.Note)
	if err != nil {
		return meal.Item{}, huma.Error422UnprocessableEntity(oneLine(err))
	}
	return it, nil
}

// newFoodItem saves the item's new food and is the item of quantity q of
// it, with warnings about the food's values.
func (s *sessions) newFoodItem(ctx context.Context, p auth.Principal, in *itemInput, q meal.Quantity) (meal.Item, []meal.Warning, error) {
	f, warnings, err := s.saveNewFood(ctx, p, in.Food)
	if err != nil {
		return meal.Item{}, nil, err
	}
	it, err := meal.FromFood(f, q, in.Note)
	if err != nil {
		return meal.Item{}, nil, huma.Error422UnprocessableEntity(oneLine(err))
	}
	return it, warnings, nil
}

// foodOf returns the user's food with id, or a 422 naming the item's
// food, as the meal is what the request is about.
func (s *sessions) foodOf(ctx context.Context, p auth.Principal, id string) (food.Food, error) {
	fid, err := uuid.Parse(id)
	if err != nil {
		return food.Food{}, huma.Error422UnprocessableEntity("no such food: " + id)
	}
	f, err := s.db.FoodByID(ctx, p.User.ID, fid)
	if errors.Is(err, store.ErrNotFound) {
		return food.Food{}, huma.Error422UnprocessableEntity("no such food: " + id + "; find_foods gives the user's foods")
	}
	if err != nil {
		return food.Food{}, internalError("reading a food failed", err)
	}
	return f, nil
}

// saveNewFood saves the food an item brings, by the rule of the same name
// and brand, and returns it as stored: an item must take its id from the
// save, so a retry of the meal finds the same food (DESIGN.md §9).
func (s *sessions) saveNewFood(ctx context.Context, p auth.Principal, in *FoodFields) (food.Food, []meal.Warning, error) {
	f := food.Food{
		ID: uuid.NewV7(), Name: in.Name, Brand: in.Brand, Basis: food.Basis(in.Basis), Nutrients: in.Nutrients.nutrients(),
		PortionGrams: in.PortionGrams, Aliases: in.Aliases, Source: food.Source(in.Source), Note: in.Note,
	}
	for _, sv := range in.Servings {
		f.Servings = append(f.Servings, food.Serving{Name: sv.Name, Amount: sv.Amount})
	}
	if err := f.Validate(); err != nil {
		return food.Food{}, nil, huma.Error422UnprocessableEntity("the new food: " + oneLine(err))
	}
	saved, err := s.db.SaveFood(ctx, p.User.ID, actorOf(p), f, s.now())
	var conflict *store.FoodConflictError
	switch {
	case errors.As(err, &conflict):
		return food.Food{}, nil, huma.Error409Conflict(conflictDetail(conflict))
	case err != nil:
		return food.Food{}, nil, internalError("saving the food failed", err)
	}
	return saved, meal.CheckNutrients(saved.Name, saved.Nutrients), nil
}

// oneLine is err's text on one line: a domain error joins its parts with
// line breaks.
func oneLine(err error) string { return strings.ReplaceAll(err.Error(), "\n", "; ") }

// warningBody is something about a logged meal to look at, usually with
// the user: the meal is stored anyway.
type warningBody struct {
	Code    string `json:"code" enum:"atwater,saturated_fat,sugars,polyols,duplicate_meal" doc:"What kind of warning it is."`
	Message string `json:"message"`
}

func newWarningBodies(ws []meal.Warning) []warningBody {
	out := make([]warningBody, 0, len(ws))
	for _, w := range ws {
		out = append(out, warningBody{Code: w.Code, Message: w.Message})
	}
	return out
}

// mealAnswerBody is the answer of a tool that writes a meal: the meal as
// stored, with what each item brought, where its day stands now, and
// warnings to look at.
type mealAnswerBody struct {
	Meal     mealBody      `json:"meal"`
	Day      DaySummary    `json:"day" doc:"The meal's day, with the meal."`
	Warnings []warningBody `json:"warnings" nullable:"false"`
}

type mealOutput struct {
	Status int
	Body   mealAnswerBody
}

type logMealInput struct {
	Body logMealBody
}

// logMealBody is a meal as log_meal takes it.
type logMealBody struct {
	ID          string      `json:"id" format:"uuid" doc:"A UUIDv7 the client makes when the meal is logged, no more than a day before and five minutes ahead of the clock: the same meal sent again under it is a retry."`
	EatenAt     *time.Time  `json:"eaten_at,omitempty" doc:"When the meal was eaten; the time of the id when left out, so a retry has the same."`
	Description string      `json:"description,omitempty" maxLength:"200" doc:"What the meal was, in the user's words."`
	Items       []itemInput `json:"items" minItems:"1" maxItems:"50"`
	Note        string      `json:"note,omitempty" maxLength:"2000"`
}

func (s *sessions) logMeal(ctx context.Context, in *logMealInput) (*mealOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	m, cal, err := s.newMeal(ctx, p, &in.Body, now)
	if err != nil {
		return nil, err
	}
	var warnings []meal.Warning
	if m.Items, warnings, err = s.items(ctx, p, in.Body.Items); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity("the meal: " + oneLine(err))
	}
	stored, created, err := s.db.CreateMeal(ctx, p.User.ID, actorOf(p), m, now)
	if err != nil {
		return nil, createError(err)
	}
	if w, ok, err := s.duplicateOf(ctx, p, stored, cal); err != nil {
		return nil, internalError("logging the meal failed", err)
	} else if ok {
		warnings = append(warnings, w)
	}
	out, err := s.mealAnswer(ctx, p, stored, warnings, now)
	if err != nil {
		return nil, internalError("logging the meal failed", err)
	}
	out.Status = http.StatusOK
	if created {
		out.Status = http.StatusCreated
	}
	return out, nil
}

// newMeal is the meal b logs, without its items, and the user's calendar.
// An id already stored is a retry, which its fingerprint decides; a new
// one must be made now (meal.CheckNewID). The meal was eaten at the id's
// time unless b says when.
func (s *sessions) newMeal(ctx context.Context, p auth.Principal, b *logMealBody, now time.Time) (meal.Meal, day.Calendar, error) {
	id, err := uuid.Parse(b.ID)
	if err != nil {
		return meal.Meal{}, day.Calendar{}, huma.Error422UnprocessableEntity("the meal's id: " + err.Error())
	}
	if _, err := s.db.MealByID(ctx, p.User.ID, id); errors.Is(err, store.ErrNotFound) {
		if err := meal.CheckNewID(id, now); err != nil {
			return meal.Meal{}, day.Calendar{}, huma.Error422UnprocessableEntity(err.Error())
		}
	} else if err != nil {
		return meal.Meal{}, day.Calendar{}, internalError("logging the meal failed", err)
	}
	cal, _, err := userDay(p, now)
	if err != nil {
		return meal.Meal{}, day.Calendar{}, internalError("logging the meal failed", err)
	}
	m := meal.Meal{ID: id, Description: b.Description, Client: clientOf(p), Note: b.Note}
	if b.EatenAt != nil {
		m.EatenAt = b.EatenAt.UTC()
	} else {
		m.EatenAt, _ = meal.IDTime(id)
	}
	m.Day = cal.DayOf(m.EatenAt)
	return m, cal, nil
}

// items works out the items in, with their warnings.
func (s *sessions) items(ctx context.Context, p auth.Principal, in []itemInput) ([]meal.Item, []meal.Warning, error) {
	var items []meal.Item
	var warnings []meal.Warning
	for i := range in {
		it, ws, err := s.item(ctx, p, &in[i])
		if err != nil {
			return nil, nil, itemError(i, err)
		}
		items = append(items, it)
		warnings = append(warnings, ws...)
	}
	return items, warnings, nil
}

// createError is the answer to CreateMeal's err.
func createError(err error) error {
	switch {
	case errors.Is(err, store.ErrMealConflict):
		return huma.Error409Conflict("another meal is under this id: a new meal takes a new id")
	case errors.Is(err, store.ErrMealDeleted):
		return huma.Error409Conflict("the meal under this id was deleted: a new meal takes a new id")
	}
	return internalError("logging the meal failed", err)
}

// itemError says which item err is about, when it is a 422 of the item's
// own.
func itemError(i int, err error) error {
	var e *huma.ErrorModel
	if errors.As(err, &e) && e.Status == http.StatusUnprocessableEntity {
		return huma.Error422UnprocessableEntity(fmt.Sprintf("item %d: %s", i+1, e.Detail))
	}
	return err
}

// duplicateOf warns when m looks like a meal of the user's logged within
// minutes of it, sent twice.
func (s *sessions) duplicateOf(ctx context.Context, p auth.Principal, m meal.Meal, cal day.Calendar) (meal.Warning, bool, error) {
	recent, err := s.db.MealsBetween(ctx, p.User.ID, m.EatenAt.Add(-meal.DuplicateWindow), m.EatenAt.Add(meal.DuplicateWindow+time.Second))
	if err != nil {
		return meal.Warning{}, false, err
	}
	w, ok := meal.DuplicateOf(m, recent, cal.Location)
	return w, ok, nil
}

// mealAnswer is the answer about m, as stored: the meal, where its day
// stands at now, and warnings.
func (s *sessions) mealAnswer(ctx context.Context, p auth.Principal, m meal.Meal, warnings []meal.Warning, now time.Time) (*mealOutput, error) {
	d, err := s.readDay(ctx, p, m.Day, now)
	if err != nil {
		return nil, err
	}
	return &mealOutput{Body: mealAnswerBody{Meal: newMealBody(&m), Day: d.DaySummary, Warnings: newWarningBodies(warnings)}}, nil
}

// mealOp is one change of a meal's items.
type mealOp struct {
	Op     string     `json:"op" enum:"add,replace,remove" doc:"add an item; replace item_id with item; remove item_id."`
	ItemID string     `json:"item_id,omitempty" format:"uuid" doc:"The item to replace or remove."`
	Item   *itemInput `json:"item,omitempty" doc:"The item to add, or to put in the place of item_id."`
}

type updateMealInput struct {
	ID   string `path:"id" format:"uuid"`
	Body updateMealBody
}

// updateMealBody is the change of a meal update_meal takes.
type updateMealBody struct {
	EatenAt     *time.Time `json:"eaten_at,omitempty" doc:"When the meal was eaten, which may move it to another day."`
	Description *string    `json:"description,omitempty" maxLength:"200"`
	Note        *string    `json:"note,omitempty" maxLength:"2000"`
	Ops         []mealOp   `json:"ops,omitempty" nullable:"false" maxItems:"50" doc:"The changes of the items, in order."`
}

// mealID is the meal id of a path, or the 404 of a meal that does not
// exist.
func mealID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil(), huma.Error404NotFound("no such meal")
	}
	return id, nil
}

func (s *sessions) updateMeal(ctx context.Context, in *updateMealInput) (*mealOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	id, err := mealID(in.ID)
	if err != nil {
		return nil, err
	}
	m, err := s.db.MealByID(ctx, p.User.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such meal")
	} else if err != nil {
		return nil, internalError("changing the meal failed", err)
	}
	warnings, err := s.edit(ctx, p, &m, &in.Body, now)
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity("the meal: " + oneLine(err))
	}
	stored, err := s.db.ReplaceMeal(ctx, p.User.ID, actorOf(p), m, now)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, huma.Error404NotFound("no such meal")
	case errors.Is(err, store.ErrMealConflict):
		return nil, huma.Error409Conflict("an item's id is an item of another meal: a new item takes a new id")
	case err != nil:
		return nil, internalError("changing the meal failed", err)
	}
	out, err := s.mealAnswer(ctx, p, stored, warnings, now)
	if err != nil {
		return nil, internalError("changing the meal failed", err)
	}
	out.Status = http.StatusOK
	return out, nil
}

// edit changes m as b says: its time, which may move it to another day,
// its words, and its items, op by op; with the warnings of the items
// added.
func (s *sessions) edit(ctx context.Context, p auth.Principal, m *meal.Meal, b *updateMealBody, now time.Time) ([]meal.Warning, error) {
	if b.EatenAt != nil {
		cal, _, err := userDay(p, now)
		if err != nil {
			return nil, internalError("changing the meal failed", err)
		}
		m.EatenAt = b.EatenAt.UTC()
		m.Day = cal.DayOf(m.EatenAt)
	}
	if b.Description != nil {
		m.Description = *b.Description
	}
	if b.Note != nil {
		m.Note = *b.Note
	}
	var warnings []meal.Warning
	for i := range b.Ops {
		ws, err := s.applyOp(ctx, p, m, &b.Ops[i])
		if err != nil {
			return nil, opError(i, err)
		}
		warnings = append(warnings, ws...)
	}
	return warnings, nil
}

// applyOp changes m's items as op says. An item op leaves keeps what it
// brought when it was logged: a food corrected since changes no meal
// already eaten. An add of an item m has under its id is a retry when it
// is the same (meal.Item.SameAs), a conflict when it is not; a remove of
// an item m does not have is one done already.
func (s *sessions) applyOp(ctx context.Context, p auth.Principal, m *meal.Meal, op *mealOp) ([]meal.Warning, error) {
	at := slices.IndexFunc(m.Items, func(it meal.Item) bool { return op.ItemID != "" && it.ID.String() == op.ItemID })
	if err := checkOp(op, at); err != nil {
		return nil, err
	}
	if op.Op == "remove" {
		if at >= 0 {
			m.Items = slices.Delete(m.Items, at, at+1)
		}
		return nil, nil
	}
	it, warnings, err := s.item(ctx, p, op.Item)
	if err != nil {
		return nil, err
	}
	if op.Op == "replace" {
		it.ID = m.Items[at].ID
		m.Items[at] = it
		return warnings, nil
	}
	if i := slices.IndexFunc(m.Items, func(had meal.Item) bool { return it.ID != uuid.Nil() && had.ID == it.ID }); i >= 0 {
		if m.Items[i].SameAs(it) {
			return nil, nil
		}
		return nil, huma.Error409Conflict("the meal has another item under the id " + it.ID.String() + ": a new item takes a new id")
	}
	m.Items = append(m.Items, it)
	return warnings, nil
}

// checkOp refuses an op of the wrong shape, or a replace of an item the
// meal does not have (at < 0).
func checkOp(op *mealOp, at int) error {
	switch {
	case op.Op == "add" && (op.Item == nil || op.ItemID != ""):
		return huma.Error422UnprocessableEntity("an add takes an item, with its id inside it, and no item_id")
	case op.Op == "remove" && (op.ItemID == "" || op.Item != nil):
		return huma.Error422UnprocessableEntity("a remove takes an item_id, and no item")
	case op.Op == "replace" && (op.ItemID == "" || op.Item == nil):
		return huma.Error422UnprocessableEntity("a replace takes an item_id and the item to put in its place")
	case op.Op == "replace" && at < 0:
		return huma.Error422UnprocessableEntity("the meal has no item " + op.ItemID)
	}
	return nil
}

// opError says which op err is about, when it is a 422 of the op's own.
func opError(i int, err error) error {
	var e *huma.ErrorModel
	if errors.As(err, &e) && e.Status == http.StatusUnprocessableEntity {
		return huma.Error422UnprocessableEntity(fmt.Sprintf("op %d: %s", i+1, e.Detail))
	}
	return err
}

type deleteMealInput struct {
	ID string `path:"id" format:"uuid"`
}

type deleteMealOutput struct {
	Body struct {
		Day DaySummary `json:"day" doc:"The meal's day, without the meal."`
	}
}

func (s *sessions) deleteMeal(ctx context.Context, in *deleteMealInput) (*deleteMealOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	id, err := mealID(in.ID)
	if err != nil {
		return nil, err
	}
	m, err := s.db.MealByID(ctx, p.User.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such meal")
	} else if err != nil {
		return nil, internalError("deleting the meal failed", err)
	}
	if err := s.db.DeleteMeal(ctx, p.User.ID, actorOf(p), id, now); errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such meal")
	} else if err != nil {
		return nil, internalError("deleting the meal failed", err)
	}
	d, err := s.readDay(ctx, p, m.Day, now)
	if err != nil {
		return nil, internalError("deleting the meal failed", err)
	}
	out := &deleteMealOutput{}
	out.Body.Day = d.DaySummary
	return out, nil
}
