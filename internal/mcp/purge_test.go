package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPPurgeArchived(t *testing.T) {
	s := setupTestServer(t)
	assert.Contains(t, toolNames(s.ListTools()), "crm_purge_archived")
	var p map[string]any
	for _, n := range []string{"Ann", "Bea"} {
		callJSON(t, s, "crm_person_create", map[string]any{"first_name": n}, &p)
		_, isErr := call(t, s, "crm_person_archive", map[string]any{"id": p["id"]})
		assert.False(t, isErr)
	}
	var res map[string]any
	callJSON(t, s, "crm_purge_archived", map[string]any{}, &res)
	assert.Equal(t, false, res["applied"], "dry run by default")
	assert.EqualValues(t, 2, res["count"])
	callJSON(t, s, "crm_purge_archived", map[string]any{"ids": []any{1}, "apply": true}, &res)
	assert.EqualValues(t, 1, res["purged"])
	callJSON(t, s, "crm_purge_archived", map[string]any{"apply": true}, &res)
	assert.EqualValues(t, 1, res["purged"])
}

// Purge is a verb of every archivable type (owner decision 2026-10-28).
func TestMCPPurgeArchivedEveryType(t *testing.T) {
	s := setupTestServer(t)
	var org, deal, task, person map[string]any
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme"}, &org)
	callJSON(t, s, "crm_deal_create", map[string]any{"title": "Site"}, &deal)
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Send deck"}, &task)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann"}, &person)
	for tool, obj := range map[string]map[string]any{"crm_org_delete": org, "crm_deal_delete": deal, "crm_task_delete": task, "crm_person_archive": person} {
		_, isErr := call(t, s, tool, map[string]any{"id": obj["id"]})
		require.False(t, isErr, tool)
	}
	var res map[string]any
	callJSON(t, s, "crm_purge_archived", map[string]any{}, &res)
	assert.Equal(t, false, res["applied"], "dry run by default")
	assert.EqualValues(t, 4, res["count"])
	kinds := map[string]bool{}
	for _, it := range res["items"].([]any) {
		kinds[it.(map[string]any)["kind"].(string)] = true
	}
	assert.Equal(t, map[string]bool{"organization": true, "deal": true, "task": true, "person": true}, kinds)

	callJSON(t, s, "crm_purge_archived", map[string]any{"type": "organization", "ids": []any{org["id"]}, "apply": true}, &res)
	assert.EqualValues(t, 1, res["purged"])
	callJSON(t, s, "crm_purge_archived", map[string]any{"apply": true}, &res)
	assert.EqualValues(t, 3, res["purged"])
	_, isErr := call(t, s, "crm_purge_archived", map[string]any{"type": "widget"})
	assert.True(t, isErr)
}
