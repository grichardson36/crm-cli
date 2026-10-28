package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPDealStageTools(t *testing.T) {
	s := setupTestServer(t)
	tools := toolNames(s.ListTools())
	for _, n := range []string{"crm_deal_stage_list", "crm_deal_stage_add", "crm_deal_stage_rename", "crm_deal_stage_remove", "crm_deal_stage_reorder", "crm_deal_stage_outcome"} {
		assert.Contains(t, tools, n)
	}
	c := func(name string, args map[string]any) (string, bool) { return call(t, s, name, args) }
	code := func(text string) string {
		var b struct{ Code string }
		require.NoError(t, json.Unmarshal([]byte(text), &b), text)
		return b.Code
	}
	list := func() (names []string, outcomes map[string]string) {
		text, isErr := c("crm_deal_stage_list", map[string]any{})
		require.False(t, isErr, text)
		var rows []map[string]any
		require.NoError(t, json.Unmarshal([]byte(text), &rows))
		outcomes = map[string]string{}
		for _, r := range rows {
			names = append(names, r["name"].(string))
			outcomes[r["name"].(string)] = r["outcome"].(string)
		}
		return
	}

	names, outcomes := list()
	assert.Equal(t, []string{"lead", "prospect", "proposal", "negotiation", "won", "lost"}, names)
	assert.Equal(t, "won", outcomes["won"])
	assert.Equal(t, "lost", outcomes["lost"])
	assert.Equal(t, "open", outcomes["lead"])

	var d map[string]any
	callJSON(t, s, "crm_deal_create", map[string]any{"title": "Site", "stage": "proposal"}, &d)
	did := d["id"]

	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantErr  bool
		wantCode string
	}{
		{"add", "crm_deal_stage_add", map[string]any{"name": "qualified"}, false, ""},
		{"add duplicate", "crm_deal_stage_add", map[string]any{"name": "LEAD"}, true, "E_VOCAB_DUPLICATE"},
		{"add missing name", "crm_deal_stage_add", map[string]any{}, true, ""},
		{"outcome", "crm_deal_stage_outcome", map[string]any{"name": "qualified", "outcome": "lost"}, false, ""},
		{"outcome invalid", "crm_deal_stage_outcome", map[string]any{"name": "lead", "outcome": "closed"}, true, "E_STAGE_OUTCOME_INVALID"},
		{"outcome would leave no won stage", "crm_deal_stage_outcome", map[string]any{"name": "won", "outcome": "open"}, true, "E_STAGE_NO_WON"},
		{"remove the only won stage", "crm_deal_stage_remove", map[string]any{"name": "won"}, true, "E_STAGE_NO_WON"},
		{"rename", "crm_deal_stage_rename", map[string]any{"name": "negotiation", "new_name": "talks"}, false, ""},
		{"rename onto another", "crm_deal_stage_rename", map[string]any{"name": "talks", "new_name": "Lead"}, true, "E_VOCAB_DUPLICATE"},
		{"reorder names", "crm_deal_stage_reorder", map[string]any{"names": []string{"talks", "lead"}}, false, ""},
		{"reorder one", "crm_deal_stage_reorder", map[string]any{"name": "lead", "position": 1}, false, ""},
		{"reorder nothing", "crm_deal_stage_reorder", map[string]any{}, true, ""},
		{"remove a used stage needs a replacement", "crm_deal_stage_remove", map[string]any{"name": "proposal"}, true, "E_VOCAB_IN_USE"},
		{"keep is not a stage option", "crm_deal_stage_remove", map[string]any{"name": "proposal", "keep": true}, true, "E_VOCAB_IN_USE"},
		{"remove with a replacement", "crm_deal_stage_remove", map[string]any{"name": "proposal", "replace_with": "talks"}, false, ""},
		{"remove unknown", "crm_deal_stage_remove", map[string]any{"name": "ghost"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, isErr := c(tt.tool, tt.args)
			assert.Equal(t, tt.wantErr, isErr, text)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, code(text))
			}
		})
	}

	names, outcomes = list()
	assert.Equal(t, []string{"lead", "talks", "prospect", "won", "lost", "qualified"}, names[:6], "order after the reorders")
	assert.Equal(t, "lost", outcomes["qualified"])

	t.Run("the moved deal has the replacement stage", func(t *testing.T) {
		var got map[string]any
		callJSON(t, s, "crm_deal_update", map[string]any{"id": did}, &got)
		assert.Equal(t, "talks", got["stage"])
	})
	t.Run("deal tools use the list and the outcome", func(t *testing.T) {
		text, isErr := c("crm_deal_update", map[string]any{"id": did, "stage": "nowhere"})
		require.True(t, isErr)
		assert.Equal(t, "E_VOCAB_UNKNOWN", code(text))
		var got map[string]any
		callJSON(t, s, "crm_deal_update", map[string]any{"id": did, "stage": "qualified"}, &got)
		assert.NotNil(t, got["closed_at"], "qualified is a lost stage, closed_at follows the outcome")
		callJSON(t, s, "crm_deal_update", map[string]any{"id": did, "stage": "talks"}, &got)
		assert.Nil(t, got["closed_at"])
		callJSON(t, s, "crm_deal_create", map[string]any{"title": "Default stage"}, &got)
		assert.Equal(t, "lead", got["stage"], "the first open stage")
	})
}
