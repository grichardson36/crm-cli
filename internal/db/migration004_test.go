package db_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func migrationSection(t *testing.T, file, section string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("migrations", file))
	require.NoError(t, err)
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	up, down, ok := strings.Cut(s, "\n-- down\n")
	require.True(t, ok)
	if section == "down" {
		return down
	}
	return strings.TrimPrefix(up, "-- up\n")
}

// openAtV2 builds a database as it looked after migration 002 and loads sample data.
func openAtV2(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v2.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = raw.Exec(migrationSection(t, "001_initial.sql", "up"))
	require.NoError(t, err)
	_, err = raw.Exec(migrationSection(t, "002_dex_parity.sql", "up"))
	require.NoError(t, err)
	stmts := []string{
		`INSERT INTO people (uuid, first_name, keep_in_touch, created_at) VALUES ('p1', 'Cadence', '1 mon', '2020-01-01 00:00:00')`,
		`INSERT INTO people (uuid, first_name, created_at) VALUES ('p2', 'NoCadence', '2020-01-01 00:00:00')`,
		`INSERT INTO tags (name) VALUES ('vip')`,
		`INSERT INTO interactions (uuid, type, subject, content, occurred_at) VALUES ('i1', 'call', 'Hello call', 'discussed pelicans', '2026-01-10 10:00:00')`,
		`INSERT INTO interactions (uuid, type, subject, occurred_at) VALUES ('i2', 'note', 'A note', '2026-01-11 10:00:00')`,
		`INSERT INTO interaction_people (interaction_id, person_id) VALUES (1, 1), (2, 1), (2, 2)`,
		`PRAGMA user_version = 2`,
	}
	for _, s := range stmts {
		_, err = raw.Exec(s)
		require.NoError(t, err, s)
	}
	require.NoError(t, raw.Close())
	return path
}

func TestMigration004_UpgradeFromV2(t *testing.T) {
	path := openAtV2(t)
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM interactions`).Scan(&n))
	assert.Equal(t, 2, n, "interactions survive the table rebuild")
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM interaction_people`).Scan(&n))
	assert.Equal(t, 3, n, "interaction links survive the table rebuild")
	require.NoError(t, d.QueryRow(`SELECT id FROM interactions WHERE uuid = 'i2'`).Scan(&n))
	assert.Equal(t, 2, n, "ids are preserved")

	// FTS is rebuilt and its triggers still work.
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM interactions_fts WHERE interactions_fts MATCH 'pelicans'`).Scan(&n))
	assert.Equal(t, 1, n)
	_, err = d.Exec(`INSERT INTO interactions (uuid, type, content) VALUES ('i3', 'standup', 'zebra sync')`)
	require.NoError(t, err, "custom types are no longer blocked by a CHECK")
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM interactions_fts WHERE interactions_fts MATCH 'zebra'`).Scan(&n))
	assert.Equal(t, 1, n)

	// Seeds.
	rows, err := d.Query(`SELECT name, counts_as_contact FROM note_types ORDER BY name`)
	require.NoError(t, err)
	got := map[string]int{}
	for rows.Next() {
		var name string
		var c int
		require.NoError(t, rows.Scan(&name, &c))
		got[name] = c
	}
	require.NoError(t, rows.Close())
	assert.Equal(t, map[string]int{"call": 1, "email": 1, "meeting": 1, "message": 1, "note": 0}, got)

	// Name is unique case-insensitively.
	_, err = d.Exec(`INSERT INTO note_types (uuid, name) VALUES ('x', 'CALL')`)
	assert.Error(t, err)

	// Tags got a uuid and timestamps.
	var tagUUID, created, updated sql.NullString
	require.NoError(t, d.QueryRow(`SELECT uuid, created_at, updated_at FROM tags WHERE name = 'vip'`).Scan(&tagUUID, &created, &updated))
	assert.True(t, tagUUID.Valid && tagUUID.String != "")
	assert.True(t, created.Valid && updated.Valid)

	// Device id is a UUID.
	var device string
	require.NoError(t, d.QueryRow(`SELECT value FROM meta WHERE key = 'device_id'`).Scan(&device))
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), device)

	// Foreign keys still hold after the rebuild.
	var fk int
	require.NoError(t, d.QueryRow(`PRAGMA foreign_keys`).Scan(&fk))
	assert.Equal(t, 1, fk)
	_, err = d.Exec(`INSERT INTO interaction_people (interaction_id, person_id) VALUES (999, 1)`)
	assert.Error(t, err)

	var version int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.Equal(t, 7, version)
}

func TestMigration004_BackfillsKeepInTouchSetAt(t *testing.T) {
	path := openAtV2(t)
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	var setAt sql.NullString
	require.NoError(t, d.QueryRow(`SELECT keep_in_touch_set_at FROM people WHERE uuid = 'p1'`).Scan(&setAt))
	assert.True(t, setAt.Valid, "a person with a cadence is stamped at migration time")
	require.NoError(t, d.QueryRow(`SELECT keep_in_touch_set_at FROM people WHERE uuid = 'p2'`).Scan(&setAt))
	assert.False(t, setAt.Valid, "a person without a cadence is left alone")

	// The call logged for p1 (2026-01-10) is the last contact, so the person is due a month after it.
	var due string
	require.NoError(t, d.QueryRow(`SELECT due_at FROM keep_in_touch_status WHERE person_id = 1`).Scan(&due))
	assert.Equal(t, "2026-02-10 10:00:00", due)
}

func TestMigration004_KeepInTouchFirstDue(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	_, err = d.Exec(`INSERT INTO people (uuid, first_name, keep_in_touch, keep_in_touch_set_at, created_at)
		VALUES ('a', 'SetAt', '1 mon', '2026-06-01 00:00:00', '2020-01-01 00:00:00')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (uuid, first_name, keep_in_touch, created_at)
		VALUES ('b', 'CreatedOnly', '1 mon', '2020-01-01 00:00:00')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (uuid, first_name, keep_in_touch, keep_in_touch_set_at, last_contacted_at, created_at)
		VALUES ('c', 'Seen', '1 mon', '2026-06-01 00:00:00', '2026-07-01 00:00:00', '2020-01-01 00:00:00')`)
	require.NoError(t, err)

	due := func(id int) string {
		var s string
		require.NoError(t, d.QueryRow(`SELECT due_at FROM keep_in_touch_status WHERE person_id = ?`, id).Scan(&s))
		return s
	}
	assert.Equal(t, "2026-07-01 00:00:00", due(1), "never contacted: set_at beats created_at")
	assert.Equal(t, "2020-02-01 00:00:00", due(2), "no set_at falls back to created_at")
	assert.Equal(t, "2026-08-01 00:00:00", due(3), "last_seen beats set_at")
}

func TestMigration004_CountsAsContact(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	_, err = d.Exec(`INSERT INTO people (uuid, first_name, keep_in_touch, keep_in_touch_set_at, created_at)
		VALUES ('a', 'A', '1 mon', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	require.NoError(t, err)
	stmts := []string{
		`INSERT INTO note_types (uuid, name, counts_as_contact) VALUES ('t1', 'standup', 1)`,
		`INSERT INTO note_types (uuid, name, counts_as_contact) VALUES ('t2', 'journal', 0)`,
		`INSERT INTO interactions (uuid, type, occurred_at) VALUES ('i1', 'standup', '2026-02-01 00:00:00')`,
		`INSERT INTO interactions (uuid, type, occurred_at) VALUES ('i2', 'journal', '2026-03-01 00:00:00')`,
		`INSERT INTO interactions (uuid, type, occurred_at) VALUES ('i3', 'note', '2026-04-01 00:00:00')`,
		`INSERT INTO interactions (uuid, type, occurred_at) VALUES ('i4', 'unlisted', '2026-02-15 00:00:00')`,
		`INSERT INTO interaction_people (interaction_id, person_id) VALUES (1, 1), (2, 1), (3, 1), (4, 1)`,
	}
	for _, s := range stmts {
		_, err = d.Exec(s)
		require.NoError(t, err, s)
	}
	var lastSeen string
	require.NoError(t, d.QueryRow(`SELECT last_seen FROM keep_in_touch_status WHERE person_id = 1`).Scan(&lastSeen))
	assert.Equal(t, "2026-02-15 00:00:00", lastSeen, "journal and note are ignored, unknown types count by default")
}

func TestMigration004_Down(t *testing.T) {
	path := openAtV2(t)
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	_, err = d.Exec(`INSERT INTO interactions (uuid, type) VALUES ('i9', 'standup')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO interaction_people (interaction_id, person_id) VALUES (3, 1)`)
	require.NoError(t, err)

	_, err = d.Exec(migrationSection(t, "004_kit_notetypes_archive.sql", "down"))
	require.NoError(t, err)

	var n int
	for _, table := range []string{"note_types", "meta", "tombstones"} {
		err = d.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
		assert.Error(t, err, "%s is dropped", table)
	}
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM interactions`).Scan(&n))
	assert.Equal(t, 3, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM interaction_people`).Scan(&n))
	assert.Equal(t, 4, n)
	var typ string
	require.NoError(t, d.QueryRow(`SELECT type FROM interactions WHERE uuid = 'i9'`).Scan(&typ))
	assert.Equal(t, "note", typ, "custom types collapse to note")
	_, err = d.Exec(`INSERT INTO interactions (uuid, type) VALUES ('bad', 'standup')`)
	assert.Error(t, err, "the CHECK is back")
	_, err = d.Exec(`SELECT keep_in_touch_set_at FROM people`)
	assert.Error(t, err, "column dropped")
	_, err = d.Exec(`SELECT uuid FROM tags`)
	assert.Error(t, err, "tag uuid dropped")

	// The 002 view is back: a never-contacted person is due from created_at.
	var due string
	require.NoError(t, d.QueryRow(`SELECT due_at FROM keep_in_touch_status WHERE person_id = 1`).Scan(&due))
	assert.Equal(t, "2026-02-10 10:00:00", due)
}
