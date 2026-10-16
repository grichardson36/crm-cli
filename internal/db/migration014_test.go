package db_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration014_RemindersSent(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	_, err = d.Exec(`INSERT INTO events (uuid, title, start_utc, end_utc) VALUES ('e1', 'x', 'a', 'b')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO reminders_sent (event_id, instance_start, minutes_before, method) VALUES (1, '2026-10-12T15:00:00Z', 10, 'display')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO reminders_sent (event_id, instance_start, minutes_before, method) VALUES (1, '2026-10-12T15:00:00Z', 10, 'display')`)
	assert.Error(t, err, "once per reminder")
	_, err = d.Exec(`DELETE FROM events WHERE id = 1`)
	require.NoError(t, err)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM reminders_sent`).Scan(&n))
	assert.Equal(t, 0, n, "marks go with their event")
	_, err = d.Exec(migrationSection(t, "014_reminders_sent.sql", "down"))
	require.NoError(t, err)
}
