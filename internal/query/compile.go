package query

import (
	"fmt"
	"strings"
)

// Stmt is a compiled query: one SELECT with only ? placeholders. Every identifier in SQL comes from the
// field catalog of this package, never from the query, and every value from the query is an argument.
type Stmt struct {
	Kind string
	SQL  string
	Args []any

	query Query
	d     *kindDef
	group *Field
	cols  []Field
	env   Env
}

// likeEscape makes text safe inside a LIKE pattern that uses ESCAPE '\'.
func likeEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

func marks(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

type builder struct {
	args []any
	env  Env
}

func (b *builder) add(v ...any) { b.args = append(b.args, v...) }

func anys(vals []string) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = strings.TrimSpace(v)
	}
	return out
}

func orJoin(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// Compile validates the query and builds its statement.
func Compile(kind string, q Query, env Env) (*Stmt, error) {
	if err := Validate(kind, q, env); err != nil {
		return nil, err
	}
	d := catalog[kind]
	b := &builder{env: env}
	st := &Stmt{Kind: kind, query: q, d: d, env: env}

	sel := []string{"t.id", d.label, d.sub, d.meta}
	grp := "''"
	if q.GroupBy != "" {
		f, _ := FieldOf(kind, q.GroupBy)
		st.group = &f
		grp = groupExpr(f, d, bucketOf(q))
	}
	sel = append(sel, grp)
	for _, c := range q.Columns {
		f, _ := FieldOf(kind, c)
		st.cols = append(st.cols, f)
		sel = append(sel, columnExpr(f, d))
	}

	where := []string{d.live}
	for _, c := range q.Filter {
		f, _ := FieldOf(kind, c.Field)
		where = append(where, b.condition(f, c, d))
	}
	for _, w := range strings.Fields(q.Search) {
		var parts []string
		for _, col := range d.search {
			parts = append(parts, fmt.Sprintf("%s LIKE ? ESCAPE '\\'", col))
			b.add("%" + likeEscape(w) + "%")
		}
		where = append(where, "("+strings.Join(parts, " OR ")+")")
	}

	sorts := q.Sort
	if len(sorts) == 0 {
		sorts = d.def
	}
	var order []string
	for _, s := range sorts {
		f, _ := FieldOf(kind, s.Field)
		order = append(order, b.orderBy(f, s.Desc())...)
	}
	order = append(order, "LOWER("+d.label+")", "t.id")

	st.SQL = "SELECT " + strings.Join(sel, ", ") + " FROM " + d.table + " t WHERE " + strings.Join(where, " AND ") + " ORDER BY " + strings.Join(order, ", ")
	st.Args = b.args
	return st, nil
}

func bucketOf(q Query) string {
	if q.Bucket == "" {
		return BucketDay
	}
	return q.Bucket
}

// scalar is the SQL value of a single-valued field.
func scalar(f Field) string {
	if f.Birthday {
		return "CASE WHEN t.birthday_month IS NULL THEN NULL ELSE printf('%02d-%02d', t.birthday_month, t.birthday_day) END"
	}
	return f.expr
}

func (f Field) relFrom() string {
	return f.rel.table + " r JOIN " + f.rel.far + " f ON f.id = " + f.rel.other + " AND f.archived = 0"
}

func tagSub(entity string) string {
	return "taggings g JOIN tags tg ON tg.id = g.tag_id WHERE g.entity_type = '" + entity + "' AND g.entity_id = t.id AND g.deleted_at IS NULL"
}

func groupExpr(f Field, d *kindDef, bucket string) string {
	switch {
	case f.Birthday:
		return "CASE WHEN t.birthday_month IS NULL THEN '' ELSE printf('%02d', t.birthday_month) END"
	case f.rel != nil:
		return "COALESCE((SELECT " + f.rel.label + " FROM " + f.relFrom() + " WHERE " + f.rel.owner + " = t.id AND " + f.rel.live + " ORDER BY r.id LIMIT 1), '')"
	case f.tagEntity != "":
		return "COALESCE((SELECT MIN(tg.name) FROM " + tagSub(f.tagEntity) + "), '')"
	case f.Class == ClassDate:
		e := f.expr
		switch bucket {
		case BucketWeek:
			return "COALESCE(date(" + e + ", '-' || ((CAST(strftime('%w', " + e + ") AS INTEGER) + 6) % 7) || ' days'), '')"
		case BucketMonth:
			return "COALESCE(strftime('%Y-%m', " + e + "), '')"
		}
		return "COALESCE(date(" + e + "), '')"
	}
	return "COALESCE(CAST(" + f.expr + " AS TEXT), '')"
}

func columnExpr(f Field, d *kindDef) string {
	switch {
	case f.rel != nil:
		return "COALESCE((SELECT group_concat(" + f.rel.label + ", ', ') FROM " + f.relFrom() + " WHERE " + f.rel.owner + " = t.id AND " + f.rel.live + "), '')"
	case f.tagEntity != "":
		return "COALESCE((SELECT group_concat(tg.name, ', ') FROM " + tagSub(f.tagEntity) + "), '')"
	case f.multi != nil:
		return "COALESCE((SELECT group_concat(x." + f.multi.col + ", ', ') FROM " + f.multi.table + " x WHERE x." + f.multi.owner + " = t.id), '')"
	}
	return "COALESCE(CAST(" + scalar(f) + " AS TEXT), '')"
}

func (b *builder) orderBy(f Field, desc bool) []string {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	e := f.expr
	var out []string
	if f.Class == ClassNumber {
		out = append(out, fmt.Sprintf("CASE WHEN %s IS NULL THEN 1 ELSE 0 END", e))
	} else {
		out = append(out, fmt.Sprintf("CASE WHEN %s IS NULL OR TRIM(CAST(%s AS TEXT)) = '' THEN 1 ELSE 0 END", e, e))
	}
	if f.Class == ClassChoice && len(f.Choices) > 0 {
		var sb strings.Builder
		sb.WriteString("CASE " + e)
		for i, c := range f.Choices {
			sb.WriteString(fmt.Sprintf(" WHEN ? THEN %d", i))
			b.add(c.Value)
		}
		sb.WriteString(fmt.Sprintf(" ELSE %d END %s", len(f.Choices), dir))
		return append(out, sb.String())
	}
	return append(out, e+" "+dir)
}

func (b *builder) condition(f Field, c Condition, d *kindDef) string {
	switch f.Class {
	case ClassTag:
		return b.tagCond(f, c)
	case ClassRelation:
		return b.relCond(f, c)
	case ClassDate:
		if f.Birthday {
			return b.birthdayCond(c)
		}
		return b.dateCond(f.expr, c)
	case ClassNumber:
		return b.numberCond(f.expr, c)
	case ClassChoice:
		return b.choiceCond(f.expr, c)
	}
	if f.multi != nil {
		return b.multiCond(f, c)
	}
	return b.textCond(f.expr, c)
}

func (b *builder) textCond(e string, c Condition) string {
	switch c.Op {
	case OpEmpty:
		return fmt.Sprintf("(%s IS NULL OR TRIM(%s) = '')", e, e)
	case OpNotEmpty:
		return fmt.Sprintf("(%s IS NOT NULL AND TRIM(%s) <> '')", e, e)
	case OpIs:
		b.add(anys(c.Values)...)
		return fmt.Sprintf("%s COLLATE NOCASE IN (%s)", e, marks(len(c.Values)))
	case OpIsNot:
		b.add(anys(c.Values)...)
		return fmt.Sprintf("(%s IS NULL OR %s COLLATE NOCASE NOT IN (%s))", e, e, marks(len(c.Values)))
	}
	// contains
	var parts []string
	for _, v := range c.Values {
		parts = append(parts, fmt.Sprintf("%s LIKE ? ESCAPE '\\'", e))
		b.add("%" + likeEscape(strings.TrimSpace(v)) + "%")
	}
	return orJoin(parts)
}

func (b *builder) multiCond(f Field, c Condition) string {
	m := f.multi
	base := fmt.Sprintf("SELECT 1 FROM %s x WHERE x.%s = t.id", m.table, m.owner)
	col := "x." + m.col
	switch c.Op {
	case OpEmpty:
		return fmt.Sprintf("NOT EXISTS (%s AND TRIM(%s) <> '')", base, col)
	case OpNotEmpty:
		return fmt.Sprintf("EXISTS (%s AND TRIM(%s) <> '')", base, col)
	case OpIs:
		b.add(anys(c.Values)...)
		return fmt.Sprintf("EXISTS (%s AND %s COLLATE NOCASE IN (%s))", base, col, marks(len(c.Values)))
	case OpIsNot:
		b.add(anys(c.Values)...)
		return fmt.Sprintf("NOT EXISTS (%s AND %s COLLATE NOCASE IN (%s))", base, col, marks(len(c.Values)))
	}
	var parts []string
	for _, v := range c.Values {
		parts = append(parts, fmt.Sprintf("%s LIKE ? ESCAPE '\\'", col))
		b.add("%" + likeEscape(strings.TrimSpace(v)) + "%")
	}
	return fmt.Sprintf("EXISTS (%s AND %s)", base, orJoin(parts))
}

func (b *builder) choiceCond(e string, c Condition) string {
	b.add(anys(c.Values)...)
	if c.Op == OpIsNot {
		return fmt.Sprintf("(%s IS NULL OR %s COLLATE NOCASE NOT IN (%s))", e, e, marks(len(c.Values)))
	}
	return fmt.Sprintf("%s COLLATE NOCASE IN (%s)", e, marks(len(c.Values)))
}

func (b *builder) numberCond(e string, c Condition) string {
	nums := make([]any, 0, len(c.Values))
	for _, v := range c.Values {
		n, _ := ParseNumber(v)
		nums = append(nums, n)
	}
	switch c.Op {
	case OpBetween:
		lo, hi := nums[0].(float64), nums[1].(float64)
		if lo > hi {
			lo, hi = hi, lo
		}
		b.add(lo, hi)
		return fmt.Sprintf("%s BETWEEN ? AND ?", e)
	case OpLt, OpGt:
		var parts []string
		for _, n := range nums {
			parts = append(parts, fmt.Sprintf("%s %s ?", e, c.Op))
			b.add(n)
		}
		return orJoin(parts)
	}
	b.add(nums...)
	return fmt.Sprintf("%s IN (%s)", e, marks(len(nums)))
}

func (b *builder) dateCond(e string, c Condition) string {
	de := "date(" + e + ")"
	now := b.env.now()
	today := dayStart(now)
	switch c.Op {
	case OpEmpty:
		return fmt.Sprintf("(%s IS NULL OR TRIM(%s) = '')", e, e)
	case OpNotEmpty:
		return fmt.Sprintf("(%s IS NOT NULL AND TRIM(%s) <> '')", e, e)
	case OpWithinLast:
		n, _ := ParseDays(c.Values[0])
		b.add(iso(today.AddDate(0, 0, -n)), iso(today))
		return fmt.Sprintf("%s BETWEEN ? AND ?", de)
	case OpInNext:
		n, _ := ParseDays(c.Values[0])
		b.add(iso(today), iso(today.AddDate(0, 0, n)))
		return fmt.Sprintf("%s BETWEEN ? AND ?", de)
	}
	op := "<"
	if c.Op == OpAfter {
		op = ">"
	}
	var parts []string
	for _, v := range c.Values {
		t, _ := ParseDate(v, now)
		parts = append(parts, fmt.Sprintf("%s %s ?", de, op))
		b.add(iso(t))
	}
	return orJoin(parts)
}

// birthdayCond compares the month and day of a yearless birthday. The days in the window become a
// list of month*100+day numbers, so a birthday in a leap year or any year matches.
func (b *builder) birthdayCond(c Condition) string {
	md := "(t.birthday_month * 100 + t.birthday_day)"
	today := dayStart(b.env.now())
	switch c.Op {
	case OpEmpty:
		return "(t.birthday_month IS NULL OR t.birthday_day IS NULL)"
	case OpNotEmpty:
		return "(t.birthday_month IS NOT NULL AND t.birthday_day IS NOT NULL)"
	case OpWithinLast, OpInNext:
		n, _ := ParseDays(c.Values[0])
		from, to := today, today.AddDate(0, 0, n)
		if c.Op == OpWithinLast {
			from, to = today.AddDate(0, 0, -n), today
		}
		days := monthDays(from, to)
		seen := map[int]bool{}
		var uniq []any
		for _, d := range days {
			if !seen[d] {
				seen[d] = true
				uniq = append(uniq, d)
			}
		}
		b.add(uniq...)
		return fmt.Sprintf("%s IN (%s)", md, marks(len(uniq)))
	}
	return "0"
}

func (b *builder) relCond(f Field, c Condition) string {
	r := f.rel
	exists := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = t.id AND %s", f.relFrom(), r.owner, r.live)
	switch c.Op {
	case OpHasAny:
		return "EXISTS (" + exists + ")"
	}
	b.add(anys(c.Values)...)
	in := fmt.Sprintf("%s AND f.uuid IN (%s)", exists, marks(len(c.Values)))
	if c.Op == OpNotLinkedTo {
		return "NOT EXISTS (" + in + ")"
	}
	return "EXISTS (" + in + ")"
}

// tagCond matches a tag by name, ignoring case. A tag also matches its children: has work holds for
// an object tagged work/clients (patterns/nested-tags).
func (b *builder) tagCond(f Field, c Condition) string {
	var parts []string
	for _, v := range c.Values {
		name := strings.TrimSpace(v)
		parts = append(parts, "(tg.name COLLATE NOCASE = ? OR tg.name LIKE ? ESCAPE '\\')")
		b.add(name, likeEscape(name)+"/%")
	}
	sub := "EXISTS (SELECT 1 FROM " + tagSub(f.tagEntity) + " AND " + orJoin(parts) + ")"
	if c.Op == OpNotHas {
		return "NOT " + sub
	}
	return sub
}
