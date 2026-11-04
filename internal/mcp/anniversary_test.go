package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-022, S9 on the MCP door: crm_person_link sets the anniversary of a spouse link, crm_links shows
// it, a person merge keeps it and its undo puts the old link back.
func TestPersonLinkSetsAndShowsTheAnniversary(t *testing.T) {
	s := setupTestServer(t)
	var joey, hannah, joe map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Joey", "last_name": "Osgood"}, &joey)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Hannah", "last_name": "Osgood"}, &hannah)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Joe", "last_name": "Osgood"}, &joe)

	var out map[string]any
	callJSON(t, s, "crm_person_link", map[string]any{"id": joey["id"], "other": "person", "other_id": hannah["id"], "kind": "spouse", "anniversary": "2014-06-20"}, &out)
	assert.Equal(t, true, out["created"])

	var links []map[string]any
	callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": hannah["id"]}, &links)
	require.Len(t, links, 1)
	assert.Equal(t, "2014-06-20", links[0]["anniversary"], "the other partner sees the one date")

	t.Run("change and clear", func(t *testing.T) {
		callJSON(t, s, "crm_person_link", map[string]any{"id": joey["id"], "other": "person", "other_id": hannah["id"], "kind": "spouse", "anniversary": "--06-21"}, &out)
		callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": joey["id"]}, &links)
		assert.Equal(t, "--06-21", links[0]["anniversary"])
		callJSON(t, s, "crm_person_link", map[string]any{"id": joey["id"], "other": "person", "other_id": hannah["id"], "kind": "spouse", "anniversary": ""}, &out)
		links = nil // a decode into the old maps would keep their keys
		callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": joey["id"]}, &links)
		assert.NotContains(t, links[0], "anniversary")
		callJSON(t, s, "crm_person_link", map[string]any{"id": joey["id"], "other": "person", "other_id": hannah["id"], "kind": "spouse", "anniversary": "2014-06-20"}, &out)
	})

	t.Run("another kind and a bad date are refused", func(t *testing.T) {
		var cy map[string]any
		callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Cy"}, &cy)
		text, isErr := call(t, s, "crm_person_link", map[string]any{"id": joey["id"], "other": "person", "other_id": cy["id"], "kind": "friend", "anniversary": "2014-06-20"})
		assert.True(t, isErr)
		assert.Contains(t, text, "spouse")
		_, isErr = call(t, s, "crm_person_link", map[string]any{"id": joey["id"], "other": "person", "other_id": hannah["id"], "kind": "spouse", "anniversary": "soon"})
		assert.True(t, isErr)
	})

	t.Run("a merge keeps it and undo restores the old link", func(t *testing.T) {
		var res map[string]any
		callJSON(t, s, "crm_person_merge", map[string]any{"keep_id": joe["id"], "drop_ids": []any{joey["id"]}}, &res)
		callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": joe["id"]}, &links)
		require.Len(t, links, 1)
		assert.Equal(t, "2014-06-20", links[0]["anniversary"])

		_, isErr := call(t, s, "crm_merge_undo", map[string]any{"id": res["log_id"]})
		require.False(t, isErr)
		callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": joey["id"]}, &links)
		require.Len(t, links, 1)
		assert.Equal(t, "2014-06-20", links[0]["anniversary"])
	})
}

func TestModulesToolRefusesTheOldBirthdaysKey(t *testing.T) {
	s := setupTestServer(t)
	_, isErr := call(t, s, "crm_modules_set", map[string]any{"module": "birthdays", "on": false})
	assert.True(t, isErr)
	var rows []map[string]any
	callJSON(t, s, "crm_modules_list", map[string]any{}, &rows)
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r["id"].(string)] = true
	}
	assert.True(t, ids["dates"])
	assert.False(t, ids["birthdays"])
}
