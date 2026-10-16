package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalHoursDefaultsSetOverrideConfig(t *testing.T) {
	db := newCalDB(t)

	show := calJSON(t, db, "cal", "hours", "show")
	require.Len(t, show, 1)
	assert.Equal(t, "work", show[0]["name"])
	assert.Equal(t, "America/Chicago", show[0]["tz"], "the device zone")
	assert.Equal(t, true, show[0]["default"])
	cfg := show[0]["config"].(map[string]any)
	assert.EqualValues(t, 10, cfg["buffer_min"])
	assert.EqualValues(t, 15, cfg["step_min"])
	assert.EqualValues(t, 14, cfg["horizon_days"])
	weekly := show[0]["weekly"].(map[string]any)
	assert.Len(t, weekly["mon"], 1)
	assert.NotContains(t, weekly, "sat")

	set := calJSON(t, db, "cal", "hours", "set", "mon,wed", "08:00-12:00,13:00-16:00", "--tz", "America/Denver")
	assert.Equal(t, "America/Denver", set[0]["tz"])
	assert.Len(t, set[0]["weekly"].(map[string]any)["wed"], 2)
	assert.Nil(t, set[0]["default"])
	closed := calJSON(t, db, "cal", "hours", "set", "fri", "off")
	assert.NotContains(t, closed[0]["weekly"], "fri")
	calOK(t, db, "cal", "hours", "set", "sat", "10:00-14:00")

	ov := calJSON(t, db, "cal", "hours", "override", "2026-12-24", "09:00-12:00")
	assert.Contains(t, ov[0]["overrides"], "2026-12-24")
	calOK(t, db, "cal", "hours", "override", "2026-12-25", "off")
	rm := calJSON(t, db, "cal", "hours", "override", "2026-12-24", "--remove")
	assert.NotContains(t, rm[0]["overrides"], "2026-12-24")
	assert.Contains(t, rm[0]["overrides"], "2026-12-25")

	table := calOK(t, db, "cal", "hours", "show", "-f", "table")
	assert.Contains(t, table, "mon")
	assert.Contains(t, table, "08:00-12:00, 13:00-16:00")
	assert.Contains(t, table, "2026-12-25")
	assert.Contains(t, table, "off")

	conf := calJSON(t, db, "cal", "hours", "config", "--buffer", "5", "--step", "30", "--horizon", "21")
	assert.EqualValues(t, 5, conf[0]["buffer_min"])
	assert.EqualValues(t, 30, conf[0]["step_min"])
	assert.EqualValues(t, 21, conf[0]["horizon_days"])
	conf = calJSON(t, db, "cal", "hours", "config")
	assert.EqualValues(t, 21, conf[0]["horizon_days"], "stored")
	_, _, code := calCrm(t, db, "cal", "hours", "config", "--step", "0")
	assert.Equal(t, 2, code)
}

func TestCalHoursExpandFollowsDaylightSaving(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "hours", "set", "weekdays", "09:00-17:00", "--tz", "America/Chicago")
	list := calJSON(t, db, "cal", "hours", "show", "--expand", "2026-10-30")
	require.GreaterOrEqual(t, len(list), 3)
	assert.Equal(t, "2026-10-30T14:00:00Z", list[0]["start"])
	assert.Equal(t, "2026-11-02T15:00:00Z", list[1]["start"], "Monday after the clocks went back")
	assert.Equal(t, "09:00-17:00", list[1]["hours"])
	assert.Equal(t, "Mon 2 Nov", list[1]["date"])
}

func TestCalHoursErrors(t *testing.T) {
	db := newCalDB(t)
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"bad days", []string{"cal", "hours", "set", "funday", "09:00-17:00"}, 2},
		{"bad hours", []string{"cal", "hours", "set", "mon", "nine-five"}, 2},
		{"overlap", []string{"cal", "hours", "set", "mon", "09:00-13:00,12:00-15:00"}, 2},
		{"bad zone", []string{"cal", "hours", "set", "mon", "09:00-17:00", "--tz", "Mars/Base"}, 2},
		{"override needs hours", []string{"cal", "hours", "override", "2026-12-24"}, 2},
		{"bad date", []string{"cal", "hours", "override", "christmas", "off"}, 2},
		{"remove missing", []string{"cal", "hours", "override", "2026-12-24", "--remove"}, 3},
		{"unknown rule", []string{"cal", "hours", "show", "--name", "nope"}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := calCrm(t, db, append(tt.args, "-f", "json")...)
			assert.Equal(t, tt.code, code, stderr)
		})
	}
	var plan map[string]any
	jsonOut(t, calOK(t, db, "cal", "hours", "set", "mon", "off", "--dry-run", "-f", "json"), &plan)
	assert.Equal(t, true, plan["dry_run"])
	after := calJSON(t, db, "cal", "hours", "show")
	assert.Equal(t, true, after[0]["default"], "a dry run stores nothing")
}

func TestTaskSchedulingFlags(t *testing.T) {
	db := newCalDB(t)
	added := calJSON(t, db, "task", "add", "Write report", "--due", "2026-11-10", "--duration", "2h", "--earliest-start", "2026-11-02",
		"--deadline", "fri 17:00", "--hard-deadline", "--min-chunk", "30m", "--auto-schedule", "--schedule-name", "deep")
	require.Len(t, added, 1)
	task := added[0]
	assert.EqualValues(t, 120, task["duration_min"])
	assert.Equal(t, "2026-11-02", task["earliest_start"])
	assert.Equal(t, "2026-10-16T22:00:00Z", task["deadline_at"], "fri 17:00 in Chicago")
	assert.Equal(t, true, task["deadline_hard"])
	assert.EqualValues(t, 30, task["min_chunk_min"])
	assert.Equal(t, true, task["auto_schedule"])
	assert.Equal(t, "deep", task["schedule_name"])
	assert.Equal(t, "2026-11-10", task["due_at"])

	// An auto-scheduled task without a duration is allowed: the planner uses 30 minutes.
	noLen := calJSON(t, db, "task", "add", "Plan trip", "--auto-schedule")
	assert.Nil(t, noLen[0]["duration_min"])

	// Placing by hand locks; the lock can be released.
	placed := calJSON(t, db, "task", "edit", "2", "--duration", "45m", "--scheduled-start", "mon 14:00")
	assert.Equal(t, "2026-10-19T19:00:00Z", placed[0]["scheduled_start"])
	assert.Equal(t, true, placed[0]["scheduled_locked"])
	unlocked := calJSON(t, db, "task", "edit", "2", "--lock=false")
	assert.Equal(t, false, unlocked[0]["scheduled_locked"])
	cleared := calJSON(t, db, "task", "edit", "2", "--scheduled-start", "", "--duration", "")
	assert.Nil(t, cleared[0]["scheduled_start"])
	assert.Nil(t, cleared[0]["duration_min"])

	// Unchanged flags leave the fields alone.
	again := calJSON(t, db, "task", "edit", "1", "--priority", "low")
	assert.EqualValues(t, 120, again[0]["duration_min"])

	for _, args := range [][]string{
		{"task", "add", "x", "--duration", "soon"},
		{"task", "add", "x", "--deadline", "whenever"},
		{"task", "add", "x", "--scheduled-start", "2026-11-03"},
		{"task", "edit", "1", "--min-chunk", "abc"},
	} {
		_, _, code := calCrm(t, db, append(args, "-f", "json")...)
		assert.Equal(t, 2, code, args)
	}
	table := calOK(t, db, "task", "list", "-f", "table")
	assert.Contains(t, table, "LENGTH")
}
