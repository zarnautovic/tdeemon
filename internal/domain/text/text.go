// Package text checks the free-text fields of the domain (names, notes,
// reasons) by one set of rules, with the same messages everywhere.
package text

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Check reports whether s is a usable one-line value for field (a name,
// a reason): valid UTF-8, nothing invisible (Invisible), no space at the
// start or end, at most maxLen characters, and not empty when required.
// An optional field may be "", but not only space.
func Check(field, s string, maxLen int, required bool) error {
	return check(field, s, maxLen, required, false)
}

// CheckLines is Check for a value that may run over several lines (a
// note): line breaks are allowed in it.
func CheckLines(field, s string, maxLen int, required bool) error {
	return check(field, s, maxLen, required, true)
}

func check(field, s string, maxLen int, required, lines bool) error {
	switch {
	case !utf8.ValidString(s):
		return fmt.Errorf("%s is not valid UTF-8", field)
	case s == "" && required:
		return fmt.Errorf("%s is required", field)
	case s == "":
		return nil
	case strings.TrimSpace(s) == "" && required:
		return fmt.Errorf("%s is required", field)
	case strings.TrimSpace(s) == "":
		return fmt.Errorf("%s is only space", field)
	// A tab, a line break in a name, a direction override that reorders
	// a list, or a blank that only looks like a name: "Egg" and "Egg"
	// with a zero-width space would be two foods that look the same.
	case strings.ContainsFunc(s, func(r rune) bool { return Invisible(r) && (r != '\n' || !lines) }):
		return fmt.Errorf("%s %q: contains a control or invisible character", field, s)
	case s != strings.TrimSpace(s):
		return fmt.Errorf("%s %q: has space at the start or end", field, s)
	case utf8.RuneCountInString(s) > maxLen:
		return fmt.Errorf("%s is %d characters long, want at most %d", field, utf8.RuneCountInString(s), maxLen)
	}
	return nil
}

// Invisible reports a character that shows as nothing, or as a blank:
// control and format characters (a tab, a zero-width space, a byte order
// mark, direction marks), line and paragraph separators, characters
// Unicode says to ignore when not supported (a combining grapheme joiner,
// Hangul fillers), variation selectors, the blank Braille pattern, and
// U+FFFD, which stands in for bytes that were not text.
func Invisible(r rune) bool {
	return !unicode.IsGraphic(r) || r == utf8.RuneError || r == '\u2800' ||
		unicode.In(r, unicode.Other_Default_Ignorable_Code_Point, unicode.Variation_Selector)
}
