package api_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// limitsFile is the web app's copy of the limits it checks a form by
// before sending it, so that it says a refusal in the user's language
// (docs/conventions.md, Languages); relative to this package.
var limitsFile = filepath.Join("..", "..", "web", "src", "lib", "limits.json")

// webLimits are the server's limits the web app checks its forms by.
type webLimits struct {
	// Metrics are each kind's least and most value, in its unit.
	Metrics map[metric.Kind]bounds `json:"metrics"`
	Meal    struct {
		MaxAmount       float64 `json:"maxAmount"`
		MaxServings     float64 `json:"maxServings"`
		MaxPortions     float64 `json:"maxPortions"`
		MaxEstimateKcal float64 `json:"maxEstimateKcal"`
	} `json:"meal"`
	Food struct {
		MaxKcalPer100     float64 `json:"maxKcalPer100"`
		MaxGramsPer100g   float64 `json:"maxGramsPer100g"`
		MaxGramsPer100ml  float64 `json:"maxGramsPer100ml"`
		MaxKcalPerPortion float64 `json:"maxKcalPerPortion"`
	} `json:"food"`
	Profile struct {
		MinHeightCm float64 `json:"minHeightCm"`
		MaxHeightCm float64 `json:"maxHeightCm"`
		MaxAge      int     `json:"maxAge"`
	} `json:"profile"`
	User struct {
		MaxNameLen int `json:"maxNameLen"`
	} `json:"user"`
	// History is the first day a history can be of, the first day the
	// day's page goes back to.
	History struct {
		Since string `json:"since"`
	} `json:"history"`
	// Plan holds, beside the most kcal of a target, the energy of a kg of
	// body weight, by which the progress says what a deficit comes to.
	Plan struct {
		MaxKcal   float64 `json:"maxKcal"`
		KcalPerKg float64 `json:"kcalPerKg"`
	} `json:"plan"`
}

// bounds are the least and the most a value may be.
type bounds struct {
	Least float64 `json:"least"`
	Most  float64 `json:"most"`
}

func newWebLimits() webLimits {
	var l webLimits
	l.Metrics = map[metric.Kind]bounds{}
	for _, k := range metric.Kinds() {
		least, most := k.Bounds()
		l.Metrics[k] = bounds{least, most}
	}
	l.Meal.MaxAmount, l.Meal.MaxServings, l.Meal.MaxPortions = meal.MaxAmount, meal.MaxServings, meal.MaxPortions
	l.Meal.MaxEstimateKcal = meal.MaxEstimateKcal
	l.Food.MaxKcalPer100, l.Food.MaxKcalPerPortion = food.MaxKcalPer100, food.MaxKcalPerPortion
	l.Food.MaxGramsPer100g, l.Food.MaxGramsPer100ml = food.MaxGramsPer100g, food.MaxGramsPer100ml
	l.Profile.MinHeightCm, l.Profile.MaxHeightCm, l.Profile.MaxAge = profile.MinHeightCm, profile.MaxHeightCm, profile.MaxAge
	l.User.MaxNameLen = user.MaxNameLen
	l.History.Since = api.HistorySince.String()
	l.Plan.MaxKcal, l.Plan.KcalPerKg = plan.MaxKcal, plan.KcalPerKg
	return l
}

// TestWebLimitsAreCurrent keeps the web app's copy of the limits equal to
// the server's, so a form checks a value as the server would.
func TestWebLimitsAreCurrent(t *testing.T) {
	t.Parallel()

	got, err := json.MarshalIndent(newWebLimits(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(limitsFile, got, 0o600); err != nil {
			t.Fatalf("write %s: %v", limitsFile, err)
		}
		return
	}
	want, err := os.ReadFile(limitsFile)
	if err != nil {
		t.Fatalf("read %s: %v (create it with -update)", limitsFile, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale; refresh it with: go test ./internal/api -run TestWebLimits -update", limitsFile)
	}
}
