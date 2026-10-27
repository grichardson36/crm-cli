package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Owner decisions 2026-10-27, verbs and undo branch (X14, X15, X8).

func TestPersonDeleteIsTheVerbAndArchiveIsAnAlias(t *testing.T) {
	cases := []struct {
		name, tool string
	}{
		{"delete is the verb", "crm_person_delete"},
		{"archive stays as a deprecated alias", "crm_person_archive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := setupTestServer(t)
			var p map[string]any
			callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &p)
			text, isErr := call(t, s, tc.tool, map[string]any{"id": p["id"]})
			require.False(t, isErr, text)
			var list []map[string]any
			callJSON(t, s, "crm_person_search", map[string]any{"query": "Jane"}, &list)
			assert.Empty(t, list)
			_, isErr = call(t, s, "crm_person_restore", map[string]any{"id": p["id"]})
			assert.False(t, isErr)
		})
	}
	tools := setupTestServer(t).ListTools()
	assert.Contains(t, tools["crm_person_archive"].Tool.Description, "Deprecated alias of crm_person_delete")
	assert.NotContains(t, tools["crm_person_delete"].Tool.Description, "Deprecated")
}

func TestTaskDeleteHasNoRestoreOption(t *testing.T) {
	tools := setupTestServer(t).ListTools()
	require.Contains(t, tools, "crm_task_delete")
	require.Contains(t, tools, "crm_task_restore")
	assert.NotContains(t, tools["crm_task_delete"].Tool.InputSchema.Properties, "restore")
}

func TestPersonRelateStaysAsAliasWithPointer(t *testing.T) {
	s := setupTestServer(t)
	var a, b, out map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann"}, &a)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob"}, &b)
	callJSON(t, s, "crm_person_relate", map[string]any{"person_id": a["id"], "related_person_id": b["id"], "type": "friend"}, &out)
	assert.Contains(t, out["note"], "crm_person_link")
	assert.Contains(t, setupTestServer(t).ListTools()["crm_person_relate"].Tool.Description, "Deprecated")
}
