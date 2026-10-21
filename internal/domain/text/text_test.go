package text

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, s  string
		required bool
		want     string // "" for no error
	}{
		{"a value", "Egg", true, ""},
		{"lines", "one\ntwo", false, ""},
		{"empty, optional", "", false, ""},
		{"empty, required", "", true, "note is required"},
		{"only space, required", "  ", true, "note is required"},
		{"only space, optional", "  ", false, "note is only space"},
		{"space around", " Egg", false, "space at the start or end"},
		{"a tab", "E\tgg", false, "control character"},
		{"DEL", "E\x7fgg", false, "control character"},
		{"not UTF-8", "\xff", false, "not valid UTF-8"},
		{"too long", strings.Repeat("ž", 11), false, "11 characters long, want at most 10"},
		{"just long enough", strings.Repeat("ž", 10), false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := Check("note", tc.s, 10, tc.required)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Errorf("Check(%q) = %v, want %q", tc.s, err, tc.want)
			}
		})
	}
}
