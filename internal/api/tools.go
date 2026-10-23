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
	// destructive says the tool replaces or deletes what is there, which
	// a client may want the user to confirm.
	destructive bool
	// writesAgain says the tool sent again may write again: a meal sent
	// without an id is another.
	writesAgain bool
}

// mayWriteAgain is t, which sent again may write again.
func (t tool) mayWriteAgain() tool {
	t.writesAgain = true
	return t
}

// destroys is t, which replaces or deletes what is there.
func (t tool) destroys() tool {
	t.destructive = true
	return t
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
			Description: "Who the user is and aims for: the profile, today's phase, the targets by type of day, today's type, " +
				"the training weekdays, the notes for a coach, the units, and the time now.",
			Errors: []int{http.StatusUnauthorized},
		}, s.getMe),
		newTool("get_day", readScopes, huma.Operation{
			OperationID: "get-day",
			Method:      http.MethodGet,
			Path:        Prefix + "/days/{date}",
			Summary:     "Get a day",
			Description: "A day: its meals and items, the total against its target, what remains, its metrics, check-in, type and status.",
			Errors:      []int{http.StatusUnauthorized, http.StatusUnprocessableEntity},
		}, s.getDay),
		newTool("get_summary", readScopes, huma.Operation{
			OperationID: "get-summary",
			Method:      http.MethodGet,
			Path:        Prefix + "/summary",
			Summary:     "Get the summary",
			Description: "Calibration's summary up to yesterday: weekly averages, the trend weight, the measured TDEE with its interval, " +
				"the phase's verdict, recovery, and days maybe not all logged. Explain it; do not compute it again.",
			Errors: []int{http.StatusUnauthorized},
		}, s.getSummary),
		newTool("log_metric", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "log-metric",
			Method:      http.MethodPut,
			Path:        Prefix + "/days/{date}/metrics/{kind}",
			Summary:     "Log a measurement",
			Description: "Stores a measurement the user gives, of today or a day before, in the kind's unit; " +
				"it replaces the day's value of the kind and counts over imports. Answers the day.",
			Errors: []int{http.StatusUnauthorized},
		}, s.logMetric).destroys(),
		newTool("delete_metric", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "delete-metric",
			Method:      http.MethodDelete,
			Path:        Prefix + "/days/{date}/metrics/{kind}",
			Summary:     "Delete a measurement",
			Description: "Deletes a wrong measurement of a day; imports apply to it again. Answers the day.",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.deleteMetric).destroys(),
		newTool("checkin", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "check-in",
			Method:      http.MethodPatch,
			Path:        Prefix + "/days/{date}",
			Summary:     "Check in a day",
			Description: "Sets what the user says of a day, the fields given only: mood and hunger (1 to 5), a note, the day's type, " +
				"and whether it is out of calibration and why. A day to come takes its type and exclusion only. Answers the day.",
			Errors: []int{http.StatusUnauthorized},
		}, s.checkIn).destroys(),
		newTool("undo_last", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "undo-last",
			Method:      http.MethodPost,
			Path:        Prefix + "/undo",
			Summary:     "Undo the latest change",
			Description: "Takes back the latest change not undone, of any client: a meal, a food, a measurement, a check-in; not the plan nor an import. " +
				"Each undo takes a new id; the same id again answers as before. A change that cannot be taken back is a 409 saying why, " +
				"and the next undo goes past it. Answers what was taken back.",
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
		}, s.undoLast).destroys().mayWriteAgain(),
		newTool("find_foods", readScopes, huma.Operation{
			OperationID: "find-foods",
			Method:      http.MethodGet,
			Path:        Prefix + "/foods",
			Summary:     "Find foods",
			Description: "The user's foods whose name, brand or an alias has words starting with a query's, in any case, without diacritics, " +
				"the name's matches first. One query a food, a meal's at once: each food says which queries found it, and its last use.",
			Errors: []int{http.StatusUnauthorized},
		}, s.findFoods),
		newTool("get_food", readScopes, huma.Operation{
			OperationID: "get-food",
			Method:      http.MethodGet,
			Path:        Prefix + "/foods/{id}",
			Summary:     "Get a food",
			Description: "A food by its id, archived too.",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.getFood),
		newTool("upsert_food", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "upsert-food",
			Method:      http.MethodPost,
			Path:        Prefix + "/foods",
			Summary:     "Save a food",
			Description: "Saves a food, or corrects one: by its id, or else its name and brand; else a new one. A save replaces the whole food, " +
				"so a field left out is cleared: correct from what find_foods gave. A name or alias another food has is a 409 naming it.",
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
		}, s.upsertFood).destroys(),
		newTool("log_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID:   "log-meal",
			Method:        http.MethodPost,
			Path:          Prefix + "/meals",
			Summary:       "Log a meal",
			DefaultStatus: http.StatusCreated,
			Description: "Logs a meal of items: a food of the user's by food_id, a new food (saved), or an estimate of a whole plate. " +
				"The same meal sent again under its id is a retry; without an id the server makes one, and a meal sent again is another. " +
				"Answers the meal with each item's values, where its day stands, and warnings.",
			Errors: []int{http.StatusUnauthorized, http.StatusConflict},
		}, s.logMeal).retried().mayWriteAgain(),
		newTool("update_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "update-meal",
			Method:      http.MethodPatch,
			Path:        Prefix + "/meals/{id}",
			Summary:     "Change a meal",
			Description: "Changes a meal: its time, its words, and its items by ops in order (add, replace, remove). " +
				"An item no op touches keeps its values. An add under an item id the meal has is a retry when the same, a 409 when not. " +
				"Answers as log_meal does.",
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
		}, s.updateMeal).destroys(),
		newTool("delete_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "delete-meal",
			Method:      http.MethodDelete,
			Path:        Prefix + "/meals/{id}",
			Summary:     "Delete a meal",
			Description: "Deletes a meal and answers its day without it; its id cannot be logged again.",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.deleteMeal).destroys(),
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
	Now              time.Time   `json:"now" doc:"The time now, on the user's clock: for when a meal was eaten, and for the time of an id a client makes."`
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
	now := s.now()
	cal, today, err := userDay(p, now)
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
		Today: today.String(), Now: now.In(cal.Location).Truncate(time.Second), TodayType: string(dt), TrainingWeekdays: []string{},
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

// toggleOn returns the check-in's toggle of day d's type, of toggles,
// nil for none.
func toggleOn(toggles map[day.Date]bool, d day.Date) *bool {
	if v, ok := toggles[d]; ok {
		return &v
	}
	return nil
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
	toggle := toggleOn(toggles, d)
	workouts, err := s.db.WorkoutDays(ctx, userID, d, d)
	if err != nil {
		return "", 0, nil, err
	}
	return plan.TypeOf(d, weekdays, workouts[d], toggle), weekdays, toggle, nil
}
