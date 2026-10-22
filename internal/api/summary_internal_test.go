package api

import (
	"reflect"
	"strconv"
	"testing"
)

// The input's maximum of weeks is summaryWeeks, the weeks whose days the
// summary reads in full: a week past them would read days it never
// loaded.
func TestSummaryWeeksIsTheInputsMaximum(t *testing.T) {
	t.Parallel()

	f, _ := reflect.TypeFor[summaryInput]().FieldByName("Weeks")
	if got := f.Tag.Get("maximum"); got != strconv.Itoa(summaryWeeks) {
		t.Errorf("the input's maximum is %s, summaryWeeks %d", got, summaryWeeks)
	}
}
