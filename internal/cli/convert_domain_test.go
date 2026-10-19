package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgConvertShowsDomainsAndNeverBulkAcceptsADomainMatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"person", "add", "Ann Lee", "--company", "Acme Worldwide", "--email", "ann@acme.com"},
		{"person", "add", "Bob Roe", "--company", "ACME Worldwide", "--email", "bob@acme.com"},
		{"person", "add", "Cy Poe", "--company", "Globex", "--email", "cy@globex.com"},
		{"person", "add", "Dee Fox", "--company", "Hooli", "--email", "dee@gmail.com"},
		{"org", "add", "Acme Holdings", "--domain", "acme.com"},
	} {
		_, stderr, code := crm(t, dbPath, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	rows := convertGroups(t, dbPath)
	by := map[string]map[string]any{}
	for _, r := range rows {
		by[r["key"].(string)] = r
	}
	acme := by["acme worldwide"]
	assert.Equal(t, "acme.com", acme["domain"])
	assert.Equal(t, "domain", acme["domain_match"].(map[string]any)["kind"])
	assert.Contains(t, acme["domain_match_text"], "Acme Holdings")
	assert.Nil(t, acme["match"], "shown apart from name matches")
	assert.Equal(t, "globex.com", by["globex"]["domain"])
	assert.Nil(t, by["hooli"]["domain"], "free mail is ignored")

	stdout, _, code := crm(t, dbPath, "org", "convert", "--stats", "-f", "json")
	require.Equal(t, 0, code)
	var stats []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &stats))
	assert.EqualValues(t, 1, stats[0]["domain_matches"])
	assert.EqualValues(t, 2, stats[0]["domains"])

	// accept-exact has nothing to take: the only suggestion is by domain.
	_, stderr, code := crm(t, dbPath, "org", "convert", "--accept-exact", "--apply")
	assert.NotEqual(t, 0, code, stderr)

	// Create sets the group's domain, and the preview says so.
	stdout, _, code = crm(t, dbPath, "org", "convert", "--create", "globex=Globex", "--apply", "-f", "json")
	require.Equal(t, 0, code)
	var plan []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &plan))
	assert.Equal(t, "globex.com", plan[0]["domain"])
	stdout, _, _ = crm(t, dbPath, "org", "list", "-f", "json")
	var orgs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &orgs))
	domains := map[string]any{}
	for _, o := range orgs {
		domains[o["name"].(string)] = o["domain"]
	}
	assert.Equal(t, "globex.com", domains["Globex"])
}
