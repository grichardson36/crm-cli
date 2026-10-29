package db_test

import (
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 013 adds the vocabulary table and seeds the deal roles. A schema 12 database keeps its data
// and its existing free-text deal roles.
func TestMigration013_AddsVocabularyWithSeededDealRoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v12.db")
	d, err := db.OpenUpTo(path, 12)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO deals (uuid, title) VALUES ('d1', 'Big deal')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('l1', 1, 1, 'Board observer')`)
	require.NoError(t, err)
	require.NoError(t, d.Close())

	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, db.LatestVersion(), scalar(t, d, `PRAGMA user_version`))
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM people`))

	vs, ok := model.VocabularyNamed(model.DealRoleKind)
	require.True(t, ok)
	rows, err := d.Query(`SELECT name FROM vocabulary WHERE kind = 'deal_role' AND archived = 0 ORDER BY position`)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		got = append(got, n)
	}
	assert.Equal(t, vs.Seeds, got, "the migration seeds exactly model.Vocabularies[deal_role].Seeds, in order")

	var role string
	require.NoError(t, d.QueryRow(`SELECT role FROM deal_person`).Scan(&role))
	assert.Equal(t, "Board observer", role, "an existing free-text role is untouched")

	for _, tc := range []struct{ name, sql string }{
		{"names are unique per kind ignoring case", `INSERT INTO vocabulary (uuid, kind, name) VALUES ('x1', 'deal_role', 'CHAMPION')`},
		{"archived is 0 or 1", `INSERT INTO vocabulary (uuid, kind, name, archived) VALUES ('x2', 'deal_role', 'new', 2)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := d.Exec(tc.sql)
			assert.Error(t, err)
		})
	}
	t.Run("the same name under another kind is fine", func(t *testing.T) {
		_, err := d.Exec(`INSERT INTO vocabulary (uuid, kind, name) VALUES ('x3', 'other', 'champion')`)
		assert.NoError(t, err)
	})
}
