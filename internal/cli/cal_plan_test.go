package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clock: Thursday 2026-10-15 07:00 in Chicago, hours Mon-Fri 09:00-17:00.
func planDB(t *testing.T) string {
	t.Helper()
	db := newCalDB(t)
	calOK(t, db, "cal", "hours", "set", "weekdays", "09:00-17:00", "--tz", "America/Chicago")
	calOK(t, db, "task", "add", "Write report", "--duration", "2h", "--deadline", "2026-10-16", "--auto-schedule")
	calOK(t, db, "task", "add", "Review notes", "--duration", "1h", "--auto-schedule", "--priority", "high")
	calOK(t, db, "task", "add", "No length", "--auto-schedule")
	calOK(t, db, "task", "add", "Manual", "--duration", "1h")
	return db
}

func planOut(t *testing.T, db string, args ...string) map[string]any {
	t.Helper()
	var out []map[string]any
	jsonOut(t, calOK(t, db, append([]string{"cal", "plan"}, append(args, "-f", "json")...)...), &out)
	require.Len(t, out, 1)
	return out[0]
}

func TestCalPlanProposalWritesNothing(t *testing.T) {
	db := planDB(t)
	out := planOut(t, db)
	assert.Equal(t, "proposal", out["mode"])
	assert.Equal(t, false, out["applied"])
	tasks := out["tasks"].([]any)
	require.Len(t, tasks, 3, "the manual task is not planned")
	first := tasks[0].(map[string]any)
	assert.Equal(t, "Write report", first["task"], "the dated task goes first")
	assert.Equal(t, "placed", first["decision"])
	assert.Equal(t, "2026-10-15T14:00:00Z", first["blocks"].([]any)[0].(map[string]any)["start"])
	assert.NotEmpty(t, first["why"])
	sum := out["summary"].(map[string]any)
	assert.EqualValues(t, 2, sum["placed"])
	assert.EqualValues(t, 1, sum["skipped_no_duration"])

	for _, row := range calJSON(t, db, "task", "list") {
		assert.Nil(t, row["scheduled_start"], "a proposal writes nothing")
	}
	table := calOK(t, db, "cal", "plan", "-f", "table")
	assert.Contains(t, table, "Write report")
	assert.Contains(t, table, "placed")
}

func TestCalPlanApplyDryRunUndo(t *testing.T) {
	db := planDB(t)
	preview := planOut(t, db, "--apply", "--dry-run")
	assert.Equal(t, "apply preview", preview["mode"])
	diff := preview["diff"].([]any)
	require.Len(t, diff, 2)
	assert.Equal(t, "Write report", diff[0].(map[string]any)["task"])
	assert.NotEmpty(t, diff[0].(map[string]any)["new_start"])
	for _, row := range calJSON(t, db, "task", "list") {
		assert.Nil(t, row["scheduled_start"], "the preview writes nothing")
	}

	applied := planOut(t, db, "--apply")
	assert.Equal(t, true, applied["applied"])
	assert.EqualValues(t, 1, applied["journal_id"])
	starts := map[string]any{}
	for _, row := range calJSON(t, db, "task", "list") {
		starts[row["title"].(string)] = row["scheduled_start"]
		if row["title"] != "No length" && row["title"] != "Manual" {
			assert.Equal(t, false, row["scheduled_locked"])
		}
	}
	assert.Equal(t, "2026-10-15T14:00:00Z", starts["Write report"])
	assert.NotNil(t, starts["Review notes"])
	assert.Nil(t, starts["Manual"])

	// The scheduled blocks are busy time for find-a-time once locked, and show in the calendar list of tasks.
	again, _, code := calCrm(t, db, "cal", "plan", "--apply", "-f", "json")
	assert.Equal(t, 4, code, "applying an unchanged plan is a conflict: %s", again)

	hist := calJSON(t, db, "cal", "plan", "--history")
	require.Len(t, hist, 1)
	assert.Equal(t, false, hist[0]["undone"])

	undone := calJSON(t, db, "cal", "plan", "--undo")
	assert.Equal(t, true, undone[0]["undone"])
	for _, row := range calJSON(t, db, "task", "list") {
		assert.Nil(t, row["scheduled_start"])
	}
	_, _, code = calCrm(t, db, "cal", "plan", "--undo")
	assert.Equal(t, 3, code)
}

func TestCalPlanOnlyRejectLockAndFlags(t *testing.T) {
	db := planDB(t)
	only := planOut(t, db, "--only", "2")
	assert.Len(t, only["tasks"], 1)

	planOut(t, db, "--apply", "--reject", "1", "--lock", "2")
	rows := map[string]map[string]any{}
	for _, row := range calJSON(t, db, "task", "list") {
		rows[row["title"].(string)] = row
	}
	assert.Nil(t, rows["Write report"]["scheduled_start"])
	assert.Equal(t, true, rows["Review notes"]["scheduled_locked"])

	_, _, code := calCrm(t, db, "cal", "plan", "--now", "yesterday")
	assert.Equal(t, 2, code)
	_, _, code = calCrm(t, db, "cal", "plan", "--horizon", "soon")
	assert.Equal(t, 2, code)
	h := planOut(t, db, "--horizon", "1d", "--now", "2026-10-15T13:00:00Z")
	assert.Equal(t, "2026-10-16T13:00:00Z", h["horizon_end"])
}
