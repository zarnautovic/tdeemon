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
	ID       string      `json:"id,omitempty" format:"uuid" doc:"The item's id; made when left out."`
	FoodID   string      `json:"food_id,omitempty" format:"uuid" doc:"A food of the user's, from find_foods."`
	Food     *FoodFields `json:"food,omitempty" doc:"A new food, saved; one of the name and brand of the user's food is that food, unchanged."`
	Amount   float64     `json:"amount,omitempty" minimum:"0" doc:"How much of the food, in unit; none for an estimate."`
	Unit     string      `json:"unit,omitempty" enum:"g,ml" doc:"The food's unit, g or ml (g of a portion of known weight). Left out for a food_id, the food's; a new food or one per portion needs it."`
	Servings float64     `json:"servings,omitempty" minimum:"0" doc:"How many of the food's serving."`
	Serving  string      `json:"serving,omitempty" doc:"The serving's name, as the food names it; left out, the food's one, but a new food names it."`
	Portions float64     `json:"portions,omitempty" minimum:"0" doc:"Portions of a food per portion."`
	// Name and Nutrients make an estimate.
	Name      string             `json:"name,omitempty" maxLength:"200" doc:"A whole plate's estimate: its name, with nutrients."`
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
	// A serving's name of spaces is none, as the item stores it, so the
	// item sent again is the same.
	serving := in.Serving
	if strings.TrimSpace(serving) == "" {
		serving = ""
	}
	q := meal.Quantity{Amount: in.Amount, Unit: in.Unit, Servings: in.Servings, Serving: serving, Portions: in.Portions}
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
		// Its warnings come with the food it is (item).
		nf, it.SentFood = &f, meal.SentFood(f)
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

// warnFoodKept is the warning of a new food that is a food the user has.
const warnFoodKept = "food_kept"

// newFoodQuantity refuses a quantity of a new food that the food's being
// the user's, as it may turn out to be, could read otherwise: an amount
// without its unit (the user's food may be by another basis), servings
// without their name (it may have another serving).
func newFoodQuantity(nf *food.Food, q meal.Quantity) error {
	switch {
	case q.Amount > 0 && q.Unit == "":
		return huma.Error422UnprocessableEntity(nf.Name + ": a new food's amount names its unit, g or ml")
	case q.Servings > 0 && strings.TrimSpace(q.Serving) == "":
		return huma.Error422UnprocessableEntity(nf.Name + ": a new food's servings name the serving")
	}
	return nil
}

// item is sent worked out whole, with warnings about the values of its
// new food: the item of its quantity of the user's food, or of its new
// food nf, which the store saves with the meal and whose id the item
// names until then; an estimate is whole as sent. A new food of the name
// and brand of a food the user has is that food, as the user has it
// (the store keeps it so in the meal's transaction), which the warning
// says, rather than the values sent.
func (s *sessions) item(ctx context.Context, p auth.Principal, sent meal.Item, nf *food.Food) (meal.Item, []meal.Warning, error) {
	var f food.Food
	var warnings []meal.Warning
	if nf != nil {
		if err := newFoodQuantity(nf, sent.Quantity); err != nil {
			return meal.Item{}, nil, err
		}
	}
	switch {
	case nf != nil:
		had, err := s.db.FoodByKey(ctx, p.User.ID, nf.Name, nf.Brand)
		switch {
		case err == nil:
			f = had
			warnings = []meal.Warning{{Code: warnFoodKept, Message: fmt.Sprintf(
				"%s is a food the user has, logged with its values, not those sent: upsert_food corrects it", had.Name)}}
		case errors.Is(err, store.ErrNotFound):
			f, warnings = *nf, meal.CheckNutrients(nf.Name, nf.Nutrients)
		default:
			return meal.Item{}, nil, internalError("reading a food failed", err)
		}
	case sent.FoodID != uuid.Nil():
		var err error
		if f, err = s.foodOf(ctx, p, sent.FoodID); err != nil {
			return meal.Item{}, nil, err
		}
	default:
		return sent, nil, nil
	}
	it, err := meal.FromFood(f, sent.Quantity, sent.Note)
	if err != nil {
		return meal.Item{}, nil, huma.Error422UnprocessableEntity(oneLine(err))
	}
	it.ID, it.SentFood = sent.ID, sent.SentFood
	if nf != nil {
		// The store finds the food the item is by the new food's id.
		it.FoodID = nf.ID
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
	Code    string `json:"code" enum:"atwater,saturated_fat,sugars,polyols,duplicate_meal,food_kept" doc:"What kind of warning it is."`
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
	ID          string      `json:"id,omitempty" format:"uuid" doc:"A UUIDv7 made now (a day back to five minutes ahead): the meal sent again under it is a retry. Left out (not empty), the server makes one, and a meal sent again is another."`
	EatenAt     *time.Time  `json:"eaten_at,omitempty" doc:"When eaten; left out, the time of the id."`
	Description string      `json:"description,omitempty" maxLength:"200" doc:"What the meal was, in the user's words."`
	Items       []itemInput `json:"items" nullable:"false" minItems:"1" maxItems:"50"`
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
		var ws []meal.Warning
		if m.Items, ws, err = s.items(ctx, p, sent, foods); err != nil {
			return nil, err
		}
		warnings = append(warnings, ws...)
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
	// A client that cannot make an id (a model without a clock) leaves it
	// out: the server makes one now.
	id := meal.NewIDAt(now)
	if b.ID != "" {
		var err error
		if id, err = uuid.Parse(b.ID); err != nil {
			return meal.Meal{}, day.Calendar{}, huma.Error422UnprocessableEntity("the meal's id: " + err.Error())
		}
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

// items works out each of sent whole, with its new food in foods, and the
// warnings about the new foods' values.
func (s *sessions) items(ctx context.Context, p auth.Principal, sent []meal.Item, foods []*food.Food) ([]meal.Item, []meal.Warning, error) {
	items := make([]meal.Item, len(sent))
	var warnings []meal.Warning
	for i := range sent {
		it, ws, err := s.item(ctx, p, sent[i], foods[i])
		if err != nil {
			return nil, nil, itemError(i, err)
		}
		items[i] = it
		warnings = append(warnings, ws...)
	}
	return items, warnings, nil
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

// foodItemError is the 422 of an item of a new food that is a food the
// user has, whose quantity that food does not take.
func foodItemError(e *store.FoodItemError) error {
	return huma.Error422UnprocessableEntity(fmt.Sprintf("a new food named as the user's food %q is that food, as the user has it (upsert_food corrects it), "+
		"which takes no such quantity: %s; give an amount it takes", e.Food.Name, oneLine(e.Err)))
}

// createError is the answer to CreateMeal's err.
func createError(err error) error {
	var conflict *store.FoodConflictError
	var fie *store.FoodItemError
	switch {
	case errors.As(err, &fie):
		return foodItemError(fie)
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
	Op     string     `json:"op" enum:"add,replace,remove,note" doc:"add an item; replace item_id with item; remove item_id; note: give item_id note, and nothing else."`
	ItemID string     `json:"item_id,omitempty" format:"uuid" doc:"The item to replace, remove or note."`
	Item   *itemInput `json:"item,omitempty" doc:"The item to add, or to put in the place of item_id."`
	Note   *string    `json:"note,omitempty" maxLength:"2000" doc:"A note op's note, empty for none: the item keeps what it brought when it was logged."`
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
	var fie *store.FoodItemError
	switch {
	case errors.As(err, &e):
		return nil, err
	case errors.As(err, &fie):
		return nil, foodItemError(fie)
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
	it, fws, err := s.item(ctx, p, sent, nf)
	if err != nil {
		return oi, nil, nil, err
	}
	oi.item, oi.worked = it, true
	return oi, nf, append(ws, fws...), nil
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
	case at < 0 && op.Op == "note":
		return huma.Error422UnprocessableEntity("the meal has no item " + op.ItemID)
	case op.Op == "note":
		it, err := m.Items[at].WithNote(*op.Note)
		if err != nil {
			return huma.Error422UnprocessableEntity(oneLine(err))
		}
		m.Items[at] = it
		return nil
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
	return checkNoteOp(op)
}

// checkNoteOp refuses a note op of the wrong shape, and a note on
// another op.
func checkNoteOp(op *mealOp) error {
	switch {
	case op.Op == "note" && (op.ItemID == "" || op.Note == nil || op.Item != nil):
		return huma.Error422UnprocessableEntity("a note takes an item_id and its note, and no item")
	case op.Op != "note" && op.Note != nil:
		return huma.Error422UnprocessableEntity("only a note op takes a note: an item's own goes in its item")
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
