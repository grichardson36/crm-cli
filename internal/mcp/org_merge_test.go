package mcp_test

import (
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	crmmcp "github.com/jdanielnd/crm-cli/internal/mcp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgCreateRefusesDuplicatesAndOrgMergeTool(t *testing.T) {
	s := setupTestServer(t)
	assert.Contains(t, toolNames(s.ListTools()), "crm_org_merge")
	var a, b map[string]any
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Blue Origin", "domain": "blueorigin.com"}, &a)

	text, isErr := call(t, s, "crm_org_create", map[string]any{"name": "BLUE ORIGIN"})
	assert.True(t, isErr)
	assert.Contains(t, text, "Blue Origin")
	assert.Contains(t, strings.ToLower(text), "allow")

	callJSON(t, s, "crm_org_create", map[string]any{"name": "BLUE ORIGIN", "allow_duplicate": true}, &b)
	require.NotNil(t, b["id"])

	var res map[string]any
	callJSON(t, s, "crm_org_merge", map[string]any{"keep_id": b["id"], "drop_ids": []any{a["id"]}, "dry_run": true}, &res)
	assert.Equal(t, true, res["dry_run"])
	callJSON(t, s, "crm_org_merge", map[string]any{"keep_id": b["id"], "drop_ids": []any{a["id"]}}, &res)
	assert.Equal(t, false, res["dry_run"])
	fields := res["fields"].([]any)
	require.Len(t, fields, 1)
	assert.Equal(t, "domain", fields[0].(map[string]any)["field"])

	_, isErr = call(t, s, "crm_org_merge", map[string]any{"keep_id": b["id"], "drop_ids": []any{b["id"]}})
	assert.True(t, isErr)
}

func TestOrgNativeFieldsThroughMCP(t *testing.T) {
	s := setupTestServer(t)
	var o map[string]any
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme", "website": "https://acme.com", "size": "11-50", "city": "Austin", "country": "US",
		"linkedin_url": "https://linkedin.com/company/acme", "description": "Rockets", "logo_path": "a.png", "region": "TX"}, &o)
	assert.Equal(t, "11-50", o["size"])
	assert.Equal(t, "TX", o["region"])
	callJSON(t, s, "crm_org_update", map[string]any{"id": o["id"], "size": "51-200"}, &o)
	assert.Equal(t, "51-200", o["size"])
	assert.Equal(t, "https://acme.com", o["website"])
	assert.Equal(t, "a.png", o["logo_path"])
}

func TestModulesTools(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	dir := t.TempDir()
	s := crmmcp.NewServer(d, "test", crmmcp.WithDataDir(func() (string, error) { return dir, nil }))
	for _, name := range []string{"crm_modules_list", "crm_modules_set", "crm_modules_reset"} {
		assert.Contains(t, toolNames(s.ListTools()), name)
	}
	var rows []map[string]any
	callJSON(t, s, "crm_modules_list", map[string]any{}, &rows)
	require.Len(t, rows, 12)
	assert.Equal(t, "home", rows[0]["id"])
	assert.Equal(t, "deals", rows[5]["id"])
	assert.Equal(t, "tags", rows[6]["id"], "Tags comes after Deals")
	for _, r := range rows {
		hidden := r["id"] == "archived" || r["id"] == "sync"
		assert.Equal(t, !hidden, r["on"], "Archived and Sync start hidden, the rest is on: %v", r["id"])
	}

	callJSON(t, s, "crm_modules_set", map[string]any{"module": "deals", "on": false}, &rows)
	callJSON(t, s, "crm_modules_set", map[string]any{"module": "deals", "on": true}, &rows)
	callJSON(t, s, "crm_modules_set", map[string]any{"module": "organizations", "position": 2}, &rows)
	callJSON(t, s, "crm_modules_set", map[string]any{"module": "birthdays", "on": false}, &rows)
	byID := map[string]map[string]any{}
	for _, r := range rows {
		byID[r["id"].(string)] = r
	}
	assert.EqualValues(t, 2, byID["organizations"]["order"])
	assert.Equal(t, false, byID["birthdays"]["on"])
	assert.Equal(t, true, byID["deals"]["on"])

	_, isErr := call(t, s, "crm_modules_set", map[string]any{"module": "nope", "on": true})
	assert.True(t, isErr)
	_, isErr = call(t, s, "crm_modules_set", map[string]any{"module": "people"})
	assert.True(t, isErr, "give on or position")
	callJSON(t, s, "crm_modules_set", map[string]any{"module": "home", "on": false}, &rows)
	_, isErr = call(t, s, "crm_modules_set", map[string]any{"module": "people", "on": false})
	assert.True(t, isErr, "Home or People stays on")

	callJSON(t, s, "crm_modules_reset", map[string]any{}, &rows)
	assert.Equal(t, true, rows[0]["on"])
	assert.Equal(t, "settings", rows[11]["id"])
	assert.Equal(t, true, rows[11]["on"])
	assert.Equal(t, false, rows[10]["on"])
	assert.Equal(t, "home", rows[0]["id"])
	_, isErr = call(t, s, "crm_modules_set", map[string]any{"module": "settings", "on": false})
	assert.True(t, isErr, "Settings stays on")
}
