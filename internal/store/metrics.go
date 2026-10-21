package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
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
// nothing the second time.
func (db *DB) MergeMetrics(ctx context.Context, userID uuid.UUID, d day.Date, vs []metric.Value, now time.Time) ([]MetricChange, error) {
	if err := metric.ValidateDay(vs); err != nil {
		return nil, fmt.Errorf("invalid metrics: %w", err)
	}
	if d.IsZero() {
		return nil, errors.New("invalid metrics: the day is missing")
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
			if _, err := tx.ExecContext(ctx, `INSERT INTO metrics (user_id, day, metric, value, source, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT (user_id, day, metric) DO UPDATE SET
					value = excluded.value, source = excluded.source, updated_at = excluded.updated_at`,
				userID.String(), d.String(), string(kept.Kind), kept.Value, string(kept.Source), formatTime(now)); err != nil {
				return err
			}
			out = append(out, MetricChange{Before: current, After: kept})
		}
		return nil
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
// the day held, and returns what it held (nil for nothing): an undo
// putting back what was there, recorded with what it replaced. Merging is
// MergeMetrics.
func (db *DB) SetMetric(ctx context.Context, userID uuid.UUID, d day.Date, v metric.Value, now time.Time) (*metric.Value, error) {
	if err := v.Validate(); err != nil {
		return nil, fmt.Errorf("invalid metric: %w", err)
	}
	if d.IsZero() {
		return nil, errors.New("invalid metric: the day is missing")
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
		_, err = tx.ExecContext(ctx, `INSERT INTO metrics (user_id, day, metric, value, source, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (user_id, day, metric) DO UPDATE SET
				value = excluded.value, source = excluded.source, updated_at = excluded.updated_at`,
			userID.String(), d.String(), string(v.Kind), v.Value, string(v.Source), formatTime(now))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("set metric: %w", err)
	}
	return before, nil
}

// DeleteMetric removes the value of kind k from userID's day d and
// returns it, or returns ErrNotFound. A wrong manual value deleted lets
// imports apply again.
func (db *DB) DeleteMetric(ctx context.Context, userID uuid.UUID, d day.Date, k metric.Kind) (metric.Value, error) {
	if d.IsZero() {
		return metric.Value{}, errors.New("delete metric: the day is missing")
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
		_, err = tx.ExecContext(ctx, "DELETE FROM metrics WHERE user_id = ? AND day = ? AND metric = ?",
			userID.String(), d.String(), string(k))
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
