package food_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

var update = flag.Bool("update", false, "rewrite the web app's table of Key and its cases from food.Key")

// The web app finds a serving by its name as Key does (the meal's
// editor, web/src/lib/name-key.ts): the table it folds names with, and
// the names with their keys its tests check it by, are written from Key.
var (
	tableFile = filepath.Join("..", "..", "..", "web", "src", "lib", "name-key-table.json")
	casesFile = filepath.Join("..", "..", "..", "web", "src", "test", "name-keys.json")
)

// keyTable is what the web app folds a name with, rune by rune, after
// NFC: a range folds first, first+step, … (count of them) each to itself
// plus delta; a special to its string; and the spaces split words.
type keyTable struct {
	Ranges   [][4]rune         `json:"ranges"`
	Specials map[string]string `json:"specials"`
	Spaces   []rune            `json:"spaces"`
}

// newKeyTable is the table of cases.Fold for every rune it changes, and
// of the runes strings.Fields splits at.
func newKeyTable() keyTable {
	fold := cases.Fold()
	t := keyTable{Specials: map[string]string{}}
	for r := range rune(unicode.MaxRune + 1) {
		if !utf8.ValidRune(r) {
			continue
		}
		if unicode.IsSpace(r) {
			t.Spaces = append(t.Spaces, r)
		}
		s := string(r)
		f := fold.String(s)
		if f == s {
			continue
		}
		to, size := utf8.DecodeRuneInString(f)
		if size != len(f) {
			t.Specials[strconv.Itoa(int(r))] = f
			continue
		}
		t.addRange(r, to-r)
	}
	return t
}

// addRange adds r's fold, r+delta, to the last range when it goes on
// from it, or as a range of its own.
func (t *keyTable) addRange(r, delta rune) {
	if n := len(t.Ranges); n > 0 {
		last := &t.Ranges[n-1]
		first, count, step, d := last[0], last[1], last[2], last[3]
		switch {
		case d == delta && count == 1 && (r-first == 1 || r-first == 2):
			last[1], last[2] = 2, r-first
			return
		case d == delta && count > 1 && r == first+count*step:
			last[1]++
			return
		}
	}
	t.Ranges = append(t.Ranges, [4]rune{r, 1, 1, delta})
}

// webKeyer is the web app's key of a name by t, as name-key.ts makes it,
// for checking that the table gives Key's keys.
func webKeyer(t keyTable) func(string) string {
	folds := map[rune]string{}
	for _, rg := range t.Ranges {
		for i := range rg[1] {
			r := rg[0] + i*rg[2]
			folds[r] = string(r + rg[3])
		}
	}
	for r, f := range t.Specials {
		n, err := strconv.ParseInt(r, 10, 32)
		if err != nil {
			panic(err)
		}
		folds[rune(n)] = f
	}
	spaces := map[rune]bool{}
	for _, r := range t.Spaces {
		spaces[r] = true
	}
	return func(s string) string {
		var words []string
		var word strings.Builder
		for _, r := range norm.NFC.String(s) {
			if spaces[r] {
				if word.Len() > 0 {
					words = append(words, word.String())
					word.Reset()
				}
				continue
			}
			if f, ok := folds[r]; ok {
				word.WriteString(f)
			} else {
				word.WriteRune(r)
			}
		}
		if word.Len() > 0 {
			words = append(words, word.String())
		}
		return strings.Join(words, " ")
	}
}

// names are the names the web app's tests key: each way the table folds,
// and the names a folding of its own got wrong.
var names = []string{
	"Slice", "BIG  slice", "  piece ", "Gro\u00dfes St\u00fcck", "GRO\u1e9eES ST\u00dcCK", "grosses st\u00fcck",
	"\ufb02 oz", "FL OZ", "\u00b5g", "\u03bcg", "\u039cG", "\u03ba\u03bf\u03c5\u03c0\u03b1\u03c2", "\u039a\u039f\u03a5\u03a0\u0391\u03a3",
	"I", "\u0131", "\u0130", "i\u0307", "\u01c5", "\u01c6", "\u01c4", "\u13a0", "\uab70", "\u1ffc", "\u1fb3", "\u212a", "\u212b", "\u2126",
	"\u0106evapi", "Cevapi", "C\u0301evapi", "slice\u00a0x", "slice\u0085x", "slice\ufeffx", "a\u3000b",
}

// The web app's table and its cases are the ones Key gives: refresh them
// with go test ./internal/domain/food -run TestWebKey -update.
func TestWebKeyTableIsCurrent(t *testing.T) {
	t.Parallel()

	table := newKeyTable()
	tableJSON, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	keys := make([][2]string, len(names))
	for i, n := range names {
		keys[i] = [2]string{n, food.Key(n)}
	}
	casesJSON, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct {
		path string
		got  []byte
	}{{tableFile, append(tableJSON, '\n')}, {casesFile, append(casesJSON, '\n')}} {
		if *update {
			if err := os.WriteFile(f.path, f.got, 0o600); err != nil {
				t.Fatalf("write %s: %v", f.path, err)
			}
			continue
		}
		want, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatalf("read %s: %v (create it with -update)", f.path, err)
		}
		if !bytes.Equal(f.got, want) {
			t.Errorf("%s is stale; refresh it with: go test ./internal/domain/food -run TestWebKey -update", f.path)
		}
	}
}

// The table keys every rune, and the names, as Key does: the web app,
// which folds by it, cannot tell names apart otherwise than the server.
func TestWebKeyTableKeysAsKey(t *testing.T) {
	t.Parallel()

	webKey := webKeyer(newKeyTable())
	for r := range rune(unicode.MaxRune + 1) {
		if !utf8.ValidRune(r) {
			continue
		}
		s := "a" + string(r) + "b"
		if got, want := webKey(s), food.Key(s); got != want {
			t.Errorf("U+%04X: the table keys %q, Key %q", r, got, want)
		}
	}
	for _, n := range names {
		if got, want := webKey(n), food.Key(n); got != want {
			t.Errorf("%q: the table keys %q, Key %q", n, got, want)
		}
	}
}
