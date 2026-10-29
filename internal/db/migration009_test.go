package db_test

import (
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 009 adds the native organization fields. A schema 8 database keeps its organizations
// and gets the new columns empty.
func TestMigration009_AddsNativeOrganizationFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v8.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	// Step back to schema 8 the way it was: drop the columns, set the version.
	_, err = d.Exec(migrationSection(t, "013_vocabulary.sql", "down"))
	require.NoError(t, err)
	_, err = d.Exec(migrationSection(t, "012_name_aliases.sql", "down"))
	require.NoError(t, err)
	_, err = d.Exec(migrationSection(t, "011_verbs_undo.sql", "down"))
	require.NoError(t, err)
	_, err = d.Exec(migrationSection(t, "010_company_ignore.sql", "down"))
	require.NoError(t, err)
	_, err = d.Exec(migrationSection(t, "009_org_fields.sql", "down"))
	require.NoError(t, err)
	_, err = d.Exec("PRAGMA user_version = 8")
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO organizations (uuid, name, domain, industry, notes) VALUES ('o1', 'Acme', 'acme.com', 'Software', 'n')`)
	require.NoError(t, err)
	require.NoError(t, d.Close())

	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	var version int
	require.NoError(t, d.QueryRow("PRAGMA user_version").Scan(&version))
	assert.Equal(t, db.LatestVersion(), version)

	var name, domain, notes string
	var website, size, city, region, country, linkedin, description, logo *string
	require.NoError(t, d.QueryRow(`SELECT name, domain, notes, website, size, city, region, country, linkedin_url, description, logo_path
		FROM organizations WHERE uuid = 'o1'`).Scan(&name, &domain, &notes, &website, &size, &city, &region, &country, &linkedin, &description, &logo))
	assert.Equal(t, []string{"Acme", "acme.com", "n"}, []string{name, domain, notes})
	for _, v := range []*string{website, size, city, region, country, linkedin, description, logo} {
		assert.Nil(t, v)
	}
	_, err = d.Exec(`UPDATE organizations SET website = 'https://acme.com', size = '51-200', city = 'Austin', region = 'TX', country = 'US' WHERE uuid = 'o1'`)
	require.NoError(t, err)
}
