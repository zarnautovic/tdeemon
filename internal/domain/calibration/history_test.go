package calibration

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

// anonymise writes testdata/history.json from the real history in
// TDEEMON_PRIVATE_FIXTURES: go test ./internal/domain/calibration -run
// TestWriteAnonymisedHistory -anonymise. Its output is read before it is
// committed (docs/conventions.md).
var anonymise = flag.Bool("anonymise", false, "write testdata/history.json from the real history")

// history is a user's history as a fixture: the days, the plan, the
// profile, and the manual revisions of the target calibration is held to.
type history struct {
	// Today is the day the history was taken, whose day is open.
	Today   string         `json:"today"`
	Profile historyProfile `json:"profile"`
	Phases  []historyPhase `json:"phases"`
	// Targets are the kcal of each target, from the day it took effect;
	// TrainingWeekdays the plan's, Sunday being 0.
	Targets          []historyTarget `json:"targets"`
	TrainingWeekdays []time.Weekday  `json:"training_weekdays"`
	// Limits are the user's limits of data a trusted estimate needs, when
	// they tightened them, as get_summary judges with them.
	Limits    *historyLimits `json:"limits,omitempty"`
	Revisions []revision     `json:"revisions"`
	Days      []historyDay   `json:"days"`
	// Anonymise is how the real history is anonymised, which only it
	// holds: the code is public, so its numbers are not.
	Anonymise *anonymisation `json:"anonymise,omitempty"`
}

type historyProfile struct {
	Sex       string  `json:"sex"`
	BirthDate string  `json:"birth_date"`
	HeightCm  float64 `json:"height_cm"`
}

type historyPhase struct {
	Type        string  `json:"type"`
	Start       string  `json:"start"`
	End         string  `json:"end,omitempty"`
	RatePctWeek float64 `json:"rate_pct_week"`
}

type historyTarget struct {
	From         string  `json:"from"`
	TrainingKcal float64 `json:"training_kcal"`
	RestKcal     float64 `json:"rest_kcal"`
}

type historyLimits struct {
	LoggedDays float64 `json:"logged_days"`
	WeighIns   float64 `json:"weigh_ins"`
}

// anonymisation moves the days by Weeks whole weeks, so weekdays stay; it
// takes each weight to WeightScale times it plus WeightShift, each intake
// and target to KcalScale times it, each body fat to it plus
// BodyFatShift, and the steps and the active energy to StepsScale and
// ActiveScale times them, with noise from Seed.
type anonymisation struct {
	Weeks        int     `json:"weeks"`
	WeightScale  float64 `json:"weight_scale"`
	WeightShift  float64 `json:"weight_shift"`
	KcalScale    float64 `json:"kcal_scale"`
	BodyFatShift float64 `json:"body_fat_shift"`
	StepsScale   float64 `json:"steps_scale"`
	ActiveScale  float64 `json:"active_scale"`
	Seed         uint64  `json:"seed"`
}

// An anonymisation moves the days by at least minWeeks, and each kind of
// value, on all but the tenth of the days that move least, by at least
// minMoveKg for a weight or lean mass, minMovePoints for body fat, and
// minMoveShare of it for the intake, steps and active energy: the values
// are checked, not the parameters, as a scale and a shift may undo each
// other.
const (
	minWeeks      = 52
	minMoveKg     = 2
	minMovePoints = 1.5
	minMoveShare  = 0.08
)

// targetNoise is the most noise a target's kcal take, half the step
// between round hundreds, so that round targets do not give KcalScale
// away, and with it every day's intake.
const targetNoise = 50

// validate reports the parameters an anonymisation cannot work with,
// never their values.
func (a *anonymisation) validate() []string {
	var bad []string
	if a.Weeks < minWeeks {
		bad = append(bad, "weeks")
	}
	for name, v := range map[string]float64{
		"weight_scale": a.WeightScale, "kcal_scale": a.KcalScale,
		"steps_scale": a.StepsScale, "active_scale": a.ActiveScale,
	} {
		if !(v > 0) {
			bad = append(bad, name)
		}
	}
	slices.Sort(bad)
	return bad
}

// derivedLean returns the lean mass calibration derives of d's weight and
// body fat, 0 for none.
func derivedLean(d historyDay) float64 {
	kg, _ := leanMass(Day{Status: day.Complete, WeightKg: d.WeightKg, BodyFatPct: d.BodyFatPct})
	return kg
}

// stillNear returns the kinds of value the anonymised days anon move from
// the real days orig by less than the least move, but on the tenth of the
// days that move least, never their values.
func stillNear(orig, anon []historyDay) []string {
	var near []string
	for _, k := range []struct {
		name  string
		v     func(d historyDay) float64
		least float64
		share bool
	}{
		{"weight", func(d historyDay) float64 { return d.WeightKg }, minMoveKg, false},
		{"lean mass", func(d historyDay) float64 { return d.LeanMassKg }, minMoveKg, false},
		{"lean mass of weight and body fat", derivedLean, minMoveKg, false},
		{"body fat", func(d historyDay) float64 { return d.BodyFatPct }, minMovePoints, false},
		{"intake", func(d historyDay) float64 { return d.Kcal }, minMoveShare, true},
		{"steps", func(d historyDay) float64 { return d.Steps }, minMoveShare, true},
		{"active energy", func(d historyDay) float64 { return d.ActiveKcal }, minMoveShare, true},
	} {
		var moves []float64
		for i := range orig {
			if r := k.v(orig[i]); r > 0 {
				m := math.Abs(k.v(anon[i]) - r)
				if k.share {
					m /= r
				}
				moves = append(moves, m)
			}
		}
		if len(moves) == 0 {
			continue
		}
		slices.Sort(moves)
		if !(moves[len(moves)/10] >= k.least) {
			near = append(near, k.name)
		}
	}
	return near
}

// revision is a manual revision of the target: the day it took effect,
// the TDEE it was made on (when the fixture keeps it), and the action it
// took, named as a verdict names it, against the target it replaced: a
// lowered target is lower_kcal. The first verdict within a week of it,
// against that target, takes the same action. Or it takes Verdict
// instead, and Differs says why, in words the public fixture can keep;
// or there is none, and NoVerdict is the reason each verdict of the week
// gives (Verdict.Reasons). Note is the revision in words, which only the
// real history keeps.
type revision struct {
	Date      string  `json:"date"`
	TDEELow   float64 `json:"tdee_low,omitempty"`
	TDEEHigh  float64 `json:"tdee_high,omitempty"`
	Action    string  `json:"action"`
	Verdict   string  `json:"verdict,omitempty"`
	Differs   string  `json:"differs,omitempty"`
	NoVerdict string  `json:"no_verdict,omitempty"`
	Note      string  `json:"note,omitempty"`
}

// actions are the actions a revision may take, a verdict's, and the way
// each moves the target's weekly average: down, up, or 0 for as it was;
// a nil way is any. noVerdicts are the reasons a verdict gives for none.
var (
	actions = map[string]*int{
		ActionHold: new(0), ActionRaiseKcal: new(1), ActionLowerKcal: new(-1), ActionShrinkSurplus: new(-1),
		ActionRaiseProteinOrKcal: nil, ActionMaintain: nil,
	}
	noVerdicts = []string{ReasonNoPhase, ReasonNoTarget, ReasonNoEstimate, ReasonBeforePhase, ReasonNotTrusted, ReasonNoTDEE}
)

type historyDay struct {
	Date       string  `json:"date"`
	Status     string  `json:"status"`
	Meals      int     `json:"meals,omitempty"`
	Kcal       float64 `json:"kcal,omitempty"`
	WeightKg   float64 `json:"weight_kg,omitempty"`
	BodyFatPct float64 `json:"body_fat_pct,omitempty"`
	LeanMassKg float64 `json:"lean_mass_kg,omitempty"`
	Steps      float64 `json:"steps,omitempty"`
	ActiveKcal float64 `json:"active_kcal,omitempty"`
}

// maxDailyChange is the most an estimate may move from one day to the
// next, in kcal: the predecessor's moved by thousands from week to week.
const maxDailyChange = 150

// readHistory reads a history, and fails for a key of no field, a key an
// object has twice, or something after the history: the real history is
// written by hand, and a typo would leave a value out. A private
// history's failure tells none of its values.
func readHistory(tb testing.TB, path string, private bool) history {
	tb.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // G703: the directory is the one the test's runner names
	if err != nil {
		tb.Fatal(err)
	}
	switch k, err := duplicateKey(b); {
	case err != nil:
		tb.Fatalf("%s: %s", path, tell(private, decodeError(err), err.Error()))
	case k != "":
		tb.Fatalf("%s: the key %q twice in an object", path, k)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var h history
	if err := dec.Decode(&h); err != nil {
		tb.Fatalf("%s: %s", path, tell(private, decodeError(err), err.Error()))
	}
	var more json.RawMessage
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		tb.Fatalf("%s: something after the history", path)
	}
	return h
}

// duplicateKey returns a key an object of b has twice, ignoring case as
// encoding/json does when it matches a field, which keeps the last of
// them without a word.
func duplicateKey(b []byte) (string, error) {
	// An object's keys so far, and whether its next token is a key; nil
	// for an array.
	type object struct {
		keys    map[string]bool
		wantKey bool
	}
	var stack []*object
	dec := json.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		var top *object
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		if k, ok := tok.(string); ok && top != nil && top.wantKey {
			if top.keys[strings.ToLower(k)] {
				return k, nil
			}
			top.keys[strings.ToLower(k)], top.wantKey = true, false
			continue
		}
		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			stack = stack[:len(stack)-1]
			continue
		}
		// A value: the object's next token is a key again, after the
		// object or array this one opens.
		if top != nil {
			top.wantKey = true
		}
		switch tok {
		case json.Delim('{'):
			stack = append(stack, &object{keys: map[string]bool{}, wantKey: true})
		case json.Delim('['):
			stack = append(stack, nil)
		}
	}
}

// decodeError tells what is wrong with a private history's JSON without
// the value: the field and the type it wants, the key of no field, or
// where the JSON breaks.
func decodeError(err error) string {
	var te *json.UnmarshalTypeError
	var se *json.SyntaxError
	switch {
	case errors.As(err, &te):
		return fmt.Sprintf("%s: want a value of type %s", te.Field, te.Type)
	case errors.As(err, &se):
		return fmt.Sprintf("no JSON at byte %d", se.Offset)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return err.Error()
	}
	return "not a history"
}

// reader fails a test over a history, and for a private one tells
// neither its values nor its dates, as the test's output may be shared.
type reader struct {
	tb      testing.TB
	private bool
}

// fail fails the test, telling shown, or hidden for a private history.
func (r reader) fail(hidden, shown string) {
	r.tb.Helper()
	r.tb.Fatal(tell(r.private, hidden, shown))
}

// date returns the day s names, or the zero Date for no day.
func (r reader) date(s string) day.Date {
	r.tb.Helper()
	if s == "" {
		return day.Date{}
	}
	d, err := day.ParseDate(s)
	if err != nil {
		r.fail("a date is not one", err.Error())
	}
	return d
}

// tell returns shown, or hidden for a private history.
func tell(private bool, hidden, shown string) string {
	if private {
		return hidden
	}
	return shown
}

// built is a history as calibration takes it, with the params and limits
// get_summary would judge it with.
type built struct {
	today    day.Date
	days     map[day.Date]Day
	phases   []plan.Phase
	targets  []plan.Target
	training plan.Weekdays
	pr       profile.Profile
	params   Params
	lim      limits.Limits
}

// build returns the history as calibration takes it, and fails for one
// that would not hold calibration to anything, or to less than it says:
// no today, the days as days fails them, no phase, phases or targets out
// of order, no training weekday or one twice, a profile not given in full
// or of no kind, and the revisions as checkRevisions fails them.
func (h *history) build(r reader) built {
	r.tb.Helper()
	if h.Today == "" {
		r.fail("no today", "no today")
	}
	b := built{today: r.date(h.Today), params: DefaultParams(), lim: limits.UserDefaults()}
	b.days = h.days(r, b.today)
	b.phases, b.targets = h.timeline(r)
	var err error
	if b.training, err = plan.WeekdaysOf(h.TrainingWeekdays...); err != nil || len(h.TrainingWeekdays) == 0 || b.training.Count() != len(h.TrainingWeekdays) {
		r.fail("the training weekdays: none, one twice, or of no kind", fmt.Sprintf("the training weekdays %v: %v", h.TrainingWeekdays, err))
	}
	// Validate checks only what a profile gives; calibration needs all of
	// it, the height for the BMI.
	b.pr = profile.Profile{Sex: profile.Sex(h.Profile.Sex), BirthDate: r.date(h.Profile.BirthDate), HeightCm: h.Profile.HeightCm}
	if err := b.pr.Validate(b.today); err != nil || b.pr.Sex == "" || b.pr.BirthDate.IsZero() || b.pr.HeightCm == 0 {
		r.fail("the profile: not all of it, or of no kind", fmt.Sprintf("the profile %+v: %v", b.pr, err))
	}
	if l := h.Limits; l != nil {
		b.lim.LoggedDays, b.lim.WeighIns = l.LoggedDays, l.WeighIns
		if err := b.lim.Validate(); err != nil {
			r.fail("the limits are of no kind", fmt.Sprintf("the limits: %v", err))
		}
	}
	b.params.LoggedDays, b.params.WeighIns = b.lim.LoggedDays, b.lim.WeighIns
	b.checkRevisions(r, h.Revisions)
	return b
}

// timeline returns the history's phases and targets, and fails for no
// phase or target, or either out of order.
func (h *history) timeline(r reader) ([]plan.Phase, []plan.Target) {
	r.tb.Helper()
	var phases []plan.Phase
	for _, p := range h.Phases {
		phases = append(phases, plan.Phase{Type: plan.PhaseType(p.Type), Start: r.date(p.Start), End: r.date(p.End), RatePctWeek: p.RatePctWeek})
	}
	if err := plan.ValidateTimeline(phases); err != nil || len(phases) == 0 {
		r.fail("the phases: none, or not a timeline", fmt.Sprintf("the phases: %v, %d of them; want one or more in order", err, len(phases)))
	}
	var targets []plan.Target
	for _, t := range h.Targets {
		// Calibration reads only the kcal of a target, for the step.
		targets = append(targets, plan.Target{
			EffectiveFrom: r.date(t.From),
			Training:      plan.DayTarget{Kcal: t.TrainingKcal}, Rest: plan.DayTarget{Kcal: t.RestKcal},
		})
	}
	if err := plan.ValidateHistory(targets); err != nil || len(targets) == 0 {
		r.fail("the targets: none, or not a history", fmt.Sprintf("the targets: %v, %d of them", err, len(targets)))
	}
	return phases, targets
}

// days returns the history's days by date, and fails for a day with no
// date or another day's, after today, of a status of no kind or not its
// own (today is open or excluded, a day before it complete or excluded),
// with meals and no kcal or kcal and no meals, with a value under 0, or
// with body fat of 100 % or more.
func (h *history) days(r reader, today day.Date) map[day.Date]Day {
	r.tb.Helper()
	days := map[day.Date]Day{}
	statuses := []day.Status{day.Open, day.Complete, day.Excluded}
	for i, d := range h.Days {
		on := r.date(d.Date)
		_, dup := days[on]
		v := Day{
			Status: day.Status(d.Status), Meals: d.Meals, Kcal: d.Kcal, WeightKg: d.WeightKg,
			BodyFatPct: d.BodyFatPct, LeanMassKg: d.LeanMassKg, Steps: d.Steps, ActiveKcal: d.ActiveKcal,
		}
		var why string
		switch {
		case d.Date == "" || dup:
			why = "no date, or another day's"
		case today.Before(on):
			why = "after today"
		case !slices.Contains(statuses, v.Status) || v.Status != day.Excluded && (v.Status == day.Open) != (on == today):
			why = fmt.Sprintf("a status of no kind, or not its own: want one of %v, open only today", statuses)
		case (v.Meals > 0) != (v.Kcal > 0):
			why = "meals and no kcal, or kcal and no meals"
		case slices.ContainsFunc([]float64{float64(v.Meals), v.Kcal, v.WeightKg, v.BodyFatPct, v.LeanMassKg, v.Steps, v.ActiveKcal}, func(x float64) bool { return x < 0 }) || v.BodyFatPct >= 100:
			why = "a value under 0, or body fat of 100 % or more"
		}
		if why != "" {
			r.fail(fmt.Sprintf("day %d: %s", i+1, why), fmt.Sprintf("day %d, %+v: %s", i+1, d, why))
		}
		days[on] = v
	}
	return days
}

// checkRevisions fails for no revision, and for one fault finds wrong.
func (b built) checkRevisions(r reader, revisions []revision) {
	r.tb.Helper()
	if len(revisions) == 0 {
		r.fail("no revision", "no revision")
	}
	revised := map[day.Date]plan.Target{}
	for i, t := range b.targets[1:] {
		revised[t.EffectiveFrom] = b.targets[i]
	}
	var prev day.Date
	for i, rv := range revisions {
		on := r.date(rv.Date)
		if why := b.fault(i, rv, on, prev, revised); why != "" {
			r.fail(fmt.Sprintf("revision %d: %s", i+1, why), fmt.Sprintf("revision %d, %s, %+v: %s", i+1, rv.Date, rv, why))
		}
		prev = on
	}
}

// fault returns what is wrong with the revision rv, the i-th, on the day
// on, after the one on prev, of the targets each day a target took effect
// replaced; "" for nothing. It names no value of the revision's: a
// revision on a day no target took effect after another, within a week
// of the one before it, or whose week ends after today; a TDEE with one
// bound, the bounds out of order or under 0; an action or a verdict of no
// kind, a verdict the revision's own, a verdict without why or why
// without one, why with a number in it; an action the way the targets
// moved does not take; and a reason for no verdict of no kind, or beside
// a verdict.
func (b built) fault(i int, rv revision, on, prev day.Date, revised map[day.Date]plan.Target) string {
	replaced, ok := revised[on]
	switch {
	case !ok:
		return "no target took effect that day after another"
	case i > 0 && !prev.AddDays(weekAfter).Before(on):
		return "within a week of the revision before it, or before it"
	case b.today.Before(on.AddDays(weekAfter)):
		return "the week after it ends after today"
	}
	return b.faultOf(rv, replaced, on)
}

// faultOf returns what is wrong with what the revision rv on the day on,
// which replaced the target replaced, says; "" for nothing.
func (b built) faultOf(rv revision, replaced plan.Target, on day.Date) string {
	way, known := actions[rv.Action]
	_, verdict := actions[rv.Verdict]
	switch {
	case (rv.TDEELow > 0) != (rv.TDEEHigh > 0) || rv.TDEELow > rv.TDEEHigh || rv.TDEELow < 0:
		return "a TDEE with one bound, or the bounds out of order or under 0"
	case !known || rv.Verdict != "" && (!verdict || rv.Verdict == rv.Action):
		return fmt.Sprintf("an action or a verdict of no kind, or a verdict the revision's own: want one of %v", slices.Sorted(maps.Keys(actions)))
	case (rv.Verdict == "") != (rv.Differs == "") || strings.ContainsAny(rv.Differs, "0123456789"):
		return "a verdict other than the revision's without why, why without it, or why with a number"
	case way != nil && b.way(replaced, on) != *way:
		return "an action the way the targets moved does not take"
	case rv.NoVerdict != "" && (rv.Verdict != "" || !slices.Contains(noVerdicts, rv.NoVerdict)):
		return fmt.Sprintf("a reason for no verdict beside a verdict, or of no kind: want one of %v", noVerdicts)
	}
	return ""
}

// way returns the way the weekly average of the target in force on on
// moved from replaced's: -1 down, 1 up, 0 as it was.
func (b built) way(replaced plan.Target, on day.Date) int {
	now, _ := plan.TargetOn(b.targets, on)
	n := b.training.Count()
	return cmp.Compare(now.WeeklyAverage(n).Kcal, replaced.WeeklyAverage(n).Kcal)
}

// weekAfter is the last day after a revision whose verdict counts as
// within its week: the verdicts for the day it took effect and the six
// after it.
const weekAfter = 6

// judge returns the verdict of the history on the day after last,
// against the target in force on targetOn.
func (b built) judge(last, targetOn day.Date) Verdict {
	e := Calibrate(b.days, b.phases, last, b.params)
	var ph *plan.Phase
	if got, ok := plan.PhaseOn(b.phases, last.AddDays(1)); ok {
		ph = &got
	}
	var tg *plan.Target
	if got, ok := plan.TargetOn(b.targets, targetOn); ok {
		tg = &got
	}
	return Judge(e, ph, tg, b.training, b.pr, b.lim, b.params)
}

// week returns the verdicts within a week of the revision on, from the
// day it took effect, against the target it replaced. It reads no day
// from today on, as they are open: a verdict for today reads the days up
// to yesterday, as get_summary's does. A verdict after the first day
// reads days the revision did not have: the week is how long calibration
// may take to trust what the revision acted on.
func (b built) week(on day.Date) []Verdict {
	replaced := on.AddDays(-1)
	var out []Verdict
	for last := replaced; last.Before(on.AddDays(weekAfter)) && last.Before(b.today); last = last.AddDays(1) {
		out = append(out, b.judge(last, replaced))
	}
	return out
}

// checkHistory holds calibration to a history: an estimate that moves
// little from day to day, an interval that holds the TDEE each revision
// was made on, and a verdict that takes each revision's action within a
// week of it, or the history's in its place, or none for the reason the
// history gives. It returns each revision's first verdict, as its rate,
// action, guards and day of the week, or "none". A private history is
// told without its values or dates, as the test's output may be shared.
func checkHistory(t *testing.T, h history, private bool) []string {
	t.Helper()
	r := reader{t, private}
	b := h.build(r)
	b.checkSteady(t, private)
	var got []string
	for i, rv := range h.Revisions {
		on := r.date(rv.Date)
		name := tell(private, fmt.Sprintf("revision %d", i+1), rv.Date)
		e := Calibrate(b.days, b.phases, on.AddDays(-1), b.params)
		t.Logf("%s: the estimate the day before%s, trusted %v, doubts %v", name, tell(private, "", fmt.Sprintf(" was %.0f ± %.0f", e.TDEE, e.HalfWidth)), e.Trusted, e.Doubts)
		if rv.TDEEHigh > 0 && !(e.TDEE-e.HalfWidth <= rv.TDEELow && e.TDEE+e.HalfWidth >= rv.TDEEHigh) {
			t.Errorf("%s: the interval does not hold the revision's TDEE", name)
		}
		week := b.week(on)
		first := slices.IndexFunc(week, func(v Verdict) bool { return v.Rate != RateNone })
		if first < 0 {
			got = append(got, ActionNone)
			switch {
			case rv.NoVerdict == "":
				t.Errorf("%s: no verdict within a week, and the history gives no reason for none", name)
			case slices.ContainsFunc(week, func(v Verdict) bool { return !slices.Contains(v.Reasons, rv.NoVerdict) }):
				t.Errorf("%s: no verdict within a week, but not every day's for %s", name, rv.NoVerdict)
			default:
				t.Logf("%s: no verdict within a week: %s", name, rv.NoVerdict)
			}
			continue
		}
		v := week[first]
		got = append(got, fmt.Sprintf("%s %s %v on day %d", v.Rate, v.Action, v.Guards, first))
		t.Logf("%s: the first verdict%s: %s, %s, guards %v", name, tell(private, "", ", for "+on.AddDays(first).String()), v.Rate, v.Action, v.Guards)
		switch want := cmp.Or(rv.Verdict, rv.Action); {
		case rv.NoVerdict != "":
			t.Errorf("%s: a verdict within a week, to %s, where the history gives none for %s", name, v.Action, rv.NoVerdict)
		case v.Action != want:
			t.Errorf("%s: the first verdict is to %s, want %s", name, v.Action, want)
		case rv.Verdict != "":
			t.Logf("%s: the revision was to %s: %s", name, rv.Action, tell(private, "the history says why they differ", rv.Differs))
		}
	}
	return got
}

// checkSteady holds calibration to an estimate that moves little from
// one measured day to the next, over the days it measures nothing too,
// and is trusted on some day. A day-to-day bound, not a weekly one: the
// TDEE the manual revisions were made on moved by more than a week's
// bound between them, and calibration is to follow the data.
func (b built) checkSteady(t *testing.T, private bool) {
	t.Helper()
	changes := intakeChanges(b.phases)
	var prev *Estimate
	var measured, trusted int
	for last := b.first(); last.Before(b.today); last = last.AddDays(1) {
		e := Calibrate(b.days, b.phases, last, b.params)
		if !e.Measured {
			continue
		}
		measured++
		trusted += btoi(e.Trusted)
		on := tell(private, "a day", last.String())
		if math.IsNaN(e.TDEE) || math.IsInf(e.TDEE, 0) || math.IsNaN(e.HalfWidth) || math.IsInf(e.HalfWidth, 0) {
			t.Errorf("%s: an estimate that is no number", on)
		}
		// A window of data since another change of intake, as the
		// estimate from before the phase gives way to the phase's own once
		// its days are enough: a jump the design expects.
		if prev != nil && latestChange(changes, prev.To) == latestChange(changes, e.To) && !(math.Abs(e.TDEE-prev.TDEE) <= maxDailyChange) {
			t.Errorf("%s: the estimate moved more than %d kcal from the day measured before", on, maxDailyChange)
		}
		prev = &e
	}
	t.Logf("%d days measured, %d trusted", measured, trusted)
	if trusted == 0 {
		t.Error("no estimate was trusted")
	}
}

// first returns the history's first day.
func (b built) first() day.Date {
	first := b.today
	for d := range b.days {
		if d.Before(first) {
			first = d
		}
	}
	return first
}

// TestCalibrateHistory holds calibration to the anonymised history. It
// keeps no revision's TDEE; its revisions keep the real ones' actions,
// and the anonymisation gives the same verdicts as the real history.
func TestCalibrateHistory(t *testing.T) {
	t.Parallel()

	checkHistory(t, readHistory(t, filepath.Join("testdata", "history.json"), false), false)
}

// TestCalibrateRealHistory holds calibration to a real history, outside
// git: calibration.json in the directory TDEEMON_PRIVATE_FIXTURES names.
// Without it the test is skipped.
func TestCalibrateRealHistory(t *testing.T) {
	t.Parallel()

	dir := os.Getenv("TDEEMON_PRIVATE_FIXTURES")
	if dir == "" {
		t.Skip("TDEEMON_PRIVATE_FIXTURES is not set")
	}
	checkHistory(t, readHistory(t, filepath.Join(dir, "calibration.json"), true), true)
}

// TestWriteAnonymisedHistory writes testdata/history.json from the real
// history, with -anonymise, by the history's own anonymisation. It writes
// nothing unless both histories pass checkHistory, every kind of value
// moved, and the anonymised one gives each revision the verdict the real
// one does: its rate, action and guards, on the same day of the week; or
// none where the real one gives none.
func TestWriteAnonymisedHistory(t *testing.T) {
	if !*anonymise {
		t.Skip("-anonymise is not set")
	}
	dir := os.Getenv("TDEEMON_PRIVATE_FIXTURES")
	if dir == "" {
		t.Fatal("TDEEMON_PRIVATE_FIXTURES is not set")
	}
	h := readHistory(t, filepath.Join(dir, "calibration.json"), true)
	a := h.Anonymise
	if a == nil {
		t.Fatal("the real history has no anonymisation")
	}
	if bad := a.validate(); len(bad) > 0 {
		t.Fatalf("the anonymisation cannot work with %v", bad)
	}
	verdicts := checkHistory(t, h, true)
	if t.Failed() {
		t.Fatal("the real history fails checkHistory: calibration.json needs fixing, not the anonymisation")
	}
	anon := h.anonymised(reader{t, true}, a)
	if near := stillNear(h.Days, anon.Days); len(near) > 0 {
		t.Fatalf("the anonymisation leaves the %v nearly as they are: choose other parameters", near)
	}
	// Told as a private history's until written: a history it refuses is
	// shown nowhere.
	if got := checkHistory(t, anon, true); !slices.Equal(got, verdicts) {
		t.Fatalf("the anonymised history's verdicts are %q, the real one's %q: choose other parameters", got, verdicts)
	}
	if t.Failed() {
		t.Fatal("the anonymised history fails checkHistory: choose other parameters")
	}
	out, err := json.MarshalIndent(anon, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "history.json"), append(out, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// anonymised returns the anonymised history of the real history h, by a:
// a history of its own, which takes each field of h as anonymised says,
// so that one it does not copy is left out. It fails as secret does,
// telling no value.
func (h *history) anonymised(secret reader, a *anonymisation) history {
	secret.tb.Helper()
	r := rand.New(rand.NewPCG(a.Seed, a.Seed+1)) //nolint:gosec // G404: noise for a fixture, no secret
	move := func(s string) string {
		if s == "" {
			return ""
		}
		return secret.date(s).AddDays(7 * a.Weeks).String()
	}
	out := history{
		Today: move(h.Today), Profile: historyProfile{Sex: "male", BirthDate: "1990-01-01", HeightCm: 180},
		TrainingWeekdays: slices.Clone(h.TrainingWeekdays),
	}
	if h.Limits != nil {
		out.Limits = &historyLimits{LoggedDays: h.Limits.LoggedDays, WeighIns: h.Limits.WeighIns}
	}
	for _, p := range h.Phases {
		out.Phases = append(out.Phases, historyPhase{Type: p.Type, Start: move(p.Start), End: move(p.End), RatePctWeek: p.RatePctWeek})
	}
	for _, t := range h.Targets {
		// A target the same on both types of day stays so.
		tg := historyTarget{From: move(t.From), RestKcal: roundTo(t.RestKcal*a.KcalScale+targetNoise*(2*r.Float64()-1), 0.1)}
		tg.TrainingKcal = tg.RestKcal
		if t.TrainingKcal != t.RestKcal {
			tg.TrainingKcal = roundTo(t.TrainingKcal*a.KcalScale+targetNoise*(2*r.Float64()-1), 0.1)
		}
		out.Targets = append(out.Targets, tg)
	}
	for _, d := range h.Days {
		out.Days = append(out.Days, anonymisedDay(secret, d, a, move, r))
	}
	for _, rv := range h.Revisions {
		out.Revisions = append(out.Revisions, revision{
			Date: move(rv.Date), Action: rv.Action, Verdict: rv.Verdict, Differs: rv.Differs, NoVerdict: rv.NoVerdict,
		})
	}
	return out
}

// anonymised says what the anonymisation does with each field of a
// history. A field it does not list fails
// TestAnonymisationCoversEveryField, and one the anonymisation does not
// copy is left out of the public fixture.
var anonymised = map[string]string{
	"Today":                "moved",
	"Profile.Sex":          "another profile's",
	"Profile.BirthDate":    "another profile's",
	"Profile.HeightCm":     "another profile's",
	"Phases.Type":          "kept: the kind of phase the verdict judges",
	"Phases.Start":         "moved",
	"Phases.End":           "moved",
	"Phases.RatePctWeek":   "kept: the rate the verdict judges against",
	"Targets.From":         "moved",
	"Targets.TrainingKcal": "scaled, with noise",
	"Targets.RestKcal":     "scaled, with noise",
	"TrainingWeekdays":     "kept: the days' types, which the dates keep as they move by weeks",
	"Limits.LoggedDays":    "kept: the data a trusted estimate needs",
	"Limits.WeighIns":      "kept: the data a trusted estimate needs",
	"Revisions.Date":       "moved",
	"Revisions.TDEELow":    "left out",
	"Revisions.TDEEHigh":   "left out",
	"Revisions.Action":     "kept: a verdict's action",
	"Revisions.Verdict":    "kept: a verdict's action",
	"Revisions.Differs":    "kept: written for the public fixture, with no number",
	"Revisions.NoVerdict":  "kept: a verdict's reason",
	"Revisions.Note":       "left out",
	"Days.Date":            "moved",
	"Days.Status":          "kept",
	"Days.Meals":           "moved by one at most",
	"Days.Kcal":            "scaled, with noise",
	"Days.WeightKg":        "scaled and shifted, with noise",
	"Days.BodyFatPct":      "shifted, with noise",
	"Days.LeanMassKg":      "scaled and shifted, with noise",
	"Days.Steps":           "scaled, with noise",
	"Days.ActiveKcal":      "scaled, with noise",
	"Anonymise":            "left out",
}

func TestAnonymisationCoversEveryField(t *testing.T) {
	t.Parallel()

	var fields []string
	var walk func(prefix string, typ reflect.Type)
	walk = func(prefix string, typ reflect.Type) {
		for f := range typ.Fields() {
			ft := f.Type
			for ft.Kind() == reflect.Slice || ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if name := prefix + f.Name; ft.Kind() == reflect.Struct && f.Name != "Anonymise" {
				walk(name+".", ft)
			} else {
				fields = append(fields, name)
			}
		}
	}
	walk("", reflect.TypeFor[history]())
	for _, f := range fields {
		if anonymised[f] == "" {
			t.Errorf("%s: the anonymisation says nothing of it", f)
		}
	}
	for f := range anonymised {
		if !slices.Contains(fields, f) {
			t.Errorf("%s: no such field of a history", f)
		}
	}
}

// anonymisedDay returns the anonymised day of the real day d, and fails
// for a value the anonymisation takes to 0 or less, which would read as
// no value. A day's meals move by one at most, and stay one or more.
func anonymisedDay(secret reader, d historyDay, a *anonymisation, move func(string) string, r *rand.Rand) historyDay {
	secret.tb.Helper()
	out := historyDay{Date: move(d.Date), Status: d.Status}
	if d.Meals > 0 {
		out.Meals = max(1, d.Meals+r.IntN(3)-1)
	}
	for _, f := range []struct {
		name  string
		real  float64
		anon  *float64
		moved func(float64) float64
	}{
		{"weight", d.WeightKg, &out.WeightKg, func(v float64) float64 { return roundTo(v*a.WeightScale+a.WeightShift+0.05*r.NormFloat64(), 20) }},
		{"lean mass", d.LeanMassKg, &out.LeanMassKg, func(v float64) float64 { return roundTo(v*a.WeightScale+a.WeightShift/2+0.05*r.NormFloat64(), 20) }},
		{"body fat", d.BodyFatPct, &out.BodyFatPct, func(v float64) float64 { return roundTo(v+a.BodyFatShift+0.2*r.NormFloat64(), 10) }},
		{"intake", d.Kcal, &out.Kcal, func(v float64) float64 { return roundTo(v*a.KcalScale+30*r.NormFloat64(), 1) }},
		{"steps", d.Steps, &out.Steps, func(v float64) float64 { return roundTo(v*a.StepsScale*(0.95+0.1*r.Float64()), 1) }},
		{"active energy", d.ActiveKcal, &out.ActiveKcal, func(v float64) float64 { return roundTo(v*a.ActiveScale*(0.9+0.2*r.Float64()), 1) }},
	} {
		if f.real <= 0 {
			continue
		}
		if *f.anon = f.moved(f.real); *f.anon <= 0 {
			msg := "a day's " + f.name + " came to 0 or less: choose other parameters"
			secret.fail(msg, msg)
		}
	}
	return out
}

// roundTo rounds v to the nearest 1/per: 20 for twentieths, 1 for whole
// numbers, 0.1 for tens. It divides last, so a twentieth prints as one.
func roundTo(v, per float64) float64 { return math.Round(v*per) / per }
