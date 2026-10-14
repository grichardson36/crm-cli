package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonRows(t *testing.T, out string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows), out)
	return rows
}

func TestPersonContactMethodsAndShow(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	_, _, code := crm(t, db, "person", "add", "Jane Smith", "--email", "jane@work.com", "--birthday", "1990-03-14", "--keep-in-touch", "3 mons", "--favorite")
	require.Equal(t, 0, code)

	_, _, code = crm(t, db, "person", "email", "add", "1", "jane@home.com", "--label", "Home")
	assert.Equal(t, 0, code)
	_, _, code = crm(t, db, "person", "phone", "add", "1", "+15551112222")
	assert.Equal(t, 0, code)
	out, _, code := crm(t, db, "person", "handle", "set", "1", "linkedin", "https://linkedin.com/in/jane", "-f", "json")
	require.Equal(t, 0, code)

	row := jsonRows(t, out)[0]
	assert.Equal(t, "1990-03-14", row["birthday"])
	assert.Equal(t, "3 mons", row["keep_in_touch"])
	assert.Equal(t, true, row["is_favorite"])
	assert.Len(t, row["emails"], 2)
	assert.Len(t, row["phones"], 1)
	assert.Len(t, row["handles"], 1)
	assert.NotContains(t, row, "emails_text", "JSON output carries arrays, not the table text")

	// Table output shows all emails in one cell.
	out, _, code = crm(t, db, "person", "show", "1", "-f", "table")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "jane@work.com, jane@home.com")

	// Removing and bad input.
	_, _, code = crm(t, db, "person", "email", "rm", "1", "jane@home.com")
	assert.Equal(t, 0, code)
	_, _, code = crm(t, db, "person", "email", "rm", "1", "jane@home.com")
	assert.Equal(t, 3, code, "removing a missing email is not-found")
	_, _, code = crm(t, db, "person", "handle", "set", "1", "myspace", "x")
	assert.Equal(t, 2, code, "bad handle kind is a validation error")
	_, _, code = crm(t, db, "person", "edit", "1", "--keep-in-touch", "weekly")
	assert.Equal(t, 2, code)

	// Context includes the details.
	out, _, code = crm(t, db, "context", "1", "-f", "json")
	require.Equal(t, 0, code)
	var ctx map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &ctx))
	person := ctx["person"].(map[string]any)
	assert.Equal(t, "1990-03-14", person["birthday"])
	assert.NotEmpty(t, person["handles"])
}

func TestDueAndContacted(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	crm(t, db, "person", "add", "Overdue Olive", "--keep-in-touch", "7 days")
	crm(t, db, "person", "add", "No Cadence")
	_, _, code := crm(t, db, "person", "edit", "1", "--last-contacted", "2020-01-01")
	require.Equal(t, 0, code)

	out, _, code := crm(t, db, "due", "-f", "json")
	require.Equal(t, 0, code)
	rows := jsonRows(t, out)
	require.Len(t, rows, 1)
	assert.Equal(t, "Overdue Olive", rows[0]["name"])

	// Marking contacted resets the clock.
	_, _, code = crm(t, db, "person", "contacted", "1")
	require.Equal(t, 0, code)
	out, _, _ = crm(t, db, "due", "-f", "json")
	assert.Empty(t, jsonRows(t, out))
	out, _, _ = crm(t, db, "due", "--within", "30", "-f", "json")
	assert.Len(t, jsonRows(t, out), 1)

	// Snoozing hides an overdue person.
	crm(t, db, "person", "edit", "1", "--last-contacted", "2020-01-01")
	_, _, code = crm(t, db, "person", "edit", "1", "--snooze-until", "2999-01-01")
	require.Equal(t, 0, code)
	out, _, _ = crm(t, db, "due", "-f", "json")
	assert.Empty(t, jsonRows(t, out))
}

func TestBirthdaysCommand(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	crm(t, db, "person", "add", "Far Fred", "--birthday", "--01-01")
	out, _, code := crm(t, db, "birthdays", "--days", "366", "-f", "json")
	require.Equal(t, 0, code)
	rows := jsonRows(t, out)
	require.Len(t, rows, 1)
	assert.Equal(t, "Far Fred", rows[0]["name"])
	out, _, _ = crm(t, db, "birthdays", "--days", "0", "-f", "json")
	assert.LessOrEqual(t, len(jsonRows(t, out)), 1)
}

func TestRecurringTaskWithTwoPeople(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	crm(t, db, "person", "add", "Ann")
	crm(t, db, "person", "add", "Ben")
	out, _, code := crm(t, db, "task", "add", "Quarterly sync", "--person", "1", "--person", "2", "--due", "2026-01-15", "--recurrence", "3 mons", "-f", "json")
	require.Equal(t, 0, code)
	row := jsonRows(t, out)[0]
	assert.Equal(t, "3 mons", row["recurrence"])
	assert.Len(t, row["person_ids"], 2)

	for _, pid := range []string{"1", "2"} {
		out, _, _ = crm(t, db, "task", "list", "--person", pid, "-f", "json")
		assert.Len(t, jsonRows(t, out), 1, "task is listed under person %s", pid)
	}

	_, stderr, code := crm(t, db, "task", "done", "1")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "Next occurrence: task #2")
	out, _, _ = crm(t, db, "task", "list", "-f", "json")
	rows := jsonRows(t, out)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 2, rows[0]["id"])

	_, _, code = crm(t, db, "task", "add", "bad", "--recurrence", "weekly")
	assert.Equal(t, 2, code, "recurrence needs a due date")
}

func TestBackupCommand(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "crm.db")
	crm(t, db, "person", "add", "Jane Smith")
	dest := filepath.Join(dir, "snapshot.db")

	_, _, code := crm(t, db, "backup", dest)
	require.Equal(t, 0, code)
	out, _, code := crm(t, dest, "person", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Len(t, jsonRows(t, out), 1)

	_, _, code = crm(t, db, "backup", dest)
	assert.Equal(t, 4, code, "refuses to overwrite")
}
