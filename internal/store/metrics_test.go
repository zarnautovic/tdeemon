package store_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/store"
)

func TestMergeMetrics(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	got, err := db.MergeMetrics(t.Context(), ana, testActor, d, []metric.Value{
		{Kind: metric.Weight, Value: 82.4, Source: metric.Manual},
		{Kind: metric.Steps, Value: 5120, Source: metric.Import},
		{Kind: metric.Sleep, Value: 7.6, Source: metric.Import},
	}, t0)
	if err != nil || len(got) != 3 || got[0].Before != nil {
		t.Fatalf("first merge = %+v, %v", got, err)
	}
	// A phone sync of part of the day: the import's weight loses to the
	// manual one, the fragment of steps and sleep changes nothing, the
	// HRV is new.
	got, err = db.MergeMetrics(t.Context(), ana, testActor, d, []metric.Value{
		{Kind: metric.Weight, Value: 83.1, Source: metric.Import},
		{Kind: metric.Steps, Value: 250, Source: metric.Import},
		{Kind: metric.Sleep, Value: 3.9, Source: metric.Import},
		{Kind: metric.HRV, Value: 48, Source: metric.Import},
	}, t0)
	if err != nil || len(got) != 1 || got[0].After.Kind != metric.HRV {
		t.Errorf("a partial sync = %+v, %v; want only the HRV changed", got, err)
	}
	// More steps later in the day apply, with what was there before.
	got, err = db.MergeMetrics(t.Context(), ana, testActor, d, []metric.Value{{Kind: metric.Steps, Value: 5480, Source: metric.Import}}, t0)
	want := []store.MetricChange{{
		Before: &metric.Value{Kind: metric.Steps, Value: 5120, Source: metric.Import},
		After:  metric.Value{Kind: metric.Steps, Value: 5480, Source: metric.Import},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("more steps = %+v, %v; want %+v", got, err, want)
	}
	days, err := db.Metrics(t.Context(), ana, d, d)
	wantDay := []metric.Value{
		{Kind: metric.Weight, Value: 82.4, Source: metric.Manual},
		{Kind: metric.Steps, Value: 5480, Source: metric.Import},
		{Kind: metric.Sleep, Value: 7.6, Source: metric.Import},
		{Kind: metric.HRV, Value: 48, Source: metric.Import},
	}
	if err != nil || len(days) != 1 || !reflect.DeepEqual(days[d], wantDay) {
		t.Errorf("Metrics = %+v, %v; want %+v", days, err, wantDay)
	}
}

func TestMergeMetricsRefuses(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	for name, vs := range map[string][]metric.Value{
		"an unknown kind": {{Kind: "mood", Value: 3, Source: metric.Manual}},
		"out of bounds":   {{Kind: metric.Sleep, Value: 450, Source: metric.Import}},
		"a kind twice":    {{Kind: metric.Steps, Value: 1, Source: metric.Import}, {Kind: metric.Steps, Value: 2, Source: metric.Import}},
	} {
		if _, err := db.MergeMetrics(t.Context(), ana, testActor, d, vs, t0); err == nil || !strings.Contains(err.Error(), "invalid metrics") {
			t.Errorf("%s = %v", name, err)
		}
	}
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, day.Date{}, []metric.Value{{Kind: metric.Steps, Value: 1, Source: metric.Import}}, t0); err == nil ||
		!strings.Contains(err.Error(), "the day is missing") {
		t.Errorf("metrics without a day = %v", err)
	}
	if n := countRows(t, db, "metrics"); n != 0 {
		t.Errorf("%d rows stored", n)
	}
}

func TestMetricsBelongToTheirUser(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	d := day.DateOf(t0)
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, d, []metric.Value{{Kind: metric.Weight, Value: 70, Source: metric.Manual}}, t0); err != nil {
		t.Fatal(err)
	}
	// Ivo's import is his: ana's manual weight does not keep it out.
	got, err := db.MergeMetrics(t.Context(), ivo, testActor, d, []metric.Value{{Kind: metric.Weight, Value: 90, Source: metric.Import}}, t0)
	if err != nil || len(got) != 1 || got[0].Before != nil {
		t.Errorf("ivo's weight = %+v, %v", got, err)
	}
	if days, _ := db.Metrics(t.Context(), ana, d, d); len(days[d]) != 1 || days[d][0].Value != 70 {
		t.Errorf("ana's day = %+v", days)
	}
	if _, err := db.DeleteMetric(t.Context(), ivo, testActor, d.AddDays(1), metric.Weight, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleting what is not there = %v", err)
	}
	if before, err := db.DeleteMetric(t.Context(), ivo, testActor, d, metric.Weight, t0); err != nil || before.Value != 90 {
		t.Fatalf("ivo's delete = %+v, %v; want his weight back", before, err)
	}
	if days, _ := db.Metrics(t.Context(), ana, d, d); len(days[d]) != 1 {
		t.Errorf("ivo's delete reached ana's day: %+v", days)
	}
}

// SetMetric puts a value back whatever the day holds: an undo.
func TestSetMetric(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, d, []metric.Value{{Kind: metric.Steps, Value: 9000, Source: metric.Import}}, t0); err != nil {
		t.Fatal(err)
	}
	back := metric.Value{Kind: metric.Steps, Value: 4000, Source: metric.Import}
	before, err := db.SetMetric(t.Context(), ana, testActor, d, back, t0)
	if err != nil || before == nil || before.Value != 9000 {
		t.Fatalf("SetMetric = %+v, %v; want the 9000 steps it replaced", before, err)
	}
	if before, err := db.SetMetric(t.Context(), ana, testActor, d.AddDays(1), back, t0); err != nil || before != nil {
		t.Errorf("SetMetric on an empty day = %+v, %v; want nothing before", before, err)
	}
	if days, _ := db.Metrics(t.Context(), ana, d, d); !reflect.DeepEqual(days[d], []metric.Value{back}) {
		t.Errorf("after SetMetric = %+v", days[d])
	}
	if _, err := db.SetMetric(t.Context(), ana, testActor, d, metric.Value{Kind: metric.Steps, Value: -1, Source: metric.Import}, t0); err == nil {
		t.Error("an invalid value was set")
	}
}

// Every kind and source goes through the table, at both ends of its
// bounds: the table's CHECKs list what the domain does.
func TestEveryMetricKindIsStored(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	for i, src := range []metric.Source{metric.Manual, metric.Import} {
		for j, lowest := range []bool{true, false} {
			var vs []metric.Value
			for _, k := range metric.Kinds() {
				lo, hi := k.Bounds()
				v := hi
				if lowest {
					v = lo
				}
				vs = append(vs, metric.Value{Kind: k, Value: v, Source: src})
			}
			on := d.AddDays(2*i + j)
			if _, err := db.MergeMetrics(t.Context(), ana, testActor, on, vs, t0); err != nil {
				t.Fatalf("%s, lowest %v: %v", src, lowest, err)
			}
			got, err := db.Metrics(t.Context(), ana, on, on)
			if err != nil || !reflect.DeepEqual(got[on], vs) {
				t.Errorf("%s, lowest %v: read back %+v, %v; want %+v", src, lowest, got[on], err, vs)
			}
		}
	}
}

// A wrong manual value deleted is gone, and the next import applies.
func TestDeleteMetricLetsImportsApply(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	wrong := metric.Value{Kind: metric.Weight, Value: 28.4, Source: metric.Manual} // a typo for 82.4
	steps := metric.Value{Kind: metric.Steps, Value: 5120, Source: metric.Import}
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, d, []metric.Value{wrong, steps}, t0); err != nil {
		t.Fatal(err)
	}
	other := metric.Value{Kind: metric.Weight, Value: 82.9, Source: metric.Manual}
	if _, err := db.MergeMetrics(t.Context(), ana, testActor, d.AddDays(1), []metric.Value{other}, t0); err != nil {
		t.Fatal(err)
	}
	if before, err := db.DeleteMetric(t.Context(), ana, testActor, d, metric.Weight, t0); err != nil || before != wrong {
		t.Fatalf("DeleteMetric = %+v, %v; want the wrong value back", before, err)
	}
	// Only that kind of that day goes: the day's steps and the next day's
	// weight stay.
	days, err := db.Metrics(t.Context(), ana, d, d.AddDays(1))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(days[d], []metric.Value{steps}) || !reflect.DeepEqual(days[d.AddDays(1)], []metric.Value{other}) {
		t.Errorf("after the delete the days hold %+v", days)
	}
	scale := metric.Value{Kind: metric.Weight, Value: 82.5, Source: metric.Import}
	got, err := db.MergeMetrics(t.Context(), ana, testActor, d, []metric.Value{scale}, t0)
	if err != nil || len(got) != 1 || got[0].Before != nil || got[0].After != scale {
		t.Errorf("the next import = %+v, %v; want it applied over nothing", got, err)
	}
}

// SetMetric and DeleteMetric need a day, as MergeMetrics does.
func TestSetAndDeleteMetricNeedADay(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	scale := metric.Value{Kind: metric.Weight, Value: 82.5, Source: metric.Import}
	if _, err := db.DeleteMetric(t.Context(), ana, testActor, day.Date{}, metric.Weight, t0); err == nil || !strings.Contains(err.Error(), "the day is missing") {
		t.Errorf("DeleteMetric without a day = %v", err)
	}
	if _, err := db.SetMetric(t.Context(), ana, testActor, day.Date{}, scale, t0); err == nil || !strings.Contains(err.Error(), "the day is missing") {
		t.Errorf("SetMetric without a day = %v", err)
	}
}

func TestMetricsAcrossDays(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	for i := range 5 {
		if _, err := db.MergeMetrics(t.Context(), ana, testActor, d.AddDays(i), []metric.Value{
			{Kind: metric.Steps, Value: float64(1000 * i), Source: metric.Import},
		}, t0); err != nil {
			t.Fatal(err)
		}
	}
	days, err := db.Metrics(t.Context(), ana, d.AddDays(1), d.AddDays(3))
	if err != nil || len(days) != 3 || days[d.AddDays(2)][0].Value != 2000 {
		t.Errorf("Metrics of three days = %+v, %v", days, err)
	}
}

func TestWeighIns(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	d := day.DateOf(t0)
	for i := range 4 {
		if _, err := db.MergeMetrics(t.Context(), ana, testActor, d.AddDays(-400*i), []metric.Value{
			{Kind: metric.Weight, Value: 80 + float64(i), Source: metric.Import},
			{Kind: metric.Steps, Value: 5000, Source: metric.Import},
		}, t0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.MergeMetrics(t.Context(), ivo, testActor, d.AddDays(-1), []metric.Value{{Kind: metric.Weight, Value: 70, Source: metric.Import}}, t0); err != nil {
		t.Fatal(err)
	}
	// Up to the day before d: three of ana's, years apart, and no steps.
	got, err := db.WeighIns(t.Context(), ana, d.AddDays(-1))
	if err != nil || len(got) != 3 || got[d.AddDays(-1200)] != 83 || got[d.AddDays(-400)] != 81 {
		t.Errorf("WeighIns = %v, %v; want ana's three weigh-ins before d", got, err)
	}
}

func TestMetricStoreFailures(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithoutCancel(t.Context())
	d := day.DateOf(t0)
	v := metric.Value{Kind: metric.Steps, Value: 1, Source: metric.Import}
	if _, err := db.MergeMetrics(ctx, ana, testActor, d, []metric.Value{v}, t0); err == nil {
		t.Error("MergeMetrics on a closed database succeeded")
	}
	if _, err := db.SetMetric(ctx, ana, testActor, d, v, t0); err == nil {
		t.Error("SetMetric on a closed database succeeded")
	}
	if _, err := db.DeleteMetric(ctx, ana, testActor, d, metric.Steps, t0); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Errorf("DeleteMetric on a closed database = %v", err)
	}
	if _, err := db.Metrics(ctx, ana, d, d); err == nil {
		t.Error("Metrics on a closed database succeeded")
	}
}
