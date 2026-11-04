package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-022, S7. Migration 023 (database version 22) adds person_relation.anniversary. A database at
// version 21 with a spouse link and a friend link keeps both, with no date, and takes a date after.
func TestMigration023_AddsAnniversaryToACopyOfAPopulatedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v21.db")
	d, err := db.OpenUpTo(path, 21)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (id, uuid, first_name, last_name) VALUES (1, 'p1', 'Joey', 'Osgood'), (2, 'p2', 'Hannah', 'Osgood'), (3, 'p3', 'Cy', 'Lee')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO person_relation (uuid, a_id, b_id, kind_id) VALUES
		('l1', 1, 2, (SELECT id FROM relation_kind WHERE uuid = 'a0000001-0000-4000-8000-000000000002')),
		('l2', 1, 3, (SELECT id FROM relation_kind WHERE name = 'friend' AND object_type = 'person'))`)
	require.NoError(t, err)
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM pragma_table_info('person_relation') WHERE name = 'anniversary'`), "version 21 has no column")
	require.NoError(t, d.Close())

	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, db.LatestVersion(), scalar(t, d, `PRAGMA user_version`))
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM pragma_table_info('person_relation') WHERE name = 'anniversary'`))
	assert.Equal(t, 2, scalar(t, d, `SELECT COUNT(*) FROM person_relation WHERE anniversary IS NULL`), "existing links keep their rows with no date")

	_, err = d.Exec(`UPDATE person_relation SET anniversary = '2014-06-20' WHERE uuid = 'l1'`)
	require.NoError(t, err)
	var got sql.NullString
	require.NoError(t, d.QueryRow(`SELECT anniversary FROM person_relation WHERE uuid = 'l1'`).Scan(&got))
	assert.Equal(t, "2014-06-20", got.String)
}

func TestMigration023_DownRemovesTheColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	_, err = d.Exec(`INSERT INTO people (id, uuid, first_name) VALUES (1, 'p1', 'A'), (2, 'p2', 'B')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO person_relation (uuid, a_id, b_id, kind_id, anniversary) VALUES ('l1', 1, 2, (SELECT id FROM relation_kind WHERE object_type = 'person' LIMIT 1), '2014-06-20')`)
	require.NoError(t, err)

	_, err = d.Exec(migrationSection(t, "023_spouse_anniversary.sql", "down"))
	require.NoError(t, err)
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM pragma_table_info('person_relation') WHERE name = 'anniversary'`))
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM person_relation`), "the link rows stay")
}
