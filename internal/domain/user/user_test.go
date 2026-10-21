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
		{"email with a display name", func(u *User) { u.Email = "ana <ana@example.com>" }, "plain address"},
		{"email with a comment", func(u *User) { u.Email = "ana@example.com (ana)" }, "plain address"},
		{"email too long", func(u *User) { u.Email = strings.Repeat("a", 64) + "@" + strings.Repeat("b", 186) + ".com" }, "at most 254"},
		{"no name", func(u *User) { u.Name = "  " }, "name is required"},
		{"name with space around", func(u *User) { u.Name = " Ana" }, "space at the start or end"},
		{"name too long", func(u *User) { u.Name = strings.Repeat("ž", MaxNameLen+1) }, "at most 100"},
		{"name with a control character", func(u *User) { u.Name = "Ana\nMarić" }, "control character"},
		{"name not UTF-8", func(u *User) { u.Name = "Ana\xff" }, "not valid UTF-8"},
		{"unknown role", func(u *User) { u.Role = "owner" }, `role "owner"`},
		{"unknown locale", func(u *User) { u.Locale = "de" }, `locale "de"`},
		{"no time zone", func(u *User) { u.TimeZone = "" }, "IANA name"},
		{"Local time zone", func(u *User) { u.TimeZone = "Local" }, "IANA name"},
		{"unknown time zone", func(u *User) { u.TimeZone = "Mars/Olympus" }, `time zone "Mars/Olympus"`},
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
		{"plus address", func(u *User) { u.Email = "ana+tdeemon@example.com" }},
		{"no dot in the domain", func(u *User) { u.Email = "ana@localhost" }},
	} {
		u := valid()
		tc.change(&u)
		if err := u.Validate(); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"ana@example.com":      "ana@example.com",
		"  Ana@Example.COM \t": "ana@example.com",
		"ŽELJKA@primjer.hr":    "željka@primjer.hr",
		"":                     "",
	} {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
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
