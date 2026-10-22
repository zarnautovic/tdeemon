package api

import (
	"errors"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
)

// An op not worked out, as the meal read had its item as sent, finds the
// meal changed by the time the change is made: done already when the item
// is still as sent, a conflict when it is gone or another.
func TestApplyOpOfAnItemChangedMeanwhile(t *testing.T) {
	t.Parallel()

	tea, err := meal.Estimate("Tea", food.Nutrients{Kcal: 2, Carbs: 0.3}, "")
	if err != nil {
		t.Fatal(err)
	}
	tea.ID = uuid.NewV7()
	soup, err := meal.Estimate("Soup", food.Nutrients{Kcal: 180, Protein: 6, Carbs: 20, Fat: 8}, "")
	if err != nil {
		t.Fatal(err)
	}
	soup.ID = tea.ID
	sent := opItem{sent: tea}
	add := &mealOp{Op: "add"}
	replace := &mealOp{Op: "replace", ItemID: tea.ID.String()}
	for _, tc := range []struct {
		name  string
		op    *mealOp
		items []meal.Item
		want  error
	}{
		{"an add, the item still there", add, []meal.Item{tea}, nil},
		{"an add, the item removed", add, nil, errChanged},
		{"a replace, the item still there", replace, []meal.Item{tea}, nil},
		{"a replace, the item replaced", replace, []meal.Item{soup}, errChanged},
	} {
		m := meal.Meal{Items: tc.items}
		if err := applyOp(&m, tc.op, sent); !errors.Is(err, tc.want) {
			t.Errorf("%s: applyOp = %v, want %v", tc.name, err, tc.want)
		}
	}
}
