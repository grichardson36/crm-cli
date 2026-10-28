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

func TestTagUnusedAndDuplicatesTools(t *testing.T) {
	s := setupTestServer(t)
	for _, n := range []string{"crm_tag_unused", "crm_tag_duplicates"} {
		assert.Contains(t, toolNames(s.ListTools()), n)
	}
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann"}, &p)
	for _, tag := range []string{"Friend", "friend", "gone"} {
		_, isErr := call(t, s, "crm_tag_apply", map[string]any{"entity_type": "person", "entity_id": p["id"], "tag": tag})
		assert.False(t, isErr)
	}
	_, isErr := call(t, s, "crm_tag_remove", map[string]any{"entity_type": "person", "entity_id": p["id"], "tag": "gone"})
	assert.False(t, isErr)

	var un map[string]any
	callJSON(t, s, "crm_tag_unused", map[string]any{}, &un)
	assert.Equal(t, true, un["dry_run"])
	assert.EqualValues(t, 1, un["count"])
	callJSON(t, s, "crm_tag_unused", map[string]any{"apply": true}, &un)
	assert.Equal(t, false, un["dry_run"])
	callJSON(t, s, "crm_tag_unused", map[string]any{}, &un)
	assert.EqualValues(t, 0, un["count"])

	var dup map[string]any
	callJSON(t, s, "crm_tag_duplicates", map[string]any{}, &dup)
	assert.Equal(t, false, dup["applied"])
	assert.Len(t, dup["groups"], 1)
	callJSON(t, s, "crm_tag_duplicates", map[string]any{"apply": true}, &dup)
	assert.Equal(t, true, dup["applied"])
	callJSON(t, s, "crm_tag_duplicates", map[string]any{}, &dup)
	assert.Empty(t, dup["groups"])
}
