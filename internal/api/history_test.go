package api_test

import (
	"encoding/json"
	"net/http"
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
	now := in.clock.Now()
	today := day.DateOf(now)
	for i := 60; i >= 1; i-- {
		d := today.AddDays(-i)
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
			{Kind: metric.Weight, Value: 85 - 0.05*float64(60-i), Source: metric.Import},
			{Kind: metric.Steps, Value: 8000, Source: metric.Import},
			{Kind: metric.Sleep, Value: 7.25, Source: metric.Import},
		}, now); err != nil {
			t.Fatal(err)
		}
	}
	return today
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

// A history by week has a row for each 7 days ending on to.
func TestGetHistoryByWeek(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	today := saveHistory(t, in)
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
	if len(b.Days) != 0 || len(b.Weeks) != 2 || b.Weeks[1].To != today.AddDays(-1).String() || b.Weeks[0].From != today.AddDays(-14).String() ||
		b.Weeks[1].LoggedDays != 7 || b.Weeks[1].Kcal != 2000 {
		t.Errorf("weeks = %+v", b)
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
	tok, created := in.createToken(t, session, `{"name":"phone","scopes":["read","tools"]}`)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/history", "", nil, bearer(tok.Token)); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "closed to API tokens") {
		t.Errorf("with a token: %d %s, want 403", rec.Code, rec.Body.String())
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
