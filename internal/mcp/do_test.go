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
	callJSON(t, s, "crm_do", map[string]any{"text": "n Bob called about the deal #deal remind 2026-12-25", "dry_run": true}, &out)
	assert.Equal(t, true, out["dry_run"])
	assert.Equal(t, false, out["applied"])
	assert.Equal(t, "Bob Baker", out["subject"].(map[string]any)["name"])
	assert.Len(t, out["changes"], 3)
	var list []map[string]any
	callJSON(t, s, "crm_interaction_list", map[string]any{}, &list)
	assert.Empty(t, list, "dry run writes nothing")

	callJSON(t, s, "crm_do", map[string]any{"text": "n Bob called about the deal #deal remind 2026-12-25"}, &out)
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
	callJSON(t, s, "crm_do", map[string]any{"text": "t buy milk 2026-12-25 !high"}, &out)
	assert.Equal(t, true, out["applied"])
	var tasks []map[string]any
	callJSON(t, s, "crm_task_list", map[string]any{}, &tasks)
	require.Len(t, tasks, 1)
	assert.Equal(t, "buy milk", tasks[0]["title"])
	assert.Equal(t, "high", tasks[0]["priority"])
	callJSON(t, s, "crm_do", map[string]any{"text": "t keep fri literal", "no_syntax": true}, &out)
	callJSON(t, s, "crm_task_list", map[string]any{}, &tasks)
	titles := []string{}
	for _, tk := range tasks {
		titles = append(titles, tk["title"].(string))
	}
	assert.Contains(t, titles, "keep fri literal")
}

func TestTaskTagsThroughMCP(t *testing.T) {
	s := setupTestServer(t)
	names := toolNames(s.ListTools())
	assert.Contains(t, names, "crm_task_tag")
	assert.Contains(t, names, "crm_task_untag")

	var task map[string]any
	callJSON(t, s, "crm_task_create", map[string]any{"title": "A", "tags": []any{"work", "#urgent"}}, &task)
	assert.Equal(t, []any{"urgent", "work"}, task["tags"])
	callJSON(t, s, "crm_task_create", map[string]any{"title": "B"}, &task)
	callJSON(t, s, "crm_task_tag", map[string]any{"id": task["id"], "tags": []any{"home", "work"}}, &task)
	assert.Equal(t, []any{"home", "work"}, task["tags"])

	var list []map[string]any
	callJSON(t, s, "crm_task_list", map[string]any{"tag": "home"}, &list)
	require.Len(t, list, 1)
	callJSON(t, s, "crm_task_list", map[string]any{"tag": "work"}, &list)
	assert.Len(t, list, 2)

	callJSON(t, s, "crm_task_untag", map[string]any{"id": task["id"], "tags": []any{"work"}}, &task)
	assert.Equal(t, []any{"home"}, task["tags"])
	_, isErr := call(t, s, "crm_task_untag", map[string]any{"id": task["id"], "tags": []any{"work"}})
	assert.True(t, isErr)
	_, isErr = call(t, s, "crm_task_tag", map[string]any{"id": task["id"], "tags": []any{}})
	assert.True(t, isErr)

	// crm_tag_apply takes tasks as well, and crm_do tags a task from its text.
	text, isErr := call(t, s, "crm_tag_apply", map[string]any{"entity_type": "task", "entity_id": task["id"], "tag": "extra"})
	assert.False(t, isErr, text)
	var out map[string]any
	callJSON(t, s, "crm_do", map[string]any{"text": "t pay rent #money"}, &out)
	callJSON(t, s, "crm_task_list", map[string]any{"tag": "money"}, &list)
	require.Len(t, list, 1)
	assert.Equal(t, "pay rent", list[0]["title"])
}
func TestDoToolNamesAndTheSettleFlags(t *testing.T) {
	s := setupTestServer(t)
	var bob map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob", "last_name": "Baker"}, &bob)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Sam", "last_name": "Lee"}, &map[string]any{})
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Sam", "last_name": "Ng"}, &map[string]any{})

	text, isErr := call(t, s, "crm_do", map[string]any{"text": "n coffee with @Sam", "person_id": bob["id"]})
	require.True(t, isErr)
	var bad map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &bad))
	names := bad["names"].([]any)
	require.Len(t, names, 1)
	assert.Len(t, names[0].(map[string]any)["candidates"], 2)

	var out map[string]any
	callJSON(t, s, "crm_do", map[string]any{"text": "n coffee with @Sam", "person_id": bob["id"], "keep_unresolved": true}, &out)
	assert.Equal(t, true, out["applied"])
	var list []map[string]any
	callJSON(t, s, "crm_interaction_list", map[string]any{}, &list)
	require.Len(t, list, 1)
	assert.Equal(t, "coffee with @Sam", list[0]["content"])

	callJSON(t, s, "crm_do", map[string]any{"text": "n met @Priya_Nair", "person_id": bob["id"], "link_new": true, "dry_run": true}, &out)
	assert.Contains(t, out["warnings"], "would create a contact for Priya Nair")
	callJSON(t, s, "crm_do", map[string]any{"text": "n met @Priya_Nair", "person_id": bob["id"], "link_new": true}, &out)
	assert.Equal(t, true, out["applied"])
	var found []map[string]any
	callJSON(t, s, "crm_person_search", map[string]any{"query": "Priya"}, &found)
	require.Len(t, found, 1)
}
func TestTaskDeleteToolDeletesAndRestores(t *testing.T) {
	s := setupTestServer(t)
	var task map[string]any
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Temp"}, &task)
	_, isErr := call(t, s, "crm_task_delete", map[string]any{"id": task["id"]})
	assert.False(t, isErr)
	var list []map[string]any
	callJSON(t, s, "crm_task_list", map[string]any{}, &list)
	assert.Empty(t, list)
	// restore=true is gone from crm_task_delete (X15): it deletes again and the task stays deleted.
	_, isErr = call(t, s, "crm_task_delete", map[string]any{"id": task["id"], "restore": true})
	assert.True(t, isErr, "an already deleted task cannot be deleted again, restore is not an option")
	callJSON(t, s, "crm_task_list", map[string]any{}, &list)
	assert.Empty(t, list)
	_, isErr = call(t, s, "crm_task_restore", map[string]any{"id": task["id"]})
	assert.False(t, isErr)
	callJSON(t, s, "crm_task_list", map[string]any{}, &list)
	assert.Len(t, list, 1)
}
