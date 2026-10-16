package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The clock in calCrm is Thursday 2026-10-15 07:00 in Chicago.
func TestCalFreeSlots(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "hours", "set", "weekdays", "09:00-17:00", "--tz", "America/Chicago")
	calOK(t, db, "cal", "add", "Planning", "--start", "2026-10-15 10:00", "--duration", "1h")
	calOK(t, db, "cal", "add", "Standup", "--start", "2026-10-16 09:00", "--duration", "30m", "--repeat", "weekdays")

	slots := calJSON(t, db, "cal", "free", "--len", "30m", "--within", "2d", "--limit", "6")
	require.Len(t, slots, 6)
	times := []string{}
	for _, s := range slots {
		times = append(times, s["date"].(string)+" "+s["time"].(string))
	}
	assert.Equal(t, []string{
		"Thu 15 Oct 09:00-09:30", "Thu 15 Oct 09:15-09:45", // 10 minute buffer before the 10:00 meeting
		"Thu 15 Oct 11:15-11:45", "Thu 15 Oct 11:30-12:00", "Thu 15 Oct 11:45-12:15", "Thu 15 Oct 12:00-12:30",
	}, times)
	assert.Equal(t, "2026-10-15T14:00:00Z", slots[0]["start"])
	assert.Equal(t, "2026-10-15T09:00:00-05:00", slots[0]["local_start"])

	// Settings can be overridden per call.
	slots = calJSON(t, db, "cal", "free", "--len", "1h", "--within", "1d", "--buffer", "0", "--step", "60", "--limit", "3")
	first := []string{}
	for _, s := range slots {
		first = append(first, s["time"].(string))
	}
	assert.Equal(t, []string{"09:00-10:00", "11:00-12:00", "12:00-13:00"}, first)

	// The next day starts after the standup and its buffer.
	next := calJSON(t, db, "cal", "free", "--len", "30m", "--from", "2026-10-16", "--within", "1", "--limit", "1")
	assert.Equal(t, "09:45-10:15", next[0]["time"])

	// Minimum notice from now (07:00).
	late := calJSON(t, db, "cal", "free", "--len", "30m", "--notice", "4h", "--limit", "1")
	assert.Equal(t, "11:15-11:45", late[0]["time"], "07:00 plus 4h is 11:00, and the meeting buffer ends at 11:10")

	// A day off has nothing.
	calOK(t, db, "cal", "hours", "override", "2026-10-15", "off")
	off := calJSON(t, db, "cal", "free", "--len", "30m", "--within", "1d")
	assert.Empty(t, off)

	table := calOK(t, db, "cal", "free", "--len", "30m", "--limit", "2", "-f", "table")
	assert.Contains(t, table, "TIME")

	for _, args := range [][]string{
		{"cal", "free", "--len", "soon"},
		{"cal", "free", "--within", "0.5d"},
		{"cal", "free", "--notice", "soon"},
		{"cal", "free", "--rule", "nope"},
		{"cal", "free", "--step", "0"},
	} {
		_, _, code := calCrm(t, db, append(args, "-f", "json")...)
		assert.NotEqual(t, 0, code, args)
	}
}
