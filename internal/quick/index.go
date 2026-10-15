// Package quick turns one line of text into a plan of changes and applies it. The CLI (crm do),
// the MCP tool crm_do and the TUI command line all use it, so a line does the same thing
// everywhere. Parsing reads no database: it works on an Env that the caller fills once.
package quick

import (
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// Person names a person in a plan.
type Person struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Index finds people by name. Build it once per change of the people list.
type Index struct {
	people   []model.PersonSummary
	folded   []string // folded full name per person
	byFull   map[string][]int
	byFirst  map[string][]int
	byLast   map[string][]int
	byID     map[int64]int
	names    []string
	maxWords int
	nameSet  *shortsyntax.NameSet
}

// NewIndex builds an Index over the people summaries.
func NewIndex(people []model.PersonSummary) *Index {
	ix := &Index{
		people: people, folded: make([]string, len(people)),
		byFull: map[string][]int{}, byFirst: map[string][]int{}, byLast: map[string][]int{}, byID: map[int64]int{},
	}
	for i, p := range people {
		full := shortsyntax.Fold(p.Name)
		ix.folded[i] = full
		ix.byID[p.ID] = i
		if full == "" {
			continue
		}
		ix.names = append(ix.names, p.Name)
		ix.byFull[full] = append(ix.byFull[full], i)
		words := strings.Fields(full)
		if len(words) > ix.maxWords {
			ix.maxWords = len(words)
		}
		ix.byFirst[words[0]] = append(ix.byFirst[words[0]], i)
		if len(words) > 1 {
			ix.byLast[words[len(words)-1]] = append(ix.byLast[words[len(words)-1]], i)
		}
	}
	ix.nameSet = shortsyntax.NewNameSet(ix.names)
	return ix
}

// Len is the number of people.
func (ix *Index) Len() int {
	if ix == nil {
		return 0
	}
	return len(ix.people)
}

// Names are the full names, for shortsyntax.Options.People.
func (ix *Index) Names() []string {
	if ix == nil {
		return nil
	}
	return ix.names
}

// Get returns a person by id.
func (ix *Index) Get(id int64) (model.PersonSummary, bool) {
	if ix == nil {
		return model.PersonSummary{}, false
	}
	i, ok := ix.byID[id]
	if !ok {
		return model.PersonSummary{}, false
	}
	return ix.people[i], true
}

func (ix *Index) person(i int) Person { return Person{ID: ix.people[i].ID, Name: ix.people[i].Name} }

func (ix *Index) list(idx []int) []Person {
	out := make([]Person, 0, len(idx))
	for _, i := range idx {
		out = append(out, ix.person(i))
	}
	sort.SliceStable(out, func(a, b int) bool { return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name) })
	return out
}

// Resolve finds people for a typed name. Exact is true when the whole name matched a full name.
// Without an exact match it tries the first name, the last name, and a prefix of the full name.
func (ix *Index) Resolve(name string) (matches []Person, exact bool) {
	if ix == nil {
		return nil, false
	}
	f := shortsyntax.Fold(name)
	if f == "" {
		return nil, false
	}
	if idx := ix.byFull[f]; len(idx) > 0 {
		return ix.list(idx), true
	}
	if !strings.Contains(f, " ") {
		if idx := ix.byFirst[f]; len(idx) > 0 {
			return ix.list(idx), false
		}
		if idx := ix.byLast[f]; len(idx) > 0 {
			return ix.list(idx), false
		}
	}
	return nil, false
}

// ResolveLoose is Resolve with a fallback to people whose full name starts with the text. Use it
// where the user named the person on purpose (+Sam, "set Sam:").
func (ix *Index) ResolveLoose(name string) (matches []Person, exact bool) {
	if m, ex := ix.Resolve(name); len(m) > 0 {
		return m, ex
	}
	if ix == nil {
		return nil, false
	}
	f := shortsyntax.Fold(name)
	if f == "" {
		return nil, false
	}
	var idx []int
	for i, full := range ix.folded {
		if strings.HasPrefix(full, f) {
			idx = append(idx, i)
		}
	}
	return ix.list(idx), false
}

// Suggest lists up to limit people whose name starts with prefix (a word prefix of any name word).
func (ix *Index) Suggest(prefix string, limit int) []Person {
	if ix == nil {
		return nil
	}
	f := shortsyntax.Fold(prefix)
	// Names that start with the prefix come first, then names where a later word starts with it.
	var first, later []int
	for i, full := range ix.folded {
		switch {
		case full == "":
		case f == "" || strings.HasPrefix(full, f):
			first = append(first, i)
		case strings.Contains(full, " "+f):
			later = append(later, i)
		}
	}
	out := append(ix.list(first), ix.list(later)...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// NameSet is the full names prepared for shortsyntax.Options.Names.
func (ix *Index) NameSet() *shortsyntax.NameSet {
	if ix == nil {
		return nil
	}
	return ix.nameSet
}

// HasTag reports whether the person carries the tag (ignoring case).
func (ix *Index) HasTag(id int64, tag string) bool {
	p, ok := ix.Get(id)
	if !ok {
		return false
	}
	for _, t := range strings.Split(p.Tags, ",") {
		if strings.EqualFold(strings.TrimSpace(t), tag) {
			return true
		}
	}
	return false
}
