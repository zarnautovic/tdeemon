package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// historyDay is a row of a history, as the tests read it.
type historyDay struct {
	Date           string `json:"date"`
	Status         string `json:"status"`
	ExcludedReason string `json:"excluded_reason"`
	Type           string `json:"type"`
	Meals          int    `json:"meals"`
	Intake         *struct{ Kcal, Protein, Carbs, Fat float64 }
	Target         *struct{ Kcal float64 }
	WeightKg       *float64 `json:"weight_kg"`
	TrendKg        *float64 `json:"trend_kg"`
	Steps          *float64 `json:"steps"`
	SleepH         *float64 `json:"sleep_h"`
	TDEE           *struct {
		Kcal        float64 `json:"kcal"`
		HalfWidth   float64 `json:"half_width"`
		Trusted     bool    `json:"trusted"`
		BeforePhase bool    `json:"before_phase"`
		To          string  `json:"to"`
	} `json:"tdee"`
}

// saveHistory gives the user 60 days up to yesterday of a 2000 kcal day and
// a weigh-in that loses 0.05 kg a day, made up; and returns today.
func saveHistory(t *testing.T, in *instance) day.Date {
	t.Helper()
	today := day.DateOf(in.clock.Now())
	saveDays(t, in, today.AddDays(-60), today.AddDays(-1))
	return today
}

// saveDays gives the user the days from from to to of saveHistory: a 2000
// kcal day, and a weigh-in that starts at 85 kg and loses 0.05 kg a day.
func saveDays(t *testing.T, in *instance, from, to day.Date) {
	t.Helper()
	now := in.clock.Now()
	for d := from; !to.Before(d); d = d.AddDays(1) {
		at := time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, time.UTC)
		item, err := meal.Estimate("Plate", food.Nutrients{Kcal: 2000, Protein: 150, Carbs: 200, Fat: 67}, "")
		if err != nil {
			t.Fatal(err)
		}
		m := meal.Meal{ID: uuid.NewV7(), EatenAt: at, Day: d, Client: meal.ClientWeb, Items: []meal.Item{item}}
		if _, _, err := in.db.CreateMeal(t.Context(), in.user.ID, isolationActor, m, now); err != nil {
			t.Fatal(err)
		}
		if _, err := in.db.MergeMetrics(t.Context(), in.user.ID, isolationActor, d, []metric.Value{
			{Kind: metric.Weight, Value: 85 - 0.05*float64(d.DaysSince(from)), Source: metric.Import},
			{Kind: metric.Steps, Value: 8000, Source: metric.Import},
			{Kind: metric.Sleep, Value: 7.25, Source: metric.Import},
		}, now); err != nil {
			t.Fatal(err)
		}
	}
}

func getHistory(t *testing.T, in *instance, session *http.Cookie, query string) (int, []historyDay, string) {
	t.Helper()
	rec := in.do(t, http.MethodGet, "/api/v1/history"+query, "", []*http.Cookie{session}, nil)
	var b struct {
		Days []historyDay `json:"days"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, b.Days, rec.Body.String()
}

// get-history gives every day of a span: what was eaten against the
// target, the measurements, the trend weight and the TDEE up to the day.
func TestGetHistory(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveHistory(t, in)
	saveIsolationPlan(t, in.db, in.user, in.clock.Now())
	if err := in.db.ExcludeDay(t.Context(), in.user.ID, isolationActor, today.AddDays(-3), day.Exclusion{Reason: "restaurant"}, in.clock.Now()); err != nil {
		t.Fatal(err)
	}

	code, days, body := getHistory(t, in, session, "?from="+today.AddDays(-3).String())
	if code != http.StatusOK || len(days) != 4 {
		t.Fatalf("%d %s, want the 4 days up to today", code, body)
	}
	checkHistoryDays(t, days, today)
	if code, days, body := getHistory(t, in, session, ""); code != http.StatusOK || len(days) != 90 || days[89].Date != today.String() {
		t.Errorf("by default: %d, %d days, want the 90 up to today: %s", code, len(days), body[:min(200, len(body))])
	}
}

// A history by week has a row for each calendar week, Monday to Sunday,
// the last up to to.
func TestGetHistoryByWeek(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveHistory(t, in)
	if today != (day.Date{Year: 2026, Month: time.October, Day: 21}) {
		t.Fatalf("today is %s, want Wednesday 21 October", today)
	}
	// From Wednesday the 7th to Tuesday the 20th: the week of the 5th,
	// the 12th's, and the 19th's to the 20th.
	rec := in.do(t, http.MethodGet, "/api/v1/history?granularity=week&from="+today.AddDays(-14).String()+"&to="+today.AddDays(-1).String(),
		"", []*http.Cookie{session}, nil)
	var b struct {
		Days  []historyDay `json:"days"`
		Weeks []struct {
			From       string  `json:"from"`
			To         string  `json:"to"`
			LoggedDays int     `json:"logged_days"`
			Kcal       float64 `json:"kcal"`
		} `json:"weeks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	type week struct {
		from, to string
		logged   int
	}
	var got []week
	for _, w := range b.Weeks {
		got = append(got, week{w.From, w.To, w.LoggedDays})
	}
	want := []week{{"2026-10-05", "2026-10-11", 7}, {"2026-10-12", "2026-10-18", 7}, {"2026-10-19", "2026-10-20", 2}}
	if len(b.Days) != 0 || !slices.Equal(got, want) || b.Weeks[2].Kcal != 2000 {
		t.Errorf("weeks = %+v, want %+v of 2000 kcal", got, want)
	}
}

func TestGetHistoryRefuses(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := day.DateOf(in.clock.Now())
	for name, query := range map[string]string{
		"a day to come":      "?to=" + today.AddDays(1).String(),
		"from after to":      "?from=" + today.String() + "&to=" + today.AddDays(-1).String(),
		"more than 366 days": "?from=" + today.AddDays(-366).String(),
		"no date":            "?from=yesterday",
		"another grain":      "?granularity=month",
		"before 1900":        "?from=1899-12-31&to=1900-01-02",
		"year 0":             "?to=0000-01-01",
	} {
		if code, _, body := getHistory(t, in, session, query); code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s, want 422", name, code, body)
		}
	}
	if code, _, _ := getHistory(t, in, session, "?from="+today.AddDays(-365).String()); code != http.StatusOK {
		t.Errorf("366 days: %d, want 200", code)
	}
	// The tool get_history: a token that reads reads it, an ingest one not.
	tok, created := in.createToken(t, session, `{"name":"claude","scopes":["read"]}`)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/history", "", nil, bearer(tok.Token)); rec.Code != http.StatusOK {
		t.Errorf("with a read token: %d %s, want 200", rec.Code, rec.Body.String())
	}
	phone, created := in.createToken(t, session, `{"name":"phone","scopes":["ingest"]}`)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/history", "", nil, bearer(phone.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("with an ingest token: %d %s, want 403", rec.Code, rec.Body.String())
	}
}

// checkHistoryDays holds the 4 days up to today of TestGetHistory to what
// was saved of them.
func checkHistoryDays(t *testing.T, days []historyDay, today day.Date) {
	t.Helper()
	excluded, yesterday, now := days[0], days[2], days[3]
	if excluded.Status != "excluded" || excluded.ExcludedReason != "restaurant" {
		t.Errorf("the excluded day = %+v", excluded)
	}
	if yesterday.Date != today.AddDays(-1).String() || yesterday.Status != "complete" || yesterday.Meals != 1 ||
		yesterday.Intake == nil || *yesterday.Intake != struct{ Kcal, Protein, Carbs, Fat float64 }{2000, 150, 200, 67} {
		t.Errorf("yesterday = %+v", yesterday)
	}
	checkHistoryMeasures(t, yesterday)
	// Today: open, nothing eaten, its target the plan's; the trend goes on.
	if now.Status != "open" || now.Meals != 0 || now.Intake != nil || now.Target == nil || now.Target.Kcal != 2100 || now.TrendKg == nil {
		t.Errorf("today = %+v", now)
	}
}

// checkHistoryMeasures holds yesterday of TestGetHistory to its
// measurements, its trend weight and the TDEE they give.
func checkHistoryMeasures(t *testing.T, d historyDay) {
	t.Helper()
	if d.WeightKg == nil || *d.WeightKg != 82.05 || d.TrendKg == nil || *d.TrendKg <= 82.05 {
		t.Errorf("the weight and trend = %v, %v; want 82.05 and a trend above it, as the weight falls", d.WeightKg, d.TrendKg)
	}
	if d.Steps == nil || *d.Steps != 8000 || d.SleepH == nil || *d.SleepH != 7.25 {
		t.Errorf("the steps and sleep = %v, %v", d.Steps, d.SleepH)
	}
	// 0.35 kg a week less, on 2000 kcal: a TDEE near 2000 + 0.35 × 7700 / 7.
	if d.TDEE == nil || d.TDEE.Kcal < 2200 || d.TDEE.Kcal > 2600 || d.TDEE.HalfWidth <= 0 {
		t.Errorf("the TDEE = %+v, want one near 2385", d.TDEE)
	}
}

// A week of today leaves today's steps out, which still grow; its other
// days' count.
func TestGetHistoryWeekOfToday(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveHistory(t, in)
	if _, err := in.db.MergeMetrics(t.Context(), in.user.ID, isolationActor, today, []metric.Value{
		{Kind: metric.Steps, Value: 500, Source: metric.Import},
	}, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	rec := in.do(t, http.MethodGet, "/api/v1/history?granularity=week&from="+today.AddDays(-6).String(), "", []*http.Cookie{session}, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"steps":8000`) {
		t.Errorf("%d %s, want the 6 days' 8000 steps", rec.Code, rec.Body.String())
	}
}

// After the intake changes, the TDEE is the one of the days before, for
// some weeks, and says so.
func TestGetHistoryBeforePhase(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveHistory(t, in)
	saveIsolationPlan(t, in.db, in.user, in.clock.Now()) // a phase from today
	_, days, body := getHistory(t, in, session, "?from="+today.String())
	if len(days) != 1 || days[0].TDEE == nil || !days[0].TDEE.BeforePhase || days[0].TDEE.To != today.AddDays(-1).String() {
		t.Errorf("today's TDEE in a phase begun today: %s, want the days before's, said so", body)
	}
}

// Today is not over: its trend weight and TDEE are the summary's, of the
// days up to yesterday, though it has a weigh-in; a window that ended on
// today would count its weigh-in and not its intake, and say another
// TDEE beside the summary's.
func TestGetHistoryTodayIsTheSummarys(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveHistory(t, in)
	// Today's weigh-in, well under the trend.
	if _, err := in.db.MergeMetrics(t.Context(), in.user.ID, isolationActor, today, []metric.Value{
		{Kind: metric.Weight, Value: 80, Source: metric.Manual},
	}, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	code, days, body := getHistory(t, in, session, "?from="+today.AddDays(-1).String())
	if code != http.StatusOK || len(days) != 2 {
		t.Fatalf("%d %s, want yesterday and today", code, body)
	}
	trend, tdee, half := summaryNow(t, in, session)

	yesterday, now := days[0], days[1]
	if now.WeightKg == nil || *now.WeightKg != 80 {
		t.Errorf("today's weigh-in = %v, want 80", now.WeightKg)
	}
	if now.TrendKg == nil || *now.TrendKg != trend {
		t.Errorf("today's trend = %v, want the summary's, %v", now.TrendKg, trend)
	}
	if now.TDEE == nil || now.TDEE.Kcal != tdee || now.TDEE.HalfWidth != half || now.TDEE.To != yesterday.Date {
		t.Errorf("today's TDEE = %+v, want the summary's, %v ± %v, up to yesterday", now.TDEE, tdee, half)
	}
	if yesterday.TDEE == nil || *yesterday.TDEE != *now.TDEE {
		t.Errorf("yesterday's TDEE = %+v, want today's, %+v", yesterday.TDEE, now.TDEE)
	}
}

// summaryNow is the summary's trend weight, and its TDEE with half its
// interval, which the test needs it to have.
func summaryNow(t *testing.T, in *instance, session *http.Cookie) (trendKg, tdee, halfWidth float64) {
	t.Helper()
	rec := in.do(t, http.MethodGet, "/api/v1/summary", "", []*http.Cookie{session}, nil)
	var summary struct {
		Trend *struct {
			Kg float64 `json:"kg"`
		} `json:"trend"`
		TDEE struct {
			Kcal      *float64 `json:"kcal"`
			HalfWidth *float64 `json:"half_width"`
		} `json:"tdee"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); rec.Code != http.StatusOK || err != nil ||
		summary.Trend == nil || summary.TDEE.Kcal == nil || summary.TDEE.HalfWidth == nil {
		t.Fatalf("the summary: %d %s", rec.Code, rec.Body.String())
	}
	return summary.Trend.Kg, *summary.TDEE.Kcal, *summary.TDEE.HalfWidth
}
