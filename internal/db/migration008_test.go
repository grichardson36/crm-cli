package db_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration008_TaskSchedulingAndHours(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	var version int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.Equal(t, 10, version)

	// Existing tasks keep working and get the defaults.
	_, err = d.Exec(`INSERT INTO tasks (uuid, title) VALUES ('t1', 'Old task')`)
	require.NoError(t, err)
	var auto, hard, locked int
	var name string
	var dur *int
	require.NoError(t, d.QueryRow(`SELECT auto_schedule, deadline_hard, scheduled_locked, schedule_name, duration_min FROM tasks`).Scan(&auto, &hard, &locked, &name, &dur))
	assert.Equal(t, [3]int{0, 0, 0}, [3]int{auto, hard, locked})
	assert.Equal(t, "work", name)
	assert.Nil(t, dur)

	_, err = d.Exec(`UPDATE tasks SET duration_min = 0`)
	assert.Error(t, err, "a duration is positive or empty")
	_, err = d.Exec(`UPDATE tasks SET min_chunk_min = -5`)
	assert.Error(t, err)

	_, err = d.Exec(`INSERT INTO availability_rules (uuid, name, tz, weekly_json) VALUES ('r1', 'work', 'UTC', '{}')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO availability_rules (uuid, name, tz, weekly_json) VALUES ('r2', 'work', 'UTC', '{}')`)
	assert.Error(t, err, "rule names are unique")
	_, err = d.Exec(`INSERT INTO availability_overrides (rule_id, date, intervals_json) VALUES (1, '2026-12-25', '[]')`)
	require.NoError(t, err)
	_, err = d.Exec(`DELETE FROM availability_rules WHERE id = 1`)
	require.NoError(t, err)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM availability_overrides`).Scan(&n))
	assert.Equal(t, 0, n, "overrides go with their rule")

	down := migrationSection(t, "008_task_scheduling.sql", "down")
	_, err = d.Exec(down)
	require.NoError(t, err)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('tasks') WHERE name IN ('duration_min', 'schedule_name')`).Scan(&n))
	assert.Equal(t, 0, n)
}
