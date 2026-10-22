package plan

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestKgPerWeek(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name               string
		rate, trend, wantK float64
	}{
		{"a cut of 0.5 % at 80 kg", -0.5, 80, -0.4},
		{"a bulk of 0.25 % at 70 kg", 0.25, 70, 0.175},
		{"maintenance", 0, 90, 0},
		// Above about 150 kg, −1.0 % a week is more than 1.5 kg
		// (docs/limits.md).
		{"−1.0 % at 160 kg", -1, 160, -1.6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := KgPerWeek(tc.rate, tc.trend); !near(got, tc.wantK) {
				t.Errorf("KgPerWeek(%v, %v) = %v, want %v", tc.rate, tc.trend, got, tc.wantK)
			}
			if tc.wantK != 0 {
				if got := RatePctWeek(tc.wantK, tc.trend); !near(got, tc.rate) {
					t.Errorf("RatePctWeek(%v, %v) = %v, want %v", tc.wantK, tc.trend, got, tc.rate)
				}
			}
		})
	}
}

func TestImpliedRate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                    string
		avg, tdee, trend, wantR float64
	}{
		// 550 kcal a day under: 3850 kcal a week, 0.5 kg, 0.625 % of 80 kg.
		{"a deficit", 2050, 2600, 80, -0.625},
		// 275 kcal a day over: 0.25 kg a week, 0.3125 % of 80 kg.
		{"a surplus", 2875, 2600, 80, 0.3125},
		{"even", 2600, 2600, 80, 0},
		// Maintenance at 2650 kcal with a TDEE of 2600 implies a slow gain,
		// which the limits of gain allow (docs/limits.md).
		{"50 kcal over", 2650, 2600, 70, 0.05 * 7 / 7.7 / 0.7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ImpliedRate(tc.avg, tc.tdee, tc.trend); !near(got, tc.wantR) {
				t.Errorf("ImpliedRate(%v, %v, %v) = %v, want %v", tc.avg, tc.tdee, tc.trend, got, tc.wantR)
			}
		})
	}
}

func TestKcalPerDay(t *testing.T) {
	t.Parallel()

	// 0.5 % a week of 80 kg is 0.4 kg, 3080 kcal, 440 a day; the inverse
	// of ImpliedRate.
	if got := KcalPerDay(-0.5, 80); !near(got, -440) {
		t.Errorf("KcalPerDay(-0.5, 80) = %v, want -440", got)
	}
	if got := ImpliedRate(2600+KcalPerDay(-0.5, 80), 2600, 80); !near(got, -0.5) {
		t.Errorf("ImpliedRate of KcalPerDay = %v, want -0.5", got)
	}
}
