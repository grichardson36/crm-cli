package mcp_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestICSToolsViaMCP(t *testing.T) {
	fixClock(t)
	s := setupTestServer(t)
	names := toolNames(s.ListTools())
	assert.Contains(t, names, "crm_cal_import_ics")
	assert.Contains(t, names, "crm_cal_export_ics")

	var jane map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane", "email": "jane@example.com"}, &jane)
	var ev map[string]any
	callJSON(t, s, "crm_event_create", map[string]any{"title": "Lunch", "start": "fri 12:30", "tz": "America/Chicago", "person_ids": []any{jane["id"]}, "tags": []any{"work"}}, &ev)
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Send proposal", "due": "2026-11-01"}, &map[string]any{})

	file, isErr := call(t, s, "crm_cal_export_ics", map[string]any{"include_tasks": true})
	require.False(t, isErr, file)
	assert.Contains(t, file, "BEGIN:VCALENDAR")
	assert.Contains(t, file, "SUMMARY:Lunch")
	assert.Contains(t, file, "BEGIN:VTODO")
	assert.Contains(t, strings.ReplaceAll(file, "\r\n ", ""), "mailto:jane@example.com", "long lines are folded")

	ranged, isErr := call(t, s, "crm_cal_export_ics", map[string]any{"from": "2026-12-01", "to": "2026-12-31"})
	require.False(t, isErr, ranged)
	assert.NotContains(t, ranged, "SUMMARY:Lunch")

	// Importing the file into the same database changes nothing.
	var res map[string]any
	callJSON(t, s, "crm_cal_import_ics", map[string]any{"ics": file}, &res)
	assert.Equal(t, float64(1), res["events"].(map[string]any)["unchanged"])
	assert.Equal(t, float64(1), res["todos"].(map[string]any)["unchanged"])

	// A second source sees new events; a dry run writes nothing.
	callJSON(t, s, "crm_cal_import_ics", map[string]any{"ics": file, "source": "other", "dry_run": true, "include_tasks": false}, &res)
	assert.Equal(t, true, res["dry_run"])
	assert.Equal(t, float64(0), res["todos"].(map[string]any)["created"])
	var list map[string]any
	callJSON(t, s, "crm_event_list", map[string]any{"days": 30, "tz": "America/Chicago"}, &list)
	assert.Len(t, list["events"], 1)

	text, isErr := call(t, s, "crm_cal_import_ics", map[string]any{"ics": "nonsense"})
	assert.True(t, isErr)
	assert.Contains(t, text, "validation error")
	_, isErr = call(t, s, "crm_cal_import_ics", map[string]any{"ics": ""})
	assert.True(t, isErr)
}
