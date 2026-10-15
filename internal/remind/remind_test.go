package remind

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func chicago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return loc
}

func at(t *testing.T, y int, mo time.Month, d, h, mi int) time.Time {
	t.Helper()
	return tz.Instant(time.Date(y, mo, d, h, mi, 0, 0, time.UTC), chicago(t))
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func add(t *testing.T, d *sql.DB, in model.CreateEventInput) *model.Event {
	t.Helper()
	if in.TZ == "" {
		in.TZ = "America/Chicago"
	}
	e, err := repo.NewEventRepo(d).Create(ctx, in)
	require.NoError(t, err)
	return e
}

func rems(minutes ...int) []model.EventReminder {
	var out []model.EventReminder
	for _, m := range minutes {
		out = append(out, model.EventReminder{MinutesBefore: m, Method: "display"})
	}
	return out
}

func titles(list []Due) []string {
	var out []string
	for _, d := range list {
		out = append(out, d.Title)
	}
	return out
}

func TestDueWindowAndSentMarker(t *testing.T) {
	d := openDB(t)
	loc := chicago(t)
	add(t, d, model.CreateEventInput{Title: "Standup", Start: at(t, 2026, 10, 12, 9, 30), End: at(t, 2026, 10, 12, 10, 0), Reminders: rems(15)})
	add(t, d, model.CreateEventInput{Title: "Lunch", Start: at(t, 2026, 10, 12, 12, 0), End: at(t, 2026, 10, 12, 13, 0), Reminders: rems(60, 10)})
	add(t, d, model.CreateEventInput{Title: "No reminder", Start: at(t, 2026, 10, 12, 9, 20), End: at(t, 2026, 10, 12, 9, 40)})

	tests := []struct {
		name   string
		now    time.Time
		within time.Duration
		want   []string
	}{
		{"before any fire time", at(t, 2026, 10, 12, 8, 0), 0, nil},
		{"one minute before the fire time", at(t, 2026, 10, 12, 9, 14), 0, nil},
		{"at the fire time", at(t, 2026, 10, 12, 9, 15), 0, []string{"Standup"}},
		{"after the fire time, event not started", at(t, 2026, 10, 12, 9, 29), 0, []string{"Standup"}},
		{"event started", at(t, 2026, 10, 12, 9, 30), 0, nil},
		{"within looks ahead", at(t, 2026, 10, 12, 9, 0), 15 * time.Minute, []string{"Standup"}},
		{"both lunch reminders are due", at(t, 2026, 10, 12, 11, 55), 0, []string{"Lunch", "Lunch"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := List(ctx, d, tt.now, tt.within, loc)
			require.NoError(t, err)
			assert.Equal(t, tt.want, titles(got))
		})
	}

	got, err := List(ctx, d, at(t, 2026, 10, 12, 11, 55), 0, loc)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, 60, got[0].MinutesBefore, "the earlier reminder first")
	assert.Equal(t, 10, got[1].MinutesBefore)
	assert.Equal(t, "in 5m", got[0].StartsIn)
	assert.Equal(t, "2026-10-12T17:00:00Z", got[0].Start)
	assert.Equal(t, "2026-10-12T16:00:00Z", got[0].FireAt)
	assert.Equal(t, "Mon 12 Oct 12:00", got[0].LocalStart)

	// Handing them out once.
	require.NoError(t, Mark(ctx, d, got))
	again, err := List(ctx, d, at(t, 2026, 10, 12, 11, 55), 0, loc)
	require.NoError(t, err)
	assert.Empty(t, again)
	require.NoError(t, Mark(ctx, d, got), "marking twice is fine")
}

func TestRepeatingEventsRemindPerInstance(t *testing.T) {
	d := openDB(t)
	loc := chicago(t)
	add(t, d, model.CreateEventInput{Title: "Daily", Start: at(t, 2026, 10, 12, 9, 0), End: at(t, 2026, 10, 12, 9, 15), RRule: "FREQ=DAILY", Reminders: rems(10)})

	mon, err := List(ctx, d, at(t, 2026, 10, 12, 8, 55), 0, loc)
	require.NoError(t, err)
	require.Len(t, mon, 1)
	require.NoError(t, Mark(ctx, d, mon))

	again, _ := List(ctx, d, at(t, 2026, 10, 12, 8, 58), 0, loc)
	assert.Empty(t, again, "Monday's instance is done")
	tue, err := List(ctx, d, at(t, 2026, 10, 13, 8, 55), 0, loc)
	require.NoError(t, err)
	require.Len(t, tue, 1, "Tuesday's instance is a new reminder")
	assert.Equal(t, "2026-10-13T14:00:00Z", tue[0].Start)
}

func TestMovedInstanceRemindsAgainAndCancelledIsSkipped(t *testing.T) {
	d := openDB(t)
	loc := chicago(t)
	er := repo.NewEventRepo(d)
	e := add(t, d, model.CreateEventInput{Title: "Call", Start: at(t, 2026, 10, 12, 10, 0), End: at(t, 2026, 10, 12, 10, 30), Reminders: rems(10)})
	first, _ := List(ctx, d, at(t, 2026, 10, 12, 9, 55), 0, loc)
	require.Len(t, first, 1)
	require.NoError(t, Mark(ctx, d, first))

	st, en := at(t, 2026, 10, 12, 15, 0), at(t, 2026, 10, 12, 15, 30)
	_, err := er.Update(ctx, e.ID, model.UpdateEventInput{Start: &st, End: &en}, model.ScopeAll, "")
	require.NoError(t, err)
	moved, _ := List(ctx, d, at(t, 2026, 10, 12, 14, 55), 0, loc)
	require.Len(t, moved, 1, "a new start is a new reminder")

	cancelled := model.EventCancelled
	_, err = er.Update(ctx, e.ID, model.UpdateEventInput{Status: &cancelled}, model.ScopeAll, "")
	require.NoError(t, err)
	none, _ := List(ctx, d, at(t, 2026, 10, 12, 14, 55), 0, loc)
	assert.Empty(t, none)
}

func TestAllDayEventCountsFromMidnightAndDaylightSaving(t *testing.T) {
	d := openDB(t)
	loc := chicago(t)
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	add(t, d, model.CreateEventInput{Title: "Birthday party", AllDay: true, Start: day, End: day.AddDate(0, 0, 1), Reminders: rems(24 * 60)})
	got, err := List(ctx, d, at(t, 2026, 10, 19, 0, 0), 0, loc)
	require.NoError(t, err)
	require.Len(t, got, 1, "one day before midnight Chicago")
	assert.Equal(t, "2026-10-20T05:00:00Z", got[0].Start, "midnight CDT")
	none, _ := List(ctx, d, at(t, 2026, 10, 18, 23, 59), 0, loc)
	assert.Empty(t, none)

	// 09:00 across the end of daylight saving time: the reminder follows the wall clock.
	add(t, d, model.CreateEventInput{Title: "Monday standup", Start: at(t, 2026, 11, 2, 9, 0), End: at(t, 2026, 11, 2, 9, 15), Reminders: rems(30)})
	dst, err := List(ctx, d, at(t, 2026, 11, 2, 8, 30), 0, loc)
	require.NoError(t, err)
	require.Len(t, dst, 1)
	assert.Equal(t, "2026-11-02T15:00:00Z", dst[0].Start, "09:00 CST")
	assert.Equal(t, "2026-11-02T14:30:00Z", dst[0].FireAt)
}

func TestNoRemindersAtAll(t *testing.T) {
	d := openDB(t)
	got, err := List(ctx, d, time.Now(), 0, time.UTC)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestPeopleAndGuestsAreListed(t *testing.T) {
	d := openDB(t)
	p, err := repo.NewPersonRepo(d).Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	add(t, d, model.CreateEventInput{Title: "Sync", Start: at(t, 2026, 10, 12, 10, 0), End: at(t, 2026, 10, 12, 10, 30), Reminders: []model.EventReminder{{MinutesBefore: 10, Method: "email"}},
		Attendees: []model.EventAttendee{{PersonID: p.ID}}, Guests: []model.EventGuest{{Email: "x@y.com"}}, Location: ptr("Room 2")})
	got, err := List(ctx, d, at(t, 2026, 10, 12, 9, 55), 0, chicago(t))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "email", got[0].Method)
	assert.Equal(t, []string{"x@y.com"}, got[0].Guests)
	assert.Equal(t, "Room 2", got[0].Location)
	assert.Len(t, got[0].People, 1)
}

func ptr(s string) *string { return &s }

func TestHumanize(t *testing.T) {
	tests := map[time.Duration]string{
		10 * time.Second: "now", 90 * time.Second: "in 1m", 10 * time.Minute: "in 10m", time.Hour: "in 1h",
		2*time.Hour + 5*time.Minute: "in 2h 5m", 72 * time.Hour: "in 3d",
	}
	for d, want := range tests {
		assert.Equal(t, want, Humanize(d), d.String())
	}
}
