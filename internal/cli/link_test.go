package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func linkSeed(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"person", "add", "Jane Smith"},
		{"person", "add", "Bob Jones"},
		{"org", "add", "Acme Corp"},
		{"org", "add", "Globex"},
		{"deal", "add", "Website"},
		{"task", "add", "Send proposal"},
	} {
		_, stderr, code := crm(t, dbPath, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	return dbPath
}

func TestLinkAndUnlinkFromEitherSide(t *testing.T) {
	dbPath := linkSeed(t)
	cases := []struct {
		aKind, aID, bKind, bID string
	}{
		{"person", "1", "org", "1"},
		{"deal", "1", "org", "1"},
		{"deal", "1", "person", "1"},
		{"task", "1", "deal", "1"},
		{"task", "1", "person", "1"},
		{"task", "1", "org", "1"},
		{"person", "1", "person", "2"},
	}
	for _, c := range cases {
		stdout, stderr, code := crm(t, dbPath, c.aKind, "link", c.bKind, c.aID, c.bID, "-f", "json")
		require.Equal(t, 0, code, "%s link %s: %s", c.aKind, c.bKind, stderr)
		var rows []map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
		assert.Equal(t, true, rows[0]["changed"])

		// The other side links the same pair: nothing changes.
		var stderr2 string
		stdout, stderr2, code = crm(t, dbPath, c.bKind, "link", c.aKind, c.bID, c.aID, "-f", "json")
		require.Equal(t, 0, code, "%s link %s from the other side: %s", c.bKind, c.aKind, stderr2)
		require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
		assert.Equal(t, false, rows[0]["changed"], "%s and %s are one stored link", c.aKind, c.bKind)

		// Remove it from the other side.
		_, stderr, code = crm(t, dbPath, c.bKind, "unlink", c.aKind, c.bID, c.aID)
		require.Equal(t, 0, code, stderr)
		_, stderr, code = crm(t, dbPath, c.aKind, "unlink", c.bKind, c.aID, c.bID)
		assert.Equal(t, 3, code, "unlinking twice is not found")
		assert.Contains(t, stderr, "not linked")
	}
}

func TestLinkRefusesBadInput(t *testing.T) {
	dbPath := linkSeed(t)
	_, stderr, code := crm(t, dbPath, "person", "link", "org", "1", "99")
	assert.Equal(t, 3, code)
	assert.Contains(t, stderr, "not found")
	_, _, code = crm(t, dbPath, "person", "link", "org", "x", "1")
	assert.Equal(t, 2, code)
}

func TestLinkDryRunWritesNothing(t *testing.T) {
	dbPath := linkSeed(t)
	stdout, _, code := crm(t, dbPath, "task", "link", "person", "1", "1", "--dry-run", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, `"dry_run":true`)
	stdout, _, _ = crm(t, dbPath, "task", "show", "1", "-f", "json")
	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Nil(t, data[0]["person_ids"])
}

func TestLinkTagOnAnything(t *testing.T) {
	dbPath := linkSeed(t)
	for _, kind := range []string{"person", "org", "deal", "task"} {
		stdout, stderr, code := crm(t, dbPath, kind, "link", "tag", "1", "vip", "-f", "json")
		require.Equal(t, 0, code, "%s: %s", kind, stderr)
		assert.Contains(t, stdout, "vip")
		stdout, _, code = crm(t, dbPath, kind, "unlink", "tag", "1", "vip", "-f", "json")
		require.Equal(t, 0, code)
		assert.NotContains(t, stdout, "vip")
	}
}

func TestOrgAndDealRestore(t *testing.T) {
	dbPath := linkSeed(t)
	for _, kind := range []string{"org", "deal", "task"} {
		_, stderr, code := crm(t, dbPath, kind, "delete", "1")
		require.Equal(t, 0, code, stderr)
		_, _, code = crm(t, dbPath, kind, "show", "1")
		assert.Equal(t, 3, code, "%s is deleted", kind)
		_, stderr, code = crm(t, dbPath, kind, "restore", "1")
		require.Equal(t, 0, code, stderr)
		assert.Contains(t, stderr, "Restored")
		_, _, code = crm(t, dbPath, kind, "show", "1")
		assert.Equal(t, 0, code, "%s is back", kind)
		_, _, code = crm(t, dbPath, kind, "restore", "1")
		assert.Equal(t, 3, code, "restoring a live %s is not found", kind)
	}
}
