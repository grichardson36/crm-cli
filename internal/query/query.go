// Package query is the neutral declarative query of a saved view (patterns/saved-view-query). A query
// is data: a list of conditions over the fields of one object kind, a sort, a group and a list of
// columns. It holds no SQL. This package validates it (codes live in internal/validate), compiles it to
// one parameterized statement and runs it. The CLI, MCP, TUI and the TOML file all use these types.
package query

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The object kinds a saved view can be about. These are the words the CLI and MCP use.
const (
	KindPerson       = "person"
	KindOrganization = "organization"
	KindDeal         = "deal"
	KindTask         = "task"
	KindNote         = "note"
	KindTag          = "tag"
)

// Kinds lists the kinds a saved view supports, in display order.
var Kinds = []string{KindPerson, KindOrganization, KindDeal, KindTask, KindNote, KindTag}

// ValidKind reports whether k is a kind a saved view can be about.
func ValidKind(k string) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// The operators. A field class takes a fixed subset (OperatorsFor).
const (
	OpIs          = "is"
	OpIsNot       = "is_not"
	OpContains    = "contains"
	OpEmpty       = "empty"
	OpNotEmpty    = "not_empty"
	OpBefore      = "before"
	OpAfter       = "after"
	OpWithinLast  = "within_last"
	OpInNext      = "in_next"
	OpIn          = "in"
	OpEq          = "="
	OpLt          = "<"
	OpGt          = ">"
	OpBetween     = "between"
	OpLinkedTo    = "linked_to"
	OpNotLinkedTo = "not_linked_to"
	OpHasAny      = "has_any"
	OpHas         = "has"
	OpNotHas      = "does_not_have"
)

// Limits keep a query small enough to read and to run.
const (
	MaxConditions = 20
	MaxValues     = 50
	MaxColumns    = 12
	MaxSorts      = 2
	MaxValueLen   = 200
	// MaxDays is the largest count of days of within_last and in_next.
	MaxDays = 3660
)

// Condition is one test on a field. Values holds the options: the condition holds when the field
// matches any of them (for the negative operators, none of them). between takes two values, within_last
// and in_next one count of days, empty, not_empty and has_any none.
type Condition struct {
	Field  string   `json:"field" toml:"field"`
	Op     string   `json:"op" toml:"op"`
	Values []string `json:"values,omitempty" toml:"values,omitempty"`
}

// Sort is one sort key. Dir is "asc" (default) or "desc".
type Sort struct {
	Field string `json:"field" toml:"field"`
	Dir   string `json:"dir,omitempty" toml:"dir,omitempty"`
}

// Desc reports whether the direction is descending.
func (s Sort) Desc() bool { return strings.EqualFold(s.Dir, "desc") }

// Query is the whole neutral query. Conditions are combined with AND.
type Query struct {
	Filter  []Condition `json:"filter,omitempty" toml:"filter,omitempty"`
	Sort    []Sort      `json:"sort,omitempty" toml:"sort,omitempty"`
	GroupBy string      `json:"group_by,omitempty" toml:"group_by,omitempty"`
	// Bucket is the size of a date group: day (default), week or month.
	Bucket  string   `json:"bucket,omitempty" toml:"bucket,omitempty"`
	Columns []string `json:"columns,omitempty" toml:"columns,omitempty"`
	// Search is plain text every word of which must appear in a text field of the object.
	Search string `json:"search,omitempty" toml:"search,omitempty"`
}

// Date buckets of a group.
const (
	BucketDay   = "day"
	BucketWeek  = "week"
	BucketMonth = "month"
)

// JSON is the stored form of the query.
func (q Query) JSON() string {
	b, err := json.Marshal(q)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Parse reads the stored JSON form. An empty string is the empty query.
func Parse(s string) (Query, error) {
	var q Query
	if strings.TrimSpace(s) == "" {
		return q, nil
	}
	if err := json.Unmarshal([]byte(s), &q); err != nil {
		return Query{}, fmt.Errorf("saved view query is not valid JSON: %w", err)
	}
	return q, nil
}

// Clone copies the query so a session edit never changes the saved one.
func (q Query) Clone() Query {
	out := Query{GroupBy: q.GroupBy, Bucket: q.Bucket, Search: q.Search}
	for _, c := range q.Filter {
		out.Filter = append(out.Filter, Condition{Field: c.Field, Op: c.Op, Values: append([]string(nil), c.Values...)})
	}
	out.Sort = append(out.Sort, q.Sort...)
	out.Columns = append(out.Columns, q.Columns...)
	return out
}
