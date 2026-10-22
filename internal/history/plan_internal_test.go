package history

import (
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// The trend stand-in is the mean of the last week with weigh-ins, within
// 30 days, the store's over the ones given for the same day.
func TestTrendKg(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	u := user.User{ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser, Locale: "en", TimeZone: "UTC", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatal(err)
	}
	today := day.Date{Year: 2026, Month: 10, Day: 22}
	put := func(d day.Date, kg float64) {
		t.Helper()
		if _, err := db.MergeMetrics(t.Context(), u.ID, Actor, d, []metric.Value{{Kind: metric.Weight, Value: kg, Source: metric.Import}}, now); err != nil {
			t.Fatal(err)
		}
	}
	put(today.AddDays(-40), 60)
	if got, err := trendKg(t.Context(), db, u, today, map[day.Date]float64{today.AddDays(-31): 61}); err != nil || got != 0 {
		t.Errorf("only weigh-ins older than 30 days: %v, %v", got, err)
	}
	put(today.AddDays(-10), 100)
	put(today.AddDays(-3), 80)
	put(today.AddDays(-2), 82)
	got, err := trendKg(t.Context(), db, u, today, map[day.Date]float64{today.AddDays(-2): 90, today.AddDays(-1): 84})
	if err != nil || got != 82 {
		t.Errorf("trend = %v, %v; want the last week's mean, 82", got, err)
	}
}
