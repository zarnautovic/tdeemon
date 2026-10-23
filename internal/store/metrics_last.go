package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"uuid"
)

// LastMetricUpdate returns when a metric of userID last changed, by the
// user or an import, and whether one ever did.
func (db *DB) LastMetricUpdate(ctx context.Context, userID uuid.UUID) (time.Time, bool, error) {
	var last sql.NullString
	err := db.read.QueryRowContext(ctx, "SELECT max(updated_at) FROM metrics WHERE user_id = ?", userID.String()).Scan(&last)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("last metric update: %w", err)
	}
	if !last.Valid {
		return time.Time{}, false, nil
	}
	t, err := time.Parse(time.RFC3339, last.String)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("last metric update: %w", err)
	}
	return t, true, nil
}
