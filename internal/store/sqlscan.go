package store

import (
	"strings"
	"unicode"
)

// transactionControl reports the first statement in script that would
// begin, commit or roll back a transaction, with the line it starts on.
// The runner wraps every migration in its own transaction, so a COMMIT in
// the file would commit half a migration. SAVEPOINT, RELEASE and
// ROLLBACK TO stay allowed: they nest inside the runner's transaction.
//
// Comments and quoted strings or identifiers are skipped, and so are
// trigger bodies, whose closing END (like the END of a CASE expression)
// is not a statement of its own.
func transactionControl(script string) (stmt string, line int, found bool) {
	inTrigger := false
	for _, st := range splitStatements(script) {
		words := strings.FieldsFunc(strings.ToUpper(st.text), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
		})
		if len(words) == 0 {
			continue
		}
		if inTrigger {
			// Body statements end with ";"; a lone END closes the trigger.
			inTrigger = len(words) != 1 || words[0] != "END"
			continue
		}
		if isCreateTrigger(words) {
			inTrigger = true
			continue
		}
		switch words[0] {
		case "BEGIN", "COMMIT", "END":
			return words[0], st.line, true
		case "ROLLBACK":
			// ROLLBACK [TRANSACTION] TO [SAVEPOINT] name.
			rest := words[1:]
			if len(rest) > 0 && rest[0] == "TRANSACTION" {
				rest = rest[1:]
			}
			if len(rest) == 0 || rest[0] != "TO" {
				return "ROLLBACK", st.line, true
			}
		}
	}
	return "", 0, false
}

// isCreateTrigger matches CREATE [TEMP | TEMPORARY] TRIGGER.
func isCreateTrigger(words []string) bool {
	if len(words) < 2 || words[0] != "CREATE" {
		return false
	}
	if words[1] == "TEMP" || words[1] == "TEMPORARY" {
		return len(words) > 2 && words[2] == "TRIGGER"
	}
	return words[1] == "TRIGGER"
}

type statement struct {
	text string // comments removed, quoted content replaced by a space
	line int    // 1-based line of the statement's first significant character
}

// splitStatements cuts script at every ";" outside comments, string
// literals and quoted identifiers.
func splitStatements(script string) []statement {
	var (
		s   = sqlScanner{runes: []rune(script), line: 1}
		out []statement
		cur strings.Builder
		at  int // line of the current statement's first character, 0 before it
	)
	flush := func() {
		if strings.TrimSpace(cur.String()) != "" {
			out = append(out, statement{text: cur.String(), line: at})
		}
		cur.Reset()
		at = 0
	}

	for ; s.i < len(s.runes); s.i++ {
		r := s.runes[s.i]
		switch {
		case r == '\n':
			s.line++
			cur.WriteRune(' ')
		case s.at("--"):
			s.skipLineComment()
		case s.at("/*"):
			s.skipBlockComment()
			cur.WriteRune(' ')
		case r == '\'' || r == '"' || r == '`' || r == '[':
			if at == 0 {
				at = s.line
			}
			s.skipQuoted()
			cur.WriteRune(' ')
		case r == ';':
			flush()
		default:
			if at == 0 && !unicode.IsSpace(r) {
				at = s.line
			}
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// sqlScanner walks a script one rune at a time and counts its lines. The
// skip methods leave i on the last rune of what they skip, for the caller
// to step past.
type sqlScanner struct {
	runes []rune
	i     int // the current rune
	line  int // 1-based line of the current rune
}

// at reports whether the script continues with prefix from the current rune.
func (s *sqlScanner) at(prefix string) bool {
	j := s.i
	for _, r := range prefix {
		if j >= len(s.runes) || s.runes[j] != r {
			return false
		}
		j++
	}
	return true
}

// skipLineComment stops before the newline, so the caller counts it.
func (s *sqlScanner) skipLineComment() {
	for s.i+1 < len(s.runes) && s.runes[s.i+1] != '\n' {
		s.i++
	}
}

// skipBlockComment stops on the "/" that closes the comment, or past the
// end when it is not closed.
func (s *sqlScanner) skipBlockComment() {
	for s.i += 2; s.i < len(s.runes) && !s.at("*/"); s.i++ {
		if s.runes[s.i] == '\n' {
			s.line++
		}
	}
	s.i++
}

// skipQuoted stops on the quote that closes the string or identifier
// opened at the current rune, or at the end when it is not closed.
func (s *sqlScanner) skipQuoted() {
	closing := s.runes[s.i]
	if closing == '[' {
		closing = ']'
	}
	for s.i++; s.i < len(s.runes); s.i++ {
		r := s.runes[s.i]
		if r == '\n' {
			s.line++
		}
		if r != closing {
			continue
		}
		// A doubled quote is an escaped quote, not the end; [...] has none.
		if closing != ']' && s.i+1 < len(s.runes) && s.runes[s.i+1] == closing {
			s.i++
			continue
		}
		return
	}
}
