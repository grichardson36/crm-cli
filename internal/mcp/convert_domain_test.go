package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgConvertToolsDomainSuggestionAndCreateDomain(t *testing.T) {
	s := setupTestServer(t)
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann", "company": "Acme Worldwide", "email": "ann@acme.com"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob", "company": "Acme Worldwide", "email": "bob@acme.com"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Cy", "company": "Globex", "email": "cy@globex.com"}, &p)
	var org map[string]any
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme Holdings", "domain": "acme.com"}, &org)

	var out struct {
		Stats  map[string]any   `json:"stats"`
		Groups []map[string]any `json:"groups"`
	}
	callJSON(t, s, "crm_org_convert_list", map[string]any{}, &out)
	require.Len(t, out.Groups, 2)
	assert.Equal(t, "acme.com", out.Groups[0]["domain"])
	dm := out.Groups[0]["domain_match"].(map[string]any)
	assert.Equal(t, "domain", dm["kind"])
	assert.Nil(t, out.Groups[0]["match"])
	assert.EqualValues(t, 1, out.Stats["domain_matches"])

	// A domain match is not taken by accept_exact.
	_, isErr := call(t, s, "crm_org_convert_apply", map[string]any{"accept_exact": true})
	assert.True(t, isErr, "no decisions: the domain match is only a suggestion")

	var res map[string]any
	callJSON(t, s, "crm_org_convert_apply", map[string]any{
		"decisions": []any{map[string]any{"key": "globex", "action": "create"}},
	}, &res)
	assert.Equal(t, true, res["applied"])
	plan := res["plan"].([]any)[0].(map[string]any)
	assert.Equal(t, "globex.com", plan["domain"])
}
