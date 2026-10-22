package api

import (
	"context"
	"fmt"
	"maps"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

// A tool is an operation the agents call through MCP, as the web app and
// scripts call it through REST: one definition, an operation of this API,
// so the two cannot drift (DESIGN.md §3). MCP runs a tool through its
// operation, with the same validation, token gate and answers.
type tool struct {
	// name is the tool's name in MCP, as DESIGN.md §5 lists them.
	name string
	// scopes are those of the API tokens that may call it (readScopes for
	// a tool that only reads, the tools scope alone for one that writes).
	scopes []auth.Scope
	// op is its operation; registerTools opens it to tokens and marks it
	// a tool.
	op huma.Operation
	// register adds op, with its handler, to an API.
	register func(huma.API, huma.Operation)
	// retryOK says a create sent again answers 200 with what it created
	// (201) the first time.
	retryOK bool
}

// retried is t, whose create sent again answers 200, as the first time.
func (t tool) retried() tool {
	t.retryOK = true
	return t
}

// newTool is the tool name, the operation op with handler, open to tokens
// with any of scopes.
func newTool[I, O any](name string, scopes []auth.Scope, op huma.Operation, handler func(context.Context, *I) (*O, error)) tool {
	return tool{name: name, scopes: scopes, op: op, register: func(a huma.API, op huma.Operation) { huma.Register(a, op, handler) }}
}

// readScopes open a tool that only reads to tokens that may read or use
// the tools.
var readScopes = []auth.Scope{auth.ScopeRead, auth.ScopeTools}

// toolKey, in an operation's Metadata, names the tool the operation is.
const toolKey = "tool"

// tools is the registry: every tool of the API.
func (s *sessions) tools() []tool {
	return []tool{
		newTool("get_profile", readScopes, huma.Operation{
			OperationID: "get-me",
			Method:      http.MethodGet,
			Path:        Prefix + "/me",
			Summary:     "Get your profile and plan today",
			Description: "Who you are and what you aim for today: the profile, the phase today is in, the targets in force by type of day, " +
				"today's type, the training weekdays, your notes for a coach, and the units every amount is in.",
			Errors: []int{http.StatusUnauthorized},
		}, s.getMe),
		newTool("get_day", readScopes, huma.Operation{
			OperationID: "get-day",
			Method:      http.MethodGet,
			Path:        Prefix + "/days/{date}",
			Summary:     "Get a day",
			Description: "A day of yours: its meals with what each item brought, the total against the target of the day's type, " +
				"what remains, its metrics, its check-in, its type and where it stands (open, complete or excluded).",
			Errors: []int{http.StatusUnauthorized, http.StatusUnprocessableEntity},
		}, s.getDay),
		newTool("log_metric", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "log-metric",
			Method:      http.MethodPut,
			Path:        Prefix + "/days/{date}/metrics/{kind}",
			Summary:     "Log a measurement",
			Description: "Stores a measurement of a day the user gave (a weighing, the night's sleep), of today or a day before, in the kind's unit: " +
				"it counts over what an import brings, and replaces what the day held of the kind. The answer is the day, as get_day gives it.",
			Errors: []int{http.StatusUnauthorized},
		}, s.logMetric),
		newTool("delete_metric", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "delete-metric",
			Method:      http.MethodDelete,
			Path:        Prefix + "/days/{date}/metrics/{kind}",
			Summary:     "Delete a measurement",
			Description: "Deletes a day's measurement of a kind, a wrong one: imports apply to the day again. The answer is the day, as get_day gives it.",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.deleteMetric),
		newTool("checkin", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "check-in",
			Method:      http.MethodPatch,
			Path:        Prefix + "/days/{date}",
			Summary:     "Check in a day",
			Description: "Says what the user says of a day, each field given and only those: the mood and hunger, 1 to 5, a note, " +
				"the day's type (training, rest, or plan: the training weekdays decide), and whether the day is out of calibration and why " +
				"(a restaurant meal they could not weigh, being sick, a day they forgot to log). One undo takes the whole check-in back. " +
				"The answer is the day, as get_day gives it.",
			Errors: []int{http.StatusUnauthorized},
		}, s.checkIn),
		newTool("find_foods", readScopes, huma.Operation{
			OperationID: "find-foods",
			Method:      http.MethodGet,
			Path:        Prefix + "/foods",
			Summary:     "Find foods",
			Description: "Your foods whose name, brand or an alias has words starting with each word of the query, in any case and without the diacritics: " +
				"the name's matches first, then the shorter name. Each has how much a meal had of it last. Archived foods are left out.",
			Errors: []int{http.StatusUnauthorized},
		}, s.findFoods),
		newTool("get_food", readScopes, huma.Operation{
			OperationID: "get-food",
			Method:      http.MethodGet,
			Path:        Prefix + "/foods/{id}",
			Summary:     "Get a food",
			Description: "A food of yours by its id, archived too: what a correction of a food starts from, such as the one in the way of a save (409).",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.getFood),
		newTool("upsert_food", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "upsert-food",
			Method:      http.MethodPost,
			Path:        Prefix + "/foods",
			Summary:     "Save a food",
			Description: "Saves a food, or corrects one: the food with the id, or else the one of the same name and brand, keeping its id; else a new one. " +
				"A save replaces the whole food: a field left out (a serving, an alias, an optional nutrient, the note) is cleared, so correct a food from what find_foods gave, " +
				"without its last_use. archived archives the food, or keeps it so; left out, a save brings an archived food back. " +
				"A name and brand, or an alias, another food has is refused (409), naming that food, which may be archived and so in no search: get_food gives it.",
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
		}, s.upsertFood),
		newTool("log_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID:   "log-meal",
			Method:        http.MethodPost,
			Path:          Prefix + "/meals",
			Summary:       "Log a meal",
			DefaultStatus: http.StatusCreated,
			Description: "Logs a meal under the client's UUIDv7 (201); the same meal sent again under it is a retry (200), another a conflict (409). " +
				"An item is a food of the user's by its food_id, or a new food, which is saved, each with a quantity in the food's unit (g or ml), servings or portions; " +
				"or an estimate of a whole plate. The answer holds the meal with what each item brought, where its day stands, " +
				"and warnings: values that do not add up, or a meal that looks like one logged minutes before.",
			Errors: []int{http.StatusUnauthorized, http.StatusConflict},
		}, s.logMeal).retried(),
		newTool("update_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "update-meal",
			Method:      http.MethodPatch,
			Path:        Prefix + "/meals/{id}",
			Summary:     "Change a meal",
			Description: "Changes a meal: when it was eaten, its words, and its items, by ops in order: add an item, replace one, remove one. " +
				"An item no op touches keeps what it brought when logged, though its food was corrected since. " +
				"An add carries its item's id, a UUID the client makes: an add of an item the meal has under its id is a retry when it is the same, a conflict (409) when not. " +
				"A replace with what the item has already changes nothing, so it does not take a corrected food's values: remove the item and add it again under a new id for that. " +
				"A remove of an item no longer there changes nothing.",
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
		}, s.updateMeal),
		newTool("delete_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "delete-meal",
			Method:      http.MethodDelete,
			Path:        Prefix + "/meals/{id}",
			Summary:     "Delete a meal",
			Description: "Deletes a meal, and says where its day stands without it. Its id stays the deleted meal's: a meal logged under it again is refused (409).",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.deleteMeal),
	}
}

// registerTools adds the tools ts to a: each operation marked as its tool,
// and open to its scopes, which a tool must name.
func registerTools(a huma.API, ts []tool) {
	for i := range ts {
		t := &ts[i]
		if len(t.scopes) == 0 {
			panic("api: tool " + t.name + " names no scopes")
		}
		op := t.op
		op.Tags = append(op.Tags, "tools")
		md := map[string]any{}
		maps.Copy(md, op.Metadata)
		maps.Copy(md, openToTokens(t.scopes...))
		md[toolKey] = t.name
		op.Metadata = md
		t.register(a, op)
		if t.retryOK {
			// The document lists the 200 of a retry, with the 201's answer.
			registered := a.OpenAPI().Paths[op.Path].Post
			created := registered.Responses[strconv.Itoa(http.StatusCreated)]
			registered.Responses[strconv.Itoa(http.StatusOK)] = &huma.Response{Description: "The same sent again: a retry.", Content: created.Content}
		}
	}
}

// requireUser returns the principal of a request someone made, with a web
// session or an API token.
func requireUser(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, huma.Error401Unauthorized("not signed in")
	}
	return p, nil
}

// userDay is the user's calendar and the day it is at now.
func userDay(p auth.Principal, now time.Time) (day.Calendar, day.Date, error) {
	c, err := day.NewCalendar(p.User.TimeZone, p.User.DayStartHour)
	if err != nil {
		return day.Calendar{}, day.Date{}, fmt.Errorf("the user's calendar: %w", err)
	}
	return c, c.DayOf(now), nil
}

// roundGrams rounds grams as they are shown, to a tenth
// (food.Nutrients.Rounded).
func roundGrams(v float64) float64 { return math.Round(v*10) / 10 }

// dayTargetBody is a target for one type of day, rounded as it is shown:
// kcal to a whole number, grams to a tenth.
type dayTargetBody struct {
	Kcal    float64 `json:"kcal" doc:"Energy, in kcal."`
	Protein float64 `json:"protein" doc:"Grams of protein."`
	Fat     float64 `json:"fat" doc:"Grams of fat."`
	Carbs   float64 `json:"carbs" doc:"Grams of carbs: what the energy leaves after protein and fat."`
}

func newDayTargetBody(t plan.DayTarget) dayTargetBody {
	return dayTargetBody{Kcal: math.Round(t.Kcal), Protein: roundGrams(t.Protein), Fat: roundGrams(t.Fat), Carbs: roundGrams(max(t.Carbs(), 0))}
}

// targetBody is the targets in force from a day on.
type targetBody struct {
	EffectiveFrom string        `json:"effective_from" format:"date" doc:"The first day the targets hold."`
	Training      dayTargetBody `json:"training" doc:"On a training day."`
	Rest          dayTargetBody `json:"rest" doc:"On a rest day."`
}

// phaseBody is a phase of the plan.
type phaseBody struct {
	Type            string   `json:"type" enum:"cut,maintain,bulk,recomp"`
	Start           string   `json:"start" format:"date" doc:"The phase's first day."`
	End             *string  `json:"end,omitempty" format:"date" doc:"Its last day, when it has a planned one."`
	RatePctWeek     float64  `json:"rate_pct_week" doc:"The target rate, in % of the trend weight a week; a minus is a loss."`
	GoalWeight      *float64 `json:"goal_weight,omitempty" doc:"The goal weight, in kg."`
	MilestoneWeight *float64 `json:"milestone_weight,omitempty" doc:"A milestone on the way, in kg."`
}

func newPhaseBody(p plan.Phase) *phaseBody {
	b := &phaseBody{Type: string(p.Type), Start: p.Start.String(), RatePctWeek: p.RatePctWeek, GoalWeight: p.GoalWeight, MilestoneWeight: p.MilestoneWeight}
	if !p.End.IsZero() {
		b.End = new(p.End.String())
	}
	return b
}

// MeProfile is the profile with the age it gives. It is exported so
// meBody can embed it: huma describes no field of an unexported embedded
// struct.
type MeProfile struct {
	ProfileBody
	Age *int `json:"age,omitempty" doc:"The age today, in years, when the birth date is known."`
}

// unitsBody names the units every amount of the API is in.
type unitsBody struct {
	Energy    string `json:"energy" enum:"kcal"`
	Nutrients string `json:"nutrients" enum:"g" doc:"Of protein, carbs, fat and the rest."`
	Weight    string `json:"weight" enum:"kg" doc:"Of a person."`
	Height    string `json:"height" enum:"cm"`
}

type meBody struct {
	Name             string      `json:"name"`
	Locale           string      `json:"locale" doc:"The language of the app."`
	TimeZone         string      `json:"time_zone" example:"Europe/Zagreb"`
	DayStartHour     int         `json:"day_start_hour" minimum:"0" maximum:"23" doc:"The hour the user's day starts: what comes before it belongs to the day before."`
	Today            string      `json:"today" format:"date" doc:"The user's day now."`
	TodayType        string      `json:"today_type" enum:"training,rest" doc:"Today's type, which picks its target."`
	TrainingWeekdays []string    `json:"training_weekdays" nullable:"false" enum:"sunday,monday,tuesday,wednesday,thursday,friday,saturday" doc:"The days of the week the plan makes training days."`
	Profile          MeProfile   `json:"profile"`
	Phase            *phaseBody  `json:"phase,omitempty" doc:"The phase today is in; absent when there is none."`
	Target           *targetBody `json:"target,omitempty" doc:"The targets in force today; absent before the first."`
	Notes            string      `json:"notes" doc:"What the user wants a coach to keep in mind."`
	Units            unitsBody   `json:"units"`
}

type meOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         meBody
}

func (s *sessions) getMe(ctx context.Context, _ *struct{}) (*meOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	_, today, err := userDay(p, s.now())
	if err != nil {
		return nil, internalError("reading the profile failed", err)
	}
	id := p.User.ID
	pr, err := s.db.Profile(ctx, id)
	if err != nil {
		return nil, internalError("reading the profile failed", err)
	}
	phases, err := s.db.Phases(ctx, id)
	if err != nil {
		return nil, internalError("reading the profile failed", err)
	}
	targets, err := s.db.Targets(ctx, id)
	if err != nil {
		return nil, internalError("reading the profile failed", err)
	}
	notes, err := s.db.ProfileNotes(ctx, id)
	if err != nil {
		return nil, internalError("reading the profile failed", err)
	}
	dt, weekdays, _, err := s.dayType(ctx, id, today)
	if err != nil {
		return nil, internalError("reading the profile failed", err)
	}
	b := meBody{
		Name: p.User.Name, Locale: p.User.Locale, TimeZone: p.User.TimeZone, DayStartHour: p.User.DayStartHour,
		Today: today.String(), TodayType: string(dt), TrainingWeekdays: []string{},
		Profile: MeProfile{ProfileBody: newProfileBody(pr)}, Notes: notes,
		Units: unitsBody{Energy: "kcal", Nutrients: "g", Weight: "kg", Height: "cm"},
	}
	for _, d := range weekdays.Days() {
		b.TrainingWeekdays = append(b.TrainingWeekdays, strings.ToLower(d.String()))
	}
	if !pr.BirthDate.IsZero() {
		b.Profile.Age = new(pr.AgeOn(today))
	}
	if ph, ok := plan.PhaseOn(phases, today); ok {
		b.Phase = newPhaseBody(ph)
	}
	if t, ok := plan.TargetOn(targets, today); ok {
		b.Target = &targetBody{EffectiveFrom: t.EffectiveFrom.String(), Training: newDayTargetBody(t.Training), Rest: newDayTargetBody(t.Rest)}
	}
	return &meOutput{CacheControl: "no-store", Body: b}, nil
}

// dayType returns the type of the user's day d, the plan's training
// weekdays and the check-in's toggle of the day's type, if there is one.
// A workout a source recorded on d (Strong's export) makes it a training
// day unless the toggle says otherwise.
func (s *sessions) dayType(ctx context.Context, userID uuid.UUID, d day.Date) (plan.DayType, plan.Weekdays, *bool, error) {
	weekdays, err := s.db.TrainingWeekdays(ctx, userID)
	if err != nil {
		return "", 0, nil, err
	}
	toggles, err := s.db.TrainingDayToggles(ctx, userID, d, d)
	if err != nil {
		return "", 0, nil, err
	}
	var toggle *bool
	if v, ok := toggles[d]; ok {
		toggle = &v
	}
	workouts, err := s.db.WorkoutDays(ctx, userID, d, d)
	if err != nil {
		return "", 0, nil, err
	}
	return plan.TypeOf(d, weekdays, workouts[d], toggle), weekdays, toggle, nil
}
