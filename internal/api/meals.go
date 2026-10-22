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
// app with a session, MCP or the API with a token.
func clientOf(p auth.Principal) meal.Client {
	switch {
	case p.Via == auth.ViaSession:
		return meal.ClientWeb
	case p.Client == auth.ClientMCP:
		return meal.ClientMCP
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

// sentItem is the item in as the client sent it, worked out without the
// store: its kind checked, its food by id, its new food checked and told
// by meal.SentFood, its estimate whole; with the new food, and warnings
// about values the client stated that do not add up. That is all a
// retry is told by (meal.Item.SameAs), so one is known before anything is
// read or saved.
func sentItem(in *itemInput) (meal.Item, *food.Food, []meal.Warning, error) {
	kinds := 0
	for _, given := range []bool{in.FoodID != "", in.Food != nil, in.Name != "" || in.Nutrients != nil} {
		if given {
			kinds++
		}
	}
	// None at all fails as an estimate without its values.
	if kinds > 1 {
		return meal.Item{}, nil, nil, huma.Error422UnprocessableEntity(errNotItem.Error())
	}
	q := meal.Quantity{Amount: in.Amount, Unit: in.Unit, Servings: in.Servings, Serving: in.Serving, Portions: in.Portions}
	it := meal.Item{Quantity: q, Note: in.Note}
	var nf *food.Food
	var warnings []meal.Warning
	var err error
	switch {
	case in.FoodID != "":
		if it.FoodID, err = uuid.Parse(in.FoodID); err != nil {
			return meal.Item{}, nil, nil, huma.Error422UnprocessableEntity("no such food: " + in.FoodID)
		}
	case in.Food != nil:
		f, err := newFood(in.Food)
		if err != nil {
			return meal.Item{}, nil, nil, err
		}
		nf, it.SentFood, warnings = &f, meal.SentFood(f), meal.CheckNutrients(f.Name, f.Nutrients)
	default:
		if it, warnings, err = estimateItem(in, q); err != nil {
			return meal.Item{}, nil, nil, err
		}
	}
	if in.ID != "" {
		if it.ID, err = uuid.Parse(in.ID); err != nil {
			return meal.Item{}, nil, nil, huma.Error422UnprocessableEntity("the item's id: " + err.Error())
		}
	}
	return it, nf, warnings, nil
}

// item is sent worked out whole: the item of its quantity of the user's
// food, or of its new food nf as sent, which the store saves with the
// meal and whose id the item names until then; an estimate is whole as
// sent.
func (s *sessions) item(ctx context.Context, p auth.Principal, sent meal.Item, nf *food.Food) (meal.Item, error) {
	var f food.Food
	switch {
	case nf != nil:
		f = *nf
	case sent.FoodID != uuid.Nil():
		var err error
		if f, err = s.foodOf(ctx, p, sent.FoodID); err != nil {
			return meal.Item{}, err
		}
	default:
		return sent, nil
	}
	it, err := meal.FromFood(f, sent.Quantity, sent.Note)
	if err != nil {
		return meal.Item{}, huma.Error422UnprocessableEntity(oneLine(err))
	}
	it.ID, it.SentFood = sent.ID, sent.SentFood
	return it, nil
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

// foodOf returns the user's food with id, or a 422 naming the item's
// food, as the meal is what the request is about.
func (s *sessions) foodOf(ctx context.Context, p auth.Principal, id uuid.UUID) (food.Food, error) {
	f, err := s.db.FoodByID(ctx, p.User.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		return food.Food{}, huma.Error422UnprocessableEntity("no such food: " + id.String() + "; find_foods gives the user's foods")
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
	m, cal, err := s.newMeal(p, &in.Body, now)
	if err != nil {
		return nil, err
	}
	sent, foods, warnings, err := sentItems(in.Body.Items)
	if err != nil {
		return nil, err
	}
	// A meal sent again is told by what was sent, before anything of it is
	// read or saved: its foods corrected or renamed since, it is the same.
	m.Items = sent
	stored, err := s.db.RetryMeal(ctx, p.User.ID, m.ID, m.Fingerprint())
	created := false
	switch {
	case errors.Is(err, store.ErrNotFound):
		// A new meal's id must be made now.
		if err := meal.CheckNewID(m.ID, now); err != nil {
			return nil, huma.Error422UnprocessableEntity(err.Error())
		}
		if m.Items, err = s.items(ctx, p, sent, foods); err != nil {
			return nil, err
		}
		if err := m.Validate(); err != nil {
			return nil, huma.Error422UnprocessableEntity("the meal: " + oneLine(err))
		}
		if stored, created, err = s.db.CreateMealWithFoods(ctx, p.User.ID, actorOf(p), m, newFoods(foods), now); err != nil {
			return nil, createError(err)
		}
	case err != nil:
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
// The meal was eaten at the id's time unless b says when.
func (s *sessions) newMeal(p auth.Principal, b *logMealBody, now time.Time) (meal.Meal, day.Calendar, error) {
	id, err := uuid.Parse(b.ID)
	if err != nil {
		return meal.Meal{}, day.Calendar{}, huma.Error422UnprocessableEntity("the meal's id: " + err.Error())
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

// sentItems is sentItem of each of in: the items as sent, the new food of
// each (nil for none), and their warnings.
func sentItems(in []itemInput) ([]meal.Item, []*food.Food, []meal.Warning, error) {
	var items []meal.Item
	var foods []*food.Food
	var warnings []meal.Warning
	for i := range in {
		it, nf, ws, err := sentItem(&in[i])
		if err != nil {
			return nil, nil, nil, itemError(i, err)
		}
		items = append(items, it)
		foods = append(foods, nf)
		warnings = append(warnings, ws...)
	}
	return items, foods, warnings, nil
}

// items works out each of sent whole, with its new food in foods.
func (s *sessions) items(ctx context.Context, p auth.Principal, sent []meal.Item, foods []*food.Food) ([]meal.Item, error) {
	items := make([]meal.Item, len(sent))
	for i := range sent {
		var err error
		if items[i], err = s.item(ctx, p, sent[i], foods[i]); err != nil {
			return nil, itemError(i, err)
		}
	}
	return items, nil
}

// newFoods is the new foods of foods, to save with the meal.
func newFoods(foods []*food.Food) []food.Food {
	var out []food.Food
	for _, f := range foods {
		if f != nil {
			out = append(out, *f)
		}
	}
	return out
}

// errItemTaken is the 409 of an item under the id of another meal's item.
var errItemTaken = huma.Error409Conflict("an item's id is another meal's item: a new item takes a new id")

// createError is the answer to CreateMeal's err.
func createError(err error) error {
	var conflict *store.FoodConflictError
	switch {
	case errors.As(err, &conflict):
		return huma.Error409Conflict(conflictDetail(conflict))
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
	items, foods, warnings, err := s.opItems(ctx, p, &had, b.Ops)
	if err != nil {
		return nil, err
	}
	// The change is made to the meal as it is when it is stored, with
	// the new foods of its ops, in one transaction: changes sent at once
	// are each kept, a change refused saves no food, and an op found done
	// already saves none.
	stored, err := s.db.UpdateMeal(ctx, p.User.ID, actorOf(p), id, foods, func(m meal.Meal) (meal.Meal, error) {
		return edit(m, b, items, cal)
	}, now)
	var e *huma.ErrorModel
	var conflict *store.FoodConflictError
	switch {
	case errors.As(err, &e):
		return nil, err
	case errors.As(err, &conflict):
		return nil, huma.Error409Conflict(conflictDetail(conflict))
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

// opItem is the item of an op: as sent, which tells an op done already,
// and worked out whole, unless the meal had it as sent; and the id of the
// item the op is about.
type opItem struct {
	target uuid.UUID
	sent   meal.Item
	item   meal.Item
	worked bool
}

// opItems works out the items of ops, each at its place (none for a
// remove), with the new foods to save with them and their warnings. The
// ops are dry run in order on m, the meal as read, which they change: an
// op whose item the meal, as the ops before it leave it, has as sent (one
// of a request sent again) is done already, and is not worked out:
// nothing of it is read, and its food is not saved.
func (s *sessions) opItems(ctx context.Context, p auth.Principal, m *meal.Meal, ops []mealOp) ([]opItem, []food.Food, []meal.Warning, error) {
	items := make([]opItem, len(ops))
	var foods []food.Food
	var warnings []meal.Warning
	for i := range ops {
		op := &ops[i]
		oi, nf, ws, err := s.opItem(ctx, p, m, op)
		if err == nil {
			err = applyOp(m, op, oi)
		}
		if err != nil {
			return nil, nil, nil, opError(i, err)
		}
		items[i] = oi
		if nf != nil {
			foods = append(foods, *nf)
		}
		warnings = append(warnings, ws...)
	}
	return items, foods, warnings, nil
}

// opItem works out the item of op on m, the meal as the ops before it
// leave it, with its new food to save when it is worked out, and its
// warnings.
func (s *sessions) opItem(ctx context.Context, p auth.Principal, m *meal.Meal, op *mealOp) (opItem, *food.Food, []meal.Warning, error) {
	var oi opItem
	if err := checkOp(op); err != nil {
		return oi, nil, nil, err
	}
	if op.ItemID != "" {
		var err error
		if oi.target, err = uuid.Parse(op.ItemID); err != nil {
			return oi, nil, nil, huma.Error422UnprocessableEntity("the item_id: " + err.Error())
		}
	}
	if op.Item == nil {
		return oi, nil, nil, nil
	}
	sent, nf, ws, err := sentItem(op.Item)
	if err != nil {
		return oi, nil, nil, err
	}
	oi.sent = sent
	if op.Op == "add" {
		oi.target = sent.ID
	}
	if at := opTarget(m, oi); at >= 0 && m.Items[at].SameAs(sent) {
		return oi, nil, ws, nil
	}
	if oi.item, err = s.item(ctx, p, sent, nf); err != nil {
		return oi, nil, nil, err
	}
	oi.worked = true
	return oi, nf, ws, nil
}

// opTarget is the index in m of the item oi is about, or -1 when m has
// none.
func opTarget(m *meal.Meal, oi opItem) int {
	return slices.IndexFunc(m.Items, func(had meal.Item) bool { return had.ID == oi.target })
}

// edit is m changed as b says: its time, which may move it to another day
// of cal, its words, and its items, op by op, with the items of the ops.
func edit(m meal.Meal, b *updateMealBody, items []opItem, cal day.Calendar) (meal.Meal, error) {
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

// errChanged is the 409 of an op sent again whose item changed after the
// meal was read, before the change was made.
var errChanged = huma.Error409Conflict("the item changed while the meal was being changed: read the meal, and send the change again")

// applyOp changes m's items as op says, with oi, the op's item. An item
// op leaves keeps what it brought when it was logged: a food corrected
// since changes no meal already eaten. An op whose item m has as sent
// (meal.Item.SameAs) is done already; an add under the id of another item
// of m's is a conflict; a remove of an item m does not have is one done
// already.
func applyOp(m *meal.Meal, op *mealOp, oi opItem) error {
	at := opTarget(m, oi)
	switch {
	case op.Op == "remove":
		if at >= 0 {
			m.Items = slices.Delete(m.Items, at, at+1)
		}
		return nil
	case at < 0 && op.Op == "replace":
		return huma.Error422UnprocessableEntity("the meal has no item " + op.ItemID)
	case at >= 0 && m.Items[at].SameAs(oi.sent):
		return nil
	case at >= 0 && op.Op == "add":
		return huma.Error409Conflict("the meal has another item under the id " + oi.sent.ID.String() + ": a new item takes a new id")
	case !oi.worked:
		return errChanged
	case at < 0:
		m.Items = append(m.Items, oi.item)
		return nil
	}
	it := oi.item
	it.ID = m.Items[at].ID
	m.Items[at] = it
	return nil
}

// checkOp refuses an op of the wrong shape.
func checkOp(op *mealOp) error {
	switch {
	case op.Op == "add" && (op.Item == nil || op.Item.ID == "" || op.ItemID != ""):
		return huma.Error422UnprocessableEntity("an add takes an item with its id, a UUID the client makes, so an add sent again adds once; and no item_id")
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
