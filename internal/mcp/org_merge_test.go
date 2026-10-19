package mcp_test

import (
	"strings"
	"testing"

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
