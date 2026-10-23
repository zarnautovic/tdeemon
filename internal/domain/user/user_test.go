package user

import (
	"strings"
	"testing"
	"time"
	"uuid"
)

func valid() User {
	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	return User{
		ID:           uuid.NewV7(),
		Email:        "ana@example.com",
		Name:         "Ana",
		Role:         RoleAdmin,
		Locale:       DefaultLocale,
		TimeZone:     "Europe/Zagreb",
		DayStartHour: DefaultDayStartHour,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	if err := valid().Validate(); err != nil {
		t.Fatalf("a valid user: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*User)
		want   string // a part of the error
	}{
		{"no id", func(u *User) { u.ID = uuid.Nil() }, "id is missing"},
		{"no email", func(u *User) { u.Email = "" }, "email is required"},
		{"email in upper case", func(u *User) { u.Email = "Ana@example.com" }, "not normalized"},
		{"email with space", func(u *User) { u.Email = " ana@example.com" }, "not normalized"},
		{"email without @", func(u *User) { u.Email = "ana.example.com" }, "plain address"},
		{"email with a display name", func(u *User) { u.Email = "ana <ana@example.com>" }, "a space"},
		{"email in angle brackets", func(u *User) { u.Email = "<ana@example.com>" }, "plain address"},
		{"email with a comment", func(u *User) { u.Email = "ana@example.com(ana)" }, "plain address"},
		{"email with a quoted local part", func(u *User) { u.Email = `"ana"@example.com` }, "plain address"},
		{"email with a zero-width space", func(u *User) { u.Email = "ana@example.com\u200b" }, "an invisible character"},
		{"email with a byte order mark", func(u *User) { u.Email = "\ufeffana@example.com" }, "an invisible character"},
		{"email with a direction mark", func(u *User) { u.Email = "ana\u202e@example.com" }, "an invisible character"},
		{"email with a no-break space", func(u *User) { u.Email = "ana\u00a0x@example.com" }, "a space"},
		{"email not UTF-8", func(u *User) { u.Email = "ana\xff@example.com" }, "not valid UTF-8"},
		{"email with a combining grapheme joiner", func(u *User) { u.Email = "ana\u034f@example.com" }, "an invisible character"},
		{"email with a variation selector", func(u *User) { u.Email = "ana\ufe0f@example.com" }, "an invisible character"},
		{"email with a supplementary variation selector", func(u *User) { u.Email = "ana\U000e0100@example.com" }, "an invisible character"},
		{"email with a Hangul filler", func(u *User) { u.Email = "ana\u3164@example.com" }, "an invisible character"},
		{"email with a blank Braille pattern", func(u *User) { u.Email = "ana\u2800@example.com" }, "an invisible character"},
		{"email with a replacement character", func(u *User) { u.Email = "\ufffdaba@primjer.hr" }, "an invisible character"},
		{"email decomposed", func(u *User) { u.Email = "c\u030caba@example.com" }, "not normalized"},
		{"email too long", func(u *User) { u.Email = strings.Repeat("a", 64) + "@" + strings.Repeat("b", 186) + ".com" }, "at most 254"},
		{"no name", func(u *User) { u.Name = "  " }, "name is required"},
		{"name with space around", func(u *User) { u.Name = " Ana" }, "space at the start or end"},
		{"name too long", func(u *User) { u.Name = strings.Repeat("ž", MaxNameLen+1) }, "at most 100"},
		{"name with a control character", func(u *User) { u.Name = "Ana\nMarić" }, "control or invisible"},
		{"name of a zero-width space", func(u *User) { u.Name = "\u200b" }, "control or invisible"},
		{"name with a direction override", func(u *User) { u.Name = "Ana\u202eMarić" }, "control or invisible"},
		{"name with a line separator", func(u *User) { u.Name = "Ana\u2028Marić" }, "control or invisible"},
		{"name of a Hangul filler", func(u *User) { u.Name = "\u3164" }, "control or invisible"},
		{"name not UTF-8", func(u *User) { u.Name = "Ana\xff" }, "not valid UTF-8"},
		{"name of a joiner only", func(u *User) { u.Name = "\u200d" }, "control or invisible"},
		{"name with a joiner at its start", func(u *User) { u.Name = "\u200dAna" }, "control or invisible"},
		{"name with a non-joiner at its end", func(u *User) { u.Name = "Ana\u200c" }, "control or invisible"},
		{"name with a selector first", func(u *User) { u.Name = "\ufe0fAna" }, "control or invisible"},
		{"name with a joiner by a space", func(u *User) { u.Name = "Ana \u200dMarić" }, "control or invisible"},
		{"unknown role", func(u *User) { u.Role = "owner" }, `role "owner"`},
		{"unknown locale", func(u *User) { u.Locale = "de" }, `locale "de"`},
		{"no time zone", func(u *User) { u.TimeZone = "" }, "IANA name"},
		{"Local time zone", func(u *User) { u.TimeZone = "Local" }, "IANA name"},
		{"unknown time zone", func(u *User) { u.TimeZone = "Mars/Olympus" }, `time zone "Mars/Olympus"`},
		{"host localtime", func(u *User) { u.TimeZone = "localtime" }, "IANA name"},
		{"host posixrules", func(u *User) { u.TimeZone = "posixrules" }, "IANA name"},
		{"Factory", func(u *User) { u.TimeZone = "Factory" }, "IANA name"},
		{"posix copy", func(u *User) { u.TimeZone = "posix/Europe/Zagreb" }, "IANA name"},
		{"right copy", func(u *User) { u.TimeZone = "right/Europe/Zagreb" }, "IANA name"},
		{"day starts before 0", func(u *User) { u.DayStartHour = -1 }, "want 0 to 23"},
		{"day starts at 24", func(u *User) { u.DayStartHour = 24 }, "want 0 to 23"},
		{"no times", func(u *User) { u.CreatedAt = time.Time{} }, "created and updated times"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u := valid()
			tc.change(&u)
			err := u.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// Validate reports every problem at once.
func TestValidateJoinsErrors(t *testing.T) {
	t.Parallel()

	err := User{}.Validate()
	if err == nil {
		t.Fatal("an empty user is valid")
	}
	for _, want := range []string{"id", "email", "name", "role", "locale", "time zone", "created"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestValidateAcceptsEdges(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		change func(*User)
	}{
		{"day starts at midnight", func(u *User) { u.DayStartHour = 0 }},
		{"day starts at 23", func(u *User) { u.DayStartHour = 23 }},
		{"UTC", func(u *User) { u.TimeZone = "UTC" }},
		{"Croatian", func(u *User) { u.Locale = "hr" }},
		{"plain user", func(u *User) { u.Role = RoleUser }},
		{"longest name", func(u *User) { u.Name = strings.Repeat("ž", MaxNameLen) }},
		{"name with spaces and marks", func(u *User) { u.Name = "Ana-Marija O'Brien Ćosić" }},
		// What writing needs between what shows.
		{"name with an emoji of a family", func(u *User) { u.Name = "Ana \U0001f468\u200d\U0001f469\u200d\U0001f467" }},
		{"name in Persian, with a non-joiner", func(u *User) { u.Name = "\u0646\u06cc\u200c\u0645\u0627" }},
		{"name in Devanagari, with a joiner", func(u *User) { u.Name = "\u0915\u094d\u200d\u0937" }},
		{"name in Japanese, with a variant selector", func(u *User) { u.Name = "\u845b\U000e0100\u57ce" }},
		{"accented address", func(u *User) { u.Email = "jos\u00e9@example.com" }},
		{"plus address", func(u *User) { u.Email = "ana+tdeemon@example.com" }},
		{"no dot in the domain", func(u *User) { u.Email = "ana@localhost" }},
		{"letters with diacritics", func(u *User) { u.Email = "\u010daba@primjer.hr" }},
		{"Etc zone", func(u *User) { u.TimeZone = "Etc/GMT-2" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u := valid()
			tc.change(&u)
			if err := u.Validate(); err != nil {
				t.Errorf("Validate = %v, want nil", err)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	// Bytes that are not UTF-8 stay as they are, for Validate to report.
	if got := NormalizeEmail(" \xe8aba@primjer.hr"); got != " \xe8aba@primjer.hr" {
		t.Errorf("NormalizeEmail changed input that is not UTF-8 into %q", got)
	}

	for _, tc := range []struct{ name, in, want string }{
		{"normalized already", "ana@example.com", "ana@example.com"},
		{"spaces and capitals", "  Ana@Example.COM \t", "ana@example.com"},
		{"capitals with diacritics", "ŽELJKA@primjer.hr", "željka@primjer.hr"},
		{"decomposed letter", "C\u030cABA@primjer.hr", "\u010daba@primjer.hr"}, // decomposed Č becomes one character
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeEmail(tc.in); got != tc.want {
				t.Errorf("NormalizeEmail(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Locales hands out a copy, so a caller cannot change the list.
func TestLocalesIsACopy(t *testing.T) {
	t.Parallel()

	l := Locales()
	l[0] = "xx"
	if Locales()[0] != "en" {
		t.Error("changing the slice Locales returned changed the list")
	}
}
