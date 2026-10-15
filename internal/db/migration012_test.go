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

// The tasks table is rebuilt to allow ASAP. Everything that points at tasks must survive.
func TestMigration012_AsapKeepsTasksAndTheirLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v11.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	files := []string{"001_initial.sql", "002_dex_parity.sql", "003_dedupe.sql", "004_kit_notetypes_archive.sql", "005_external_ids_sync.sql",
		"006_dav.sql", "007_events.sql", "008_task_scheduling.sql", "009_plan.sql", "010_caldav.sql", "011_event_guests.sql"}
	for _, f := range files {
		_, err = raw.Exec(migrationSection(t, f, "up"))
		require.NoError(t, err, f)
	}
	for _, s := range []string{
		`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Jane'), ('p2', 'Bob')`,
		`INSERT INTO tasks (uuid, title, priority, person_id, due_at, duration_min, auto_schedule, scheduled_start, schedule_name)
		 VALUES ('t1', 'Send deck', 'high', 1, '2026-11-01', 90, 1, '2026-10-14T15:00:00Z', 'deep'), ('t2', 'Call back', 'low', NULL, NULL, NULL, 0, NULL, 'work')`,
		`INSERT INTO task_people (task_id, person_id) VALUES (1, 1), (1, 2)`,
		`INSERT INTO task_chunks (task_id, start_utc, end_utc) VALUES (1, '2026-10-14T15:00:00Z', '2026-10-14T16:00:00Z')`,
		`INSERT INTO external_ids (uuid, entity, local_id, local_uuid, source, external_id) VALUES ('x1', 'task', 1, 't1', 'ics', 'abc')`,
		`PRAGMA user_version = 11`,
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
	assert.Equal(t, 12, version)
	var title, prio, sched string
	var dur int
	require.NoError(t, d.QueryRow(`SELECT title, priority, duration_min, schedule_name FROM tasks WHERE id = 1`).Scan(&title, &prio, &dur, &sched))
	assert.Equal(t, []any{"Send deck", "high", 90, "deep"}, []any{title, prio, dur, sched})
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM task_people WHERE task_id = 1`).Scan(&n))
	assert.Equal(t, 2, n, "task links survive the rebuild")
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM task_chunks WHERE task_id = 1`).Scan(&n))
	assert.Equal(t, 1, n, "placed blocks survive")
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM external_ids WHERE entity = 'task'`).Scan(&n))
	assert.Equal(t, 1, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('idx_tasks_person', 'idx_tasks_deal', 'idx_tasks_due', 'idx_task_people_person')`).Scan(&n))
	assert.Equal(t, 4, n, "indexes are back")

	_, err = d.Exec(`INSERT INTO tasks (uuid, title, priority) VALUES ('t3', 'Now', 'asap')`)
	require.NoError(t, err, "ASAP is allowed")
	_, err = d.Exec(`INSERT INTO tasks (uuid, title, priority) VALUES ('t4', 'Bad', 'urgent')`)
	assert.Error(t, err)
	_, err = d.Exec(`INSERT INTO task_people (task_id, person_id) VALUES (3, 1)`)
	require.NoError(t, err)

	// New ids continue after the old ones.
	var id int64
	require.NoError(t, d.QueryRow(`SELECT id FROM tasks WHERE uuid = 't3'`).Scan(&id))
	assert.Equal(t, int64(3), id)

	// Down: ASAP becomes high, links stay.
	_, err = d.Exec(migrationSection(t, "012_priority_asap.sql", "down"))
	require.NoError(t, err)
	require.NoError(t, d.QueryRow(`SELECT priority FROM tasks WHERE uuid = 't3'`).Scan(&prio))
	assert.Equal(t, "high", prio)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM task_people`).Scan(&n))
	assert.Equal(t, 3, n)
}

func TestMigration011_EventGuests(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	_, err = d.Exec(`INSERT INTO events (uuid, title, start_utc, end_utc) VALUES ('e1', 'x', 'a', 'b')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO event_guests (event_id, email, name) VALUES (1, 'a@x.com', 'A')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO event_guests (event_id, email) VALUES (1, 'a@x.com')`)
	assert.Error(t, err, "one row per address")
	_, err = d.Exec(`DELETE FROM events WHERE id = 1`)
	require.NoError(t, err)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM event_guests`).Scan(&n))
	assert.Equal(t, 0, n)
}
