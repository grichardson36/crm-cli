package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagRenameAndMergeCommands(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"person", "add", "Ann"}, {"person", "add", "Ben"},
		{"tag", "apply", "person", "1", "friends"}, {"tag", "apply", "person", "2", "friends"},
		{"tag", "apply", "person", "2", "Friend"},
	} {
		_, stderr, code := crm(t, db, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	_, stderr, code := crm(t, db, "tag", "rename", "friends", "friend")
	assert.Equal(t, 4, code, "a name that exists is a conflict")
	assert.Contains(t, stderr, "merge")

	out, stderr, code := crm(t, db, "tag", "merge", "friends", "Friend", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "Preview only")
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Equal(t, true, res["dry_run"])
	assert.EqualValues(t, 1, res["total"])
	assert.EqualValues(t, 1, res["duplicates"].(map[string]any)["person"])
	out, _, _ = crm(t, db, "tag", "list", "-f", "json")
	assert.Len(t, jsonRows(t, out), 2, "the preview changed nothing")

	_, stderr, code = crm(t, db, "tag", "merge", "friends", "Friend", "--apply")
	require.Equal(t, 0, code, stderr)
	out, _, _ = crm(t, db, "tag", "list", "-f", "json")
	rows := jsonRows(t, out)
	require.Len(t, rows, 1)
	assert.Equal(t, "Friend", rows[0]["name"])

	_, stderr, code = crm(t, db, "tag", "rename", "Friend", "pals")
	require.Equal(t, 0, code, stderr)
	out, _, _ = crm(t, db, "tag", "list", "-f", "json")
	assert.Equal(t, "pals", jsonRows(t, out)[0]["name"])
	_, _, code = crm(t, db, "tag", "rename", "ghost", "x")
	assert.Equal(t, 3, code)
}

func TestTagUnusedAndDuplicatesCommands(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"person", "add", "Ann"}, {"person", "add", "Ben"},
		{"tag", "apply", "person", "1", "Friend"}, {"tag", "apply", "person", "2", "friend"},
		{"tag", "apply", "person", "1", "gone"}, {"tag", "remove", "person", "1", "gone"},
	} {
		_, stderr, code := crm(t, db, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}

	out, stderr, code := crm(t, db, "tag", "duplicates", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var dup map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &dup))
	assert.Equal(t, false, dup["applied"])
	assert.Len(t, dup["groups"], 1)
	out, _, _ = crm(t, db, "tag", "duplicates", "-f", "table")
	assert.Contains(t, out, "MERGE INTO")

	out, stderr, code = crm(t, db, "tag", "unused", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "1 unused")
	var un map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &un))
	assert.Equal(t, true, un["dry_run"])
	assert.EqualValues(t, 1, un["count"])

	_, stderr, code = crm(t, db, "tag", "unused", "--apply")
	require.Equal(t, 0, code, stderr)
	out, _, _ = crm(t, db, "tag", "unused", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(out), &un))
	assert.EqualValues(t, 0, un["count"], "applied")

	_, stderr, code = crm(t, db, "tag", "duplicates", "--apply")
	require.Equal(t, 0, code, stderr)
	out, _, _ = crm(t, db, "tag", "list", "-f", "json")
	assert.Len(t, jsonRows(t, out), 1, "the two spellings are one tag")
}
