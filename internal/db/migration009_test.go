package db_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration009_ChunksAndJournal(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	var version int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.GreaterOrEqual(t, version, 9)

	_, err = d.Exec(`INSERT INTO tasks (uuid, title) VALUES ('t1', 'x')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO task_chunks (task_id, start_utc, end_utc) VALUES (1, '2026-10-12T14:00:00Z', '2026-10-12T15:00:00Z')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO task_chunks (task_id, start_utc, end_utc) VALUES (1, '2026-10-12T14:00:00Z', '2026-10-12T16:00:00Z')`)
	assert.Error(t, err, "one chunk per start")
	_, err = d.Exec(`INSERT INTO plan_journal (uuid, note, entries_json) VALUES ('j1', 'n', '[]')`)
	require.NoError(t, err)
	var undone int
	require.NoError(t, d.QueryRow(`SELECT undone FROM plan_journal`).Scan(&undone))
	assert.Equal(t, 0, undone)

	_, err = d.Exec(`DELETE FROM tasks WHERE id = 1`)
	require.NoError(t, err)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM task_chunks`).Scan(&n))
	assert.Equal(t, 0, n, "chunks go with their task")

	_, err = d.Exec(migrationSection(t, "009_plan.sql", "down"))
	require.NoError(t, err)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('task_chunks', 'plan_journal')`).Scan(&n))
	assert.Equal(t, 0, n)
}
