package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagTreeTools(t *testing.T) {
	s := setupTestServer(t)
	for _, n := range []string{"crm_tag_tree", "crm_tag_nest", "crm_tag_unnest", "crm_tag_delete"} {
		assert.Contains(t, toolNames(s.ListTools()), n)
	}
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann"}, &p)
	for _, tag := range []string{"work", "work/clients/acme", "home"} {
		_, isErr := call(t, s, "crm_tag_apply", map[string]any{"entity_type": "person", "entity_id": p["id"], "tag": tag})
		require.False(t, isErr)
	}
	_, isErr := call(t, s, "crm_tag_apply", map[string]any{"entity_type": "person", "entity_id": p["id"], "tag": "a//b"})
	assert.True(t, isErr, "the validator refuses an empty segment")

	var nodes []map[string]any
	callJSON(t, s, "crm_tag_tree", map[string]any{}, &nodes)
	var paths []string
	for _, n := range nodes {
		paths = append(paths, n["path"].(string))
	}
	assert.Equal(t, []string{"home", "work", "work/clients", "work/clients/acme"}, paths)
	assert.Equal(t, true, nodes[2]["virtual"])

	var res map[string]any
	callJSON(t, s, "crm_tag_nest", map[string]any{"tags": []string{"home"}, "parent": "area", "dry_run": true}, &res)
	assert.Equal(t, true, res["dry_run"])
	callJSON(t, s, "crm_tag_nest", map[string]any{"tags": []string{"home", "work"}, "parent": "area"}, &res)
	assert.EqualValues(t, 3, res["renamed"])
	text, isErr := call(t, s, "crm_tag_nest", map[string]any{"tags": []string{"area"}, "parent": "area/home"})
	assert.True(t, isErr, text)

	callJSON(t, s, "crm_tag_unnest", map[string]any{"tags": []string{"area/home"}}, &res)
	assert.EqualValues(t, 1, res["renamed"])

	var renamed map[string]any
	callJSON(t, s, "crm_tag_rename", map[string]any{"tag": "area", "new_name": "zone"}, &renamed)
	assert.EqualValues(t, 2, renamed["renamed"], "area itself is virtual: area/work and area/work/clients/acme move")

	var del map[string]any
	callJSON(t, s, "crm_tag_delete", map[string]any{"tag": "zone", "dry_run": true}, &del)
	assert.EqualValues(t, 2, del["tags"])
	callJSON(t, s, "crm_tag_delete", map[string]any{"tag": "zone"}, &del)
	assert.Len(t, del["tagging_ids"], 2)
}
