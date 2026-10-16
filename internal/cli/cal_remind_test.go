package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clock: Thursday 2026-10-15 07:00 in Chicago.
func TestCalDueRemindersListsOnce(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "add", "Early call", "--start", "thu 07:10", "--duration", "20m", "--remind", "15m")
	calOK(t, db, "cal", "add", "Lunch", "--start", "thu 12:00", "--duration", "1h", "--remind", "30m", "--remind", "1d:email")
	calOK(t, db, "cal", "add", "Over", "--start", "thu 06:00", "--duration", "30m", "--remind", "15m")

	preview := calJSON(t, db, "cal", "due-reminders", "--dry-run")
	require.Len(t, preview, 2, "the call that has not started, and Lunch's reminder from a day ahead; the finished call is skipped")
	assert.Equal(t, "Early call", preview[0]["title"])
	assert.EqualValues(t, 15, preview[0]["minutes_before"])
	assert.Equal(t, "in 10m", preview[0]["starts_in"])
	assert.Equal(t, "Thu 15 Oct 07:10", preview[0]["local_start"])
	assert.Len(t, calJSON(t, db, "cal", "due-reminders", "--dry-run"), 2, "a dry run does not mark")

	first := calJSON(t, db, "cal", "due-reminders")
	require.Len(t, first, 2)
	assert.Equal(t, "email", first[1]["method"])
	assert.Empty(t, calJSON(t, db, "cal", "due-reminders"), "each reminder is listed once")

	ahead := calJSON(t, db, "cal", "due-reminders", "--within", "5h", "--dry-run")
	require.Len(t, ahead, 1)
	assert.Equal(t, "Lunch", ahead[0]["title"])
	assert.EqualValues(t, 30, ahead[0]["minutes_before"])

	_, _, code := calCrm(t, db, "cal", "due-reminders", "--within", "soon")
	assert.Equal(t, 2, code)
}

func TestBookRemindDraftsOnce(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "hours", "set", "weekdays", "09:00-17:00", "--tz", "America/Chicago")
	calOK(t, db, "book", "add", "Intro call", "--email", "me@example.com", "--notice", "1h", "--remind", "1d", "--remind", "1h", "--followup", "2h")
	calOK(t, db, "book", "confirm", "intro-call", "--at", "thu 09:00", "--name", "Jane Doe", "--email", "jane@example.com")

	// 07:00 now: the day-before reminder is due, the 1h one is not (fires 08:00).
	dry := calJSON(t, db, "book", "remind", "--dry-run")
	require.Len(t, dry, 1)
	assert.Equal(t, "reminder", dry[0]["kind"])
	assert.EqualValues(t, 1440, dry[0]["minutes_before"])
	assert.Equal(t, "jane@example.com", dry[0]["to"])
	assert.Contains(t, dry[0]["body"], "reply to this email or write to me@example.com")
	assert.Len(t, calJSON(t, db, "book", "remind", "--dry-run"), 1)

	text := calOK(t, db, "book", "remind", "-f", "table")
	assert.Contains(t, text, "To: jane@example.com")
	assert.Contains(t, text, "Subject: Reminder: Intro call, Thu 15 Oct 09:00 CDT")
	assert.Empty(t, calJSON(t, db, "book", "remind"), "drafted once")
}
