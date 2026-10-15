package when

import (
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// now is Thursday 2026-10-15 07:00 in Chicago (12:00Z, CDT).
var now = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func chicago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return loc
}

func TestParse(t *testing.T) {
	loc := chicago(t)
	tests := []struct {
		in      string
		start   string // RFC 3339 UTC, or a date for all-day
		length  time.Duration
		allDay  bool
		wantErr bool
	}{
		{"2026-10-20 14:00", "2026-10-20T19:00:00Z", 0, false, false},
		{"2026-10-20T14:00:00Z", "2026-10-20T14:00:00Z", 0, false, false},
		{"2026-10-20", "2026-10-20", 0, true, false},
		{"fri 12:30", "2026-10-16T17:30:00Z", 0, false, false},
		{"fri 12:30 1h", "2026-10-16T17:30:00Z", time.Hour, false, false},
		{"lunch fri", "", 0, false, true},
		{"friday at 3pm for 90m", "2026-10-16T20:00:00Z", 90 * time.Minute, false, false},
		{"tomorrow 3:15pm", "2026-10-16T20:15:00Z", 0, false, false},
		{"tomorrow", "2026-10-16", 0, true, false},
		{"today 9:00", "2026-10-15T14:00:00Z", 0, false, false},
		{"thu", "2026-10-15", 0, true, false},
		{"thu 9am", "2026-10-15T14:00:00Z", 0, false, false},
		{"thu 6am", "2026-10-22T11:00:00Z", 0, false, false}, // already past today
		{"next thu", "2026-10-22", 0, true, false},
		{"mon 9:00", "2026-10-19T14:00:00Z", 0, false, false},
		{"3pm", "2026-10-15T20:00:00Z", 0, false, false},
		{"6am", "2026-10-16T11:00:00Z", 0, false, false}, // next 6am
		{"noon", "2026-10-15T17:00:00Z", 0, false, false},
		{"oct 20 10:00", "2026-10-20T15:00:00Z", 0, false, false},
		{"20 oct 10:00", "2026-10-20T15:00:00Z", 0, false, false},
		{"jan 5", "2027-01-05", 0, true, false},
		{"jan 5 3d", "2027-01-05", 72 * time.Hour, true, false},
		// Daylight saving: Chicago falls back 2026-11-01. 09:00 on the 2nd is CST.
		{"2026-11-02 09:00", "2026-11-02T15:00:00Z", 0, false, false},
		{"2026-03-08 02:30", "2026-03-08T08:30:00Z", 0, false, false}, // gap: offset before it
		{"", "", 0, false, true},
		{"1h", "", 0, false, true},
		{"whenever", "", 0, false, true},
		{"fri 25:00", "", 0, false, true},
		{"oct", "", 0, false, true},
		{"fri 1h", "", 0, false, true}, // a length in hours needs a time
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in, now, loc)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.allDay, got.AllDay)
			assert.Equal(t, tt.length, got.Length)
			if tt.allDay {
				assert.Equal(t, tt.start, got.Start.Format(tz.DateLayout))
			} else {
				assert.Equal(t, tt.start, tz.FormatInstant(got.Start))
			}
			if tt.length > 0 {
				assert.Equal(t, tt.length, got.End.Sub(got.Start))
			} else {
				assert.True(t, got.End.IsZero())
			}
		})
	}
}

func TestParseIsDeterministic(t *testing.T) {
	loc := chicago(t)
	a, err := Parse("fri 12:30 1h", now, loc)
	require.NoError(t, err)
	b, err := Parse("fri 12:30 1h", now, loc)
	require.NoError(t, err)
	assert.Equal(t, a, b)
}

func TestParseLength(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"30m", 30 * time.Minute, false},
		{"1h", time.Hour, false},
		{"1h30m", 90 * time.Minute, false},
		{"2d", 48 * time.Hour, false},
		{"45", 45 * time.Minute, false},
		{"", 0, true},
		{"0m", 0, true},
		{"abc", 0, true},
		{"h", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseLength(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	m, err := ParseReminder("1h")
	require.NoError(t, err)
	assert.Equal(t, 60, m)
}
