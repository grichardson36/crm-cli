package db_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration010_CaldavResources(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	var version int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.GreaterOrEqual(t, version, 10)

	_, err = d.Exec(`INSERT INTO caldav_resources (source, href, uid, etag) VALUES ('caldav', '/c/a.ics', 'a', 'e1')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO caldav_resources (source, href, uid) VALUES ('caldav', '/c/a.ics', 'a')`)
	assert.Error(t, err, "one row per source and href")
	var hash, js string
	require.NoError(t, d.QueryRow(`SELECT base_hash, base_json FROM caldav_resources`).Scan(&hash, &js))
	assert.Equal(t, "", hash)
	assert.Equal(t, "{}", js)

	_, err = d.Exec(migrationSection(t, "010_caldav.sql", "down"))
	require.NoError(t, err)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'caldav_resources'`).Scan(&n))
	assert.Equal(t, 0, n)
}
