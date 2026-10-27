package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func orgNames(t *testing.T, dbPath string) []string {
	t.Helper()
	stdout, stderr, code := crm(t, dbPath, "org", "list", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var orgs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &orgs))
	var names []string
	for _, o := range orgs {
		names = append(names, o["name"].(string))
	}
	return names
}

func TestPersonCompanyCreatesOrReusesOrganization(t *testing.T) {
	tests := []struct {
		name     string
		steps    [][]string
		wantOrgs []string
	}{
		{"add creates", [][]string{{"person", "add", "Ann Lee", "--company", "Acme Corp"}}, []string{"Acme Corp"}},
		{"add reuses a normalized match", [][]string{
			{"person", "add", "Ann Lee", "--company", "Acme Corp"},
			{"person", "add", "Bob Ray", "--company", "ACME corp."},
		}, []string{"Acme Corp"}},
		{"add reuses an existing org", [][]string{
			{"org", "add", "Globex"}, {"person", "add", "Bob Ray", "--company", "Globex, Inc."},
		}, []string{"Globex"}},
		{"close name makes its own org", [][]string{
			{"org", "add", "Acme Corp"}, {"person", "add", "Bob Ray", "--company", "Acme Cop"},
		}, []string{"Acme Corp", "Acme Cop"}},
		{"junk creates nothing", [][]string{
			{"person", "add", "Ann Lee", "--company", "N/A"}, {"person", "add", "Bob Ray", "--company", "Self-Employed"},
		}, nil},
		{"edit creates", [][]string{
			{"person", "add", "Ann Lee"}, {"person", "edit", "1", "--company", "Initech"},
		}, []string{"Initech"}},
		{"explicit --org skips the rule", [][]string{
			{"org", "add", "Globex"}, {"person", "add", "Ann Lee", "--company", "Elsewhere", "--org", "1"},
		}, []string{"Globex"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "crm.db")
			for _, s := range tt.steps {
				_, stderr, code := crm(t, dbPath, s...)
				require.Equal(t, 0, code, "%v: %s", s, stderr)
			}
			// org list orders by updated_at, which ties inside one second, so compare as a set
			assert.ElementsMatch(t, tt.wantOrgs, orgNames(t, dbPath))
		})
	}
}

func TestOrgBackfillDryRunThenApply(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	// Company text from before the rule: written through a database copy without the rule is not
	// possible from the CLI, so clear the links the rule made.
	for _, s := range [][]string{
		{"person", "add", "A One", "--company", "Acme Inc"},
		{"person", "add", "A Two", "--company", "ACME"},
		{"person", "add", "A Three", "--company", "acme, inc."},
		{"person", "add", "Retired Guy", "--company", "retired"},
	} {
		_, stderr, code := crm(t, dbPath, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	for _, id := range []string{"1", "2", "3"} {
		_, stderr, code := crm(t, dbPath, "person", "unlink", "org", id, "1")
		require.Equal(t, 0, code, stderr)
	}

	stdout, stderr, code := crm(t, dbPath, "org", "backfill", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "Dry run")
	var out struct {
		Applied bool           `json:"applied"`
		Counts  map[string]any `json:"counts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	assert.False(t, out.Applied)
	assert.EqualValues(t, 4, out.Counts["people_with_company"])
	assert.EqualValues(t, 2, out.Counts["groups"])
	assert.EqualValues(t, 0, out.Counts["orgs_to_create"])
	assert.EqualValues(t, 1, out.Counts["orgs_reused"])
	assert.EqualValues(t, 1, out.Counts["ignored_people"])
	assert.EqualValues(t, 3, out.Counts["people_to_link"])
	assert.Equal(t, []string{"Acme Inc"}, orgNames(t, dbPath))

	_, stderr, code = crm(t, dbPath, "org", "backfill", "--apply")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "linked 3 people")
	_, stderr, code = crm(t, dbPath, "org", "backfill", "--apply", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "linked 0 people", "idempotent")

	// The deprecated alias is a dry run too.
	_, stderr, code = crm(t, dbPath, "org", "convert")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "Dry run")
}

func TestOrgIgnoreListCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	stdout, stderr, code := crm(t, dbPath, "org", "ignore", "list", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "self-employed")

	_, stderr, code = crm(t, dbPath, "org", "ignore", "add", "Gig Work")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, dbPath, "person", "add", "Ann Lee", "--company", "gig work")
	require.Equal(t, 0, code, stderr)
	assert.Nil(t, orgNames(t, dbPath))

	_, stderr, code = crm(t, dbPath, "org", "ignore", "remove", "gig work")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, dbPath, "person", "add", "Bob Ray", "--company", "Gig Work")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"Gig Work"}, orgNames(t, dbPath))

	_, _, code = crm(t, dbPath, "org", "ignore", "remove", "never listed")
	assert.Equal(t, 3, code, "not found")
}

func TestOrgDupesListsNearNamesAndMergesNothing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"person", "add", "Ann Lee", "--company", "Acme Corp"},
		{"person", "add", "Bob Ray", "--company", "Acme Corpp"},
		{"person", "add", "Cy Doe", "--company", "Globex"},
	} {
		_, stderr, code := crm(t, dbPath, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	stdout, stderr, code := crm(t, dbPath, "org", "dupes", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var pairs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &pairs))
	require.Len(t, pairs, 1)
	assert.Equal(t, "crm org merge 1 2", pairs[0]["merge"])
	assert.Len(t, orgNames(t, dbPath), 3)
}
