package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clock: Thursday 2026-10-15 07:00 in Chicago. Hours Mon-Fri 09:00-17:00.
func bookDB(t *testing.T) string {
	t.Helper()
	db := newCalDB(t)
	calOK(t, db, "cal", "hours", "set", "weekdays", "09:00-17:00", "--tz", "America/Chicago")
	calOK(t, db, "book", "add", "Intro call", "--email", "me@example.com", "--len", "30m,45m", "--notice", "4h", "--horizon", "7",
		"--buffer", "10", "--max-day", "3", "--where", "https://meet.example.com/room", "--question", "Agenda!", "--remind", "1h", "--followup", "1d")
	return db
}

func TestBookAddListAndDryRun(t *testing.T) {
	db := bookDB(t)
	links := calJSON(t, db, "book", "list")
	require.Len(t, links, 1)
	l := links[0]
	assert.Equal(t, "intro-call", l["slug"])
	assert.Equal(t, "30,45", l["durations"])
	assert.EqualValues(t, 240, l["min_notice_min"])
	assert.EqualValues(t, 10, l["buffer_before_min"])
	assert.EqualValues(t, 10, l["buffer_after_min"])
	assert.EqualValues(t, 3, l["max_per_day"])
	assert.Equal(t, "url", l["location_kind"])
	assert.EqualValues(t, 60, l["reminders_min"].([]any)[0])

	stdout, stderr, code := calCrm(t, db, "book", "add", "Other", "--dry-run", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, `"dry_run":true`)
	assert.Len(t, calJSON(t, db, "book", "list"), 1, "a dry run writes nothing")

	_, stderr, code = calCrm(t, db, "book", "add", "Intro call")
	assert.Equal(t, 4, code, "same slug is a conflict: "+stderr)
	_, _, code = calCrm(t, db, "book", "add", "Bad", "--len", "soon")
	assert.Equal(t, 2, code)
	_, _, code = calCrm(t, db, "book", "offer", "nope")
	assert.Equal(t, 3, code)
}

func TestBookOfferTextAndJSON(t *testing.T) {
	db := bookDB(t)
	text := calOK(t, db, "book", "offer", "intro-call", "--count", "4", "--tz", "America/New_York")
	assert.Equal(t, "Intro call (30 min)\nWhere: https://meet.example.com/room\nTimes are in America/Chicago.\nYour time (America/New_York) is in brackets.\n\n"+
		"Thu 15 Oct: 11:00 [12:00], 11:30 [12:30], 12:00 [13:00], 12:30 [13:30]\n\nReply with the time you want.\n", text)

	out := calJSON(t, db, "book", "offer", "intro-call", "--count", "2", "--len", "45m")
	require.Len(t, out, 1)
	assert.EqualValues(t, 45, out[0]["length_min"])
	slots := out[0]["slots"].([]any)
	require.Len(t, slots, 2)
	assert.Equal(t, "2026-10-15T16:00:00Z", slots[0].(map[string]any)["start"])
	_, _, code := calCrm(t, db, "book", "offer", "intro-call", "--len", "20m")
	assert.Equal(t, 2, code, "a length the link does not offer")
}

func TestBookExportWritesAPageWithoutEventTitles(t *testing.T) {
	db := bookDB(t)
	calOK(t, db, "cal", "add", "Private dentist visit", "--start", "fri 10:00", "--duration", "1h")
	dir := filepath.Join(t.TempDir(), "site")
	out := calJSON(t, db, "book", "export", "intro-call", "--out", dir)
	assert.Equal(t, filepath.Join(dir, "index.html"), out[0]["file"])
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	require.NoError(t, err)
	page := string(data)
	assert.Contains(t, page, "mailto:me@example.com")
	assert.Contains(t, page, "Friday 16 October")
	assert.NotContains(t, strings.ToLower(page), "dentist")

	db2 := newCalDB(t)
	calOK(t, db2, "book", "add", "No host")
	_, _, code := calCrm(t, db2, "book", "export", "no-host")
	assert.Equal(t, 2, code, "mail links need your address")
}

func TestBookConfirmCancelAndInvite(t *testing.T) {
	db := bookDB(t)
	calOK(t, db, "person", "add", "Jane Doe", "--email", "jane@example.com")
	invite := filepath.Join(t.TempDir(), "invite.ics")

	_, stderr, code := calCrm(t, db, "book", "confirm", "intro-call", "--at", "fri 10:00", "--email", "jane@example.com")
	assert.Equal(t, 2, code, "the required question: "+stderr)

	dry, stderr, code := calCrm(t, db, "book", "confirm", "intro-call", "--at", "fri 10:00", "--name", "Jane Doe", "--email", "jane@example.com",
		"--answer", "Agenda=pricing", "--dry-run", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, dry, `"dry_run":true`)
	assert.Empty(t, calJSON(t, db, "book", "list", "--bookings"))

	// The slot line from the invitee's mail works as the argument.
	res := calJSON(t, db, "book", "confirm", "intro-call/2026-10-16T15:00:00Z/30", "--name", "Jane Doe", "--email", "Jane@example.com",
		"--answer", "Agenda=pricing", "--follow-up", "--invite", invite)
	require.Len(t, res, 1)
	b := res[0]["booking"].(map[string]any)
	assert.Equal(t, "2026-10-16T15:00:00Z", b["start"])
	assert.Equal(t, "Jane Doe", res[0]["person"])
	assert.Equal(t, "Intro call with Jane Doe", res[0]["event"].(map[string]any)["title"])

	data, err := os.ReadFile(invite)
	require.NoError(t, err)
	assert.Contains(t, string(data), "METHOD:REQUEST")

	events := calJSON(t, db, "cal", "list", "--days", "7")
	require.Len(t, events, 1)
	assert.Equal(t, "Intro call with Jane Doe", events[0]["title"])
	logs := calJSON(t, db, "log", "list")
	require.NotEmpty(t, logs)
	assert.Equal(t, "meeting", logs[0]["type"])
	tasks := calJSON(t, db, "task", "list")
	require.Len(t, tasks, 1)
	assert.Equal(t, "Follow up: Intro call with Jane Doe", tasks[0]["title"])

	// Taken, and inside the buffer.
	for _, at := range []string{"fri 10:00", "fri 10:30"} {
		_, _, code = calCrm(t, db, "book", "confirm", "intro-call", "--at", at, "--email", "bob@example.com", "--answer", "Agenda=x")
		assert.Equal(t, 4, code, at)
	}
	offered := calOK(t, db, "book", "offer", "intro-call", "--count", "0")
	assert.Contains(t, offered, "Fri 16 Oct: 09:00, 11:00,", "10:00 is booked and 10:30 is inside the buffer")
	bookings := calJSON(t, db, "book", "list", "--bookings")
	require.Len(t, bookings, 1)

	cancel := filepath.Join(t.TempDir(), "cancel.ics")
	_, _, code = calCrm(t, db, "book", "cancel", "1", "--dry-run")
	require.Equal(t, 0, code)
	require.Len(t, calJSON(t, db, "cal", "list", "--days", "7"), 1)
	calOK(t, db, "book", "cancel", "1", "--invite", cancel)
	data, err = os.ReadFile(cancel)
	require.NoError(t, err)
	assert.Contains(t, string(data), "METHOD:CANCEL")
	assert.Empty(t, calJSON(t, db, "cal", "list", "--days", "7"))
	assert.Empty(t, calJSON(t, db, "book", "list", "--bookings"))
	assert.Len(t, calJSON(t, db, "book", "list", "--bookings", "--all"), 1)
	_, _, code = calCrm(t, db, "book", "cancel", "1")
	assert.Equal(t, 4, code)

	calOK(t, db, "book", "pause", "intro-call")
	assert.Contains(t, calOK(t, db, "book", "offer", "intro-call"), "No times are open")
	calOK(t, db, "book", "resume", "intro-call")
	calOK(t, db, "book", "rm", "intro-call")
	assert.Empty(t, calJSON(t, db, "book", "list"))
}

func TestCalInvite(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "add", "Lunch", "--start", "fri 12:30", "--duration", "1h")
	out := calOK(t, db, "cal", "invite", "1", "--to", "sam@example.com", "--from", "me@example.com")
	unfolded := strings.ReplaceAll(out, "\r\n ", "")
	for _, want := range []string{"METHOD:REQUEST", "SUMMARY:Lunch", "mailto:me@example.com", "mailto:sam@example.com"} {
		assert.Contains(t, unfolded, want)
	}
	_, _, code := calCrm(t, db, "cal", "invite", "1")
	assert.Equal(t, 2, code, "needs --to")
	_, _, code = calCrm(t, db, "cal", "invite", "9", "--to", "a@b.c")
	assert.Equal(t, 3, code)
}
