package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMCPAliasTools(t *testing.T) {
	s := setupTestServer(t)
	for _, n := range []string{"crm_alias_list", "crm_alias_add", "crm_alias_remove"} {
		assert.Contains(t, toolNames(s.ListTools()), n)
	}
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Zachary", "last_name": "Lee"}, &p)
	var found []map[string]any
	callJSON(t, s, "crm_person_search", map[string]any{"query": "Zeke"}, &found)
	assert.Empty(t, found)
	callJSON(t, s, "crm_alias_add", map[string]any{"alias": "zeke", "name": "zachary"}, &p)
	assert.Equal(t, true, p["changed"])
	callJSON(t, s, "crm_person_search", map[string]any{"query": "Zeke"}, &found)
	assert.Len(t, found, 1)
	var list []map[string]any
	callJSON(t, s, "crm_alias_list", map[string]any{}, &list)
	assert.Greater(t, len(list), 50)
	callJSON(t, s, "crm_alias_remove", map[string]any{"alias": "zeke"}, &p)
	assert.Equal(t, true, p["removed"])
}
