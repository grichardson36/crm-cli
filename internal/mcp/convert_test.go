package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgConvertTools(t *testing.T) {
	s := setupTestServer(t)
	assert.Contains(t, toolNames(s.ListTools()), "crm_org_convert_list")
	assert.Contains(t, toolNames(s.ListTools()), "crm_org_convert_apply")
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann", "last_name": "Lee", "company": "Acme Inc", "title": "CTO"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob", "last_name": "Roe", "company": "ACME"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Cy", "last_name": "Poe", "company": "Globex"}, &p)
	var org map[string]any
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme"}, &org)

	var out struct {
		Stats  map[string]any   `json:"stats"`
		Line   string           `json:"line"`
		Groups []map[string]any `json:"groups"`
	}
	callJSON(t, s, "crm_org_convert_list", map[string]any{}, &out)
	assert.Equal(t, "3 people with company text, 2 groups, 1 matched", out.Line)
	require.Len(t, out.Groups, 2)
	assert.Equal(t, "acme", out.Groups[0]["key"])

	// Nothing to apply without a decision.
	_, isErr := call(t, s, "crm_org_convert_apply", map[string]any{})
	assert.True(t, isErr)
	// Preview writes nothing.
	var res map[string]any
	callJSON(t, s, "crm_org_convert_apply", map[string]any{"accept_exact": true, "dry_run": true}, &res)
	assert.Equal(t, false, res["applied"])
	callJSON(t, s, "crm_org_convert_list", map[string]any{}, &out)
	assert.Len(t, out.Groups, 2)

	// Apply: bulk accept plus an explicit create under a chosen name.
	callJSON(t, s, "crm_org_convert_apply", map[string]any{
		"accept_exact": true,
		"decisions":    []any{map[string]any{"key": "globex", "action": "create", "name": "Globex Corp"}},
	}, &res)
	assert.Equal(t, true, res["applied"])
	result := res["result"].(map[string]any)
	assert.EqualValues(t, 2, result["groups_done"])
	assert.EqualValues(t, 3, result["people_done"])
	callJSON(t, s, "crm_org_convert_list", map[string]any{}, &out)
	assert.Empty(t, out.Groups)

	var links []map[string]any
	callJSON(t, s, "crm_links", map[string]any{"kind": "org", "id": org["id"]}, &links)
	require.Len(t, links, 2)
	roles := map[string]bool{}
	for _, l := range links {
		if r, ok := l["role"].(string); ok {
			roles[r] = true
		}
	}
	assert.True(t, roles["CTO"], "the title became the role")

	_, isErr = call(t, s, "crm_org_convert_apply", map[string]any{"decisions": []any{map[string]any{"key": "x", "action": "merge"}}})
	assert.True(t, isErr)
}
