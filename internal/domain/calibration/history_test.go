package calibration

import (
	"encoding/json"
	"flag"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
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

// anonymisation moves the days by Weeks whole weeks, so weekdays stay,
// and takes each weight to WeightScale times it plus WeightShift, and
// each intake to KcalScale times it, with noise from Seed.
type anonymisation struct {
	Weeks                               int
	WeightScale, WeightShift, KcalScale float64
	Seed                                uint64
}

// revision is a manual revision of the target: the day it took effect,
// the TDEE it was made on (0 when the fixture leaves it out), and the
// action it took. A verdict within a week of it takes the same action,
// or NoVerdict says why there is none.
type revision struct {
	Date      string  `json:"date"`
	TDEELow   float64 `json:"tdee_low,omitempty"`
	TDEEHigh  float64 `json:"tdee_high,omitempty"`
	Action    string  `json:"action"`
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

// plan returns the history's days, phases, targets, training weekdays and
// profile as calibration takes them.
func (h *history) plan(tb testing.TB) (map[day.Date]Day, []plan.Phase, []plan.Target, plan.Weekdays, profile.Profile) {
	tb.Helper()
	days := map[day.Date]Day{}
	for _, d := range h.Days {
		days[date(tb, d.Date)] = Day{
			Status: day.Status(d.Status), Meals: d.Meals, Kcal: d.Kcal, WeightKg: d.WeightKg,
			BodyFatPct: d.BodyFatPct, LeanMassKg: d.LeanMassKg, Steps: d.Steps, ActiveKcal: d.ActiveKcal,
		}
	}
	var phases []plan.Phase
	for _, p := range h.Phases {
		phases = append(phases, plan.Phase{Type: plan.PhaseType(p.Type), Start: date(tb, p.Start), End: date(tb, p.End), RatePctWeek: p.RatePctWeek})
	}
	if err := plan.ValidateTimeline(phases); err != nil {
		tb.Fatal(err)
	}
	var targets []plan.Target
	for _, t := range h.Targets {
		// Calibration reads only the kcal of a target, for the step.
		targets = append(targets, plan.Target{
			EffectiveFrom: date(tb, t.From),
			Training:      plan.DayTarget{Kcal: t.TrainingKcal}, Rest: plan.DayTarget{Kcal: t.RestKcal},
		})
	}
	training, err := plan.WeekdaysOf(h.TrainingWeekdays...)
	if err != nil {
		tb.Fatal(err)
	}
	pr := profile.Profile{Sex: profile.Sex(h.Profile.Sex), BirthDate: date(tb, h.Profile.BirthDate), HeightCm: h.Profile.HeightCm}
	return days, phases, targets, training, pr
}

// judge returns the estimate and the verdict of the history on the day
// after last.
func (h *history) judge(tb testing.TB, last day.Date) (Estimate, Verdict) {
	tb.Helper()
	days, phases, targets, training, pr := h.plan(tb)
	p := DefaultParams()
	e := Calibrate(days, phases, last, p)
	var ph *plan.Phase
	if got, ok := plan.PhaseOn(phases, last.AddDays(1)); ok {
		ph = &got
	}
	var tg *plan.Target
	if got, ok := plan.TargetOn(targets, last.AddDays(1)); ok {
		tg = &got
	}
	return e, Judge(e, ph, tg, training, pr, limits.UserDefaults(), p)
}

// firstVerdict returns the first verdict within a week of the revision
// on, from the day before it, and the day it was given for.
func (h *history) firstVerdict(tb testing.TB, on day.Date) (Verdict, day.Date, bool) {
	tb.Helper()
	for last := on.AddDays(-1); last.Before(on.AddDays(6)); last = last.AddDays(1) {
		if _, v := h.judge(tb, last); v.Rate != RateNone {
			return v, last, true
		}
	}
	return Verdict{}, day.Date{}, false
}

// checkHistory holds calibration to a history: an estimate that moves
// little from day to day, an interval that holds the TDEE each revision
// was made on, and a verdict that takes each revision's action within a
// week of it, or none for the reason the history gives.
func checkHistory(t *testing.T, h history) {
	t.Helper()
	days, phases, _, _, _ := h.plan(t)
	today := date(t, h.Today)
	p := DefaultParams()
	var prev *Estimate
	var measured, trusted int
	for last := phases[0].Start; last.Before(today); last = last.AddDays(1) {
		e := Calibrate(days, phases, last, p)
		if !e.Measured {
			continue
		}
		measured++
		trusted += btoi(e.Trusted)
		if prev != nil && math.Abs(e.TDEE-prev.TDEE) > maxDailyChange {
			t.Errorf("%s: the estimate moved %+.0f kcal from the day before", last, e.TDEE-prev.TDEE)
		}
		prev = &e
	}
	t.Logf("%d days measured, %d trusted", measured, trusted)
	if trusted == 0 {
		t.Error("no estimate was trusted")
	}
	for _, r := range h.Revisions {
		on := date(t, r.Date)
		e, _ := h.judge(t, on.AddDays(-1))
		t.Logf("%s: the estimate the day before, %.0f ± %.0f, trusted %v, doubts %v", on, e.TDEE, e.HalfWidth, e.Trusted, e.Doubts)
		if r.TDEEHigh > 0 && (e.TDEE+e.HalfWidth < r.TDEELow || e.TDEE-e.HalfWidth > r.TDEEHigh) {
			t.Errorf("%s: the interval %.0f ± %.0f misses the revision's TDEE, %.0f to %.0f", on, e.TDEE, e.HalfWidth, r.TDEELow, r.TDEEHigh)
		}
		switch v, last, ok := h.firstVerdict(t, on); {
		case ok:
			t.Logf("%s: the first verdict, on %s: %s, %s, guards %v", on, last, v.Rate, v.Action, v.Guards)
			if v.Action != r.Action {
				t.Errorf("%s: the verdict on %s is to %s, the revision's was to %s", on, last, v.Action, r.Action)
			}
		case r.NoVerdict == "":
			t.Errorf("%s: no verdict within a week, and the history gives no reason for none", on)
		default:
			t.Logf("%s: no verdict within a week: %s", on, r.NoVerdict)
		}
	}
}

func TestCalibrateHistory(t *testing.T) {
	t.Parallel()

	checkHistory(t, readHistory(t, filepath.Join("testdata", "history.json")))
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
	checkHistory(t, readHistory(t, filepath.Join(dir, "calibration.json")))
}

// TestWriteAnonymisedHistory writes testdata/history.json from the real
// history, with -anonymise, by the history's own anonymisation, which
// keeps the rates in % near the real ones, and so the verdicts. The
// weights, the lean mass and the intake take noise as well; the profile
// is another; the TDEE of each revision is left out. Each revision's
// action is the verdict the anonymised history gives, or none: the real
// history holds the agreement with the revisions, the anonymised one
// that calibration keeps to it.
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
	h.Anonymise = nil
	r := rand.New(rand.NewPCG(a.Seed, a.Seed+1)) //nolint:gosec // G404: noise for a fixture, no secret
	move := func(s string) string {
		if s == "" {
			return ""
		}
		return date(t, s).AddDays(7 * a.Weeks).String()
	}
	round := func(v, step float64) float64 { return math.Round(v/step) * step }
	h.Today = move(h.Today)
	h.Profile.Sex, h.Profile.BirthDate, h.Profile.HeightCm = "male", "1990-01-01", 180
	for i := range h.Phases {
		h.Phases[i].Start, h.Phases[i].End = move(h.Phases[i].Start), move(h.Phases[i].End)
	}
	for i := range h.Targets {
		h.Targets[i].From = move(h.Targets[i].From)
		h.Targets[i].TrainingKcal = round(h.Targets[i].TrainingKcal*a.KcalScale, 10)
		h.Targets[i].RestKcal = round(h.Targets[i].RestKcal*a.KcalScale, 10)
	}
	for i := range h.Days {
		anonymiseDay(&h.Days[i], a, move, r)
	}
	for i := range h.Revisions {
		rv := &h.Revisions[i]
		rv.Date, rv.TDEELow, rv.TDEEHigh = move(rv.Date), 0, 0
		rv.Action, rv.NoVerdict = "", "none within a week in the anonymised history"
		if v, _, ok := h.firstVerdict(t, date(t, rv.Date)); ok {
			rv.Action, rv.NoVerdict = v.Action, ""
		}
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "history.json"), append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// anonymiseDay takes a day of the real history to the anonymised one.
func anonymiseDay(d *historyDay, a *anonymisation, move func(string) string, r *rand.Rand) {
	round := func(v, step float64) float64 { return math.Round(v/step) * step }
	d.Date = move(d.Date)
	if d.WeightKg > 0 {
		d.WeightKg = round(d.WeightKg*a.WeightScale+a.WeightShift+0.05*r.NormFloat64(), 0.05)
	}
	if d.LeanMassKg > 0 {
		d.LeanMassKg = round(d.LeanMassKg*a.WeightScale+a.WeightShift/2+0.05*r.NormFloat64(), 0.05)
	}
	if d.BodyFatPct > 0 {
		d.BodyFatPct = round(d.BodyFatPct-5+0.2*r.NormFloat64(), 0.1)
	}
	if d.Kcal > 0 {
		d.Kcal = round(d.Kcal*a.KcalScale+30*r.NormFloat64(), 1)
	}
	if d.Steps > 0 {
		d.Steps = round(d.Steps*(0.95+0.1*r.Float64()), 1)
	}
	if d.ActiveKcal > 0 {
		d.ActiveKcal = round(d.ActiveKcal*(0.9+0.2*r.Float64()), 1)
	}
}
