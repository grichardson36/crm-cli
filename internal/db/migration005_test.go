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

func TestMigration005_BackfillsDexIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v4.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	for _, f := range []string{"001_initial.sql", "002_dex_parity.sql", "003_dedupe.sql", "004_kit_notetypes_archive.sql"} {
		_, err = raw.Exec(migrationSection(t, f, "up"))
		require.NoError(t, err, f)
	}
	stmts := []string{
		`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Jane'), ('p2', 'Bob'), ('p3', 'NoDex')`,
		`INSERT INTO custom_fields (entity_type, entity_id, field_name, field_value) VALUES
			('person', 1, 'dex_id', 'dex-aaa'), ('person', 2, 'dex_id', 'dex-bbb'), ('person', 1, 'Discord', 'jd#1'),
			('organization', 3, 'dex_id', 'not-a-person')`,
		`PRAGMA user_version = 4`,
	}
	for _, s := range stmts {
		_, err = raw.Exec(s)
		require.NoError(t, err, s)
	}
	require.NoError(t, raw.Close())

	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	var version, n int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.Equal(t, 13, version)

	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM external_ids`).Scan(&n))
	assert.Equal(t, 2, n)
	var source, entity, localUUID string
	var localID int64
	require.NoError(t, d.QueryRow(`SELECT source, entity, local_id, local_uuid FROM external_ids WHERE external_id = 'dex-aaa'`).
		Scan(&source, &entity, &localID, &localUUID))
	assert.Equal(t, "dex", source)
	assert.Equal(t, "person", entity)
	assert.Equal(t, int64(1), localID)
	assert.Equal(t, "p1", localUUID)

	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM custom_fields WHERE field_name = 'dex_id'`).Scan(&n))
	assert.Equal(t, 3, n, "custom field rows stay")

	for _, table := range []string{"sync_runs", "sync_cursors", "sync_inbox", "field_versions"} {
		require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n), table)
	}

	// Constraints: one external record per local row and source, and the reverse.
	_, err = d.Exec(`INSERT INTO external_ids (uuid, entity, local_id, local_uuid, source, external_id) VALUES ('x', 'person', 1, 'p1', 'dex', 'other')`)
	assert.Error(t, err)
	_, err = d.Exec(`INSERT INTO external_ids (uuid, entity, local_id, local_uuid, source, external_id) VALUES ('y', 'person', 3, 'p3', 'dex', 'dex-aaa')`)
	assert.Error(t, err)
}

func TestMigration005_Down(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	_, err = d.Exec(migrationSection(t, "005_external_ids_sync.sql", "down"))
	require.NoError(t, err)
	var n int
	for _, table := range []string{"external_ids", "sync_runs", "sync_cursors", "sync_inbox", "field_versions"} {
		assert.Error(t, d.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n), table)
	}
}
