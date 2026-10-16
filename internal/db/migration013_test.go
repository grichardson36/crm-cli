package db_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration013_BookingTables(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	_, err = d.Exec(`INSERT INTO booking_links (uuid, name, title, slug) VALUES ('u1', 'Intro', 'Intro', 'intro')`)
	require.NoError(t, err)
	var dur, notice, step int
	var durations string
	require.NoError(t, d.QueryRow(`SELECT durations_json, min_notice_min, slot_step_min, horizon_days FROM booking_links`).Scan(&durations, &notice, &step, &dur))
	assert.Equal(t, []any{"[30]", 240, 30, 14}, []any{durations, notice, step, dur}, "defaults")

	for _, bad := range []string{
		`INSERT INTO booking_links (uuid, name, title, slug) VALUES ('u2', 'x', 'x', 'intro')`,
		`INSERT INTO booking_links (uuid, name, title, slug, slot_step_min) VALUES ('u3', 'x', 'x', 'x3', 0)`,
		`INSERT INTO booking_links (uuid, name, title, slug, max_per_day) VALUES ('u4', 'x', 'x', 'x4', 0)`,
		`INSERT INTO booking_links (uuid, name, title, slug, location_kind) VALUES ('u5', 'x', 'x', 'x5', 'teleport')`,
		`INSERT INTO bookings (uuid, link_id, invitee_email, start_utc, end_utc, status) VALUES ('b1', 1, 'a@x.com', 'a', 'b', 'maybe')`,
		`INSERT INTO bookings (uuid, link_id, invitee_email, start_utc, end_utc) VALUES ('b2', 99, 'a@x.com', 'a', 'b')`,
	} {
		_, err = d.Exec(bad)
		assert.Error(t, err, bad)
	}
	_, err = d.Exec(`INSERT INTO bookings (uuid, link_id, invitee_email, start_utc, end_utc) VALUES ('b3', 1, 'a@x.com', '2026-10-12T15:00:00Z', '2026-10-12T15:30:00Z')`)
	require.NoError(t, err)
	var status string
	require.NoError(t, d.QueryRow(`SELECT status FROM bookings`).Scan(&status))
	assert.Equal(t, "confirmed", status)

	_, err = d.Exec(migrationSection(t, "013_booking.sql", "down"))
	require.NoError(t, err)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('booking_links', 'bookings')`).Scan(&n))
	assert.Equal(t, 0, n)
}
