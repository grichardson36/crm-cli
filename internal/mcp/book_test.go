package mcp_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookingViaMCP(t *testing.T) {
	fixClock(t) // Thursday 2026-10-15 07:00 Chicago
	t.Setenv("CRM_TZ", "America/Chicago")
	s := setupTestServer(t)
	for _, n := range []string{"crm_book_add", "crm_book_list", "crm_book_offer", "crm_book_export", "crm_book_confirm", "crm_book_cancel", "crm_book_link_update", "crm_cal_invite"} {
		assert.Contains(t, toolNames(s.ListTools()), n)
	}
	var got map[string]any
	callJSON(t, s, "crm_cal_hours_set", map[string]any{"days": "weekdays", "hours": "09:00-17:00", "tz": "America/Chicago"}, &got)

	var dry map[string]any
	callJSON(t, s, "crm_book_add", map[string]any{"name": "Intro call", "dry_run": true}, &dry)
	assert.Equal(t, true, dry["dry_run"])
	var link map[string]any
	callJSON(t, s, "crm_book_add", map[string]any{
		"name": "Intro call", "email": "me@example.com", "lengths": "30m,45m", "buffer": 10, "max_per_day": 3,
		"questions": []any{"Agenda!"}, "reminders": []any{"1h"},
	}, &link)
	assert.Equal(t, "intro-call", link["slug"])
	_, isErr := call(t, s, "crm_book_add", map[string]any{"name": "Intro call"})
	assert.True(t, isErr, "same slug is a conflict")
	_, isErr = call(t, s, "crm_book_add", map[string]any{"name": "Bad", "lengths": "soon"})
	assert.True(t, isErr)

	var offer map[string]any
	callJSON(t, s, "crm_book_offer", map[string]any{"link": "intro-call", "count": 3, "tz": "America/New_York"}, &offer)
	assert.Len(t, offer["slots"], 3)
	assert.Contains(t, offer["text"], "Thu 15 Oct: 11:00 [12:00], 11:30 [12:30], 12:00 [13:00]")

	var page map[string]any
	callJSON(t, s, "crm_book_export", map[string]any{"link": "intro-call", "count": 5}, &page)
	assert.Contains(t, page["html"], "mailto:me@example.com")

	_, isErr = call(t, s, "crm_book_confirm", map[string]any{"link": "intro-call", "at": "fri 10:00", "email": "jane@example.com"})
	assert.True(t, isErr, "the required question has no answer")
	var res map[string]any
	callJSON(t, s, "crm_book_confirm", map[string]any{
		"slot": "intro-call/2026-10-16T15:00:00Z/30", "name": "Jane", "email": "jane@example.com",
		"answers": map[string]any{"Agenda": "pricing"}, "follow_up": true,
	}, &res)
	booking := res["booking"].(map[string]any)
	assert.Equal(t, "2026-10-16T15:00:00Z", booking["start"])
	assert.Contains(t, res["invite_ics"], "METHOD:REQUEST")
	assert.NotZero(t, res["task_id"])
	_, isErr = call(t, s, "crm_book_confirm", map[string]any{"link": "intro-call", "at": "fri 10:00", "email": "bob@example.com", "answers": map[string]any{"Agenda": "x"}})
	assert.True(t, isErr, "the time is taken")

	var list []map[string]any
	callJSON(t, s, "crm_book_list", map[string]any{"bookings": true}, &list)
	require.Len(t, list, 1)

	var inv map[string]any
	callJSON(t, s, "crm_cal_invite", map[string]any{"event_id": 1, "to": []any{"sam@example.com"}, "from": "me@example.com"}, &inv)
	assert.True(t, strings.Contains(inv["ics"].(string), "METHOD:REQUEST"))

	var cancelled map[string]any
	callJSON(t, s, "crm_book_cancel", map[string]any{"id": booking["id"]}, &cancelled)
	assert.Contains(t, cancelled["cancel_ics"], "METHOD:CANCEL")
	callJSON(t, s, "crm_book_list", map[string]any{"bookings": true}, &list)
	assert.Empty(t, list)

	callJSON(t, s, "crm_book_link_update", map[string]any{"link": "intro-call", "active": false}, &link)
	assert.Equal(t, false, link["active"])
	callJSON(t, s, "crm_book_offer", map[string]any{"link": "intro-call"}, &offer)
	assert.Equal(t, "paused", offer["reason"])
	callJSON(t, s, "crm_book_link_update", map[string]any{"link": "intro-call", "delete": true}, &link)
	callJSON(t, s, "crm_book_list", map[string]any{}, &list)
	assert.Empty(t, list)
}
