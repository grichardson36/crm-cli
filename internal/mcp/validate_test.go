package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func toolText(t *testing.T, resp any) (string, bool) {
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
	require.NotEmpty(t, r.Result.Content)
	return r.Result.Content[0].Text, r.Result.IsError
}

// MCP errors carry code, field, expected, received, fix and suggestions as isError content.
func TestMCPValidationErrorsAreStructured(t *testing.T) {
	s := setupTestServer(t)
	text, isErr := toolText(t, callTool(t, s, "crm_person_create", map[string]any{"first_name": "Jane", "email": "nope"}))
	require.True(t, isErr)
	var body struct {
		Code, Field, Expected, Received, Fix string
		Issues                               []map[string]any
	}
	require.NoError(t, json.Unmarshal([]byte(text), &body), text)
	assert.Equal(t, "E_EMAIL_INVALID", body.Code)
	assert.Equal(t, "email", body.Field)
	assert.Equal(t, "nope", body.Received)
	assert.NotEmpty(t, body.Expected)
	assert.NotEmpty(t, body.Fix)
	assert.Len(t, body.Issues, 1)
}

func TestMCPSuggestionsAccompanyErrors(t *testing.T) {
	s := setupTestServer(t)
	text, isErr := toolText(t, callTool(t, s, "crm_person_create", map[string]any{
		"first_name": "Jane", "email": "nope", "company": "jane@acme.com"}))
	require.True(t, isErr)
	var body struct {
		Suggestions []struct{ Code, Field, Target, Value string }
	}
	require.NoError(t, json.Unmarshal([]byte(text), &body), text)
	require.Len(t, body.Suggestions, 1)
	assert.Equal(t, "S_EMAIL_IN_TEXT_FIELD", body.Suggestions[0].Code)
	assert.Equal(t, "email", body.Suggestions[0].Target)
	assert.Equal(t, "jane@acme.com", body.Suggestions[0].Value)
}
