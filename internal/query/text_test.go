package query_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWhere(t *testing.T) {
	tests := []struct {
		in   string
		want query.Condition
	}{
		{"last_contacted before 90 days ago", cond("last_contacted", "before", "90 days ago")},
		{"tag has client|lead", cond("tag", "has", "client", "lead")},
		{"tag has  client | lead ", cond("tag", "has", "client", "lead")},
		{"value between 1000|5000", cond("value", "between", "1000", "5000")},
		{"company is not Acme", cond("company", "is_not", "Acme")},
		{"company is_not Acme", cond("company", "is_not", "Acme")},
		{"company != Acme", cond("company", "is_not", "Acme")},
		{"company not empty", cond("company", "not_empty")},
		{"company empty", cond("company", "empty")},
		{"value > 10", cond("value", ">", "10")},
		{"due within last 7", cond("due", "within_last", "7")},
		{"due in next 7", cond("due", "in_next", "7")},
		{"organization linked to Acme Corp", cond("organization", "linked_to", "Acme Corp")},
		{"organization has any", cond("organization", "has_any")},
		{"tag does not have x", cond("tag", "does_not_have", "x")},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := query.ParseWhere(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want.Field, got.Field)
			assert.Equal(t, tt.want.Op, got.Op)
			assert.Equal(t, tt.want.Values, got.Values)
		})
	}
	for _, bad := range []string{"", "company", "  "} {
		_, err := query.ParseWhere(bad)
		assert.Error(t, err, bad)
	}
}

func TestParseSort(t *testing.T) {
	s, err := query.ParseSort("name:DESC")
	require.NoError(t, err)
	assert.Equal(t, query.Sort{Field: "name", Dir: "desc"}, s)
	s, err = query.ParseSort("due")
	require.NoError(t, err)
	assert.Equal(t, query.Sort{Field: "due"}, s)
	for _, bad := range []string{"", ":desc", "name:up"} {
		_, err := query.ParseSort(bad)
		assert.Error(t, err, bad)
	}
}

func TestDescribe(t *testing.T) {
	q := query.Query{
		Filter: []query.Condition{cond("last_contacted", "before", "90 days ago"), cond("tier", "in", "1", "2"), cond("organization", "linked_to", "u1"), cond("value", "between", "1", "2")},
		Search: "ann", Sort: []query.Sort{{Field: "name", Dir: "desc"}, {Field: "company"}}, GroupBy: "tier", Bucket: "week",
	}
	got := query.Describe("person", q, func(far, v string) string {
		if far == "organizations" && v == "u1" {
			return "Initech"
		}
		return ""
	})
	assert.Equal(t, `last contacted before 90 days ago, priority in Top or High, organizations linked to Initech, value between 1 and 2, text contains "ann"`, got)
	assert.Equal(t, "everything", query.Describe("person", query.Query{}, nil))
	assert.Equal(t, "sorted by name desc, then company asc, grouped by tier (week)", query.DescribeOrder(q))
	assert.Equal(t, "", query.DescribeOrder(query.Query{}))
}

func TestViewFileRoundTrip(t *testing.T) {
	f := query.File{Name: "Hot", Kind: "person", Query: query.Query{
		Filter: []query.Condition{cond("tag", "has", "a", "b"), cond("company", "empty")},
		Sort:   []query.Sort{{Field: "name", Dir: "desc"}}, GroupBy: "tier", Columns: []string{"email"}, Search: "x"}}
	text, err := f.Encode()
	require.NoError(t, err)
	got, err := query.DecodeFile([]byte(text))
	require.NoError(t, err)
	assert.Equal(t, "Hot", got.Name)
	assert.Equal(t, "person", got.Kind)
	assert.Equal(t, f.Query, got.Query)
	assert.NotContains(t, text, "id")
	for _, bad := range []string{"name = 'x'", "version = 1\nname = 'x'\nextra = 1", "version = 2\nname='x'\nkind='person'", "=== not toml"} {
		_, err := query.DecodeFile([]byte(bad))
		require.Error(t, err, bad)
		ve, ok := validate.As(err)
		require.True(t, ok)
		assert.Contains(t, ve.First().Code, "E_QUERY")
	}
}

func TestKindsMatchTheObjectRegistry(t *testing.T) {
	assert.Equal(t, query.Kinds, model.SavedViewObjectKinds)
	assert.NotEmpty(t, model.Fields[model.SavedViewKind])
	for in, want := range map[string]string{"org": "organization", "Orgs": "organization", "interaction": "note", "people": "person", "deal": "deal", "widget": "widget"} {
		assert.Equal(t, want, query.NormalizeKind(in), in)
	}
}
