package query_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cond(field, op string, vals ...string) query.Condition {
	return query.Condition{Field: field, Op: op, Values: vals}
}

func where(cs ...query.Condition) query.Query { return query.Query{Filter: cs} }

// Every operator of every field class, with the rows each one keeps. Rows come in the default order of
// the kind (people and organizations by name, deals by title, tasks by due date, notes newest first).
func TestOperators(t *testing.T) {
	d := fixture(t)
	tests := []struct {
		name string
		kind string
		q    query.Query
		want []string
	}{
		// text
		{"text is", "person", where(cond("first", query.OpIs, "ann")), []string{"Ann Alder"}},
		{"text is any of", "person", where(cond("first", query.OpIs, "ann", "BOB")), []string{"Ann Alder", "Bob Brown"}},
		{"text is not", "person", where(cond("company", query.OpIsNot, "Initech")), []string{"Bob Brown", "Cy Cole", "Eve Egan", "Fay Frost"}},
		{"text contains", "person", where(cond("company", query.OpContains, "init")), []string{"Ann Alder", "Dee Dunn"}},
		{"text contains any of", "person", where(cond("company", query.OpContains, "init", "globex")), []string{"Ann Alder", "Bob Brown", "Dee Dunn"}},
		{"text contains percent is literal", "person", where(cond("company", query.OpContains, "100%")), []string{"Eve Egan"}},
		{"text contains underscore is literal", "person", where(cond("company", query.OpContains, "_")), []string{"Eve Egan"}},
		{"text empty", "person", where(cond("company", query.OpEmpty)), []string{"Cy Cole", "Fay Frost"}},
		{"text not empty", "person", where(cond("company", query.OpNotEmpty)), []string{"Ann Alder", "Bob Brown", "Dee Dunn", "Eve Egan"}},
		{"list text is", "person", where(cond("email", query.OpIs, "ANN@X.COM")), []string{"Ann Alder"}},
		{"list text is not", "person", where(cond("email", query.OpIsNot, "ann@x.com")), []string{"Bob Brown", "Cy Cole", "Dee Dunn", "Eve Egan", "Fay Frost"}},
		{"list text contains", "person", where(cond("email", query.OpContains, "globex")), []string{"Bob Brown"}},
		{"list text empty", "person", where(cond("email", query.OpEmpty)), []string{"Cy Cole", "Dee Dunn", "Eve Egan", "Fay Frost"}},
		{"list text not empty", "person", where(cond("email", query.OpNotEmpty)), []string{"Ann Alder", "Bob Brown"}},
		// date, relative to Thursday 2026-10-29
		{"date before absolute", "person", where(cond("last_contacted", query.OpBefore, "2026-09-01")), []string{"Ann Alder", "Dee Dunn"}},
		{"date before relative 90 days ago", "person", where(cond("last_contacted", query.OpBefore, "90 days ago")), []string{"Ann Alder"}},
		{"date before relative 100 days ago", "person", where(cond("last_contacted", query.OpBefore, "-100d")), []string{"Ann Alder"}},
		{"date before today", "person", where(cond("last_contacted", query.OpBefore, "today")), []string{"Ann Alder", "Bob Brown", "Dee Dunn", "Eve Egan"}},
		{"date before any of", "person", where(cond("last_contacted", query.OpBefore, "2026-02-01", "2026-08-15")), []string{"Ann Alder", "Dee Dunn"}},
		{"date after", "person", where(cond("last_contacted", query.OpAfter, "2026-10-01")), []string{"Bob Brown", "Eve Egan"}},
		{"date after relative", "person", where(cond("last_contacted", query.OpAfter, "2 weeks ago")), []string{"Bob Brown", "Eve Egan"}},
		{"date within last", "person", where(cond("last_contacted", query.OpWithinLast, "30")), []string{"Bob Brown", "Eve Egan"}},
		{"date within last weeks", "person", where(cond("last_contacted", query.OpWithinLast, "1w")), []string{"Eve Egan"}},
		{"date in next", "task", where(cond("due", query.OpInNext, "7")), []string{"Call Ann", "Send quote"}},
		{"date empty", "person", where(cond("last_contacted", query.OpEmpty)), []string{"Cy Cole", "Fay Frost"}},
		{"date not empty", "person", where(cond("last_contacted", query.OpNotEmpty)), []string{"Ann Alder", "Bob Brown", "Dee Dunn", "Eve Egan"}},
		{"birthday in next 30 days", "person", where(cond("birthday", query.OpInNext, "30")), []string{"Ann Alder", "Bob Brown"}},
		{"birthday within last 30 days", "person", where(cond("birthday", query.OpWithinLast, "30")), []string{"Dee Dunn"}},
		{"birthday empty", "person", where(cond("birthday", query.OpEmpty)), []string{"Cy Cole"}},
		{"birthday not empty", "person", where(cond("birthday", query.OpNotEmpty)), []string{"Ann Alder", "Bob Brown", "Dee Dunn", "Eve Egan", "Fay Frost"}},
		// choice
		{"choice is", "person", where(cond("tier", query.OpIs, "1")), []string{"Ann Alder", "Eve Egan"}},
		{"choice is not", "person", where(cond("tier", query.OpIsNot, "1")), []string{"Bob Brown", "Cy Cole", "Dee Dunn", "Fay Frost"}},
		{"choice in", "person", where(cond("tier", query.OpIn, "1", "2")), []string{"Ann Alder", "Bob Brown", "Eve Egan"}},
		{"choice favorite", "person", where(cond("favorite", query.OpIs, "yes")), []string{"Ann Alder"}},
		{"choice stage from the list", "deal", where(cond("stage", query.OpIn, "lead", "won")), []string{"Site build", "Small", "Won deal"}},
		{"choice outcome open", "deal", where(cond("outcome", query.OpIs, "open")), []string{"Big deal", "Site build", "Small"}},
		{"choice status done", "task", where(cond("status", query.OpIs, "done")), []string{"Old thing"}},
		// number
		{"number equals", "deal", where(cond("value", query.OpEq, "5000")), []string{"Site build"}},
		{"number equals any of", "deal", where(cond("value", query.OpEq, "5000", "1000")), []string{"Site build", "Won deal"}},
		{"number less than", "deal", where(cond("value", query.OpLt, "5000")), []string{"Won deal"}},
		{"number greater than with dollar sign", "deal", where(cond("value", query.OpGt, "$5,000")), []string{"Big deal"}},
		{"number between", "deal", where(cond("value", query.OpBetween, "1000", "5000")), []string{"Site build", "Won deal"}},
		{"number between reversed", "deal", where(cond("value", query.OpBetween, "5000", "1000")), []string{"Site build", "Won deal"}},
		{"number used", "tag", where(cond("used", query.OpGt, "1")), []string{"client"}},
		// relation (values are uuids)
		{"relation linked to", "person", where(cond("organization", query.OpLinkedTo, "uuid-Initech")), []string{"Ann Alder", "Dee Dunn"}},
		{"relation linked to any of", "person", where(cond("organization", query.OpLinkedTo, "uuid-Initech", "uuid-Globex")), []string{"Ann Alder", "Bob Brown", "Dee Dunn"}},
		{"relation not linked to", "person", where(cond("organization", query.OpNotLinkedTo, "uuid-Initech")), []string{"Bob Brown", "Cy Cole", "Eve Egan", "Fay Frost"}},
		{"relation has any", "person", where(cond("organization", query.OpHasAny)), []string{"Ann Alder", "Bob Brown", "Dee Dunn"}},
		{"relation of a deal", "deal", where(cond("organization", query.OpLinkedTo, "uuid-Globex")), []string{"Big deal"}},
		{"relation from the other side", "organization", where(cond("member", query.OpLinkedTo, "uuid-Ann")), []string{"Initech"}},
		{"relation of a task", "task", where(cond("person", query.OpLinkedTo, "uuid-Ann")), []string{"Call Ann"}},
		{"relation of a note", "note", where(cond("organization", query.OpLinkedTo, "uuid-Initech")), []string{"Follow up on pricing", "Intro call"}},
		{"relation of a note to a deal", "note", where(cond("deal", query.OpLinkedTo, "uuid-Site build")), []string{"Follow up on pricing"}},
		// tag
		{"tag has", "person", where(cond("tag", query.OpHas, "lead")), []string{"Cy Cole"}},
		{"tag has includes children, ignores case", "person", where(cond("tag", query.OpHas, "CLIENT")), []string{"Ann Alder", "Bob Brown"}},
		{"tag has a child name only", "person", where(cond("tag", query.OpHas, "client/vip")), []string{"Bob Brown"}},
		{"tag has a suffix is not a match", "person", where(cond("tag", query.OpHas, "vip")), []string{}},
		{"tag has any of", "person", where(cond("tag", query.OpHas, "client", "lead")), []string{"Ann Alder", "Bob Brown", "Cy Cole"}},
		{"tag does not have", "person", where(cond("tag", query.OpNotHas, "client")), []string{"Cy Cole", "Dee Dunn", "Eve Egan", "Fay Frost"}},
		{"tag on organizations", "organization", where(cond("tag", query.OpHas, "client")), []string{"Initech"}},
		// notes and tags as kinds
		{"note type", "note", where(cond("type", query.OpIs, "call")), []string{"Intro call"}},
		{"note text", "note", where(cond("text", query.OpContains, "pricing")), []string{"Follow up on pricing"}},
		{"note date", "note", where(cond("date", query.OpWithinLast, "10")), []string{"Follow up on pricing"}},
		{"tag name", "tag", where(cond("name", query.OpContains, "client")), []string{"client", "client/vip"}},
		// AND across conditions
		{"and", "person", where(cond("company", query.OpContains, "init"), cond("tier", query.OpIs, "3")), []string{"Dee Dunn"}},
		{"and with no match", "person", where(cond("company", query.OpContains, "init"), cond("tier", query.OpIs, "2")), []string{}},
		{"open tasks due this week", "task", where(cond("status", query.OpIs, "open"), cond("due", query.OpInNext, "7")), []string{"Call Ann", "Send quote"}},
		// search
		{"search words", "person", query.Query{Search: "init"}, []string{"Ann Alder", "Dee Dunn"}},
		{"search every word", "person", query.Query{Search: "ann alder"}, []string{"Ann Alder"}},
		{"search plus filter", "person", query.Query{Search: "init", Filter: []query.Condition{cond("tier", query.OpIs, "1")}}, []string{"Ann Alder"}},
		{"no conditions lists the live rows", "organization", query.Query{}, []string{"Globex", "Initech", "Umbrella"}},
	}
	used := map[string]bool{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, labels(run(t, d, tt.kind, tt.q)))
		})
		for _, c := range tt.q.Filter {
			used[c.Op] = true
		}
	}
	t.Run("every operator of every class is covered", func(t *testing.T) {
		for _, c := range []query.Class{query.ClassText, query.ClassDate, query.ClassChoice, query.ClassNumber, query.ClassRelation, query.ClassTag} {
			for _, op := range query.OperatorsFor(c) {
				assert.True(t, used[op], "operator %s of %s has no test", op, c)
			}
		}
	})
}

func TestBirthdayWindowCrossesTheYear(t *testing.T) {
	d := fixture(t)
	env := query.Env{Now: time.Date(2026, 12, 20, 9, 0, 0, 0, time.UTC)}
	rows, err := query.Run(context.Background(), d, "person", where(cond("birthday", query.OpInNext, "30")), env)
	require.NoError(t, err)
	assert.Equal(t, []string{"Eve Egan", "Fay Frost"}, labels(rows), "Dec 25 and Jan 5 are in the next 30 days of Dec 20")
}

func TestDeletedAndArchivedRowsAreNotListed(t *testing.T) {
	d := fixture(t)
	exec(t, d, `UPDATE people SET archived = 1 WHERE first_name = 'Ann'`)
	exec(t, d, `UPDATE taggings SET deleted_at = datetime('now') WHERE tag_id = 3`)
	assert.NotContains(t, labels(run(t, d, "person", query.Query{})), "Ann Alder")
	assert.NotContains(t, labels(run(t, d, "tag", query.Query{})), "lead", "a tag with no live tagging is not listed")
	exec(t, d, `UPDATE people SET archived = 1 WHERE first_name = 'Bob'`)
	assert.Equal(t, []string{"Dee Dunn"}, labels(run(t, d, "person", where(cond("organization", query.OpLinkedTo, "uuid-Initech")))))
}

func TestSort(t *testing.T) {
	d := fixture(t)
	tests := []struct {
		name string
		kind string
		sort []query.Sort
		want []string
	}{
		{"one field desc, empty last", "person", []query.Sort{{Field: "last_contacted", Dir: "desc"}}, []string{"Eve Egan", "Bob Brown", "Dee Dunn", "Ann Alder", "Cy Cole", "Fay Frost"}},
		{"one field asc, empty last", "person", []query.Sort{{Field: "last_contacted"}}, []string{"Ann Alder", "Dee Dunn", "Bob Brown", "Eve Egan", "Cy Cole", "Fay Frost"}},
		{"two fields", "person", []query.Sort{{Field: "company", Dir: "asc"}, {Field: "name", Dir: "desc"}}, []string{"Eve Egan", "Bob Brown", "Dee Dunn", "Ann Alder", "Fay Frost", "Cy Cole"}},
		{"number desc, empty last", "deal", []query.Sort{{Field: "value", Dir: "desc"}}, []string{"Big deal", "Site build", "Won deal", "Small"}},
		{"choice sorts in option order", "task", []query.Sort{{Field: "priority", Dir: "desc"}, {Field: "title"}}, []string{"Call Ann", "Next month", "Send quote", "Someday", "Old thing"}},
		{"default order of tasks is due date", "task", nil, []string{"Old thing", "Call Ann", "Send quote", "Next month", "Someday"}},
		{"default order of notes is newest first", "note", nil, []string{"Follow up on pricing", "Intro call"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, labels(run(t, d, tt.kind, query.Query{Sort: tt.sort})))
		})
	}
}

func dayLabel(date string) string {
	tm, _ := time.Parse("2006-01-02", date)
	return date + " " + tm.Format("Mon")
}

func TestGroup(t *testing.T) {
	d := fixture(t)
	type g struct {
		label string
		rows  []string
	}
	grouped := func(rows []query.Row) []g {
		var out []g
		for _, r := range rows {
			if len(out) == 0 || out[len(out)-1].label != r.GroupLabel {
				out = append(out, g{label: r.GroupLabel})
			}
			out[len(out)-1].rows = append(out[len(out)-1].rows, r.Label)
		}
		return out
	}
	tests := []struct {
		name string
		kind string
		q    query.Query
		want []g
	}{
		{"deals by stage in stage order", "deal", query.Query{GroupBy: "stage"},
			[]g{{"lead", []string{"Site build", "Small"}}, {"negotiation", []string{"Big deal"}}, {"won", []string{"Won deal"}}}},
		{"tasks by day", "task", query.Query{GroupBy: "due"},
			[]g{{dayLabel("2026-10-01"), []string{"Old thing"}}, {dayLabel("2026-10-30"), []string{"Call Ann"}}, {dayLabel("2026-11-02"), []string{"Send quote"}},
				{dayLabel("2026-12-15"), []string{"Next month"}}, {"(none)", []string{"Someday"}}}},
		{"tasks by week", "task", query.Query{GroupBy: "due", Bucket: query.BucketWeek},
			[]g{{"Week of 2026-09-28", []string{"Old thing"}}, {"Week of 2026-10-26", []string{"Call Ann"}}, {"Week of 2026-11-02", []string{"Send quote"}},
				{"Week of 2026-12-14", []string{"Next month"}}, {"(none)", []string{"Someday"}}}},
		{"tasks by month", "task", query.Query{GroupBy: "due", Bucket: query.BucketMonth, Sort: []query.Sort{{Field: "due"}}},
			[]g{{"October 2026", []string{"Old thing", "Call Ann"}}, {"November 2026", []string{"Send quote"}}, {"December 2026", []string{"Next month"}}, {"(none)", []string{"Someday"}}}},
		{"people by organization, alphabetical, none last", "person", query.Query{GroupBy: "organization"},
			[]g{{"Globex", []string{"Bob Brown"}}, {"Initech", []string{"Ann Alder", "Dee Dunn"}}, {"(none)", []string{"Cy Cole", "Eve Egan", "Fay Frost"}}}},
		{"people by priority choice", "person", query.Query{GroupBy: "tier"},
			[]g{{"Top", []string{"Ann Alder", "Eve Egan"}}, {"High", []string{"Bob Brown"}}, {"Medium", []string{"Dee Dunn"}}, {"(none)", []string{"Cy Cole", "Fay Frost"}}}},
		{"people by birthday month", "person", query.Query{GroupBy: "birthday"},
			[]g{{"January", []string{"Fay Frost"}}, {"October", []string{"Bob Brown", "Dee Dunn"}}, {"November", []string{"Ann Alder"}}, {"December", []string{"Eve Egan"}}, {"(none)", []string{"Cy Cole"}}}},
		{"people by tag", "person", query.Query{GroupBy: "tag"},
			[]g{{"client", []string{"Ann Alder"}}, {"client/vip", []string{"Bob Brown"}}, {"lead", []string{"Cy Cole"}}, {"(none)", []string{"Dee Dunn", "Eve Egan", "Fay Frost"}}}},
		{"descending sort on the group field reverses the groups", "deal", query.Query{GroupBy: "stage", Sort: []query.Sort{{Field: "stage", Dir: "desc"}}},
			[]g{{"won", []string{"Won deal"}}, {"negotiation", []string{"Big deal"}}, {"lead", []string{"Site build", "Small"}}}},
		{"filter and group together", "deal", query.Query{GroupBy: "stage", Filter: []query.Condition{cond("outcome", query.OpIs, "open")}},
			[]g{{"lead", []string{"Site build", "Small"}}, {"negotiation", []string{"Big deal"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, grouped(run(t, d, tt.kind, tt.q)))
		})
	}
	t.Run("stages follow the order the user gave them", func(t *testing.T) {
		env := query.Env{Now: now, Choices: func(string) []string { return []string{"won", "negotiation", "lead"} }}
		rows, err := query.Run(context.Background(), d, "deal", query.Query{GroupBy: "stage"}, env)
		require.NoError(t, err)
		var order []string
		for _, r := range rows {
			if len(order) == 0 || order[len(order)-1] != r.GroupLabel {
				order = append(order, r.GroupLabel)
			}
		}
		assert.Equal(t, []string{"won", "negotiation", "lead"}, order)
	})
}

func TestColumns(t *testing.T) {
	d := fixture(t)
	rows := run(t, d, "person", query.Query{Columns: []string{"organization", "tag", "email", "birthday", "tier"}, Filter: []query.Condition{cond("first", query.OpIs, "Ann")}})
	require.Len(t, rows, 1)
	assert.Equal(t, []string{"Initech", "client", "ann@x.com", "11-05", "1"}, rows[0].Extra)
}

func TestSQLIsParameterized(t *testing.T) {
	d := fixture(t)
	hostile := []string{
		`'; DROP TABLE people; --`,
		`" OR 1=1 --`,
		`x') OR ('1'='1`,
		`%`,
		`\`,
		"a\x00b",
	}
	for _, h := range hostile {
		for _, c := range []struct{ kind, field, op string }{
			{"person", "first", query.OpIs}, {"person", "first", query.OpIsNot}, {"person", "company", query.OpContains},
			{"person", "email", query.OpContains}, {"person", "tag", query.OpHas}, {"person", "tag", query.OpNotHas},
			{"person", "organization", query.OpLinkedTo}, {"deal", "stage", query.OpIn}, {"note", "text", query.OpContains},
		} {
			q := query.Query{Filter: []query.Condition{cond(c.field, c.op, h)}, Search: h}
			st, err := query.Compile(c.kind, q, testEnv())
			if c.field == "stage" {
				continue // a choice value must be an option or come from a list, checked below
			}
			require.NoError(t, err, "%s %s %q", c.field, c.op, h)
			if len(h) > 2 {
				assert.NotContains(t, st.SQL, h, "the value must travel as an argument")
			}
			assert.Equal(t, strings.Count(st.SQL, "?"), len(st.Args), "one argument per placeholder")
			rows, err := d.QueryContext(context.Background(), st.SQL, st.Args...)
			require.NoError(t, err)
			rows.Close()
		}
	}
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&n))
	assert.Equal(t, 6, n, "the people table is intact")
	// A hostile text is only a text: no row matches it.
	assert.Empty(t, run(t, d, "person", where(cond("first", query.OpIs, `'; DROP TABLE people; --`))))
	assert.Len(t, run(t, d, "person", where(cond("first", query.OpIsNot, `' OR 1=1 --`))), 6)
	assert.Empty(t, run(t, d, "deal", where(cond("stage", query.OpIs, `x'; DROP TABLE deals; --`))), "a stage comes from a list the user manages, any text is only a value")
}

func TestValidationErrors(t *testing.T) {
	many := make([]query.Condition, query.MaxConditions+1)
	for i := range many {
		many[i] = cond("first", query.OpIs, "a")
	}
	tests := []struct {
		name string
		kind string
		q    query.Query
		code string
	}{
		{"unknown kind", "widget", query.Query{}, validate.CodeViewKind},
		{"unknown field", "person", where(cond("shoe_size", query.OpIs, "9")), validate.CodeQueryField},
		{"field of another kind", "task", where(cond("company", query.OpIs, "x")), validate.CodeQueryField},
		{"hostile field name", "person", where(cond("first_name; DROP TABLE people", query.OpIs, "x")), validate.CodeQueryField},
		{"unknown operator", "person", where(cond("first", "like", "x")), validate.CodeQueryOperator},
		{"hostile operator", "person", where(cond("first", "= 1 OR 1=1 --", "x")), validate.CodeQueryOperator},
		{"operator of another class", "person", where(cond("first", query.OpBefore, "2026-01-01")), validate.CodeQueryOperator},
		{"date operator on a text field", "deal", where(cond("title", query.OpWithinLast, "3")), validate.CodeQueryOperator},
		{"number operator on a choice", "person", where(cond("tier", query.OpGt, "1")), validate.CodeQueryOperator},
		{"before on a birthday", "person", where(cond("birthday", query.OpBefore, "2026-01-01")), validate.CodeQueryOperator},
		{"tag operator on relation", "person", where(cond("organization", query.OpHas, "x")), validate.CodeQueryOperator},
		{"value for empty", "person", where(cond("company", query.OpEmpty, "x")), validate.CodeQueryValue},
		{"no value for is", "person", where(cond("company", query.OpIs)), validate.CodeQueryValue},
		{"blank value", "person", where(cond("company", query.OpIs, "  ")), validate.CodeQueryValue},
		{"bad date", "person", where(cond("last_contacted", query.OpBefore, "someday soonish")), validate.CodeQueryValue},
		{"bad calendar date", "person", where(cond("last_contacted", query.OpBefore, "2026-02-31")), validate.CodeQueryValue},
		{"days is not a number", "person", where(cond("last_contacted", query.OpWithinLast, "many")), validate.CodeQueryValue},
		{"two counts of days", "person", where(cond("last_contacted", query.OpWithinLast, "3", "4")), validate.CodeQueryValue},
		{"days out of range", "person", where(cond("last_contacted", query.OpInNext, "999999")), validate.CodeQueryValue},
		{"bad number", "deal", where(cond("value", query.OpEq, "lots")), validate.CodeQueryValue},
		{"infinite number", "deal", where(cond("value", query.OpEq, "Inf")), validate.CodeQueryValue},
		{"between with one value", "deal", where(cond("value", query.OpBetween, "1")), validate.CodeQueryValue},
		{"choice not in the list", "person", where(cond("tier", query.OpIs, "9")), validate.CodeChoiceInvalid},
		{"too many conditions", "person", query.Query{Filter: many}, validate.CodeQueryTooLarge},
		{"sort by unknown field", "person", query.Query{Sort: []query.Sort{{Field: "nope"}}}, validate.CodeQueryField},
		{"sort by a relation", "person", query.Query{Sort: []query.Sort{{Field: "organization"}}}, validate.CodeQuerySort},
		{"three sort fields", "person", query.Query{Sort: []query.Sort{{Field: "name"}, {Field: "title"}, {Field: "company"}}}, validate.CodeQuerySort},
		{"bad direction", "person", query.Query{Sort: []query.Sort{{Field: "name", Dir: "sideways"}}}, validate.CodeQuerySort},
		{"group by unknown field", "person", query.Query{GroupBy: "nope"}, validate.CodeQueryField},
		{"group by a list", "person", query.Query{GroupBy: "email"}, validate.CodeQueryGroup},
		{"bad bucket", "task", query.Query{GroupBy: "due", Bucket: "decade"}, validate.CodeQueryGroup},
		{"unknown column", "person", query.Query{Columns: []string{"nope"}}, validate.CodeQueryField},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := query.Validate(tt.kind, tt.q, testEnv())
			require.Error(t, err)
			ve, ok := validate.As(err)
			require.True(t, ok, "a validation error")
			assert.Equal(t, tt.code, ve.First().Code)
			_, cerr := query.Compile(tt.kind, tt.q, testEnv())
			assert.Error(t, cerr, "an invalid query never compiles")
		})
	}
	t.Run("every query code is documented once", func(t *testing.T) {
		seen := map[string]int{}
		for _, c := range validate.Codes {
			seen[c.Code]++
		}
		for _, code := range []string{validate.CodeViewKind, validate.CodeViewName, validate.CodeQueryField, validate.CodeQueryOperator, validate.CodeQueryValue, validate.CodeQuerySort, validate.CodeQueryGroup, validate.CodeQueryTooLarge} {
			assert.Equal(t, 1, seen[code], code)
		}
	})
}

func TestValidName(t *testing.T) {
	for _, bad := range []string{"", "   ", "two\nlines", strings.Repeat("x", 81)} {
		_, err := query.ValidName(bad)
		require.Error(t, err, "%q", bad)
		ve, _ := validate.As(err)
		assert.Equal(t, validate.CodeViewName, ve.First().Code)
	}
	n, err := query.ValidName("  Hot leads  ")
	require.NoError(t, err)
	assert.Equal(t, "Hot leads", n)
}

func TestRelativeDates(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"today", "2026-10-29"}, {"yesterday", "2026-10-28"}, {"tomorrow", "2026-10-30"},
		{"2026-12-01", "2026-12-01"},
		{"90 days ago", "2026-07-31"}, {"1 day ago", "2026-10-28"}, {"2 weeks ago", "2026-10-15"},
		{"3 months ago", "2026-07-29"}, {"1 year ago", "2025-10-29"},
		{"in 30 days", "2026-11-28"}, {"in 2 weeks", "2026-11-12"},
		{"-90d", "2026-07-31"}, {"+30d", "2026-11-28"}, {"-2w", "2026-10-15"}, {"+3m", "2027-01-29"}, {"-1y", "2025-10-29"},
		{"next friday", "2026-11-06"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := query.ParseDate(tt.in, now)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Format("2006-01-02"))
		})
	}
	for _, bad := range []string{"", "soon", "2026-13-01", "-d", "ago"} {
		_, err := query.ParseDate(bad, now)
		assert.Error(t, err, bad)
	}
	for in, want := range map[string]int{"30": 30, "30d": 30, "4w": 28, "0": 0} {
		n, err := query.ParseDays(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, n, in)
	}
	for _, bad := range []string{"", "x", "-3", "99999"} {
		_, err := query.ParseDays(bad)
		assert.Error(t, err, bad)
	}
}

func TestQueryJSONRoundTrip(t *testing.T) {
	q := query.Query{
		Filter: []query.Condition{cond("tag", query.OpHas, "a", "b"), cond("company", query.OpEmpty)},
		Sort:   []query.Sort{{Field: "name", Dir: "desc"}}, GroupBy: "tier", Bucket: "week", Columns: []string{"email"}, Search: "x",
	}
	got, err := query.Parse(q.JSON())
	require.NoError(t, err)
	assert.Equal(t, q, got)
	empty, err := query.Parse("")
	require.NoError(t, err)
	assert.Equal(t, query.Query{}, empty)
	_, err = query.Parse("{not json")
	assert.Error(t, err)
	c := q.Clone()
	c.Filter[0].Values[0] = "changed"
	assert.Equal(t, "a", q.Filter[0].Values[0], "a clone does not share values")
}

func TestCatalogIsDrivenByTheRegistry(t *testing.T) {
	for _, kind := range query.Kinds {
		fields := query.Fields(kind)
		require.NotEmpty(t, fields, kind)
		assert.True(t, sort.StringsAreSorted(query.FieldKeys(kind)))
		for _, f := range fields {
			assert.NotEmpty(t, query.OperatorsFor(f.Class), "%s.%s has operators", kind, f.Key)
			assert.NotEmpty(t, f.Label, "%s.%s has a label", kind, f.Key)
		}
	}
	// The registry fields of person are all queryable by their registry key.
	for _, key := range []string{"first", "last", "title", "company", "organization", "location", "birthday", "email", "phone", "tier", "notes", "tag"} {
		_, ok := query.FieldOf("person", key)
		assert.True(t, ok, key)
	}
}

func TestLastContactedCountsInteractions(t *testing.T) {
	d := fixture(t)
	// Cy has no manual date. A call about Cy on 2026-10-27 makes Cy contacted two days ago, a note does not count.
	exec(t, d, `INSERT INTO interactions (uuid, type, content, occurred_at) VALUES ('n3', 'call', 'quick call', '2026-10-27 10:00:00'), ('n4', 'note', 'just a thought', '2026-10-28 10:00:00')`)
	exec(t, d, `INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES ('a5', 'interaction', 3, 'person', 3), ('a6', 'interaction', 4, 'person', 6)`)
	got := labels(run(t, d, "person", where(cond("last_contacted", query.OpWithinLast, "3"))))
	assert.Equal(t, []string{"Cy Cole", "Eve Egan"}, got, "a call counts as contact, a plain note does not")
}
