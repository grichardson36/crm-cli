package icsio_test

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/ics"
	"github.com/jdanielnd/crm-cli/internal/cal/icsio"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func ptr(s string) *string { return &s }

func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

func chicago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return loc
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func addPerson(t *testing.T, d *sql.DB, first, email string) int64 {
	t.Helper()
	p, err := repo.NewPersonRepo(d).Create(ctx, model.CreatePersonInput{FirstName: first, Email: ptr(email)})
	require.NoError(t, err)
	return p.ID
}

// seed builds a calendar that uses every feature the file format carries.
func seed(t *testing.T, d *sql.DB) {
	t.Helper()
	jane := addPerson(t, d, "Jane", "jane@example.com")
	er := repo.NewEventRepo(d)
	tr := repo.NewTaskRepo(d)

	_, err := er.Create(ctx, model.CreateEventInput{
		Title: "Lunch, with Jane", Notes: ptr("bring the deck;\nsecond line"), Location: ptr("Cafe Rio"),
		Start: utc(2026, 10, 16, 17, 30), End: utc(2026, 10, 16, 18, 30), TZ: "America/Chicago",
		Attendees: []model.EventAttendee{{PersonID: jane, Role: "organizer", RSVP: "accepted"}},
		Reminders: []model.EventReminder{{MinutesBefore: 15}, {MinutesBefore: 1440, Method: "email"}},
		Tags:      []string{"lunch", "work"}, Status: "tentative",
	})
	require.NoError(t, err)

	series, err := er.Create(ctx, model.CreateEventInput{
		Title: "Standup", Start: utc(2026, 3, 2, 15, 0), End: utc(2026, 3, 2, 15, 15), TZ: "America/Chicago",
		RRule: "every mon", Exdates: []time.Time{utc(2026, 3, 9, 14, 0)}, Busy: boolp(false),
	})
	require.NoError(t, err)
	_, err = er.Update(ctx, series.ID, model.UpdateEventInput{Title: ptr("Standup (late)"), Start: timep(utc(2026, 3, 16, 15, 0))}, "this", "2026-03-16T14:00:00Z")
	require.NoError(t, err)

	_, err = er.Create(ctx, model.CreateEventInput{Title: "Thanksgiving", AllDay: true, Start: utc(2026, 11, 26, 0, 0)})
	require.NoError(t, err)
	_, err = er.Create(ctx, model.CreateEventInput{Title: "Conference", AllDay: true, Start: utc(2026, 11, 10, 0, 0), End: utc(2026, 11, 13, 0, 0), RRule: "FREQ=YEARLY;COUNT=2"})
	require.NoError(t, err)
	_, err = er.Create(ctx, model.CreateEventInput{Title: "Floating", Start: utc(2026, 10, 20, 14, 0), TZ: "America/Chicago", Floating: true})
	require.NoError(t, err)
	_, err = er.Create(ctx, model.CreateEventInput{Title: "Call", Start: utc(2026, 10, 21, 14, 0), TZ: "UTC"})
	require.NoError(t, err)

	_, err = tr.Create(ctx, model.CreateTaskInput{Title: "Send proposal", Priority: "high", DueAt: ptr("2026-11-01"), Description: ptr("v2")})
	require.NoError(t, err)
	done, err := tr.Create(ctx, model.CreateTaskInput{Title: "File taxes", Priority: "low", DueAt: ptr("2026-10-30T15:00:00Z")})
	require.NoError(t, err)
	_, err = tr.Complete(ctx, done.ID)
	require.NoError(t, err)
}

func boolp(b bool) *bool           { return &b }
func timep(t time.Time) *time.Time { return &t }

func exportAll(t *testing.T, d *sql.DB) *icsio.ExportResult {
	t.Helper()
	res, err := icsio.Export(ctx, d, icsio.ExportOptions{Tasks: true, Name: "crm"})
	require.NoError(t, err)
	return res
}

func decode(t *testing.T, data []byte) *ics.Calendar {
	t.Helper()
	cal, err := ics.Decode(data, chicago(t))
	require.NoError(t, err)
	return cal
}

func imp(t *testing.T, d *sql.DB, data []byte, opt icsio.ImportOptions) *icsio.ImportResult {
	t.Helper()
	opt.Loc = chicago(t)
	res, err := icsio.Import(ctx, d, data, opt)
	require.NoError(t, err)
	return res
}

func TestExportCounts(t *testing.T) {
	d := openDB(t)
	seed(t, d)
	res := exportAll(t, d)
	assert.Equal(t, 6, res.Events)
	assert.Equal(t, 1, res.Overrides)
	assert.Equal(t, 2, res.Todos)
	assert.Equal(t, 0, res.AttendeesWithoutEmail)
	text := string(res.Data)
	assert.Contains(t, text, "X-WR-CALNAME:crm")
	assert.Contains(t, text, "BEGIN:VTODO")
	assert.Contains(t, text, "SUMMARY:Lunch\\, with Jane")
	assert.Contains(t, text, "CATEGORIES:lunch,work")
	assert.Contains(t, text, "ORGANIZER;CN=Jane:mailto:jane@example.com")
	assert.Contains(t, text, "TRANSP:TRANSPARENT")
	assert.Contains(t, text, "STATUS:TENTATIVE")
	assert.Contains(t, text, "RRULE:FREQ=YEARLY;COUNT=2")
	assert.Contains(t, text, "DTSTART;VALUE=DATE:20261110")
	assert.Contains(t, text, "DTEND;VALUE=DATE:20261113")
}

// The acceptance test of the pass: export, import into another database, export again. Every
// event keeps its UID and every modeled field.
func TestRoundTripIsLossless(t *testing.T) {
	a := openDB(t)
	seed(t, a)
	first := exportAll(t, a)

	b := openDB(t)
	addPerson(t, b, "Jane", "JANE@example.com") // same address, any case
	res := imp(t, b, first.Data, icsio.ImportOptions{})
	assert.Equal(t, icsio.Counts{Created: 6}, res.Events)
	assert.Equal(t, icsio.Counts{Created: 1}, res.Overrides)
	assert.Equal(t, icsio.Counts{Created: 2}, res.Todos)
	assert.Empty(t, res.Warnings)

	second := exportAll(t, b)
	ca, cb := decode(t, first.Data), decode(t, second.Data)
	require.Len(t, cb.Events, len(ca.Events))
	index := func(c *ics.Calendar) map[string]*ics.Event {
		m := map[string]*ics.Event{}
		for _, e := range c.Events {
			key := e.UID
			if e.RecurrenceID != nil {
				key += "#" + tz.FormatInstant(*e.RecurrenceID)
			}
			m[key] = e
		}
		return m
	}
	ia, ib := index(ca), index(cb)
	require.Len(t, ib, len(ia))
	for key, x := range ia {
		y, ok := ib[key]
		require.True(t, ok, "UID %s survives", key)
		assert.Equal(t, x.Summary, y.Summary, key)
		assert.Equal(t, x.Description, y.Description, key)
		assert.Equal(t, x.Location, y.Location, key)
		assert.True(t, x.Start.Equal(y.Start), key)
		assert.True(t, x.End.Equal(y.End), key)
		assert.Equal(t, x.AllDay, y.AllDay, key)
		assert.Equal(t, x.TZ, y.TZ, key)
		assert.Equal(t, x.Floating, y.Floating, key)
		assert.Equal(t, x.RRule, y.RRule, key)
		assert.Equal(t, x.Exdates, y.Exdates, key)
		assert.Equal(t, x.Status, y.Status, key)
		assert.Equal(t, x.Transparent, y.Transparent, key)
		assert.ElementsMatch(t, x.Categories, y.Categories, key)
		assert.ElementsMatch(t, x.Alarms, y.Alarms, key)
		assert.Equal(t, x.Attendees, y.Attendees, key)
		assert.Equal(t, x.Organizer, y.Organizer, key)
	}
	require.Len(t, cb.Todos, 2)
	for i, td := range ca.Todos {
		assert.Equal(t, td.UID, cb.Todos[i].UID)
		assert.Equal(t, td.Summary, cb.Todos[i].Summary)
		assert.Equal(t, td.Status, cb.Todos[i].Status)
		assert.Equal(t, td.Priority, cb.Todos[i].Priority)
		assert.Equal(t, td.Due, cb.Todos[i].Due)
		assert.Equal(t, td.DueIsDate, cb.Todos[i].DueIsDate)
	}

	// The floating event is still floating after the trip.
	assert.Contains(t, string(second.Data), "SUMMARY:Floating\r\nDTSTART:20261020T090000\r\n")
	assert.True(t, ib[findUID(ca, "Floating")].Floating)

	// Instances read the same in both databases.
	listA, _, err := repo.NewEventRepo(a).List(ctx, model.EventFilters{From: utc(2026, 3, 1, 0, 0), To: utc(2026, 4, 5, 0, 0), Loc: chicago(t)})
	require.NoError(t, err)
	listB, _, err := repo.NewEventRepo(b).List(ctx, model.EventFilters{From: utc(2026, 3, 1, 0, 0), To: utc(2026, 4, 5, 0, 0), Loc: chicago(t)})
	require.NoError(t, err)
	require.Len(t, listB, len(listA))
	for i := range listA {
		assert.Equal(t, listA[i].Title, listB[i].Title)
		assert.Equal(t, listA[i].Start, listB[i].Start)
		assert.Equal(t, listA[i].Occurrence, listB[i].Occurrence)
	}
}

func findUID(c *ics.Calendar, summary string) string {
	for _, e := range c.Events {
		if e.Summary == summary {
			return e.UID
		}
	}
	return ""
}

func TestReimportIsIdempotent(t *testing.T) {
	a := openDB(t)
	seed(t, a)
	data := exportAll(t, a).Data

	var before string
	require.NoError(t, a.QueryRow(`SELECT group_concat(updated_at) FROM events`).Scan(&before))

	t.Run("into the database that made the file", func(t *testing.T) {
		res := imp(t, a, data, icsio.ImportOptions{})
		assert.Equal(t, icsio.Counts{Unchanged: 6}, res.Events)
		assert.Equal(t, icsio.Counts{Unchanged: 1}, res.Overrides)
		assert.Equal(t, icsio.Counts{Unchanged: 2}, res.Todos)
		var after string
		require.NoError(t, a.QueryRow(`SELECT group_concat(updated_at) FROM events`).Scan(&after))
		assert.Equal(t, before, after, "unchanged rows are not touched")
		var n int
		require.NoError(t, a.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n))
		assert.Equal(t, 7, n)
	})

	t.Run("into a fresh database, twice", func(t *testing.T) {
		b := openDB(t)
		addPerson(t, b, "Jane", "jane@example.com")
		imp(t, b, data, icsio.ImportOptions{})
		var n1, n2, t1, t2 int
		require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n1))
		require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&t1))
		res := imp(t, b, data, icsio.ImportOptions{})
		assert.Equal(t, icsio.Counts{Unchanged: 6}, res.Events)
		assert.Equal(t, icsio.Counts{Unchanged: 1}, res.Overrides)
		assert.Equal(t, icsio.Counts{Unchanged: 2}, res.Todos)
		require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n2))
		require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&t2))
		assert.Equal(t, n1, n2)
		assert.Equal(t, t1, t2)
	})
}

func TestReimportAppliesChangesAndDryRunWritesNothing(t *testing.T) {
	a := openDB(t)
	seed(t, a)
	data := exportAll(t, a).Data
	b := openDB(t)
	addPerson(t, b, "Jane", "jane@example.com")
	imp(t, b, data, icsio.ImportOptions{})

	edited := strings.Replace(string(data), "SUMMARY:Call\r\n", "SUMMARY:Call (renamed)\r\n", 1)
	edited = strings.Replace(edited, "SUMMARY:Send proposal\r\n", "SUMMARY:Send proposal v3\r\n", 1)
	require.NotEqual(t, string(data), edited)

	dry := imp(t, b, []byte(edited), icsio.ImportOptions{DryRun: true})
	assert.True(t, dry.DryRun)
	assert.Equal(t, icsio.Counts{Updated: 1, Unchanged: 5}, dry.Events)
	assert.Equal(t, icsio.Counts{Updated: 1, Unchanged: 1}, dry.Todos)
	var title string
	require.NoError(t, b.QueryRow(`SELECT title FROM events WHERE title LIKE 'Call%'`).Scan(&title))
	assert.Equal(t, "Call", title)

	res := imp(t, b, []byte(edited), icsio.ImportOptions{})
	assert.Equal(t, icsio.Counts{Updated: 1, Unchanged: 5}, res.Events)
	assert.Equal(t, icsio.Counts{Updated: 1, Unchanged: 1}, res.Todos)
	require.NoError(t, b.QueryRow(`SELECT title FROM events WHERE title LIKE 'Call%'`).Scan(&title))
	assert.Equal(t, "Call (renamed)", title)
	require.NoError(t, b.QueryRow(`SELECT title FROM tasks WHERE title LIKE 'Send%'`).Scan(&title))
	assert.Equal(t, "Send proposal v3", title)
}

func TestDryRunOnAnEmptyDatabase(t *testing.T) {
	a := openDB(t)
	seed(t, a)
	data := exportAll(t, a).Data
	b := openDB(t)
	res := imp(t, b, data, icsio.ImportOptions{DryRun: true})
	assert.Equal(t, icsio.Counts{Created: 6}, res.Events)
	assert.Equal(t, icsio.Counts{Created: 1}, res.Overrides, "overrides of series that would be created are counted")
	assert.Equal(t, icsio.Counts{Created: 2}, res.Todos)
	var n int
	require.NoError(t, b.QueryRow(`SELECT (SELECT COUNT(*) FROM events) + (SELECT COUNT(*) FROM tasks) + (SELECT COUNT(*) FROM external_ids)`).Scan(&n))
	assert.Equal(t, 0, n)
}

func TestImportEdgeCases(t *testing.T) {
	b := openDB(t)
	data := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:hourly
SUMMARY:Hourly thing
DTSTART:20261016T173000Z
RRULE:FREQ=HOURLY;COUNT=3
END:VEVENT
BEGIN:VEVENT
UID:orphan
RECURRENCE-ID:20261017T173000Z
SUMMARY:Orphan override
DTSTART:20261017T180000Z
END:VEVENT
BEGIN:VEVENT
UID:weekly
SUMMARY:Weekly
DTSTART:20261016T173000Z
RRULE:FREQ=WEEKLY;COUNT=3
END:VEVENT
BEGIN:VEVENT
UID:weekly
RECURRENCE-ID:20261017T173000Z
SUMMARY:Not an instance
DTSTART:20261017T180000Z
END:VEVENT
BEGIN:VEVENT
UID:weekly
RECURRENCE-ID:20261023T173000Z
SUMMARY:Weekly
DTSTART:20261023T173000Z
END:VEVENT
BEGIN:VEVENT
UID:bad end
SUMMARY:Ends before it starts
DTSTART:20261016T173000Z
DTEND:20261016T163000Z
END:VEVENT
BEGIN:VTODO
UID:t1
SUMMARY:Repeating task
DUE:20261101T120000Z
RRULE:FREQ=WEEKLY
PRIORITY:7
END:VTODO
END:VCALENDAR
`
	res := imp(t, b, []byte(data), icsio.ImportOptions{})
	all := strings.Join(res.Warnings, "\n")
	assert.Contains(t, all, "not supported, imported as a single event")
	assert.Contains(t, all, "has no series")
	assert.Contains(t, all, "ends before it starts")
	assert.Equal(t, 3, res.Events.Created)
	assert.Equal(t, 2, res.Overrides.Skipped, "the orphan and the non-instance are skipped")
	assert.Equal(t, 1, res.Overrides.Unchanged, "an override equal to the series needs no row")
	assert.Equal(t, 1, res.Todos.Created)

	var rrule sql.NullString
	require.NoError(t, b.QueryRow(`SELECT rrule FROM events WHERE title = 'Hourly thing'`).Scan(&rrule))
	assert.False(t, rrule.Valid)
	var n int
	require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM events WHERE recurrence_parent_id IS NOT NULL`).Scan(&n))
	assert.Equal(t, 0, n)
	var priority, due string
	require.NoError(t, b.QueryRow(`SELECT priority, due_at FROM tasks WHERE title = 'Repeating task'`).Scan(&priority, &due))
	assert.Equal(t, "low", priority)
	assert.Equal(t, "2026-11-01T12:00:00Z", due)
}

func TestImportRejectsGarbage(t *testing.T) {
	b := openDB(t)
	_, err := icsio.Import(ctx, b, []byte("not a calendar"), icsio.ImportOptions{})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = icsio.Import(ctx, b, []byte("BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR\n"), icsio.ImportOptions{})
	require.NoError(t, err, "an empty calendar is fine")
}

func TestImportedSeriesKeepsLocalTimeAcrossDST(t *testing.T) {
	b := openDB(t)
	data := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:s\nSUMMARY:Standup\nDTSTART;TZID=America/Chicago:20260302T090000\nDTEND;TZID=America/Chicago:20260302T091500\nRRULE:FREQ=WEEKLY;BYDAY=MO\nEND:VEVENT\nEND:VCALENDAR\n"
	imp(t, b, []byte(data), icsio.ImportOptions{})
	list, _, err := repo.NewEventRepo(b).List(ctx, model.EventFilters{From: utc(2026, 3, 1, 0, 0), To: utc(2026, 3, 24, 0, 0), Loc: chicago(t)})
	require.NoError(t, err)
	var got []string
	for _, l := range list {
		got = append(got, l.Start)
	}
	assert.Equal(t, []string{"2026-03-02T15:00:00Z", "2026-03-09T14:00:00Z", "2026-03-16T14:00:00Z", "2026-03-23T14:00:00Z"}, got)
}

func TestImportTaskStateFollowsTheFile(t *testing.T) {
	b := openDB(t)
	open := "BEGIN:VCALENDAR\nBEGIN:VTODO\nUID:t\nSUMMARY:Draft\nSTATUS:NEEDS-ACTION\nEND:VTODO\nEND:VCALENDAR\n"
	done := "BEGIN:VCALENDAR\nBEGIN:VTODO\nUID:t\nSUMMARY:Draft\nSTATUS:COMPLETED\nEND:VTODO\nEND:VCALENDAR\n"
	imp(t, b, []byte(open), icsio.ImportOptions{})
	res := imp(t, b, []byte(done), icsio.ImportOptions{})
	assert.Equal(t, 1, res.Todos.Updated)
	var completed int
	require.NoError(t, b.QueryRow(`SELECT completed FROM tasks`).Scan(&completed))
	assert.Equal(t, 1, completed)

	// The file wins: open in the file reopens the task.
	res = imp(t, b, []byte(open), icsio.ImportOptions{})
	assert.Equal(t, 1, res.Todos.Updated)
	require.NoError(t, b.QueryRow(`SELECT completed FROM tasks`).Scan(&completed))
	assert.Equal(t, 0, completed)

	res = imp(t, b, []byte(open), icsio.ImportOptions{SkipTasks: true})
	assert.Equal(t, icsio.Counts{}, res.Todos)
}

func TestExportRangeAndMissingEmails(t *testing.T) {
	d := openDB(t)
	er := repo.NewEventRepo(d)
	noEmail, err := repo.NewPersonRepo(d).Create(ctx, model.CreatePersonInput{FirstName: "NoMail"})
	require.NoError(t, err)
	_, err = er.Create(ctx, model.CreateEventInput{Title: "In", Start: utc(2026, 10, 16, 14, 0), TZ: "UTC", Attendees: []model.EventAttendee{{PersonID: noEmail.ID}}})
	require.NoError(t, err)
	_, err = er.Create(ctx, model.CreateEventInput{Title: "Out", Start: utc(2026, 12, 16, 14, 0), TZ: "UTC"})
	require.NoError(t, err)
	_, err = er.Create(ctx, model.CreateEventInput{Title: "Series from before", Start: utc(2026, 1, 5, 14, 0), TZ: "UTC", RRule: "weekly"})
	require.NoError(t, err)

	res, err := icsio.Export(ctx, d, icsio.ExportOptions{From: utc(2026, 10, 1, 0, 0), To: utc(2026, 11, 1, 0, 0)})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Events)
	assert.Equal(t, 1, res.AttendeesWithoutEmail)
	assert.Contains(t, string(res.Data), "SUMMARY:In")
	assert.NotContains(t, string(res.Data), "SUMMARY:Out")
	assert.NotContains(t, string(res.Data), "VTODO", "tasks only when asked")
}

func TestExportIncludesVTimezoneForEachZone(t *testing.T) {
	d := openDB(t)
	er := repo.NewEventRepo(d)
	for _, z := range []string{"America/Chicago", "Europe/London", "Australia/Sydney"} {
		_, err := er.Create(ctx, model.CreateEventInput{Title: z, Start: utc(2026, 6, 1, 14, 0), TZ: z})
		require.NoError(t, err)
	}
	res, err := icsio.Export(ctx, d, icsio.ExportOptions{})
	require.NoError(t, err)
	text := string(res.Data)
	for _, z := range []string{"America/Chicago", "Europe/London", "Australia/Sydney"} {
		assert.Contains(t, text, "BEGIN:VTIMEZONE\r\nTZID:"+z+"\r\n")
	}
	assert.Equal(t, 3, strings.Count(text, "BEGIN:VTIMEZONE"))
	assert.Contains(t, text, "TZNAME:BST")
	assert.Contains(t, text, "TZNAME:AEDT")
	// The file reads back with the same instants.
	cal, err := ics.Decode(res.Data, time.UTC)
	require.NoError(t, err)
	require.Len(t, cal.Events, 3)
	for _, e := range cal.Events {
		assert.Equal(t, "2026-06-01T14:00:00Z", tz.FormatInstant(e.Start), e.TZ)
	}
}

func TestImportIsPlainUIDMatching(t *testing.T) {
	data := []byte("BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:a@x\nSUMMARY:One\nDTSTART:20261016T173000Z\nATTENDEE;CN=Jane:mailto:Jane@Example.com\nEND:VEVENT\nEND:VCALENDAR\n")
	b := openDB(t)
	res := imp(t, b, data, icsio.ImportOptions{Source: "file-a"})
	assert.Equal(t, 1, res.Events.Created)
	var people, linked, guests int
	require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&people))
	require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM event_people`).Scan(&linked))
	require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM event_guests WHERE email = 'jane@example.com' AND name = 'Jane'`).Scan(&guests))
	assert.Equal(t, []int{0, 0, 1}, []int{people, linked, guests}, "unknown attendee is recorded on the event, nobody is created")
	assert.Empty(t, res.Warnings)

	// Same UID from another file updates the same event.
	res = imp(t, b, bytes.ReplaceAll(data, []byte("One"), []byte("Two")), icsio.ImportOptions{Source: "file-b"})
	assert.Equal(t, 1, res.Events.Updated)

	// Deleted locally: the file brings it back.
	var id int64
	require.NoError(t, b.QueryRow(`SELECT id FROM events`).Scan(&id))
	require.NoError(t, repo.NewEventRepo(b).Archive(ctx, id, "", ""))
	res = imp(t, b, data, icsio.ImportOptions{})
	assert.Equal(t, 1, res.Events.Created)

	// A known contact is linked instead.
	addPerson(t, b, "Jane", "jane@example.com")
	imp(t, b, bytes.ReplaceAll(data, []byte("One"), []byte("Three")), icsio.ImportOptions{})
	require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM event_people`).Scan(&linked))
	assert.Equal(t, 1, linked)
}
