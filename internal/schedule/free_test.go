package schedule

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

func ptr(s string) *string { return &s }

func TestSubtract(t *testing.T) {
	sp := func(h1, m1, h2, m2 int) Span { return Span{utc(2026, 1, 5, h1, m1), utc(2026, 1, 5, h2, m2)} }
	tests := []struct {
		name       string
		free, busy []Span
		want       []Span
	}{
		{"nothing busy", []Span{sp(9, 0, 17, 0)}, nil, []Span{sp(9, 0, 17, 0)}},
		{"middle", []Span{sp(9, 0, 17, 0)}, []Span{sp(12, 0, 13, 0)}, []Span{sp(9, 0, 12, 0), sp(13, 0, 17, 0)}},
		{"start overlap", []Span{sp(9, 0, 17, 0)}, []Span{sp(8, 0, 10, 0)}, []Span{sp(10, 0, 17, 0)}},
		{"end overlap", []Span{sp(9, 0, 17, 0)}, []Span{sp(16, 0, 18, 0)}, []Span{sp(9, 0, 16, 0)}},
		{"covers everything", []Span{sp(9, 0, 17, 0)}, []Span{sp(8, 0, 18, 0)}, nil},
		{"unsorted overlapping busy", []Span{sp(9, 0, 17, 0)}, []Span{sp(14, 0, 15, 0), sp(10, 0, 11, 30), sp(11, 0, 12, 0)}, []Span{sp(9, 0, 10, 0), sp(12, 0, 14, 0), sp(15, 0, 17, 0)}},
		{"busy outside", []Span{sp(9, 0, 10, 0)}, []Span{sp(11, 0, 12, 0)}, []Span{sp(9, 0, 10, 0)}},
		{"touching busy leaves the rest", []Span{sp(9, 0, 10, 0)}, []Span{sp(10, 0, 11, 0)}, []Span{sp(9, 0, 10, 0)}},
		{"two free spans", []Span{sp(9, 0, 12, 0), sp(13, 0, 17, 0)}, []Span{sp(11, 0, 14, 0)}, []Span{sp(9, 0, 11, 0), sp(14, 0, 17, 0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Subtract(tt.free, tt.busy))
		})
	}
}

func TestSlotsStepGridLengthAndLimit(t *testing.T) {
	loc := chicago(t)
	free := []Span{{utc(2026, 10, 14, 14, 7), utc(2026, 10, 14, 16, 0)}} // 09:07-11:00 CDT
	hm := func(sl []Span) []string {
		var out []string
		for _, s := range sl {
			out = append(out, s.Start.In(loc).Format("15:04")+"-"+s.End.In(loc).Format("15:04"))
		}
		return out
	}
	assert.Equal(t, []string{"09:15-10:00", "09:30-10:15", "09:45-10:30", "10:00-10:45", "10:15-11:00"}, hm(Slots(free, 45*time.Minute, 15*time.Minute, loc, 0)))
	assert.Equal(t, []string{"09:30-10:00", "10:00-10:30", "10:30-11:00"}, hm(Slots(free, 30*time.Minute, 30*time.Minute, loc, 0)), "a 30 minute step offers :00 and :30")
	assert.Equal(t, []string{"09:15-10:00", "09:30-10:15"}, hm(Slots(free, 45*time.Minute, 15*time.Minute, loc, 2)))
	assert.Empty(t, Slots(free, 2*time.Hour, 15*time.Minute, loc, 0), "longer than the gap")
	assert.Nil(t, Slots(free, 0, 15*time.Minute, loc, 0))
	assert.Nil(t, Slots(free, time.Hour, 0, loc, 0))
}

func TestSlotsGridFollowsTheClockAcrossDaylightSaving(t *testing.T) {
	loc := chicago(t)
	// 2026-11-01 is the 25 hour day. 09:00 local is 15:00Z there, 14:00Z the day before.
	free := []Span{{utc(2026, 10, 31, 13, 0), utc(2026, 10, 31, 15, 0)}, {utc(2026, 11, 1, 14, 0), utc(2026, 11, 1, 16, 0)}}
	var got []string
	for _, s := range Slots(free, time.Hour, time.Hour, loc, 0) {
		got = append(got, s.Start.In(loc).Format("2006-01-02 15:04"))
	}
	assert.Equal(t, []string{"2026-10-31 08:00", "2026-10-31 09:00", "2026-11-01 08:00", "2026-11-01 09:00"}, got)
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func seedHours(t *testing.T, d *sql.DB) {
	t.Helper()
	_, err := repo.NewAvailabilityRepo(d).SetDays(ctx, "", "America/Chicago", model.WeekdayKeys[:5], []model.TimeRange{{Start: "09:00", End: "17:00"}})
	require.NoError(t, err)
}

func event(t *testing.T, d *sql.DB, in model.CreateEventInput) *model.Event {
	t.Helper()
	if in.TZ == "" {
		in.TZ = "America/Chicago"
	}
	e, err := repo.NewEventRepo(d).Create(ctx, in)
	require.NoError(t, err)
	return e
}

func localSlots(t *testing.T, res *FindResult) []string {
	t.Helper()
	loc := chicago(t)
	var out []string
	for _, s := range res.Slots {
		st, err := tz.ParseInstant(s.Start)
		require.NoError(t, err)
		en, err := tz.ParseInstant(s.End)
		require.NoError(t, err)
		out = append(out, st.In(loc).Format("Mon 15:04")+"-"+en.In(loc).Format("15:04"))
	}
	return out
}

// now is Wednesday 2026-10-14 08:00 in Chicago.
var now = utc(2026, 10, 14, 13, 0)

func find(t *testing.T, d *sql.DB, p FindParams) *FindResult {
	t.Helper()
	p.Now = now
	res, err := FindTime(ctx, d, p)
	require.NoError(t, err)
	return res
}

func TestFindTimeAroundEventsWithBuffers(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	event(t, d, model.CreateEventInput{Title: "Meeting", Start: utc(2026, 10, 14, 15, 0), End: utc(2026, 10, 14, 16, 0)}) // 10:00-11:00 CDT
	to := utc(2026, 10, 14, 19, 0)                                                                                        // 14:00 CDT

	res := find(t, d, FindParams{Length: 30 * time.Minute, To: to})
	assert.Equal(t, 10, res.BufferMin)
	assert.Equal(t, 15, res.StepMin)
	assert.Equal(t, "America/Chicago", res.TZ)
	assert.Equal(t, []string{
		"Wed 09:00-09:30", "Wed 09:15-09:45", // the buffer keeps 09:50-11:10 clear
		"Wed 11:15-11:45", "Wed 11:30-12:00", "Wed 11:45-12:15", "Wed 12:00-12:30", "Wed 12:15-12:45", "Wed 12:30-13:00",
		"Wed 12:45-13:15", "Wed 13:00-13:30", "Wed 13:15-13:45", "Wed 13:30-14:00",
	}, localSlots(t, res))

	zero := time.Duration(0)
	res = find(t, d, FindParams{Length: 30 * time.Minute, To: to, Buffer: &zero, Limit: 4})
	assert.Equal(t, []string{"Wed 09:00-09:30", "Wed 09:15-09:45", "Wed 09:30-10:00", "Wed 11:00-11:30"}, localSlots(t, res), "no buffer, limited")

	half := 30 * time.Minute
	res = find(t, d, FindParams{Length: time.Hour, To: to, Step: &half, Buffer: &zero})
	assert.Equal(t, []string{"Wed 09:00-10:00", "Wed 11:00-12:00", "Wed 11:30-12:30", "Wed 12:00-13:00", "Wed 12:30-13:30", "Wed 13:00-14:00"}, localSlots(t, res), "an hour slot fits right up to the meeting when there is no buffer")
}

func TestFindTimeMinimumNoticeAndPast(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	to := utc(2026, 10, 14, 17, 0) // 12:00 CDT
	res := find(t, d, FindParams{Length: 30 * time.Minute, To: to, MinNotice: 90 * time.Minute})
	assert.Equal(t, "Wed 09:30-10:00", localSlots(t, res)[0], "08:00 plus 90 minutes of notice")
	res = find(t, d, FindParams{Length: 30 * time.Minute, From: utc(2026, 10, 14, 0, 0), To: to})
	assert.Equal(t, "Wed 09:00-09:30", localSlots(t, res)[0], "the window cannot start before now but the working day starts at 09:00")
	res = find(t, d, FindParams{Length: 30 * time.Minute, To: now})
	assert.Empty(t, res.Slots)
}

func TestFindTimeRecurringBusyAndOtherKinds(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	// Daily 09:00-09:30 standup, weekdays.
	event(t, d, model.CreateEventInput{Title: "Standup", Start: utc(2026, 10, 12, 14, 0), End: utc(2026, 10, 12, 14, 30), RRule: "weekdays"})
	// None of these block: free, cancelled.
	free := false
	event(t, d, model.CreateEventInput{Title: "Lunch break", Start: utc(2026, 10, 14, 17, 0), End: utc(2026, 10, 14, 18, 0), Busy: &free})
	event(t, d, model.CreateEventInput{Title: "Called off", Start: utc(2026, 10, 14, 19, 0), End: utc(2026, 10, 14, 20, 0), Status: "cancelled"})
	// A busy all-day event takes the whole Thursday.
	event(t, d, model.CreateEventInput{Title: "Conference", AllDay: true, Start: utc(2026, 10, 15, 0, 0)})

	res := find(t, d, FindParams{Length: time.Hour, To: utc(2026, 10, 16, 23, 0), Step: durp(time.Hour)})
	got := localSlots(t, res)
	assert.NotContains(t, got, "Wed 09:00-10:00", "the standup")
	assert.Contains(t, got, "Wed 10:00-11:00")
	assert.Contains(t, got, "Wed 12:00-13:00", "a transparent event does not block")
	assert.Contains(t, got, "Wed 14:00-15:00", "a cancelled event does not block")
	for _, s := range got {
		assert.NotContains(t, s, "Thu", "the all-day event blocks the day")
	}
	assert.Contains(t, got, "Fri 10:00-11:00")
	assert.NotContains(t, got, "Fri 09:00-10:00", "the repeat blocks every weekday")
}

func durp(d time.Duration) *time.Duration { return &d }

func TestFindTimeTasksAndDaysOff(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	tr := repo.NewTaskRepo(d)
	locked, err := tr.Create(ctx, model.CreateTaskInput{Title: "Locked block", Priority: "medium",
		Schedule: model.TaskSchedule{DurationMin: ip(60), ScheduledStart: ptr("2026-10-14T15:00:00Z")}})
	require.NoError(t, err)
	assert.True(t, locked.ScheduledLocked)
	_, err = tr.Create(ctx, model.CreateTaskInput{Title: "Floating block", Priority: "medium",
		Schedule: model.TaskSchedule{DurationMin: ip(60), ScheduledStart: ptr("2026-10-14T17:00:00Z"), ScheduledLocked: boolp(false)}})
	require.NoError(t, err)
	_, err = repo.NewAvailabilityRepo(d).SetOverride(ctx, "", "2026-10-15", nil)
	require.NoError(t, err)

	zero := time.Duration(0)
	p := FindParams{Length: time.Hour, To: utc(2026, 10, 16, 0, 0), Step: durp(time.Hour), Buffer: &zero}
	got := localSlots(t, find(t, d, p))
	assert.NotContains(t, got, "Wed 10:00-11:00", "a locked task is busy")
	assert.Contains(t, got, "Wed 12:00-13:00", "a placed task that is not locked is not")
	for _, s := range got {
		assert.NotContains(t, s, "Thu", "the day off")
	}

	p.BusyTasks = true
	assert.NotContains(t, localSlots(t, find(t, d, p)), "Wed 12:00-13:00")
}

func ip(n int) *int      { return &n }
func boolp(b bool) *bool { return &b }

func TestFindTimeFollowsDaylightSavingAndIsStable(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	p := FindParams{Length: time.Hour, From: utc(2026, 10, 30, 0, 0), To: utc(2026, 11, 3, 6, 0), Step: durp(time.Hour), Now: utc(2026, 10, 29, 0, 0)}
	a, err := FindTime(ctx, d, p)
	require.NoError(t, err)
	b, err := FindTime(ctx, d, p)
	require.NoError(t, err)
	assert.Equal(t, a, b, "the same input gives the same slots")
	got := localSlots(t, a)
	assert.Equal(t, "Fri 09:00-10:00", got[0])
	assert.Equal(t, "2026-10-30T14:00:00Z", a.Slots[0].Start, "Friday is still CDT")
	var monday string
	for i, s := range got {
		if s == "Mon 09:00-10:00" {
			monday = a.Slots[i].Start
		}
	}
	assert.Equal(t, "2026-11-02T15:00:00Z", monday, "Monday 09:00 is CST: the instant moved, the clock did not")
	assert.Len(t, got, 8*2, "two working days of eight one hour slots")
}

func TestFindTimeValidation(t *testing.T) {
	d := openDB(t)
	_, err := FindTime(ctx, d, FindParams{Now: now})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = FindTime(ctx, d, FindParams{Length: time.Hour, Now: now, Rule: "nope"})
	assert.ErrorIs(t, err, model.ErrNotFound)
	_, err = FindTime(ctx, d, FindParams{Length: time.Hour, Now: now, Step: durp(0)})
	assert.ErrorIs(t, err, model.ErrValidation)

	// The default horizon applies when no end is given.
	seedHours(t, d)
	res, err := FindTime(ctx, d, FindParams{Length: time.Hour, Now: now, Step: durp(time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, "2026-10-28T13:00:00Z", res.To, "14 days")
	assert.NotEmpty(t, res.Slots)
}
