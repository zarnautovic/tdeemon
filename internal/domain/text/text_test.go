package text

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, s         string
		required, lines bool
		want            string // "" for no error
	}{
		{"a value", "Egg", true, false, ""},
		{"inner space", "Egg white", true, false, ""},
		{"lines in a note", "one\ntwo", false, true, ""},
		{"lines in a name", "one\ntwo", false, false, "control or invisible"},
		{"a tab in a note", "one\ttwo", false, true, "control or invisible"},
		{"empty, optional", "", false, false, ""},
		{"empty, required", "", true, false, "note is required"},
		{"only space, required", "  ", true, false, "note is required"},
		{"only space, optional", "  ", false, false, "note is only space"},
		{"space around", " Egg", false, false, "space at the start or end"},
		{"a tab", "E\tgg", false, false, "control or invisible"},
		{"DEL", "E\x7fgg", false, false, "control or invisible"},
		{"NEL", "Egg\u0085x", false, true, "control or invisible"},
		{"a zero-width space", "Egg\u200b", false, false, "control or invisible"},
		{"a direction override", "Egg\u202e", false, false, "control or invisible"},
		{"a line separator", "x\u2028y", false, true, "control or invisible"},
		{"a blank Braille pattern", "\u2800", true, false, "control or invisible"},
		{"a replacement character", "\ufffd", true, false, "control or invisible"},
		{"an emoji with its selector", "Pizza \u2764\ufe0f", false, true, ""},
		{"a coffee with its selector", "Kava \u2615\ufe0f", false, false, ""},
		{"a joined emoji", "\U0001f3c3\u200d\u2642\ufe0f", false, false, ""},
		{"a family", "\U0001f468\u200d\U0001f469\u200d\U0001f467", false, false, ""},
		{"a rainbow flag", "\U0001f3f3\ufe0f\u200d\U0001f308", false, false, ""},
		{"a skin tone, joined", "\U0001f44b\U0001f3fd\u200d\u2642\ufe0f", false, false, ""},
		{"a keycap", "#\ufe0f\u20e3", false, false, ""},
		{"a text-style smiley", "\u263a\ufe0e", false, false, ""},
		{"a double exclamation", "Slatko\u203c\ufe0f", false, true, ""},
		{"an information sign", "\u2139\ufe0f info", false, false, ""},
		{"the flag of England", "\U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f", false, false, ""},
		{"a selector after a caret", "Egg^\ufe0f", false, false, "control or invisible"},
		{"a joiner between carets", "^\u200d^", false, false, "control or invisible"},
		{"a tag after a letter", "Egg\U000e0067", false, false, "control or invisible"},
		{"a cancel tag alone", "\U000e007f", false, false, "control or invisible"},
		{"a selector after a letter", "Egg\ufe0f", false, false, "control or invisible"},
		{"a selector at the start", "\ufe0fEgg", false, false, "control or invisible"},
		{"a selector after a digit, no keycap", "1\ufe0f", false, false, "control or invisible"},
		{"a joiner between letters", "E\u200dgg", false, false, "control or invisible"},
		{"a joiner at the end of an emoji", "\U0001f3c3\u200d", false, false, "control or invisible"},
		{"a joiner before a letter", "\U0001f3c3\u200dx", false, false, "control or invisible"},
		{"a joiner at the start", "\u200d\U0001f3c3", false, false, "control or invisible"},
		{"not UTF-8", "\xff", false, false, "not valid UTF-8"},
		{"too long", strings.Repeat("ž", 11), false, false, "11 characters long, want at most 10"},
		{"just long enough", strings.Repeat("ž", 10), false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			check := Check
			if tc.lines {
				check = CheckLines
			}
			err := check("note", tc.s, 10, tc.required)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Errorf("Check(%q) = %v, want %q", tc.s, err, tc.want)
			}
		})
	}
}
