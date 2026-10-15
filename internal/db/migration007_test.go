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

// A v6 database keeps its tags when taggings is rebuilt to allow events.
func TestMigration007_UpgradeFromV6KeepsTaggings(t *testing.T) {
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
	require.NoError(t, raw.Close())

	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	var version, n int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.Equal(t, 14, version)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM taggings WHERE entity_type = 'person'`).Scan(&n))
	assert.Equal(t, 2, n, "existing taggings survive the rebuild")

	// Events can be tagged now, other types still cannot.
	_, err = d.Exec(`INSERT INTO events (uuid, title, start_utc, end_utc) VALUES ('e1', 'Lunch', '2026-10-16T17:30:00Z', '2026-10-16T18:30:00Z')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'event', 1)`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES (1, 'planet', 1)`)
	assert.Error(t, err)

	// The index on taggings was recreated and deleting a tag still cascades.
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'idx_taggings_entity'`).Scan(&n))
	assert.Equal(t, 1, n)
	_, err = d.Exec(`DELETE FROM tags WHERE name = 'vip'`)
	require.NoError(t, err)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM taggings WHERE tag_id = 1`).Scan(&n))
	assert.Equal(t, 0, n)
}

func TestMigration007_EventsConstraintsAndDown(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	// A timed event needs instants, an all-day event needs dates.
	_, err = d.Exec(`INSERT INTO events (uuid, title) VALUES ('e0', 'no times')`)
	assert.Error(t, err)
	_, err = d.Exec(`INSERT INTO events (uuid, title, all_day, start_utc, end_utc) VALUES ('e0', 'all-day with instants', 1, 'a', 'b')`)
	assert.Error(t, err)
	_, err = d.Exec(`INSERT INTO events (uuid, title, all_day, start_date, end_date) VALUES ('e1', 'holiday', 1, '2026-11-26', '2026-11-27')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO events (uuid, title, start_utc, end_utc, status) VALUES ('e2', 'x', 'a', 'b', 'maybe')`)
	assert.Error(t, err, "status is checked")

	_, err = d.Exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Jane')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO event_people (event_id, person_id) VALUES (1, 1)`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO event_reminders (event_id, minutes_before) VALUES (1, 15)`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO event_reminders (event_id, minutes_before, method) VALUES (1, -1, 'display')`)
	assert.Error(t, err, "no negative reminders")

	// Full text search follows inserts, updates and deletes.
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM events_fts WHERE events_fts MATCH 'holiday'`).Scan(&n))
	assert.Equal(t, 1, n)
	_, err = d.Exec(`UPDATE events SET title = 'vacation' WHERE id = 1`)
	require.NoError(t, err)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM events_fts WHERE events_fts MATCH 'holiday'`).Scan(&n))
	assert.Equal(t, 0, n)

	// Removing the event removes its children.
	_, err = d.Exec(`DELETE FROM events WHERE id = 1`)
	require.NoError(t, err)
	require.NoError(t, d.QueryRow(`SELECT (SELECT COUNT(*) FROM event_people) + (SELECT COUNT(*) FROM event_reminders)`).Scan(&n))
	assert.Equal(t, 0, n)

	_, err = d.Exec(migrationSection(t, "011_event_guests.sql", "down"))
	require.NoError(t, err)
	down := migrationSection(t, "007_events.sql", "down")
	_, err = d.Exec(down)
	require.NoError(t, err)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'event%'`).Scan(&n))
	assert.Equal(t, 0, n)
}
