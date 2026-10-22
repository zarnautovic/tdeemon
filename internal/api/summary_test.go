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
	for _, tc := range []struct {
		name, query string
		session     []*http.Cookie
		want        int
	}{
		{"no week", "weeks=0", session, http.StatusUnprocessableEntity},
		{"13 weeks", "weeks=13", session, http.StatusUnprocessableEntity},
		{"weeks that are no number", "weeks=a", session, http.StatusUnprocessableEntity},
		{"no session", "", nil, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if rec := in.do(t, http.MethodGet, "/api/v1/summary?"+tc.query, "", tc.session, nil); rec.Code != tc.want {
				t.Errorf("%d %s, want %d", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
}

// summaryOf returns the summary of in's user, of the query q.
func summaryOf(t *testing.T, in *instance, q string) summaryAnswer {
	t.Helper()
	rec := in.do(t, http.MethodGet, "/api/v1/summary?"+q, "", []*http.Cookie{sessionOf(t, in)}, nil)
	var b summaryAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	return b
}

// get_summary reads a day as calibration does: its meals by their day,
// an excluded day's weigh-in as none, a value of 0 as none.
func TestGetSummaryReadsTheDays(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	saveSummaryHistory(t, in.db, in.user, in.clock.Now())
	saveDaysToRead(t, in)
	b := summaryOf(t, in, "weeks=1")
	// Six days logged, yesterday with the snack: 2300, 500 and four of
	// 2000; six weigh-ins; the steps of five days, as the excluded day's
	// count for nothing; the sleep of six.
	w := b.Weeks[0]
	if w.LoggedDays != 6 || w.Kcal == nil || *w.Kcal != 1800 || w.WeighIns != 6 {
		t.Errorf("the week %+v: 6 days of 1800 kcal on average, 6 weigh-ins", w)
	}
	if w.Steps == nil || *w.Steps != 7000 || w.SleepH == nil || *w.SleepH != 6 || b.Recovery.SleepH == nil || *b.Recovery.SleepH != 6 {
		t.Errorf("the week %+v, recovery %+v: 7000 steps and 6 hours of sleep, without the zeros", w, b.Recovery)
	}
}

// saveDaysToRead adds to saveSummaryHistory's days a snack of 300 kcal
// yesterday by its day but eaten today by the calendar, a day excluded
// two days ago with 1000 steps, and a night of 0 hours and a day of 0
// steps four days ago.
func saveDaysToRead(t *testing.T, in *instance) {
	t.Helper()
	now := in.clock.Now()
	ctx := t.Context()
	cal, err := day.NewCalendar(in.user.TimeZone, in.user.DayStartHour)
	if err != nil {
		t.Fatal(err)
	}
	today := cal.DayOf(now)
	// Yesterday's by its day, though eaten after yesterday ended as the
	// calendar has it now: as after the day's start hour moved earlier.
	snack, err := meal.Estimate("a late snack", food.Nutrients{Kcal: 300, Protein: 10, Carbs: 40, Fat: 10}, "")
	if err != nil {
		t.Fatal(err)
	}
	at := cal.Start(today).Add(time.Minute)
	m := meal.Meal{ID: meal.NewIDAt(at), EatenAt: at, Day: today.AddDays(-1), Description: "a late snack", Client: meal.ClientMCP, Items: []meal.Item{snack}}
	if _, _, err := in.db.CreateMeal(ctx, in.user.ID, isolationActor, m, now); err != nil {
		t.Fatal(err)
	}
	if err := in.db.ExcludeDay(ctx, in.user.ID, isolationActor, today.AddDays(-2), day.Exclusion{Reason: "a fever"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := in.db.MergeMetrics(ctx, in.user.ID, isolationActor, today.AddDays(-2), []metric.Value{{Kind: metric.Steps, Value: 1000, Source: metric.Manual}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := in.db.MergeMetrics(ctx, in.user.ID, isolationActor, today.AddDays(-4), []metric.Value{
		{Kind: metric.Sleep, Value: 0, Source: metric.Manual},
		{Kind: metric.Steps, Value: 0, Source: metric.Manual},
	}, now); err != nil {
		t.Fatal(err)
	}
}

// get_summary flags a maintenance phase's drift from the trend weight it
// started with, though it started more than summaryHistory days ago.
func TestGetSummaryDriftOfAnOldPhase(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	now := in.clock.Now()
	ctx := t.Context()
	today := day.DateOf(now)
	start := today.AddDays(-250)
	if err := in.db.SetPhase(ctx, in.user.ID, isolationActor, plan.Phase{Type: plan.Maintain, Start: start}, now); err != nil {
		t.Fatal(err)
	}
	d := plan.DayTarget{Kcal: 2400, Protein: 160, Fat: 70}
	if err := in.db.SetTarget(ctx, in.user.ID, isolationActor, plan.Target{EffectiveFrom: start, Training: d, Rest: d}, now); err != nil {
		t.Fatal(err)
	}
	// 80 kg up to 191 days ago, then 83, the last weigh-in 8 days ago.
	for i := 260; i >= 8; i-- {
		kg := 83.0
		if i > 190 {
			kg = 80
		}
		if _, err := in.db.MergeMetrics(ctx, in.user.ID, isolationActor, today.AddDays(-i), []metric.Value{{Kind: metric.Weight, Value: kg, Source: metric.Import}}, now); err != nil {
			t.Fatal(err)
		}
	}
	b := summaryOf(t, in, "weeks=1")
	if !slices.Contains(b.Verdict.Flags, "maintenance_drift") {
		t.Errorf("flags %v, want maintenance_drift: 83 kg against the 80 the phase started with", b.Verdict.Flags)
	}
	// The last week has no weigh-in, so no trend of its own.
	if b.Weeks[0].TrendKg != nil || b.Trend == nil || b.Trend.Date != today.AddDays(-8).String() {
		t.Errorf("week %+v, trend %+v: no trend for the week, the trend of 8 days ago", b.Weeks[0], b.Trend)
	}
}

// get_summary's window starts at the user's first meal of the whole
// history, though it is older than the days the summary reads in full:
// a user back to logging for 21 days, after a meal 300 days ago, has 21
// logged days in a window of 42, too few a week.
func TestGetSummaryWindowStartsAtTheFirstMeal(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	now := in.clock.Now()
	ctx := t.Context()
	cal, err := day.NewCalendar(in.user.TimeZone, in.user.DayStartHour)
	if err != nil {
		t.Fatal(err)
	}
	today := cal.DayOf(now)
	start := today.AddDays(-300)
	if err := in.db.SetPhase(ctx, in.user.ID, isolationActor, plan.Phase{Type: plan.Cut, Start: start, RatePctWeek: -0.5}, now); err != nil {
		t.Fatal(err)
	}
	d := plan.DayTarget{Kcal: 2400, Protein: 160, Fat: 70}
	if err := in.db.SetTarget(ctx, in.user.ID, isolationActor, plan.Target{EffectiveFrom: start, Training: d, Rest: d}, now); err != nil {
		t.Fatal(err)
	}
	logDay := func(on day.Date) {
		item, err := meal.Estimate("a day's food", food.Nutrients{Kcal: 2000, Protein: 150, Carbs: 250, Fat: 50}, "")
		if err != nil {
			t.Fatal(err)
		}
		at := cal.Start(on).Add(8 * time.Hour)
		m := meal.Meal{ID: meal.NewIDAt(at), EatenAt: at, Day: on, Description: "the day's food", Client: meal.ClientMCP, Items: []meal.Item{item}}
		if _, _, err := in.db.CreateMeal(ctx, in.user.ID, isolationActor, m, now); err != nil {
			t.Fatal(err)
		}
	}
	logDay(start)
	for i := 1; i <= 60; i++ {
		if i <= 21 {
			logDay(today.AddDays(-i))
		}
		if _, err := in.db.MergeMetrics(ctx, in.user.ID, isolationActor, today.AddDays(-i), []metric.Value{
			{Kind: metric.Weight, Value: 80 + 0.05*float64(i), Source: metric.Import},
		}, now); err != nil {
			t.Fatal(err)
		}
	}
	if b := summaryOf(t, in, "weeks=1"); !slices.Contains(b.TDEE.Doubts, "few_logged_days") {
		t.Errorf("TDEE %+v, want the doubt few_logged_days: 21 logged days in a window of 42", b.TDEE)
	}
}
