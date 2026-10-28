// Package suggest is the one matching implementation behind every autocomplete in the program: the
// command bar's completions, the relation typeahead of a record page and the popup of the pipe-line
// prompts. It ranks candidates for the text typed so far. Names match through the shared nickname
// aliases (Zach finds Zachary), words match as prefixes anywhere in the name, and a misspelling
// (Zachery) is forgiven by the same spelling ratio the duplicate finder uses. It reads no database
// and draws nothing: callers pass the candidates and show the result (patterns/autocomplete-component).
package suggest

import (
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/alias"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

const (
	// DefaultLimit is the number of matches Rank returns when Options.Limit is zero.
	DefaultLimit = 5

	typoWord   = 0.75 // spelling ratio of a typed word to a word of a name
	typoFull   = 0.80 // spelling ratio of the typed text to the whole name
	minTypoLen = 3
)

// Options tune Rank.
type Options struct {
	// Aliases is the nickname table. Nil uses the built-in set.
	Aliases *alias.Table
	// Limit is the most matches returned. Zero means DefaultLimit.
	Limit int
}

func (o Options) limit() int {
	if o.Limit > 0 {
		return o.Limit
	}
	return DefaultLimit
}

// Rank orders the candidates that match text. label gives the text a candidate is matched on. Direct
// matches (the text, or an alias of a typed word, inside the name) come first, best first. Names one
// typo away follow while there is room. An empty text returns the first candidates in the order given.
func Rank[T any](items []T, text string, label func(T) string, o Options) []T {
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "#"))
	limit := o.limit()
	if text == "" {
		out := append([]T(nil), items...)
		if len(out) > limit {
			out = out[:limit]
		}
		return out
	}
	low := strings.ToLower(text)
	words := strings.Fields(low)
	type scored struct {
		it    T
		score int
	}
	var direct, typo []scored
	for _, it := range items {
		name := strings.ToLower(strings.TrimPrefix(label(it), "#"))
		sc := Score(name, low, words, o.Aliases)
		if sc >= 0 {
			direct = append(direct, scored{it, sc})
			continue
		}
		if len([]rune(low)) >= minTypoLen && Typo(name, low, words) {
			typo = append(typo, scored{it, 9})
		}
	}
	sort.SliceStable(direct, func(i, j int) bool { return direct[i].score < direct[j].score })
	var out []T
	for _, s := range direct {
		out = append(out, s.it)
	}
	for _, s := range typo {
		out = append(out, s.it)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Score is 0 for an exact name, 1 for a prefix, 2 for a word prefix, 3 for any other substring, and -1
// for no match. name and low are lower case, words are the words of low. Every word typed must match,
// each through its aliases.
func Score(name, low string, words []string, tbl *alias.Table) int {
	if name == low {
		return 0
	}
	if strings.HasPrefix(name, low) {
		return 1
	}
	best := 3
	for _, w := range words {
		ok := false
		for _, n := range tbl.Expand(w) {
			switch {
			case strings.HasPrefix(name, n):
				ok = true
			case strings.Contains(" "+name, " "+n):
				ok = true
				if best > 2 {
					best = 2
				}
			case strings.Contains(name, n):
				ok = true
			}
			if ok {
				break
			}
		}
		if !ok {
			return -1
		}
	}
	return best
}

// FuzzyWeak is the rank Fuzzy adds for a word that only matches as scattered letters. A rank at or
// above it means some word matched that loosely.
const FuzzyWeak = 30

// Fuzzy ranks free text against a haystack of words, for the palette and the pickers where the
// candidate is a title plus its hint, id and description. It returns a rank (lower is better) when
// every typed word is found in text: a word found as a substring ranks by how early it appears, and
// a word that only has its letters in order ranks FuzzyWeak. An empty query matches everything with
// rank 0. Rank is the sibling for names, where a typo or a nickname should still find the person.
func Fuzzy(query, text string) (int, bool) {
	text = strings.ToLower(text)
	score := 0
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if i := strings.Index(text, w); i >= 0 {
			score += min(i, 20) / 4
			continue
		}
		pos := 0
		for _, r := range w {
			j := strings.IndexRune(text[pos:], r)
			if j < 0 {
				return 0, false
			}
			pos += j + len(string(r))
		}
		score += FuzzyWeak
	}
	return score, true
}

// Typo says whether the text is a misspelling of the name: the whole text close to the whole name, or
// each typed word close to a word of the name (dedupe.Ratio).
func Typo(name, low string, words []string) bool {
	if dedupe.Ratio(low, name) >= typoFull {
		return true
	}
	parts := strings.Fields(name)
	for _, w := range words {
		if len([]rune(w)) < minTypoLen {
			return false
		}
		hit := false
		for _, p := range parts {
			if dedupe.Ratio(w, p) >= typoWord {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return len(words) > 0
}

// Prefix returns the items that start with prefix, ignoring case and accents, in the order given and
// without repeats. limit 0 means no limit. The command bar completes verbs, tags and presets with it.
func Prefix(items []string, prefix string, limit int) []string {
	f := shortsyntax.Fold(prefix)
	var out []string
	seen := map[string]bool{}
	for _, it := range items {
		if seen[it] {
			continue
		}
		if f == "" || strings.HasPrefix(shortsyntax.Fold(it), f) {
			seen[it] = true
			out = append(out, it)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// DatePhrases are the natural date phrases offered while a date is typed: the command bar after by,
// remind and on, and the date segment of a pipe line.
var DatePhrases = []string{
	"today", "tomorrow", "mon", "tue", "wed", "thu", "fri", "sat", "sun",
	"next week", "next month", "+1w", "+2w", "+1m", "9am", "noon", "3pm", "5pm",
}
