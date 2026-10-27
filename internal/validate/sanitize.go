package validate

import (
	"strings"
	"unicode"
)

const replacementChar = '�'

// Sanitize cleans text: it drops control characters and the byte order mark, collapses runs of
// whitespace and trims. With multiline the line breaks stay, one blank line at most. Text holding
// U+FFFD (a damaged encoding, seen in imported company names) is never rewritten: the mark stays and
// the result carries a warning and a suggestion with the mark removed.
func Sanitize(field, raw string, multiline bool) (string, []Issue, []Suggestion) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r == 0xFEFF:
		case r == '\n' || r == '\r' || r == '\t':
			if multiline && r != '\t' {
				b.WriteRune('\n')
			} else {
				b.WriteRune(' ')
			}
		case unicode.IsControl(r):
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	s := b.String()
	if multiline {
		lines := strings.Split(s, "\n")
		out := lines[:0]
		blank := 0
		for _, l := range lines {
			l = collapseSpaces(l)
			if l == "" {
				blank++
				if blank > 1 {
					continue
				}
			} else {
				blank = 0
			}
			out = append(out, l)
		}
		s = strings.TrimSpace(strings.Join(out, "\n"))
	} else {
		s = collapseSpaces(s)
	}
	if !strings.ContainsRune(s, replacementChar) {
		return s, nil, nil
	}
	fixed := strings.TrimSpace(collapseSpaces(strings.ReplaceAll(s, string(replacementChar), "")))
	return s,
		[]Issue{{Code: CodeEncodingDamaged, Field: field, Expected: "text without the replacement character U+FFFD",
			Received: s, Fix: "retype the damaged characters or accept the suggestion that removes them"}},
		[]Suggestion{{Code: CodeEncodingDamaged, Field: field, Target: field, Value: fixed,
			Message: "remove the damaged characters"}}
}

func collapseSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }
