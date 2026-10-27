package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeLogToolsUndoAndRedoBothKinds(t *testing.T) {
	s := setupTestServer(t)
	var a, b, c, d, out map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &a)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Janet"}, &b)
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Blue Origin"}, &c)
	callJSON(t, s, "crm_org_create", map[string]any{"name": "BLUE ORIGIN", "allow_duplicate": true}, &d)

	var pm, om map[string]any
	callJSON(t, s, "crm_person_merge", map[string]any{"keep_id": a["id"], "drop_ids": []any{b["id"]}}, &pm)
	callJSON(t, s, "crm_org_merge", map[string]any{"keep_id": c["id"], "drop_ids": []any{d["id"]}}, &om)
	require.NotZero(t, pm["log_id"])
	require.NotZero(t, om["log_id"])

	callJSON(t, s, "crm_merge_log", map[string]any{}, &out)
	assert.EqualValues(t, 2, out["count"])
	callJSON(t, s, "crm_merge_log", map[string]any{"kind": "organization"}, &out)
	assert.EqualValues(t, 1, out["count"])

	for _, res := range []map[string]any{pm, om} {
		text, isErr := call(t, s, "crm_merge_undo", map[string]any{"id": res["log_id"]})
		require.False(t, isErr, text)
		_, isErr = call(t, s, "crm_merge_undo", map[string]any{"id": res["log_id"]})
		assert.True(t, isErr, "undoing twice is refused")
	}
	var people []map[string]any
	callJSON(t, s, "crm_person_search", map[string]any{"query": "Janet"}, &people)
	assert.Len(t, people, 1)

	for _, res := range []map[string]any{pm, om} {
		text, isErr := call(t, s, "crm_merge_redo", map[string]any{"id": res["log_id"]})
		require.False(t, isErr, text)
	}
	callJSON(t, s, "crm_person_search", map[string]any{"query": "Janet"}, &people)
	assert.Empty(t, people)
	_, isErr := call(t, s, "crm_merge_undo", map[string]any{"id": 999})
	assert.True(t, isErr)
}

func TestOrgDupesDismissTool(t *testing.T) {
	s := setupTestServer(t)
	var a, b, out map[string]any
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Blue Origin"}, &a)
	callJSON(t, s, "crm_org_create", map[string]any{"name": "BLUE ORIGIN", "allow_duplicate": true}, &b)
	callJSON(t, s, "crm_org_dupes", map[string]any{}, &out)
	assert.EqualValues(t, 1, out["count"])

	text, isErr := call(t, s, "crm_org_dupes_dismiss", map[string]any{"org_a": b["id"], "org_b": a["id"]})
	require.False(t, isErr, text)
	callJSON(t, s, "crm_org_dupes", map[string]any{}, &out)
	assert.EqualValues(t, 0, out["count"])

	_, isErr = call(t, s, "crm_org_dupes_dismiss", map[string]any{"org_a": a["id"], "org_b": b["id"], "undo": true})
	require.False(t, isErr)
	callJSON(t, s, "crm_org_dupes", map[string]any{}, &out)
	assert.EqualValues(t, 1, out["count"])
}
