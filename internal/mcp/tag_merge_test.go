package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagRenameAndMergeTools(t *testing.T) {
	s := setupTestServer(t)
	for _, n := range []string{"crm_tag_rename", "crm_tag_merge"} {
		assert.Contains(t, toolNames(s.ListTools()), n)
	}
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann"}, &p)
	_, isErr0 := call(t, s, "crm_tag_apply", map[string]any{"entity_type": "person", "entity_id": p["id"], "tag": "friends"})
	assert.False(t, isErr0)
	_, isErr0 = call(t, s, "crm_tag_apply", map[string]any{"entity_type": "person", "entity_id": 1, "tag": "Friend"})
	assert.False(t, isErr0)

	text, isErr := call(t, s, "crm_tag_rename", map[string]any{"tag": "friends", "new_name": "friend"})
	assert.True(t, isErr)
	assert.Contains(t, text, "conflict")
	assert.Contains(t, text, "merge")

	var res map[string]any
	callJSON(t, s, "crm_tag_merge", map[string]any{"source": "friends", "target": "Friend", "dry_run": true}, &res)
	assert.Equal(t, true, res["dry_run"])
	assert.EqualValues(t, 0, res["total"], "Ann already has the target")
	callJSON(t, s, "crm_tag_merge", map[string]any{"source": "friends", "target": "Friend"}, &res)
	assert.Equal(t, false, res["dry_run"])
	var renamed map[string]any
	callJSON(t, s, "crm_tag_rename", map[string]any{"tag": "Friend", "new_name": "pals"}, &renamed)
	assert.Equal(t, "pals", renamed["name"])
}
