package api

import (
	"context"
	"fmt"
	"maps"
	"math"
	"net/http"
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
				"what remains, its metrics, its type and where it stands (open, complete or excluded).",
			Errors: []int{http.StatusUnauthorized, http.StatusUnprocessableEntity},
		}, s.getDay),
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
// No source records training sessions yet.
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
	return plan.TypeOf(d, weekdays, false, toggle), weekdays, toggle, nil
}
