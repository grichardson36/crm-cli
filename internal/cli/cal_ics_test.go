package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalExportImportRoundTripThroughFiles(t *testing.T) {
	src := newCalDB(t)
	calOK(t, src, "person", "add", "Jane Smith", "--email", "jane@example.com")
	calOK(t, src, "cal", "add", "Lunch", "--start", "fri 12:30", "--duration", "1h", "--person", "1", "--tag", "work", "--remind", "15m")
	calOK(t, src, "cal", "add", "Standup", "--start", "2026-10-19 09:15", "--duration", "15m", "--repeat", "weekdays", "--count", "10")
	calOK(t, src, "cal", "add", "Holiday", "--start", "2026-11-26")
	calOK(t, src, "task", "add", "Send proposal", "--due", "2026-11-01", "--priority", "high")

	// To standard output.
	out := calOK(t, src, "cal", "export", "--tasks")
	assert.True(t, strings.HasPrefix(out, "BEGIN:VCALENDAR\r\n"), "the file is the data on stdout")
	assert.Contains(t, out, "SUMMARY:Lunch")
	assert.Contains(t, out, "BEGIN:VTODO")

	// To a file, with a summary.
	file := filepath.Join(t.TempDir(), "crm.ics")
	summary := calJSON(t, src, "cal", "export", "--tasks", "--out", file)
	require.Len(t, summary, 1)
	assert.EqualValues(t, 3, summary[0]["events"])
	assert.EqualValues(t, 1, summary[0]["todos"])
	assert.Equal(t, file, summary[0]["file"])
	onDisk, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, out, string(onDisk))

	dst := newCalDB(t)
	calOK(t, dst, "person", "add", "Jane Smith", "--email", "jane@example.com")
	dry := calJSON(t, dst, "cal", "import", file, "--dry-run")
	assert.Equal(t, true, dry[0]["dry_run"])
	assert.EqualValues(t, 3, dry[0]["events"].(map[string]any)["created"])
	assert.Empty(t, calJSON(t, dst, "cal", "list", "--days", "60"), "a dry run writes nothing")

	res := calJSON(t, dst, "cal", "import", file)
	assert.EqualValues(t, 3, res[0]["events"].(map[string]any)["created"])
	assert.EqualValues(t, 1, res[0]["todos"].(map[string]any)["created"])

	again := calJSON(t, dst, "cal", "import", file)
	assert.EqualValues(t, 3, again[0]["events"].(map[string]any)["unchanged"])
	assert.EqualValues(t, 1, again[0]["todos"].(map[string]any)["unchanged"])

	list := calJSON(t, dst, "cal", "list", "--from", "2026-10-16", "--days", "5")
	titles := []string{}
	for _, l := range list {
		titles = append(titles, l["title"].(string))
	}
	assert.Equal(t, []string{"Lunch", "Standup", "Standup"}, titles)
	show := calJSON(t, dst, "cal", "show", "1")
	assert.Len(t, show[0]["attendees"], 1, "the attendee was matched by email")
	assert.Equal(t, []any{"work"}, show[0]["tags"])

	var tasks []map[string]any
	jsonOut(t, calOK(t, dst, "task", "list", "-f", "json"), &tasks)
	require.Len(t, tasks, 1)
	assert.Equal(t, "high", tasks[0]["priority"])
}

func TestCalImportFromStdinAndErrors(t *testing.T) {
	db := newCalDB(t)
	home := filepath.Join(filepath.Dir(db), "home")
	require.NoError(t, os.MkdirAll(home, 0o700))

	_, stderr, code := calCrm(t, db, "cal", "import", filepath.Join(t.TempDir(), "missing.ics"))
	assert.Equal(t, 2, code, stderr)

	bad := filepath.Join(t.TempDir(), "bad.ics")
	require.NoError(t, os.WriteFile(bad, []byte("not a calendar"), 0o600))
	_, stderr, code = calCrm(t, db, "cal", "import", bad, "-f", "json")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, `"validation"`)

	good := filepath.Join(t.TempDir(), "good.ics")
	require.NoError(t, os.WriteFile(good, []byte("BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:a@x\nSUMMARY:From a file\nDTSTART;TZID=Europe/London:20261016T093000\nATTENDEE:mailto:nobody@example.com\nX-ODD:1\nEND:VEVENT\nEND:VCALENDAR\n"), 0o600))
	stdout, stderr, code := calCrm(t, db, "cal", "import", good, "-f", "json", "--source", "feed")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "nobody@example.com")
	assert.Contains(t, stderr, "X-ODD")
	assert.Contains(t, stdout, `"unmatched_attendees"`)

	ev := calJSON(t, db, "cal", "show", "1")
	assert.Equal(t, "Europe/London", ev[0]["tz"])
	assert.Equal(t, "2026-10-16T08:30:00Z", ev[0]["start_utc"])
	assert.Equal(t, "feed", ev[0]["source"])

	table := calOK(t, db, "cal", "import", good, "--source", "feed", "-f", "table")
	assert.Contains(t, table, "events")
	assert.Contains(t, table, "UNCHANGED")
}

func TestCalExportRangeUsesTheWindow(t *testing.T) {
	db := newCalDB(t)
	calOK(t, db, "cal", "add", "October", "--start", "2026-10-20 10:00")
	calOK(t, db, "cal", "add", "December", "--start", "2026-12-20 10:00")
	out := calOK(t, db, "cal", "export", "--from", "2026-10-01", "--to", "2026-11-01")
	assert.Contains(t, out, "SUMMARY:October")
	assert.NotContains(t, out, "SUMMARY:December")
	_, _, code := calCrm(t, db, "cal", "export", "--from", "2026-11-01", "--to", "2026-10-01")
	assert.Equal(t, 2, code)
}
