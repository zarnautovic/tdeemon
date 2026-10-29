package api

import (
	"context"
	"fmt"
	"maps"
	"math"
	"net/http"
	"strconv"
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
	// brief is the tool's description as MCP gives it, which a model is
	// sent with each request: what it does and answers, the policy of
	// logging being the instructions'. The operation's description, in
	// the OpenAPI document, says it all.
	brief string
	// webOnly are the properties of the body only the web app sends, a
	// person's answer in it, which MCP neither shows a model nor takes.
	webOnly []string
}

// briefly is t, described to a model as d.
func (t tool) briefly(d string) tool {
	t.brief = d
	return t
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

// forTheWebOnly is t, whose body's properties names only the web app
// sends.
func (t tool) forTheWebOnly(names ...string) tool {
	t.webOnly = names
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
			Description: "Who you are and what you aim for today: the profile, the phase today is in, the targets in force by type of day, " +
				"today's type, the training weekdays, your notes for a coach, and the units every amount is in.",
			Errors: []int{http.StatusUnauthorized},
		}, s.getMe).
			briefly("Who the user is and aims for: the profile, today's phase, the targets by type of day, today's type, " +
				"the training weekdays, the notes for a coach, the units, and the time now."),
		newTool("get_day", readScopes, huma.Operation{
			OperationID: "get-day",
			Method:      http.MethodGet,
			Path:        Prefix + "/days/{date}",
			Summary:     "Get a day",
			Description: "A day of yours: its meals with what each item brought, the total against the target of the day's type, " +
				"what remains, its metrics, its check-in, its type and where it stands (open, complete or excluded). " +
				"today is the user's day now, by their day's start hour.",
			Errors: []int{http.StatusUnauthorized, http.StatusUnprocessableEntity},
		}, s.getDay).
			briefly("A day: its meals and items, the total against its target, what remains, its metrics, check-in, type and status."),
		newTool("get_summary", readScopes, huma.Operation{
			OperationID: "get-summary",
			Method:      http.MethodGet,
			Path:        Prefix + "/summary",
			Summary:     "Get the summary",
			Description: "Calibration's summary of the days up to yesterday: the weekly averages, the trend weight, the measured TDEE with its interval " +
				"and doubts, the verdict on the phase with its guards and flags, the recovery, and the days possibly not all logged. " +
				"The server computes it: explain it, do not compute it again.",
			Errors: []int{http.StatusUnauthorized},
		}, s.getSummary).
			briefly("Calibration's summary up to yesterday: weekly averages, the trend weight, the measured TDEE with its interval, " +
				"the phase's verdict, recovery, and days maybe not all logged. Explain it; do not compute it again."),
		newTool("get_history", readScopes, huma.Operation{
			OperationID: "get-history",
			Method:      http.MethodGet,
			Path:        Prefix + "/history",
			Summary:     "Get your history",
			Description: "Your days from from to to, the 90 up to today by default and at most 366 of them up to today, or their weeks: " +
				"what was eaten against the target, the measurements (weight, body fat, steps, sleep, resting heart rate, HRV), " +
				"the trend weight and the TDEE measured up to each day, as the summary computes them. " +
				"The charts read it, and a coach looking back; the verdict on the phase and the averages now are the summary's.",
			Tags:   []string{"history"},
			Errors: []int{http.StatusUnauthorized, http.StatusUnprocessableEntity},
		}, s.getHistory).
			briefly("The user's days, or weeks, from from to to (90 days to today by default, 366 at most): intake against target, " +
				"weight and trend, the TDEE measured to each day, sleep, resting HR, HRV. For a look back (a trend, a month), by week for a long span; " +
				"the verdict, today's TDEE and the averages now: get_summary."),
		newTool("get_plan", readScopes, huma.Operation{
			OperationID: "get-plan",
			Method:      http.MethodGet,
			Path:        Prefix + "/plan",
			Summary:     "Get your plan",
			Description: "The plan as a coach reads it: the goal timeline, the targets in force within the last 365 days and those to come, " +
				"each with what its days came to (the intake against the target, and the TDEE and the rate calibration measures of them, " +
				"against the phase's planned rate), the limits a change of the plan must keep, the proposal that waits for the user's consent, " +
				"and the latest revisions. The server computes the outcomes: explain them, do not compute them again.",
			Errors: []int{http.StatusUnauthorized},
		}, s.getPlan).
			briefly("The plan, for a coach: phases, the last year's targets with what each came to (intake vs target, measured TDEE, " +
				"rate vs planned), the limits a change must keep, the pending proposal, the latest revisions. Explain it; do not compute it again."),
		newTool("propose_changes", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID:   "propose-changes",
			Method:        http.MethodPost,
			Path:          Prefix + "/proposals",
			Summary:       "Propose a change of the plan",
			DefaultStatus: http.StatusCreated,
			Description: "Proposes a change of the plan: phases and targets to set or delete, the training weekdays, and the user's limits, with why and the days it was drawn from. " +
				"Nothing changes until the user consents and apply_proposal applies it; one that changes the limits applies only in the app, a web session. " +
				"The server holds it to the limits (docs/limits.md), a limit it changes to the instance's and the rest to the limits it leaves: " +
				"one past them is refused (422), each violation an error detail whose value says the rule, the type of day, the limit, the value proposed " +
				"and the level that refuses it (user: the user may loosen it in the app; instance: the admin; code: nothing). " +
				"A change of the past is refused too: a target takes effect from today on, and a phase that has started keeps its type. " +
				"Else it waits, in place of any proposal that waited, and the answer holds it with a preview: what the limits checked, " +
				"the weekly average of each target with the rate it implies with the measured TDEE, and warnings.",
			Tags:   []string{"plan"},
			Errors: []int{http.StatusUnauthorized, http.StatusUnprocessableEntity},
		}, s.proposeChanges).
			briefly("Proposes a change of the plan (phases, targets from today on, training weekdays, limits), applied only once the user agrees. " +
				"Past the limits: 422, each violation as data (rule, limit, proposed, level); explain it, propose what they allow. " +
				"Else it waits, replacing any other, with a preview (weekly kcal, implied rate, warnings): show it, ask the user.").mayWriteAgain(),
		newTool("apply_proposal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "apply-proposal",
			Method:      http.MethodPost,
			Path:        Prefix + "/proposals/{id}/apply",
			Summary:     "Apply a proposal",
			Description: "Applies a pending proposal: its changes become the plan, a revision of it. Only once the user agreed to that proposal in so many words. " +
				"One that changes the user's limits applies only in a web session, never with a token (403), and one that loosens a limit only with confirm (409), " +
				"the user's confirmation in the app. " +
				"The server holds it to the limits again, as the data or the limits may have changed (422, as propose_changes). " +
				"A proposal that waits no more (applied, rejected, expired, or superseded: the plan changed under it, or a newer one replaced it) is a 409; " +
				"applied again it answers as it is.",
			Tags:   []string{"plan"},
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity},
		}, s.applyProposal).
			briefly("Applies a pending proposal, making its changes the plan. Call it only after the user said yes to that proposal, never on your own. " +
				"The limits are checked again (422); one that waits no more is a 409.").destroys().forTheWebOnly("confirm"),
		newTool("reject_proposal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "reject-proposal",
			Method:      http.MethodPost,
			Path:        Prefix + "/proposals/{id}/reject",
			Summary:     "Reject a proposal",
			Description: "Rejects a pending proposal the user turned down; one that waits no more is a 409, a rejected one again answers as it is.",
			Tags:        []string{"plan"},
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
		}, s.rejectProposal).
			briefly("Rejects a pending proposal the user turned down."),
		newTool("revert_proposal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID:   "revert-proposal",
			Method:        http.MethodPost,
			Path:          Prefix + "/proposals/{id}/revert",
			Summary:       "Propose taking a revision back",
			DefaultStatus: http.StatusCreated,
			Description: "Proposes taking an applied proposal back: each of its changes put back as it was, as a new pending proposal with a preview, " +
				"held to the limits as propose_changes is; applied, it marks the one it takes back reverted. " +
				"It needs the plan as the proposal left it (a later change would be lost) and changes still to come: " +
				"a change that took effect is refused (422); propose the old values from today instead.",
			Tags:   []string{"plan"},
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusUnprocessableEntity},
		}, s.revertProposal).
			briefly("Proposes taking an applied proposal back, as a pending one the user must agree to. " +
				"A change that took effect cannot be: propose the old values from today.").mayWriteAgain(),
		newTool("update_profile_notes", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "set-notes",
			Method:      http.MethodPut,
			Path:        Prefix + "/me/notes",
			Summary:     "Set your notes for a coach",
			Description: "What a coach, a model, should know of you, which get_me gives it: an injury, a food you avoid, how you train. " +
				"The notes are replaced whole, and empty deletes them. Undo takes a change of them back.",
			Tags:   []string{"settings"},
			Errors: []int{http.StatusUnauthorized, http.StatusUnprocessableEntity},
		}, s.setNotes).
			briefly("Replaces the notes for a coach whole: read them first (get_profile's notes), then send them all with the change; " +
				"what you leave out is deleted, empty deletes all.").destroys(),
		newTool("log_metric", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "log-metric",
			Method:      http.MethodPut,
			Path:        Prefix + "/days/{date}/metrics/{kind}",
			Summary:     "Log a measurement",
			Description: "Stores a measurement of a day the user gave (a weighing, the night's sleep), of today or a day before, in the kind's unit: " +
				"it counts over what an import brings, and replaces what the day held of the kind. The answer is the day, as get_day gives it.",
			Errors: []int{http.StatusUnauthorized},
		}, s.logMetric).
			briefly("Stores a measurement the user gives, of today or a day before, in the kind's unit; " +
				"it replaces the day's value of the kind and counts over imports. Answers the day.").destroys(),
		newTool("delete_metric", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "delete-metric",
			Method:      http.MethodDelete,
			Path:        Prefix + "/days/{date}/metrics/{kind}",
			Summary:     "Delete a measurement",
			Description: "Deletes a day's measurement of a kind, a wrong one: imports apply to the day again. The answer is the day, as get_day gives it.",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.deleteMetric).
			briefly("Deletes a wrong measurement of a day; imports apply to it again. Answers the day.").destroys(),
		newTool("checkin", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "check-in",
			Method:      http.MethodPatch,
			Path:        Prefix + "/days/{date}",
			Summary:     "Check in a day",
			Description: "Says what the user says of a day, each field given and only those: the mood and hunger, 1 to 5, a note, " +
				"the day's type (training, rest, or plan: the training weekdays decide), and whether the day is out of calibration and why " +
				"(a restaurant meal they could not weigh, being sick, a day they forgot to log). A day to come takes its type and exclusion, not a mood, hunger or note: " +
				"send today for the user's day now. One undo takes the whole check-in back. " +
				"The answer is the day, as get_day gives it.",
			Errors: []int{http.StatusUnauthorized},
		}, s.checkIn).
			briefly("Sets what the user says of a day, the fields given only: mood and hunger (1 to 5), a note, the day's type, " +
				"and whether it is out of calibration and why. A day to come takes its type and exclusion only. Answers the day.").destroys(),
		newTool("undo_last", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "undo-last",
			Method:      http.MethodPost,
			Path:        Prefix + "/undo",
			Summary:     "Undo the latest change",
			Description: "Takes back the user's latest change not undone yet, made through any client: a meal, a food, a measurement, a check-in, the notes for a coach. " +
				"Each undo takes a new UUIDv7 id: the same id sent again (a retry) answers as before, marked repeated, and takes back nothing more; " +
				"without an id, from a client that cannot make one, the server makes it, and the undo sent again takes back another change; " +
				"undo under a new id to go further back. Nothing is redone. Changes of the plan and an import's writes are left. " +
				"A meal logged or changed with new foods is one change: its undo takes the foods back too, whole or not at all. " +
				"A change that cannot be taken back (an import wrote over it, or what came since clashes with it) is a conflict (409) that says why, " +
				"and is passed over, so the next undo takes back the change before it. The answer says what was taken back and the days it was of; get_day shows each after it. " +
				"The web app's undo of a change it just made names that change (last), and takes it back only while it is the latest: after another, nothing (412).",
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusPreconditionFailed},
		}, s.undoLast).
			briefly("Takes back the latest change not undone, of any client: a meal, a food, a measurement, a check-in, the notes; not the plan nor an import. " +
				"Each undo takes a new id; the same id again answers as before. A change that cannot be taken back is a 409 saying why, " +
				"and the next undo goes past it. Nothing is redone: the next undo takes back the change before. Answers what was taken back and its days; get_day shows each after it.").destroys().mayWriteAgain().forTheWebOnly("last"),
		newTool("find_foods", readScopes, huma.Operation{
			OperationID: "find-foods",
			Method:      http.MethodGet,
			Path:        Prefix + "/foods",
			Summary:     "Find foods",
			Description: "Your foods whose name, brand or an alias has words starting with each word of a query, in any case and without the diacritics: " +
				"the name's matches first, then the shorter name. Send a meal's foods at once, one query each: each food says which queries found it. " +
				"Each has how much a meal had of it last. Archived foods are left out, or alone found with archived.",
			Errors: []int{http.StatusUnauthorized},
		}, s.findFoods).
			briefly("The user's foods whose name, brand or an alias has words starting with a query's, in any case, without diacritics, " +
				"the name's matches first. One query a food, a meal's at once: each food says which queries found it, and its last use."),
		newTool("get_food", readScopes, huma.Operation{
			OperationID: "get-food",
			Method:      http.MethodGet,
			Path:        Prefix + "/foods/{id}",
			Summary:     "Get a food",
			Description: "A food of yours by its id, archived too: what a correction of a food starts from, such as the one in the way of a save (409).",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.getFood).
			briefly("A food by its id, archived too."),
		newTool("upsert_food", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "upsert-food",
			Method:      http.MethodPost,
			Path:        Prefix + "/foods",
			Summary:     "Save a food",
			Description: "Saves a food, or corrects one: the food with the id (404 for an id the user has no food of), or else the one of the same name and brand, keeping its id; else a new one. " +
				"A save replaces the whole food: a field left out (a serving, an alias, an optional nutrient, the note) is cleared, so correct a food from what find_foods gave, " +
				"without its last_use. archived archives the food, or keeps it so; left out, a save brings an archived food back. " +
				"A name and brand, an alias or a barcode another food has is refused (409), naming that food, which may be archived and so in no search: get_food gives it. " +
				"Its errors say it as data: where (body.name, the alias's body.aliases[i] or the barcode's body.barcodes[i]) and the food's id as the value. " +
				"A barcode the user says is a food's is added to its barcodes, so a scan finds it.",
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
		}, s.upsertFood).
			briefly("Saves a food, or corrects one: by its id, or else its name and brand; else a new one. A save replaces the whole food, " +
				"so a field left out is cleared: correct from what find_foods or get_food gave. A name, alias or barcode another food has is a 409 naming it.").destroys(),
		newTool("log_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID:   "log-meal",
			Method:        http.MethodPost,
			Path:          Prefix + "/meals",
			Summary:       "Log a meal",
			DefaultStatus: http.StatusCreated,
			Description: "Logs a meal under the client's UUIDv7 (201); the same meal sent again under it is a retry (200), another a conflict (409). " +
				"Without an id, from a client that cannot make one, the server makes it: that meal sent again is another (or a 409, when its items carry ids of their own), " +
				"so after a timeout, look at get_day before sending it again. eaten_at left out is the id's time, so a retry has the same; a time still to come is refused (422). " +
				"An item is a food of the user's by its food_id, or a new food, which is saved, each with an amount in the food's unit (g or ml; a food of the user's takes its own when left out), servings or portions; " +
				"or an estimate, of a part nobody weighed or of a whole plate. The answer holds the meal with what each item brought, where its day stands (so no get_day around a log), " +
				"and warnings: values that do not add up, or a meal that looks like one logged minutes before.",
			Errors: []int{http.StatusUnauthorized, http.StatusConflict},
		}, s.logMeal).
			briefly("Logs a meal of items: a food of the user's by food_id, a new food (saved), or an estimate of an unweighed part or a plate. " +
				"The same meal sent again under its id is a retry; without an id the server makes one, and a meal sent again is another. " +
				"Answers the meal with each item's values, where its day stands, and warnings.").retried().mayWriteAgain(),
		newTool("update_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "update-meal",
			Method:      http.MethodPatch,
			Path:        Prefix + "/meals/{id}",
			Summary:     "Change a meal",
			Description: "Changes a meal: when it was eaten (a time still to come is refused, 422), its words, and its items, by ops in order: add an item, replace one, remove one, note one. " +
				"An item no op touches keeps what it brought when logged, though its food was corrected since; so does an item a note op gives a note, its only change. " +
				"An add carries its item's id, a UUID the client makes: an add of an item the meal has under its id is a retry when it is the same, a conflict (409) when not. " +
				"A replace with what the item has already changes nothing, so it does not take a corrected food's values: remove the item and add it again under a new id for that. " +
				"A remove of an item no longer there changes nothing.",
			Errors: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict},
		}, s.updateMeal).
			briefly("Changes a meal: its time, its words, and its items by ops in order (add, replace, remove, note: an item's note alone). " +
				"An item no op touches keeps its values, and so does a replace with what it has: for a corrected food's values, remove the item and add it under a new id. An add under an item id the meal has is a retry when the same, a 409 when not. " +
				"Answers as log_meal does.").destroys(),
		newTool("delete_meal", []auth.Scope{auth.ScopeTools}, huma.Operation{
			OperationID: "delete-meal",
			Method:      http.MethodDelete,
			Path:        Prefix + "/meals/{id}",
			Summary:     "Delete a meal",
			Description: "Deletes a meal, and says where its day stands without it. Its id stays the deleted meal's: a meal logged under it again is refused (409).",
			Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		}, s.deleteMeal).
			briefly("Deletes a meal and answers its day without it; its id cannot be logged again.").destroys(),
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
		Today: today.String(), Now: now.In(cal.Location).Truncate(time.Second), TodayType: string(dt), TrainingWeekdays: weekdayNames(weekdays),
		Profile: MeProfile{ProfileBody: newProfileBody(pr)}, Notes: notes,
		Units: unitsBody{Energy: "kcal", Nutrients: "g", Weight: "kg", Height: "cm"},
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
