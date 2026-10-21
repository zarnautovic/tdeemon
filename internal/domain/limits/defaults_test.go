package limits

import (
	"math"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/plan"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestDefaultRate(t *testing.T) {
	t.Parallel()

	user := UserDefaults()
	slow := user
	slow.CutRate, slow.CutRateObese, slow.BulkRate, slow.RecompMin, slow.RecompMax = -0.3, -0.6, 0.1, -0.05, 0.25
	high := user
	high.RecompMin, high.RecompMax = 0, 0.25
	low := user
	low.RecompMin, low.RecompMax = -0.5, -0.2
	for _, tc := range []struct {
		name       string
		t          plan.PhaseType
		bmi, trend float64
		l          Limits
		want       float64
	}{
		{"a cut below a BMI of 30", plan.Cut, 26, 80, user, -0.5},
		{"a cut at a BMI of 30", plan.Cut, 30, 100, user, -0.75},
		// −0.75 % of 160 kg is 1.2 kg a week: it stays.
		{"a cut at 160 kg", plan.Cut, 49, 160, user, -0.75},
		// −0.75 % of 210 kg would be 1.575 kg: cut to 1.5 kg.
		{"a cut at 210 kg", plan.Cut, 60, 210, user, -1.5 / 210 * 100},
		{"a cut, cut to the user's limit", plan.Cut, 26, 80, slow, -0.3},
		{"an obese cut, cut to the user's limit", plan.Cut, 35, 110, slow, -0.6},
		{"a cut without a trend weight", plan.Cut, 26, 0, user, -0.5},
		{"a bulk", plan.Bulk, 22, 70, user, 0.25},
		{"a bulk, cut to the user's limit", plan.Bulk, 22, 70, slow, 0.1},
		{"a recomp", plan.Recomp, 24, 75, user, -0.1},
		{"a recomp, cut to the user's range", plan.Recomp, 24, 75, slow, -0.05},
		{"a recomp, raised to the user's range", plan.Recomp, 24, 75, high, 0},
		{"a recomp, lowered to the user's range", plan.Recomp, 24, 75, low, -0.2},
		{"maintenance", plan.Maintain, 24, 75, user, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DefaultRate(tc.t, tc.bmi, tc.trend, tc.l); !near(got, tc.want) {
				t.Errorf("DefaultRate = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultProtein(t *testing.T) {
	t.Parallel()

	user := UserDefaults()
	narrow := user
	narrow.ProteinFloor, narrow.ProteinCeiling = 1.7, 2.0
	for _, tc := range []struct {
		name string
		t    plan.PhaseType
		l    Limits
		want float64
	}{
		{"a cut", plan.Cut, user, 2.2},
		{"maintenance", plan.Maintain, user, 1.6},
		{"a bulk", plan.Bulk, user, 1.8},
		{"a recomp", plan.Recomp, user, 2.0},
		// A ceiling of 2.0 cuts a cut's 2.2 (docs/limits.md).
		{"a cut under a ceiling", plan.Cut, narrow, 2.0},
		{"maintenance over a floor", plan.Maintain, narrow, 1.7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DefaultProtein(tc.t, tc.l); !near(got, tc.want) {
				t.Errorf("DefaultProtein = %v, want %v", got, tc.want)
			}
		})
	}
}
