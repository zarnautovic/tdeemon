package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// Session is a web session. ID is the hash of the token the browser
// holds (auth.SessionID), never the token itself.
type Session struct {
	ID         string
	UserID     uuid.UUID
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// CreateSession stores s and removes every session that has expired by
// s.CreatedAt, so ended sessions do not pile up.
func (db *DB) CreateSession(ctx context.Context, s Session) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", formatTime(s.CreatedAt)); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO sessions (id, user_id, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?)",
			s.ID, s.UserID.String(), formatTime(s.CreatedAt), formatTime(s.LastSeenAt), formatTime(s.ExpiresAt))
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		return nil
	})
}

// SessionUser returns the session with id and its user, or ErrNotFound if
// there is no such session or it has expired by now.
func (db *DB) SessionUser(ctx context.Context, id string, now time.Time) (Session, user.User, error) {
	var created, lastSeen, expires string
	row := db.read.QueryRowContext(ctx, `
		SELECT `+userColumnsOfU+`, s.created_at, s.last_seen_at, s.expires_at
		FROM sessions AS s JOIN users AS u ON u.id = s.user_id
		WHERE s.id = ? AND s.expires_at > ?`, id, formatTime(now))
	u, err := scanUser(row.Scan, &created, &lastSeen, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, user.User{}, ErrNotFound
	}
	if err != nil {
		return Session{}, user.User{}, fmt.Errorf("session: %w", err)
	}
	s := Session{ID: id, UserID: u.ID}
	for _, f := range []struct {
		dst *time.Time
		src string
	}{{&s.CreatedAt, created}, {&s.LastSeenAt, lastSeen}, {&s.ExpiresAt, expires}} {
		if *f.dst, err = parseTime(f.src); err != nil {
			return Session{}, user.User{}, fmt.Errorf("session: %w", err)
		}
	}
	return s, u, nil
}

// TouchSession records that the session with id was used at now.
func (db *DB) TouchSession(ctx context.Context, id string, now time.Time) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE sessions SET last_seen_at = ? WHERE id = ?", formatTime(now), id); err != nil {
			return fmt.Errorf("touch session: %w", err)
		}
		return nil
	})
}

// DeleteSession ends the session with id. Ending one that does not exist
// is not an error: signing out twice is fine.
func (db *DB) DeleteSession(ctx context.Context, id string) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id); err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
		return nil
	})
}
