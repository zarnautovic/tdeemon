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

	"golang.org/x/text/unicode/norm"
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
// without surrounding space, in lower case and in Unicode NFC (RFC 6532),
// so a "č" typed as one character or as c + caron is the same address.
// Mail providers treat the part before the @ without regard to case, even
// where RFC 5321 allows otherwise, and two accounts that differ only in
// case are a mistake.
//
// Input that is not valid UTF-8 comes back unchanged, so Validate reports
// it instead of a stand-in character taking its place.
func NormalizeEmail(email string) string {
	if !utf8.ValidString(email) {
		return email
	}
	return norm.NFC.String(strings.ToLower(strings.TrimSpace(email)))
}

// Validate reports every field that breaks the rules, joined into one
// error. It expects the email already normalized (NormalizeEmail).
func (u User) Validate() error {
	var errs []error
	if u.ID == uuid.Nil() {
		errs = append(errs, errors.New("id is missing"))
	}
	if err := ValidateEmail(u.Email); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateName(u.Name); err != nil {
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

// ValidateEmail reports whether email is a plain address in
// NormalizeEmail's form, without spaces or invisible characters.
func ValidateEmail(email string) error {
	if email == "" {
		return errors.New("email is required")
	}
	if !utf8.ValidString(email) {
		return errors.New("email is not valid UTF-8")
	}
	if email != NormalizeEmail(email) {
		return fmt.Errorf("email %q: not normalized; want NormalizeEmail's form", email)
	}
	if len(email) > MaxEmailLen {
		return fmt.Errorf("email is %d bytes long, want at most %d", len(email), MaxEmailLen)
	}
	// An invisible character (a zero-width space pasted from a chat, a
	// variation selector) would make an address nobody can type at sign-in
	// that looks like another user's.
	if strings.IndexFunc(email, func(r rune) bool { return invisible(r) || unicode.IsSpace(r) }) >= 0 {
		return fmt.Errorf("email %q: contains a space or an invisible character", email)
	}
	// A bare address only: no display name ("Ana <ana@example.com>"), no
	// comments, nothing ParseAddress would strip.
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return fmt.Errorf("email %q: want a plain address such as ana@example.com", email)
	}
	return nil
}

// ValidateName reports whether name is a usable display name: not empty,
// without space around it or control characters, at most MaxNameLen
// characters.
func ValidateName(name string) error {
	switch {
	case !utf8.ValidString(name):
		return errors.New("name is not valid UTF-8")
	// Spaces are fine inside a name; a tab, a line break, a direction
	// override that reorders the list of users, or a blank that only
	// looks like a name are not.
	case strings.IndexFunc(name, invisible) >= 0:
		return fmt.Errorf("name %q: contains a control or invisible character", name)
	case strings.TrimSpace(name) == "":
		return errors.New("name is required")
	case name != strings.TrimSpace(name):
		return fmt.Errorf("name %q: has space at the start or end", name)
	case utf8.RuneCountInString(name) > MaxNameLen:
		return fmt.Errorf("name is %d characters long, want at most %d", utf8.RuneCountInString(name), MaxNameLen)
	}
	return nil
}

// invisible reports a character that shows as nothing, or as a blank:
// control and format characters (a tab, a zero-width space, a byte order
// mark, direction marks), line and paragraph separators, characters
// Unicode says to ignore when not supported (a combining grapheme joiner,
// Hangul fillers), variation selectors, the blank Braille pattern, and
// U+FFFD, which stands in for bytes that were not text.
func invisible(r rune) bool {
	return !unicode.IsGraphic(r) || r == utf8.RuneError || r == '\u2800' ||
		unicode.In(r, unicode.Other_Default_Ignorable_Code_Point, unicode.Variation_Selector)
}

// ValidateTimeZone accepts an IANA time zone name, such as
// Europe/Zagreb or UTC. It rejects names Go may resolve but that mean
// nothing outside this machine: the empty string and "Local", and files
// of the host's zoneinfo directory that are no zone of their own
// (localtime, posixrules, Factory, the posix/ and right/ copies), which
// an image without that directory would not find.
func ValidateTimeZone(name string) error {
	switch {
	case name == "", name == "Local", name == "localtime", name == "posixrules", name == "Factory",
		strings.HasPrefix(name, "posix/"), strings.HasPrefix(name, "right/"):
		return fmt.Errorf("time zone %q: want an IANA name such as Europe/Zagreb", name)
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("time zone %q: %w", name, err)
	}
	return nil
}
