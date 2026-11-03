package mcp_test

import (
	"encoding/json"
	"testing"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mnText(t *testing.T, resp gomcp.JSONRPCMessage) (string, bool) {
	t.Helper()
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var r struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &r))
	require.NotEmpty(t, r.Result.Content, string(raw))
	return r.Result.Content[0].Text, r.Result.IsError
}

// CRM-019, MCP door: create, update, get and search carry the middle name like the other name fields.
func TestMCPPersonMiddleName_CreateUpdateGetSearch(t *testing.T) {
	s := setupTestServer(t)

	text, isErr := mnText(t, callTool(t, s, "crm_person_create", map[string]any{
		"first_name": "Ann", "last_name": "Smith", "middle_name": "  Marigold ",
	}))
	require.False(t, isErr, text)
	var p map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &p))
	assert.Equal(t, "Marigold", p["middle_name"], "trimmed like the other names")
	assert.Equal(t, "Ann", p["first_name"])

	text, isErr = mnText(t, callTool(t, s, "crm_person_search", map[string]any{"query": "Marigold"}))
	require.False(t, isErr, text)
	assert.Contains(t, text, "Ann")

	text, isErr = mnText(t, callTool(t, s, "crm_person_update", map[string]any{"id": 1, "middle_name": "Quill"}))
	require.False(t, isErr, text)
	require.NoError(t, json.Unmarshal([]byte(text), &p))
	assert.Equal(t, "Quill", p["middle_name"])

	text, _ = mnText(t, callTool(t, s, "crm_person_get", map[string]any{"id": 1}))
	assert.Contains(t, text, `"middle_name": "Quill"`)

	text, isErr = mnText(t, callTool(t, s, "crm_person_update", map[string]any{"id": 1, "middle_name": ""}))
	require.False(t, isErr, text)
	require.NoError(t, json.Unmarshal([]byte(text), &p))
	assert.Equal(t, "", p["middle_name"], "empty clears")
}
