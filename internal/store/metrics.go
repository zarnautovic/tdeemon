package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
)

// MetricChange is a value MergeMetrics stored: what the day held of its
// kind before (nil for nothing), and what it holds now. It is what an
// undo puts back.
type MetricChange struct {
	Before *metric.Value
	After  metric.Value
}

// MergeMetrics merges vs, values of userID's logical day d, into what the
// day holds, by metric.Merge, and returns the values that changed, in the
// order of vs. Values that change nothing (an import under a manual value,
// a part of the day's steps) are left out, so a sync sent twice changes
// nothing the second time. What changed is recorded as one event of a's.
func (db *DB) MergeMetrics(ctx context.Context, userID uuid.UUID, a event.Actor, d day.Date, vs []metric.Value, now time.Time) ([]MetricChange, error) {
	if err := metric.ValidateDay(vs); err != nil {
		return nil, fmt.Errorf("invalid metrics: %w", err)
	}
	vs = append([]metric.Value(nil), vs...)
	for i := range vs {
		vs[i].Value += 0 // -0 is 0, as the table reads it back
	}
	if d.IsZero() {
		return nil, errors.New("invalid metrics: the day is missing")
	}
	if err := a.Validate(); err != nil {
		return nil, fmt.Errorf("invalid actor: %w", err)
	}
	var out []MetricChange
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, v := range vs {
			stored, found, err := metricOf(ctx, tx, userID, d, v.Kind)
			if err != nil {
				return err
			}
			var current *metric.Value
			if found {
				current = &stored
			}
			kept, changed := metric.Merge(current, v)
			if !changed {
				continue
			}
			if err := putMetric(ctx, tx, userID, d, kept, now); err != nil {
				return err
			}
			out = append(out, MetricChange{Before: current, After: kept})
		}
		if len(out) == 0 {
			return nil
		}
		before, after := metricsState{}, metricsState{}
		for _, c := range out {
			before[c.After.Kind], after[c.After.Kind] = c.Before, &c.After
		}
		_, err := record(ctx, tx, userID, a, event.MetricsMerge, d.String(), before, after, now)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("merge metrics: %w", err)
	}
	return out, nil
}

// metricOf returns the value of kind k that userID's day d holds, and
// whether it holds one.
func metricOf(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date, k metric.Kind) (metric.Value, bool, error) {
	v := metric.Value{Kind: k}
	var source string
	err := tx.QueryRowContext(ctx, "SELECT value, source FROM metrics WHERE user_id = ? AND day = ? AND metric = ?",
		userID.String(), d.String(), string(k)).Scan(&v.Value, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return metric.Value{}, false, nil
	}
	if err != nil {
		return metric.Value{}, false, err
	}
	v.Source = metric.Source(source)
	return v, true, nil
}

// SetMetric stores v as the value of its kind on userID's day d, whatever
// the day held and the merge rules aside, and returns what it held (nil
// for nothing). It is recorded as a's event. Merging is MergeMetrics.
func (db *DB) SetMetric(ctx context.Context, userID uuid.UUID, a event.Actor, d day.Date, v metric.Value, now time.Time) (*metric.Value, error) {
	if err := v.Validate(); err != nil {
		return nil, fmt.Errorf("invalid metric: %w", err)
	}
	if d.IsZero() {
		return nil, errors.New("invalid metric: the day is missing")
	}
	v.Value += 0 // -0 is 0, as the table reads it back
	if err := a.Validate(); err != nil {
		return nil, fmt.Errorf("invalid actor: %w", err)
	}
	var before *metric.Value
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		stored, found, err := metricOf(ctx, tx, userID, d, v.Kind)
		if err != nil {
			return err
		}
		if found {
			before = &stored
		}
		if err := putMetric(ctx, tx, userID, d, v, now); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.MetricSet, metricEntity(d, v.Kind), before, v, now)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("set metric: %w", err)
	}
	return before, nil
}

// DeleteMetric removes the value of kind k from userID's day d and
// returns it, or returns ErrNotFound. A wrong manual value deleted lets
// imports apply again. It is recorded as a's event.
func (db *DB) DeleteMetric(ctx context.Context, userID uuid.UUID, a event.Actor, d day.Date, k metric.Kind, now time.Time) (metric.Value, error) {
	if d.IsZero() {
		return metric.Value{}, errors.New("delete metric: the day is missing")
	}
	if err := a.Validate(); err != nil {
		return metric.Value{}, fmt.Errorf("invalid actor: %w", err)
	}
	var before metric.Value
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		stored, found, err := metricOf(ctx, tx, userID, d, k)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		before = stored
		if err := dropMetric(ctx, tx, userID, d, k); err != nil {
			return err
		}
		_, err = record(ctx, tx, userID, a, event.MetricDelete, metricEntity(d, k), stored, nil, now)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return metric.Value{}, err
	}
	if err != nil {
		return metric.Value{}, fmt.Errorf("delete metric: %w", err)
	}
	return before, nil
}

// Metrics returns the values of userID's days from from to to, both
// included, each day's in the order of metric.Kinds.
func (db *DB) Metrics(ctx context.Context, userID uuid.UUID, from, to day.Date) (map[day.Date][]metric.Value, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT day, metric, value, source FROM metrics
		WHERE user_id = ? AND day BETWEEN ? AND ? ORDER BY day`, userID.String(), from.String(), to.String())
	if err != nil {
		return nil, fmt.Errorf("metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byDay := map[day.Date]map[metric.Kind]metric.Value{}
	for rows.Next() {
		var d, kind, source string
		var v float64
		if err := rows.Scan(&d, &kind, &v, &source); err != nil {
			return nil, fmt.Errorf("metrics: %w", err)
		}
		date, err := day.ParseDate(d)
		if err != nil {
			return nil, fmt.Errorf("metrics: %w", err)
		}
		if byDay[date] == nil {
			byDay[date] = map[metric.Kind]metric.Value{}
		}
		byDay[date][metric.Kind(kind)] = metric.Value{Kind: metric.Kind(kind), Value: v, Source: metric.Source(source)}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metrics: %w", err)
	}
	out := make(map[day.Date][]metric.Value, len(byDay))
	for d, vs := range byDay {
		for _, k := range metric.Kinds() {
			if v, ok := vs[k]; ok {
				out[d] = append(out[d], v)
			}
		}
	}
	return out, nil
}

// WeighIns returns userID's weigh-ins of the days up to to, by day: the
// whole history's, which a trend weight reads.
func (db *DB) WeighIns(ctx context.Context, userID uuid.UUID, to day.Date) (map[day.Date]float64, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT day, value FROM metrics
		WHERE user_id = ? AND metric = ? AND day <= ? ORDER BY day`, userID.String(), string(metric.Weight), to.String())
	if err != nil {
		return nil, fmt.Errorf("weigh-ins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[day.Date]float64{}
	for rows.Next() {
		var d string
		var v float64
		if err := rows.Scan(&d, &v); err != nil {
			return nil, fmt.Errorf("weigh-ins: %w", err)
		}
		date, err := day.ParseDate(d)
		if err != nil {
			return nil, fmt.Errorf("weigh-ins: %w", err)
		}
		out[date] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("weigh-ins: %w", err)
	}
	return out, nil
}

// metricsState is a metrics.merge event's state: the value of each kind
// that changed, nil where the day had none.
type metricsState map[metric.Kind]*metric.Value

// metricEntity names a day's metric in an event: "2026-10-21/weight".
func metricEntity(d day.Date, k metric.Kind) string { return d.String() + "/" + string(k) }

// putMetric stores v as userID's day d's value of its kind.
func putMetric(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date, v metric.Value, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO metrics (user_id, day, metric, value, source, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, day, metric) DO UPDATE SET
			value = excluded.value, source = excluded.source, updated_at = excluded.updated_at`,
		userID.String(), d.String(), string(v.Kind), v.Value, string(v.Source), formatTime(now))
	return err
}

func dropMetric(ctx context.Context, tx *sql.Tx, userID uuid.UUID, d day.Date, k metric.Kind) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM metrics WHERE user_id = ? AND day = ? AND metric = ?",
		userID.String(), d.String(), string(k))
	return err
}

// metricsStateNow returns the day's values of the kinds e changed, as e
// records them: the value (nil when none) of a single kind, or each kind's.
func metricsStateNow(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event) (any, error) {
	dayPart, kind, single := strings.Cut(e.Entity, "/")
	d, err := entityDay(dayPart)
	if err != nil {
		return nil, err
	}
	if single {
		v, found, err := metricOf(ctx, tx, userID, d, metric.Kind(kind))
		if err != nil || !found {
			return nil, err
		}
		return v, nil
	}
	var after metricsState
	if err := decode(e.After, &after); err != nil {
		return nil, err
	}
	now := metricsState{}
	for k := range after {
		v, found, err := metricOf(ctx, tx, userID, d, k)
		if err != nil {
			return nil, err
		}
		if found {
			now[k] = &v
		} else {
			now[k] = nil
		}
	}
	return now, nil
}

// takeBackMetrics puts the day's values of e back as they were: each
// value e changed as before, or gone where the day had none.
func takeBackMetrics(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e event.Event, now time.Time) error {
	dayPart, kind, single := strings.Cut(e.Entity, "/")
	d, err := entityDay(dayPart)
	if err != nil {
		return err
	}
	before := metricsState{}
	if single {
		var v *metric.Value
		if e.Before != nil {
			if err := decode(e.Before, &v); err != nil {
				return err
			}
		}
		before[metric.Kind(kind)] = v
	} else if err := decode(e.Before, &before); err != nil {
		return err
	}
	for k, v := range before {
		if v == nil {
			err = dropMetric(ctx, tx, userID, d, k)
		} else {
			err = putMetric(ctx, tx, userID, d, *v, now)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
