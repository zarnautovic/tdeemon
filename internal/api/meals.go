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

// itemMode is what item does with an item's new food.
type itemMode int

const (
	// validate checks the new food and works out the item from it as
	// sent, saving nothing: a meal is checked whole before anything of it
	// is saved.
	validate itemMode = iota
	// retry takes the food of the new food's name and brand as it is
	// stored, saving nothing: a request sent again saves no food again,
	// so one corrected since stays corrected.
	retry
	// save saves the new food and works out the item from it as saved.
	save
)

// item works out the item in: its food (a new one as mode says) and the
// nutrients its quantity brings, or the estimate; with warnings about
// values a client stated that do not add up.
func (s *sessions) item(ctx context.Context, p auth.Principal, in *itemInput, mode itemMode) (meal.Item, []meal.Warning, error) {
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
		it, warnings, err = s.newFoodItem(ctx, p, in, q, mode)
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

// newFoodItem is the item of quantity q of the item's new food, which
// mode says what to do with, with warnings about the food's values. The
// item takes the food's id from the save, so a retry of the meal finds
// the same food (DESIGN.md §9).
func (s *sessions) newFoodItem(ctx context.Context, p auth.Principal, in *itemInput, q meal.Quantity, mode itemMode) (meal.Item, []meal.Warning, error) {
	f, err := newFood(in.Food)
	if err != nil {
		return meal.Item{}, nil, err
	}
	warnings := meal.CheckNutrients(f.Name, f.Nutrients)
	switch mode {
	case save:
		if f, err = s.saveFood(ctx, p, f); err != nil {
			return meal.Item{}, nil, err
		}
	case retry:
		stored, err := s.db.FoodByKey(ctx, p.User.ID, f.Name, f.Brand)
		if err == nil {
			f = stored
		} else if !errors.Is(err, store.ErrNotFound) {
			return meal.Item{}, nil, internalError("reading a food failed", err)
		}
	case validate:
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

// newFood is the food in, checked, under a new id.
func newFood(in *FoodFields) (food.Food, error) {
	f := food.Food{
		ID: uuid.NewV7(), Name: in.Name, Brand: in.Brand, Basis: food.Basis(in.Basis), Nutrients: in.Nutrients.nutrients(),
		PortionGrams: in.PortionGrams, Aliases: in.Aliases, Source: food.Source(in.Source), Note: in.Note,
	}
	for _, sv := range in.Servings {
		f.Servings = append(f.Servings, food.Serving{Name: sv.Name, Amount: sv.Amount})
	}
	if err := f.Validate(); err != nil {
		return food.Food{}, huma.Error422UnprocessableEntity("the new food: " + oneLine(err))
	}
	return f, nil
}

// saveFood saves f by the rule of the same name and brand, and returns it
// as stored.
func (s *sessions) saveFood(ctx context.Context, p auth.Principal, f food.Food) (food.Food, error) {
	saved, err := s.db.SaveFood(ctx, p.User.ID, actorOf(p), f, s.now())
	var conflict *store.FoodConflictError
	switch {
	case errors.As(err, &conflict):
		return food.Food{}, huma.Error409Conflict(conflictDetail(conflict))
	case err != nil:
		return food.Food{}, internalError("saving the food failed", err)
	}
	return saved, nil
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
	m, cal, used, err := s.newMeal(ctx, p, &in.Body, now)
	if err != nil {
		return nil, err
	}
	// The meal whole is checked before any of its new foods is saved;
	// a meal sent again saves none.
	mode := validate
	if used {
		mode = retry
	}
	var warnings []meal.Warning
	if m.Items, warnings, err = s.items(ctx, p, in.Body.Items, mode); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity("the meal: " + oneLine(err))
	}
	if !used {
		if m.Items, warnings, err = s.items(ctx, p, in.Body.Items, save); err != nil {
			return nil, err
		}
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

// newMeal is the meal b logs, without its items, the user's calendar, and
// whether the id is used already: an id of a meal stored, or deleted, is
// a retry, which the fingerprint decides; a new one must be made now
// (meal.CheckNewID). The meal was eaten at the id's time unless b says
// when.
func (s *sessions) newMeal(ctx context.Context, p auth.Principal, b *logMealBody, now time.Time) (meal.Meal, day.Calendar, bool, error) {
	id, err := uuid.Parse(b.ID)
	if err != nil {
		return meal.Meal{}, day.Calendar{}, false, huma.Error422UnprocessableEntity("the meal's id: " + err.Error())
	}
	used, err := s.db.MealIDUsed(ctx, p.User.ID, id)
	if err != nil {
		return meal.Meal{}, day.Calendar{}, false, internalError("logging the meal failed", err)
	}
	if !used {
		if err := meal.CheckNewID(id, now); err != nil {
			return meal.Meal{}, day.Calendar{}, false, huma.Error422UnprocessableEntity(err.Error())
		}
	}
	cal, _, err := userDay(p, now)
	if err != nil {
		return meal.Meal{}, day.Calendar{}, false, internalError("logging the meal failed", err)
	}
	m := meal.Meal{ID: id, Description: b.Description, Client: clientOf(p), Note: b.Note}
	if b.EatenAt != nil {
		m.EatenAt = b.EatenAt.UTC()
	} else {
		m.EatenAt, _ = meal.IDTime(id)
	}
	m.Day = cal.DayOf(m.EatenAt)
	return m, cal, used, nil
}

// items works out the items in, with their warnings.
func (s *sessions) items(ctx context.Context, p auth.Principal, in []itemInput, mode itemMode) ([]meal.Item, []meal.Warning, error) {
	var items []meal.Item
	var warnings []meal.Warning
	for i := range in {
		it, ws, err := s.item(ctx, p, &in[i], mode)
		if err != nil {
			return nil, nil, itemError(i, err)
		}
		items = append(items, it)
		warnings = append(warnings, ws...)
	}
	return items, warnings, nil
}

// errItemTaken is the 409 of an item under the id of another meal's item.
var errItemTaken = huma.Error409Conflict("an item's id is another meal's item: a new item takes a new id")

// createError is the answer to CreateMeal's err.
func createError(err error) error {
	switch {
	case errors.Is(err, store.ErrItemTaken):
		return errItemTaken
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
	had, err := s.db.MealByID(ctx, p.User.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such meal")
	} else if err != nil {
		return nil, internalError("changing the meal failed", err)
	}
	b := &in.Body
	var cal day.Calendar
	if b.EatenAt != nil {
		if cal, _, err = userDay(p, now); err != nil {
			return nil, internalError("changing the meal failed", err)
		}
	}
	// Every op is checked, on the meal as it was read, before any new
	// food of an op is saved.
	items, _, err := s.opItems(ctx, p, &had, b.Ops, validate)
	if err != nil {
		return nil, err
	}
	if _, err := edit(had, b, items, cal); err != nil {
		return nil, err
	}
	items, warnings, err := s.opItems(ctx, p, &had, b.Ops, save)
	if err != nil {
		return nil, err
	}
	// The change is made to the meal as it is when it is stored, so
	// changes sent at once are each kept.
	stored, err := s.db.UpdateMeal(ctx, p.User.ID, actorOf(p), id, func(m meal.Meal) (meal.Meal, error) {
		return edit(m, b, items, cal)
	}, now)
	var e *huma.ErrorModel
	switch {
	case errors.As(err, &e):
		return nil, err
	case errors.Is(err, store.ErrNotFound):
		return nil, huma.Error404NotFound("no such meal")
	case errors.Is(err, store.ErrItemTaken):
		return nil, errItemTaken
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

// opItems works out the items of ops, each at its place (none for a
// remove), with their warnings; their new foods as mode says, but for an
// add of an item the meal had (one sent again), which saves none.
func (s *sessions) opItems(ctx context.Context, p auth.Principal, had *meal.Meal, ops []mealOp, mode itemMode) ([]meal.Item, []meal.Warning, error) {
	items := make([]meal.Item, len(ops))
	var warnings []meal.Warning
	for i := range ops {
		op := &ops[i]
		if err := checkOp(op); err != nil {
			return nil, nil, opError(i, err)
		}
		if op.Item == nil {
			continue
		}
		m := mode
		if op.Op == "add" && slices.ContainsFunc(had.Items, func(it meal.Item) bool { return op.Item.ID != "" && it.ID.String() == op.Item.ID }) {
			m = retry
		}
		it, ws, err := s.item(ctx, p, op.Item, m)
		if err != nil {
			return nil, nil, opError(i, err)
		}
		items[i] = it
		warnings = append(warnings, ws...)
	}
	return items, warnings, nil
}

// edit is m changed as b says: its time, which may move it to another day
// of cal, its words, and its items, op by op, with the items of the ops
// worked out.
func edit(m meal.Meal, b *updateMealBody, items []meal.Item, cal day.Calendar) (meal.Meal, error) {
	if b.EatenAt != nil {
		m.EatenAt = b.EatenAt.UTC()
		m.Day = cal.DayOf(m.EatenAt)
	}
	if b.Description != nil {
		m.Description = *b.Description
	}
	if b.Note != nil {
		m.Note = *b.Note
	}
	for i := range b.Ops {
		if err := applyOp(&m, &b.Ops[i], items[i]); err != nil {
			return m, opError(i, err)
		}
	}
	if err := m.Validate(); err != nil {
		return m, huma.Error422UnprocessableEntity("the meal: " + oneLine(err))
	}
	return m, nil
}

// applyOp changes m's items as op says, with it, the op's item worked
// out. An item op leaves keeps what it brought when it was logged: a food
// corrected since changes no meal already eaten. An add of an item m has
// under its id is a retry when it is the same (meal.Item.SameAs), a
// conflict when it is not; a remove of an item m does not have is one
// done already.
func applyOp(m *meal.Meal, op *mealOp, it meal.Item) error {
	at := slices.IndexFunc(m.Items, func(had meal.Item) bool { return op.ItemID != "" && had.ID.String() == op.ItemID })
	switch op.Op {
	case "remove":
		if at >= 0 {
			m.Items = slices.Delete(m.Items, at, at+1)
		}
		return nil
	case "replace":
		if at < 0 {
			return huma.Error422UnprocessableEntity("the meal has no item " + op.ItemID)
		}
		it.ID = m.Items[at].ID
		m.Items[at] = it
		return nil
	}
	if i := slices.IndexFunc(m.Items, func(had meal.Item) bool { return it.ID != uuid.Nil() && had.ID == it.ID }); i >= 0 {
		if m.Items[i].SameAs(it) {
			return nil
		}
		return huma.Error409Conflict("the meal has another item under the id " + it.ID.String() + ": a new item takes a new id")
	}
	m.Items = append(m.Items, it)
	return nil
}

// checkOp refuses an op of the wrong shape.
func checkOp(op *mealOp) error {
	switch {
	case op.Op == "add" && (op.Item == nil || op.ItemID != ""):
		return huma.Error422UnprocessableEntity("an add takes an item, with its id inside it, and no item_id")
	case op.Op == "remove" && (op.ItemID == "" || op.Item != nil):
		return huma.Error422UnprocessableEntity("a remove takes an item_id, and no item")
	case op.Op == "replace" && (op.ItemID == "" || op.Item == nil):
		return huma.Error422UnprocessableEntity("a replace takes an item_id and the item to put in its place")
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
