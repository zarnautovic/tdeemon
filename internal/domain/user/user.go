// Package user defines a user of the instance and the rules its fields
// follow, independent of how users are stored or created.
package user

import (
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // Time zone names must resolve even in images without system tzdata.
	"unicode"
	"unicode/utf8"
	"uuid"
)

// Role is what a user may do on the instance.
type Role string

// Roles.
const (
	// RoleAdmin manages the instance and its users.
	RoleAdmin Role = "admin"
	// RoleUser uses the instance for their own data.
	RoleUser Role = "user"
)

// Defaults for a new user.
const (
	DefaultLocale = "en"
	// DefaultDayStartHour puts a 01:30 snack on the day before.
	DefaultDayStartHour = 4
)

// Limits on the fields.
const (
	// MaxEmailLen is the longest address SMTP can deliver to (RFC 5321).
	MaxEmailLen = 254
	// MaxNameLen counts characters (code points), not bytes.
	MaxNameLen = 100
)

// Locales lists the languages the app speaks.
func Locales() []string { return []string{"en", "hr"} }

// User is a person who uses the instance.
type User struct {
	ID    uuid.UUID
	Email string
	Name  string
	Role  Role
	// Locale is the language of the app, one of Locales.
	Locale string
	// TimeZone is an IANA time zone name, such as Europe/Zagreb.
	TimeZone string
	// DayStartHour is the hour (0–23) at which the user's day starts.
	DayStartHour int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NormalizeEmail returns the address as tdeemon stores and compares it:
// without surrounding space and in lower case. Mail providers treat the
// part before the @ without regard to case, even where RFC 5321 allows
// otherwise, and two accounts that differ only in case are a mistake.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Validate reports every field that breaks the rules, joined into one
// error. It expects the email already normalized (NormalizeEmail).
func (u User) Validate() error {
	var errs []error
	if u.ID == uuid.Nil() {
		errs = append(errs, errors.New("id is missing"))
	}
	if err := validateEmail(u.Email); err != nil {
		errs = append(errs, err)
	}
	if err := validateName(u.Name); err != nil {
		errs = append(errs, err)
	}
	if u.Role != RoleAdmin && u.Role != RoleUser {
		errs = append(errs, fmt.Errorf("role %q: want %s or %s", u.Role, RoleAdmin, RoleUser))
	}
	if !slices.Contains(Locales(), u.Locale) {
		errs = append(errs, fmt.Errorf("locale %q: want one of %s", u.Locale, strings.Join(Locales(), ", ")))
	}
	if err := ValidateTimeZone(u.TimeZone); err != nil {
		errs = append(errs, err)
	}
	if u.DayStartHour < 0 || u.DayStartHour > 23 {
		errs = append(errs, fmt.Errorf("day start hour %d: want 0 to 23", u.DayStartHour))
	}
	if u.CreatedAt.IsZero() || u.UpdatedAt.IsZero() {
		errs = append(errs, errors.New("created and updated times are required"))
	}
	return errors.Join(errs...)
}

func validateEmail(email string) error {
	if email == "" {
		return errors.New("email is required")
	}
	if email != NormalizeEmail(email) {
		return fmt.Errorf("email %q: not normalized; want it trimmed and in lower case", email)
	}
	if len(email) > MaxEmailLen {
		return fmt.Errorf("email is %d bytes long, want at most %d", len(email), MaxEmailLen)
	}
	// A bare address only: no display name ("Ana <ana@example.com>"), no
	// comments, nothing ParseAddress would strip.
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return fmt.Errorf("email %q: want a plain address such as ana@example.com", email)
	}
	return nil
}

func validateName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("name is required")
	case name != strings.TrimSpace(name):
		return fmt.Errorf("name %q: has space at the start or end", name)
	case !utf8.ValidString(name):
		return errors.New("name is not valid UTF-8")
	case utf8.RuneCountInString(name) > MaxNameLen:
		return fmt.Errorf("name is %d characters long, want at most %d", utf8.RuneCountInString(name), MaxNameLen)
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return errors.New("name contains a control character")
	}
	return nil
}

// ValidateTimeZone accepts an IANA time zone name, such as
// Europe/Zagreb or UTC. It rejects "Local" and the empty string, which
// Go accepts but which mean nothing outside this machine.
func ValidateTimeZone(name string) error {
	if name == "" || name == "Local" {
		return fmt.Errorf("time zone %q: want an IANA name such as Europe/Zagreb", name)
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("time zone %q: %w", name, err)
	}
	return nil
}
