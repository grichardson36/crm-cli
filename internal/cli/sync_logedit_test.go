package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogEditAndRm(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "-f", "json")
	crm(t, dbPath, "log", "call", "1", "--subject", "Intro", "--content", "talked pelicans", "-f", "json")

	out, _, code := crm(t, dbPath, "log", "edit", "1", "--type", "email", "--content", "talked herons", "--date", "2026-02-03", "-f", "json")
	require.Equal(t, 0, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	assert.Equal(t, "email", rows[0]["type"])
	assert.Equal(t, "talked herons", rows[0]["content"])
	assert.Equal(t, "2026-02-03 00:00:00", rows[0]["occurred_at"])
	assert.Equal(t, "Intro", rows[0]["subject"], "untouched fields stay")

	out, _, _ = crm(t, dbPath, "search", "herons", "-f", "json")
	assert.Contains(t, out, `"interaction"`)
	out, _, _ = crm(t, dbPath, "search", "pelicans", "-f", "json")
	assert.NotContains(t, out, `"interaction"`, "FTS follows the edit")

	_, _, code = crm(t, dbPath, "log", "edit", "1")
	assert.Equal(t, 2, code, "nothing to change")
	_, _, code = crm(t, dbPath, "log", "edit", "1", "--type", "nonsense")
	assert.Equal(t, 2, code)
	_, _, code = crm(t, dbPath, "log", "edit", "1", "--date", "last tuesday-ish")
	assert.Equal(t, 2, code)
	_, _, code = crm(t, dbPath, "log", "edit", "99", "--content", "x")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, dbPath, "log", "edit", "abc", "--content", "x")
	assert.Equal(t, 2, code)

	_, stderr, code := crm(t, dbPath, "log", "rm", "1")
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "Deleted interaction #1")
	out, _, _ = crm(t, dbPath, "log", "list", "-f", "json")
	assert.JSONEq(t, "[]", out)
	_, _, code = crm(t, dbPath, "log", "rm", "1")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, dbPath, "log", "edit", "1", "--content", "back")
	assert.Equal(t, 3, code, "archived rows cannot be edited")
}

const fakeRecords = `[
 {"seq":1,"kind":"person","external_id":"g1","person":{"emails":["jane@acme.com"],"title":"CTO"}},
 {"seq":2,"kind":"person","external_id":"g2","person":{"first_name":"Zed","emails":["zed@x.com"]}}
]`

func TestSyncCLI(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	records := filepath.Join(t.TempDir(), "fake.json")
	require.NoError(t, os.WriteFile(records, []byte(fakeRecords), 0o600))

	// Without the fake file there are no sources.
	out, _, code := crm(t, dbPath, "sync", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.JSONEq(t, "[]", out)
	_, _, code = crm(t, dbPath, "sync", "run", "fake")
	assert.Equal(t, 3, code, "unknown source")

	t.Setenv("CRM_SYNC_FAKE_FILE", records)
	crm(t, dbPath, "person", "add", "Jane", "--email", "jane@acme.com", "-f", "json")

	out, _, _ = crm(t, dbPath, "sync", "list", "-f", "json")
	assert.Contains(t, out, `"fake"`)

	out, _, code = crm(t, dbPath, "sync", "run", "fake", "--dry-run", "-f", "json")
	require.Equal(t, 0, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	assert.Equal(t, true, rows[0]["dry_run"])
	assert.EqualValues(t, 1, rows[0]["updated"])
	assert.EqualValues(t, 1, rows[0]["inbox"])
	out, _, _ = crm(t, dbPath, "sync", "status", "-f", "json")
	assert.NotContains(t, out, `"status"`, "dry run leaves no history")

	out, _, code = crm(t, dbPath, "sync", "run", "fake", "-f", "json")
	require.Equal(t, 0, code)
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	assert.Equal(t, "ok", rows[0]["status"])

	out, _, _ = crm(t, dbPath, "sync", "status", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	assert.Equal(t, "ok", rows[0]["status"])
	assert.EqualValues(t, 1, rows[0]["inbox_pending"])
	assert.Equal(t, "2", rows[0]["cursor"])

	out, _, _ = crm(t, dbPath, "status", "-f", "json")
	var st []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &st))
	last, ok := st[0]["last_sync"].(map[string]any)
	require.True(t, ok, out)
	assert.Equal(t, "fake", last["source"])
	assert.Equal(t, "ok", last["status"])
	assert.EqualValues(t, 1, last["inbox_pending"])

	out, _, code = crm(t, dbPath, "sync", "inbox", "-f", "json")
	require.Equal(t, 0, code)
	var inbox []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &inbox))
	require.Len(t, inbox, 1)
	assert.Equal(t, "g2", inbox[0]["external_id"])
	assert.Equal(t, "Zed", inbox[0]["summary"])

	_, _, code = crm(t, dbPath, "sync", "inbox", "accept", "1")
	assert.Equal(t, 0, code)
	out, _, _ = crm(t, dbPath, "person", "list", "-f", "json")
	assert.Contains(t, out, "Zed")
	_, _, code = crm(t, dbPath, "sync", "inbox", "ignore", "1")
	assert.Equal(t, 4, code, "already decided")
	_, _, code = crm(t, dbPath, "sync", "inbox", "attach", "1", "1")
	assert.Equal(t, 4, code)
	_, _, code = crm(t, dbPath, "sync", "inbox", "accept", "x")
	assert.Equal(t, 2, code)

	// Ignore and attach on fresh items.
	require.NoError(t, os.WriteFile(records, []byte(`[
	 {"seq":3,"kind":"person","external_id":"g3","person":{"first_name":"Ivy","emails":["ivy@x.com"]}},
	 {"seq":4,"kind":"person","external_id":"g4","person":{"first_name":"Al","emails":["al@x.com"],"title":"Dir"}}]`), 0o600))
	crm(t, dbPath, "sync", "run", "fake")
	crm(t, dbPath, "person", "add", "Pat", "-f", "json")
	_, _, code = crm(t, dbPath, "sync", "inbox", "ignore", "2")
	assert.Equal(t, 0, code)
	_, _, code = crm(t, dbPath, "sync", "inbox", "attach", "3", "3")
	assert.Equal(t, 0, code)
	out, _, _ = crm(t, dbPath, "person", "show", "3", "-f", "json")
	assert.Contains(t, out, "Dir")
	out, _, _ = crm(t, dbPath, "sync", "inbox", "--status", "all", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(out), &inbox))
	assert.Len(t, inbox, 3)
}

func TestSyncFailureExitsNonZero(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	t.Setenv("CRM_SYNC_FAKE_FILE", filepath.Join(t.TempDir(), "missing.json"))
	_, _, code := crm(t, dbPath, "sync", "run", "fake", "-f", "json")
	assert.Equal(t, 1, code)
	out, _, _ := crm(t, dbPath, "sync", "status", "-f", "json")
	assert.Contains(t, out, `"failed"`)
}
