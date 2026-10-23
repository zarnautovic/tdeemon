package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// user create takes a locale, and user set-locale changes it; a language
// tdeemon does not speak is refused.
func TestLocale(t *testing.T) {
	t.Parallel()

	env, dbPath := instance(t, nil)
	if err := run(t.Context(), []string{"user", "create", "--email", "ana@example.com", "--locale", "hr"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if u, _ := stored(t, dbPath, "ana@example.com"); u.Locale != "hr" {
		t.Errorf("created with %q", u.Locale)
	}
	if err := run(t.Context(), []string{"user", "create", "--email", "ivo@example.com"}, env, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if u, _ := stored(t, dbPath, "ivo@example.com"); u.Locale != "en" {
		t.Errorf("created by default with %q", u.Locale)
	}
	var out bytes.Buffer
	if err := run(t.Context(), []string{"user", "set-locale", "--email", "IVO@example.com", "--locale", "hr"}, env, noInput(), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if u, _ := stored(t, dbPath, "ivo@example.com"); u.Locale != "hr" || !strings.Contains(out.String(), "to hr") {
		t.Errorf("after set-locale %q; said %q", u.Locale, out.String())
	}
	for name, args := range map[string][]string{
		"a language not spoken": {"--email", "ivo@example.com", "--locale", "de"},
		"no such user":          {"--email", "eve@example.com", "--locale", "hr"},
	} {
		if err := run(t.Context(), append([]string{"user", "set-locale"}, args...), env, noInput(), io.Discard, io.Discard); err == nil || errors.Is(err, errUsage) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, args := range map[string][]string{
		"no locale":   {"--email", "ivo@example.com"},
		"no email":    {"--locale", "hr"},
		"an argument": {"--email", "ivo@example.com", "--locale", "hr", "more"},
		"a bad flag":  {"--lang", "hr"},
	} {
		if err := run(t.Context(), append([]string{"user", "set-locale"}, args...), env, noInput(), io.Discard, io.Discard); !errors.Is(err, errUsage) {
			t.Errorf("%s: %v, want a usage error", name, err)
		}
	}
	if err := run(t.Context(), []string{"user", "create", "--email", "eve@example.com", "--locale", "de"}, env, noInput(), io.Discard, io.Discard); err == nil {
		t.Error("user create took a language not spoken")
	}
}
