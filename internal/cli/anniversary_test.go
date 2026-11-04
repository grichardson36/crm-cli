package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-022, S9 and S8 on the CLI door. The anniversary is an attribute of the spouse link.

func linksOf(t *testing.T, db string, id string) []map[string]any {
	t.Helper()
	out, stderr, code := crm(t, db, "person", "show", id, "-f", "json")
	require.Equal(t, 0, code, stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	require.Len(t, rows, 1)
	var links []map[string]any
	raw, _ := rows[0]["links"].([]any)
	for _, l := range raw {
		links = append(links, l.(map[string]any))
	}
	return links
}

func twoPeople(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "crm.db")
	crm(t, db, "person", "add", "Joey Osgood", "-f", "json")
	crm(t, db, "person", "add", "Hannah Osgood", "-f", "json")
	return db
}

func TestPersonLinkPersonAnniversary(t *testing.T) {
	db := twoPeople(t)
	_, stderr, code := crm(t, db, "person", "link", "person", "1", "2", "--kind", "spouse", "--anniversary", "2014-06-20")
	require.Equal(t, 0, code, stderr)
	for _, id := range []string{"1", "2"} {
		links := linksOf(t, db, id)
		require.Len(t, links, 1)
		assert.Equal(t, "2014-06-20", links[0]["anniversary"], "both partners see the one date")
	}

	t.Run("change and clear on the existing link", func(t *testing.T) {
		_, stderr, code := crm(t, db, "person", "link", "person", "1", "2", "--kind", "spouse", "--anniversary=--06-21")
		require.Equal(t, 0, code, stderr)
		assert.Equal(t, "--06-21", linksOf(t, db, "1")[0]["anniversary"])
		_, stderr, code = crm(t, db, "person", "link", "person", "1", "2", "--kind", "spouse", "--anniversary", "")
		require.Equal(t, 0, code, stderr)
		assert.NotContains(t, linksOf(t, db, "1")[0], "anniversary")
	})
	t.Run("table output shows it", func(t *testing.T) {
		crm(t, db, "person", "link", "person", "1", "2", "--kind", "spouse", "--anniversary", "2014-06-20")
		out, _, code := crm(t, db, "person", "show", "1", "-f", "table")
		require.Equal(t, 0, code)
		assert.Contains(t, out, "anniversary 2014-06-20")
	})
}

func TestPersonLinkPersonAnniversaryRefusals(t *testing.T) {
	db := twoPeople(t)
	_, stderr, code := crm(t, db, "person", "link", "person", "1", "2", "--kind", "friend", "--anniversary", "2014-06-20")
	assert.Equal(t, 2, code, "another kind rejects the attribute")
	assert.Contains(t, stderr, "spouse")
	_, stderr, code = crm(t, db, "person", "link", "person", "1", "2", "--kind", "spouse", "--anniversary", "soon")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "anniversary")
	assert.Empty(t, linksOf(t, db, "1"), "nothing was written")
}

func TestPersonRelateAnniversary(t *testing.T) {
	db := twoPeople(t)
	_, stderr, code := crm(t, db, "person", "relate", "1", "2", "--type", "spouse", "--anniversary", "2014-06-20")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, "2014-06-20", linksOf(t, db, "1")[0]["anniversary"])
}

func TestPersonMergeKeepsTheAnniversary(t *testing.T) {
	db := twoPeople(t)
	crm(t, db, "person", "add", "Joe Osgood", "-f", "json")
	_, stderr, code := crm(t, db, "person", "link", "person", "1", "2", "--kind", "spouse", "--anniversary", "2014-06-20")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, db, "person", "merge", "3", "1", "-f", "json")
	require.Equal(t, 0, code, stderr)
	links := linksOf(t, db, "3")
	require.Len(t, links, 1)
	assert.Equal(t, "2014-06-20", links[0]["anniversary"])
}

func TestBirthdaysCommandListsBirthdaysOnly(t *testing.T) {
	db := twoPeople(t)
	crm(t, db, "person", "link", "person", "1", "2", "--kind", "spouse", "--anniversary", "2014-06-20")
	out, stderr, code := crm(t, db, "birthdays", "--days", "366", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	assert.Empty(t, rows)
}

func TestModulesCommandRefusesTheOldBirthdaysKey(t *testing.T) {
	home := t.TempDir()
	_, stderr, code := keysRun(t, home, "modules", "off", "birthdays")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "no module")
	stdout, stderr, code := keysRun(t, home, "modules", "list", "-f", "json")
	require.Equal(t, 0, code, stderr)
	rows := keyRows(t, stdout)
	assert.Contains(t, rows, "dates")
	assert.NotContains(t, rows, "birthdays")
}
