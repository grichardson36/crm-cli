package tz

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransitions(t *testing.T) {
	chicago := mustLoad(t, "America/Chicago")
	got := Transitions(chicago, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	require.Len(t, got, 2)

	assert.Equal(t, "2026-03-08T08:00:00Z", FormatInstant(got[0].At))
	assert.Equal(t, -6*3600, got[0].From)
	assert.Equal(t, -5*3600, got[0].To)
	assert.Equal(t, "CDT", got[0].Name)
	assert.True(t, got[0].Daylight)
	assert.Equal(t, "2026-03-08T02:00:00Z", FormatInstant(got[0].WallBefore), "the clock reads 02:00 when it springs forward")

	assert.Equal(t, "2026-11-01T07:00:00Z", FormatInstant(got[1].At))
	assert.Equal(t, "CST", got[1].Name)
	assert.False(t, got[1].Daylight)
	assert.Equal(t, "2026-11-01T02:00:00Z", FormatInstant(got[1].WallBefore))

	assert.Empty(t, Transitions(mustLoad(t, "America/Phoenix"), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)))
	assert.Len(t, Transitions(chicago, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)), 8)

	st := StateAt(chicago, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, Initial{Offset: -5 * 3600, Name: "CDT", Daylight: true}, st)
}
