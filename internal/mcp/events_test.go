package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The clock is fixed to Thursday 2026-10-15 07:00 Chicago so phrases such as "fri 12:30" are stable.
func fixClock(t *testing.T) {
	t.Helper()
	t.Setenv("CRM_NOW", "2026-10-15T12:00:00Z")
}

func TestEventToolsAreRegistered(t *testing.T) {
	names := toolNames(setupTestServer(t).ListTools())
	for _, n := range []string{"crm_event_create", "crm_event_list", "crm_event_get", "crm_event_update", "crm_event_delete", "crm_event_attendee"} {
		assert.Contains(t, names, n)
	}
}

func TestEventCreateGetListViaMCP(t *testing.T) {
	fixClock(t)
	s := setupTestServer(t)
	var jane, ev map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &jane)

	callJSON(t, s, "crm_event_create", map[string]any{
		"title": "Lunch", "start": "fri 12:30", "duration": "1h", "tz": "America/Chicago", "location": "Cafe Rio",
		"person_ids": []any{jane["id"]}, "reminders": []any{"15m"}, "tags": []any{"lunch"},
	}, &ev)
	assert.Equal(t, "2026-10-16T17:30:00Z", ev["start_utc"])
	assert.Equal(t, "2026-10-16T18:30:00Z", ev["end_utc"])
	assert.Equal(t, "America/Chicago", ev["tz"])
	assert.Len(t, ev["attendees"], 1)
	assert.Len(t, ev["reminders"], 1)
	assert.Equal(t, []any{"lunch"}, ev["tags"])

	var got map[string]any
	callJSON(t, s, "crm_event_get", map[string]any{"id": ev["id"]}, &got)
	assert.Equal(t, "Lunch", got["title"])

	var list map[string]any
	callJSON(t, s, "crm_event_list", map[string]any{"tz": "America/Chicago", "days": 7}, &list)
	events := list["events"].([]any)
	require.Len(t, events, 1)
	assert.Equal(t, "Lunch", events[0].(map[string]any)["title"])
	assert.Equal(t, false, list["truncated"])
	assert.Equal(t, "2026-10-15T05:00:00Z", list["from"])

	callJSON(t, s, "crm_event_list", map[string]any{"tz": "America/Chicago", "tag": "none"}, &list)
	assert.Empty(t, list["events"])
	callJSON(t, s, "crm_event_list", map[string]any{"tz": "America/Chicago", "person_id": jane["id"], "query": "lunch"}, &list)
	assert.Len(t, list["events"], 1)
}

func TestEventRepeatScopesViaMCP(t *testing.T) {
	fixClock(t)
	s := setupTestServer(t)
	var ev map[string]any
	callJSON(t, s, "crm_event_create", map[string]any{
		"title": "Standup", "start": "2026-10-19 09:15", "duration": "15m", "tz": "America/Chicago", "repeat": "weekdays", "count": 10,
	}, &ev)
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;COUNT=10", ev["rrule"])
	assert.Equal(t, "every weekday, 10 times", ev["repeat"])

	var list map[string]any
	callJSON(t, s, "crm_event_list", map[string]any{"tz": "America/Chicago", "from": "2026-10-19", "days": 7}, &list)
	events := list["events"].([]any)
	require.Len(t, events, 5)
	third := events[2].(map[string]any)
	assert.Equal(t, "2026-10-21T14:15:00Z", third["occurrence"])

	var moved map[string]any
	callJSON(t, s, "crm_event_update", map[string]any{
		"id": ev["id"], "scope": "this", "at": third["occurrence"], "title": "Standup (late)", "start": "2026-10-21 11:00",
	}, &moved)
	assert.Equal(t, "Standup (late)", moved["title"])
	assert.NotNil(t, moved["recurrence_parent_id"])

	text, isErr := call(t, s, "crm_event_delete", map[string]any{"id": ev["id"], "scope": "this", "at": "2026-10-22T14:15:00Z"})
	assert.False(t, isErr, text)
	callJSON(t, s, "crm_event_list", map[string]any{"tz": "America/Chicago", "from": "2026-10-19", "days": 7}, &list)
	assert.Len(t, list["events"], 4)
	assert.Equal(t, "Standup (late)", list["events"].([]any)[2].(map[string]any)["title"])

	var split map[string]any
	callJSON(t, s, "crm_event_update", map[string]any{"id": ev["id"], "scope": "following", "at": "2026-10-26T14:15:00Z", "location": "Annex", "reminders": []any{"10m"}}, &split)
	assert.NotEqual(t, ev["id"], split["id"])
	assert.Equal(t, "Annex", split["location"])
	assert.Len(t, split["reminders"], 1)
}

func TestEventDeleteRestorePurgeAndAttendeesViaMCP(t *testing.T) {
	fixClock(t)
	s := setupTestServer(t)
	var jane, ev, after, removed map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &jane)
	callJSON(t, s, "crm_event_create", map[string]any{"title": "x", "start": "2026-11-26", "all_day": true}, &ev)
	assert.Equal(t, true, ev["all_day"])
	assert.Equal(t, "2026-11-27", ev["end_date"])

	callJSON(t, s, "crm_event_attendee", map[string]any{"event_id": ev["id"], "person_id": jane["id"], "role": "organizer", "rsvp": "accepted"}, &after)
	require.Len(t, after["attendees"], 1)
	assert.Equal(t, "organizer", after["attendees"].([]any)[0].(map[string]any)["role"])
	callJSON(t, s, "crm_event_attendee", map[string]any{"event_id": ev["id"], "person_id": jane["id"], "remove": true}, &removed)
	assert.Empty(t, removed["attendees"])

	_, isErr := call(t, s, "crm_event_delete", map[string]any{"id": ev["id"], "purge": true})
	assert.True(t, isErr, "archive first")
	text, isErr := call(t, s, "crm_event_delete", map[string]any{"id": ev["id"]})
	assert.False(t, isErr, text)
	_, isErr = call(t, s, "crm_event_get", map[string]any{"id": ev["id"]})
	assert.True(t, isErr)
	text, isErr = call(t, s, "crm_event_delete", map[string]any{"id": ev["id"], "restore": true})
	assert.False(t, isErr, text)
	_, isErr = call(t, s, "crm_event_get", map[string]any{"id": ev["id"]})
	assert.False(t, isErr)
	call(t, s, "crm_event_delete", map[string]any{"id": ev["id"]})
	text, isErr = call(t, s, "crm_event_delete", map[string]any{"id": ev["id"], "purge": true})
	assert.False(t, isErr, text)
}

func TestEventToolErrorsViaMCP(t *testing.T) {
	fixClock(t)
	s := setupTestServer(t)
	tests := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{"missing title", "crm_event_create", map[string]any{"start": "fri 10:00"}, ""},
		{"bad start", "crm_event_create", map[string]any{"title": "x", "start": "later"}, "validation error"},
		{"bad zone", "crm_event_create", map[string]any{"title": "x", "start": "fri 10:00", "tz": "Mars/Base"}, "validation error"},
		{"bad repeat", "crm_event_create", map[string]any{"title": "x", "start": "fri 10:00", "repeat": "now and then"}, "validation error"},
		{"bad reminder", "crm_event_create", map[string]any{"title": "x", "start": "fri 10:00", "reminders": []any{"soon"}}, "validation error"},
		{"unknown person", "crm_event_create", map[string]any{"title": "x", "start": "fri 10:00", "person_ids": []any{99}}, "not found"},
		{"get unknown", "crm_event_get", map[string]any{"id": 9}, "not found"},
		{"update unknown", "crm_event_update", map[string]any{"id": 9, "title": "x"}, "not found"},
		{"bad window", "crm_event_list", map[string]any{"from": "2026-10-20", "to": "2026-10-19"}, "validation error"},
		{"bad id", "crm_event_get", map[string]any{"id": 0}, "positive integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, isErr := call(t, s, tt.tool, tt.args)
			assert.True(t, isErr, text)
			assert.Contains(t, text, tt.want)
		})
	}
}
