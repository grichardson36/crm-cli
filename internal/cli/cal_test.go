package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// calCrm runs the binary against dbPath with a fixed clock (Thursday 2026-10-15 07:00 in Chicago)
// and a fixed device zone, in a throwaway HOME, so relative phrases give the same answer every run.
func calCrm(t *testing.T, dbPath string, args ...string) (string, string, int) {
	t.Helper()
	home := filepath.Join(filepath.Dir(dbPath), "home")
	require.NoError(t, os.MkdirAll(home, 0o700))
	full := append([]string{"--db", dbPath}, args...)
	return crmEnv(t, home, []string{"CRM_NOW=2026-10-15T12:00:00Z", "CRM_TZ=America/Chicago"}, full...)
}

func calOK(t *testing.T, dbPath string, args ...string) string {
	t.Helper()
	stdout, stderr, code := calCrm(t, dbPath, args...)
	require.Equal(t, 0, code, "crm %v\nstderr: %s", args, stderr)
	return stdout
}

func calJSON(t *testing.T, dbPath string, args ...string) []map[string]any {
	t.Helper()
	var out []map[string]any
	jsonOut(t, calOK(t, dbPath, append(args, "-f", "json")...), &out)
	return out
}

func newCalDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "crm.db")
}

func TestCalAddListShow(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "person", "add", "Sam Lee")

	added := calJSON(t, db, "cal", "add", "Lunch with Sam", "--start", "fri 12:30", "--duration", "1h",
		"--location", "Cafe Rio", "--person", "1", "--remind", "15m", "--tag", "lunch")
	require.Len(t, added, 1)
	ev := added[0]
	assert.Equal(t, "Lunch with Sam", ev["title"])
	assert.Equal(t, "2026-10-16T17:30:00Z", ev["start_utc"], "12:30 Chicago CDT is 17:30Z")
	assert.Equal(t, "2026-10-16T18:30:00Z", ev["end_utc"])
	assert.Equal(t, "America/Chicago", ev["tz"])
	assert.Equal(t, "2026-10-16 12:30", ev["start"])
	assert.NotEmpty(t, ev["uuid"])

	list := calJSON(t, db, "cal", "list", "--days", "7")
	require.Len(t, list, 1)
	assert.Equal(t, "Lunch with Sam", list[0]["title"])
	assert.Equal(t, "Fri 16 Oct", list[0]["date"])
	assert.Equal(t, "12:30-13:30", list[0]["time"])
	assert.Equal(t, "Cafe Rio", list[0]["location"])
	assert.EqualValues(t, 1, list[0]["attendees"])
	assert.Equal(t, "2026-10-16T12:30:00-05:00", list[0]["local_start"])

	// Another display zone changes the clock, not the instant.
	other := calJSON(t, db, "cal", "list", "--tz", "Europe/London")
	assert.Equal(t, "2026-10-16T18:30:00+01:00", other[0]["local_start"])
	assert.Equal(t, "2026-10-16T17:30:00Z", other[0]["start"])

	show := calJSON(t, db, "cal", "show", "1")
	require.Len(t, show, 1)
	assert.Equal(t, []any{"lunch"}, show[0]["tags"])
	att := show[0]["attendees"].([]any)
	require.Len(t, att, 1)
	assert.Equal(t, "Sam Lee", att[0].(map[string]any)["name"])
	assert.Len(t, show[0]["reminders"], 1)

	// Window filters.
	assert.Empty(t, calJSON(t, db, "cal", "list", "--from", "2026-10-20", "--days", "3"))
	assert.Len(t, calJSON(t, db, "cal", "list", "--person", "1"), 1)
	assert.Len(t, calJSON(t, db, "cal", "list", "--tag", "lunch"), 1)
	assert.Len(t, calJSON(t, db, "cal", "list", "--query", "lunch"), 1)
	assert.Empty(t, calJSON(t, db, "cal", "list", "--tag", "nope"))

	table := calOK(t, db, "cal", "list", "-f", "table")
	assert.Contains(t, table, "Lunch with Sam")
	assert.Contains(t, table, "12:30-13:30")
	assert.Equal(t, "1\n", calOK(t, db, "cal", "list", "-q"))
}

func TestCalAllDayAndMultiDay(t *testing.T) {
	db := newCalDB(t)
	e := calJSON(t, db, "cal", "add", "Conference", "--start", "2026-10-20", "--end", "2026-10-22")
	assert.Equal(t, true, e[0]["all_day"])
	assert.Equal(t, "2026-10-20", e[0]["start_date"])
	assert.Equal(t, "2026-10-23", e[0]["end_date"], "the end day is inclusive on the command line, exclusive when stored")
	assert.Nil(t, e[0]["tz"])

	h := calJSON(t, db, "cal", "add", "Holiday", "--start", "2026-11-26", "--all-day")
	assert.Equal(t, "2026-11-27", h[0]["end_date"])

	list := calJSON(t, db, "cal", "list", "--from", "2026-10-21", "--days", "1")
	require.Len(t, list, 1)
	assert.Equal(t, "Tue 20 Oct - Thu 22 Oct", list[0]["date"])
	assert.Equal(t, "all day", list[0]["time"])
}

func TestCalRepeatAndScopes(t *testing.T) {
	db := newCalDB(t)
	added := calJSON(t, db, "cal", "add", "Standup", "--start", "2026-10-19 09:15", "--duration", "15m", "--repeat", "weekdays", "--count", "10")
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;COUNT=10", added[0]["rrule"])
	assert.Equal(t, "every weekday, 10 times", added[0]["repeat"])

	list := calJSON(t, db, "cal", "list", "--from", "2026-10-19", "--days", "7")
	require.Len(t, list, 5)
	assert.Equal(t, true, list[0]["recurring"])
	assert.Equal(t, "2026-10-19T14:15:00Z", list[0]["occurrence"])

	// Edit one instance.
	calOK(t, db, "cal", "edit", "1", "--scope", "this", "--at", "2026-10-21T14:15:00Z", "--start", "2026-10-21 11:00", "--title", "Standup (late)")
	list = calJSON(t, db, "cal", "list", "--from", "2026-10-19", "--days", "7")
	require.Len(t, list, 5)
	titles := []string{}
	for _, l := range list {
		titles = append(titles, l["title"].(string))
	}
	assert.Equal(t, []string{"Standup", "Standup", "Standup (late)", "Standup", "Standup"}, titles)
	assert.Equal(t, "11:00-11:15", list[2]["time"])

	// Skip one instance.
	calOK(t, db, "cal", "rm", "1", "--scope", "this", "--at", "2026-10-22T14:15:00Z")
	assert.Len(t, calJSON(t, db, "cal", "list", "--from", "2026-10-19", "--days", "7"), 4)

	// Change this and the following ones.
	calOK(t, db, "cal", "edit", "1", "--scope", "following", "--at", "2026-10-26T14:15:00Z", "--location", "Annex")
	week2 := calJSON(t, db, "cal", "list", "--from", "2026-10-26", "--days", "7")
	require.Len(t, week2, 5)
	assert.Equal(t, "Annex", week2[0]["location"])
	week1 := calJSON(t, db, "cal", "list", "--from", "2026-10-19", "--days", "1")
	assert.Equal(t, "", week1[0]["location"])

	// The instance anchor can be a phrase in the event's zone.
	_, stderr, code := calCrm(t, db, "cal", "edit", "1", "--scope", "this", "--at", "2026-10-20", "--title", "x")
	assert.Equal(t, 2, code, "a date alone does not name a timed instance: %s", stderr)
	_, _, code = calCrm(t, db, "cal", "edit", "1", "--scope", "this", "--at", "2026-10-20T14:16:00Z", "--title", "x")
	assert.Equal(t, 3, code, "no instance starts then")
	_, _, code = calCrm(t, db, "cal", "edit", "1", "--scope", "sometimes", "--title", "x")
	assert.Equal(t, 2, code)
}

func TestCalEditAndDateMove(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "add", "Dentist", "--start", "2026-10-20 14:00", "--duration", "45m")
	// A date alone keeps the time of day.
	e := calJSON(t, db, "cal", "edit", "1", "--start", "2026-10-22")
	assert.Equal(t, "2026-10-22 14:00", e[0]["start"])
	assert.Equal(t, "2026-10-22 14:45", e[0]["end"], "the length is kept")

	e = calJSON(t, db, "cal", "edit", "1", "--end", "15:30")
	assert.Equal(t, "2026-10-22 15:30", e[0]["end"])

	e = calJSON(t, db, "cal", "edit", "1", "--status", "tentative", "--busy=false", "--notes", "bring forms", "--tz", "America/Denver")
	assert.Equal(t, "tentative", e[0]["status"])
	assert.Equal(t, false, e[0]["busy"])
	assert.Equal(t, "America/Denver", e[0]["tz"])

	e = calJSON(t, db, "cal", "edit", "1", "--remind", "10m", "--remind", "1h:email")
	assert.Len(t, e[0]["reminders"], 2)
	e = calJSON(t, db, "cal", "edit", "1", "--clear-reminders")
	assert.Nil(t, e[0]["reminders"])

	// Repeat added later, bounded, then cleared.
	e = calJSON(t, db, "cal", "edit", "1", "--repeat", "every fri", "--until", "2026-12-31")
	assert.Contains(t, e[0]["rrule"], "FREQ=WEEKLY;BYDAY=FR;UNTIL=2027010")
	e = calJSON(t, db, "cal", "edit", "1", "--repeat", "")
	assert.Nil(t, e[0]["rrule"])
	_, _, code := calCrm(t, db, "cal", "edit", "1", "--until", "2026-12-31")
	assert.Equal(t, 2, code, "an until needs a repeat")

	_, _, code = calCrm(t, db, "cal", "edit", "99", "--title", "x")
	assert.Equal(t, 3, code)
}

func TestCalDryRunWritesNothing(t *testing.T) {
	db := newCalDB(t)
	var plan map[string]any
	jsonOut(t, calOK(t, db, "cal", "add", "Ghost", "--start", "fri 12:30", "--duration", "1h", "--repeat", "weekdays", "--dry-run", "-f", "json"), &plan)
	assert.Equal(t, true, plan["dry_run"])
	ev := plan["event"].(map[string]any)
	assert.Equal(t, "2026-10-16T17:30:00Z", ev["start"], "the dry run shows the resolved time")
	assert.Equal(t, "2026-10-16T18:30:00Z", ev["end"])
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", ev["rrule"])
	assert.Empty(t, calJSON(t, db, "cal", "list", "--days", "30"))

	calOK(t, db, "cal", "add", "Real", "--start", "fri 12:30")
	jsonOut(t, calOK(t, db, "cal", "edit", "1", "--title", "Changed", "--start", "sat 10:00", "--dry-run", "-f", "json"), &plan)
	assert.Equal(t, true, plan["dry_run"])
	assert.Equal(t, "Changed", plan["event"].(map[string]any)["title"])
	assert.Equal(t, "2026-10-17T15:00:00Z", plan["event"].(map[string]any)["start_resolved"])
	assert.Equal(t, "Real", calJSON(t, db, "cal", "show", "1")[0]["title"])

	// The generic dry run covers delete, restore, purge and attendees.
	for _, args := range [][]string{{"cal", "rm", "1"}, {"cal", "restore", "1"}, {"cal", "purge", "1"}, {"cal", "attendee", "rm", "1", "1"}} {
		var p map[string]any
		jsonOut(t, calOK(t, db, append(args, "--dry-run", "-f", "json")...), &p)
		assert.Equal(t, true, p["dry_run"], args)
	}
	assert.Len(t, calJSON(t, db, "cal", "list", "--days", "30"), 1, "still there")
}

func TestCalDeleteRestorePurge(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "add", "Temp", "--start", "fri 12:30")
	calOK(t, db, "cal", "rm", "1")
	assert.Empty(t, calJSON(t, db, "cal", "list"))
	_, _, code := calCrm(t, db, "cal", "show", "1")
	assert.Equal(t, 3, code)

	calOK(t, db, "cal", "restore", "1")
	assert.Len(t, calJSON(t, db, "cal", "list"), 1)

	_, _, code = calCrm(t, db, "cal", "purge", "1", "--yes")
	assert.Equal(t, 4, code, "must be deleted first")
	calOK(t, db, "cal", "rm", "1")
	_, stderr, code := calCrm(t, db, "cal", "purge", "1")
	assert.Equal(t, 2, code, "purge needs --yes: %s", stderr)
	calOK(t, db, "cal", "purge", "1", "--yes")
	_, _, code = calCrm(t, db, "cal", "restore", "1")
	assert.Equal(t, 3, code)
}

func TestCalAttendees(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "person", "add", "Sam Lee")
	calOK(t, db, "person", "add", "Pat Kim")
	calOK(t, db, "cal", "add", "Sync", "--start", "fri 10:00")
	calOK(t, db, "cal", "attendee", "add", "1", "1", "--role", "organizer", "--rsvp", "accepted")
	e := calJSON(t, db, "cal", "attendee", "add", "1", "2")
	att := e[0]["attendees"].([]any)
	require.Len(t, att, 2)
	assert.Equal(t, "organizer", att[0].(map[string]any)["role"])
	assert.Equal(t, "Pat Kim", att[1].(map[string]any)["name"])

	calOK(t, db, "cal", "attendee", "rm", "1", "2")
	_, _, code := calCrm(t, db, "cal", "attendee", "rm", "1", "2")
	assert.Equal(t, 3, code)
	_, _, code = calCrm(t, db, "cal", "attendee", "add", "1", "99")
	assert.Equal(t, 3, code)
	_, _, code = calCrm(t, db, "cal", "attendee", "add", "1", "1", "--rsvp", "dunno")
	assert.Equal(t, 2, code)
}

func TestCalValidationErrors(t *testing.T) {
	db := newCalDB(t)
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"no start", []string{"cal", "add", "x"}, 2},
		{"bad start", []string{"cal", "add", "x", "--start", "whenever"}, 2},
		{"bad zone", []string{"cal", "add", "x", "--start", "fri 10:00", "--tz", "Mars/Base"}, 2},
		{"end before start", []string{"cal", "add", "x", "--start", "fri 10:00", "--end", "2026-10-16 09:00"}, 2},
		{"bad duration", []string{"cal", "add", "x", "--start", "fri 10:00", "--duration", "soon"}, 2},
		{"bad repeat", []string{"cal", "add", "x", "--start", "fri 10:00", "--repeat", "now and then"}, 2},
		{"until and count", []string{"cal", "add", "x", "--start", "fri 10:00", "--repeat", "daily", "--until", "2026-12-01", "--count", "3"}, 2},
		{"until without repeat", []string{"cal", "add", "x", "--start", "fri 10:00", "--until", "2026-12-01"}, 2},
		{"bad status", []string{"cal", "add", "x", "--start", "fri 10:00", "--status", "maybe"}, 2},
		{"bad reminder", []string{"cal", "add", "x", "--start", "fri 10:00", "--remind", "soon"}, 2},
		{"unknown person", []string{"cal", "add", "x", "--start", "fri 10:00", "--person", "9"}, 3},
		{"empty title", []string{"cal", "add", " ", "--start", "fri 10:00"}, 2},
		{"bad list window", []string{"cal", "list", "--from", "2026-10-20", "--to", "2026-10-19"}, 2},
		{"bad id", []string{"cal", "show", "abc"}, 2},
		{"unknown event", []string{"cal", "show", "5"}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := calCrm(t, db, append(tt.args, "-f", "json")...)
			assert.Equal(t, tt.code, code, stderr)
			assert.Empty(t, strings.TrimSpace(stdout))
			assert.Contains(t, stderr, `"error"`)
		})
	}
	assert.Empty(t, calJSON(t, db, "cal", "list", "--days", "365"), "failed adds create nothing")
}

func TestCalSearchAndTags(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "add", "Budget review", "--start", "fri 10:00", "--location", "Boardroom")
	var res []map[string]any
	jsonOut(t, calOK(t, db, "search", "boardroom", "-f", "json"), &res)
	require.Len(t, res, 1)
	assert.Equal(t, "event", res[0]["type"])
	assert.Equal(t, "Budget review", res[0]["name"])
	jsonOut(t, calOK(t, db, "search", "boardroom", "--type", "event", "-f", "json"), &res)
	assert.Len(t, res, 1)

	calOK(t, db, "tag", "apply", "event", "1", "work")
	assert.Len(t, calJSON(t, db, "cal", "list", "--tag", "work"), 1)
}

func TestCalSchemaAndHelp(t *testing.T) {
	db := newCalDB(t)
	var m struct {
		Commands []struct {
			Path        string   `json:"path"`
			Mutates     bool     `json:"mutates"`
			Destructive bool     `json:"destructive"`
			DryRun      bool     `json:"dry_run"`
			Examples    []string `json:"examples"`
		} `json:"commands"`
	}
	jsonOut(t, calOK(t, db, "schema", "-f", "json"), &m)
	by := map[string]int{}
	for i, c := range m.Commands {
		by[c.Path] = i
	}
	for _, p := range []string{"cal add", "cal list", "cal show", "cal edit", "cal rm", "cal restore", "cal purge", "cal attendee add", "cal attendee rm"} {
		require.Contains(t, by, p)
		c := m.Commands[by[p]]
		assert.NotEmpty(t, c.Examples, p)
		if c.Mutates {
			assert.True(t, c.DryRun, p+" has --dry-run")
		}
	}
	assert.True(t, m.Commands[by["cal purge"]].Destructive)
	assert.False(t, m.Commands[by["cal list"]].Mutates)
}
