package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func roleList(t *testing.T, _ any, call func(string, map[string]any) (string, bool), args map[string]any) []string {
	t.Helper()
	text, isErr := call("crm_deal_role_list", args)
	require.False(t, isErr, text)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &rows))
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r["name"].(string)
	}
	return out
}

func TestMCPDealRoleTools(t *testing.T) {
	s := setupTestServer(t)
	tools := toolNames(s.ListTools())
	for _, n := range []string{"crm_deal_role_list", "crm_deal_role_add", "crm_deal_role_rename", "crm_deal_role_remove", "crm_deal_role_reorder", "crm_deal_role_backfill"} {
		assert.Contains(t, tools, n)
	}
	c := func(name string, args map[string]any) (string, bool) { return call(t, s, name, args) }
	code := func(text string) string {
		var b struct{ Code string }
		require.NoError(t, json.Unmarshal([]byte(text), &b), text)
		return b.Code
	}

	var p, d map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &p)
	callJSON(t, s, "crm_deal_create", map[string]any{"title": "Site"}, &d)
	pid, did := p["id"], d["id"]

	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantErr  bool
		wantCode string
		wantList []string // the head of the list afterwards
	}{
		{"seeded", "crm_deal_role_list", map[string]any{}, false, "", []string{"champion", "decision maker"}},
		{"add", "crm_deal_role_add", map[string]any{"name": "technical evaluator"}, false, "", nil},
		{"add duplicate", "crm_deal_role_add", map[string]any{"name": "CHAMPION"}, true, "E_VOCAB_DUPLICATE", nil},
		{"add missing name", "crm_deal_role_add", map[string]any{}, true, "", nil},
		{"rename", "crm_deal_role_rename", map[string]any{"name": "signer", "new_name": "contract signer"}, false, "", nil},
		{"rename onto another", "crm_deal_role_rename", map[string]any{"name": "blocker", "new_name": "influencer"}, true, "E_VOCAB_DUPLICATE", nil},
		{"reorder names", "crm_deal_role_reorder", map[string]any{"names": []string{"influencer", "champion"}}, false, "", []string{"influencer", "champion", "decision maker"}},
		{"reorder one", "crm_deal_role_reorder", map[string]any{"name": "contract signer", "position": 1}, false, "", []string{"contract signer", "influencer", "champion"}},
		{"reorder nothing", "crm_deal_role_reorder", map[string]any{}, true, "", nil},
		{"remove unused", "crm_deal_role_remove", map[string]any{"name": "end user"}, false, "", nil},
		{"remove unknown", "crm_deal_role_remove", map[string]any{"name": "ghost"}, true, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, isErr := c(tt.tool, tt.args)
			assert.Equal(t, tt.wantErr, isErr, text)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, code(text))
			}
			if tt.wantList != nil {
				got := roleList(t, s, c, map[string]any{})
				assert.Equal(t, tt.wantList, got[:len(tt.wantList)])
			}
		})
	}

	t.Run("deal link: unknown role refused with a fix, create adds it", func(t *testing.T) {
		text, isErr := c("crm_deal_link", map[string]any{"id": did, "other": "person", "other_id": pid, "role": "kingmaker"})
		require.True(t, isErr, text)
		var body struct{ Code, Fix string }
		require.NoError(t, json.Unmarshal([]byte(text), &body), text)
		assert.Equal(t, "E_VOCAB_UNKNOWN", body.Code)
		assert.Contains(t, body.Fix, "create")
		assert.NotContains(t, roleList(t, s, c, map[string]any{}), "kingmaker")

		text, isErr = c("crm_deal_link", map[string]any{"id": did, "other": "person", "other_id": pid, "role": "kingmaker", "create": true})
		require.False(t, isErr, text)
		assert.Contains(t, roleList(t, s, c, map[string]any{}), "kingmaker")
	})
	t.Run("remove of a used role needs replace_with or keep", func(t *testing.T) {
		text, isErr := c("crm_deal_role_remove", map[string]any{"name": "kingmaker"})
		require.True(t, isErr)
		assert.Equal(t, "E_VOCAB_IN_USE", code(text))
		text, isErr = c("crm_deal_role_remove", map[string]any{"name": "kingmaker", "replace_with": "influencer"})
		require.False(t, isErr, text)
		var res map[string]any
		require.NoError(t, json.Unmarshal([]byte(text), &res))
		assert.EqualValues(t, 1, res["moved"])
	})
	t.Run("backfill dry run, then apply", func(t *testing.T) {
		text, isErr := c("crm_deal_role_backfill", map[string]any{})
		require.False(t, isErr, text)
		var res map[string]any
		require.NoError(t, json.Unmarshal([]byte(text), &res))
		assert.Equal(t, false, res["applied"])
		assert.Contains(t, res["counts"], "to_add")
		text, isErr = c("crm_deal_role_backfill", map[string]any{"apply": true})
		require.False(t, isErr, text)
	})
}
