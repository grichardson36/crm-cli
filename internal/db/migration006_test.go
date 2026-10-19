package db_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration006_DavTables(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	var version int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.Equal(t, 9, version)

	_, err = d.Exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Jane')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO dav_state (person_id, base_json, etag, extra_vcard, synced_at) VALUES (1, '{}', 'abc', 'x', datetime('now'))`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO sync_conflicts (uuid, person_id, field, crm_value, device_value, winner) VALUES ('c1', 1, 'title', 'a', 'b', 'device')`)
	require.NoError(t, err)

	var undone int
	var created string
	require.NoError(t, d.QueryRow(`SELECT undone, created_at FROM sync_conflicts WHERE id = 1`).Scan(&undone, &created))
	assert.Equal(t, 0, undone)
	assert.NotEmpty(t, created)

	// Purging a person takes their device state and conflicts with them.
	_, err = d.Exec(`DELETE FROM people WHERE id = 1`)
	require.NoError(t, err)
	var n int
	require.NoError(t, d.QueryRow(`SELECT (SELECT COUNT(*) FROM dav_state) + (SELECT COUNT(*) FROM sync_conflicts)`).Scan(&n))
	assert.Equal(t, 0, n)

	down := migrationSection(t, "006_dav.sql", "down")
	_, err = d.Exec(down)
	require.NoError(t, err)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('dav_state', 'sync_conflicts')`).Scan(&n))
	assert.Equal(t, 0, n)
}
