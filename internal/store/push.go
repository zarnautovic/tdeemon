package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"uuid"
)

// PushSubscription is a browser's Web Push subscription.
type PushSubscription struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	Endpoint string
	// P256dh and Auth are the browser's keys, base64url.
	P256dh, Auth string
	// Device is what the device is, for the list of devices.
	Device    string
	CreatedAt time.Time
	UpdatedAt time.Time
	// LastSentAt is zero until a message went to it.
	LastSentAt time.Time
}

const pushColumns = "id, user_id, endpoint, p256dh, auth, device, created_at, updated_at, last_sent_at"

// SavePushSubscription stores userID's subscription s, by its endpoint:
// the same endpoint again takes the new keys and device, and one another
// user subscribed with (the device's previous user) becomes userID's,
// under a new id. s.ID is the id of a new one; it returns the
// subscription as stored.
func (db *DB) SavePushSubscription(ctx context.Context, userID uuid.UUID, s PushSubscription, now time.Time) (PushSubscription, error) {
	var out PushSubscription
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, device, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (endpoint) DO UPDATE SET
				id = CASE WHEN user_id = excluded.user_id THEN id ELSE excluded.id END,
				created_at = CASE WHEN user_id = excluded.user_id THEN created_at ELSE excluded.created_at END,
				last_sent_at = CASE WHEN user_id = excluded.user_id THEN last_sent_at END,
				user_id = excluded.user_id, p256dh = excluded.p256dh, auth = excluded.auth,
				device = excluded.device, updated_at = excluded.updated_at
			RETURNING `+pushColumns,
			s.ID.String(), userID.String(), s.Endpoint, s.P256dh, s.Auth, s.Device, formatTime(now), formatTime(now))
		var err error
		out, err = scanPush(row.Scan)
		return err
	})
	if err != nil {
		return PushSubscription{}, fmt.Errorf("save push subscription: %w", err)
	}
	return out, nil
}

// PushSubscriptions returns userID's subscriptions, the newest first.
func (db *DB) PushSubscriptions(ctx context.Context, userID uuid.UUID) ([]PushSubscription, error) {
	rows, err := db.read.QueryContext(ctx, "SELECT "+pushColumns+
		" FROM push_subscriptions WHERE user_id = ? ORDER BY created_at DESC, id DESC", userID.String())
	if err != nil {
		return nil, fmt.Errorf("push subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PushSubscription
	for rows.Next() {
		s, err := scanPush(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("push subscriptions: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("push subscriptions: %w", err)
	}
	return out, nil
}

// DeletePushSubscription deletes userID's subscription id, and reports
// whether they had it.
func (db *DB) DeletePushSubscription(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	var n int64
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM push_subscriptions WHERE user_id = ? AND id = ?", userID.String(), id.String())
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return false, fmt.Errorf("delete push subscription: %w", err)
	}
	return n > 0, nil
}

// MarkPushSent records that a message went to userID's subscription id at
// now.
func (db *DB) MarkPushSent(ctx context.Context, userID, id uuid.UUID, now time.Time) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE push_subscriptions SET last_sent_at = ? WHERE user_id = ? AND id = ?",
			formatTime(now), userID.String(), id.String()); err != nil {
			return fmt.Errorf("mark push sent: %w", err)
		}
		return nil
	})
}

func scanPush(scan func(...any) error) (PushSubscription, error) {
	var (
		s                        PushSubscription
		id, userID, created, upd string
		lastSent                 sql.NullString
	)
	if err := scan(&id, &userID, &s.Endpoint, &s.P256dh, &s.Auth, &s.Device, &created, &upd, &lastSent); err != nil {
		return PushSubscription{}, err
	}
	var err error
	if s.ID, err = uuid.Parse(id); err != nil {
		return PushSubscription{}, fmt.Errorf("push subscription id %q: %w", id, err)
	}
	if s.UserID, err = uuid.Parse(userID); err != nil {
		return PushSubscription{}, fmt.Errorf("push subscription user %q: %w", userID, err)
	}
	if s.CreatedAt, err = parseTime(created); err != nil {
		return PushSubscription{}, err
	}
	if s.UpdatedAt, err = parseTime(upd); err != nil {
		return PushSubscription{}, err
	}
	if lastSent.Valid {
		if s.LastSentAt, err = parseTime(lastSent.String); err != nil {
			return PushSubscription{}, err
		}
	}
	return s, nil
}
