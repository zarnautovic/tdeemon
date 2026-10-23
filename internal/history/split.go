package history

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// The splitting of the history (DESIGN.md §10): the predecessor kept a
// meal as one line of text, and a food of its table at times as a meal
// of several (a combination: "Jogurt + Whey + Müsli"). A split proposes
// the parts of each, an amount and a food of the table for each part it
// can tell, for the user to review before anything is written of it.

// Part is a part of a meal or a combination: what the text says of it,
// the name left once its amount is taken out, the amount, in g, ml or
// pieces, and the food of the table it reads as, when one does.
type Part struct {
	Text   string  `json:"text"`
	Name   string  `json:"name"`
	Amount float64 `json:"amount,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	Food   string  `json:"food,omitempty"`
}

// The units of a part's amount.
const (
	Grams      = "g"
	Millilitre = "ml"
	Pieces     = "piece"
)

// MealClass is how far a meal's parts can be told.
type MealClass string

// The classes of a meal, the most told first: every part has its amount
// and a food of the table; every part its amount, but some no food;
// some parts an amount; no part one.
const (
	Known   MealClass = "known"
	Amounts MealClass = "amounts"
	Partial MealClass = "partial"
	Untold  MealClass = "untold"
)

// MealSplit is a meal of the history and its parts.
type MealSplit struct {
	Date        string    `json:"date"`
	Time        string    `json:"time,omitempty"`
	Description string    `json:"description"`
	Kcal        float64   `json:"kcal"`
	Class       MealClass `json:"class"`
	Parts       []Part    `json:"parts"`
}

// FoodSplit is a food of the table that reads as a combination, and its
// parts, from its portion where that names them, else from its name.
type FoodSplit struct {
	Line    int    `json:"line"`
	Name    string `json:"name"`
	Portion string `json:"portion"`
	Parts   []Part `json:"parts"`
}

// SplitReport is the split proposed of a history: its meals, each with
// its parts, and the foods of its table that read as combinations.
type SplitReport struct {
	Meals []MealSplit `json:"meals"`
	Foods []FoodSplit `json:"foods"`
}

// Count is how many meals are of each class.
func (r SplitReport) Count() map[MealClass]int {
	n := map[MealClass]int{}
	for _, m := range r.Meals {
		n[m.Class]++
	}
	return n
}

// ProposeSplits splits each meal of days, and each food of foods that
// reads as a combination, against the foods of the table that do not.
func ProposeSplits(days []Day, foods []FoodRow) SplitReport {
	var plain []FoodRow
	r := SplitReport{Meals: []MealSplit{}, Foods: []FoodSplit{}}
	for i := range foods {
		if !combination(&foods[i]) {
			plain = append(plain, foods[i])
		}
	}
	m := newMatcher(plain)
	for i := range foods {
		f := &foods[i]
		if !combination(f) {
			continue
		}
		text := f.Name
		if strings.Contains(f.Portion, "+") {
			text = f.Portion
		}
		r.Foods = append(r.Foods, FoodSplit{Line: f.Line, Name: f.Name, Portion: f.Portion, Parts: m.parts(text)})
	}
	for _, d := range days {
		for _, meal := range d.Meals {
			parts := m.parts(meal.Description)
			r.Meals = append(r.Meals, MealSplit{
				Date: d.Date, Time: meal.Time, Description: meal.Description,
				Kcal: meal.Nutrients.Kcal, Class: classOf(parts), Parts: parts,
			})
		}
	}
	return r
}

// combination reports whether a food of the table is several foods: a
// plus in its name or its portion.
func combination(f *FoodRow) bool {
	return strings.Contains(f.Name, " + ") || strings.Contains(f.Portion, "+")
}

func classOf(parts []Part) MealClass {
	amounts, foods := 0, 0
	for _, p := range parts {
		if p.Amount > 0 {
			amounts++
			if p.Food != "" {
				foods++
			}
		}
	}
	switch {
	case len(parts) > 0 && foods == len(parts):
		return Known
	case len(parts) > 0 && amounts == len(parts):
		return Amounts
	case amounts > 0:
		return Partial
	default:
		return Untold
	}
}

// SplitText returns the parts of a meal's text: a label before a colon
// left out when the parts follow it ("French toast: 120 g kruha, 2
// jaja"), the rest split at each " + ", and a part at its commas and
// " i " too when each piece then has an amount and a name of its own, so
// "pistacije, 15 komada" stays one part. A split ignores what is in
// brackets.
func SplitText(text string) []string {
	text = strings.TrimSpace(text)
	if i := topIndex(text, ": "); i > 0 && len(topSplit(text[i+2:], ", ")) > 1 {
		text = text[i+2:]
	}
	var out []string
	for _, piece := range topSplit(text, " + ") {
		if subs := splitList(piece); len(subs) > 1 && allTold(subs) {
			out = append(out, subs...)
		} else if subs := splitAfterBrackets(piece); len(subs) > 1 && allTold(subs) {
			// "tost sa sirom i šunkom (2 kriške) i jogurt (200 ml)": the
			// " i " inside a part's name stays.
			out = append(out, subs...)
		} else {
			out = append(out, piece)
		}
	}
	return out
}

// splitAfterBrackets splits s at the commas and " i " that follow a
// closing bracket only.
func splitAfterBrackets(s string) []string {
	s = strings.NewReplacer("), ", ")\x00", ") i ", ")\x00").Replace(s)
	return topSplit(s, "\x00")
}

func splitList(s string) []string {
	var out []string
	for _, a := range topSplit(s, ", ") {
		out = append(out, topSplit(a, " i ")...)
	}
	return out
}

// allTold reports whether each of subs has an amount and a name.
func allTold(subs []string) bool {
	for _, s := range subs {
		amount, _ := amountOf(s)
		if amount == 0 || nameOf(s) == "" {
			return false
		}
	}
	return true
}

// topIndex is the index of the first sep in s outside brackets, or -1.
func topIndex(s, sep string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth = max(0, depth-1)
		default:
			if depth == 0 && strings.HasPrefix(s[i:], sep) {
				return i
			}
		}
	}
	return -1
}

// topSplit splits s at each sep outside brackets, trimming the pieces
// and leaving out empty ones.
func topSplit(s, sep string) []string {
	var out []string
	for {
		i := topIndex(s, sep)
		if i < 0 {
			break
		}
		if p := strings.TrimSpace(s[:i]); p != "" {
			out = append(out, p)
		}
		s = s[i+len(sep):]
	}
	if p := strings.TrimSpace(s); p != "" {
		out = append(out, p)
	}
	return out
}

var (
	// number is a number as the notes write one: a decimal comma or
	// point, a range ("450–500"), "about" (~) before it.
	number = `~?\s*(\d+(?:[.,]\d+)?(?:/\d+)?)(?:\s*[–-]\s*(\d+(?:[.,]\d+)?))?`
	// measured is a number with a unit of weight or volume, times a
	// count before it ("2 × 220 g").
	measured = regexp.MustCompile(`(?i)(?:(\d+)\s*[×x]\s*)?` + number + `\s*(kg|g|dl|ml|l)\b`)
	// bracketCount is a count of pieces a bracket starts with ("(1
	// srednja)").
	bracketCount = regexp.MustCompile(`^\(\s*` + number + `\s+\pL`)
	// counted is a number of pieces before the name ("2 jaja").
	counted = regexp.MustCompile(`^` + number + `\s+\pL`)
	// bracketed is a part in brackets.
	bracketed = regexp.MustCompile(`\([^()]*\)`)
	// leading is a number a part's text starts with.
	leading = regexp.MustCompile(`^` + number + `\s+`)
	// fillers are the words around an amount, which name no food.
	fillers = regexp.MustCompile(`(?i)\b(oko|procjena|procijenjeno|cca)\b:?`)
	// countWords are what a count is of, not a food's name.
	countWords = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(komada|komad|kom|kriške|kriška|šnite|šnita|srednjih|srednja|srednje|srednji|mala|male|malih|veća|velika|velike|mjerice|mjerica|šake|šaka|žlice|žlica|žličice|žličica)(\s|$|[,;)])`)
)

// amountOf returns the amount of a part's text and its unit: a weight or
// a volume first, one in brackets before one outside ("2 mjerice (60
// g)" is 60 g), else a count of pieces before its name or in brackets;
// 0 for none.
func amountOf(s string) (float64, string) {
	brackets := bracketed.FindAllString(s, -1)
	for _, m := range brackets {
		if v, unit, ok := measure(m); ok {
			return v, unit
		}
	}
	if v, unit, ok := measure(bracketed.ReplaceAllString(s, " ")); ok {
		return v, unit
	}
	if m := counted.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		return mean(m[1], m[2]), Pieces
	}
	for _, b := range brackets {
		if m := bracketCount.FindStringSubmatch(b); m != nil {
			return mean(m[1], m[2]), Pieces
		}
	}
	// "banana, 1 srednja"
	if m := countWords.FindStringSubmatch(s); m != nil {
		return parseDecimal(m[1]), Pieces
	}
	return 0, ""
}

func measure(s string) (float64, string, bool) {
	m := measured.FindStringSubmatch(s)
	if m == nil {
		return 0, "", false
	}
	v := mean(m[2], m[3])
	if m[1] != "" {
		v *= parseDecimal(m[1])
	}
	switch strings.ToLower(m[4]) {
	case "kg":
		return v * 1000, Grams, true
	case "g":
		return v, Grams, true
	case "l":
		return v * 1000, Millilitre, true
	case "dl":
		return v * 100, Millilitre, true
	default:
		return v, Millilitre, true
	}
}

// mean is a number, or a range's middle.
func mean(a, b string) float64 {
	x := parseDecimal(a)
	if b == "" {
		return x
	}
	return (x + parseDecimal(b)) / 2
}

func parseDecimal(s string) float64 {
	if num, den, ok := strings.Cut(s, "/"); ok {
		d := parseDecimal(den)
		if d == 0 {
			return 0
		}
		return math.Round(parseDecimal(num)/d*100) / 100
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil {
		return 0
	}
	return math.Round(v*100) / 100
}

// nameOf is a part's text without what is in brackets, its amounts and
// counts, and the words around them ("oko", "procjena").
func nameOf(s string) string {
	s = bracketed.ReplaceAllString(s, " ")
	s = measured.ReplaceAllString(s, " ")
	s = countWords.ReplaceAllString(s, " $3")
	s = fillers.ReplaceAllString(s, " ")
	s = leading.ReplaceAllString(strings.TrimSpace(s), "")
	return strings.Trim(strings.Join(strings.Fields(s), " "), " ,;:~-–—")
}

// matcher tells the food of the table a part's name reads as.
type matcher struct {
	foods []FoodRow
	words [][]string
}

func newMatcher(foods []FoodRow) matcher {
	m := matcher{foods: foods}
	for i := range foods {
		m.words = append(m.words, wordsOf(foods[i].Name))
	}
	return m
}

func (m matcher) parts(text string) []Part {
	parts := []Part{}
	for _, t := range SplitText(text) {
		amount, unit := amountOf(t)
		name := nameOf(t)
		parts = append(parts, Part{Text: t, Name: name, Amount: amount, Unit: unit, Food: m.food(name)})
	}
	return parts
}

// food returns the name of the food name reads as, or "": the one whose
// words name's words cover the most of, at least half of them, by more
// than half of name's words; a word covers the same word inflected
// (sameWord).
func (m matcher) food(name string) string {
	words := wordsOf(name)
	if len(words) == 0 {
		return ""
	}
	best, bestScore := "", 0.0
	for i, fw := range m.words {
		if len(fw) == 0 {
			continue
		}
		hit := 0
		for _, w := range words {
			for _, f := range fw {
				if sameWord(w, f) {
					hit++
					break
				}
			}
		}
		score := float64(hit) / float64(len(fw))
		if hit == 0 || 2*hit <= len(words) || score < 0.5 {
			continue
		}
		if score > bestScore {
			best, bestScore = m.foods[i].Name, score
		}
	}
	return best
}

// sameWord reports whether a and b are one word inflected ("jogurta"
// and "jogurt", "maline" and "malina"): without their last vowels, one
// starts with the other, which is three letters at least and 60 % of the
// other's length; "piletina" is not "pileća".
func sameWord(a, b string) bool {
	a, b = strings.TrimRight(a, "aeiou"), strings.TrimRight(b, "aeiou")
	if len(a) > len(b) {
		a, b = b, a
	}
	return len(a) >= 3 && 10*len(a) >= 6*len(b) && strings.HasPrefix(b, a)
}

// stopWords are words of a name that name no food.
var stopWords = map[string]bool{
	"sa": true, "od": true, "na": true, "za": true, "malo": true, "uz": true,
	"bez": true, "pola": true, "the": true, "and": true, "with": true,
}

// wordsOf is s's words of three letters or more, folded: lower case,
// without diacritics, stop words left out.
func wordsOf(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(fold(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) >= 3 && !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

func fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, strings.ToLower(s))
	if err != nil {
		return strings.ToLower(s)
	}
	return strings.ReplaceAll(out, "đ", "d")
}
