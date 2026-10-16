package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doDB makes a database with Sam Lee (1), Sam Ng (2) and Bob Baker (3).
func doDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crm.db")
	for _, name := range []string{"Sam Lee", "Sam Ng", "Bob Baker"} {
		_, stderr, code := crm(t, path, "person", "add", name, "-q")
		require.Equal(t, 0, code, stderr)
	}
	return path
}

func decodeObject(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m), s)
	return m
}

func decodeList(t *testing.T, s string) []map[string]any {
	t.Helper()
	var m []map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m), s)
	return m
}

func TestDoNoteDryRunWritesNothing(t *testing.T) {
	path := doDB(t)
	stdout, _, code := crm(t, path, "do", "n Bob called about the deal #deal remind 2026-12-25", "--dry-run", "-f", "json")
	require.Equal(t, 0, code)
	out := decodeObject(t, stdout)
	assert.Equal(t, true, out["dry_run"])
	assert.Equal(t, false, out["applied"])
	assert.Equal(t, "note", out["verb"])
	assert.Equal(t, "Bob Baker", out["subject"].(map[string]any)["name"])
	assert.Len(t, out["changes"], 3)
	assert.Contains(t, out["preview"], "remind Fri 25 Dec")

	stdout, _, _ = crm(t, path, "log", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 0)
	stdout, _, _ = crm(t, path, "task", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 0)
	stdout, _, _ = crm(t, path, "tag", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 0)
}

func TestDoNoteApplies(t *testing.T) {
	path := doDB(t)
	stdout, stderr, code := crm(t, path, "do", "n Bob called about the deal #deal remind 2026-12-25", "-f", "json")
	require.Equal(t, 0, code, stderr)
	out := decodeObject(t, stdout)
	assert.Equal(t, true, out["applied"])

	stdout, _, _ = crm(t, path, "log", "list", "--person", "3", "-f", "json")
	logs := decodeList(t, stdout)
	require.Len(t, logs, 1)
	assert.Equal(t, "called about the deal", logs[0]["content"])
	stdout, _, _ = crm(t, path, "task", "list", "-f", "json")
	tasks := decodeList(t, stdout)
	require.Len(t, tasks, 1)
	assert.Equal(t, "2026-12-25", tasks[0]["due_at"])
	stdout, _, _ = crm(t, path, "tag", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 1)
}

func TestDoWithPersonFlagNeedsNoName(t *testing.T) {
	path := doDB(t)
	_, stderr, code := crm(t, path, "do", "t send the contract !high", "--person", "1")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ := crm(t, path, "task", "list", "--person", "1", "-f", "json")
	tasks := decodeList(t, stdout)
	require.Len(t, tasks, 1)
	assert.Equal(t, "send the contract", tasks[0]["title"])
	assert.Equal(t, "high", tasks[0]["priority"])

	_, _, code = crm(t, path, "do", "t x", "--person", "99")
	assert.Equal(t, 3, code)
}

func TestDoSetMultiFieldAndTable(t *testing.T) {
	path := doDB(t)
	stdout, stderr, code := crm(t, path, "do", "set Bob: company=Acme title=CTO birthday=Mar 4", "-f", "table")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "Bob Baker company: (empty) -> Acme")
	assert.Contains(t, stdout, "birthday: (empty) -> --03-04")
	stdout, _, _ = crm(t, path, "person", "show", "3", "-f", "json")
	shown := decodeList(t, stdout)[0]
	assert.Equal(t, "Acme", shown["company"])
	assert.Equal(t, "CTO", shown["title"])
}

func TestDoAmbiguousNameExits2WithCandidates(t *testing.T) {
	path := doDB(t)
	stdout, stderr, code := crm(t, path, "do", "n Sam called", "-f", "json")
	assert.Equal(t, 2, code)
	out := decodeObject(t, stdout)
	problems := out["problems"].([]any)
	require.Len(t, problems, 1)
	assert.Equal(t, "ambiguous", problems[0].(map[string]any)["code"])
	assert.Len(t, problems[0].(map[string]any)["candidates"], 2)
	assert.Contains(t, stderr, "Sam Lee #1")
	stdout, _, _ = crm(t, path, "log", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 0)
}

func TestDoUnknownNameExits3(t *testing.T) {
	path := doDB(t)
	_, _, code := crm(t, path, "do", "set Nobody: title=CTO")
	assert.Equal(t, 3, code)
}

func TestDoNeedsAVerb(t *testing.T) {
	path := doDB(t)
	_, stderr, code := crm(t, path, "do", "hello there")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "must start with a verb")
}

func TestDoNothingToDoExits2(t *testing.T) {
	path := doDB(t)
	_, stderr, code := crm(t, path, "do", "n ", "--person", "1")
	assert.Equal(t, 2, code, stderr)
}

func TestDoNoSyntaxKeepsTokens(t *testing.T) {
	path := doDB(t)
	_, stderr, code := crm(t, path, "do", "n Bob called #deal", "--no-syntax")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ := crm(t, path, "log", "list", "-f", "json")
	logs := decodeList(t, stdout)
	require.Len(t, logs, 1)
	assert.Equal(t, "called #deal", logs[0]["content"])
	stdout, _, _ = crm(t, path, "tag", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 0)
}

func TestDoTagAndSnooze(t *testing.T) {
	path := doDB(t)
	_, stderr, code := crm(t, path, "do", "tag Bob vip client")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ := crm(t, path, "tag", "show", "person", "3", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 2)
	_, stderr, code = crm(t, path, "do", "snooze Bob whenever")
	assert.Equal(t, 2, code, "not a number of days or a date: %s", stderr)
	_, stderr, code = crm(t, path, "do", "snooze Bob 2026-12-24")
	require.Equal(t, 0, code, stderr)
}

func TestTaskAddInlineSyntax(t *testing.T) {
	path := doDB(t)
	stdout, stderr, code := crm(t, path, "task", "add", "Call back 2026-12-24 3pm ~30m !high @Bob", "--dry-run")
	require.Equal(t, 0, code, stderr)
	out := decodeObject(t, stdout)
	task := out["task"].(map[string]any)
	assert.Equal(t, "Call back", task["title"])
	assert.Equal(t, "2026-12-24 15:00", task["due_at"])
	assert.Equal(t, "high", task["priority"])
	assert.Equal(t, []any{float64(3)}, task["person_ids"])
	assert.Equal(t, float64(30), task["duration_min"])
	assert.Equal(t, "Call back", out["syntax"].(map[string]any)["title"])

	stdout, _, _ = crm(t, path, "task", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 0, "dry run writes nothing")

	// For real.
	stdout, stderr, code = crm(t, path, "task", "add", "Call back 2026-12-24 3pm !high @Bob by 2026-12-30", "-f", "json")
	require.Equal(t, 0, code, stderr)
	created := decodeList(t, stdout)[0]
	assert.Equal(t, "Call back", created["title"])
	assert.Equal(t, "2026-12-24 15:00", created["due_at"])
	assert.Equal(t, float64(3), created["person_id"])
	assert.Equal(t, "2026-12-30", created["deadline_at"])
}

func TestTaskAddFlagsWinOverInlineSyntax(t *testing.T) {
	path := doDB(t)
	stdout, stderr, code := crm(t, path, "task", "add", "Call back 2026-12-24 !high @Bob", "--due", "2027-01-05", "--priority", "low", "--person", "1", "-f", "json")
	require.Equal(t, 0, code, stderr)
	created := decodeList(t, stdout)[0]
	assert.Equal(t, "2027-01-05", created["due_at"])
	assert.Equal(t, "low", created["priority"])
	assert.Equal(t, float64(1), created["person_id"])
	assert.Equal(t, "Call back", created["title"], "tokens are still removed from the title")
}

func TestTaskAddNoSyntaxAndWarnings(t *testing.T) {
	path := doDB(t)
	stdout, _, code := crm(t, path, "task", "add", "Call back fri #work @Bob", "--no-syntax", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "Call back fri #work @Bob", decodeList(t, stdout)[0]["title"])

	stdout, stderr, code := crm(t, path, "task", "add", "Fix sink #home #house", "-f", "json")
	require.Equal(t, 0, code, stderr)
	created := decodeList(t, stdout)[0]
	assert.Equal(t, "Fix sink", created["title"])
	assert.Equal(t, []any{"home", "house"}, created["tags"])
	assert.Empty(t, stderr)

	_, _, code = crm(t, path, "task", "add", "Ping @Sam", "-f", "json")
	assert.Equal(t, 2, code, "ambiguous @Sam")
	_, _, code = crm(t, path, "task", "add", "Ping @Nobody", "-f", "json")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, path, "task", "add", "fri", "-f", "json")
	assert.Equal(t, 2, code, "empty title")
}

func TestTaskAddTomorrowFollowsTheClock(t *testing.T) {
	path := doDB(t)
	want := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	stdout, _, code := crm(t, path, "task", "add", "Water plants tomorrow", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, want, decodeList(t, stdout)[0]["due_at"])
}

func TestLogAddInlineSyntax(t *testing.T) {
	path := doDB(t)
	stdout, stderr, code := crm(t, path, "log", "add", "--type", "call", "3", "Talked about the offsite #trip 2026-10-01 14:30 @Sam_Lee every 3m", "--dry-run")
	require.Equal(t, 0, code, stderr)
	out := decodeObject(t, stdout)
	inter := out["interaction"].(map[string]any)
	assert.Equal(t, "Talked about the offsite", inter["content"])
	assert.Equal(t, "2026-10-01 14:30:00", inter["occurred_at"])
	assert.Equal(t, []any{float64(3), float64(1)}, inter["person_ids"])
	assert.Equal(t, []any{"trip"}, inter["tags"])
	assert.Equal(t, "3 mons", inter["person_cadence"])

	stdout, stderr, code = crm(t, path, "log", "add", "--type", "call", "3", "Talked about the offsite #trip 2026-10-01 14:30 @Sam_Lee every 3m", "-f", "json")
	require.Equal(t, 0, code, stderr)
	logged := decodeList(t, stdout)[0]
	assert.Equal(t, "2026-10-01 14:30:00", logged["occurred_at"])
	assert.ElementsMatch(t, []any{float64(3), float64(1)}, logged["person_ids"])
	stdout, _, _ = crm(t, path, "tag", "show", "interaction", "1", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 1)
	stdout, _, _ = crm(t, path, "person", "show", "3", "-f", "json")
	shown := decodeList(t, stdout)[0]
	assert.Equal(t, "3 mons", shown["keep_in_touch"])

	// --at wins over @date, and --content is never parsed.
	stdout, _, code = crm(t, path, "log", "add", "--type", "note", "3", "hello 2026-10-02", "--at", "2026-11-11 10:00:00", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "2026-11-11 10:00:00", decodeList(t, stdout)[0]["occurred_at"])
	assert.Equal(t, "hello", decodeList(t, stdout)[0]["content"])
	stdout, _, code = crm(t, path, "log", "add", "--type", "note", "3", "--content", "keep #this fri", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "keep #this fri", decodeList(t, stdout)[0]["content"])
}

func TestPersonAddInlineSyntax(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crm.db")
	stdout, stderr, code := crm(t, path, "person", "add", "Dana Dunn #friend #golf every 6m !top", "--dry-run")
	require.Equal(t, 0, code, stderr)
	p := decodeObject(t, stdout)["person"].(map[string]any)
	assert.Equal(t, "Dana Dunn", p["name"])
	assert.Equal(t, "6 mons", p["keep_in_touch"])
	assert.Equal(t, float64(1), p["priority_tier"])
	assert.Equal(t, []any{"friend", "golf"}, p["tags"])
	stdout, _, _ = crm(t, path, "person", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 0, "dry run writes nothing")

	stdout, stderr, code = crm(t, path, "person", "add", "Dana Dunn #friend #golf every 6m !top", "-f", "json")
	require.Equal(t, 0, code, stderr)
	created := decodeList(t, stdout)[0]
	assert.Equal(t, "Dana Dunn", created["name"])
	assert.Equal(t, "6 mons", created["keep_in_touch"])
	assert.Equal(t, float64(1), created["priority_tier"])
	stdout, _, _ = crm(t, path, "tag", "show", "person", "1", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 2)

	// A flag wins over the token.
	stdout, _, code = crm(t, path, "person", "add", "Eve Ng every 6m", "--keep-in-touch", "1 year", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "1 year", decodeList(t, stdout)[0]["keep_in_touch"])

	// --no-syntax keeps the name as typed.
	stdout, _, code = crm(t, path, "person", "add", "Frank #1 Fan", "--no-syntax", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "Frank #1 Fan", decodeList(t, stdout)[0]["name"])
	_, _, code = crm(t, path, "person", "add", "#tag", "-f", "json")
	assert.Equal(t, 2, code, "empty name")
}

func TestPlainNamesAndTitlesAreUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crm.db")
	stdout, _, code := crm(t, path, "person", "add", "Jane O'Neil", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "Jane O'Neil", decodeList(t, stdout)[0]["name"])
	stdout, _, code = crm(t, path, "task", "add", "Email jane@example.com about +1 555 0100", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "Email jane@example.com about +1 555 0100", decodeList(t, stdout)[0]["title"])
}

func TestTaskTagUntagAndListByTag(t *testing.T) {
	path := doDB(t)
	crm(t, path, "task", "add", "One #work", "-q")
	crm(t, path, "task", "add", "Two", "--tag", "home", "--tag", "work", "-q")
	crm(t, path, "task", "add", "Three", "-q")

	stdout, stderr, code := crm(t, path, "task", "tag", "3", "#urgent", "work", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []any{"urgent", "work"}, decodeList(t, stdout)[0]["tags"])

	stdout, _, code = crm(t, path, "task", "list", "--tag", "work", "-f", "json")
	require.Equal(t, 0, code)
	assert.Len(t, decodeList(t, stdout), 3)
	stdout, _, _ = crm(t, path, "task", "list", "--tag", "home", "-f", "json")
	list := decodeList(t, stdout)
	require.Len(t, list, 1)
	assert.Equal(t, "Two", list[0]["title"])
	assert.Equal(t, []any{"home", "work"}, list[0]["tags"])

	stdout, _, code = crm(t, path, "task", "untag", "3", "work", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, []any{"urgent"}, decodeList(t, stdout)[0]["tags"])
	_, _, code = crm(t, path, "task", "untag", "3", "work", "-f", "json")
	assert.Equal(t, 3, code, "the tag is not on the task")
	_, _, code = crm(t, path, "task", "tag", "99", "x")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, path, "task", "tag", "x", "x")
	assert.Equal(t, 2, code)

	// The generic tag commands accept tasks too.
	_, stderr, code = crm(t, path, "tag", "apply", "task", "1", "generic")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ = crm(t, path, "tag", "show", "task", "1", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 2)
	_, _, code = crm(t, path, "tag", "apply", "bogus", "1", "x")
	assert.Equal(t, 2, code)

	stdout, _, _ = crm(t, path, "task", "tag", "1", "dry", "--dry-run", "-f", "json")
	assert.Contains(t, stdout, `"dry_run":true`)
	stdout, _, _ = crm(t, path, "task", "show", "1", "-f", "json")
	assert.NotContains(t, stdout, "dry")
}
func TestDoUnsettledNamesCarryCandidatesInJSON(t *testing.T) {
	path := doDB(t)
	stdout, _, code := crm(t, path, "do", "n coffee with @Sam", "--person", "3", "-f", "json")
	assert.Equal(t, 2, code)
	out := decodeObject(t, stdout)
	names := out["names"].([]any)
	require.Len(t, names, 1)
	ref := names[0].(map[string]any)
	assert.Equal(t, "@Sam", ref["token"])
	assert.Equal(t, "link", ref["kind"])
	assert.Len(t, ref["candidates"], 2)
	assert.Equal(t, float64(-1), ref["default"])

	_, _, code = crm(t, path, "do", "n coffee with @Nobody_Here", "--person", "3")
	assert.Equal(t, 3, code, "a capitalized two-word name with no match is not found")
	stdout, _, _ = crm(t, path, "log", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 0, "nothing was written")

	// A single @word that is nobody's name is plain text: no error.
	_, _, code = crm(t, path, "do", "n coffee with @Nobody", "--person", "3")
	assert.Equal(t, 0, code)
	stdout, _, _ = crm(t, path, "log", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 1)
}

func TestDoKeepUnresolvedAndLinkNew(t *testing.T) {
	path := doDB(t)
	// Keep as text.
	stdout, stderr, code := crm(t, path, "do", "n coffee with @Sam and @Nobody", "--person", "3", "--keep-unresolved", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, true, decodeObject(t, stdout)["applied"])
	stdout, _, _ = crm(t, path, "log", "list", "-f", "json")
	logs := decodeList(t, stdout)
	require.Len(t, logs, 1)
	assert.Equal(t, "coffee with @Sam and @Nobody", logs[0]["content"])

	// A dry run with --link-new creates nobody and says so.
	stdout, _, code = crm(t, path, "do", "n met @Priya_Nair", "--person", "3", "--link-new", "--dry-run", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "would create a contact for Priya Nair")
	stdout, _, _ = crm(t, path, "person", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 3)

	// For real: the contact is created and linked. Several matches are never guessed.
	stdout, stderr, code = crm(t, path, "do", "n met @Priya_Nair and @Sam", "--person", "3", "--link-new", "-f", "json")
	assert.Equal(t, 3, code, "the first unsettled name decides the error: %s", stderr)
	stdout, _, _ = crm(t, path, "person", "list", "-f", "json")
	assert.Len(t, decodeList(t, stdout), 3, "a line that still fails creates nobody")
	stdout, stderr, code = crm(t, path, "do", "n met @Priya_Nair", "--person", "3", "--link-new", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "created a contact for Priya Nair")
	stdout, _, _ = crm(t, path, "person", "list", "-f", "json")
	people := decodeList(t, stdout)
	require.Len(t, people, 4)
	var priya float64
	for _, p := range people {
		if p["name"] == "Priya Nair" {
			priya = p["id"].(float64)
		}
	}
	require.NotZero(t, priya)
	stdout, _, _ = crm(t, path, "log", "list", "--person", strconv.Itoa(int(priya)), "-f", "json")
	assert.Len(t, decodeList(t, stdout), 1)
}
// calRun runs crm with a pinned clock: Tuesday 2026-10-13 12:00 UTC, zone UTC.
func calRun(t *testing.T, path string, args ...string) (string, string, int) {
	t.Helper()
	home := filepath.Dir(path)
	return crmEnv(t, home, []string{"CRM_NOW=2026-10-13T12:00:00Z", "TZ=UTC"}, append([]string{"--db", path}, args...)...)
}

func TestCalAddReadsShortSyntaxFromTheTitle(t *testing.T) {
	path := doDB(t)
	stdout, stderr, code := calRun(t, path, "cal", "add", "Lunch with @Bob fri 12:30 for 1h at Cafe Rio every week #food", "--dry-run", "-f", "json")
	require.Equal(t, 0, code, stderr)
	out := decodeObject(t, stdout)
	ev := out["event"].(map[string]any)
	assert.Equal(t, "Lunch with", ev["title"])
	assert.Equal(t, "Cafe Rio", ev["location"])
	assert.Equal(t, "2026-10-16T12:30:00Z", ev["start"])
	assert.Equal(t, "2026-10-16T13:30:00Z", ev["end"])
	assert.Contains(t, ev["rrule"], "FREQ=WEEKLY")

	stdout, stderr, code = calRun(t, path, "cal", "add", "Lunch with @Bob fri 12:30 for 1h at Cafe Rio every week #food", "-f", "json")
	require.Equal(t, 0, code, stderr)
	created := decodeList(t, stdout)[0]
	assert.Equal(t, "Lunch with", created["title"])
	assert.Equal(t, "Cafe Rio", created["location"])
	assert.Equal(t, []any{"food"}, created["tags"])
	atts := created["attendees"].([]any)
	require.Len(t, atts, 1)
	assert.Equal(t, float64(3), atts[0].(map[string]any)["person_id"])
}

func TestCalAddFlagsWinAndStartIsStillRequired(t *testing.T) {
	path := doDB(t)
	stdout, stderr, code := calRun(t, path, "cal", "add", "Call fri 9am at Home", "--start", "2026-10-20 14:00", "--location", "Office", "--dry-run", "-f", "json")
	require.Equal(t, 0, code, stderr)
	ev := decodeObject(t, stdout)["event"].(map[string]any)
	assert.Equal(t, "2026-10-20T14:00:00Z", ev["start"])
	assert.Equal(t, "Office", ev["location"])
	assert.Equal(t, "Call", ev["title"], "tokens are still removed from the title")

	_, stderr, code = calRun(t, path, "cal", "add", "Call Sam", "-f", "json")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "write it in the title")

	// --no-syntax keeps the title as typed and needs --start.
	stdout, _, code = calRun(t, path, "cal", "add", "Lunch fri #x", "--no-syntax", "--start", "2026-10-20 14:00", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "Lunch fri #x", decodeList(t, stdout)[0]["title"])
	_, _, code = calRun(t, path, "cal", "add", "Lunch with @Nobody fri", "-f", "json")
	assert.Equal(t, 3, code)
}