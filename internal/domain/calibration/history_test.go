package calibration

import (
	"encoding/json"
	"errors"
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
// to KcalScale times it, each body fat to it plus BodyFatShift, and the
// steps and the active energy to StepsScale and ActiveScale times them,
// with noise from Seed.
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

// validate reports an anonymisation that would leave the real days as
// they are: no move of the dates, a scale of 0 or less.
func (a *anonymisation) validate() error {
	var errs []error
	if a.Weeks <= 0 {
		errs = append(errs, fmt.Errorf("weeks %d: want more than 0", a.Weeks))
	}
	for _, s := range []struct {
		name string
		v    float64
	}{{"weight_scale", a.WeightScale}, {"kcal_scale", a.KcalScale}, {"steps_scale", a.StepsScale}, {"active_scale", a.ActiveScale}} {
		if !(s.v > 0) {
			errs = append(errs, fmt.Errorf("%s %v: want more than 0", s.name, s.v))
		}
	}
	if a.WeightShift == 0 || a.BodyFatShift == 0 {
		errs = append(errs, errors.New("weight_shift and body_fat_shift: want them other than 0"))
	}
	return errors.Join(errs...)
}

// revision is a manual revision of the target: the day it took effect,
// the TDEE it was made on (when the fixture keeps it), and the action it
// took. A verdict within a week of it, against the target it revised,
// takes the same action, or NoVerdict says why there is none.
type revision struct {
	Date      string  `json:"date"`
	TDEELow   float64 `json:"tdee_low,omitempty"`
	TDEEHigh  float64 `json:"tdee_high,omitempty"`
	Action    string  `json:"action,omitempty"`
	NoVerdict string  `json:"no_verdict,omitempty"`
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

func readHistory(tb testing.TB, path string) history {
	tb.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // G703: the directory is the one the test's runner names
	if err != nil {
		tb.Fatal(err)
	}
	var h history
	if err := json.Unmarshal(b, &h); err != nil {
		tb.Fatalf("%s: %v", path, err)
	}
	return h
}

func date(tb testing.TB, s string) day.Date {
	tb.Helper()
	if s == "" {
		return day.Date{}
	}
	d, err := day.ParseDate(s)
	if err != nil {
		tb.Fatal(err)
	}
	return d
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
// targets out of order, a day's status or the profile of no kind.
func (h *history) build(tb testing.TB) built {
	tb.Helper()
	b := built{today: date(tb, h.Today), days: map[day.Date]Day{}}
	statuses := []day.Status{day.Open, day.Complete, day.Excluded}
	for _, d := range h.Days {
		if !slices.Contains(statuses, day.Status(d.Status)) {
			tb.Fatalf("%s: status %q, want one of %v", d.Date, d.Status, statuses)
		}
		b.days[date(tb, d.Date)] = Day{
			Status: day.Status(d.Status), Meals: d.Meals, Kcal: d.Kcal, WeightKg: d.WeightKg,
			BodyFatPct: d.BodyFatPct, LeanMassKg: d.LeanMassKg, Steps: d.Steps, ActiveKcal: d.ActiveKcal,
		}
	}
	for _, p := range h.Phases {
		b.phases = append(b.phases, plan.Phase{Type: plan.PhaseType(p.Type), Start: date(tb, p.Start), End: date(tb, p.End), RatePctWeek: p.RatePctWeek})
	}
	if err := plan.ValidateTimeline(b.phases); err != nil || len(b.phases) == 0 {
		tb.Fatalf("the phases: %v, %d of them; want one or more in order", err, len(b.phases))
	}
	for _, t := range h.Targets {
		// Calibration reads only the kcal of a target, for the step.
		b.targets = append(b.targets, plan.Target{
			EffectiveFrom: date(tb, t.From),
			Training:      plan.DayTarget{Kcal: t.TrainingKcal}, Rest: plan.DayTarget{Kcal: t.RestKcal},
		})
	}
	if err := plan.ValidateHistory(b.targets); err != nil {
		tb.Fatalf("the targets: %v", err)
	}
	var err error
	if b.training, err = plan.WeekdaysOf(h.TrainingWeekdays...); err != nil {
		tb.Fatal(err)
	}
	b.pr = profile.Profile{Sex: profile.Sex(h.Profile.Sex), BirthDate: date(tb, h.Profile.BirthDate), HeightCm: h.Profile.HeightCm}
	if err := b.pr.Validate(b.today); err != nil {
		tb.Fatalf("the profile: %v", err)
	}
	return b
}

// judge returns the estimate and the verdict of the history on the day
// after last, against the target in force on targetOn.
func (b built) judge(last, targetOn day.Date) (Estimate, Verdict) {
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
	return e, Judge(e, ph, tg, b.training, b.pr, limits.UserDefaults(), p)
}

// firstVerdict returns the first verdict within a week of the revision
// on, from the day it took effect, against the target it revised, and the
// day the verdict is for; never for today or later, whose days are open.
func (b built) firstVerdict(on day.Date) (Verdict, day.Date, bool) {
	revised := on.AddDays(-1)
	for last := on.AddDays(-1); last.Before(on.AddDays(6)) && last.Before(b.today); last = last.AddDays(1) {
		if _, v := b.judge(last, revised); v.Rate != RateNone {
			return v, last.AddDays(1), true
		}
	}
	return Verdict{}, day.Date{}, false
}

// checkHistory holds calibration to a history: an estimate that moves
// little from day to day, an interval that holds the TDEE each revision
// was made on, and a verdict that takes each revision's action within a
// week of it, or none for the reason the history gives. A real history
// is told without its values or dates, as the test's output may be
// shared.
func checkHistory(t *testing.T, h history, private bool) {
	t.Helper()
	b := h.build(t)
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
		if prev != nil && math.Abs(e.TDEE-prev.TDEE) > maxDailyChange {
			t.Errorf("%s: the estimate moved more than %d kcal from the day before", tell(private, "a day", last.String()), maxDailyChange)
		}
		prev = &e
	}
	t.Logf("%d days measured, %d trusted", measured, trusted)
	if trusted == 0 {
		t.Error("no estimate was trusted")
	}
	for i, r := range h.Revisions {
		on := date(t, r.Date)
		name := tell(private, fmt.Sprintf("revision %d", i+1), r.Date)
		e, _ := b.judge(on.AddDays(-1), on.AddDays(-1))
		t.Logf("%s: the estimate the day before%s, trusted %v, doubts %v", name, tell(private, "", fmt.Sprintf(" was %.0f ± %.0f", e.TDEE, e.HalfWidth)), e.Trusted, e.Doubts)
		if (r.TDEELow > 0) != (r.TDEEHigh > 0) {
			t.Errorf("%s: the revision's TDEE has one bound, want both or neither", name)
		}
		if r.TDEEHigh > 0 && (e.TDEE-e.HalfWidth > r.TDEELow || e.TDEE+e.HalfWidth < r.TDEEHigh) {
			t.Errorf("%s: the interval does not hold the revision's TDEE", name)
		}
		switch v, d, ok := b.firstVerdict(on); {
		case ok:
			t.Logf("%s: the first verdict%s: %s, %s, guards %v", name, tell(private, "", ", for "+d.String()), v.Rate, v.Action, v.Guards)
			if v.Action != r.Action {
				t.Errorf("%s: the first verdict is to %s, the revision's was to %s", name, v.Action, r.Action)
			}
		case r.NoVerdict == "":
			t.Errorf("%s: no verdict within a week, and the history gives no reason for none", name)
		default:
			t.Logf("%s: no verdict within a week: %s", name, tell(private, "the reason is in the history", r.NoVerdict))
		}
	}
}

// tell returns shown, or hidden for a private history.
func tell(private bool, hidden, shown string) string {
	if private {
		return hidden
	}
	return shown
}

// TestCalibrateHistory holds calibration to the anonymised history. It
// keeps no revision's TDEE, and its revisions' actions are the verdicts
// it gave when it was written: it holds calibration to what it does with
// them, and the real history to the revisions.
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
// profile is another, and the TDEE of each revision is left out. Each
// revision's action is the verdict the anonymised history gives, or none.
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
	if err := a.validate(); err != nil {
		t.Fatalf("the anonymisation: %v", err)
	}
	h.Anonymise = nil
	r := rand.New(rand.NewPCG(a.Seed, a.Seed+1)) //nolint:gosec // G404: noise for a fixture, no secret
	move := func(s string) string {
		if s == "" {
			return ""
		}
		return date(t, s).AddDays(7 * a.Weeks).String()
	}
	h.Today = move(h.Today)
	h.Profile.Sex, h.Profile.BirthDate, h.Profile.HeightCm = "male", "1990-01-01", 180
	for i := range h.Phases {
		h.Phases[i].Start, h.Phases[i].End = move(h.Phases[i].Start), move(h.Phases[i].End)
	}
	for i := range h.Targets {
		h.Targets[i].From = move(h.Targets[i].From)
		h.Targets[i].TrainingKcal = roundTo(h.Targets[i].TrainingKcal*a.KcalScale, 0.1)
		h.Targets[i].RestKcal = roundTo(h.Targets[i].RestKcal*a.KcalScale, 0.1)
	}
	for i := range h.Days {
		anonymiseDay(&h.Days[i], a, move, r)
	}
	b := h.build(t)
	for i := range h.Revisions {
		rv := &h.Revisions[i]
		rv.Date, rv.TDEELow, rv.TDEEHigh = move(rv.Date), 0, 0
		rv.Action, rv.NoVerdict = "", "none within a week in the anonymised history"
		if v, _, ok := b.firstVerdict(date(t, rv.Date)); ok {
			rv.Action, rv.NoVerdict = v.Action, ""
		}
	}
	out, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "history.json"), append(out, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// anonymiseDay takes a day of the real history to the anonymised one.
func anonymiseDay(d *historyDay, a *anonymisation, move func(string) string, r *rand.Rand) {
	d.Date = move(d.Date)
	if d.WeightKg > 0 {
		d.WeightKg = roundTo(d.WeightKg*a.WeightScale+a.WeightShift+0.05*r.NormFloat64(), 20)
	}
	if d.LeanMassKg > 0 {
		d.LeanMassKg = roundTo(d.LeanMassKg*a.WeightScale+a.WeightShift/2+0.05*r.NormFloat64(), 20)
	}
	if d.BodyFatPct > 0 {
		d.BodyFatPct = roundTo(d.BodyFatPct+a.BodyFatShift+0.2*r.NormFloat64(), 10)
	}
	if d.Kcal > 0 {
		d.Kcal = roundTo(d.Kcal*a.KcalScale+30*r.NormFloat64(), 1)
	}
	if d.Steps > 0 {
		d.Steps = roundTo(d.Steps*a.StepsScale*(0.95+0.1*r.Float64()), 1)
	}
	if d.ActiveKcal > 0 {
		d.ActiveKcal = roundTo(d.ActiveKcal*a.ActiveScale*(0.9+0.2*r.Float64()), 1)
	}
}

// roundTo rounds v to the nearest 1/per: 20 for twentieths, 1 for whole
// numbers, 0.1 for tens. It divides last, so a twentieth prints as one.
func roundTo(v, per float64) float64 { return math.Round(v*per) / per }
