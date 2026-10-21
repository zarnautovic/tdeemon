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
	case hidden(s, lines):
		return fmt.Errorf("%s %q: contains a control or invisible character", field, s)
	case s != strings.TrimSpace(s):
		return fmt.Errorf("%s %q: has space at the start or end", field, s)
	case utf8.RuneCountInString(s) > maxLen:
		return fmt.Errorf("%s is %d characters long, want at most %d", field, utf8.RuneCountInString(s), maxLen)
	}
	return nil
}

// hidden reports whether s holds an invisible character that is not part
// of an emoji: a variation selector after a symbol (a heart, U+2764
// U+FE0F) or a keycap base (#, U+FE0F, U+20E3), or a zero-width joiner
// between two symbols (a runner, U+200D, the male sign) are part of what
// shows. A line break counts as invisible unless lines.
func hidden(s string, lines bool) bool {
	rs := []rune(s)
	for i, r := range rs {
		if Invisible(r) && (r != '\n' || !lines) && !inEmoji(rs, i) {
			return true
		}
	}
	return false
}

// inEmoji reports whether the invisible character at rs[i] is part of
// an emoji sequence: a selector after its base, a tag of a flag, a joiner
// between two of its parts.
func inEmoji(rs []rune, i int) bool {
	switch r := rs[i]; {
	case r == '\ufe0e' || r == '\ufe0f':
		return i > 0 && (emoji(rs[i-1]) || keycap(rs, i-1))
	case r >= '\U000e0020' && r <= '\U000e007f':
		return tagged(rs, i)
	case r == '\u200d':
		return i > 0 && i+1 < len(rs) && emoji(before(rs, i)) && emoji(rs[i+1])
	}
	return false
}

// emoji reports a character an emoji sequence is built of: a symbol, a
// skin tone, or one of the few emoji outside the symbols that take a
// variation selector (‼, ⁉, ℹ, ↔, ⤴, ⤵, 〰, 〽, the medium squares).
// Other modifier letters and punctuation ("^", "`") are not.
func emoji(r rune) bool {
	switch {
	case unicode.Is(unicode.So, r), r >= '\U0001f3fb' && r <= '\U0001f3ff':
		return true
	}
	return strings.ContainsRune("\u203c\u2049\u2139\u2194\u2934\u2935\u3030\u303d\u25fb\u25fc\u25fd\u25fe", r)
}

// tagged reports a tag character at rs[i] that belongs to a flag: the
// black flag, then tags (the flag of England is U+1F3F4, the tags for
// "gbeng", and the cancel tag U+E007F).
func tagged(rs []rune, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch {
		case rs[j] == '\U0001f3f4':
			return true
		case rs[j] < '\U000e0020' || rs[j] > '\U000e007e':
			return false
		}
	}
	return false
}

// keycap reports a keycap base at rs[i]: #, * or a digit, then a
// variation selector and the combining keycap.
func keycap(rs []rune, i int) bool {
	return strings.ContainsRune("#*0123456789", rs[i]) && i+2 < len(rs) && rs[i+2] == '\u20e3'
}

// before returns the rune before rs[i], skipping a variation selector:
// in the rainbow flag (U+1F3F3 U+FE0F U+200D U+1F308) the joiner follows
// the flag's selector.
func before(rs []rune, i int) rune {
	r := rs[i-1]
	if (r == '\ufe0e' || r == '\ufe0f') && i > 1 {
		r = rs[i-2]
	}
	return r
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
