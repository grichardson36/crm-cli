package db_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_InMemory(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	// Verify tables exist
	tables := []string{"people", "organizations", "interactions", "deals", "tasks", "tags", "taggings", "custom_fields", "relationships"}
	for _, table := range tables {
		var name string
		err := d.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		assert.NoError(t, err, "table %s should exist", table)
		assert.Equal(t, table, name)
	}
}

func TestOpen_FTS5Tables(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	fts := []string{"people_fts", "organizations_fts", "interactions_fts", "deals_fts"}
	for _, name := range fts {
		var found string
		err := d.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&found)
		assert.NoError(t, err, "FTS table %s should exist", name)
	}
}

func TestOpen_Pragmas(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	var journalMode string
	_ = d.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	// In-memory databases use "memory" journal mode, not WAL
	assert.Contains(t, []string{"wal", "memory"}, journalMode)

	var fk int
	_ = d.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	assert.Equal(t, 1, fk)
}

func TestOpen_MigrationVersion(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	var version int
	_ = d.QueryRow("PRAGMA user_version").Scan(&version)
	assert.Equal(t, 8, version)
}

func TestMigration002_BackfillsFromV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")

	// Build a version-1 database by hand and add data before upgrading.
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	v1, err := os.ReadFile("migrations/001_initial.sql")
	require.NoError(t, err)
	up := strings.SplitN(strings.TrimPrefix(string(v1), "-- up"), "-- down", 2)[0]
	_, err = raw.Exec(up)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO people (uuid, first_name, email, phone) VALUES ('u1', 'Jane', 'jane@example.com', '+15551234567')`)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO people (uuid, first_name) VALUES ('u2', 'NoContact')`)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO tasks (uuid, title, person_id) VALUES ('t1', 'Follow up', 1)`)
	require.NoError(t, err)
	_, err = raw.Exec(`PRAGMA user_version = 1`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM person_emails WHERE person_id = 1 AND email = 'jane@example.com' AND is_primary = 1`).Scan(&n))
	assert.Equal(t, 1, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM person_phones WHERE person_id = 1 AND phone_number = '+15551234567'`).Scan(&n))
	assert.Equal(t, 1, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM person_emails WHERE person_id = 2`).Scan(&n))
	assert.Equal(t, 0, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM task_people WHERE task_id = 1 AND person_id = 1`).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestMigration002_KeepInTouchView(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	_, err = d.Exec(`INSERT INTO people (uuid, first_name, keep_in_touch, created_at) VALUES ('a', 'Monthly', '1 mon', '2026-01-01 00:00:00')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (uuid, first_name, keep_in_touch, created_at) VALUES ('b', 'Never', 'never', '2026-01-01 00:00:00')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (uuid, first_name, created_at) VALUES ('c', 'Unset', '2026-01-01 00:00:00')`)
	require.NoError(t, err)
	// A call on Feb 10 moves person 1's due date to Mar 10. A note must not count.
	_, err = d.Exec(`INSERT INTO interactions (uuid, type, occurred_at) VALUES ('i1', 'call', '2026-02-10 09:00:00')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO interactions (uuid, type, occurred_at) VALUES ('i2', 'note', '2026-03-01 09:00:00')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO interaction_people (interaction_id, person_id) VALUES (1, 1), (2, 1)`)
	require.NoError(t, err)

	var count int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM keep_in_touch_status`).Scan(&count))
	assert.Equal(t, 1, count, "only people with a cadence other than never appear")

	var lastSeen, dueAt string
	require.NoError(t, d.QueryRow(`SELECT last_seen, due_at FROM keep_in_touch_status WHERE person_id = 1`).Scan(&lastSeen, &dueAt))
	assert.Equal(t, "2026-02-10 09:00:00", lastSeen)
	assert.Equal(t, "2026-03-10 09:00:00", dueAt)
}

func TestMigration002_Constraints(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	_, err = d.Exec(`INSERT INTO people (uuid, first_name) VALUES ('a', 'A')`)
	require.NoError(t, err)

	_, err = d.Exec(`UPDATE people SET keep_in_touch = 'weekly' WHERE id = 1`)
	assert.Error(t, err, "invalid cadence rejected")
	_, err = d.Exec(`UPDATE people SET birthday_month = 13 WHERE id = 1`)
	assert.Error(t, err, "invalid month rejected")
	_, err = d.Exec(`INSERT INTO person_handles (person_id, kind, value) VALUES (1, 'myspace', 'x')`)
	assert.Error(t, err, "unknown handle kind rejected")
	_, err = d.Exec(`INSERT INTO person_handles (person_id, kind, value) VALUES (1, 'linkedin', 'jane')`)
	assert.NoError(t, err)
	_, err = d.Exec(`INSERT INTO person_handles (person_id, kind, value) VALUES (1, 'linkedin', 'jane2')`)
	assert.Error(t, err, "one handle per kind per person")
}
