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
		out  []statement
		cur  strings.Builder
		line = 1
		at   = 0 // line of the current statement's first character
	)
	flush := func() {
		if strings.TrimSpace(cur.String()) != "" {
			out = append(out, statement{text: cur.String(), line: at})
		}
		cur.Reset()
		at = 0
	}
	mark := func() {
		if at == 0 {
			at = line
		}
	}

	runes := []rune(script)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\n':
			line++
			cur.WriteRune(' ')
		case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			i-- // let the newline be counted above
		case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
			i += 2
			for i < len(runes) && (runes[i] != '*' || i+1 >= len(runes) || runes[i+1] != '/') {
				if runes[i] == '\n' {
					line++
				}
				i++
			}
			i++ // skip the closing "/"
			cur.WriteRune(' ')
		case r == '\'' || r == '"' || r == '`' || r == '[':
			mark()
			closing := r
			if r == '[' {
				closing = ']'
			}
			for i++; i < len(runes); i++ {
				if runes[i] == '\n' {
					line++
				}
				if runes[i] != closing {
					continue
				}
				// A doubled quote is an escaped quote, not the end.
				if closing != ']' && i+1 < len(runes) && runes[i+1] == closing {
					i++
					continue
				}
				break
			}
			cur.WriteRune(' ')
		case r == ';':
			flush()
		default:
			if !unicode.IsSpace(r) {
				mark()
			}
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}
