package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDedupeToolsRegistered(t *testing.T) {
	names := toolNames(setupTestServer(t).ListTools())
	for _, n := range []string{"crm_dupes", "crm_person_merge", "crm_dupes_dismiss"} {
		assert.Contains(t, names, n)
	}
}

func TestDupesMergeAndDismiss(t *testing.T) {
	s := setupTestServer(t)
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane", "last_name": "Smith", "email": "jane@x.com", "title": "CEO"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Janet", "last_name": "Smith", "email": "jane@x.com2", "company": "Acme", "title": "CTO"}, &p)
	callJSON(t, s, "crm_person_add_contact_method", map[string]any{"id": 2, "type": "email", "value": "JANE@x.com"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob", "last_name": "Jones"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Robert", "last_name": "Jones"}, &p)

	var groups []map[string]any
	callJSON(t, s, "crm_dupes", map[string]any{}, &groups)
	require.Len(t, groups, 1)
	assert.Equal(t, "high", groups[0]["level"])
	assert.Equal(t, []any{float64(1), float64(2)}, groups[0]["ids"])

	callJSON(t, s, "crm_dupes", map[string]any{"min": "low"}, &groups)
	assert.Len(t, groups, 2)
	text, isErr := call(t, s, "crm_dupes", map[string]any{"min": "nope"})
	assert.True(t, isErr, text)

	// Dismiss and undo.
	text, isErr = call(t, s, "crm_dupes_dismiss", map[string]any{"person_a": 1, "person_b": 2})
	require.False(t, isErr, text)
	callJSON(t, s, "crm_dupes", map[string]any{}, &groups)
	assert.Empty(t, groups)
	text, isErr = call(t, s, "crm_dupes_dismiss", map[string]any{"person_a": 1, "person_b": 2, "undo": true})
	require.False(t, isErr, text)
	callJSON(t, s, "crm_dupes", map[string]any{}, &groups)
	assert.Len(t, groups, 1)

	// Dry run does not change anything.
	var res map[string]any
	callJSON(t, s, "crm_person_merge", map[string]any{"keep_id": 1, "drop_ids": []any{2}, "choices": map[string]any{"title": 2}, "dry_run": true}, &res)
	assert.Equal(t, true, res["dry_run"])
	callJSON(t, s, "crm_person_get", map[string]any{"id": 2}, &p)
	assert.Equal(t, "Janet", p["first_name"])

	callJSON(t, s, "crm_person_merge", map[string]any{"keep_id": 1, "drop_ids": []any{2}, "choices": map[string]any{"title": 2}}, &res)
	assert.Equal(t, false, res["dry_run"])
	callJSON(t, s, "crm_person_get", map[string]any{"id": 1}, &p)
	assert.Equal(t, "CTO", p["title"])
	assert.Equal(t, "Acme", p["company"])
	_, isErr = call(t, s, "crm_person_get", map[string]any{"id": 2})
	assert.True(t, isErr, "dropped person is archived")

	// Bad input comes back as tool errors.
	for name, args := range map[string]map[string]any{
		"no drops":      {"keep_id": 1},
		"bad drop":      {"keep_id": 1, "drop_ids": []any{"x"}},
		"bad choice":    {"keep_id": 3, "drop_ids": []any{4}, "choices": map[string]any{"title": "x"}},
		"unknown field": {"keep_id": 3, "drop_ids": []any{4}, "choices": map[string]any{"shoe": 3}},
		"missing":       {"keep_id": 3, "drop_ids": []any{99}},
	} {
		text, isErr := call(t, s, "crm_person_merge", args)
		assert.True(t, isErr, "%s: %s", name, text)
	}
}
