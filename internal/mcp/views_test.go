package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPViewTools(t *testing.T) {
	s := setupTestServer(t)
	tools := toolNames(s.ListTools())
	for _, n := range []string{"crm_view_list", "crm_view_show", "crm_view_run", "crm_view_save", "crm_view_edit", "crm_view_rename",
		"crm_view_delete", "crm_view_restore", "crm_view_move", "crm_view_export", "crm_view_import", "crm_view_fields"} {
		assert.Contains(t, tools, n)
	}
	c := func(name string, args map[string]any) (string, bool) { return call(t, s, name, args) }
	var rows []map[string]any
	callJSON(t, s, "crm_view_list", map[string]any{}, &rows)
	require.Len(t, rows, 3)
	assert.Equal(t, "Overdue follow-up", rows[0]["name"])
	assert.Equal(t, true, rows[0]["example"])
	assert.Equal(t, false, rows[0]["tabs"])

	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann", "last_name": "Alder", "company": "Initech"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob", "last_name": "Brown", "company": "Globex"}, &p)

	t.Run("save and run", func(t *testing.T) {
		var v map[string]any
		callJSON(t, s, "crm_view_save", map[string]any{"name": "Initech people", "kind": "person",
			"where": []string{"company is Initech"}, "sort": []string{"name:desc"}, "group_by": "company", "columns": []string{"company"}, "tabs": true}, &v)
		assert.Equal(t, "Initech people", v["name"])
		assert.Equal(t, true, v["tabs"])
		assert.Equal(t, "company is Initech", v["filter"])
		var run struct {
			Count int
			Rows  []map[string]any
		}
		callJSON(t, s, "crm_view_run", map[string]any{"view": "initech people"}, &run)
		require.Equal(t, 1, run.Count)
		assert.Equal(t, "Ann Alder", run.Rows[0]["name"])
		assert.Equal(t, "Initech", run.Rows[0]["group"])
		assert.Equal(t, "Initech", run.Rows[0]["company"])
	})
	t.Run("the whole query as JSON", func(t *testing.T) {
		var v map[string]any
		callJSON(t, s, "crm_view_save", map[string]any{"name": "By JSON", "kind": "person",
			"query": `{"filter":[{"field":"company","op":"contains","values":["glob"]}]}`}, &v)
		assert.Equal(t, "company contains glob", v["filter"])
		var run struct{ Rows []map[string]any }
		callJSON(t, s, "crm_view_run", map[string]any{"view": "By JSON"}, &run)
		require.Len(t, run.Rows, 1)
		assert.Equal(t, "Bob Brown", run.Rows[0]["name"])
	})
	t.Run("edit, rename, move", func(t *testing.T) {
		var v map[string]any
		callJSON(t, s, "crm_view_edit", map[string]any{"view": "Initech people", "where": []string{"company is Globex"}, "tabs": false}, &v)
		assert.Equal(t, "company is Globex", v["filter"])
		assert.Equal(t, false, v["tabs"])
		callJSON(t, s, "crm_view_rename", map[string]any{"view": "Initech people", "name": "Globex people"}, &v)
		assert.Equal(t, "Globex people", v["name"])
		var list []map[string]any
		callJSON(t, s, "crm_view_move", map[string]any{"view": "Globex people", "direction": "up"}, &list)
		assert.Equal(t, "Globex people", list[2]["name"])
		_, isErr := c("crm_view_move", map[string]any{"view": "Globex people", "direction": "sideways"})
		assert.True(t, isErr)
		text, isErr := c("crm_view_edit", map[string]any{"view": "Globex people"})
		assert.True(t, isErr, text)
	})
	t.Run("delete and restore", func(t *testing.T) {
		var d map[string]any
		callJSON(t, s, "crm_view_delete", map[string]any{"view": "By JSON"}, &d)
		assert.Equal(t, true, d["deleted"])
		text, isErr := c("crm_view_run", map[string]any{"view": "By JSON"})
		assert.True(t, isErr, "a deleted view does not run: "+text)
		var deleted []map[string]any
		callJSON(t, s, "crm_view_list", map[string]any{"deleted": true}, &deleted)
		require.Len(t, deleted, 1)
		var v map[string]any
		callJSON(t, s, "crm_view_restore", map[string]any{"view": "By JSON"}, &v)
		assert.Equal(t, false, v["deleted"])
		text, isErr = c("crm_view_restore", map[string]any{"view": "By JSON"})
		assert.True(t, isErr, text)
	})
	t.Run("validation errors name the code", func(t *testing.T) {
		tests := []struct {
			name string
			args map[string]any
			code string
		}{
			{"field", map[string]any{"name": "A", "kind": "person", "where": []string{"nope is 1"}}, "E_QUERY_FIELD_UNKNOWN"},
			{"operator", map[string]any{"name": "A", "kind": "person", "where": []string{"company regex 1"}}, "E_QUERY_OPERATOR_INVALID"},
			{"value", map[string]any{"name": "A", "kind": "deal", "where": []string{"value > x"}}, "E_QUERY_VALUE_INVALID"},
			{"kind", map[string]any{"name": "A", "kind": "widget"}, "E_VIEW_KIND_UNKNOWN"},
			{"name", map[string]any{"name": "", "kind": "person"}, "E_VIEW_NAME_INVALID"},
			{"group", map[string]any{"name": "A", "kind": "person", "group_by": "email"}, "E_QUERY_GROUP_INVALID"},
		}
		for _, tt := range tests {
			text, isErr := c("crm_view_save", tt.args)
			assert.True(t, isErr, tt.name)
			assert.Contains(t, text, tt.code, tt.name)
		}
		text, isErr := c("crm_view_save", map[string]any{"name": "overdue follow-up", "kind": "person"})
		assert.True(t, isErr)
		assert.Contains(t, text, "conflict")
		_, isErr = c("crm_view_show", map[string]any{"view": "nope"})
		assert.True(t, isErr)
	})
	t.Run("export and import", func(t *testing.T) {
		text, isErr := c("crm_view_export", map[string]any{"view": "Overdue follow-up"})
		require.False(t, isErr, text)
		assert.Contains(t, text, `kind = "person"`)
		var v map[string]any
		callJSON(t, s, "crm_view_import", map[string]any{"toml": text, "name": "Overdue copy"}, &v)
		assert.Equal(t, "Overdue copy", v["name"])
		assert.Equal(t, "last contacted before 90 days ago", v["filter"])
		text, isErr = c("crm_view_import", map[string]any{"toml": "version = 1\nname = \"B\"\nkind = \"person\"\nsql = \"x\"\n"})
		assert.True(t, isErr)
		assert.Contains(t, text, "E_QUERY_FIELD_UNKNOWN")
	})
	t.Run("fields", func(t *testing.T) {
		var fields []map[string]any
		callJSON(t, s, "crm_view_fields", map[string]any{"kind": "task"}, &fields)
		byKey := map[string]map[string]any{}
		for _, f := range fields {
			byKey[f["field"].(string)] = f
		}
		assert.Equal(t, "date", byKey["due"]["type"])
		raw, _ := json.Marshal(byKey["due"]["operators"])
		assert.JSONEq(t, `["before","after","within_last","in_next","empty","not_empty"]`, string(raw))
		_, isErr := c("crm_view_fields", map[string]any{"kind": "widget"})
		assert.True(t, isErr)
	})
}
