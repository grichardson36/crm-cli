package db_test

import (
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 011 makes taggings sync-safe link rows (uuid, timestamps, deleted_at), adds the merge
// log and the organization dismissals. A schema 10 database keeps its taggings and their ids.
func TestMigration011_TaggingsKeepRowsAndGainSyncColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v10.db")
	d, err := db.OpenUpTo(path, 10)
	require.NoError(t, err)
	for _, s := range []string{
		`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann'), ('p2', 'Bob')`,
		`INSERT INTO tags (uuid, name) VALUES ('t1', 'vip'), ('t2', 'old')`,
		`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'person', 1), (1, 'person', 2), (2, 'person', 1)`,
	} {
		_, err = d.Exec(s)
		require.NoError(t, err, s)
	}
	require.NoError(t, d.Close())

	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, 14, scalar(t, d, `PRAGMA user_version`))
	assert.Equal(t, 3, scalar(t, d, `SELECT COUNT(*) FROM taggings WHERE deleted_at IS NULL AND uuid <> '' AND updated_at <> ''`))
	assert.Equal(t, 3, scalar(t, d, `SELECT COUNT(DISTINCT uuid) FROM taggings`), "every tagging has its own uuid")
	assert.Equal(t, 2, scalar(t, d, `SELECT id FROM taggings WHERE tag_id = 1 AND entity_id = 2`), "ids are kept")

	// An older INSERT that names three columns still works and gets a uuid.
	_, err = d.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (2, 'person', 2)`)
	require.NoError(t, err)
	assert.Equal(t, 4, scalar(t, d, `SELECT COUNT(DISTINCT uuid) FROM taggings`))

	// One live row per tag and entity, but a soft deleted row may sit beside a live one.
	_, err = d.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'person', 1)`)
	assert.Error(t, err)
	_, err = d.Exec(`UPDATE taggings SET deleted_at = datetime('now') WHERE tag_id = 1 AND entity_id = 1`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'person', 1)`)
	require.NoError(t, err)

	for _, tbl := range []string{"merge_log", "org_dupe_dismissals"} {
		assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM `+tbl), tbl)
	}
}
