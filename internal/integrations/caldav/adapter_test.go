package caldav

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// now is Thursday 2026-10-15 07:00 in Chicago.
var fixedNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func ptr(s string) *string { return &s }

type env struct {
	t   *testing.T
	f   *fakeServer
	db  *sql.DB
	er  *repo.EventRepo
	run *crmsync.Runner
	cfg Config
}

func newEnv(t *testing.T, twoWay bool) *env {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	f := newFakeServer(t)
	cfg := f.config()
	cfg.TwoWay = twoWay
	cfg.SelfEmails = []string{"grant@example.com"}
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	e := &env{t: t, f: f, db: d, er: repo.NewEventRepo(d), cfg: cfg}
	e.run = crmsync.NewRunner(d, crmsync.NewRegistry(&Adapter{Cfg: cfg, Now: func() time.Time { return fixedNow }, Loc: loc}))
	_, err = repo.NewPersonRepo(d).Create(ctx, model.CreatePersonInput{FirstName: "Jane", Email: ptr("jane@example.com")})
	require.NoError(t, err)
	return e
}

func (e *env) sync(dry bool) *crmsync.RunResult {
	e.t.Helper()
	res, err := e.run.Run(ctx, Source, crmsync.RunOptions{DryRun: dry})
	require.NoError(e.t, err)
	return res
}

func (e *env) setTwoWay(on bool) {
	e.cfg.TwoWay = on
	loc, _ := tz.Load("America/Chicago")
	e.run = crmsync.NewRunner(e.db, crmsync.NewRegistry(&Adapter{Cfg: e.cfg, Now: func() time.Time { return fixedNow }, Loc: loc}))
}

func vevent(lines ...string) string {
	return "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//test//EN\nBEGIN:VEVENT\n" + strings.Join(lines, "\n") + "\nEND:VEVENT\nEND:VCALENDAR\n"
}

func standup(title, modified string) string {
	return vevent("UID:standup", "DTSTAMP:20261001T000000Z", "LAST-MODIFIED:"+modified, "SUMMARY:"+title,
		"DTSTART;TZID=America/Chicago:20261012T090000", "DTEND;TZID=America/Chicago:20261012T091500", "RRULE:FREQ=WEEKLY;BYDAY=MO")
}

const lunch = "UID:lunch\nDTSTAMP:20261001T000000Z\nLAST-MODIFIED:20261001T000000Z\nSUMMARY:Lunch with Jane\nDTSTART:20261001T170000Z\nDTEND:20261001T180000Z\n" +
	"ORGANIZER:mailto:grant@example.com\nATTENDEE:mailto:grant@example.com\nATTENDEE:mailto:jane@example.com\nATTENDEE:mailto:stranger@example.com"

func (e *env) seedServer() {
	e.f.set("standup", standup("Standup", "20261001T000000Z"))
	e.f.set("lunch", vevent(strings.Split(lunch, "\n")...))
}

func (e *env) events() map[string]*model.Event {
	e.t.Helper()
	ids, err := e.er.SeriesIDs(ctx)
	require.NoError(e.t, err)
	out := map[string]*model.Event{}
	for _, id := range ids {
		ev, err := e.er.FindByID(ctx, id)
		require.NoError(e.t, err)
		out[ev.Title] = ev
	}
	return out
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.db.QueryRow(q, args...).Scan(&n))
	return n
}

func TestInitialPullCreatesEventsMeetingsAndRemembersResources(t *testing.T) {
	e := newEnv(t, false)
	e.seedServer()
	e.f.set("coffee", vevent("UID:coffee", "DTSTAMP:20261001T000000Z", "SUMMARY:Coffee", "DTSTART:20261002T150000Z", "DTEND:20261002T160000Z",
		"ATTENDEE:mailto:grant@example.com", "ATTENDEE:mailto:nobody@example.com"))

	res := e.sync(false)
	assert.Equal(t, model.SyncOK, res.Status, res.Note)
	assert.Equal(t, 4, res.Created, "three events and the meeting with Jane")
	assert.Equal(t, 1, res.Inbox, "the coffee with an unknown person waits in the inbox")
	assert.Equal(t, 0, res.Errors)
	assert.Equal(t, "2026-10-15T12:00:00Z", res.NextCursor)

	evs := e.events()
	require.Len(t, evs, 3)
	su := evs["Standup"]
	assert.Equal(t, "America/Chicago", *su.TZ)
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO", *su.RRule)
	assert.Equal(t, "caldav", su.Source)
	assert.Equal(t, 3, e.count(`SELECT COUNT(*) FROM external_ids WHERE source = 'caldav' AND entity = 'event'`))
	assert.Equal(t, 3, e.count(`SELECT COUNT(*) FROM caldav_resources`))

	// One meeting on Jane's timeline, with the stranger left out and the user's own address ignored.
	var subject, occurred string
	require.NoError(t, e.db.QueryRow(`SELECT i.subject, i.occurred_at FROM interactions i JOIN interaction_people ip ON ip.interaction_id = i.id WHERE i.type = 'meeting'`).Scan(&subject, &occurred))
	assert.Equal(t, "Lunch with Jane", subject)
	assert.Equal(t, "2026-10-01 17:00:00", occurred)
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM interactions WHERE type = 'meeting'`))
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM sync_inbox WHERE source = 'caldav' AND status = 'pending'`))
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM event_people`), "Jane is an attendee of the lunch")

	// The recurring standup is not a past meeting.
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM external_ids WHERE entity = 'interaction' AND external_id LIKE 'standup%'`))

	// A second run changes nothing.
	again := e.sync(false)
	assert.Equal(t, model.SyncOK, again.Status)
	assert.Equal(t, 0, again.Created+again.Updated)
	assert.Equal(t, 3, again.Skipped)
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM interactions WHERE type = 'meeting'`), "the meeting is logged once")
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM sync_inbox WHERE source = 'caldav'`))
}

func TestDryRunCountsAndWritesNothing(t *testing.T) {
	e := newEnv(t, false)
	e.seedServer()
	res := e.sync(true)
	assert.True(t, res.DryRun)
	assert.Equal(t, 2, res.Created)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM events`))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM caldav_resources`))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM sync_runs`))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM interactions`))
}

func TestServerChangeUpdatesAndServerDeleteArchives(t *testing.T) {
	e := newEnv(t, false)
	e.seedServer()
	e.sync(false)

	e.f.set("standup", standup("Standup (moved)", "20261010T000000Z"))
	res := e.sync(false)
	assert.Equal(t, 1, res.Updated)
	assert.Contains(t, e.events(), "Standup (moved)")
	assert.NotContains(t, e.events(), "Standup")
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM events`), "updated in place")

	e.f.remove("lunch")
	res = e.sync(false)
	assert.Equal(t, 1, res.Updated)
	assert.Contains(t, res.Note, "archived an event the server deleted")
	assert.NotContains(t, e.events(), "Lunch with Jane")
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM events WHERE archived = 1`), "archived, not purged")
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM caldav_resources`))
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM interactions WHERE type = 'meeting'`), "the timeline entry stays")
}

func TestEmptyServerIsNotAMassDelete(t *testing.T) {
	e := newEnv(t, false)
	for _, uid := range []string{"a", "b", "c", "d"} {
		e.f.set(uid, vevent("UID:"+uid, "DTSTAMP:20261001T000000Z", "SUMMARY:Event "+uid, "DTSTART:20261020T150000Z", "DTEND:20261020T160000Z"))
	}
	e.sync(false)
	assert.Equal(t, 4, e.count(`SELECT COUNT(*) FROM events`))
	for _, uid := range []string{"a", "b", "c", "d"} {
		e.f.remove(uid)
	}
	res := e.sync(false)
	assert.Equal(t, model.SyncFailed, res.Status)
	assert.Contains(t, res.Note, "not treating that as a mass delete")
	assert.Equal(t, 4, e.count(`SELECT COUNT(*) FROM events WHERE archived = 0`))
}

func TestBothSidesChangedDifferentFieldsAreMerged(t *testing.T) {
	e := newEnv(t, false)
	e.seedServer()
	e.sync(false)
	su := e.events()["Standup"]
	_, err := e.er.Update(ctx, su.ID, model.UpdateEventInput{Location: ptr("Room 4")}, model.ScopeAll, "")
	require.NoError(t, err)
	e.f.set("standup", standup("Standup v2", "20261010T000000Z"))

	res := e.sync(false)
	assert.Equal(t, model.SyncOK, res.Status, res.Note)
	assert.Contains(t, res.Note, "merged")
	got, err := e.er.FindByID(ctx, su.ID)
	require.NoError(t, err)
	assert.Equal(t, "Standup v2", got.Title, "the server's change")
	assert.Equal(t, "Room 4", *got.Location, "the local change")

	// Nothing was sent: one way. The next two-way run sends the merged event.
	puts, _ := e.f.requests()
	assert.Empty(t, puts)
	e.setTwoWay(true)
	res = e.sync(false)
	assert.Equal(t, model.SyncOK, res.Status, res.Note)
	puts, _ = e.f.requests()
	assert.Equal(t, []string{fakeColl + "standup.ics"}, puts)
	obj, ok := e.f.get("standup")
	require.True(t, ok)
	assert.Contains(t, obj.data, "SUMMARY:Standup v2")
	assert.Contains(t, obj.data, "LOCATION:Room 4")

	e.f.resetRequests()
	res = e.sync(false)
	assert.Equal(t, 0, res.Created+res.Updated, "converged")
	puts, deletes := e.f.requests()
	assert.Empty(t, puts)
	assert.Empty(t, deletes)
}

func TestSameFieldConflictLaterChangeWins(t *testing.T) {
	e := newEnv(t, false)
	e.f.set("localwins", standupUID("localwins", "Original A", "20261001T000000Z"))
	e.f.set("serverwins", standupUID("serverwins", "Original B", "20261001T000000Z"))
	e.sync(false)
	evs := e.events()
	require.Len(t, evs, 2)
	var localID, serverID int64
	for _, ev := range evs {
		var uid string
		require.NoError(t, e.db.QueryRow(`SELECT external_id FROM external_ids WHERE entity = 'event' AND local_id = ?`, ev.ID).Scan(&uid))
		if uid == "localwins" {
			localID = ev.ID
		} else {
			serverID = ev.ID
		}
	}
	for _, id := range []int64{localID, serverID} {
		_, err := e.er.Update(ctx, id, model.UpdateEventInput{Title: ptr("Local edit")}, model.ScopeAll, "")
		require.NoError(t, err)
	}
	e.f.set("localwins", standupUID("localwins", "Server edit", "20000101T000000Z")) // the server edit is older
	e.f.set("serverwins", standupUID("serverwins", "Server edit", "20990101T000000Z"))

	res := e.sync(false)
	assert.Equal(t, model.SyncOK, res.Status, res.Note)
	a, err := e.er.FindByID(ctx, localID)
	require.NoError(t, err)
	b, err := e.er.FindByID(ctx, serverID)
	require.NoError(t, err)
	assert.Equal(t, "Local edit", a.Title, "the local change is newer")
	assert.Equal(t, "Server edit", b.Title, "the server change is newer")
}

func standupUID(uid, title, modified string) string {
	return vevent("UID:"+uid, "DTSTAMP:20261001T000000Z", "LAST-MODIFIED:"+modified, "SUMMARY:"+title,
		"DTSTART;TZID=America/Chicago:20261012T090000", "DTEND;TZID=America/Chicago:20261012T091500")
}

func TestOneWayNeverSendsAnything(t *testing.T) {
	e := newEnv(t, false)
	_, err := e.er.Create(ctx, model.CreateEventInput{Title: "Local only", Start: time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC), TZ: "UTC"})
	require.NoError(t, err)
	e.sync(false)
	assert.Equal(t, 0, e.f.count())
}

func TestTwoWayPushesNewEditedAndDeletedEvents(t *testing.T) {
	e := newEnv(t, true)
	ev, err := e.er.Create(ctx, model.CreateEventInput{Title: "Dentist", Start: time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC), TZ: "America/Chicago",
		Attendees: nil, Reminders: []model.EventReminder{{MinutesBefore: 30}}})
	require.NoError(t, err)

	res := e.sync(false)
	assert.Equal(t, model.SyncOK, res.Status, res.Note)
	assert.Equal(t, 1, res.Created)
	href := fakeColl + ev.UUID + ".ics"
	obj, ok := e.f.get(ev.UUID)
	require.True(t, ok, "new events go to the server under their UID")
	assert.Contains(t, obj.data, "SUMMARY:Dentist")
	assert.Contains(t, obj.data, "TRIGGER:-PT30M")
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM caldav_resources WHERE href = ?`, href))
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM external_ids WHERE source = 'caldav' AND external_id = ?`, ev.UUID))

	// Nothing changes: no request.
	e.f.resetRequests()
	res = e.sync(false)
	assert.Equal(t, 0, res.Created+res.Updated)
	puts, _ := e.f.requests()
	assert.Empty(t, puts)

	// An edit goes out as a conditional update.
	_, err = e.er.Update(ctx, ev.ID, model.UpdateEventInput{Title: ptr("Dentist (moved)")}, model.ScopeAll, "")
	require.NoError(t, err)
	res = e.sync(false)
	assert.Equal(t, 1, res.Updated)
	obj, _ = e.f.get(ev.UUID)
	assert.Contains(t, obj.data, "SUMMARY:Dentist (moved)")

	// Deleting it here deletes it there.
	require.NoError(t, e.er.Archive(ctx, ev.ID, model.ScopeAll, ""))
	e.f.resetRequests()
	res = e.sync(false)
	assert.Equal(t, 1, res.Updated)
	_, deletes := e.f.requests()
	assert.Equal(t, []string{href}, deletes)
	_, ok = e.f.get(ev.UUID)
	assert.False(t, ok)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM caldav_resources`))
}

func TestLocallyDeletedEventIsNotBroughtBack(t *testing.T) {
	e := newEnv(t, false)
	e.seedServer()
	e.sync(false)
	su := e.events()["Standup"]
	require.NoError(t, e.er.Archive(ctx, su.ID, model.ScopeAll, ""))
	e.f.set("standup", standup("Standup changed", "20261010T000000Z"))
	res := e.sync(false)
	assert.Equal(t, 0, res.Created+res.Updated)
	assert.NotContains(t, e.events(), "Standup changed")

	// A two-way run removes it from the server instead.
	e.setTwoWay(true)
	e.sync(false)
	_, ok := e.f.get("standup")
	assert.False(t, ok)
}

func TestTwoWayDryRunSendsNothing(t *testing.T) {
	e := newEnv(t, true)
	_, err := e.er.Create(ctx, model.CreateEventInput{Title: "Dentist", Start: time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC), TZ: "UTC"})
	require.NoError(t, err)
	res := e.sync(true)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 0, e.f.count())
}

func TestUnsupportedAndBadObjectsDoNotStopTheRun(t *testing.T) {
	e := newEnv(t, false)
	e.f.set("good", vevent("UID:good", "DTSTAMP:20261001T000000Z", "SUMMARY:Good", "DTSTART:20261020T150000Z", "DTEND:20261020T160000Z"))
	e.f.set("nostart", vevent("UID:nostart", "DTSTAMP:20261001T000000Z", "SUMMARY:No start"))
	res := e.sync(false)
	assert.Equal(t, model.SyncOK, res.Status, res.Note)
	assert.Contains(t, e.events(), "Good")
	assert.Equal(t, 2, res.Created+res.Skipped)
}

func TestClientConditionalWritesAndConfig(t *testing.T) {
	f := newFakeServer(t)
	cl, err := NewClient(f.config(), nil)
	require.NoError(t, err)
	body := []byte(vevent("UID:x", "DTSTAMP:20261001T000000Z", "SUMMARY:X", "DTSTART:20261020T150000Z", "DTEND:20261020T160000Z"))
	href := cl.HrefFor("x")
	assert.Equal(t, fakeColl+"x.ics", href)
	assert.Equal(t, fakeColl+"a_b_c.ics", cl.HrefFor("a/b c"))

	etag, err := cl.Put(ctx, href, body, "")
	require.NoError(t, err)
	assert.NotEmpty(t, etag)
	_, err = cl.Put(ctx, href, body, "")
	assert.ErrorIs(t, err, model.ErrConflict, "create-only refuses to overwrite")
	_, err = cl.Put(ctx, href, body, `"stale"`)
	assert.ErrorIs(t, err, model.ErrConflict, "a stale ETag is refused")
	etag2, err := cl.Put(ctx, href, body, etag)
	require.NoError(t, err)
	assert.NotEqual(t, etag, etag2)
	require.NoError(t, cl.Delete(ctx, href, etag2))
	require.NoError(t, cl.Delete(ctx, href, ""), "deleting what is gone is fine")

	_, err = NewClient(Config{URL: "not a url"}, nil)
	assert.ErrorIs(t, err, model.ErrValidation)

	env := map[string]string{"CRM_CALDAV_URL": "https://dav.example.com/u/c", "CRM_CALDAV_USER": "grant@example.com", "CRM_CALDAV_TWO_WAY": "1", "CRM_CALDAV_SELF": "A@x.com, b@x.com", "CRM_CALDAV_PASSWORD_FILE": "pw"}
	cfg, ok := FromEnv(func(k string) string { return env[k] }, func(string) ([]byte, error) { return []byte("hunter2\nignored\n"), nil })
	require.True(t, ok)
	assert.Equal(t, "hunter2", cfg.Password)
	assert.True(t, cfg.TwoWay)
	assert.Equal(t, []string{"a@x.com", "b@x.com", "grant@example.com"}, cfg.SelfEmails)
	_, ok = FromEnv(func(string) string { return "" }, nil)
	assert.False(t, ok)
}

func TestAdapterWithoutTheBuiltInSinkFails(t *testing.T) {
	a := &Adapter{Cfg: Config{URL: "http://127.0.0.1:1/"}}
	_, err := a.Pull(ctx, "", nil)
	assert.Error(t, err)
}

func TestUnreachableServerFailsTheRunNotTheProgram(t *testing.T) {
	e := newEnv(t, false)
	e.f.srv.Close()
	res := e.sync(false)
	assert.Equal(t, model.SyncFailed, res.Status)
	assert.NotEmpty(t, res.Note)
	assert.Equal(t, "", res.NextCursor)
}
