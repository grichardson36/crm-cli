package input

import (
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// now is Thursday 2026-10-15 07:00 in Chicago.
var now = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func chicago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return loc
}

func inst(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return tz.FormatInstant(t)
}

func TestSpan(t *testing.T) {
	loc := chicago(t)
	tests := []struct {
		name       string
		in         SpanText
		start, end string
		allDay     bool
		wantErr    bool
	}{
		{"start only", SpanText{Start: "fri 12:30"}, "2026-10-16T17:30:00Z", "", false, false},
		{"duration", SpanText{Start: "fri 12:30", Duration: "1h30m"}, "2026-10-16T17:30:00Z", "2026-10-16T19:00:00Z", false, false},
		{"length in the start text", SpanText{Start: "fri 12:30 45m"}, "2026-10-16T17:30:00Z", "2026-10-16T18:15:00Z", false, false},
		{"end time on the same day", SpanText{Start: "fri 12:30", End: "14:00"}, "2026-10-16T17:30:00Z", "2026-10-16T19:00:00Z", false, false},
		{"end date and time", SpanText{Start: "fri 22:00", End: "sat 01:00"}, "2026-10-17T03:00:00Z", "2026-10-17T06:00:00Z", false, false},
		{"all day by date", SpanText{Start: "2026-11-26"}, "2026-11-26T00:00:00Z", "", true, false},
		{"all day end is inclusive", SpanText{Start: "2026-11-26", End: "2026-11-28"}, "2026-11-26T00:00:00Z", "2026-11-29T00:00:00Z", true, false},
		{"all day flag drops the time", SpanText{Start: "2026-11-26 15:00", AllDay: true}, "2026-11-26T00:00:00Z", "", true, false},
		{"all day by duration", SpanText{Start: "2026-11-26", Duration: "3d"}, "2026-11-26T00:00:00Z", "2026-11-29T00:00:00Z", true, false},
		{"bad start", SpanText{Start: "later"}, "", "", false, true},
		{"bad end", SpanText{Start: "fri 12:30", End: "soonish"}, "", "", false, true},
		{"bad duration", SpanText{Start: "fri 12:30", Duration: "xyz"}, "", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, allDay, err := Span(tt.in, loc, now)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, model.ErrValidation)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.start, inst(start))
			assert.Equal(t, tt.end, inst(end))
			assert.Equal(t, tt.allDay, allDay)
		})
	}
}

func TestBoundAndRule(t *testing.T) {
	loc := chicago(t)
	tests := []struct {
		name    string
		text    RepeatText
		base    string
		allDay  bool
		want    *string
		wantErr bool
	}{
		{"natural", RepeatText{Repeat: "every fri", Changed: true}, "", false, sp("FREQ=WEEKLY;BYDAY=FR"), false},
		{"count", RepeatText{Repeat: "daily", Changed: true, Count: 5}, "", false, sp("FREQ=DAILY;COUNT=5"), false},
		{"until becomes the end of that local day in UTC", RepeatText{Repeat: "daily", Changed: true, Until: "2026-12-31"}, "", false, sp("FREQ=DAILY;UNTIL=20270101T055959Z"), false},
		{"until for all-day stays a date", RepeatText{Repeat: "daily", Changed: true, Until: "2026-12-31"}, "", true, sp("FREQ=DAILY;UNTIL=20261231"), false},
		{"until across the daylight saving change", RepeatText{Repeat: "daily", Changed: true, Until: "2026-03-08"}, "", false, sp("FREQ=DAILY;UNTIL=20260309T045959Z"), false},
		{"bound replaces an existing count", RepeatText{Until: "2026-12-31"}, "FREQ=DAILY;COUNT=3", false, sp("FREQ=DAILY;UNTIL=20270101T055959Z"), false},
		{"bound replaces an existing until", RepeatText{Count: 4}, "FREQ=DAILY;UNTIL=20261231", false, sp("FREQ=DAILY;COUNT=4"), false},
		{"nothing changes", RepeatText{}, "FREQ=DAILY", false, nil, false},
		{"clear", RepeatText{Repeat: "", Changed: true}, "FREQ=DAILY", false, sp(""), false},
		{"until and count", RepeatText{Repeat: "daily", Changed: true, Until: "2026-12-31", Count: 2}, "", false, nil, true},
		{"bound without a rule", RepeatText{Until: "2026-12-31"}, "", false, nil, true},
		{"clear with a bound", RepeatText{Repeat: "", Changed: true, Count: 2}, "FREQ=DAILY", false, nil, true},
		{"bad natural", RepeatText{Repeat: "now and then", Changed: true}, "", false, nil, true},
		{"negative count", RepeatText{Repeat: "daily", Changed: true, Count: -2}, "", false, nil, true},
		{"bad until", RepeatText{Repeat: "daily", Changed: true, Until: "someday"}, "", false, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Rule(tt.text, tt.base, tt.allDay, loc, now)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, model.ErrValidation)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func sp(s string) *string { return &s }

func TestReminders(t *testing.T) {
	got, err := Reminders([]string{"15m", "1h:email", "1d", "30"})
	require.NoError(t, err)
	assert.Equal(t, []model.EventReminder{
		{MinutesBefore: 15, Method: "display"}, {MinutesBefore: 60, Method: "email"},
		{MinutesBefore: 1440, Method: "display"}, {MinutesBefore: 30, Method: "display"},
	}, got)
	_, err = Reminders([]string{"soon"})
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestWindow(t *testing.T) {
	loc := chicago(t)
	tests := []struct {
		name     string
		from, to string
		days     int
		start    string
		end      string
		wantErr  bool
	}{
		{"default is today for a week", "", "", 7, "2026-10-15T05:00:00Z", "2026-10-22T05:00:00Z", false},
		{"from a date", "2026-10-20", "", 3, "2026-10-20T05:00:00Z", "2026-10-23T05:00:00Z", false},
		{"to a date", "2026-10-20", "2026-10-22", 7, "2026-10-20T05:00:00Z", "2026-10-22T05:00:00Z", false},
		{"across the fall back day a day is 25 hours", "2026-10-31", "", 2, "2026-10-31T05:00:00Z", "2026-11-02T06:00:00Z", false},
		{"relative phrase", "tomorrow", "", 1, "2026-10-16T05:00:00Z", "2026-10-17T05:00:00Z", false},
		{"to before from", "2026-10-20", "2026-10-19", 7, "", "", true},
		{"no days", "", "", 0, "", "", true},
		{"bad from", "someday", "", 7, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, to, err := Window(tt.from, tt.to, tt.days, loc, now)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.start, inst(f))
			assert.Equal(t, tt.end, inst(to))
		})
	}
}

func TestAnchor(t *testing.T) {
	timed := &model.Event{TZ: sp("America/Chicago")}
	allDay := &model.Event{AllDay: true}
	got, err := Anchor(timed, "2026-10-26T14:15:00Z", now)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-26T14:15:00Z", got)
	got, err = Anchor(timed, "2026-10-26 09:15", now)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-26T14:15:00Z", got, "a phrase is read in the event's zone")
	_, err = Anchor(timed, "2026-10-26", now)
	assert.ErrorIs(t, err, model.ErrValidation)
	got, err = Anchor(allDay, "2026-10-26", now)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-26", got)
	got, err = Anchor(timed, "", now)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestEdit(t *testing.T) {
	loc := chicago(t)
	timed := &model.Event{TZ: sp("America/Chicago"), StartUTC: sp("2026-10-20T19:00:00Z"), EndUTC: sp("2026-10-20T19:45:00Z")}
	allDay := &model.Event{AllDay: true, StartDate: sp("2026-11-26"), EndDate: sp("2026-11-27")}

	t.Run("a date keeps the time of day", func(t *testing.T) {
		var in model.UpdateEventInput
		require.NoError(t, Edit(EditText{Start: "2026-10-22", HasStart: true}, timed, loc, now, &in))
		assert.Equal(t, "2026-10-22T19:00:00Z", inst(*in.Start))
		assert.Nil(t, in.End, "the repository keeps the length")
	})
	t.Run("a full start with a length", func(t *testing.T) {
		var in model.UpdateEventInput
		require.NoError(t, Edit(EditText{Start: "2026-10-22 10:00 2h", HasStart: true}, timed, loc, now, &in))
		assert.Equal(t, "2026-10-22T15:00:00Z", inst(*in.Start))
		assert.Equal(t, "2026-10-22T17:00:00Z", inst(*in.End))
	})
	t.Run("end alone", func(t *testing.T) {
		var in model.UpdateEventInput
		require.NoError(t, Edit(EditText{End: "15:30", HasEnd: true}, timed, loc, now, &in))
		assert.Nil(t, in.Start)
		assert.Equal(t, "2026-10-20T20:30:00Z", inst(*in.End))
	})
	t.Run("duration alone", func(t *testing.T) {
		var in model.UpdateEventInput
		require.NoError(t, Edit(EditText{Duration: "2h", HasDuration: true}, timed, loc, now, &in))
		assert.Equal(t, "2026-10-20T21:00:00Z", inst(*in.End))
	})
	t.Run("all-day end is inclusive", func(t *testing.T) {
		var in model.UpdateEventInput
		require.NoError(t, Edit(EditText{End: "2026-11-28", HasEnd: true, AllDay: true}, allDay, time.UTC, now, &in))
		assert.Equal(t, "2026-11-29T00:00:00Z", inst(*in.End))
	})
	t.Run("a timed edit of an all-day event needs a time", func(t *testing.T) {
		var in model.UpdateEventInput
		err := Edit(EditText{Start: "2026-11-26", HasStart: true}, allDay, time.UTC, now, &in)
		assert.ErrorIs(t, err, model.ErrValidation)
	})
	t.Run("bad text", func(t *testing.T) {
		var in model.UpdateEventInput
		assert.Error(t, Edit(EditText{Start: "never", HasStart: true}, timed, loc, now, &in))
		assert.Error(t, Edit(EditText{End: "never", HasEnd: true}, timed, loc, now, &in))
		assert.Error(t, Edit(EditText{Duration: "never", HasDuration: true}, timed, loc, now, &in))
	})
}
