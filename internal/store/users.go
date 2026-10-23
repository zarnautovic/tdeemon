package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
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
	row := db.read.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE email = ?", email)
	u, err := scanUser(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return user.User{}, ErrNotFound
	}
	if err != nil {
		return user.User{}, fmt.Errorf("user by email: %w", err)
	}
	return u, nil
}

// UserCredentials returns the user with email, which must be normalized,
// and their password hash ("" when they have no password), or ErrNotFound.
func (db *DB) UserCredentials(ctx context.Context, email string) (user.User, string, error) {
	var hash sql.NullString
	row := db.read.QueryRowContext(ctx, "SELECT "+userColumns+", password_hash FROM users WHERE email = ?", email)
	u, err := scanUser(row.Scan, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return user.User{}, "", ErrNotFound
	}
	if err != nil {
		return user.User{}, "", fmt.Errorf("user credentials: %w", err)
	}
	return u, hash.String, nil
}

// userColumns are the columns scanUser reads; userColumnsOfU the same,
// for a query that calls the users table u.
const (
	userColumns    = "id, email, name, role, locale, timezone, day_start_hour, created_at, updated_at"
	userColumnsOfU = "u.id, u.email, u.name, u.role, u.locale, u.timezone, u.day_start_hour, u.created_at, u.updated_at"
)

// scanUser reads the columns of userColumns, then any extra ones into
// extra, with scan (a Row's or Rows' Scan).
func scanUser(scan func(dest ...any) error, extra ...any) (user.User, error) {
	var (
		u                    user.User
		id, role             string
		createdAt, updatedAt string
	)
	dest := append([]any{&id, &u.Email, &u.Name, &role, &u.Locale, &u.TimeZone, &u.DayStartHour, &createdAt, &updatedAt}, extra...)
	if err := scan(dest...); err != nil {
		return user.User{}, err
	}
	u.Role = user.Role(role)
	var err error
	if u.ID, err = uuid.Parse(id); err != nil {
		return user.User{}, fmt.Errorf("id %q: %w", id, err)
	}
	if u.CreatedAt, err = parseTime(createdAt); err != nil {
		return user.User{}, err
	}
	if u.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return user.User{}, err
	}
	return u, nil
}

// SetPasswordHash gives the user with id a new password hash and ends
// every session of theirs, as a reset should, or returns ErrNotFound. A
// rehash at sign-in, which keeps the sessions, is ReplacePasswordHash.
func (db *DB) SetPasswordHash(ctx context.Context, id uuid.UUID, passwordHash string, now time.Time) error {
	if passwordHash == "" {
		return errors.New("set password: the hash is empty")
	}
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?",
			passwordHash, formatTime(now), id.String())
		if err != nil {
			return fmt.Errorf("set password: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("set password: %w", err)
		} else if n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", id.String()); err != nil {
			return fmt.Errorf("set password: end sessions: %w", err)
		}
		return nil
	})
}

// SetLocale sets the language of the user with id, one of user.Locales,
// or returns ErrNotFound.
func (db *DB) SetLocale(ctx context.Context, id uuid.UUID, locale string, now time.Time) error {
	if !slices.Contains(user.Locales(), locale) {
		return fmt.Errorf("set locale: %q: want one of %s", locale, strings.Join(user.Locales(), ", "))
	}
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE users SET locale = ?, updated_at = ? WHERE id = ?", locale, formatTime(now), id.String())
		if err != nil {
			return fmt.Errorf("set locale: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("set locale: %w", err)
		} else if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ReplacePasswordHash stores newHash for the user with id if their hash
// is still oldHash, and reports whether it did. A sign-in uses it to store
// a rehash: a reset that came in between wins, instead of being undone by
// a hash of the old password.
func (db *DB) ReplacePasswordHash(ctx context.Context, id uuid.UUID, oldHash, newHash string, now time.Time) (bool, error) {
	var replaced bool
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ? AND password_hash = ?",
			newHash, formatTime(now), id.String(), oldHash)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		replaced = n == 1
		return err
	})
	if err != nil {
		return false, fmt.Errorf("replace password hash: %w", err)
	}
	return replaced, nil
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

// Settings are what a user changes of their account themselves: the name
// the app shows, its language, their time zone and the hour their day
// starts.
type Settings struct {
	Name         string
	Locale       string
	TimeZone     string
	DayStartHour int
}

// Validate reports every setting that breaks the rules of user.Validate,
// joined into one error.
func (s Settings) Validate() error {
	return errors.Join(user.ValidateName(s.Name), user.ValidateLocale(s.Locale),
		user.ValidateTimeZone(s.TimeZone), user.ValidateDayStartHour(s.DayStartHour))
}

// SettingsChange is a change of some settings: each one given, and only
// those.
type SettingsChange struct {
	Name, Locale, TimeZone *string
	DayStartHour           *int
}

// ErrInvalidSettings is a change that would leave settings the rules
// refuse; the error that wraps it says which.
var ErrInvalidSettings = errors.New("invalid settings")

// ChangeSettings changes the settings of the user with id as c says, on
// the user as they are in the same transaction, so changes made at once
// each keep what they change; and returns the user as changed. It returns
// ErrNotFound for no such user, and an error that wraps
// ErrInvalidSettings for settings the rules refuse. A day's start or a
// time zone changed moves no meal or measurement logged before: each
// keeps the day it was logged on.
func (db *DB) ChangeSettings(ctx context.Context, id uuid.UUID, c SettingsChange, now time.Time) (user.User, error) {
	var u user.User
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		u, err = scanUser(tx.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE id = ?", id.String()).Scan)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if c.Name != nil {
			u.Name = *c.Name
		}
		if c.Locale != nil {
			u.Locale = *c.Locale
		}
		if c.TimeZone != nil {
			u.TimeZone = *c.TimeZone
		}
		if c.DayStartHour != nil {
			u.DayStartHour = *c.DayStartHour
		}
		s := Settings{Name: u.Name, Locale: u.Locale, TimeZone: u.TimeZone, DayStartHour: u.DayStartHour}
		if err := s.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidSettings, err)
		}
		u.UpdatedAt = now.UTC()
		_, err = tx.ExecContext(ctx, `UPDATE users SET name = ?, locale = ?, timezone = ?, day_start_hour = ?, updated_at = ?
			WHERE id = ?`, u.Name, u.Locale, u.TimeZone, u.DayStartHour, formatTime(now), id.String())
		return err
	})
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidSettings) {
		return user.User{}, err
	}
	if err != nil {
		return user.User{}, fmt.Errorf("change settings: %w", err)
	}
	return u, nil
}
