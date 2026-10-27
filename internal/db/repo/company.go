package repo

import (
	"strings"
	"unicode"
)

// Company text helpers. people.company stays free text, the source text. NormalizeCompany is the
// key that matches it to an organization name (see org_auto.go).

// companySuffixes are dropped from the end of a company text for grouping only.
var companySuffixes = map[string]bool{
	"inc": true, "incorporated": true, "llc": true, "ltd": true, "limited": true, "corp": true, "corporation": true,
	"co": true, "company": true, "gmbh": true, "plc": true, "llp": true, "lp": true, "pllc": true, "sa": true, "ag": true, "bv": true,
}

// NormalizeCompany is the grouping key of a company text: trimmed, case-folded, whitespace collapsed,
// punctuation removed, "&" read as "and", a leading "the" and trailing legal suffixes (Inc, LLC, Ltd,
// Corp, Co and their dotted forms) dropped. It is used for grouping and matching only, the text
// itself is never rewritten. A text that is only a suffix keeps it.
func NormalizeCompany(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r == '&':
			b.WriteString(" and ")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.' || r == '\'' || r == '’':
			// "L.L.C." and "Bob's" lose the mark without splitting the word
		default:
			b.WriteByte(' ')
		}
	}
	words := strings.Fields(b.String())
	full := strings.Join(words, " ")
	if len(words) > 1 && words[0] == "the" {
		words = words[1:]
	}
	for len(words) > 1 && companySuffixes[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	if len(words) == 0 {
		return full
	}
	return strings.Join(words, " ")
}

// normalizeDomain lowercases a domain and drops a scheme, "www.", a path and an "@".
func normalizeDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "@"), "www.")
	return strings.Trim(s, ". ")
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
