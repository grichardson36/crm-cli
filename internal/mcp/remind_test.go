package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemindersViaMCP(t *testing.T) {
	fixClock(t) // Thursday 2026-10-15 07:00 Chicago
	t.Setenv("CRM_TZ", "America/Chicago")
	s := setupTestServer(t)
	for _, n := range []string{"crm_cal_due_reminders", "crm_book_remind"} {
		assert.Contains(t, toolNames(s.ListTools()), n)
	}
	var ev map[string]any
	callJSON(t, s, "crm_event_create", map[string]any{"title": "Early call", "start": "thu 07:10", "duration": "20m", "reminders": []any{"15m"}}, &ev)

	var list []map[string]any
	callJSON(t, s, "crm_cal_due_reminders", map[string]any{"dry_run": true}, &list)
	require.Len(t, list, 1)
	assert.Equal(t, "Early call", list[0]["title"])
	callJSON(t, s, "crm_cal_due_reminders", map[string]any{"dry_run": true}, &list)
	require.Len(t, list, 1, "a dry run does not mark")
	callJSON(t, s, "crm_cal_due_reminders", map[string]any{}, &list)
	require.Len(t, list, 1)
	list = nil
	callJSON(t, s, "crm_cal_due_reminders", map[string]any{}, &list)
	assert.Empty(t, list, "listed once")
	_, isErr := call(t, s, "crm_cal_due_reminders", map[string]any{"within": "soon"})
	assert.True(t, isErr)

	var got map[string]any
	callJSON(t, s, "crm_cal_hours_set", map[string]any{"days": "weekdays", "hours": "09:00-17:00", "tz": "America/Chicago"}, &got)
	callJSON(t, s, "crm_book_add", map[string]any{"name": "Intro", "email": "me@example.com", "notice": "1h", "reminders": []any{"1d"}}, &got)
	callJSON(t, s, "crm_book_confirm", map[string]any{"link": "intro", "at": "thu 09:00", "name": "Jane", "email": "jane@example.com"}, &got)
	var drafts []map[string]any
	callJSON(t, s, "crm_book_remind", map[string]any{"dry_run": true}, &drafts)
	require.Len(t, drafts, 1)
	assert.Equal(t, "jane@example.com", drafts[0]["to"])
	callJSON(t, s, "crm_book_remind", map[string]any{}, &drafts)
	require.Len(t, drafts, 1)
	drafts = nil
	callJSON(t, s, "crm_book_remind", map[string]any{}, &drafts)
	assert.Empty(t, drafts)
}
