package query

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jdanielnd/crm-cli/internal/validate"
)

// Env carries what a query needs from outside: the clock for relative dates, and the live options of
// a user-managed list (deal stages) in their order.
type Env struct {
	Now time.Time
	// Choices returns the live values of a vocabulary in pick order. Nil means none are known, and a
	// vocabulary-backed field then accepts any text and groups alphabetically.
	Choices func(vocab string) []string
}

func (e Env) now() time.Time {
	if e.Now.IsZero() {
		return time.Now()
	}
	return e.Now
}

// ValidName checks the name of a saved view: one line, 1 to 80 characters. It returns the cleaned name.
func ValidName(name string) (string, error) {
	n := strings.TrimSpace(name)
	switch {
	case n == "":
		return "", validate.Fail(validate.Issue{Code: validate.CodeViewName, Field: "name", Expected: "a name", Fix: "give the view a name"})
	case strings.ContainsAny(n, "\r\n"):
		return "", validate.Fail(validate.Issue{Code: validate.CodeViewName, Field: "name", Expected: "one line", Received: n, Fix: "remove the line break"})
	case utf8.RuneCountInString(n) > 80:
		return "", validate.Fail(validate.Issue{Code: validate.CodeViewName, Field: "name", Expected: "at most 80 characters", Fix: "shorten the name"})
	}
	return n, nil
}

// Validate checks a query for a kind. Every problem is an issue with a stable code from
// internal/validate: unknown kind, field or operator, a malformed value, a bad sort or group. The
// result is nil or a *validate.Error (a model.ErrValidation, exit code 2).
func Validate(kind string, q Query, env Env) error {
	var res validate.Result
	add := func(i validate.Issue) { res.Issues = append(res.Issues, i) }
	d, ok := catalog[kind]
	if !ok {
		add(validate.Issue{Code: validate.CodeViewKind, Field: "kind", Expected: "one of " + strings.Join(Kinds, ", "), Received: kind, Fix: "pick a kind"})
		return res.Err()
	}
	if len(q.Filter) > MaxConditions {
		add(validate.Issue{Code: validate.CodeQueryTooLarge, Field: "filter", Expected: fmt.Sprintf("at most %d conditions", MaxConditions), Received: strconv.Itoa(len(q.Filter)), Fix: "remove conditions"})
	}
	if len(q.Columns) > MaxColumns {
		add(validate.Issue{Code: validate.CodeQueryTooLarge, Field: "columns", Expected: fmt.Sprintf("at most %d columns", MaxColumns), Received: strconv.Itoa(len(q.Columns)), Fix: "remove columns"})
	}
	for i, c := range q.Filter {
		if i >= MaxConditions {
			break
		}
		for _, is := range validateCondition(kind, d, c, env) {
			add(is)
		}
	}
	if len(q.Sort) > MaxSorts {
		add(validate.Issue{Code: validate.CodeQuerySort, Field: "sort", Expected: fmt.Sprintf("at most %d sort fields", MaxSorts), Received: strconv.Itoa(len(q.Sort)), Fix: "keep one or two"})
	}
	for _, s := range q.Sort {
		f, ok := FieldOf(kind, s.Field)
		switch {
		case !ok:
			add(validate.Issue{Code: validate.CodeQueryField, Field: s.Field, Expected: "a field of " + kind + ": " + strings.Join(FieldKeys(kind), ", "), Received: s.Field, Fix: "check the field name"})
		case !f.Sortable():
			add(validate.Issue{Code: validate.CodeQuerySort, Field: s.Field, Expected: "a field that holds one value", Received: s.Field, Fix: "sort by a text, date, choice or number field"})
		}
		if s.Dir != "" && !strings.EqualFold(s.Dir, "asc") && !strings.EqualFold(s.Dir, "desc") {
			add(validate.Issue{Code: validate.CodeQuerySort, Field: s.Field, Expected: "asc or desc", Received: s.Dir, Fix: "use asc or desc"})
		}
	}
	if q.GroupBy != "" {
		f, ok := FieldOf(kind, q.GroupBy)
		switch {
		case !ok:
			add(validate.Issue{Code: validate.CodeQueryField, Field: q.GroupBy, Expected: "a field of " + kind + ": " + strings.Join(FieldKeys(kind), ", "), Received: q.GroupBy, Fix: "check the field name"})
		case !f.Groupable():
			add(validate.Issue{Code: validate.CodeQueryGroup, Field: q.GroupBy, Expected: "a field that can be grouped", Received: q.GroupBy, Fix: "group by a text, date, choice, number, relation or tag field"})
		}
	}
	switch q.Bucket {
	case "", BucketDay, BucketWeek, BucketMonth:
	default:
		add(validate.Issue{Code: validate.CodeQueryGroup, Field: "bucket", Expected: "day, week or month", Received: q.Bucket, Fix: "use day, week or month"})
	}
	for _, c := range q.Columns {
		if _, ok := FieldOf(kind, c); !ok {
			add(validate.Issue{Code: validate.CodeQueryField, Field: c, Expected: "a field of " + kind + ": " + strings.Join(FieldKeys(kind), ", "), Received: c, Fix: "check the column name"})
		}
	}
	return res.Err()
}

func validateCondition(kind string, _ *kindDef, c Condition, env Env) []validate.Issue {
	f, ok := FieldOf(kind, c.Field)
	if !ok {
		return []validate.Issue{{Code: validate.CodeQueryField, Field: c.Field, Expected: "a field of " + kind + ": " + strings.Join(FieldKeys(kind), ", "), Received: c.Field, Fix: "check the field name"}}
	}
	allowed := operatorsByClass[f.Class]
	okOp := false
	for _, op := range allowed {
		if op == c.Op {
			okOp = true
		}
	}
	if f.Birthday && (c.Op == OpBefore || c.Op == OpAfter) {
		okOp = false
		allowed = []string{OpWithinLast, OpInNext, OpEmpty, OpNotEmpty}
	}
	if !okOp {
		return []validate.Issue{{Code: validate.CodeQueryOperator, Field: c.Field, Expected: "one of " + strings.Join(allowed, ", ") + " for " + string(f.Class) + " field " + c.Field, Received: c.Op, Fix: "pick one of the listed operators"}}
	}
	vals := c.Values
	bad := func(expected, received, fix string) []validate.Issue {
		return []validate.Issue{{Code: validate.CodeQueryValue, Field: c.Field, Expected: expected, Received: received, Fix: fix}}
	}
	if len(vals) > MaxValues {
		return []validate.Issue{{Code: validate.CodeQueryTooLarge, Field: c.Field, Expected: fmt.Sprintf("at most %d values", MaxValues), Received: strconv.Itoa(len(vals)), Fix: "remove values"}}
	}
	for _, v := range vals {
		if utf8.RuneCountInString(v) > MaxValueLen {
			return []validate.Issue{{Code: validate.CodeQueryTooLarge, Field: c.Field, Expected: fmt.Sprintf("values of at most %d characters", MaxValueLen), Fix: "shorten the value"}}
		}
	}
	switch c.Op {
	case OpEmpty, OpNotEmpty, OpHasAny:
		if len(vals) != 0 {
			return bad("no value for "+c.Op, strings.Join(vals, ", "), "remove the value")
		}
		return nil
	case OpWithinLast, OpInNext:
		if len(vals) != 1 {
			return bad("one count of days", strconv.Itoa(len(vals))+" values", "give a number of days such as 30")
		}
		if _, err := ParseDays(vals[0]); err != nil {
			return bad("a number of days", vals[0], err.Error())
		}
		return nil
	case OpBetween:
		if len(vals) != 2 {
			return bad("two numbers", strconv.Itoa(len(vals))+" values", "give a low and a high value")
		}
		for _, v := range vals {
			if _, err := ParseNumber(v); err != nil {
				return bad("a number", v, err.Error())
			}
		}
		return nil
	}
	if len(vals) == 0 {
		return bad("at least one value", "none", "give a value")
	}
	for _, v := range vals {
		if strings.TrimSpace(v) == "" {
			return bad("a value that is not empty", "empty", "remove the empty value")
		}
		switch f.Class {
		case ClassDate:
			if _, err := ParseDate(v, env.now()); err != nil {
				return []validate.Issue{{Code: validate.CodeQueryValue, Field: c.Field, Expected: "a date or a relative date", Received: v, Fix: err.Error()}}
			}
		case ClassNumber:
			if _, err := ParseNumber(v); err != nil {
				return bad("a number", v, err.Error())
			}
		case ClassChoice:
			if len(f.Choices) > 0 {
				found := false
				for _, ch := range f.Choices {
					if strings.EqualFold(ch.Value, strings.TrimSpace(v)) {
						found = true
					}
				}
				if !found {
					return []validate.Issue{{Code: validate.CodeChoiceInvalid, Field: c.Field, Expected: "one of " + f.choiceValues(), Received: v, Fix: "pick one of the options"}}
				}
			}
		}
	}
	return nil
}

func (f Field) choiceValues() string {
	var out []string
	for _, c := range f.Choices {
		out = append(out, c.Value)
	}
	return strings.Join(out, ", ")
}

// ParseNumber reads a number the way a number field does: a leading $ and thousands separators are ignored.
func ParseNumber(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%q is not a number", raw)
	}
	return f, nil
}
