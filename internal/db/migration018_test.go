package db_test

import (
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The saved view migration adds the table with a soft delete, a name that is unique among live rows
// only, and the three example views, off by default. The file is named 018 because migrations 015 to 017
// belong to other branches, the version a database gets is the position of the file.
func TestMigration018_SavedViewTableAndExamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	assert.Equal(t, 3, scalar(t, d, `SELECT COUNT(*) FROM saved_view WHERE example = 1 AND show_in_tabs = 0 AND deleted_at IS NULL`))
	rows, err := d.Query(`SELECT name, kind FROM saved_view ORDER BY position`)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var n, k string
		require.NoError(t, rows.Scan(&n, &k))
		got = append(got, n+" ("+k+")")
	}
	assert.Equal(t, []string{"Overdue follow-up (person)", "Open deals by stage (deal)", "Tasks due this week (task)"}, got)

	t.Run("name is unique among live rows, ignoring case", func(t *testing.T) {
		_, err := d.Exec(`INSERT INTO saved_view (uuid, name, kind) VALUES ('x1', 'overdue FOLLOW-UP', 'person')`)
		assert.Error(t, err)
	})
	t.Run("a deleted view frees its name", func(t *testing.T) {
		_, err := d.Exec(`UPDATE saved_view SET deleted_at = datetime('now') WHERE name = 'Overdue follow-up'`)
		require.NoError(t, err)
		_, err = d.Exec(`INSERT INTO saved_view (uuid, name, kind) VALUES ('x2', 'Overdue follow-up', 'person')`)
		assert.NoError(t, err)
	})
	t.Run("kind is one of the six", func(t *testing.T) {
		_, err := d.Exec(`INSERT INTO saved_view (uuid, name, kind) VALUES ('x3', 'Bad', 'widget')`)
		assert.Error(t, err)
	})
	t.Run("the example queries are valid JSON", func(t *testing.T) {
		assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM saved_view WHERE NOT json_valid(query)`))
	})
}
