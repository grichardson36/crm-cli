package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgAddRefusesADuplicateUnlessAllowed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	_, stderr, code := crm(t, dbPath, "org", "add", "Blue Origin", "--domain", "blueorigin.com")
	require.Equal(t, 0, code, stderr)

	_, stderr, code = crm(t, dbPath, "org", "add", "BLUE ORIGIN")
	assert.Equal(t, 4, code, "a conflict")
	assert.Contains(t, stderr, "Blue Origin")
	assert.Contains(t, stderr, "id 1")
	assert.Contains(t, stderr, "--allow-duplicate")

	_, stderr, code = crm(t, dbPath, "org", "add", "Other", "--domain", "www.BlueOrigin.com")
	assert.Equal(t, 4, code, stderr)

	_, stderr, code = crm(t, dbPath, "org", "add", "BLUE ORIGIN", "--allow-duplicate")
	assert.Equal(t, 0, code, stderr)
}

func TestOrgMergePreviewsThenApplies(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"org", "add", "BLUE ORIGIN"},
		{"org", "add", "Blue Origin", "--domain", "blueorigin.com", "--allow-duplicate"},
		{"person", "add", "Ann Lee", "--company", "Blue Origin"},
		{"person", "link", "org", "1", "2", "--role", "CTO"},
	} {
		_, stderr, code := crm(t, dbPath, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	stdout, stderr, code := crm(t, dbPath, "org", "merge", "1", "2", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "Preview only")
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	assert.Equal(t, true, res["dry_run"])
	assert.EqualValues(t, 1, res["moved"].(map[string]any)["memberships"])

	stdout, _, _ = crm(t, dbPath, "org", "list", "-f", "json")
	var orgs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &orgs))
	assert.Len(t, orgs, 2, "the preview changed nothing")

	_, stderr, code = crm(t, dbPath, "org", "merge", "1", "2", "--apply")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "Merged")
	stdout, _, _ = crm(t, dbPath, "org", "list", "-f", "json")
	orgs = nil
	require.NoError(t, json.Unmarshal([]byte(stdout), &orgs))
	require.Len(t, orgs, 1)
	assert.Equal(t, "blueorigin.com", orgs[0]["domain"])

	_, _, code = crm(t, dbPath, "org", "merge", "1", "1")
	assert.NotEqual(t, 0, code)
}
