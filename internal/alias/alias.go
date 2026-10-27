// Package alias is the one source of given-name aliases (Zach and Zachary, Rich and Richard). The
// built-in set lives here. The user's additions and removals live in the name_aliases table and are
// merged in by a Table. The duplicate finder, person search, the command bar and the relation
// typeahead all read a Table, so one change reaches every door.
package alias

import (
	"sort"
	"strings"
	"unicode"
)

// Key is the form an alias is stored and compared in: one lower-case word. A name that is empty or
// holds a space or a character other than a letter, an apostrophe or a hyphen gives "".
func Key(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return ""
	}
	for _, r := range n {
		if !unicode.IsLetter(r) && r != '\'' && r != '-' {
			return ""
		}
	}
	return n
}

// Builtin returns a copy of the built-in alias to canonical map.
func Builtin() map[string]string {
	out := make(map[string]string, len(builtin))
	for k, v := range builtin {
		out[k] = v
	}
	return out
}

// Table answers alias questions for one effective set: the built-ins minus the removed ones plus the
// user's additions. The zero Table and a nil *Table behave as the built-in set.
type Table struct {
	canon map[string]string   // alias -> canonical
	alias map[string][]string // canonical -> aliases, sorted
}

var defaultTable = NewTable(nil, nil)

// Default is the built-in set alone.
func Default() *Table { return defaultTable }

// NewTable builds the effective set. added maps alias to canonical, removed lists built-in aliases
// that are turned off. Keys are passed through Key. An addition whose canonical is itself an alias
// follows it to its canonical, so every canonical is a root.
func NewTable(added map[string]string, removed map[string]bool) *Table {
	canon := map[string]string{}
	for a, c := range builtin {
		if !removed[a] {
			canon[a] = c
		}
	}
	for a, c := range added {
		ka, kc := Key(a), Key(c)
		if ka == "" || kc == "" || ka == kc {
			continue
		}
		canon[ka] = kc
	}
	for a, c := range canon { // flatten one level of chains
		if root, ok := canon[c]; ok && root != a {
			canon[a] = root
		}
	}
	t := &Table{canon: canon, alias: map[string][]string{}}
	for a, c := range canon {
		t.alias[c] = append(t.alias[c], a)
	}
	for c := range t.alias {
		sort.Strings(t.alias[c])
	}
	return t
}

func (t *Table) tbl() *Table {
	if t == nil || t.canon == nil {
		return defaultTable
	}
	return t
}

// Canonical returns the canonical given name for a normalized first name, or the name itself.
func (t *Table) Canonical(first string) string {
	if c, ok := t.tbl().canon[first]; ok {
		return c
	}
	return first
}

// Expand returns the names a typed word can stand for: the word itself first, then the name it is an
// alias of, or the aliases of the name when it is a full name. Rich gives rich and richard. Richard
// gives richard, rich, rick, ricky, richie and dick. One hop only, so Rich does not reach Dick.
// The word is compared lower case and the result is lower case.
func (t *Table) Expand(word string) []string {
	k := strings.ToLower(strings.TrimSpace(word))
	out := []string{k}
	if k == "" {
		return out
	}
	tb := t.tbl()
	if c, ok := tb.canon[k]; ok {
		return append(out, c)
	}
	return append(out, tb.alias[k]...)
}

// Names lists every alias of the table with its canonical name, sorted by alias.
func (t *Table) Names() [][2]string {
	tb := t.tbl()
	out := make([][2]string, 0, len(tb.canon))
	for a, c := range tb.canon {
		out = append(out, [2]string{a, c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// IsBuiltin reports whether alias is in the built-in set, and its built-in canonical name.
func IsBuiltin(alias string) (string, bool) {
	c, ok := builtin[Key(alias)]
	return c, ok
}
