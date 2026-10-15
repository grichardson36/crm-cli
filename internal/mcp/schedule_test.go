package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskSchedulingViaMCP(t *testing.T) {
	fixClock(t)
	t.Setenv("CRM_TZ", "America/Chicago")
	s := setupTestServer(t)
	names := toolNames(s.ListTools())
	for _, n := range []string{"crm_task_update", "crm_cal_hours_get", "crm_cal_hours_set"} {
		assert.Contains(t, names, n)
	}

	var task map[string]any
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Write report", "duration": "2h", "deadline": "2026-11-06", "hard_deadline": true, "auto_schedule": true, "min_chunk": "30m"}, &task)
	assert.EqualValues(t, 120, task["duration_min"])
	assert.Equal(t, "2026-11-06", task["deadline_at"])
	assert.Equal(t, true, task["auto_schedule"])
	assert.EqualValues(t, 30, task["min_chunk_min"])

	var noLen map[string]any
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Plan", "auto_schedule": true}, &noLen)
	var upd map[string]any
	callJSON(t, s, "crm_task_update", map[string]any{"id": noLen["id"], "title": "Plan trip"}, &upd)
	assert.Equal(t, "no_duration", upd["schedule_issue"])
	upd = map[string]any{}
	callJSON(t, s, "crm_task_update", map[string]any{"id": noLen["id"], "duration": "45m", "scheduled_start": "mon 14:00"}, &upd)
	assert.Nil(t, upd["schedule_issue"])
	assert.Equal(t, "2026-10-19T19:00:00Z", upd["scheduled_start"])
	assert.Equal(t, true, upd["scheduled_locked"])
	callJSON(t, s, "crm_task_update", map[string]any{"id": noLen["id"], "locked": false, "scheduled_start": ""}, &upd)
	assert.Nil(t, upd["scheduled_start"])

	for _, args := range []map[string]any{
		{"id": noLen["id"], "duration": "soon"},
		{"id": noLen["id"], "deadline": "whenever"},
		{"id": noLen["id"], "scheduled_start": "2026-11-03"},
	} {
		text, isErr := call(t, s, "crm_task_update", args)
		assert.True(t, isErr, text)
		assert.Contains(t, text, "validation error")
	}
	_, isErr := call(t, s, "crm_task_update", map[string]any{"id": 99, "title": "x"})
	assert.True(t, isErr)
}

func TestWorkingHoursViaMCP(t *testing.T) {
	fixClock(t)
	t.Setenv("CRM_TZ", "America/Chicago")
	s := setupTestServer(t)

	var got map[string]any
	callJSON(t, s, "crm_cal_hours_get", map[string]any{}, &got)
	hours := got["hours"].(map[string]any)
	assert.Equal(t, "work", hours["name"])
	assert.Equal(t, true, hours["default"])
	assert.EqualValues(t, 10, got["config"].(map[string]any)["buffer_min"])

	callJSON(t, s, "crm_cal_hours_set", map[string]any{"days": "mon-fri", "hours": "08:00-16:00", "tz": "America/Chicago", "buffer": 5, "horizon": 7}, &got)
	assert.Len(t, got["hours"].(map[string]any)["weekly"].(map[string]any)["tue"], 1)
	assert.EqualValues(t, 5, got["config"].(map[string]any)["buffer_min"])
	assert.EqualValues(t, 7, got["config"].(map[string]any)["horizon_days"])

	callJSON(t, s, "crm_cal_hours_set", map[string]any{"date": "2026-10-20", "hours": "off"}, &got)
	assert.Contains(t, got["hours"].(map[string]any)["overrides"], "2026-10-20")
	callJSON(t, s, "crm_cal_hours_set", map[string]any{"days": "sat", "hours": "off"}, &got)

	callJSON(t, s, "crm_cal_hours_get", map[string]any{"expand_from": "2026-10-19"}, &got)
	spans := got["intervals"].([]any)
	require.NotEmpty(t, spans)
	first := spans[0].(map[string]any)
	assert.Equal(t, "2026-10-19T13:00:00Z", first["start"], "Monday 08:00 CDT")
	for _, sp := range spans {
		assert.NotEqual(t, "2026-10-20", sp.(map[string]any)["start"].(string)[:10], "the day off is skipped")
	}

	callJSON(t, s, "crm_cal_hours_set", map[string]any{"remove_date": "2026-10-20"}, &got)
	assert.NotContains(t, got["hours"].(map[string]any), "overrides")

	for _, args := range []map[string]any{
		{"days": "funday", "hours": "09:00-17:00"},
		{"days": "mon", "hours": "nine"},
		{"date": "christmas", "hours": "off"},
		{"step": 0},
	} {
		text, isErr := call(t, s, "crm_cal_hours_set", args)
		assert.True(t, isErr, "%v: %s", args, text)
	}
	_, isErr := call(t, s, "crm_cal_hours_get", map[string]any{"name": "nope"})
	assert.True(t, isErr)
}
