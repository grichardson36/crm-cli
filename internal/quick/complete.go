package quick

import (
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// Completion is a set of ways to finish the word before the cursor. Replace line[Start:End] with a
// candidate.
type Completion struct {
	Start, End int
	Kind       string // verb, tag, date, cadence, priority, duration, person, type, field
	Candidates []string
}

// Presets offered after @ and !.
var datePresets = []string{
	"today", "tomorrow", "mon", "tue", "wed", "thu", "fri", "sat", "sun",
	"next week", "next month", "+1w", "+2w", "+1m", "9am", "noon", "3pm", "5pm",
}

// CadencePresets are offered after *.
var CadencePresets = []string{"day", "weekday", "week", "month", "year", "1w", "2w", "6w", "1m", "3m", "6m", "1y", "mon", "tue", "wed", "thu", "fri"}

var priorityPresets = []string{"top", "high", "medium", "low", "none"}
var durationPresets = []string{"15m", "30m", "45m", "1h", "2h", "4h"}

const maxCandidates = 12

// Complete suggests completions for the word that ends at cursor (a byte offset into line). A
// candidate equal to the word already typed is left out, except for verbs.
func Complete(line string, cursor int, env Env) Completion {
	c := complete(line, cursor, env)
	if c.Kind == "verb" || len(c.Candidates) == 0 || c.End > len(line) || c.Start > c.End {
		return c
	}
	word := line[c.Start:c.End]
	kept := make([]string, 0, len(c.Candidates))
	for _, cand := range c.Candidates {
		if cand != word && strings.TrimSpace(cand) != word {
			kept = append(kept, cand)
		}
	}
	c.Candidates = kept
	return c
}

func complete(line string, cursor int, env Env) Completion {
	if cursor > len(line) {
		cursor = len(line)
	}
	if cursor < 0 {
		cursor = 0
	}
	start := cursor
	for start > 0 && line[start-1] != ' ' && line[start-1] != '\t' {
		start--
	}
	word := line[start:cursor]
	c := Completion{Start: start, End: cursor}

	// Which word is this? Count the words before it.
	before := strings.Fields(line[:start])
	if len(before) == 0 {
		c.Kind = "verb"
		for _, v := range Verbs {
			if word != "" && strings.HasPrefix(v, strings.ToLower(word)) {
				c.Candidates = append(c.Candidates, v+" ")
			}
		}
		return c
	}
	verb, _ := VerbOf(before[0])

	if word != "" {
		switch word[0] {
		case '#':
			c.Kind = "tag"
			c.Candidates = prefixed("#", matchFold(sortedTags(env.Tags), word[1:]))
			return c
		case '!':
			c.Kind = "priority"
			c.Candidates = prefixed("!", matchFold(priorityPresets, word[1:]))
			if word == "!" {
				c.Candidates = append(c.Candidates, "!!", "!!!")
			}
			return c
		case '~':
			c.Kind = "duration"
			c.Candidates = prefixed("~", matchFold(durationPresets, word[1:]))
			return c
		case '@':
			c.Kind = "person"
			for _, p := range env.Index.Suggest(strings.Trim(word[1:], "\""), maxCandidates) {
				c.Candidates = append(c.Candidates, "@"+p.Name)
			}
			return c
		}
	}

	// The word before decides what a date, a repeat or a duration can be.
	switch strings.ToLower(before[len(before)-1]) {
	case "by", "remind", "on", "at":
		if verb != "set" && verb != "tag" {
			c.Kind = "date"
			c.Candidates = matchFold(datePresets, word)
			if len(c.Candidates) > 0 {
				return c
			}
		}
	case "every":
		c.Kind = "cadence"
		c.Candidates = matchFold(CadencePresets, word)
		return c
	case "for":
		c.Kind = "duration"
		c.Candidates = matchFold([]string{"15m", "30m", "45m", "1h", "2h", "4h"}, word)
		return c
	}
	pos := len(before) - 1 // words after the verb
	// log takes a type first.
	if verb == "log" {
		if pos == 0 {
			c.Kind = "type"
			types := env.NoteTypes
			if len(types) == 0 {
				types = []string{"call", "email", "meeting", "note", "message"}
			}
			for _, t := range matchFold(types, word) {
				c.Candidates = append(c.Candidates, t+" ")
			}
			return c
		}
		pos--
	}

	switch verb {
	case "set":
		if strings.Contains(word, "=") {
			return c
		}
		if pos > 0 || env.Context != 0 {
			if hasAssignment(before) || pos > 0 {
				c.Kind = "field"
				for _, k := range setKeys {
					if word == "" || strings.HasPrefix(k, strings.ToLower(word)) {
						c.Candidates = append(c.Candidates, k+"=")
					}
				}
				if len(c.Candidates) > 0 {
					return c
				}
			}
		}
	case "tag":
		if pos > 0 || env.Context != 0 {
			c.Kind = "tag"
			neg := strings.HasPrefix(word, "-")
			for _, t := range matchFold(sortedTags(env.Tags), strings.TrimPrefix(word, "-")) {
				if neg {
					t = "-" + t
				}
				c.Candidates = append(c.Candidates, t)
			}
			return c
		}
	case "snooze":
		if pos > 0 || env.Context != 0 {
			c.Kind = "date"
			c.Candidates = matchFold([]string{"7", "14", "30", "2w", "tomorrow", "next week", "next month"}, word)
			return c
		}
	}

	// The name position: the first word after the verb (after the type for log).
	if pos == 0 && env.Context == 0 && word != "" {
		c.Kind = "person"
		suffix := " "
		if verb == "set" {
			suffix = ": "
		}
		for _, p := range env.Index.Suggest(word, maxCandidates) {
			c.Candidates = append(c.Candidates, p.Name+suffix)
		}
	}
	return c
}

var setKeys = []string{"company", "title", "email", "phone", "location", "birthday", "cadence", "tier", "favorite", "notes", "name", "first", "last", "linkedin", "website", "twitter"}

func sortedTags(tags []string) []string {
	out := append([]string(nil), tags...)
	sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a]) < strings.ToLower(out[b]) })
	return out
}

func hasAssignment(words []string) bool {
	for _, w := range words {
		if strings.Contains(w, "=") {
			return true
		}
	}
	return false
}

func prefixed(prefix string, list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = prefix + s
	}
	return out
}

// matchFold returns the items that start with prefix (ignoring case and accents), in the order given.
func matchFold(items []string, prefix string) []string {
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
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}
