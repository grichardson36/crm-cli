package query

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Querier is the part of *sql.DB and *sql.Tx that Run needs.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Row is one matching object.
type Row struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Sub   string `json:"detail,omitempty"`
	Meta  string `json:"meta,omitempty"`
	// Group is the group key (empty when the view is not grouped or the value is empty), GroupLabel the
	// heading text. Extra holds the optional columns in the order of Query.Columns.
	Group      string   `json:"group,omitempty"`
	GroupLabel string   `json:"group_label,omitempty"`
	Extra      []string `json:"columns,omitempty"`
}

// Run compiles and runs the query. Rows are ordered by the group first (when there is one), then by the
// sort of the query.
func Run(ctx context.Context, db Querier, kind string, q Query, env Env) ([]Row, error) {
	st, err := Compile(kind, q, env)
	if err != nil {
		return nil, err
	}
	return st.Run(ctx, db)
}

// Run runs a compiled statement.
func (st *Stmt) Run(ctx context.Context, db Querier) ([]Row, error) {
	rows, err := db.QueryContext(ctx, st.SQL, st.Args...)
	if err != nil {
		return nil, fmt.Errorf("run %s view: %w", st.Kind, err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var grp string
		extra := make([]string, len(st.cols))
		dest := []any{&r.ID, &r.Label, &r.Sub, &r.Meta, &grp}
		for i := range extra {
			dest = append(dest, &extra[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("read %s view: %w", st.Kind, err)
		}
		r.Group, r.Extra = grp, extra
		if len(extra) == 0 {
			r.Extra = nil
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read %s view: %w", st.Kind, err)
	}
	if st.group != nil {
		st.arrange(out)
		return st.sortGroups(out), nil
	}
	return out, nil
}

// arrange fills the heading text of each row.
func (st *Stmt) arrange(rows []Row) {
	for i := range rows {
		rows[i].GroupLabel = st.groupLabel(rows[i].Group)
	}
}

// groupLabel is the heading text of a group key.
func (st *Stmt) groupLabel(key string) string {
	f := st.group
	if key == "" {
		return "(none)"
	}
	switch {
	case f.Birthday:
		if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= 12 {
			return time.Month(n).String()
		}
	case f.Class == ClassDate:
		switch bucketOf(st.query) {
		case BucketMonth:
			if t, err := time.Parse("2006-01", key); err == nil {
				return t.Format("January 2006")
			}
		case BucketWeek:
			return "Week of " + key
		default:
			if t, err := time.Parse("2006-01-02", key); err == nil {
				return key + " " + t.Format("Mon")
			}
		}
	case f.Class == ClassChoice:
		for _, c := range f.Choices {
			if strings.EqualFold(c.Value, key) {
				return c.Label
			}
		}
	}
	return key
}

// choiceOrder is the position of each value of a choice group, from the field's options or from the
// live vocabulary.
func (st *Stmt) choiceOrder() map[string]int {
	f := st.group
	order := map[string]int{}
	if f.Vocabulary != "" && st.env.Choices != nil {
		for i, v := range st.env.Choices(f.Vocabulary) {
			order[strings.ToLower(v)] = i
		}
	}
	for i, c := range f.Choices {
		order[strings.ToLower(c.Value)] = i
	}
	return order
}

// sortGroups orders the groups sensibly and keeps the order of the rows inside each: dates by time,
// choices in their pick order (deal stages in stage order), numbers by value, text alphabetically, the
// rows with no value last. A descending sort on the group field reverses the order of the groups.
func (st *Stmt) sortGroups(rows []Row) []Row {
	f := st.group
	desc := len(st.query.Sort) > 0 && st.query.Sort[0].Field == f.Key && st.query.Sort[0].Desc()
	var order map[string]int
	if f.Class == ClassChoice {
		order = st.choiceOrder()
	}
	less := func(a, b string) bool {
		switch {
		case f.Class == ClassChoice:
			ia, oka := order[strings.ToLower(a)]
			ib, okb := order[strings.ToLower(b)]
			if oka && okb {
				return ia < ib
			}
			if oka != okb {
				return oka
			}
		case f.Class == ClassNumber:
			na, ea := strconv.ParseFloat(a, 64)
			nb, eb := strconv.ParseFloat(b, 64)
			if ea == nil && eb == nil && na != nb {
				return na < nb
			}
		}
		return strings.ToLower(a) < strings.ToLower(b)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].Group, rows[j].Group
		if a == b {
			return false
		}
		if a == "" || b == "" {
			return b == "" // an empty group is always last
		}
		if desc {
			return less(b, a)
		}
		return less(a, b)
	})
	return rows
}
