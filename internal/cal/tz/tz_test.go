package tz

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := Load(name)
	require.NoError(t, err)
	return loc
}

func wall(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{"America/Chicago", "America/Chicago", false},
		{"Europe/London", "Europe/London", false},
		{"UTC", "UTC", false},
		{"Z", "UTC", false},
		{"Mars/Olympus", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := Load(tt.name)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, loc.String())
		})
	}
}

func TestName(t *testing.T) {
	assert.Equal(t, "America/Chicago", Name(mustLoad(t, "America/Chicago")))
	assert.Equal(t, "UTC", Name(time.UTC))
	assert.Equal(t, "UTC", Name(nil))
	assert.Equal(t, "Local", Name(time.Local), "the unnamed device zone keeps its runtime name")
}

// Instant follows RFC 5545 section 3.3.5 for local times that do not exist or occur twice.
func TestInstant(t *testing.T) {
	tests := []struct {
		name string
		zone string
		wall time.Time
		want string
	}{
		// America/Chicago: clocks spring forward 2026-03-08 02:00 -> 03:00 and fall back 2026-11-01 02:00 -> 01:00.
		{"chicago winter", "America/Chicago", wall(2026, 1, 15, 9, 0), "2026-01-15T15:00:00Z"},
		{"chicago summer", "America/Chicago", wall(2026, 7, 15, 9, 0), "2026-07-15T14:00:00Z"},
		{"chicago before the gap", "America/Chicago", wall(2026, 3, 8, 1, 59), "2026-03-08T07:59:00Z"},
		{"chicago in the gap uses the offset before it", "America/Chicago", wall(2026, 3, 8, 2, 30), "2026-03-08T08:30:00Z"},
		{"chicago start of the gap", "America/Chicago", wall(2026, 3, 8, 2, 0), "2026-03-08T08:00:00Z"},
		{"chicago after the gap", "America/Chicago", wall(2026, 3, 8, 3, 0), "2026-03-08T08:00:00Z"},
		{"chicago repeated hour is the first occurrence", "America/Chicago", wall(2026, 11, 1, 1, 30), "2026-11-01T06:30:00Z"},
		{"chicago after the repeat", "America/Chicago", wall(2026, 11, 1, 2, 30), "2026-11-01T08:30:00Z"},
		// Europe/London: spring forward 2026-03-29 01:00 -> 02:00, fall back 2026-10-25 02:00 -> 01:00.
		{"london gap", "Europe/London", wall(2026, 3, 29, 1, 30), "2026-03-29T01:30:00Z"},
		{"london repeat", "Europe/London", wall(2026, 10, 25, 1, 30), "2026-10-25T00:30:00Z"},
		// Australia/Sydney (southern hemisphere): spring forward 2026-10-04 02:00 -> 03:00, fall back 2026-04-05 03:00 -> 02:00.
		{"sydney gap", "Australia/Sydney", wall(2026, 10, 4, 2, 30), "2026-10-03T16:30:00Z"},
		{"sydney repeat", "Australia/Sydney", wall(2026, 4, 5, 2, 30), "2026-04-04T15:30:00Z"},
		// Zones without daylight saving.
		{"phoenix", "America/Phoenix", wall(2026, 7, 1, 9, 0), "2026-07-01T16:00:00Z"},
		{"kolkata half hour offset", "Asia/Kolkata", wall(2026, 7, 1, 9, 0), "2026-07-01T03:30:00Z"},
		{"utc", "UTC", wall(2026, 7, 1, 9, 0), "2026-07-01T09:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Instant(tt.wall, mustLoad(t, tt.zone))
			assert.Equal(t, tt.want, FormatInstant(got))
		})
	}
}

func TestWallRoundTrip(t *testing.T) {
	chicago := mustLoad(t, "America/Chicago")
	instant := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	w := Wall(instant, chicago)
	assert.Equal(t, wall(2026, 7, 15, 9, 0), w)
	assert.Equal(t, time.UTC, w.Location())
	assert.True(t, Instant(w, chicago).Equal(instant))
}

func TestParseInstantAndDate(t *testing.T) {
	for _, s := range []string{"2026-10-16T12:30:00Z", "2026-10-16T12:30:00+00:00", "2026-10-16 12:30:00"} {
		got, err := ParseInstant(s)
		require.NoError(t, err, s)
		assert.Equal(t, "2026-10-16T12:30:00Z", FormatInstant(got))
	}
	_, err := ParseInstant("tomorrow")
	assert.Error(t, err)

	d, err := ParseDate("2026-10-16")
	require.NoError(t, err)
	assert.Equal(t, "2026-10-16", d.Format(DateLayout))
	_, err = ParseDate("16/10/2026")
	assert.Error(t, err)
}

func TestDeviceZoneOrder(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	none := func() string { return "" }
	tests := []struct {
		name   string
		env    map[string]string
		system func() string
		want   string
	}{
		{"CRM_TZ wins", map[string]string{"CRM_TZ": "Europe/Paris", "TZ": "Asia/Tokyo"}, func() string { return "America/Denver" }, "Europe/Paris"},
		{"TZ next", map[string]string{"TZ": "Asia/Tokyo"}, func() string { return "America/Denver" }, "Asia/Tokyo"},
		{"TZ with a leading colon", map[string]string{"TZ": ":Asia/Tokyo"}, none, "Asia/Tokyo"},
		{"system name last", nil, func() string { return "America/Denver" }, "America/Denver"},
		{"bad names are skipped", map[string]string{"CRM_TZ": "Nowhere/Land"}, func() string { return "America/Denver" }, "America/Denver"},
		{"nothing known falls back to Local", nil, none, "Local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, deviceFrom(env(tt.env), tt.system).String())
		})
	}
}

func TestWindowsToIANA(t *testing.T) {
	assert.Equal(t, "America/Chicago", WindowsToIANA("Central Standard Time"))
	assert.Equal(t, "", WindowsToIANA("Made Up Time"))
	for win, iana := range windowsZones {
		_, err := time.LoadLocation(iana)
		assert.NoError(t, err, win)
	}
}
