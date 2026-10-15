package ics

import (
	"strings"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chicago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return loc
}

func TestParseFoldingParamsAndEscapes(t *testing.T) {
	data := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:a@x\r\n" +
		"SUMMARY:Budget\\, Q4 review\\nsecond line\r\n" +
		"DESCRIPTION:A long line that\r\n  continues here\r\n" +
		"ATTENDEE;CN=\"Lee, Sam\";PARTSTAT=ACCEPTED:mailto:Sam@Example.com\r\n" +
		"DTSTART;TZID=America/Chicago:20261016T123000\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	roots, err := Parse([]byte(data))
	require.NoError(t, err)
	require.Len(t, roots, 1)
	ev := roots[0].Children[0]
	assert.Equal(t, "VEVENT", ev.Name)
	assert.Equal(t, "Budget, Q4 review\nsecond line", ev.Text("SUMMARY"))
	assert.Equal(t, "A long line that continues here", ev.Text("DESCRIPTION"))
	att, ok := ev.First("ATTENDEE")
	require.True(t, ok)
	assert.Equal(t, "Lee, Sam", att.Param("cn"))
	assert.Equal(t, "ACCEPTED", att.Param("PARTSTAT"))
	assert.Equal(t, "mailto:Sam@Example.com", att.Value)
	dt, _ := ev.First("DTSTART")
	assert.Equal(t, "America/Chicago", dt.Param("TZID"))
	assert.Equal(t, "20261016T123000", dt.Value)
}

func TestParseAcceptsBareLF(t *testing.T) {
	roots, err := Parse([]byte("BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:a\nEND:VEVENT\nEND:VCALENDAR\n"))
	require.NoError(t, err)
	assert.Len(t, roots[0].Children, 1)
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, data string
	}{
		{"empty", ""},
		{"not ical", "hello world"},
		{"unclosed", "BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:a\nEND:VCALENDAR\n"},
		{"mismatched end", "BEGIN:VCALENDAR\nEND:VEVENT\n"},
		{"stray end", "END:VCALENDAR\n"},
		{"property outside component", "UID:a\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.data))
			assert.Error(t, err)
		})
	}
}

func TestEscapeRoundTripAndSplit(t *testing.T) {
	for _, s := range []string{"plain", "a, b; c", `back\slash`, "two\nlines", "mix, \\ ; \n end", ""} {
		assert.Equal(t, s, Unescape(Escape(s)), s)
	}
	assert.Equal(t, []string{"work", "a,b", "c"}, SplitText(`work,a\,b,c`))
}

func TestFoldingKeepsLinesUnder75OctetsAndRunes(t *testing.T) {
	var w writer
	long := strings.Repeat("é", 100) + strings.Repeat("x", 100)
	w.text("DESCRIPTION", nil, long)
	for _, line := range strings.Split(strings.TrimSuffix(w.b.String(), "\r\n"), "\r\n") {
		assert.LessOrEqual(t, len(line), 75, line)
	}
	roots, err := Parse([]byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n" + w.b.String() + "END:VEVENT\r\nEND:VCALENDAR\r\n"))
	require.NoError(t, err)
	assert.Equal(t, long, roots[0].Children[0].Text("DESCRIPTION"), "folding is lossless, even inside multi-byte characters")
}

const sample = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example//EN
X-WR-CALNAME:Work
BEGIN:VTIMEZONE
TZID:Central Standard Time
END:VTIMEZONE
BEGIN:VEVENT
UID:timed@example.com
SUMMARY:Standup
DTSTART;TZID=America/Chicago:20260302T090000
DTEND;TZID=America/Chicago:20260302T091500
RRULE:FREQ=WEEKLY;BYDAY=MO;UNTIL=20261231T000000Z
EXDATE;TZID=America/Chicago:20260309T090000
STATUS:TENTATIVE
TRANSP:TRANSPARENT
LOCATION:Room 4
ORGANIZER;CN=Pat:mailto:pat@example.com
ATTENDEE;CN=Sam;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED:mailto:sam@example.com
CATEGORIES:work,daily
X-CUSTOM:value
BEGIN:VALARM
ACTION:EMAIL
TRIGGER:-PT15M
END:VALARM
BEGIN:VALARM
ACTION:DISPLAY
TRIGGER:-P1D
END:VALARM
BEGIN:VALARM
ACTION:DISPLAY
TRIGGER;VALUE=DATE-TIME:20260302T080000Z
END:VALARM
END:VEVENT
BEGIN:VEVENT
UID:windows@example.com
SUMMARY:Windows zone
DTSTART;TZID=Central Standard Time:20260716T100000
DURATION:PT1H30M
END:VEVENT
BEGIN:VEVENT
UID:utc@example.com
SUMMARY:UTC time
DTSTART:20261016T173000Z
DTEND:20261016T183000Z
END:VEVENT
BEGIN:VEVENT
UID:floating@example.com
SUMMARY:Floating
DTSTART:20261016T123000
END:VEVENT
BEGIN:VEVENT
UID:allday@example.com
SUMMARY:Holiday
DTSTART;VALUE=DATE:20261126
END:VEVENT
BEGIN:VEVENT
UID:timed@example.com
RECURRENCE-ID;TZID=America/Chicago:20260316T090000
SUMMARY:Standup (moved)
DTSTART;TZID=America/Chicago:20260316T100000
DTEND;TZID=America/Chicago:20260316T101500
END:VEVENT
BEGIN:VEVENT
SUMMARY:No uid
DTSTART:20261016T173000Z
END:VEVENT
BEGIN:VEVENT
UID:nostart@example.com
SUMMARY:No start
END:VEVENT
BEGIN:VTODO
UID:todo@example.com
SUMMARY:Send proposal
DUE;VALUE=DATE:20261101
PRIORITY:1
STATUS:COMPLETED
COMPLETED:20261020T120000Z
END:VTODO
BEGIN:VJOURNAL
UID:j@example.com
END:VJOURNAL
END:VCALENDAR
`

func TestDecode(t *testing.T) {
	loc := chicago(t)
	cal, err := Decode([]byte(sample), loc)
	require.NoError(t, err)
	assert.Equal(t, "Work", cal.Name)
	assert.Equal(t, map[string]int{"VJOURNAL": 1}, cal.Skipped)
	require.Len(t, cal.Events, 6, "events without a UID or a start are dropped")
	byUID := map[string]*Event{}
	for _, e := range cal.Events {
		if e.RecurrenceID == nil {
			byUID[e.UID] = e
		}
	}

	t.Run("timed series", func(t *testing.T) {
		e := byUID["timed@example.com"]
		assert.Equal(t, "2026-03-02T15:00:00Z", tz.FormatInstant(e.Start), "09:00 CST")
		assert.Equal(t, "2026-03-02T15:15:00Z", tz.FormatInstant(e.End))
		assert.Equal(t, "America/Chicago", e.TZ)
		assert.False(t, e.Floating)
		assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO;UNTIL=20261231T000000Z", e.RRule)
		require.Len(t, e.Exdates, 1)
		assert.Equal(t, "2026-03-09T14:00:00Z", tz.FormatInstant(e.Exdates[0]), "09:00 CDT")
		assert.Equal(t, "TENTATIVE", e.Status)
		assert.True(t, e.Transparent)
		assert.Equal(t, "Room 4", e.Location)
		assert.Equal(t, []string{"work", "daily"}, e.Categories)
		require.NotNil(t, e.Organizer)
		assert.Equal(t, "pat@example.com", e.Organizer.Email)
		require.Len(t, e.Attendees, 1)
		assert.Equal(t, "sam@example.com", e.Attendees[0].Email, "mailto is stripped and the address lower-cased")
		assert.Equal(t, "ACCEPTED", e.Attendees[0].PartStat)
		assert.Equal(t, []Alarm{{MinutesBefore: 15, Action: "EMAIL"}, {MinutesBefore: 1440, Action: "DISPLAY"}}, e.Alarms, "absolute triggers are skipped")
	})
	t.Run("windows zone name and duration", func(t *testing.T) {
		e := byUID["windows@example.com"]
		assert.Equal(t, "America/Chicago", e.TZ)
		assert.Equal(t, "2026-07-16T15:00:00Z", tz.FormatInstant(e.Start), "CDT in July")
		assert.Equal(t, 90*time.Minute, e.End.Sub(e.Start))
	})
	t.Run("utc", func(t *testing.T) {
		e := byUID["utc@example.com"]
		assert.Equal(t, "UTC", e.TZ)
		assert.Equal(t, "2026-10-16T17:30:00Z", tz.FormatInstant(e.Start))
		assert.False(t, e.Floating)
	})
	t.Run("floating times are flagged and read in the device zone", func(t *testing.T) {
		e := byUID["floating@example.com"]
		assert.True(t, e.Floating)
		assert.Equal(t, "America/Chicago", e.TZ)
		assert.Equal(t, "2026-10-16T17:30:00Z", tz.FormatInstant(e.Start))
		assert.True(t, e.End.Equal(e.Start), "no end means a zero length")
	})
	t.Run("all-day", func(t *testing.T) {
		e := byUID["allday@example.com"]
		assert.True(t, e.AllDay)
		assert.Equal(t, "2026-11-26", e.Start.Format(tz.DateLayout))
		assert.Equal(t, "2026-11-27", e.End.Format(tz.DateLayout))
		assert.Equal(t, "", e.TZ)
	})
	t.Run("override", func(t *testing.T) {
		var ov *Event
		for _, e := range cal.Events {
			if e.RecurrenceID != nil {
				ov = e
			}
		}
		require.NotNil(t, ov)
		assert.Equal(t, "timed@example.com", ov.UID)
		assert.Equal(t, "2026-03-16T14:00:00Z", tz.FormatInstant(*ov.RecurrenceID))
		assert.Equal(t, "Standup (moved)", ov.Summary)
	})
	t.Run("todo", func(t *testing.T) {
		require.Len(t, cal.Todos, 1)
		td := cal.Todos[0]
		assert.Equal(t, "Send proposal", td.Summary)
		assert.True(t, td.DueIsDate)
		assert.Equal(t, "2026-11-01", td.Due.Format(tz.DateLayout))
		assert.Equal(t, 1, td.Priority)
		assert.Equal(t, "COMPLETED", td.Status)
		require.NotNil(t, td.Completed)
	})
	t.Run("warnings name what was dropped", func(t *testing.T) {
		all := strings.Join(cal.Warnings, "\n")
		assert.Contains(t, all, "X-CUSTOM")
		assert.Contains(t, all, "no UID")
		assert.Contains(t, all, "no DTSTART")
		assert.Contains(t, all, "not relative to the start")
	})
}

func TestDecodeUnknownZoneFallsBackToFloating(t *testing.T) {
	data := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:a\nSUMMARY:x\nDTSTART;TZID=Mars Standard Time:20261016T123000\nEND:VEVENT\nEND:VCALENDAR\n"
	cal, err := Decode([]byte(data), chicago(t))
	require.NoError(t, err)
	require.Len(t, cal.Events, 1)
	assert.True(t, cal.Events[0].Floating)
	assert.Contains(t, strings.Join(cal.Warnings, " "), "Mars Standard Time")
}

func TestDecodeRequiresACalendar(t *testing.T) {
	_, err := Decode([]byte("BEGIN:VEVENT\nUID:a\nEND:VEVENT\n"), time.UTC)
	assert.Error(t, err)
}

func TestResolveZoneNames(t *testing.T) {
	for _, id := range []string{"America/Chicago", "Central Standard Time", "/mozilla.org/20050126_1/America/Chicago", `"America/Chicago"`} {
		_, name, ok := resolveZone(id)
		assert.True(t, ok, id)
		assert.Equal(t, "America/Chicago", name, id)
	}
	_, _, ok := resolveZone("Nowhere")
	assert.False(t, ok)
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"PT1H30M", 90 * time.Minute, true},
		{"P1D", 24 * time.Hour, true},
		{"P1W", 7 * 24 * time.Hour, true},
		{"-PT15M", -15 * time.Minute, true},
		{"PT0S", 0, true},
		{"P1DT2H", 26 * time.Hour, true},
		{"15 minutes", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := parseDuration(tt.in)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestEncodeThenDecodeKeepsEverythingItModels(t *testing.T) {
	loc := chicago(t)
	cal, err := Decode([]byte(sample), loc)
	require.NoError(t, err)
	data := Encode("Work", cal.Events, cal.Todos)

	text := string(data)
	assert.True(t, strings.HasPrefix(text, "BEGIN:VCALENDAR\r\n"))
	assert.Contains(t, text, "BEGIN:VTIMEZONE\r\nTZID:America/Chicago\r\n")
	assert.Contains(t, text, "TZOFFSETFROM:-0600\r\nTZOFFSETTO:-0500\r\nTZNAME:CDT")
	assert.Contains(t, text, "DTSTART;TZID=America/Chicago:20260302T090000\r\n")
	assert.Contains(t, text, "DTSTART;VALUE=DATE:20261126\r\n")
	assert.Contains(t, text, "RECURRENCE-ID;TZID=America/Chicago:20260316T090000\r\n")
	assert.Contains(t, text, "TRIGGER:-P1D\r\n")
	assert.Contains(t, text, "DTSTART:20261016T123000\r\n", "a floating time is written without a zone")
	assert.Contains(t, text, "DTSTART:20261016T173000Z\r\n")
	assert.NotContains(t, text, "Central Standard Time", "zones are written by IANA name")

	again, err := Decode(data, loc)
	require.NoError(t, err)
	require.Len(t, again.Events, len(cal.Events))
	for i, a := range cal.Events {
		b := again.Events[i]
		assert.Equal(t, a.UID, b.UID)
		assert.Equal(t, a.Summary, b.Summary)
		assert.True(t, a.Start.Equal(b.Start), a.UID)
		assert.True(t, a.End.Equal(b.End), a.UID)
		assert.Equal(t, a.AllDay, b.AllDay)
		assert.Equal(t, a.TZ, b.TZ, a.UID)
		assert.Equal(t, a.Floating, b.Floating, a.UID)
		assert.Equal(t, a.RRule, b.RRule)
		assert.Equal(t, a.Exdates, b.Exdates)
		assert.Equal(t, a.Status, b.Status)
		assert.Equal(t, a.Transparent, b.Transparent)
		assert.Equal(t, a.Location, b.Location)
		assert.Equal(t, a.Categories, b.Categories)
		assert.Equal(t, a.Alarms, b.Alarms)
		assert.Equal(t, a.RecurrenceID == nil, b.RecurrenceID == nil)
	}
	require.Len(t, again.Todos, 1)
	assert.Equal(t, cal.Todos[0].Summary, again.Todos[0].Summary)
	assert.Equal(t, cal.Todos[0].Due, again.Todos[0].Due)
	assert.Equal(t, cal.Todos[0].Completed, again.Todos[0].Completed)
	assert.Equal(t, cal.Todos[0].Priority, again.Todos[0].Priority)
}

func TestEncodeLinesAreShort(t *testing.T) {
	ev := &Event{UID: "u", Summary: strings.Repeat("title ", 40), Start: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), End: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), TZ: "UTC"}
	for _, line := range strings.Split(string(Encode("", []*Event{ev}, nil)), "\r\n") {
		assert.LessOrEqual(t, len(line), 75)
	}
}
