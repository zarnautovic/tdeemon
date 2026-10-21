package store_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

func newUser(email string) user.User {
	now := time.Date(2026, 10, 21, 8, 15, 30, 0, time.UTC)
	return user.User{
		ID:           uuid.NewV7(),
		Email:        email,
		Name:         "Ana",
		Role:         user.RoleAdmin,
		Locale:       "hr",
		TimeZone:     "Europe/Zagreb",
		DayStartHour: 5,
		CreatedAt:    now,
		UpdatedAt:    now.Add(time.Hour),
	}
}

func passwordHash(t *testing.T, db *store.DB, id uuid.UUID) sql.NullString {
	t.Helper()

	var h sql.NullString
	if err := db.Read().QueryRowContext(t.Context(), "SELECT password_hash FROM users WHERE id = ?", id.String()).Scan(&h); err != nil {
		t.Fatalf("read password_hash: %v", err)
	}
	return h
}

func TestCreateUserAndReadItBack(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	u := newUser("ana@example.com")
	// A time zone other than UTC must come back as the same instant.
	u.CreatedAt = u.CreatedAt.In(time.FixedZone("CEST", 2*60*60))
	if err := db.CreateUser(t.Context(), u, "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := db.UserByEmail(t.Context(), "ana@example.com")
	if err != nil {
		t.Fatalf("UserByEmail: %v", err)
	}
	if got.ID != u.ID || got.Email != u.Email || got.Name != u.Name || got.Role != u.Role ||
		got.Locale != u.Locale || got.TimeZone != u.TimeZone || got.DayStartHour != u.DayStartHour ||
		!got.CreatedAt.Equal(u.CreatedAt) || !got.UpdatedAt.Equal(u.UpdatedAt) {
		t.Errorf("read back %+v, want %+v", got, u)
	}
	if h := passwordHash(t, db, u.ID); !h.Valid || !strings.HasPrefix(h.String, "$argon2id$") {
		t.Errorf("password_hash = %+v, want the hash", h)
	}
}

func TestCreateUserWithoutPassword(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	u := newUser("ana@example.com")
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if h := passwordHash(t, db, u.ID); h.Valid {
		t.Errorf("password_hash = %q, want NULL", h.String)
	}
}

func TestCreateUserTakenEmail(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	if err := db.CreateUser(t.Context(), newUser("ana@example.com"), ""); err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	if err := db.CreateUser(t.Context(), newUser("ana@example.com"), ""); !errors.Is(err, store.ErrEmailTaken) {
		t.Errorf("second CreateUser = %v, want ErrEmailTaken", err)
	}
}

func TestCreateUserValidates(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	u := newUser("Ana@Example.com") // not normalized
	if err := db.CreateUser(t.Context(), u, ""); err == nil || !strings.Contains(err.Error(), "not normalized") {
		t.Errorf("CreateUser = %v, want a validation error", err)
	}
	if _, err := db.UserByEmail(t.Context(), "ana@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("UserByEmail after a rejected create = %v, want ErrNotFound", err)
	}
}

func TestUserByEmailNotFound(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	if _, err := db.UserByEmail(t.Context(), "nobody@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("UserByEmail = %v, want ErrNotFound", err)
	}
}

// The schema keeps broken rows out even when code skips the validation.
func TestUsersTableChecks(t *testing.T) {
	t.Parallel()

	const insert = `INSERT INTO users (id, email, name, role, password_hash, locale, timezone, day_start_hour, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULL, ?, ?, ?, '2026-10-21T08:00:00Z', '2026-10-21T08:00:00Z')`
	for _, tc := range []struct {
		name, email, userName, role, locale, timeZone string
		dayStart                                      int
	}{
		{"empty email", "", "Ana", "user", "en", "UTC", 4},
		{"empty name", "ana@example.com", "", "user", "en", "UTC", 4},
		{"unknown role", "ana@example.com", "Ana", "owner", "en", "UTC", 4},
		{"empty locale", "ana@example.com", "Ana", "user", "", "UTC", 4},
		{"empty time zone", "ana@example.com", "Ana", "user", "en", "", 4},
		{"day starts at 24", "ana@example.com", "Ana", "user", "en", "UTC", 24},
		{"day starts before 0", "ana@example.com", "Ana", "user", "en", "UTC", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := storetest.Open(t)
			err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
				_, err := tx.ExecContext(t.Context(), insert, uuid.NewV7().String(), tc.email, tc.userName, tc.role, tc.locale, tc.timeZone, tc.dayStart)
				return err
			})
			if err == nil || !strings.Contains(err.Error(), "CHECK constraint failed") {
				t.Errorf("insert = %v, want a CHECK constraint error", err)
			}
		})
	}

	db := storetest.Open(t)
	err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		for range 2 {
			if _, err := tx.ExecContext(t.Context(), insert, uuid.NewV7().String(), "ana@example.com", "Ana", "user", "en", "UTC", 4); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: users.email") {
		t.Errorf("two inserts with one email = %v, want a UNIQUE constraint error", err)
	}
}

// A row the code cannot read back is an error, not a zero value.
func TestUserByEmailRejectsCorruptRows(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, id, created, updated string }{
		// UUID-sized, so the CHECK on the length lets it in, but not a UUID.
		{"id", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz", "2026-10-21T08:00:00Z", "2026-10-21T08:00:00Z"},
		{"created_at", uuid.NewV7().String(), "yesterday", "2026-10-21T08:00:00Z"},
		{"updated_at", uuid.NewV7().String(), "2026-10-21T08:00:00Z", "21.10.2026."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := storetest.Open(t)
			err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
				_, err := tx.ExecContext(t.Context(), `INSERT INTO users
					(id, email, name, role, locale, timezone, day_start_hour, created_at, updated_at)
					VALUES (?, 'ana@example.com', 'Ana', 'user', 'en', 'UTC', 4, ?, ?)`, tc.id, tc.created, tc.updated)
				return err
			})
			if err != nil {
				t.Fatalf("insert: %v", err)
			}
			if _, err := db.UserByEmail(t.Context(), "ana@example.com"); err == nil || errors.Is(err, store.ErrNotFound) {
				t.Errorf("UserByEmail = %v, want an error about the %s", err, tc.name)
			}
		})
	}
}

// The id must have a UUID's length; an empty one would pass the primary
// key, which only rules out NULL.
func TestUsersTableChecksTheID(t *testing.T) {
	t.Parallel()

	for name, id := range map[string]string{"empty": "", "short": "1234", "one too long": uuid.NewV7().String() + "0"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := storetest.Open(t)
			err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
				_, err := tx.ExecContext(t.Context(), `INSERT INTO users
					(id, email, name, role, locale, timezone, day_start_hour, created_at, updated_at)
					VALUES (?, 'ana@example.com', 'Ana', 'user', 'en', 'UTC', 4, '2026-10-21T08:00:00Z', '2026-10-21T08:00:00Z')`, id)
				return err
			})
			if err == nil || !strings.Contains(err.Error(), "CHECK constraint failed") {
				t.Errorf("insert with id %q = %v, want a CHECK constraint error", id, err)
			}
		})
	}
}
