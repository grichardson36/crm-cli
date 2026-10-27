package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPPersonCompanyCreatesAndReusesOrganization(t *testing.T) {
	s := setupTestServer(t)
	for _, n := range []string{"crm_org_backfill", "crm_org_ignore_list", "crm_org_ignore_add", "crm_org_ignore_remove", "crm_org_dupes", "crm_org_convert_list", "crm_org_convert_apply"} {
		assert.Contains(t, toolNames(s.ListTools()), n)
	}
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann", "last_name": "Lee", "company": "Acme Corp"}, &p)
	org := p["organization"].(map[string]any)
	assert.Equal(t, true, org["org_created"])
	assert.Equal(t, "Acme Corp", org["org_name"])
	assert.Equal(t, "Acme Corp", p["company"], "company stays the source text")

	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob", "last_name": "Ray", "company": "ACME corp"}, &p)
	org = p["organization"].(map[string]any)
	assert.Nil(t, org["org_created"], "reused")

	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Cy", "last_name": "Doe", "company": "N/A"}, &p)
	assert.Equal(t, true, p["organization"].(map[string]any)["ignored"])

	callJSON(t, s, "crm_person_update", map[string]any{"id": p["id"], "company": "Globex"}, &p)
	assert.Equal(t, true, p["organization"].(map[string]any)["org_created"])

	for _, name := range []string{"Acme", "Globex"} {
		var orgs []map[string]any
		callJSON(t, s, "crm_org_search", map[string]any{"query": name}, &orgs)
		assert.Len(t, orgs, 1, name)
	}
	var none []map[string]any
	callJSON(t, s, "crm_org_search", map[string]any{"query": "N/A"}, &none)
	assert.Empty(t, none)
}

func TestMCPBackfillIsADryRunByDefaultAndIgnoreTools(t *testing.T) {
	s := setupTestServer(t)
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann", "last_name": "Lee"}, &p)
	// Company text without the rule: write it straight to the table.
	// (the rule runs in the tools, so set it through a junk string first and turn the string back on)
	callJSON(t, s, "crm_org_ignore_add", map[string]any{"text": "Gig Work"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob", "last_name": "Ray", "company": "Gig Work"}, &p)
	assert.Equal(t, true, p["organization"].(map[string]any)["ignored"])
	callJSON(t, s, "crm_org_ignore_remove", map[string]any{"text": "gig work"}, &p)

	var out map[string]any
	callJSON(t, s, "crm_org_backfill", map[string]any{}, &out)
	assert.Equal(t, false, out["applied"], "dry run by default")
	counts := out["counts"].(map[string]any)
	assert.EqualValues(t, 1, counts["people_with_company"])
	assert.EqualValues(t, 1, counts["orgs_to_create"])
	var orgs []map[string]any
	callJSON(t, s, "crm_org_search", map[string]any{"query": "Gig"}, &orgs)
	assert.Empty(t, orgs, "the dry run wrote nothing")

	callJSON(t, s, "crm_org_backfill", map[string]any{"apply": true}, &out)
	assert.Equal(t, true, out["applied"])
	assert.EqualValues(t, 1, out["result"].(map[string]any)["orgs_created"])
	callJSON(t, s, "crm_org_backfill", map[string]any{"apply": true}, &out)
	assert.EqualValues(t, 0, out["result"].(map[string]any)["orgs_created"], "idempotent")

	// The deprecated names map to the backfill and stay a dry run.
	callJSON(t, s, "crm_org_convert_apply", map[string]any{}, &out)
	assert.Equal(t, false, out["applied"])
	assert.Contains(t, out["note"], "deprecated")

	var list []map[string]any
	callJSON(t, s, "crm_org_ignore_list", map[string]any{}, &list)
	require.NotEmpty(t, list)
	_, isErr := call(t, s, "crm_org_ignore_remove", map[string]any{"text": "never listed"})
	assert.True(t, isErr)
}
