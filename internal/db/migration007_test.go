package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// 007 rebuilds taggings so a task can carry a tag. Existing tags must survive with their ids.
func TestMigration007_TasksCanBeTaggedAndOldTagsSurvive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	for _, f := range []string{"001_initial.sql", "002_dex_parity.sql", "003_dedupe.sql", "004_kit_notetypes_archive.sql", "005_external_ids_sync.sql", "006_dav.sql"} {
		_, err = raw.Exec(migrationSection(t, f, "up"))
		require.NoError(t, err, f)
	}
	for _, s := range []string{
		`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Jane')`,
		`INSERT INTO tags (name, uuid) VALUES ('vip', 't1'), ('old', 't2')`,
		`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'person', 1), (2, 'person', 1)`,
		`PRAGMA user_version = 6`,
	} {
		_, err = raw.Exec(s)
		require.NoError(t, err, s)
	}
	// Before the migration a task tagging is rejected.
	_, err = raw.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'task', 1)`)
	require.Error(t, err)
	require.NoError(t, raw.Close())

	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	var version, n int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.Equal(t, 7, version, "007 is the 7th file, so the version is its position")

	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM taggings WHERE entity_type = 'person'`).Scan(&n))
	assert.Equal(t, 2, n)
	var id int64
	require.NoError(t, d.QueryRow(`SELECT id FROM taggings WHERE tag_id = 2`).Scan(&id))
	assert.Equal(t, int64(2), id, "row ids are kept")

	_, err = d.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'task', 1)`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'task', 1)`)
	require.Error(t, err, "the unique key survives")
	_, err = d.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'bogus', 1)`)
	require.Error(t, err, "the check still rejects unknown entities")

	// The lookup index is back.
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_taggings_entity'`).Scan(&n))
	assert.Equal(t, 1, n)
}
