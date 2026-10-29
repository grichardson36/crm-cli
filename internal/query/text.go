package query

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/validate"
)

var opAliases = map[string]string{
	"!=": OpIsNot, "<>": OpIsNot, "eq": OpEq, "==": OpEq, "lt": OpLt, "gt": OpGt,
	"isnot": OpIsNot, "notempty": OpNotEmpty, "withinlast": OpWithinLast, "innext": OpInNext,
	"linkedto": OpLinkedTo, "notlinkedto": OpNotLinkedTo, "hasany": OpHasAny, "doesnothave": OpNotHas, "not_have": OpNotHas,
}

var knownOps = func() map[string]bool {
	m := map[string]bool{}
	for _, ops := range operatorsByClass {
		for _, op := range ops {
			m[op] = true
		}
	}
	return m
}()

func normOp(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	if a, ok := opAliases[s]; ok {
		return a
	}
	return s
}

// ParseWhere reads one condition written on a command line: FIELD OP [VALUE|VALUE...]. The operator is
// one of the names of OperatorsFor (is, is_not, contains, empty, not_empty, before, after, within_last,
// in_next, in, =, <, >, between, linked_to, not_linked_to, has_any, has, does_not_have), a two word form
// such as "is not" also works, and several values are separated by |. Examples:
//
//	last_contacted before 90 days ago
//	tag has client|lead
//	value between 1000|5000
func ParseWhere(s string) (Condition, error) {
	toks := strings.Fields(s)
	bad := func(msg string) (Condition, error) {
		return Condition{}, validate.Fail(validate.Issue{Code: validate.CodeQueryOperator, Field: s, Expected: "FIELD OPERATOR [VALUE|VALUE]", Received: s, Fix: msg})
	}
	if len(toks) < 2 {
		return bad("write the field, the operator and the values, for example: last_contacted before 90 days ago")
	}
	c := Condition{Field: toks[0]}
	rest := toks[2:]
	op := normOp(toks[1])
	for n := 4; n >= 2; n-- { // longest operator first: does not have, not linked to
		if len(toks) >= n {
			if multi := normOp(strings.Join(toks[1:n], "_")); knownOps[multi] {
				op, rest = multi, toks[n:]
				break
			}
		}
	}
	c.Op = op
	if len(rest) > 0 {
		for _, v := range strings.Split(strings.Join(rest, " "), "|") {
			if v = strings.TrimSpace(v); v != "" {
				c.Values = append(c.Values, v)
			}
		}
	}
	return c, nil
}

// ParseSort reads FIELD or FIELD:asc or FIELD:desc.
func ParseSort(s string) (Sort, error) {
	field, dir, _ := strings.Cut(strings.TrimSpace(s), ":")
	dir = strings.ToLower(strings.TrimSpace(dir))
	if field == "" || (dir != "" && dir != "asc" && dir != "desc") {
		return Sort{}, validate.Fail(validate.Issue{Code: validate.CodeQuerySort, Field: s, Expected: "FIELD or FIELD:asc or FIELD:desc", Received: s, Fix: "write the field, then :asc or :desc"})
	}
	return Sort{Field: strings.TrimSpace(field), Dir: dir}, nil
}

// Describe writes the conditions of a query as one line a person can read. label turns a relation value
// (a uuid) into the name to show, nil leaves the value as stored.
func Describe(kind string, q Query, label func(farTable, value string) string) string {
	var parts []string
	for _, c := range q.Filter {
		f, _ := FieldOf(kind, c.Field)
		text := c.Field
		if f.Label != "" {
			text = f.Label
		}
		text += " " + OperatorLabel(c.Op)
		if len(c.Values) > 0 {
			vals := make([]string, len(c.Values))
			for i, v := range c.Values {
				vals[i] = v
				if label != nil && f.rel != nil {
					if l := label(f.FarTable(), v); l != "" {
						vals[i] = l
					}
				}
				if f.Class == ClassChoice {
					vals[i] = f.choiceLabel(v)
				}
			}
			sep := " or "
			if c.Op == OpBetween {
				sep = " and "
			}
			text += " " + strings.Join(vals, sep)
		}
		parts = append(parts, text)
	}
	if q.Search != "" {
		parts = append(parts, fmt.Sprintf("text contains %q", q.Search))
	}
	if len(parts) == 0 {
		return "everything"
	}
	return strings.Join(parts, ", ")
}

func (f Field) choiceLabel(v string) string {
	for _, c := range f.Choices {
		if strings.EqualFold(c.Value, v) {
			return c.Label
		}
	}
	return v
}

// DescribeOrder writes the sort and group of a query: "sorted by value desc, grouped by stage".
func DescribeOrder(q Query) string {
	var parts []string
	if len(q.Sort) > 0 {
		var ss []string
		for _, s := range q.Sort {
			d := s.Dir
			if d == "" {
				d = "asc"
			}
			ss = append(ss, s.Field+" "+d)
		}
		parts = append(parts, "sorted by "+strings.Join(ss, ", then "))
	}
	if q.GroupBy != "" {
		g := "grouped by " + q.GroupBy
		if q.Bucket != "" {
			g += " (" + q.Bucket + ")"
		}
		parts = append(parts, g)
	}
	return strings.Join(parts, ", ")
}

// ChoiceLabelOf is the label of a stored choice value.
func (f Field) ChoiceLabelOf(v string) string { return f.choiceLabel(v) }
