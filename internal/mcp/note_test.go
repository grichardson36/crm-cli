package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoteToolsAreRegistered(t *testing.T) {
	names := toolNames(setupTestServer(t).ListTools())
	for _, n := range []string{"crm_note_create", "crm_note_update", "crm_note_delete", "crm_note_restore", "crm_note_link", "crm_note_unlink",
		"crm_log_update", "crm_log_delete", "crm_interaction_log"} {
		assert.Contains(t, names, n)
	}
}

func TestNoteVerbsAndLinkFromEitherSide(t *testing.T) {
	s := setupTestServer(t)
	var person, org, deal, note map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &person)
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme"}, &org)
	callJSON(t, s, "crm_deal_create", map[string]any{"title": "Site"}, &deal)
	callJSON(t, s, "crm_note_create", map[string]any{"content": "first", "type": "call"}, &note)
	assert.Equal(t, "call", note["type"])

	callJSON(t, s, "crm_note_update", map[string]any{"id": note["id"], "content": "second"}, &note)
	assert.Equal(t, "second", note["content"])

	for _, c := range []struct {
		kind string
		obj  map[string]any
	}{{"person", person}, {"org", org}, {"deal", deal}} {
		var out map[string]any
		callJSON(t, s, "crm_note_link", map[string]any{"id": note["id"], "other": c.kind, "other_id": c.obj["id"]}, &out)
		assert.Equal(t, true, out["changed"], c.kind)
		callJSON(t, s, "crm_"+c.kind+"_link", map[string]any{"id": c.obj["id"], "other": "note", "other_id": note["id"]}, &out)
		assert.Equal(t, false, out["changed"], "%s sees the same link", c.kind)
		callJSON(t, s, "crm_note_unlink", map[string]any{"id": note["id"], "other": c.kind, "other_id": c.obj["id"]}, &out)
	}

	_, isErr := call(t, s, "crm_note_delete", map[string]any{"id": note["id"]})
	require.False(t, isErr)
	_, isErr = call(t, s, "crm_note_delete", map[string]any{"id": note["id"]})
	assert.True(t, isErr, "deleting twice is not found")
	text, isErr := call(t, s, "crm_note_restore", map[string]any{"id": note["id"]})
	require.False(t, isErr, text)
	_, isErr = call(t, s, "crm_note_restore", map[string]any{"id": note["id"]})
	assert.True(t, isErr, "restoring a live note is not found")
}
