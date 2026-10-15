package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanViaMCP(t *testing.T) {
	fixClock(t)
	t.Setenv("CRM_TZ", "America/Chicago")
	s := setupTestServer(t)
	for _, n := range []string{"crm_cal_plan", "crm_cal_plan_undo"} {
		assert.Contains(t, toolNames(s.ListTools()), n)
	}
	var got map[string]any
	callJSON(t, s, "crm_cal_hours_set", map[string]any{"days": "weekdays", "hours": "09:00-17:00", "tz": "America/Chicago"}, &got)
	var task map[string]any
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Write report", "duration": "2h", "auto_schedule": true, "deadline": "2026-10-16"}, &task)
	callJSON(t, s, "crm_task_create", map[string]any{"title": "No length", "auto_schedule": true}, &map[string]any{})

	var plan map[string]any
	callJSON(t, s, "crm_cal_plan", map[string]any{}, &plan)
	assert.Equal(t, false, plan["applied"], "propose only by default")
	tasks := plan["tasks"].([]any)
	require.Len(t, tasks, 2)
	first := tasks[0].(map[string]any)
	assert.Equal(t, "placed", first["decision"])
	assert.Equal(t, "2026-10-15T14:00:00Z", first["blocks"].([]any)[0].(map[string]any)["start"])
	assert.EqualValues(t, 2, plan["summary"].(map[string]any)["placed"])

	var list []map[string]any
	callJSON(t, s, "crm_task_list", map[string]any{}, &list)
	for _, tk := range list {
		assert.Nil(t, tk["scheduled_start"])
	}

	callJSON(t, s, "crm_cal_plan", map[string]any{"apply": true, "lock": []any{task["id"]}}, &plan)
	assert.Equal(t, true, plan["applied"])
	assert.NotNil(t, plan["journal_id"])
	callJSON(t, s, "crm_task_list", map[string]any{}, &list)
	var placed int
	for _, tk := range list {
		if tk["scheduled_start"] != nil {
			placed++
			assert.Equal(t, tk["id"] == task["id"], tk["scheduled_locked"], "only the pinned row is locked")
		}
	}
	assert.Equal(t, 2, placed)

	text, isErr := call(t, s, "crm_cal_plan", map[string]any{"apply": true})
	assert.True(t, isErr, text)
	assert.Contains(t, text, "conflict")

	var undone map[string]any
	callJSON(t, s, "crm_cal_plan_undo", map[string]any{}, &undone)
	assert.Equal(t, true, undone["undone"])
	callJSON(t, s, "crm_task_list", map[string]any{}, &list)
	for _, tk := range list {
		assert.Nil(t, tk["scheduled_start"])
	}
	_, isErr = call(t, s, "crm_cal_plan_undo", map[string]any{})
	assert.True(t, isErr)
}
