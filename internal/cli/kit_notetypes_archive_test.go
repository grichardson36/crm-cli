package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	crmdb "github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func typeNames(t *testing.T, dbPath string, extra ...string) map[string]bool {
	t.Helper()
	args := append([]string{"log", "types", "-f", "json"}, extra...)
	out, _, code := crm(t, dbPath, args...)
	require.Equal(t, 0, code)
	names := map[string]bool{}
	for _, r := range jsonRows(t, out) {
		names[r["name"].(string)] = r["counts_as_contact"].(bool)
	}
	return names
}

func TestLogTypes_ListAddRemove(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	assert.Equal(t, map[string]bool{"call": true, "email": true, "meeting": true, "message": true, "note": false}, typeNames(t, dbPath))

	_, _, code := crm(t, dbPath, "log", "types", "add", "standup")
	assert.Equal(t, 0, code)
	_, _, code = crm(t, dbPath, "log", "types", "add", "journal", "--counts-as-contact=false")
	assert.Equal(t, 0, code)
	names := typeNames(t, dbPath)
	assert.True(t, names["standup"])
	assert.False(t, names["journal"])

	_, _, code = crm(t, dbPath, "log", "types", "add", "STANDUP")
	assert.Equal(t, 4, code, "duplicate is a conflict")
	_, _, code = crm(t, dbPath, "log", "types", "add", "bad name")
	assert.Equal(t, 2, code)

	_, _, code = crm(t, dbPath, "log", "types", "rm", "standup")
	assert.Equal(t, 0, code)
	assert.NotContains(t, typeNames(t, dbPath), "standup")
	assert.Contains(t, typeNames(t, dbPath, "--all"), "standup")
	_, _, code = crm(t, dbPath, "log", "types", "rm", "nope")
	assert.Equal(t, 3, code)
}

func TestLogAdd_GenericPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	crm(t, dbPath, "log", "types", "add", "standup")

	out, _, code := crm(t, dbPath, "log", "add", "--type", "StandUp", "1", "Talked", "about", "pelicans", "-f", "json")
	require.Equal(t, 0, code)
	row := jsonRows(t, out)[0]
	assert.Equal(t, "standup", row["type"])
	assert.Equal(t, "Talked about pelicans", row["content"])

	_, _, code = crm(t, dbPath, "log", "add", "--type", "unknown", "1", "x")
	assert.Equal(t, 2, code, "unknown type")
	_, _, code = crm(t, dbPath, "log", "add", "1", "x")
	assert.Equal(t, 2, code, "--type is required")
	_, _, code = crm(t, dbPath, "log", "add", "--type", "standup", "1", "text", "--content", "other")
	assert.Equal(t, 2, code, "text and --content conflict")

	// Shortcuts keep working, and an archived type is refused.
	_, _, code = crm(t, dbPath, "log", "call", "1", "--subject", "Hi")
	assert.Equal(t, 0, code)
	crm(t, dbPath, "log", "types", "rm", "standup")
	_, _, code = crm(t, dbPath, "log", "add", "--type", "standup", "1", "x")
	assert.Equal(t, 2, code, "archived type")
	out, _, _ = crm(t, dbPath, "log", "list", "--type", "standup", "-f", "json")
	assert.Len(t, jsonRows(t, out), 1, "existing interactions stay")
}

func TestKeepInTouch_FirstDueUsesSetAt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Old Olive", "--keep-in-touch", "7 days")

	d, err := crmdb.Open(dbPath)
	require.NoError(t, err)
	_, err = d.Exec(`UPDATE people SET created_at = '2015-01-01 00:00:00'`)
	require.NoError(t, err)
	require.NoError(t, d.Close())

	out, _, code := crm(t, dbPath, "due", "-f", "json")
	require.Equal(t, 0, code)
	assert.Empty(t, jsonRows(t, out), "old created_at does not make a freshly set cadence overdue")

	d, err = crmdb.Open(dbPath)
	require.NoError(t, err)
	_, err = d.Exec(`UPDATE people SET keep_in_touch_set_at = '2020-01-01 00:00:00'`)
	require.NoError(t, err)
	require.NoError(t, d.Close())
	out, _, _ = crm(t, dbPath, "due", "-f", "json")
	assert.Len(t, jsonRows(t, out), 1)
}

func TestPersonArchiveRestorePurge(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Ann Archive", "--email", "ann@x.com", "--birthday", "--01-01", "--keep-in-touch", "7 days")
	crm(t, dbPath, "person", "add", "Bob Stays")
	crm(t, dbPath, "person", "edit", "1", "--last-contacted", "2020-01-01")

	_, _, code := crm(t, dbPath, "person", "purge", "1", "--yes")
	assert.Equal(t, 4, code, "purge needs an archived person")
	_, _, code = crm(t, dbPath, "person", "archive", "1")
	assert.Equal(t, 0, code)

	out, _, _ := crm(t, dbPath, "person", "list", "-f", "json")
	assert.Len(t, jsonRows(t, out), 1)
	out, _, _ = crm(t, dbPath, "person", "list", "--archived", "-f", "json")
	require.Len(t, jsonRows(t, out), 1)
	assert.Equal(t, "Ann Archive", jsonRows(t, out)[0]["name"])
	out, _, _ = crm(t, dbPath, "search", "Ann", "-f", "json")
	assert.NotContains(t, out, "Ann Archive")
	out, _, _ = crm(t, dbPath, "due", "--within", "100000", "-f", "json")
	assert.Empty(t, jsonRows(t, out))
	out, _, _ = crm(t, dbPath, "birthdays", "--days", "366", "-f", "json")
	assert.Empty(t, jsonRows(t, out))

	_, _, code = crm(t, dbPath, "person", "restore", "1")
	assert.Equal(t, 0, code)
	out, _, _ = crm(t, dbPath, "due", "-f", "json")
	assert.Len(t, jsonRows(t, out), 1, "restored person is due again")
	_, _, code = crm(t, dbPath, "person", "restore", "1")
	assert.Equal(t, 3, code, "already active")

	// Several ids, with the deprecated alias.
	_, stderr, code := crm(t, dbPath, "person", "delete", "1", "2")
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "Deleted person #1")
	out, _, _ = crm(t, dbPath, "person", "list", "--archived", "-f", "json")
	assert.Len(t, jsonRows(t, out), 2)

	_, _, code = crm(t, dbPath, "person", "purge", "1")
	assert.Equal(t, 2, code, "purge needs --yes")
	_, _, code = crm(t, dbPath, "person", "purge", "1", "2", "--yes")
	assert.Equal(t, 0, code)
	out, _, _ = crm(t, dbPath, "person", "list", "--archived", "-f", "json")
	assert.Empty(t, jsonRows(t, out))

	d, err := crmdb.Open(dbPath)
	require.NoError(t, err)
	defer d.Close()
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM tombstones WHERE table_name = 'people'`).Scan(&n))
	assert.Equal(t, 2, n)
}

func TestStatus_DeviceID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	out, _, code := crm(t, dbPath, "status", "-f", "json")
	require.Equal(t, 0, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	id, _ := rows[0]["device_id"].(string)
	assert.Len(t, id, 36)

	out, _, _ = crm(t, dbPath, "status", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	assert.Equal(t, id, rows[0]["device_id"], "stable across runs")
}
