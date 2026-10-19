package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func convertSeed(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"person", "add", "Ann Lee", "--company", "Acme Inc", "--title", "CTO"},
		{"person", "add", "Bob Roe", "--company", "ACME"},
		{"person", "add", "Cy Poe", "--company", "Globex"},
		{"person", "add", "Dee Fox", "--company", "Mystery Ltd"},
		{"org", "add", "Acme"},
	} {
		_, stderr, code := crm(t, dbPath, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	return dbPath
}

func convertGroups(t *testing.T, dbPath string, extra ...string) []map[string]any {
	t.Helper()
	args := append([]string{"org", "convert", "-f", "json"}, extra...)
	stdout, stderr, code := crm(t, dbPath, args...)
	require.Equal(t, 0, code, stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	return rows
}

func TestOrgConvertListsGroupsAndCreatesNothing(t *testing.T) {
	dbPath := convertSeed(t)
	rows := convertGroups(t, dbPath)
	require.Len(t, rows, 3)
	assert.Equal(t, "acme", rows[0]["key"])
	assert.EqualValues(t, 2, rows[0]["people"])
	match := rows[0]["match"].(map[string]any)
	assert.Equal(t, "exact", match["kind"])

	_, stderr, code := crm(t, dbPath, "org", "convert")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "4 people with company text, 3 groups, 1 matched")

	stdout, _, _ := crm(t, dbPath, "org", "list", "-f", "json")
	var orgs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &orgs))
	assert.Len(t, orgs, 1, "looking created nothing")
}

func TestOrgConvertPreviewThenApplyThenNothingLeft(t *testing.T) {
	dbPath := convertSeed(t)
	// A preview writes nothing.
	stdout, stderr, code := crm(t, dbPath, "org", "convert", "--accept-exact", "--create", "globex=Globex Corp", "--skip", "mystery", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "Preview only")
	var plan []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &plan))
	require.Len(t, plan, 3)
	for _, p := range plan {
		assert.Equal(t, false, p["applied"])
	}
	assert.Len(t, convertGroups(t, dbPath), 3)

	// Apply: the exact match is linked, Globex is created under the new name, Mystery is skipped.
	stdout, stderr, code = crm(t, dbPath, "org", "convert", "--accept-exact", "--create", "globex=Globex Corp", "--skip", "mystery", "--apply", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "Converted 3 people in 2 groups, created 1 organizations")
	require.NoError(t, json.Unmarshal([]byte(stdout), &plan))
	for _, p := range plan {
		assert.Equal(t, true, p["applied"])
	}
	assert.Empty(t, convertGroups(t, dbPath), "everything left was converted or skipped")
	assert.Len(t, convertGroups(t, dbPath, "--all"), 1, "the skipped group is still known")

	// The person page: membership with the title as the role, and the text is kept.
	links := showLinks(t, dbPath, "person", "1")
	require.Len(t, links, 1)
	assert.Equal(t, "Acme", links[0]["name"])
	assert.Equal(t, "CTO", links[0]["role"])
	assert.Equal(t, true, links[0]["primary"])
	stdout, _, _ = crm(t, dbPath, "person", "show", "1", "-f", "json")
	assert.Contains(t, stdout, `"company":"Acme Inc"`)

	// Idempotent: the same decisions again change nothing and create nothing.
	_, stderr, code = crm(t, dbPath, "org", "convert", "--create", "globex", "--apply")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ = crm(t, dbPath, "org", "list", "-f", "json")
	var orgs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &orgs))
	assert.Len(t, orgs, 2)

	// Unskip brings the group back.
	_, _, code = crm(t, dbPath, "org", "convert", "--unskip", "mystery", "--apply")
	require.Equal(t, 0, code)
	assert.Len(t, convertGroups(t, dbPath), 1)
}

func TestOrgConvertDecisionsFileAndGuards(t *testing.T) {
	dbPath := convertSeed(t)
	file := filepath.Join(t.TempDir(), "d.json")
	require.NoError(t, os.WriteFile(file, []byte(`[{"key":"mystery","action":"create","name":"Mystery Labs"},{"key":"globex","action":"link","org_id":1}]`), 0o600))
	_, stderr, code := crm(t, dbPath, "org", "convert", "--decisions", file, "--apply")
	require.Equal(t, 0, code, stderr)
	assert.Empty(t, showLinks(t, dbPath, "person", "1"), "only the groups named were touched")
	links := showLinks(t, dbPath, "person", "4")
	require.Len(t, links, 1)
	assert.Equal(t, "Mystery Labs", links[0]["name"])

	_, stderr, code = crm(t, dbPath, "org", "convert", "--apply")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "--apply needs decisions")
	_, _, code = crm(t, dbPath, "org", "convert", "--link", "acme=99", "--apply")
	assert.Equal(t, 3, code, "an unknown organization is not found")
	_, _, code = crm(t, dbPath, "org", "convert", "--link", "acme", "--apply")
	assert.Equal(t, 2, code)
}
