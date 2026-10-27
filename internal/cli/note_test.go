package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noteSeed(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"person", "add", "Jane Smith"},
		{"org", "add", "Acme Corp"},
		{"deal", "add", "Website"},
	} {
		_, stderr, code := crm(t, dbPath, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	return dbPath
}

// A note has every verb of the one verb set: add, edit, delete, restore, link and unlink.
func TestNoteIsARegisteredObject(t *testing.T) {
	dbPath := noteSeed(t)
	stdout, stderr, code := crm(t, dbPath, "note", "add", "Talked about the offsite", "--type", "call", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Equal(t, "call", rows[0]["type"])
	assert.Equal(t, "Talked about the offsite", rows[0]["content"])

	// edit
	_, stderr, code = crm(t, dbPath, "note", "edit", "1", "--content", "Corrected")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ = crm(t, dbPath, "note", "show", "1", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Equal(t, "Corrected", rows[0]["content"])

	// link and unlink to every kind of object, from both sides
	for _, c := range []struct{ kind, id string }{{"person", "1"}, {"org", "1"}, {"deal", "1"}} {
		stdout, stderr, code = crm(t, dbPath, "note", "link", c.kind, "1", c.id, "-f", "json")
		require.Equal(t, 0, code, "note link %s: %s", c.kind, stderr)
		require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
		assert.Equal(t, true, rows[0]["changed"])

		stdout, stderr, code = crm(t, dbPath, c.kind, "link", "note", c.id, "1", "-f", "json")
		require.Equal(t, 0, code, "%s link note: %s", c.kind, stderr)
		require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
		assert.Equal(t, false, rows[0]["changed"], "the same stored link seen from the other side")
	}
	stdout, _, _ = crm(t, dbPath, "note", "list", "--org", "1", "-f", "json")
	assert.Contains(t, stdout, "Corrected", "the note is on the organization")
	_, stderr, code = crm(t, dbPath, "org", "unlink", "note", "1", "1")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ = crm(t, dbPath, "note", "list", "--org", "1", "-f", "json")
	assert.NotContains(t, stdout, "Corrected")

	// delete and restore
	_, stderr, code = crm(t, dbPath, "note", "delete", "1")
	require.Equal(t, 0, code, stderr)
	_, _, code = crm(t, dbPath, "note", "show", "1")
	assert.Equal(t, 3, code, "a deleted note is not found")
	_, stderr, code = crm(t, dbPath, "note", "restore", "1")
	require.Equal(t, 0, code, stderr)
	_, _, code = crm(t, dbPath, "note", "show", "1")
	assert.Equal(t, 0, code)
}

func TestNoteAddAttachesAndValidates(t *testing.T) {
	dbPath := noteSeed(t)
	cases := []struct {
		name string
		args []string
		code int
	}{
		{"person org and deal", []string{"note", "add", "Intro", "--person", "1", "--org", "1", "--deal", "1"}, 0},
		{"free-standing", []string{"note", "add", "Loose thought"}, 0},
		{"subject only", []string{"note", "add", "--subject", "Quick check-in"}, 0},
		{"no text", []string{"note", "add"}, 2},
		{"bad direction", []string{"note", "add", "x", "--direction", "sideways"}, 2},
		{"bad date", []string{"note", "add", "x", "--at", "yesterday-ish"}, 2},
		{"unknown person", []string{"note", "add", "x", "--person", "99"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, code := crm(t, dbPath, c.args...)
			assert.Equal(t, c.code, code, stderr)
		})
	}
	stdout, _, _ := crm(t, dbPath, "note", "list", "--person", "1", "-f", "json")
	assert.Contains(t, stdout, "Intro")
}

func TestLogNamesStillWork(t *testing.T) {
	dbPath := noteSeed(t)
	_, stderr, code := crm(t, dbPath, "log", "note", "1", "--content", "old name")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ := crm(t, dbPath, "note", "show", "1", "-f", "json")
	assert.Contains(t, stdout, "old name")
}
