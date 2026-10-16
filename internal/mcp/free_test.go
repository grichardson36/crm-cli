package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindATimeViaMCP(t *testing.T) {
	fixClock(t)
	t.Setenv("CRM_TZ", "America/Chicago")
	s := setupTestServer(t)
	assert.Contains(t, toolNames(s.ListTools()), "crm_cal_free")

	var got map[string]any
	callJSON(t, s, "crm_cal_hours_set", map[string]any{"days": "weekdays", "hours": "09:00-17:00", "tz": "America/Chicago"}, &got)
	var ev map[string]any
	callJSON(t, s, "crm_event_create", map[string]any{"title": "Planning", "start": "2026-10-15 10:00", "duration": "1h", "tz": "America/Chicago"}, &ev)

	var res map[string]any
	callJSON(t, s, "crm_cal_free", map[string]any{"length": "30m", "within_days": 1, "limit": 3}, &res)
	assert.Equal(t, "America/Chicago", res["tz"])
	assert.EqualValues(t, 10, res["buffer_min"])
	slots := res["slots"].([]any)
	require.Len(t, slots, 3)
	assert.Equal(t, "2026-10-15T14:00:00Z", slots[0].(map[string]any)["start"])
	assert.Equal(t, "2026-10-15T14:30:00Z", slots[0].(map[string]any)["end"])
}
