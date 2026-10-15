package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoToolIsRegistered(t *testing.T) {
	assert.Contains(t, toolNames(setupTestServer(t).ListTools()), "crm_do")
}

func TestDoToolNoteDryRunThenApply(t *testing.T) {
	s := setupTestServer(t)
	var bob, sam1, sam2 map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob", "last_name": "Baker"}, &bob)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Sam", "last_name": "Lee"}, &sam1)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Sam", "last_name": "Ng"}, &sam2)

	var out map[string]any
	callJSON(t, s, "crm_do", map[string]any{"text": "n Bob called about the deal #deal @2026-12-25", "dry_run": true}, &out)
	assert.Equal(t, true, out["dry_run"])
	assert.Equal(t, false, out["applied"])
	assert.Equal(t, "Bob Baker", out["subject"].(map[string]any)["name"])
	assert.Len(t, out["changes"], 3)
	var list []map[string]any
	callJSON(t, s, "crm_interaction_list", map[string]any{}, &list)
	assert.Empty(t, list, "dry run writes nothing")

	callJSON(t, s, "crm_do", map[string]any{"text": "n Bob called about the deal #deal @2026-12-25"}, &out)
	assert.Equal(t, true, out["applied"])
	callJSON(t, s, "crm_interaction_list", map[string]any{}, &list)
	require.Len(t, list, 1)
	assert.Equal(t, "called about the deal", list[0]["content"])
	var tasks []map[string]any
	callJSON(t, s, "crm_task_list", map[string]any{}, &tasks)
	require.Len(t, tasks, 1)
	assert.Equal(t, "2026-12-25", tasks[0]["due_at"])

	// person_id replaces the name.
	callJSON(t, s, "crm_do", map[string]any{"text": "set company=Acme title=CTO", "person_id": bob["id"]}, &out)
	assert.Equal(t, true, out["applied"])
	assert.Equal(t, true, out["needs_confirm"])
	var p map[string]any
	callJSON(t, s, "crm_person_get", map[string]any{"id": bob["id"]}, &p)
	assert.Equal(t, "Acme", p["company"])

	// An ambiguous name changes nothing and returns candidates.
	text, isErr := call(t, s, "crm_do", map[string]any{"text": "n Sam called"})
	require.True(t, isErr)
	var bad map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &bad), text)
	problems := bad["problems"].([]any)
	require.Len(t, problems, 1)
	assert.Equal(t, "ambiguous", problems[0].(map[string]any)["code"])
	assert.Len(t, problems[0].(map[string]any)["candidates"], 2)
	callJSON(t, s, "crm_interaction_list", map[string]any{}, &list)
	assert.Len(t, list, 1)
}

func TestDoToolErrors(t *testing.T) {
	s := setupTestServer(t)
	text, isErr := call(t, s, "crm_do", map[string]any{"text": ""})
	assert.True(t, isErr, text)
	text, isErr = call(t, s, "crm_do", map[string]any{"text": "hello there"})
	assert.True(t, isErr)
	assert.Contains(t, text, "must start with a verb")
	text, isErr = call(t, s, "crm_do", map[string]any{"text": "t x", "person_id": 99})
	assert.True(t, isErr)
	assert.Equal(t, "not found", text)
	text, isErr = call(t, s, "crm_do", map[string]any{"text": "n anything"})
	assert.True(t, isErr, "no person: %s", text)

	// A task needs no person, and no_syntax keeps tokens as text.
	var out map[string]any
	callJSON(t, s, "crm_do", map[string]any{"text": "t buy milk @2026-12-25 ^high"}, &out)
	assert.Equal(t, true, out["applied"])
	var tasks []map[string]any
	callJSON(t, s, "crm_task_list", map[string]any{}, &tasks)
	require.Len(t, tasks, 1)
	assert.Equal(t, "buy milk", tasks[0]["title"])
	assert.Equal(t, "high", tasks[0]["priority"])
	callJSON(t, s, "crm_do", map[string]any{"text": "t keep @fri literal", "no_syntax": true}, &out)
	callJSON(t, s, "crm_task_list", map[string]any{}, &tasks)
	titles := []string{}
	for _, tk := range tasks {
		titles = append(titles, tk["title"].(string))
	}
	assert.Contains(t, titles, "keep @fri literal")
}
