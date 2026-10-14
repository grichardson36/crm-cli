package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// call runs a tool and returns its text output and whether it was an error result.
func call(t *testing.T, s *server.MCPServer, name string, args map[string]any) (string, bool) {
	t.Helper()
	raw, err := json.Marshal(callTool(t, s, name, args))
	require.NoError(t, err)
	var r struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &r), string(raw))
	require.NotEmpty(t, r.Result.Content, string(raw))
	return r.Result.Content[0].Text, r.Result.IsError
}

func callJSON(t *testing.T, s *server.MCPServer, name string, args map[string]any, out any) {
	t.Helper()
	text, isErr := call(t, s, name, args)
	require.False(t, isErr, text)
	require.NoError(t, json.Unmarshal([]byte(text), out), text)
}

func TestNewToolsRegistered(t *testing.T) {
	names := toolNames(setupTestServer(t).ListTools())
	for _, n := range []string{
		"crm_person_add_contact_method", "crm_person_remove_contact_method",
		"crm_due", "crm_birthdays", "crm_person_mark_contacted", "crm_task_complete",
	} {
		assert.Contains(t, names, n)
	}
}

func TestContactMethodsAndProfileFields(t *testing.T) {
	s := setupTestServer(t)
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane", "email": "jane@work.com"}, &p)

	callJSON(t, s, "crm_person_add_contact_method", map[string]any{"id": 1, "type": "email", "value": "jane@home.com", "label": "Home"}, &p)
	callJSON(t, s, "crm_person_add_contact_method", map[string]any{"id": 1, "type": "phone", "value": "+15551112222"}, &p)
	callJSON(t, s, "crm_person_add_contact_method", map[string]any{"id": 1, "type": "handle", "kind": "linkedin", "value": "https://linkedin.com/in/jane"}, &p)
	assert.Len(t, p["emails"], 2)
	assert.Len(t, p["phones"], 1)
	assert.Len(t, p["handles"], 1)
	assert.Equal(t, "jane@work.com", p["email"])

	callJSON(t, s, "crm_person_update", map[string]any{
		"id": 1, "birthday": "--03-14", "keep_in_touch": "1 mon", "is_favorite": true, "priority_tier": 1,
	}, &p)
	assert.EqualValues(t, 3, p["birthday_month"])
	assert.Equal(t, "1 mon", p["keep_in_touch"])
	assert.Equal(t, true, p["is_favorite"])

	// Fetching a person includes the contact methods.
	callJSON(t, s, "crm_person_get", map[string]any{"id": 1}, &p)
	assert.Len(t, p["emails"], 2)

	callJSON(t, s, "crm_person_remove_contact_method", map[string]any{"id": 1, "type": "email", "value": "jane@home.com"}, &p)
	assert.Len(t, p["emails"], 1)

	text, isErr := call(t, s, "crm_person_update", map[string]any{"id": 1, "keep_in_touch": "weekly"})
	assert.True(t, isErr, text)
	text, isErr = call(t, s, "crm_person_add_contact_method", map[string]any{"id": 1, "type": "fax", "value": "x"})
	assert.True(t, isErr, text)
}

func TestDueAndMarkContacted(t *testing.T) {
	s := setupTestServer(t)
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Olive"}, &p)
	callJSON(t, s, "crm_person_update", map[string]any{"id": 1, "keep_in_touch": "7 days", "last_contacted_at": "2020-01-01"}, &p)

	var due []map[string]any
	callJSON(t, s, "crm_due", map[string]any{}, &due)
	require.Len(t, due, 1)
	assert.Equal(t, "Olive", due[0]["name"])

	callJSON(t, s, "crm_person_mark_contacted", map[string]any{"id": 1}, &p)
	callJSON(t, s, "crm_due", map[string]any{}, &due)
	assert.Empty(t, due)

	var b []map[string]any
	callJSON(t, s, "crm_birthdays", map[string]any{"days": 400}, &b)
	assert.Empty(t, b)
}

func TestRecurringTaskViaMCP(t *testing.T) {
	s := setupTestServer(t)
	var p, task map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann"}, &p)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ben"}, &p)

	callJSON(t, s, "crm_task_create", map[string]any{
		"title": "Sync", "due": "2026-01-15", "recurrence": "1 mon", "person_id": 1, "person_ids": []any{2},
	}, &task)
	assert.Len(t, task["person_ids"], 2)

	callJSON(t, s, "crm_task_complete", map[string]any{"id": 1}, &task)
	assert.EqualValues(t, 2, task["next_task_id"])
}
