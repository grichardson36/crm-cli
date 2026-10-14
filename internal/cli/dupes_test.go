package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedDupes(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "crm.db")
	for _, args := range [][]string{
		{"person", "add", "Jane Smith", "--email", "jane@x.com", "--title", "CEO"},
		{"person", "add", "Janet Smith", "--email", "janet@x.com", "--company", "Acme"},
		{"person", "email", "add", "2", "JANE@x.com"},
		{"person", "add", "Bob Jones"},
		{"person", "add", "Robert Jones"},
		{"person", "add", "Jonathan Hartwell"},
		{"person", "add", "Jonathon Hartwell"},
		{"person", "add", "Zed Quux"},
	} {
		_, stderr, code := crm(t, db, args...)
		require.Equal(t, 0, code, stderr)
	}
	return db
}

func dupeGroups(t *testing.T, db string, args ...string) []map[string]any {
	t.Helper()
	out, stderr, code := crm(t, db, append([]string{"dupes", "-f", "json"}, args...)...)
	require.Equal(t, 0, code, stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows), out)
	return rows
}

func TestDupes_LevelsAndMin(t *testing.T) {
	db := seedDupes(t)

	rows := dupeGroups(t, db)
	require.Len(t, rows, 1, "default minimum is medium")
	assert.Equal(t, "high", rows[0]["level"])
	assert.Equal(t, []any{float64(1), float64(2)}, rows[0]["ids"])
	assert.NotEmpty(t, rows[0]["reasons"])

	assert.Len(t, dupeGroups(t, db, "--min", "low"), 2)
	rows = dupeGroups(t, db, "--min", "fuzzy")
	require.Len(t, rows, 3)
	assert.Equal(t, []string{"high", "low", "fuzzy"}, []string{rows[0]["level"].(string), rows[1]["level"].(string), rows[2]["level"].(string)})
	assert.Len(t, dupeGroups(t, db, "--min", "fuzzy", "--limit", "2"), 2)

	_, _, code := crm(t, db, "dupes", "--min", "bogus")
	assert.Equal(t, 2, code)

	out, _, code := crm(t, db, "dupes", "--min", "fuzzy", "-f", "table")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "Jane Smith | Janet Smith")
}

func TestDupes_DismissAndUndismiss(t *testing.T) {
	db := seedDupes(t)

	_, _, code := crm(t, db, "dupes", "dismiss", "2", "1")
	require.Equal(t, 0, code)
	assert.Empty(t, dupeGroups(t, db))

	_, _, code = crm(t, db, "dupes", "undismiss", "1", "2")
	require.Equal(t, 0, code)
	assert.Len(t, dupeGroups(t, db), 1)

	_, _, code = crm(t, db, "dupes", "undismiss", "1", "2")
	assert.Equal(t, 3, code, "no dismissal to undo")
	_, _, code = crm(t, db, "dupes", "dismiss", "1", "1")
	assert.Equal(t, 2, code)
	_, _, code = crm(t, db, "dupes", "dismiss", "1", "99")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, db, "dupes", "dismiss", "x", "2")
	assert.Equal(t, 2, code)
}

func TestPersonMerge(t *testing.T) {
	db := seedDupes(t)

	// Dry run reports and changes nothing.
	out, stderr, code := crm(t, db, "person", "merge", "1", "2", "--dry-run", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &res), out)
	assert.Equal(t, true, res["dry_run"])
	assert.Contains(t, stderr, "Dry run")
	assert.Len(t, dupeGroups(t, db), 1, "still a duplicate group")

	// Real merge with a per-field choice.
	out, stderr, code = crm(t, db, "person", "merge", "1", "2", "--prefer", "title=2", "--prefer", "company=2", "-f", "json")
	require.Equal(t, 0, code, stderr)
	require.NoError(t, json.Unmarshal([]byte(out), &res), out)
	assert.Equal(t, false, res["dry_run"])

	out, _, code = crm(t, db, "person", "show", "1", "-f", "json")
	require.Equal(t, 0, code)
	row := jsonRows(t, out)[0]
	assert.Equal(t, "Acme", row["company"])
	assert.Len(t, row["emails"], 2)

	_, _, code = crm(t, db, "person", "show", "2")
	assert.Equal(t, 3, code, "dropped person is archived")
	assert.Empty(t, dupeGroups(t, db))

	// Errors.
	_, _, code = crm(t, db, "person", "merge", "1", "1")
	assert.Equal(t, 2, code)
	_, _, code = crm(t, db, "person", "merge", "1", "3", "--prefer", "shoe=3")
	assert.Equal(t, 2, code)
	_, _, code = crm(t, db, "person", "merge", "1", "3", "--prefer", "title")
	assert.Equal(t, 2, code)
	_, _, code = crm(t, db, "person", "merge", "1", "99")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, db, "person", "merge", "1")
	assert.NotEqual(t, 0, code)

	// Table output lists changes.
	out, _, code = crm(t, db, "person", "merge", "4", "5", "--dry-run", "-f", "table")
	assert.Equal(t, 0, code)
	assert.NotContains(t, out, "panic")
}
