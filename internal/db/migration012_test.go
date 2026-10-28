package db_test

import (
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 012 adds the name_aliases table. A schema 11 database keeps its data and gets an empty table.
func TestMigration012_AddsNameAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v11.db")
	d, err := db.OpenUpTo(path, 11)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	require.NoError(t, err)
	require.NoError(t, d.Close())

	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, 13, scalar(t, d, `PRAGMA user_version`))
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM people`))
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM name_aliases`))
	_, err = d.Exec(`INSERT INTO name_aliases (uuid, alias, canonical) VALUES ('a1', 'zeke', 'zachary')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO name_aliases (uuid, alias, canonical) VALUES ('a2', 'zeke', 'zachary')`)
	assert.Error(t, err, "one row per alias")
	_, err = d.Exec(`INSERT INTO name_aliases (uuid, alias, canonical, mode) VALUES ('a3', 'x', 'y', 'bad')`)
	assert.Error(t, err, "mode is add or remove")
}
