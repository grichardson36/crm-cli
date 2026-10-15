package repo_test

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

type eventFx struct {
	d   *sql.DB
	er  *repo.EventRepo
	pr  *repo.PersonRepo
	ctx context.Context
}

func newEventFx(t *testing.T) *eventFx {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return &eventFx{d: d, er: repo.NewEventRepo(d), pr: repo.NewPersonRepo(d), ctx: context.Background()}
}

func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

func chicago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return loc
}

func (f *eventFx) person(t *testing.T, first string) int64 {
	t.Helper()
	p, err := f.pr.Create(f.ctx, model.CreatePersonInput{FirstName: first})
	require.NoError(t, err)
	return p.ID
}

// weekly creates "Standup": Mondays 09:00 America/Chicago, anchored 2026-03-02 (CST).
func (f *eventFx) weekly(t *testing.T, rule string) *model.Event {
	t.Helper()
	e, err := f.er.Create(f.ctx, model.CreateEventInput{
		Title: "Standup", Start: utc(2026, 3, 2, 15, 0), End: utc(2026, 3, 2, 15, 30), TZ: "America/Chicago", RRule: rule,
	})
	require.NoError(t, err)
	return e
}

func (f *eventFx) list(t *testing.T, from, to time.Time) []model.EventInstance {
	t.Helper()
	got, _, err := f.er.List(f.ctx, model.EventFilters{From: from, To: to, Loc: chicago(t)})
	require.NoError(t, err)
	return got
}

func instStarts(list []model.EventInstance) []string {
	out := make([]string, len(list))
	for i, x := range list {
		if x.AllDay {
			out[i] = x.StartDate
		} else {
			out[i] = x.Start
		}
	}
	return out
}

func TestEventCreate(t *testing.T) {
	f := newEventFx(t)
	jane := f.person(t, "Jane")

	e, err := f.er.Create(f.ctx, model.CreateEventInput{
		Title: "  Lunch with Jane ", Start: utc(2026, 10, 16, 17, 30), End: utc(2026, 10, 16, 18, 30),
		TZ: "America/Chicago", Location: ptr("Cafe Rio"), Notes: ptr("bring the deck"),
		Attendees: []model.EventAttendee{{PersonID: jane, Role: "organizer"}},
		Reminders: []model.EventReminder{{MinutesBefore: 15}, {MinutesBefore: 15}, {MinutesBefore: 60, Method: "email"}},
		Tags:      []string{"#lunch"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Lunch with Jane", e.Title)
	assert.NotEmpty(t, e.UUID)
	assert.Equal(t, "2026-10-16T17:30:00Z", *e.StartUTC)
	assert.Equal(t, "2026-10-16T18:30:00Z", *e.EndUTC)
	assert.Equal(t, "America/Chicago", *e.TZ)
	assert.Equal(t, "confirmed", e.Status)
	assert.True(t, e.Busy)
	assert.Equal(t, "local", e.Source)
	assert.Equal(t, []string{"lunch"}, e.Tags)
	require.Len(t, e.Attendees, 1)
	assert.Equal(t, "Jane", e.Attendees[0].Name)
	assert.Equal(t, "organizer", e.Attendees[0].Role)
	assert.Equal(t, "needs-action", e.Attendees[0].RSVP)
	assert.Equal(t, []model.EventReminder{{MinutesBefore: 60, Method: "email"}, {MinutesBefore: 15, Method: "display"}}, e.Reminders)
}

func TestEventCreateDefaultsAndValidation(t *testing.T) {
	f := newEventFx(t)
	e, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "Quick", Start: utc(2026, 10, 16, 17, 30), TZ: "UTC"})
	require.NoError(t, err)
	assert.Equal(t, "2026-10-16T18:30:00Z", *e.EndUTC, "default length is one hour")

	day, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "Holiday", AllDay: true, Start: utc(2026, 11, 26, 13, 0)})
	require.NoError(t, err)
	assert.True(t, day.AllDay)
	assert.Equal(t, "2026-11-26", *day.StartDate)
	assert.Equal(t, "2026-11-27", *day.EndDate, "default length is one day, end exclusive")
	assert.Nil(t, day.StartUTC)
	assert.Nil(t, day.TZ, "all-day events carry no zone")

	tests := []struct {
		name string
		in   model.CreateEventInput
	}{
		{"no title", model.CreateEventInput{Title: " ", Start: utc(2026, 1, 1, 9, 0)}},
		{"no start", model.CreateEventInput{Title: "x"}},
		{"ends before start", model.CreateEventInput{Title: "x", Start: utc(2026, 1, 1, 9, 0), End: utc(2026, 1, 1, 8, 0)}},
		{"bad status", model.CreateEventInput{Title: "x", Start: utc(2026, 1, 1, 9, 0), Status: "maybe"}},
		{"bad zone", model.CreateEventInput{Title: "x", Start: utc(2026, 1, 1, 9, 0), TZ: "Mars/Base"}},
		{"bad rule", model.CreateEventInput{Title: "x", Start: utc(2026, 1, 1, 9, 0), RRule: "FREQ=DAILY;COUNT=2;UNTIL=20260301"}},
		{"unknown person", model.CreateEventInput{Title: "x", Start: utc(2026, 1, 1, 9, 0), Attendees: []model.EventAttendee{{PersonID: 99}}}},
		{"bad role", model.CreateEventInput{Title: "x", Start: utc(2026, 1, 1, 9, 0), Attendees: []model.EventAttendee{{PersonID: 1, Role: "boss"}}}},
		{"negative reminder", model.CreateEventInput{Title: "x", Start: utc(2026, 1, 1, 9, 0), Reminders: []model.EventReminder{{MinutesBefore: -5}}}},
		{"bad reminder method", model.CreateEventInput{Title: "x", Start: utc(2026, 1, 1, 9, 0), Reminders: []model.EventReminder{{MinutesBefore: 5, Method: "sms"}}}},
		{"all-day ends before it starts", model.CreateEventInput{Title: "x", AllDay: true, Start: utc(2026, 1, 5, 0, 0), End: utc(2026, 1, 4, 0, 0)}},
	}
	f.person(t, "Pat")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.er.Create(f.ctx, tt.in)
			require.Error(t, err)
		})
	}
	var n int
	require.NoError(t, f.d.QueryRow(`SELECT COUNT(*) FROM events WHERE title = 'x'`).Scan(&n))
	assert.Equal(t, 0, n, "failed creates leave nothing behind")
}

func TestEventNaturalRecurrenceIsCompiled(t *testing.T) {
	f := newEventFx(t)
	e := f.weekly(t, "every mon")
	require.NotNil(t, e.RRule)
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO", *e.RRule)
}

func TestEventListWindow(t *testing.T) {
	f := newEventFx(t)
	jane := f.person(t, "Jane")
	mk := func(title string, in model.CreateEventInput) {
		in.Title = title
		_, err := f.er.Create(f.ctx, in)
		require.NoError(t, err)
	}
	mk("early", model.CreateEventInput{Start: utc(2026, 10, 16, 14, 0), End: utc(2026, 10, 16, 15, 0), TZ: "America/Chicago"})
	mk("late", model.CreateEventInput{Start: utc(2026, 10, 16, 22, 0), End: utc(2026, 10, 16, 23, 0), TZ: "America/Chicago",
		Attendees: []model.EventAttendee{{PersonID: jane}}, Tags: []string{"work"}, Notes: ptr("budget review")})
	mk("other day", model.CreateEventInput{Start: utc(2026, 10, 18, 14, 0), End: utc(2026, 10, 18, 15, 0), TZ: "America/Chicago"})
	mk("holiday", model.CreateEventInput{AllDay: true, Start: utc(2026, 10, 16, 0, 0)})
	mk("trip", model.CreateEventInput{AllDay: true, Start: utc(2026, 10, 15, 0, 0), End: utc(2026, 10, 18, 0, 0)})
	mk("cancelled", model.CreateEventInput{Start: utc(2026, 10, 16, 16, 0), TZ: "UTC", Status: "cancelled"})
	mk("ends as the window starts", model.CreateEventInput{Start: utc(2026, 10, 16, 4, 0), End: utc(2026, 10, 16, 5, 0), TZ: "UTC"})

	loc := chicago(t)
	// Friday 2026-10-16 in Chicago is 05:00Z to 05:00Z the next day.
	from, to := time.Date(2026, 10, 16, 0, 0, 0, 0, loc), time.Date(2026, 10, 17, 0, 0, 0, 0, loc)
	titles := func(filters model.EventFilters) []string {
		filters.From, filters.To, filters.Loc = from, to, loc
		got, _, err := f.er.List(f.ctx, filters)
		require.NoError(t, err)
		out := make([]string, len(got))
		for i, g := range got {
			out[i] = g.Title
		}
		return out
	}

	assert.Equal(t, []string{"trip", "holiday", "early", "late"}, titles(model.EventFilters{}), "all-day first by date, then timed by start; cancelled hidden")
	assert.Equal(t, []string{"trip", "holiday", "early", "cancelled", "late"}, titles(model.EventFilters{IncludeCancelled: true}))
	assert.Equal(t, []string{"late"}, titles(model.EventFilters{PersonID: &jane}))
	assert.Equal(t, []string{"late"}, titles(model.EventFilters{Tag: "#work"}))
	assert.Equal(t, []string{"late"}, titles(model.EventFilters{Query: "budget"}))
	assert.Empty(t, titles(model.EventFilters{Tag: "none"}))
	assert.Equal(t, 0, len(instStarts(nil)))

	_, _, err := f.er.List(f.ctx, model.EventFilters{From: to, To: from})
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestEventListAttendeeCount(t *testing.T) {
	f := newEventFx(t)
	a, b := f.person(t, "A"), f.person(t, "B")
	_, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "x", Start: utc(2026, 10, 16, 14, 0), TZ: "UTC",
		Attendees: []model.EventAttendee{{PersonID: a}, {PersonID: b}}})
	require.NoError(t, err)
	got := f.list(t, utc(2026, 10, 16, 0, 0), utc(2026, 10, 17, 0, 0))
	require.Len(t, got, 1)
	assert.Equal(t, 2, got[0].Attendees)
	assert.False(t, got[0].Recurring)
}

func TestEventRecurringList(t *testing.T) {
	f := newEventFx(t)
	f.weekly(t, "FREQ=WEEKLY;BYDAY=MO")
	got := f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 3, 24, 0, 0))
	assert.Equal(t, []string{
		"2026-03-02T15:00:00Z", "2026-03-09T14:00:00Z", "2026-03-16T14:00:00Z", "2026-03-23T14:00:00Z",
	}, instStarts(got), "09:00 local across the 2026-03-08 change")
	for _, g := range got {
		assert.True(t, g.Recurring)
		assert.NotEmpty(t, g.Occurrence)
		assert.Equal(t, "America/Chicago", g.TZ)
	}
}

func TestEventAllDayRecurringBirthday(t *testing.T) {
	f := newEventFx(t)
	_, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "Anniversary", AllDay: true, Start: utc(2026, 3, 8, 0, 0), RRule: "FREQ=YEARLY"})
	require.NoError(t, err)
	got, _, err := f.er.List(f.ctx, model.EventFilters{From: utc(2027, 1, 1, 0, 0), To: utc(2029, 1, 1, 0, 0), Loc: chicago(t)})
	require.NoError(t, err)
	assert.Equal(t, []string{"2027-03-08", "2028-03-08"}, instStarts(got))
}

func TestEventUpdateAll(t *testing.T) {
	f := newEventFx(t)
	e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO")
	updated, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("Daily sync"), Location: ptr("Room 4")}, "", "")
	require.NoError(t, err)
	assert.Equal(t, "Daily sync", updated.Title)
	assert.Equal(t, "Room 4", *updated.Location)
	assert.Equal(t, "2026-03-02T15:00:00Z", *updated.StartUTC, "schedule untouched")

	// Moving the start keeps the length.
	moved := utc(2026, 3, 3, 16, 0)
	updated, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Start: &moved}, model.ScopeAll, "")
	require.NoError(t, err)
	assert.Equal(t, "2026-03-03T16:00:00Z", *updated.StartUTC)
	assert.Equal(t, "2026-03-03T16:30:00Z", *updated.EndUTC)

	// Clear the rule.
	updated, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{RRule: ptr("")}, "", "")
	require.NoError(t, err)
	assert.Nil(t, updated.RRule)

	_, err = f.er.Update(f.ctx, 999, model.UpdateEventInput{Title: ptr("x")}, "", "")
	assert.ErrorIs(t, err, model.ErrNotFound)
	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr(" ")}, "", "")
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{}, "sometimes", "")
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestEventUpdateAllDayToggle(t *testing.T) {
	f := newEventFx(t)
	e, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "x", Start: utc(2026, 5, 1, 14, 0), TZ: "UTC"})
	require.NoError(t, err)
	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{AllDay: boolp(true)}, "", "")
	assert.ErrorIs(t, err, model.ErrValidation, "needs a start")
	start := utc(2026, 5, 2, 0, 0)
	got, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{AllDay: boolp(true), Start: &start}, "", "")
	require.NoError(t, err)
	assert.True(t, got.AllDay)
	assert.Equal(t, "2026-05-02", *got.StartDate)
	assert.Equal(t, "2026-05-03", *got.EndDate)
	assert.Nil(t, got.StartUTC)
}

func boolp(b bool) *bool { return &b }

func TestEventUpdateThisInstance(t *testing.T) {
	f := newEventFx(t)
	e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=4")
	at := "2026-03-09T14:00:00Z" // second Monday, already CDT
	moved := utc(2026, 3, 10, 18, 0)
	ov, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("Standup (moved)"), Start: &moved}, model.ScopeThis, at)
	require.NoError(t, err)
	require.NotNil(t, ov.RecurrenceParentID)
	assert.Equal(t, e.ID, *ov.RecurrenceParentID)
	assert.Equal(t, at, *ov.RecurrenceID)
	assert.Nil(t, ov.RRule)

	got := f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 4, 1, 0, 0))
	assert.Equal(t, []string{"2026-03-02T15:00:00Z", "2026-03-10T18:00:00Z", "2026-03-16T14:00:00Z", "2026-03-23T14:00:00Z"}, instStarts(got))
	assert.Equal(t, "Standup (moved)", got[1].Title)
	assert.Equal(t, "Standup", got[0].Title)
	assert.Equal(t, at, got[1].Occurrence, "keeps its place in the series")
	assert.Equal(t, e.ID, got[1].SeriesID)
	assert.Equal(t, ov.ID, got[1].EventID)

	// A second edit of the same instance edits the same override row.
	again, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Location: ptr("Room 9")}, model.ScopeThis, at)
	require.NoError(t, err)
	assert.Equal(t, ov.ID, again.ID)
	assert.Equal(t, "Room 9", *again.Location)
	var n int
	require.NoError(t, f.d.QueryRow(`SELECT COUNT(*) FROM events WHERE recurrence_parent_id = ?`, e.ID).Scan(&n))
	assert.Equal(t, 1, n)

	// Errors.
	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("x")}, model.ScopeThis, "")
	assert.ErrorIs(t, err, model.ErrValidation, "needs the instance")
	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("x")}, model.ScopeThis, "2026-03-11T15:00:00Z")
	assert.ErrorIs(t, err, model.ErrNotFound, "not an instance")
	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{RRule: ptr("daily")}, model.ScopeThis, at)
	assert.ErrorIs(t, err, model.ErrValidation, "a rule belongs to a series")
}

func TestEventUpdateFollowingSplitsTheSeries(t *testing.T) {
	f := newEventFx(t)
	jane := f.person(t, "Jane")
	e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=6")
	require.NoError(t, f.er.SetAttendee(f.ctx, e.ID, model.EventAttendee{PersonID: jane}))
	require.NoError(t, f.er.SetReminders(f.ctx, e.ID, []model.EventReminder{{MinutesBefore: 10}}))
	tagged, err := f.er.FindByID(f.ctx, e.ID)
	require.NoError(t, err)
	_ = tagged

	at := "2026-03-16T14:00:00Z" // third Monday
	moved := utc(2026, 3, 16, 16, 0)
	nw, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("Standup v2"), Start: &moved}, model.ScopeFollowing, at)
	require.NoError(t, err)
	assert.NotEqual(t, e.ID, nw.ID)
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=4", *nw.RRule, "the remaining count moves to the new series")
	assert.Equal(t, "2026-03-16T16:00:00Z", *nw.StartUTC)
	require.Len(t, nw.Attendees, 1)
	assert.Equal(t, jane, nw.Attendees[0].PersonID)
	assert.Equal(t, []model.EventReminder{{MinutesBefore: 10, Method: "display"}}, nw.Reminders)

	old, err := f.er.FindByID(f.ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=2", *old.RRule)
	assert.Equal(t, "Standup", old.Title)

	got := f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 5, 1, 0, 0))
	assert.Equal(t, []string{
		"2026-03-02T15:00:00Z", "2026-03-09T14:00:00Z", // old series
		"2026-03-16T16:00:00Z", "2026-03-23T16:00:00Z", "2026-03-30T16:00:00Z", "2026-04-06T16:00:00Z",
	}, instStarts(got))
	assert.Equal(t, "Standup v2", got[2].Title)
}

func TestEventUpdateFollowingMovesExceptionsAndOverrides(t *testing.T) {
	f := newEventFx(t)
	e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO")
	require.NoError(t, f.er.Archive(f.ctx, e.ID, model.ScopeThis, "2026-03-23T14:00:00Z"))                                     // skip Mar 23
	_, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("special")}, model.ScopeThis, "2026-03-30T14:00:00Z") // override Mar 30
	require.NoError(t, err)

	nw, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Location: ptr("Annex")}, model.ScopeFollowing, "2026-03-16T14:00:00Z")
	require.NoError(t, err)
	assert.Contains(t, *nw.RRule, "BYDAY=MO")
	assert.Equal(t, []string{"2026-03-23T14:00:00Z"}, nw.Exdates)

	got := f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 4, 7, 0, 0))
	assert.Equal(t, []string{"2026-03-02T15:00:00Z", "2026-03-09T14:00:00Z", "2026-03-16T14:00:00Z", "2026-03-30T14:00:00Z", "2026-04-06T14:00:00Z"}, instStarts(got))
	assert.Equal(t, "special", got[3].Title)
	assert.Equal(t, "Annex", got[2].Location)
}

func TestEventUpdateFollowingAtTheFirstInstanceEditsTheSeries(t *testing.T) {
	f := newEventFx(t)
	e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO")
	got, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("renamed")}, model.ScopeFollowing, "2026-03-02T15:00:00Z")
	require.NoError(t, err)
	assert.Equal(t, e.ID, got.ID)
	assert.Equal(t, "renamed", got.Title)
}

func TestEventUpdateStartResetsExceptions(t *testing.T) {
	f := newEventFx(t)
	e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO")
	require.NoError(t, f.er.Archive(f.ctx, e.ID, model.ScopeThis, "2026-03-09T14:00:00Z"))
	start := utc(2026, 3, 3, 15, 0)
	got, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Start: &start, RRule: ptr("every tue")}, model.ScopeAll, "")
	require.NoError(t, err)
	assert.Empty(t, got.Exdates)
}

func TestEventDeleteScopes(t *testing.T) {
	t.Run("this instance is skipped", func(t *testing.T) {
		f := newEventFx(t)
		e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=4")
		require.NoError(t, f.er.Archive(f.ctx, e.ID, model.ScopeThis, "2026-03-09T14:00:00Z"))
		got := f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 5, 1, 0, 0))
		assert.Equal(t, []string{"2026-03-02T15:00:00Z", "2026-03-16T14:00:00Z", "2026-03-23T14:00:00Z"}, instStarts(got))
		_, err := f.er.FindByID(f.ctx, e.ID)
		assert.NoError(t, err, "the series is still there")
		err = f.er.Archive(f.ctx, e.ID, model.ScopeThis, "2026-03-10T14:00:00Z")
		assert.ErrorIs(t, err, model.ErrNotFound)
	})
	t.Run("deleting an overridden instance removes the override too", func(t *testing.T) {
		f := newEventFx(t)
		e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=3")
		_, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("special")}, model.ScopeThis, "2026-03-09T14:00:00Z")
		require.NoError(t, err)
		require.NoError(t, f.er.Archive(f.ctx, e.ID, model.ScopeThis, "2026-03-09T14:00:00Z"))
		got := f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 5, 1, 0, 0))
		assert.Equal(t, []string{"2026-03-02T15:00:00Z", "2026-03-16T14:00:00Z"}, instStarts(got))
		var tomb int
		require.NoError(t, f.d.QueryRow(`SELECT COUNT(*) FROM tombstones WHERE table_name = 'events'`).Scan(&tomb))
		assert.Equal(t, 1, tomb, "the removed override leaves a tombstone")
	})
	t.Run("following ends the series before the instance", func(t *testing.T) {
		f := newEventFx(t)
		e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO")
		require.NoError(t, f.er.Archive(f.ctx, e.ID, model.ScopeFollowing, "2026-03-16T14:00:00Z"))
		got := f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 8, 1, 0, 0))
		assert.Equal(t, []string{"2026-03-02T15:00:00Z", "2026-03-09T14:00:00Z"}, instStarts(got))
	})
	t.Run("following the first instance deletes everything", func(t *testing.T) {
		f := newEventFx(t)
		e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO")
		require.NoError(t, f.er.Archive(f.ctx, e.ID, model.ScopeFollowing, "2026-03-02T15:00:00Z"))
		assert.Empty(t, f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 8, 1, 0, 0)))
	})
	t.Run("all archives the series and its overrides, restore brings them back", func(t *testing.T) {
		f := newEventFx(t)
		e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=3")
		_, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("special")}, model.ScopeThis, "2026-03-09T14:00:00Z")
		require.NoError(t, err)
		require.NoError(t, f.er.Archive(f.ctx, e.ID, "", ""))
		assert.Empty(t, f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 8, 1, 0, 0)))
		_, err = f.er.FindByID(f.ctx, e.ID)
		assert.ErrorIs(t, err, model.ErrNotFound)

		require.NoError(t, f.er.Restore(f.ctx, e.ID))
		got := f.list(t, utc(2026, 3, 1, 0, 0), utc(2026, 8, 1, 0, 0))
		assert.Len(t, got, 3)
		assert.Equal(t, "special", got[1].Title)
		assert.ErrorIs(t, f.er.Restore(f.ctx, e.ID), model.ErrNotFound, "not archived any more")
	})
}

func TestEventPurgeWritesTombstones(t *testing.T) {
	f := newEventFx(t)
	e := f.weekly(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=3")
	_, err := f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("special")}, model.ScopeThis, "2026-03-09T14:00:00Z")
	require.NoError(t, err)
	require.NoError(t, repo.NewTagRepo(f.d).Apply(f.ctx, "event", e.ID, "work"))
	ext := repo.NewExternalIDRepo(f.d)
	_, err = ext.Upsert(f.ctx, repo.ExternalIDInput{Entity: "event", LocalID: e.ID, Source: "ics", ExternalID: "abc@example.com"})
	require.NoError(t, err)

	assert.ErrorIs(t, f.er.Purge(f.ctx, e.ID), model.ErrConflict, "archive first")
	require.NoError(t, f.er.Archive(f.ctx, e.ID, "", ""))
	require.NoError(t, f.er.Purge(f.ctx, e.ID))

	count := func(q string) int {
		var n int
		require.NoError(t, f.d.QueryRow(q).Scan(&n))
		return n
	}
	assert.Equal(t, 0, count(`SELECT COUNT(*) FROM events`))
	assert.Equal(t, 2, count(`SELECT COUNT(*) FROM tombstones WHERE table_name = 'events'`), "series and override")
	assert.Equal(t, 0, count(`SELECT COUNT(*) FROM external_ids WHERE entity = 'event'`))
	assert.Equal(t, 0, count(`SELECT COUNT(*) FROM taggings WHERE entity_type = 'event'`))
	assert.Equal(t, 0, count(`SELECT COUNT(*) FROM event_exdates`))
	assert.ErrorIs(t, f.er.Purge(f.ctx, e.ID), model.ErrNotFound)
}

func TestEventAttendeesAndReminders(t *testing.T) {
	f := newEventFx(t)
	jane, bob := f.person(t, "Jane"), f.person(t, "Bob")
	e, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "x", Start: utc(2026, 10, 16, 14, 0), TZ: "UTC"})
	require.NoError(t, err)

	require.NoError(t, f.er.SetAttendee(f.ctx, e.ID, model.EventAttendee{PersonID: jane}))
	require.NoError(t, f.er.SetAttendee(f.ctx, e.ID, model.EventAttendee{PersonID: bob, Role: "organizer", RSVP: "accepted"}))
	require.NoError(t, f.er.SetAttendee(f.ctx, e.ID, model.EventAttendee{PersonID: jane, RSVP: "declined"}), "updates the existing link")
	got, err := f.er.FindByID(f.ctx, e.ID)
	require.NoError(t, err)
	require.Len(t, got.Attendees, 2)
	assert.Equal(t, "Bob", got.Attendees[0].Name, "organizer first")
	assert.Equal(t, "declined", got.Attendees[1].RSVP)

	assert.ErrorIs(t, f.er.SetAttendee(f.ctx, e.ID, model.EventAttendee{PersonID: 999}), model.ErrNotFound)
	assert.ErrorIs(t, f.er.SetAttendee(f.ctx, e.ID, model.EventAttendee{PersonID: jane, RSVP: "dunno"}), model.ErrValidation)
	assert.ErrorIs(t, f.er.SetAttendee(f.ctx, 999, model.EventAttendee{PersonID: jane}), model.ErrNotFound)
	require.NoError(t, f.er.RemoveAttendee(f.ctx, e.ID, jane))
	assert.ErrorIs(t, f.er.RemoveAttendee(f.ctx, e.ID, jane), model.ErrNotFound)

	require.NoError(t, f.er.SetReminders(f.ctx, e.ID, []model.EventReminder{{MinutesBefore: 5}, {MinutesBefore: 30, Method: "email"}}))
	require.NoError(t, f.er.SetReminders(f.ctx, e.ID, []model.EventReminder{{MinutesBefore: 10}}))
	got, err = f.er.FindByID(f.ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, []model.EventReminder{{MinutesBefore: 10, Method: "display"}}, got.Reminders, "replaced wholesale")
	assert.ErrorIs(t, f.er.SetReminders(f.ctx, e.ID, []model.EventReminder{{MinutesBefore: -1}}), model.ErrValidation)
}

func TestEventSearchAndFTS(t *testing.T) {
	f := newEventFx(t)
	_, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "Budget review", Start: utc(2026, 10, 16, 14, 0), TZ: "UTC", Location: ptr("Boardroom")})
	require.NoError(t, err)
	_, err = f.er.Create(f.ctx, model.CreateEventInput{Title: "Dentist", Start: utc(2026, 10, 17, 14, 0), TZ: "UTC"})
	require.NoError(t, err)
	got, err := f.er.Search(f.ctx, "boardroom", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Budget review", got[0].Title)

	// The index follows edits.
	_, err = f.er.Update(f.ctx, got[0].ID, model.UpdateEventInput{Location: ptr("Annex")}, "", "")
	require.NoError(t, err)
	got, err = f.er.Search(f.ctx, "boardroom", 10)
	require.NoError(t, err)
	assert.Empty(t, got)
	got, err = f.er.Search(f.ctx, "annex", 10)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestEventExternalIDAndLookup(t *testing.T) {
	f := newEventFx(t)
	e, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "x", Start: utc(2026, 10, 16, 14, 0), TZ: "UTC"})
	require.NoError(t, err)
	got, err := f.er.FindByUUID(f.ctx, e.UUID)
	require.NoError(t, err)
	assert.Equal(t, e.ID, got.ID)
	_, err = f.er.FindByUUID(f.ctx, "nope")
	assert.ErrorIs(t, err, model.ErrNotFound)

	l, err := repo.LookupRecord(f.ctx, f.d, "event", e.ID)
	require.NoError(t, err)
	assert.Equal(t, "x", l.Label)
	_, err = repo.NewExternalIDRepo(f.d).Upsert(f.ctx, repo.ExternalIDInput{Entity: "event", LocalID: e.ID, Source: "ics", ExternalID: "uid-1"})
	require.NoError(t, err)
}

func TestEventTagsUseTheSharedTagTable(t *testing.T) {
	f := newEventFx(t)
	e, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "x", Start: utc(2026, 10, 16, 14, 0), TZ: "UTC"})
	require.NoError(t, err)
	tr := repo.NewTagRepo(f.d)
	require.NoError(t, tr.Apply(f.ctx, "event", e.ID, "work"))
	got, err := f.er.FindByID(f.ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"work"}, got.Tags)
	require.NoError(t, tr.Remove(f.ctx, "event", e.ID, "work"))
	assert.ErrorIs(t, tr.Apply(f.ctx, "event", 999, "work"), model.ErrNotFound)
}

// 5,000 events: a month view must stay fast.
func TestEventListPerformance(t *testing.T) {
	f := newEventFx(t)
	tx, err := f.d.Begin()
	require.NoError(t, err)
	base := utc(2025, 1, 1, 9, 0)
	for i := 0; i < 5000; i++ {
		s := base.Add(time.Duration(i) * 3 * time.Hour)
		_, err := tx.Exec(`INSERT INTO events (uuid, title, start_utc, end_utc, tz) VALUES (?, ?, ?, ?, 'UTC')`,
			"u"+time.Duration(i).String(), "event", tz.FormatInstant(s), tz.FormatInstant(s.Add(time.Hour)))
		require.NoError(t, err)
	}
	for i := 0; i < 20; i++ {
		_, err := tx.Exec(`INSERT INTO events (uuid, title, start_utc, end_utc, tz, rrule) VALUES (?, 'weekly', ?, ?, 'America/Chicago', 'FREQ=WEEKLY')`,
			"w"+time.Duration(i).String(), tz.FormatInstant(base), tz.FormatInstant(base.Add(time.Hour)))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())

	start := time.Now()
	got, _, err := f.er.List(f.ctx, model.EventFilters{From: utc(2025, 6, 1, 0, 0), To: utc(2025, 7, 1, 0, 0), Loc: time.UTC})
	require.NoError(t, err)
	elapsed := time.Since(start)
	assert.Greater(t, len(got), 200)
	assert.Less(t, elapsed, 250*time.Millisecond, "a month of a 5,000 event calendar (budget 50 ms, relaxed for slow CI)")
}
