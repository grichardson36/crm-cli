package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
