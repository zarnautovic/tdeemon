package api

import (
	"context"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// nutrientsBody is energy and nutrients: the four every food has, and
// those a label may add.
type nutrientsBody struct {
	Kcal         float64  `json:"kcal"`
	Protein      float64  `json:"protein"`
	Carbs        float64  `json:"carbs"`
	Fat          float64  `json:"fat"`
	Fiber        *float64 `json:"fiber,omitempty"`
	SaturatedFat *float64 `json:"saturated_fat,omitempty"`
	Sugars       *float64 `json:"sugars,omitempty"`
	Salt         *float64 `json:"salt,omitempty"`
	Polyols      *float64 `json:"polyols,omitempty" doc:"Sugar alcohols, part of the carbs."`
	Alcohol      *float64 `json:"alcohol,omitempty"`
}

func newNutrientsBody(n food.Nutrients) nutrientsBody {
	return nutrientsBody{
		Kcal: n.Kcal, Protein: n.Protein, Carbs: n.Carbs, Fat: n.Fat,
		Fiber: n.Fiber, SaturatedFat: n.SaturatedFat, Sugars: n.Sugars, Salt: n.Salt, Polyols: n.Polyols, Alcohol: n.Alcohol,
	}
}

// itemBody is one thing eaten, as it was logged.
type itemBody struct {
	ID       string  `json:"id" format:"uuid"`
	FoodID   *string `json:"food_id,omitempty" format:"uuid" doc:"The food it came from; absent for a one-off estimate."`
	Name     string  `json:"name" doc:"The food's name when the item was logged, or the estimate's."`
	Amount   float64 `json:"amount" doc:"How much, in unit."`
	Unit     string  `json:"unit" enum:"g,ml,portion"`
	Servings float64 `json:"servings,omitempty" doc:"How many of serving were eaten, when the quantity was given so."`
	Serving  string  `json:"serving,omitempty" doc:"The named serving: a slice, a piece."`
	Portions float64 `json:"portions,omitempty" doc:"How many portions, when the quantity was given so."`
	// Nutrients are the item's own, for its amount.
	Nutrients nutrientsBody `json:"nutrients"`
	Source    string        `json:"source" enum:"label,estimate,user,dish"`
	Note      string        `json:"note,omitempty"`
}

// mealBody is a meal as stored, with its total.
type mealBody struct {
	ID          string        `json:"id" format:"uuid"`
	EatenAt     time.Time     `json:"eaten_at"`
	Description string        `json:"description"`
	Client      string        `json:"client" enum:"mcp,chat,web,telegram,import" doc:"What the meal was logged through."`
	Note        string        `json:"note,omitempty"`
	Items       []itemBody    `json:"items" nullable:"false"`
	Total       nutrientsBody `json:"total"`
}

func newMealBody(m *meal.Meal) mealBody {
	b := mealBody{
		ID: m.ID.String(), EatenAt: m.EatenAt, Description: m.Description, Client: string(m.Client), Note: m.Note,
		Items: make([]itemBody, 0, len(m.Items)), Total: newNutrientsBody(m.Total()),
	}
	for i := range m.Items {
		it := &m.Items[i]
		ib := itemBody{
			ID: it.ID.String(), Name: it.Name, Amount: it.Amount, Unit: it.Unit,
			Servings: it.Quantity.Servings, Serving: it.Quantity.Serving, Portions: it.Quantity.Portions,
			Nutrients: newNutrientsBody(it.Nutrients), Source: string(it.Source), Note: it.Note,
		}
		if it.FoodID != uuid.Nil() {
			ib.FoodID = new(it.FoodID.String())
		}
		b.Items = append(b.Items, ib)
	}
	return b
}

// metricBody is one measurement of a day.
type metricBody struct {
	Kind   string  `json:"kind" enum:"weight,body_fat,lean_mass,steps,active_kcal,basal_kcal,sleep_h,rhr,hrv,vo2max,exercise_min"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	Source string  `json:"source" enum:"manual,import" doc:"manual is what the user gave, which counts over an import."`
}

type dayBody struct {
	Date           string         `json:"date" format:"date"`
	Status         string         `json:"status" enum:"open,complete,excluded" doc:"A day is open until its end, then complete; excluded is the user's choice, which drops it from calibration."`
	ExcludedReason string         `json:"excluded_reason,omitempty" doc:"Why the user excluded the day."`
	Type           string         `json:"type" enum:"training,rest" doc:"The day's type, which picks its target."`
	TrainingToggle *bool          `json:"training_toggle,omitempty" doc:"The check-in's choice of the day's type; absent when the plan's training weekdays decide."`
	Meals          []mealBody     `json:"meals" nullable:"false" doc:"In the order they were eaten."`
	Total          nutrientsBody  `json:"total" doc:"What the day's meals add up to."`
	Target         *dayTargetBody `json:"target,omitempty" doc:"The target of the day's type; absent before the first target."`
	Remaining      *dayTargetBody `json:"remaining,omitempty" doc:"What the target leaves after the total; below 0 past it."`
	Metrics        []metricBody   `json:"metrics" nullable:"false"`
}

type dayOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         dayBody
}

type dayInput struct {
	Date string `path:"date" pattern:"^(today|[0-9]{4}-[0-9]{2}-[0-9]{2})$" example:"today" doc:"The day, YYYY-MM-DD, or today: the user's day now, by their day's start hour."`
}

func (s *sessions) getDay(ctx context.Context, in *dayInput) (*dayOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	cal, d, err := s.userDay(p)
	if err != nil {
		return nil, internalError("reading the day failed", err)
	}
	if in.Date != "today" {
		if d, err = day.ParseDate(in.Date); err != nil {
			return nil, huma.Error422UnprocessableEntity("the date: " + err.Error())
		}
	}
	b, err := s.readDay(ctx, p.User.ID, d)
	if err != nil {
		return nil, internalError("reading the day failed", err)
	}
	excluded := b.ExcludedReason != ""
	b.Status = string(cal.StatusOf(d, s.now(), excluded))
	return &dayOutput{CacheControl: "no-store", Body: b}, nil
}

// readDay reads the user's day d, all but its status.
func (s *sessions) readDay(ctx context.Context, userID uuid.UUID, d day.Date) (dayBody, error) {
	b := dayBody{Date: d.String(), Meals: []mealBody{}, Metrics: []metricBody{}}
	meals, err := s.db.MealsOfDay(ctx, userID, d)
	if err != nil {
		return b, err
	}
	for i := range meals {
		b.Meals = append(b.Meals, newMealBody(&meals[i]))
	}
	total := meal.DayTotal(meals)
	b.Total = newNutrientsBody(total)
	values, err := s.db.Metrics(ctx, userID, d, d)
	if err != nil {
		return b, err
	}
	for _, v := range values[d] {
		b.Metrics = append(b.Metrics, metricBody{Kind: string(v.Kind), Value: v.Value, Unit: v.Kind.Unit(), Source: string(v.Source)})
	}
	exclusions, err := s.db.Exclusions(ctx, userID, d, d)
	if err != nil {
		return b, err
	}
	b.ExcludedReason = exclusions[d].Reason
	dt, _, toggle, err := s.dayType(ctx, userID, d)
	if err != nil {
		return b, err
	}
	b.Type, b.TrainingToggle = string(dt), toggle
	targets, err := s.db.Targets(ctx, userID)
	if err != nil {
		return b, err
	}
	if t, ok := plan.DayTargetOn(targets, d, dt); ok {
		target := newDayTargetBody(t)
		b.Target = &target
		b.Remaining = &dayTargetBody{
			Kcal: target.Kcal - total.Kcal, Protein: target.Protein - total.Protein,
			Fat: target.Fat - total.Fat, Carbs: target.Carbs - total.Carbs,
		}
	}
	return b, nil
}
