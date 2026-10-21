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

// ErrNotFound reports that the row asked for does not exist.
var ErrNotFound = errors.New("not found")

// ErrEmailTaken reports a user whose email another user already has.
var ErrEmailTaken = errors.New("a user with this email already exists")

// CreateUser stores a new user with passwordHash (an argon2id hash from
// auth.HashPassword; empty for a user without a password). It validates
// u first and returns ErrEmailTaken if the email is in use.
func (db *DB) CreateUser(ctx context.Context, u user.User, passwordHash string) error {
	if err := u.Validate(); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	hash := sql.NullString{String: passwordHash, Valid: passwordHash != ""}
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		// The write lock is held from BEGIN, so nobody can take the email
		// between this check and the insert; the UNIQUE constraint backs
		// it up.
		var taken bool
		err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE email = ?)", u.Email).Scan(&taken)
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		if taken {
			return ErrEmailTaken
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO users (id, email, name, role, password_hash, locale, timezone, day_start_hour, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			u.ID.String(), u.Email, u.Name, string(u.Role), hash, u.Locale, u.TimeZone, u.DayStartHour,
			formatTime(u.CreatedAt), formatTime(u.UpdatedAt))
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		return nil
	})
}

// UserByEmail returns the user with email, which must be normalized
// (user.NormalizeEmail), or ErrNotFound.
func (db *DB) UserByEmail(ctx context.Context, email string) (user.User, error) {
	var (
		u                    user.User
		id, role             string
		createdAt, updatedAt string
	)
	err := db.read.QueryRowContext(ctx, `
		SELECT id, email, name, role, locale, timezone, day_start_hour, created_at, updated_at
		FROM users WHERE email = ?`, email).
		Scan(&id, &u.Email, &u.Name, &role, &u.Locale, &u.TimeZone, &u.DayStartHour, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return user.User{}, ErrNotFound
	}
	if err != nil {
		return user.User{}, fmt.Errorf("user by email: %w", err)
	}
	u.Role = user.Role(role)
	if u.ID, err = uuid.Parse(id); err != nil {
		return user.User{}, fmt.Errorf("user by email: id %q: %w", id, err)
	}
	if u.CreatedAt, err = parseTime(createdAt); err != nil {
		return user.User{}, fmt.Errorf("user by email: %w", err)
	}
	if u.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return user.User{}, fmt.Errorf("user by email: %w", err)
	}
	return u, nil
}

// formatTime writes t as the schema stores timestamps: RFC 3339 in UTC,
// to the second.
func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp %q: %w", s, err)
	}
	return t, nil
}
