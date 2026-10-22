package calibration

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
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
	Today   string `json:"today"`
	Profile struct {
		Sex       string  `json:"sex"`
		BirthDate string  `json:"birth_date"`
		HeightCm  float64 `json:"height_cm"`
	} `json:"profile"`
	Phases []struct {
		Type        string  `json:"type"`
		Start       string  `json:"start"`
		End         string  `json:"end,omitempty"`
		RatePctWeek float64 `json:"rate_pct_week"`
	} `json:"phases"`
	// Targets are the kcal of each target, from the day it took effect;
	// TrainingWeekdays the plan's, Sunday being 0.
	Targets []struct {
		From         string  `json:"from"`
		TrainingKcal float64 `json:"training_kcal"`
		RestKcal     float64 `json:"rest_kcal"`
	} `json:"targets"`
	TrainingWeekdays []time.Weekday `json:"training_weekdays"`
	Revisions        []revision     `json:"revisions"`
	Days             []historyDay   `json:"days"`
	// Anonymise is how the real history is anonymised, which only it
	// holds: the code is public, so its numbers are not.
	Anonymise *anonymisation `json:"anonymise,omitempty"`
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

// An anonymisation's scales are at least minScaleChange from 1, and its
// shifts at least minShift, in kg or points of body fat, so that no
// value stays nearly what it was.
const (
	minScaleChange = 0.05
	minShift       = 1
)

// targetNoise is the most noise a target's kcal take, half the step
// between round hundreds, so that round targets do not give KcalScale
// away, and with it every day's intake.
const targetNoise = 50

// validate reports an anonymisation that would leave the real days
// nearly as they are. It names the parameters, never their values.
func (a *anonymisation) validate() []string {
	var bad []string
	if a.Weeks <= 0 {
		bad = append(bad, "weeks")
	}
	for name, v := range map[string]float64{
		"weight_scale": a.WeightScale, "kcal_scale": a.KcalScale,
		"steps_scale": a.StepsScale, "active_scale": a.ActiveScale,
	} {
		if !(v > 0 && math.Abs(v-1) >= minScaleChange) {
			bad = append(bad, name)
		}
	}
	for name, v := range map[string]float64{"weight_shift": a.WeightShift, "body_fat_shift": a.BodyFatShift} {
		if !(math.Abs(v) >= minShift) {
			bad = append(bad, name)
		}
	}
	slices.Sort(bad)
	return bad
}

// revision is a manual revision of the target: the day it took effect,
// the TDEE it was made on (when the fixture keeps it), the action it
// took, and why a verdict is not to be expected, when it is not. A
// verdict within a week of it, against the target it replaced, takes the
// same action, or NoVerdict says why there is none. Note is the
// revision in words, which only the real history keeps.
type revision struct {
	Date      string  `json:"date"`
	TDEELow   float64 `json:"tdee_low,omitempty"`
	TDEEHigh  float64 `json:"tdee_high,omitempty"`
	Action    string  `json:"action,omitempty"`
	NoVerdict string  `json:"no_verdict,omitempty"`
	Note      string  `json:"note,omitempty"`
}

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

// readHistory reads a history, and fails for a key of no field: the real
// history is written by hand, and a typo would leave a value out.
func readHistory(tb testing.TB, path string) history {
	tb.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // G703: the directory is the one the test's runner names
	if err != nil {
		tb.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var h history
	if err := dec.Decode(&h); err != nil {
		tb.Fatalf("%s: %v", path, err)
	}
	return h
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

// built is a history as calibration takes it.
type built struct {
	today    day.Date
	days     map[day.Date]Day
	phases   []plan.Phase
	targets  []plan.Target
	training plan.Weekdays
	pr       profile.Profile
}

// build returns the history as calibration takes it, and fails for one
// that would not hold calibration to anything: no phase, phases or
// targets out of order, a day's status or the profile of no kind, or a
// revision of no target.
func (h *history) build(r reader) built {
	r.tb.Helper()
	b := built{today: r.date(h.Today), days: map[day.Date]Day{}}
	statuses := []day.Status{day.Open, day.Complete, day.Excluded}
	for i, d := range h.Days {
		if !slices.Contains(statuses, day.Status(d.Status)) {
			r.fail(fmt.Sprintf("day %d: a status of no kind", i+1), fmt.Sprintf("%s: status %q, want one of %v", d.Date, d.Status, statuses))
		}
		b.days[r.date(d.Date)] = Day{
			Status: day.Status(d.Status), Meals: d.Meals, Kcal: d.Kcal, WeightKg: d.WeightKg,
			BodyFatPct: d.BodyFatPct, LeanMassKg: d.LeanMassKg, Steps: d.Steps, ActiveKcal: d.ActiveKcal,
		}
	}
	for _, p := range h.Phases {
		b.phases = append(b.phases, plan.Phase{Type: plan.PhaseType(p.Type), Start: r.date(p.Start), End: r.date(p.End), RatePctWeek: p.RatePctWeek})
	}
	if err := plan.ValidateTimeline(b.phases); err != nil || len(b.phases) == 0 {
		r.fail("the phases: none, or not a timeline", fmt.Sprintf("the phases: %v, %d of them; want one or more in order", err, len(b.phases)))
	}
	for _, t := range h.Targets {
		// Calibration reads only the kcal of a target, for the step.
		b.targets = append(b.targets, plan.Target{
			EffectiveFrom: r.date(t.From),
			Training:      plan.DayTarget{Kcal: t.TrainingKcal}, Rest: plan.DayTarget{Kcal: t.RestKcal},
		})
	}
	if err := plan.ValidateHistory(b.targets); err != nil {
		r.fail("the targets are not a history", fmt.Sprintf("the targets: %v", err))
	}
	var err error
	if b.training, err = plan.WeekdaysOf(h.TrainingWeekdays...); err != nil {
		r.fail("the training weekdays are of no kind", err.Error())
	}
	b.pr = profile.Profile{Sex: profile.Sex(h.Profile.Sex), BirthDate: r.date(h.Profile.BirthDate), HeightCm: h.Profile.HeightCm}
	if err := b.pr.Validate(b.today); err != nil {
		r.fail("the profile is of no kind", fmt.Sprintf("the profile: %v", err))
	}
	b.checkRevisions(r, h.Revisions)
	return b
}

// checkRevisions fails for a revision that would hold calibration to
// nothing: one on a day no target took effect after another, revisions
// out of order, a TDEE with one bound or bounds out of order, and
// neither an action nor the reason there is no verdict.
func (b built) checkRevisions(r reader, revisions []revision) {
	r.tb.Helper()
	revised := map[day.Date]bool{}
	for _, t := range b.targets[min(1, len(b.targets)):] {
		revised[t.EffectiveFrom] = true
	}
	var prev day.Date
	for i, rv := range revisions {
		on := r.date(rv.Date)
		var why string
		switch {
		case !revised[on]:
			why = "no target took effect that day after another"
		case i > 0 && !prev.Before(on):
			why = "not after the revision before it"
		case (rv.TDEELow > 0) != (rv.TDEEHigh > 0) || rv.TDEELow > rv.TDEEHigh:
			why = "a TDEE with one bound, or the bounds out of order"
		case rv.Action == "" && rv.NoVerdict == "":
			why = "neither an action nor the reason there is no verdict"
		}
		if why != "" {
			r.fail(fmt.Sprintf("revision %d: %s", i+1, why), fmt.Sprintf("revision %d, %s: %s", i+1, rv.Date, why))
		}
		prev = on
	}
}

// judge returns the verdict of the history on the day after last,
// against the target in force on targetOn.
func (b built) judge(last, targetOn day.Date) Verdict {
	p := DefaultParams()
	e := Calibrate(b.days, b.phases, last, p)
	var ph *plan.Phase
	if got, ok := plan.PhaseOn(b.phases, last.AddDays(1)); ok {
		ph = &got
	}
	var tg *plan.Target
	if got, ok := plan.TargetOn(b.targets, targetOn); ok {
		tg = &got
	}
	return Judge(e, ph, tg, b.training, b.pr, limits.UserDefaults(), p)
}

// firstVerdict returns the first verdict within a week of the revision
// on, from the day it took effect, against the target it replaced, and
// the day the verdict is for. It reads no day from today on, as they are
// open: a verdict for today reads the days up to yesterday, as
// get_summary's does.
func (b built) firstVerdict(on day.Date) (Verdict, day.Date, bool) {
	replaced := on.AddDays(-1)
	for last := replaced; last.Before(on.AddDays(6)) && last.Before(b.today); last = last.AddDays(1) {
		if v := b.judge(last, replaced); v.Rate != RateNone {
			return v, last.AddDays(1), true
		}
	}
	return Verdict{}, day.Date{}, false
}

// checkHistory holds calibration to a history: an estimate that moves
// little from day to day, an interval that holds the TDEE each revision
// was made on, and a verdict that takes each revision's action within a
// week of it, or none for the reason the history gives. A private
// history is told without its values or dates, as the test's output may
// be shared.
func checkHistory(t *testing.T, h history, private bool) {
	t.Helper()
	r := reader{t, private}
	b := h.build(r)
	p := DefaultParams()
	var prev *Estimate
	var measured, trusted int
	for last := b.phases[0].Start; last.Before(b.today); last = last.AddDays(1) {
		e := Calibrate(b.days, b.phases, last, p)
		if !e.Measured {
			prev = nil
			continue
		}
		measured++
		trusted += btoi(e.Trusted)
		// An estimate from before the phase gives way to the phase's own
		// once its days are enough: another window, whose jump the design
		// expects.
		if prev != nil && prev.BeforePhase == e.BeforePhase && !(math.Abs(e.TDEE-prev.TDEE) <= maxDailyChange) {
			t.Errorf("%s: the estimate moved more than %d kcal from the day before", tell(private, "a day", last.String()), maxDailyChange)
		}
		prev = &e
	}
	t.Logf("%d days measured, %d trusted", measured, trusted)
	if trusted == 0 {
		t.Error("no estimate was trusted")
	}
	for i, rv := range h.Revisions {
		on := r.date(rv.Date)
		name := tell(private, fmt.Sprintf("revision %d", i+1), rv.Date)
		e := Calibrate(b.days, b.phases, on.AddDays(-1), p)
		t.Logf("%s: the estimate the day before%s, trusted %v, doubts %v", name, tell(private, "", fmt.Sprintf(" was %.0f ± %.0f", e.TDEE, e.HalfWidth)), e.Trusted, e.Doubts)
		if rv.TDEEHigh > 0 && !(e.TDEE-e.HalfWidth <= rv.TDEELow && e.TDEE+e.HalfWidth >= rv.TDEEHigh) {
			t.Errorf("%s: the interval does not hold the revision's TDEE", name)
		}
		switch v, d, ok := b.firstVerdict(on); {
		case ok:
			t.Logf("%s: the first verdict%s: %s, %s, guards %v", name, tell(private, "", ", for "+d.String()), v.Rate, v.Action, v.Guards)
			if v.Action != rv.Action {
				t.Errorf("%s: the first verdict is to %s, the revision's was to %s", name, v.Action, rv.Action)
			}
		case rv.NoVerdict == "":
			t.Errorf("%s: no verdict within a week, and the history gives no reason for none", name)
		default:
			t.Logf("%s: no verdict within a week: %s", name, tell(private, "the reason is in the history", rv.NoVerdict))
		}
	}
}

// TestCalibrateHistory holds calibration to the anonymised history. It
// keeps no revision's TDEE; its revisions keep the real ones' actions,
// and the anonymisation gives the same verdicts as the real history.
func TestCalibrateHistory(t *testing.T) {
	t.Parallel()

	checkHistory(t, readHistory(t, filepath.Join("testdata", "history.json")), false)
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
	checkHistory(t, readHistory(t, filepath.Join(dir, "calibration.json")), true)
}

// TestWriteAnonymisedHistory writes testdata/history.json from the real
// history, with -anonymise, by the history's own anonymisation. The
// profile is another, and the revisions keep no TDEE and no note. It
// writes nothing unless the anonymised history gives each revision the
// verdict the real one does, or none where the real one gives none, and
// passes checkHistory.
func TestWriteAnonymisedHistory(t *testing.T) {
	if !*anonymise {
		t.Skip("-anonymise is not set")
	}
	dir := os.Getenv("TDEEMON_PRIVATE_FIXTURES")
	if dir == "" {
		t.Fatal("TDEEMON_PRIVATE_FIXTURES is not set")
	}
	h := readHistory(t, filepath.Join(dir, "calibration.json"))
	a := h.Anonymise
	if a == nil {
		t.Fatal("the real history has no anonymisation")
	}
	if bad := a.validate(); len(bad) > 0 {
		t.Fatalf("the anonymisation leaves values nearly as they are: %v", bad)
	}
	h.Anonymise = nil
	secret := reader{t, true}
	verdicts := outcomes(secret, h)
	r := rand.New(rand.NewPCG(a.Seed, a.Seed+1)) //nolint:gosec // G404: noise for a fixture, no secret
	move := func(s string) string {
		if s == "" {
			return ""
		}
		return secret.date(s).AddDays(7 * a.Weeks).String()
	}
	h.Today = move(h.Today)
	h.Profile.Sex, h.Profile.BirthDate, h.Profile.HeightCm = "male", "1990-01-01", 180
	for i := range h.Phases {
		h.Phases[i].Start, h.Phases[i].End = move(h.Phases[i].Start), move(h.Phases[i].End)
	}
	for i := range h.Targets {
		tg := &h.Targets[i]
		tg.From = move(tg.From)
		tg.TrainingKcal = roundTo(tg.TrainingKcal*a.KcalScale+targetNoise*(2*r.Float64()-1), 0.1)
		tg.RestKcal = roundTo(tg.RestKcal*a.KcalScale+targetNoise*(2*r.Float64()-1), 0.1)
	}
	for i := range h.Days {
		anonymiseDay(secret, &h.Days[i], a, move, r)
	}
	for i := range h.Revisions {
		rv := &h.Revisions[i]
		rv.Date, rv.TDEELow, rv.TDEEHigh, rv.Note = move(rv.Date), 0, 0, ""
		if rv.NoVerdict != "" {
			rv.NoVerdict = "none within a week, as in the real history"
		}
	}
	if got := outcomes(reader{t, false}, h); !slices.Equal(got, verdicts) {
		t.Fatalf("the anonymised history's verdicts are %q, the real one's %q: choose other parameters", got, verdicts)
	}
	checkHistory(t, h, false)
	if t.Failed() {
		t.Fatal("the anonymised history fails checkHistory: choose other parameters")
	}
	out, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "history.json"), append(out, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// outcomes returns each revision's first verdict's action, or "none".
func outcomes(r reader, h history) []string {
	r.tb.Helper()
	b := h.build(r)
	var got []string
	for _, rv := range h.Revisions {
		action := "none"
		if v, _, ok := b.firstVerdict(r.date(rv.Date)); ok {
			action = v.Action
		}
		got = append(got, action)
	}
	return got
}

// anonymiseDay takes a day of the real history to the anonymised one, and
// fails for a value the anonymisation takes to 0 or less, which would
// read as no value.
func anonymiseDay(secret reader, d *historyDay, a *anonymisation, move func(string) string, r *rand.Rand) {
	secret.tb.Helper()
	d.Date = move(d.Date)
	for _, f := range []struct {
		name string
		v    *float64
		to   func(float64) float64
	}{
		{"weight", &d.WeightKg, func(v float64) float64 { return roundTo(v*a.WeightScale+a.WeightShift+0.05*r.NormFloat64(), 20) }},
		{"lean mass", &d.LeanMassKg, func(v float64) float64 { return roundTo(v*a.WeightScale+a.WeightShift/2+0.05*r.NormFloat64(), 20) }},
		{"body fat", &d.BodyFatPct, func(v float64) float64 { return roundTo(v+a.BodyFatShift+0.2*r.NormFloat64(), 10) }},
		{"intake", &d.Kcal, func(v float64) float64 { return roundTo(v*a.KcalScale+30*r.NormFloat64(), 1) }},
		{"steps", &d.Steps, func(v float64) float64 { return roundTo(v*a.StepsScale*(0.95+0.1*r.Float64()), 1) }},
		{"active energy", &d.ActiveKcal, func(v float64) float64 { return roundTo(v*a.ActiveScale*(0.9+0.2*r.Float64()), 1) }},
	} {
		if *f.v <= 0 {
			continue
		}
		if *f.v = f.to(*f.v); *f.v <= 0 {
			msg := "a day's " + f.name + " came to 0 or less: choose other parameters"
			secret.fail(msg, msg)
		}
	}
}

// roundTo rounds v to the nearest 1/per: 20 for twentieths, 1 for whole
// numbers, 0.1 for tens. It divides last, so a twentieth prints as one.
func roundTo(v, per float64) float64 { return math.Round(v*per) / per }
