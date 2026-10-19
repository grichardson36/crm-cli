package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// showLinks returns the links of an object from `crm <kind> show <id> -f json`.
func showLinks(t *testing.T, dbPath, kind, id string) []map[string]any {
	t.Helper()
	stdout, stderr, code := crm(t, dbPath, kind, "show", id, "-f", "json")
	require.Equal(t, 0, code, stderr)
	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	raw, _ := data[0]["links"].([]any)
	var out []map[string]any
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func TestMembershipFlagsAndShow(t *testing.T) {
	dbPath := linkSeed(t)
	stdout, stderr, code := crm(t, dbPath, "person", "link", "org", "1", "1", "--role", "CTO", "--from", "2019-03", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Equal(t, true, rows[0]["created"])
	assert.EqualValues(t, 1, rows[0]["link_id"])

	links := showLinks(t, dbPath, "person", "1")
	require.Len(t, links, 1)
	assert.Equal(t, "organization", links[0]["kind"])
	assert.Equal(t, "Acme Corp", links[0]["name"])
	assert.Equal(t, "CTO", links[0]["role"])
	assert.Equal(t, "2019-03", links[0]["from"])
	assert.Equal(t, true, links[0]["primary"], "the first organization is primary")

	// The organization sees the person with the same attributes.
	orgLinks := showLinks(t, dbPath, "org", "1")
	require.Len(t, orgLinks, 1)
	assert.Equal(t, "person", orgLinks[0]["kind"])
	assert.Equal(t, "CTO", orgLinks[0]["role"])

	// A second organization is not primary. --primary moves it and reports the one that lost it.
	_, stderr, code = crm(t, dbPath, "person", "link", "org", "1", "2", "--role", "Advisor")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ = crm(t, dbPath, "person", "link", "org", "1", "2", "--primary", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Equal(t, true, rows[0]["updated"])
	assert.NotEmpty(t, rows[0]["demoted"])
	links = showLinks(t, dbPath, "person", "1")
	require.Len(t, links, 2)
	assert.Equal(t, "Globex", links[0]["name"], "the primary is listed first")

	// Rehired: --new adds a second link to the same organization. Without it the call asks which one.
	_, _, code = crm(t, dbPath, "person", "link", "org", "1", "1", "--role", "Director", "--from", "2023", "--new")
	require.Equal(t, 0, code)
	_, stderr, code = crm(t, dbPath, "person", "link", "org", "1", "1", "--role", "Chief")
	assert.Equal(t, 4, code, stderr)
	assert.Contains(t, stderr, "linked 2 times")

	// Unlink one of the two by id; the other stays.
	_, stderr, code = crm(t, dbPath, "person", "unlink", "org", "1", "1", "--id", "3")
	require.Equal(t, 0, code, stderr)
	assert.Len(t, showLinks(t, dbPath, "person", "1"), 2)

	// Bad dates and attributes the relation lacks are refused.
	_, _, code = crm(t, dbPath, "person", "link", "org", "1", "2", "--from", "last year")
	assert.Equal(t, 2, code)
	_, _, code = crm(t, dbPath, "deal", "link", "person", "1", "1", "--from", "2020")
	assert.Equal(t, 2, code, "--from is not a flag of a deal link")
}

func TestPersonLinksHaveKindsAndInverseLabels(t *testing.T) {
	dbPath := linkSeed(t)
	_, stderr, code := crm(t, dbPath, "person", "link", "person", "1", "2", "--kind", "parent")
	require.Equal(t, 0, code, stderr)
	jane := showLinks(t, dbPath, "person", "1")
	bob := showLinks(t, dbPath, "person", "2")
	require.Len(t, jane, 1)
	require.Len(t, bob, 1)
	assert.Equal(t, "child", jane[0]["label"], "Jane is the parent, so she sees Bob as her child")
	assert.Equal(t, "parent", bob[0]["label"])

	// Naming the inverse reverses the ends: Bob is a child of Jane is the same link.
	stdout, _, _ := crm(t, dbPath, "person", "link", "person", "2", "1", "--kind", "child", "-f", "json")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Equal(t, false, rows[0]["changed"])

	_, stderr, code = crm(t, dbPath, "person", "link", "person", "1", "2", "--kind", "nemesis")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "known:")
}

func TestOrgParentsRejectCycles(t *testing.T) {
	dbPath := linkSeed(t)
	_, stderr, code := crm(t, dbPath, "org", "add", "Initech")
	require.Equal(t, 0, code, stderr)
	for _, pair := range [][2]string{{"1", "2"}, {"2", "3"}} {
		_, stderr, code = crm(t, dbPath, "org", "link", "org", pair[0], pair[1])
		require.Equal(t, 0, code, stderr)
	}
	_, stderr, code = crm(t, dbPath, "org", "link", "org", "3", "1")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "cycle")
	// A second parent is fine.
	_, stderr, code = crm(t, dbPath, "org", "link", "org", "1", "3")
	assert.Equal(t, 0, code, stderr)
	assert.Len(t, showLinks(t, dbPath, "org", "3"), 2)
}

func TestRelationKindCommandsAndRoles(t *testing.T) {
	dbPath := linkSeed(t)
	stdout, stderr, code := crm(t, dbPath, "relation-kind", "list", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var kinds []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &kinds))
	assert.Len(t, kinds, 10)

	_, stderr, code = crm(t, dbPath, "relation-kind", "add", "godparent", "--inverse", "godchild")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, dbPath, "relation-kind", "add", "neighbor")
	require.Equal(t, 0, code, stderr)
	_, _, code = crm(t, dbPath, "relation-kind", "add", "friend")
	assert.Equal(t, 4, code, "the name is taken")
	_, _, code = crm(t, dbPath, "person", "link", "person", "1", "2", "--kind", "godchild")
	assert.Equal(t, 0, code, "an inverse name works as a kind name")
	_, _, code = crm(t, dbPath, "relation-kind", "rm", "godparent")
	assert.Equal(t, 4, code, "in use")
	_, _, code = crm(t, dbPath, "relation-kind", "rm", "friend")
	assert.Equal(t, 4, code, "built in")
	_, _, code = crm(t, dbPath, "relation-kind", "rm", "neighbor")
	assert.Equal(t, 0, code)

	_, _, _ = crm(t, dbPath, "person", "link", "org", "1", "1", "--role", "Founder")
	_, _, _ = crm(t, dbPath, "person", "link", "org", "2", "2", "--role", "founder")
	stdout, _, code = crm(t, dbPath, "roles", "-f", "json")
	require.Equal(t, 0, code)
	var roles []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &roles))
	require.Len(t, roles, 1)
	assert.Equal(t, "Founder", roles[0]["role"])
}

func TestDealClosedAtAndTaskDueClear(t *testing.T) {
	dbPath := linkSeed(t)
	stdout, _, code := crm(t, dbPath, "deal", "edit", "1", "--stage", "won", "-f", "json")
	require.Equal(t, 0, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.NotNil(t, rows[0]["closed_at"], "won sets closed_at")
	stdout, _, _ = crm(t, dbPath, "deal", "edit", "1", "--stage", "negotiation", "-f", "json")
	rows = nil
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Nil(t, rows[0]["closed_at"], "reopening clears it")

	_, _, code = crm(t, dbPath, "task", "edit", "1", "--due", "2030-01-01")
	require.Equal(t, 0, code)
	stdout, _, _ = crm(t, dbPath, "task", "show", "1", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Equal(t, "2030-01-01", rows[0]["due_at"])
	_, stderr, code := crm(t, dbPath, "task", "edit", "1", "--clear-due")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ = crm(t, dbPath, "task", "show", "1", "-f", "json")
	rows = nil
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Nil(t, rows[0]["due_at"])
}

func TestLogNoteOnAnOrganizationAndDeal(t *testing.T) {
	dbPath := linkSeed(t)
	stdout, stderr, code := crm(t, dbPath, "log", "add", "--type", "note", "--org", "1", "--deal", "1", "Kickoff went well", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Equal(t, []any{float64(1)}, rows[0]["org_ids"])
	assert.Equal(t, []any{float64(1)}, rows[0]["deal_ids"])

	stdout, _, _ = crm(t, dbPath, "log", "list", "--org", "1", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Len(t, rows, 1)
	stdout, _, _ = crm(t, dbPath, "log", "list", "--org", "2", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Len(t, rows, 0)

	// A task about an organization.
	_, stderr, code = crm(t, dbPath, "task", "link", "org", "1", "2")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ = crm(t, dbPath, "task", "list", "--org", "2", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Len(t, rows, 1)
}

func TestHiddenOrganizationHidesItsLinksAndRestoreBringsThemBack(t *testing.T) {
	dbPath := linkSeed(t)
	_, _, code := crm(t, dbPath, "person", "link", "org", "1", "1", "--role", "CTO")
	require.Equal(t, 0, code)
	_, _, code = crm(t, dbPath, "org", "delete", "1")
	require.Equal(t, 0, code)
	assert.Empty(t, showLinks(t, dbPath, "person", "1"))
	_, _, code = crm(t, dbPath, "org", "restore", "1")
	require.Equal(t, 0, code)
	links := showLinks(t, dbPath, "person", "1")
	require.Len(t, links, 1)
	assert.Equal(t, "CTO", links[0]["role"])
}

func TestReferredByRendersFromTheViewingSide(t *testing.T) {
	dbPath := linkSeed(t)
	_, stderr, code := crm(t, dbPath, "person", "link", "person", "1", "2", "--kind", "referred-by")
	require.Equal(t, 0, code, stderr)
	jane := showLinks(t, dbPath, "person", "1")
	bob := showLinks(t, dbPath, "person", "2")
	require.Len(t, jane, 1)
	require.Len(t, bob, 1)
	assert.Equal(t, "referred-by", jane[0]["label"], "person 1 was referred by person 2")
	assert.Equal(t, "referred", bob[0]["label"], "person 2 referred person 1")
}
