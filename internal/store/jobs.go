package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// Users returns every user, in the order they were created.
func (db *DB) Users(ctx context.Context) ([]user.User, error) {
	rows, err := db.read.QueryContext(ctx, "SELECT "+userColumns+" FROM users ORDER BY created_at, id")
	if err != nil {
		return nil, fmt.Errorf("users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []user.User
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("users: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("users: %w", err)
	}
	return out, nil
}

// JobRan reports whether the scheduler has recorded a run of job for
// occurrence for userID.
func (db *DB) JobRan(ctx context.Context, userID uuid.UUID, job, occurrence string) (bool, error) {
	var ran bool
	err := db.read.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM job_runs WHERE user_id = ? AND job = ? AND occurrence = ?)",
		userID.String(), job, occurrence).Scan(&ran)
	if err != nil {
		return false, fmt.Errorf("job ran: %w", err)
	}
	return ran, nil
}

// RecordJobRun records that job ran for occurrence for userID at now; a
// run recorded already stays as it was.
func (db *DB) RecordJobRun(ctx context.Context, userID uuid.UUID, job, occurrence string, now time.Time) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO job_runs (user_id, job, occurrence, ran_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING",
			userID.String(), job, occurrence, formatTime(now))
		if err != nil {
			return fmt.Errorf("record job run: %w", err)
		}
		return nil
	})
}

// Optimize refreshes the query planner's statistics where they are stale,
// as SQLite advises a long-lived connection to do now and then (PRAGMA
// optimize); Migrate does it at the start.
func (db *DB) Optimize(ctx context.Context) error {
	return db.withSingleUseConn(ctx, func(conn *sql.Conn) error { return optimize(ctx, conn) })
}

// optimize runs PRAGMA optimize on conn for every table, not only those
// the connection's queries used (0x10000): conn is a fresh connection,
// and the reads go through the read pool. analysis_limit keeps ANALYZE of
// a large table quick, as the SQLite documentation advises.
func optimize(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, "PRAGMA analysis_limit = 400; PRAGMA optimize = 0x10002")
	return err
}
