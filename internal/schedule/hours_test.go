package schedule

import (
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

func work(zone string) *model.WorkingHours {
	h := &model.WorkingHours{Name: "work", TZ: zone, Weekly: map[string][]model.TimeRange{}}
	for _, d := range model.WeekdayKeys[:5] {
		h.Weekly[d] = []model.TimeRange{{Start: "09:00", End: "17:00"}}
	}
	return h
}

func texts(spans []Span) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = tz.FormatInstant(s.Start) + "/" + tz.FormatInstant(s.End)
	}
	return out
}

func TestIntervalsWeek(t *testing.T) {
	got, err := Intervals(work("America/Chicago"), utc(2026, 10, 12, 0, 0), utc(2026, 10, 19, 0, 0))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-10-12T14:00:00Z/2026-10-12T22:00:00Z",
		"2026-10-13T14:00:00Z/2026-10-13T22:00:00Z",
		"2026-10-14T14:00:00Z/2026-10-14T22:00:00Z",
		"2026-10-15T14:00:00Z/2026-10-15T22:00:00Z",
		"2026-10-16T14:00:00Z/2026-10-16T22:00:00Z",
	}, texts(got), "Monday to Friday 09:00-17:00 CDT, nothing on the weekend")
}

// Hours are wall clock in the rule's zone, so the instants move when clocks change.
func TestIntervalsAcrossDaylightSaving(t *testing.T) {
	got, err := Intervals(work("America/Chicago"), utc(2026, 10, 30, 0, 0), utc(2026, 11, 4, 0, 0))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-10-30T14:00:00Z/2026-10-30T22:00:00Z", // Fri CDT
		"2026-11-02T15:00:00Z/2026-11-02T23:00:00Z", // Mon CST
		"2026-11-03T15:00:00Z/2026-11-03T23:00:00Z",
	}, texts(got))

	got, err = Intervals(work("America/Chicago"), utc(2026, 3, 5, 0, 0), utc(2026, 3, 11, 0, 0))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-03-05T15:00:00Z/2026-03-05T23:00:00Z", // Thu CST
		"2026-03-06T15:00:00Z/2026-03-06T23:00:00Z",
		"2026-03-09T14:00:00Z/2026-03-09T22:00:00Z", // Mon CDT
		"2026-03-10T14:00:00Z/2026-03-10T22:00:00Z",
	}, texts(got))
}

func TestIntervalsOnTheChangeDayItself(t *testing.T) {
	h := &model.WorkingHours{Name: "w", TZ: "America/Chicago", Weekly: map[string][]model.TimeRange{
		"sun": {{Start: "01:00", End: "04:00"}},
	}}
	// 2026-03-08: 02:00-03:00 does not exist. The 3 hour window is 2 hours of real time.
	got, err := Intervals(h, utc(2026, 3, 8, 0, 0), utc(2026, 3, 9, 0, 0))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 2*time.Hour, got[0].Len())
	// 2026-11-01: 01:00-02:00 happens twice. The window is 4 hours of real time.
	got, err = Intervals(h, utc(2026, 11, 1, 0, 0), utc(2026, 11, 2, 0, 0))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 4*time.Hour, got[0].Len())
}

func TestIntervalsOverrides(t *testing.T) {
	h := work("America/Chicago")
	h.Overrides = map[string][]model.TimeRange{
		"2026-12-24": {{Start: "09:00", End: "12:00"}}, // shorter day
		"2026-12-25": {},                               // day off
		"2026-12-26": {{Start: "10:00", End: "12:00"}}, // a Saturday that is open
	}
	got, err := Intervals(h, utc(2026, 12, 23, 0, 0), utc(2026, 12, 28, 0, 0))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-12-23T15:00:00Z/2026-12-23T23:00:00Z",
		"2026-12-24T15:00:00Z/2026-12-24T18:00:00Z",
		"2026-12-26T16:00:00Z/2026-12-26T18:00:00Z",
	}, texts(got))
}

func TestIntervalsSplitDaysMergeAndClip(t *testing.T) {
	h := &model.WorkingHours{Name: "w", TZ: "UTC", Weekly: map[string][]model.TimeRange{
		"mon": {{Start: "09:00", End: "12:00"}, {Start: "12:00", End: "17:00"}},
		"tue": {{Start: "22:00", End: "24:00"}},
		"wed": {{Start: "00:00", End: "02:00"}},
	}}
	got, err := Intervals(h, utc(2026, 10, 12, 10, 0), utc(2026, 10, 14, 1, 0))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-10-12T10:00:00Z/2026-10-12T17:00:00Z",
		"2026-10-13T22:00:00Z/2026-10-14T01:00:00Z",
	}, texts(got), "touching ranges join, the window clips, and 24:00 meets the next midnight")
}

func TestIntervalsBadInput(t *testing.T) {
	_, err := Intervals(work("Mars/Base"), utc(2026, 1, 1, 0, 0), utc(2026, 1, 2, 0, 0))
	assert.Error(t, err)
	h := &model.WorkingHours{TZ: "UTC", Weekly: map[string][]model.TimeRange{"thu": {{Start: "nine", End: "17:00"}}}}
	_, err = Intervals(h, utc(2026, 1, 1, 0, 0), utc(2026, 1, 3, 0, 0))
	assert.Error(t, err)
	got, err := Intervals(work("UTC"), utc(2026, 1, 5, 0, 0), utc(2026, 1, 5, 0, 0))
	require.NoError(t, err)
	assert.Empty(t, got, "an empty window has no hours")
}

func TestMerge(t *testing.T) {
	in := []Span{{utc(2026, 1, 1, 12, 0), utc(2026, 1, 1, 13, 0)}, {utc(2026, 1, 1, 9, 0), utc(2026, 1, 1, 10, 0)}, {utc(2026, 1, 1, 10, 0), utc(2026, 1, 1, 11, 0)}, {utc(2026, 1, 1, 9, 30), utc(2026, 1, 1, 9, 45)}}
	assert.Equal(t, []string{"2026-01-01T09:00:00Z/2026-01-01T11:00:00Z", "2026-01-01T12:00:00Z/2026-01-01T13:00:00Z"}, texts(Merge(in)))
	assert.Nil(t, Merge(nil))
}
