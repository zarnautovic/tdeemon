// Package text checks the free-text fields of the domain (names, notes,
// reasons) by one set of rules, with the same messages everywhere.
package text

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Check reports whether s is a usable value for field: valid UTF-8, no
// control characters but line breaks, no space at the start or end, at
// most maxLen characters, and not empty when required. An optional field
// may be "", but not only space.
func Check(field, s string, maxLen int, required bool) error {
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
	case s != strings.TrimSpace(s):
		return fmt.Errorf("%s %q: has space at the start or end", field, s)
	case strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 && r != '\n' || r == 0x7f }):
		return fmt.Errorf("%s contains a control character", field)
	case utf8.RuneCountInString(s) > maxLen:
		return fmt.Errorf("%s is %d characters long, want at most %d", field, utf8.RuneCountInString(s), maxLen)
	}
	return nil
}
