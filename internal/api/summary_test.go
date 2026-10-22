package api_test

import (
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// summaryAnswer is the part of get_summary's answer the tests read.
type summaryAnswer struct {
	Date  string `json:"date"`
	Weeks []struct {
		From       string   `json:"from"`
		To         string   `json:"to"`
		LoggedDays int      `json:"logged_days"`
		Kcal       *float64 `json:"kcal"`
		WeighIns   int      `json:"weigh_ins"`
		WeightKg   *float64 `json:"weight_kg"`
		TrendKg    *float64 `json:"trend_kg"`
		Steps      *float64 `json:"steps"`
		SleepH     *float64 `json:"sleep_h"`
	} `json:"weeks"`
	Trend *struct {
		Kg   float64 `json:"kg"`
		Date string  `json:"date"`
	} `json:"trend"`
	TDEE struct {
		Measured bool     `json:"measured"`
		Kcal     *float64 `json:"kcal"`
		Trusted  bool     `json:"trusted"`
		Doubts   []string `json:"doubts"`
	} `json:"tdee"`
	Verdict struct {
		Rate       string   `json:"rate"`
		Reasons    []string `json:"reasons"`
		Action     string   `json:"action"`
		GapKcal    float64  `json:"gap_kcal"`
		StepKcal   float64  `json:"step_kcal"`
		TargetKcal *float64 `json:"target_kcal"`
		Flags      []string `json:"flags"`
	} `json:"verdict"`
	Recovery struct {
		SleepH *float64 `json:"sleep_h"`
		Flags  []string `json:"flags"`
	} `json:"recovery"`
	IncompleteDays []string `json:"incomplete_days"`
}

// saveSummaryHistory saves 60 days of u's up to yesterday: a cut of 0.5 %
// a week from 59 days ago, with a target of 2400 kcal; 2000 kcal a day in
// one meal, but 500 kcal three days ago; a weigh-in losing 0.05 kg a day;
// 6 hours of sleep and 7000 steps.
func saveSummaryHistory(t *testing.T, db *store.DB, u user.User, now time.Time) {
	t.Helper()
	ctx := t.Context()
	cal, err := day.NewCalendar(u.TimeZone, u.DayStartHour)
	if err != nil {
		t.Fatal(err)
	}
	today := cal.DayOf(now)
	if err := db.SetPhase(ctx, u.ID, isolationActor, plan.Phase{Type: plan.Cut, Start: today.AddDays(-59), RatePctWeek: -0.5}, now); err != nil {
		t.Fatal(err)
	}
	d := plan.DayTarget{Kcal: 2400, Protein: 160, Fat: 70}
	if err := db.SetTarget(ctx, u.ID, isolationActor, plan.Target{EffectiveFrom: today.AddDays(-59), Training: d, Rest: d}, now); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 60; i++ {
		on := today.AddDays(-i)
		kcal := 2000.0
		if i == 3 {
			kcal = 500
		}
		item, err := meal.Estimate("a day's food", food.Nutrients{Kcal: kcal, Protein: 150, Carbs: kcal / 8, Fat: kcal / 40}, "")
		if err != nil {
			t.Fatal(err)
		}
		at := cal.Start(on).Add(8 * time.Hour)
		m := meal.Meal{ID: meal.NewIDAt(at), EatenAt: at, Day: on, Description: "the day's food", Client: meal.ClientMCP, Items: []meal.Item{item}}
		if _, _, err := db.CreateMeal(ctx, u.ID, isolationActor, m, now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.MergeMetrics(ctx, u.ID, isolationActor, on, []metric.Value{
			{Kind: metric.Weight, Value: 80 + 0.05*float64(i), Source: metric.Import},
			{Kind: metric.Sleep, Value: 6, Source: metric.Import},
			{Kind: metric.Steps, Value: 7000, Source: metric.Import},
		}, now); err != nil {
			t.Fatal(err)
		}
	}
}

// summaryOfHistory returns the summary of 2 weeks of a user with the
// history saveSummaryHistory saves, and today.
func summaryOfHistory(t *testing.T) (summaryAnswer, day.Date) {
	t.Helper()
	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	saveSummaryHistory(t, in.db, in.user, in.clock.Now())
	rec := in.do(t, http.MethodGet, "/api/v1/summary?weeks=2", "", session, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var b summaryAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b, day.DateOf(in.clock.Now())
}

// get_summary gives the weeks up to yesterday.
func TestGetSummaryWeeks(t *testing.T) {
	t.Parallel()

	b, today := summaryOfHistory(t)
	if b.Date != today.String() || len(b.Weeks) != 2 || b.Weeks[1].To != today.AddDays(-1).String() || b.Weeks[0].From != today.AddDays(-14).String() {
		t.Errorf("date %s, weeks %+v; want 2 weeks up to yesterday", b.Date, b.Weeks)
	}
	// The last week: 7 days logged, the day of 500 kcal among them.
	w := b.Weeks[1]
	if w.LoggedDays != 7 || w.WeighIns != 7 || w.Kcal == nil || *w.Kcal != math.Round((6*2000+500)/7.0) {
		t.Errorf("the last week %+v: 7 days, %v kcal", w, math.Round((6*2000+500)/7.0))
	}
	if w.Steps == nil || *w.Steps != 7000 || w.SleepH == nil || *w.SleepH != 6 || w.TrendKg == nil {
		t.Errorf("the last week %+v: 7000 steps, 6 hours of sleep, a trend", w)
	}
}

// get_summary gives the measured TDEE and the trend weight.
func TestGetSummaryTDEE(t *testing.T) {
	t.Parallel()

	b, today := summaryOfHistory(t)
	// 2000 kcal a day, losing 0.05 kg a day: a TDEE of about 2385, which
	// the day of 500 kcal lowers a little.
	if !b.TDEE.Measured || !b.TDEE.Trusted || b.TDEE.Kcal == nil || math.Abs(*b.TDEE.Kcal-2350) > 50 || len(b.TDEE.Doubts) != 0 {
		t.Errorf("TDEE %+v, want a trusted 2350 or so", b.TDEE)
	}
	if b.Trend == nil || b.Trend.Date != today.AddDays(-1).String() || math.Abs(b.Trend.Kg-80.5) > 0.5 {
		t.Errorf("trend %+v, want about 80.5 from yesterday", b.Trend)
	}
}

// get_summary gives the verdict on the phase.
func TestGetSummaryVerdict(t *testing.T) {
	t.Parallel()

	b, _ := summaryOfHistory(t)
	// About 0.45 % a week against a cut of 0.5: on target. The target of
	// 2400 is over the about 1960 kcal the rate takes, so it comes down a
	// step, 100 kcal. Without a height or a lean mass, those guards are
	// off.
	v := b.Verdict
	if v.Rate != "on_target" || v.Action != "lower_kcal" || v.StepKcal != -100 || v.GapKcal > -300 {
		t.Errorf("verdict %+v: on target, a step down", v)
	}
	if v.TargetKcal == nil || *v.TargetKcal != 2400 || !slices.Equal(v.Flags, []string{"no_height", "no_lean_mass"}) {
		t.Errorf("verdict %+v: the target of 2400, no height nor lean mass", v)
	}
}

// get_summary gives the recovery and the days possibly incomplete.
func TestGetSummaryRecovery(t *testing.T) {
	t.Parallel()

	b, today := summaryOfHistory(t)
	if !slices.Equal(b.Recovery.Flags, []string{"short_sleep"}) || b.Recovery.SleepH == nil || *b.Recovery.SleepH != 6 {
		t.Errorf("recovery %+v, want short sleep", b.Recovery)
	}
	if !slices.Equal(b.IncompleteDays, []string{today.AddDays(-3).String()}) {
		t.Errorf("incomplete days %v, want the day of 500 kcal", b.IncompleteDays)
	}
}

// get_summary of a user with nothing says why there is nothing.
func TestGetSummaryOfNothing(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	rec := in.do(t, http.MethodGet, "/api/v1/summary", "", []*http.Cookie{sessionOf(t, in)}, nil)
	var b summaryAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(b.Weeks) != 4 || b.Trend != nil || b.TDEE.Measured || b.TDEE.Kcal != nil ||
		b.Verdict.Rate != "none" || !slices.Equal(b.Verdict.Reasons, []string{"no_phase", "no_target", "no_estimate"}) {
		t.Errorf("summary %+v, want 4 empty weeks and no verdict", b)
	}
	if w := b.Weeks[0]; w.LoggedDays != 0 || w.Kcal != nil || w.WeightKg != nil {
		t.Errorf("an empty week %+v", w)
	}
}

// get_summary takes 1 to 12 weeks.
func TestGetSummaryRefuses(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	for _, q := range []string{"weeks=0", "weeks=13", "weeks=a"} {
		if rec := in.do(t, http.MethodGet, "/api/v1/summary?"+q, "", session, nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s, want 422", q, rec.Code, rec.Body.String())
		}
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/summary", "", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no session: %d, want 401", rec.Code)
	}
}
