package tui

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/model"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestModelDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

// runCmds runs the commands a key returned and feeds finished sync runs back to the model.
// Spinner ticks are run (they only sleep) and dropped.
func runCmds(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			runCmds(m, c)
		}
	case syncDoneMsg:
		m.Update(msg)
	}
}

// pressRun presses keys like press and runs the commands they return.
func pressRun(m *Model, keys ...string) {
	for _, k := range keys {
		_, cmd := m.Update(keyPress(k))
		runCmds(m, cmd)
	}
}

func personRec(seq int, ext string, p crmsync.PersonPatch) crmsync.FakeRecord {
	return crmsync.FakeRecord{Seq: seq, Kind: "person", ExternalID: ext, Person: &p}
}

// syncModel installs the fake adapter. Its records: Zed (matches by email, updated), and two
// people nobody matches (Nina, Omar), which wait in the inbox.
func syncModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	fake := &crmsync.FakeAdapter{Records: []crmsync.FakeRecord{
		personRec(1, "g1", crmsync.PersonPatch{FirstName: sp("Zed"), Emails: []string{"zed@acme.com"}, Title: sp("CEO"), ETag: "v1"}),
		personRec(2, "g2", crmsync.PersonPatch{FirstName: sp("Nina"), LastName: sp("North"), Emails: []string{"nina@north.org"}, ETag: "v1"}),
		personRec(3, "g3", crmsync.PersonPatch{FirstName: sp("Omar"), LastName: sp("Ode"), Emails: []string{"omar@ode.org"}, ETag: "v1"}),
	}}
	m.registry = crmsync.NewRegistry(fake)
	m.runner = crmsync.NewRunner(m.db, m.registry)
	return m
}

// syncedModel is syncModel after one real run, standing on the Sync view.
func syncedModel(t *testing.T) *Model {
	t.Helper()
	m := syncModel(t)
	press(m, "0")
	runIDCmd(m, "sync-now")
	require.Empty(t, m.busy)
	require.Contains(t, m.status, "fake: ok")
	return m
}

func inboxRows(m *Model) []row {
	var out []row
	for _, r := range m.rows {
		if r.sync != nil && r.sync.entry != nil {
			out = append(out, r)
		}
	}
	return out
}

func TestSyncViewEmptyStateWithoutAdapters(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "0")
	assert.Equal(t, viewSync, m.view)
	assert.Empty(t, m.rows)
	assert.Contains(t, m.screen(), "No sync sources connected yet.")
	assert.Contains(t, m.screen(), "0 sources, 0 in inbox")
}

func TestSyncViewListsSourcesAndRunsInTheBackground(t *testing.T) {
	m := syncModel(t)
	press(m, "0")
	require.Equal(t, "fake", m.rows[m.cursor].title)
	assert.Equal(t, "never run", m.rows[m.cursor].sub)
	assert.Contains(t, m.screen(), "never run")
	assert.Contains(t, m.screen(), "cursor", "the detail pane shows the cursor")

	// Sync now (a palette entry) starts the run and returns at once: the model is busy and the spinner shows.
	cmd := runID(m, "sync-now")
	require.NotNil(t, cmd)
	assert.Equal(t, "syncing fake", m.busy)
	assert.Contains(t, m.screen(), "syncing fake...")

	// A second run is refused while one is in flight.
	runID(m, "sync-now")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "already running")

	runCmds(m, cmd)
	assert.Empty(t, m.busy)
	assert.False(t, m.statusIsErr, m.status)
	assert.Equal(t, "fake: ok, 0 created, 1 updated, 0 skipped, 2 in inbox, 0 errors", m.status)

	// Source row: counts and time. Two inbox rows appear.
	var src row
	for _, r := range m.rows {
		if r.sync != nil && r.sync.source != nil {
			src = r
		}
	}
	assert.True(t, len(src.sub) > 3 && src.sub[:3] == "ok ", src.sub)
	assert.Equal(t, "+0 ~1 =2 !0", src.meta)
	require.Len(t, inboxRows(m), 2)
	assert.Equal(t, "Nina North", inboxRows(m)[0].title)
	assert.Equal(t, "person", inboxRows(m)[0].meta)
	assert.Contains(t, m.screen(), "1 sources, 2 in inbox")

	// Zed was updated through the match, the others waited.
	zed, err := m.pr.FindByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "CEO", str(zed.Title))
	assert.Len(t, m.people, 3)
}

func TestSyncDryRunWritesNothing(t *testing.T) {
	m := syncModel(t)
	press(m, "0")
	runIDCmd(m, "sync-dry")
	assert.Equal(t, "dry run fake: ok, 0 created, 1 updated, 0 skipped, 2 in inbox, 0 errors (nothing written)", m.status)
	assert.Empty(t, inboxRows(m))
	run, err := m.runner.Overview(context.Background())
	require.NoError(t, err)
	assert.Nil(t, run.Sources[0].LastRun, "no run is logged")
	assert.Equal(t, 0, run.InboxPending)
	zed, _ := m.pr.FindByID(context.Background(), 1)
	assert.Equal(t, "", str(zed.Title))
}

func TestSyncFailureShowsTheError(t *testing.T) {
	m := syncModel(t)
	fake, err := m.registry.Get("fake")
	require.NoError(t, err)
	fake.(*crmsync.FakeAdapter).FailAfter = 1
	press(m, "0")
	runIDCmd(m, "sync-now")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "simulated failure")
}

func TestInboxAcceptCreatesAPerson(t *testing.T) {
	m := syncedModel(t)
	press(m, "down") // from the source to the first inbox record (headings are skipped)
	e := m.selectedEntry()
	require.NotNil(t, e)
	assert.Equal(t, "Nina North", e.Summary)
	assert.Contains(t, m.screen(), "nina@north.org", "the detail pane shows the record")

	press(m, "enter") // enter on an inbox record accepts it
	assert.Contains(t, m.status, "accepted as person #4")
	assert.Len(t, m.people, 4)
	assert.Len(t, inboxRows(m), 1)
	got, err := m.inbox.Get(context.Background(), e.ID)
	require.NoError(t, err)
	assert.Equal(t, model.InboxAccepted, got.Status)
	assert.Equal(t, "Omar Ode", m.selectedEntry().Summary, "the cursor stays on the next record")
}

func TestInboxAttachSearchesForThePerson(t *testing.T) {
	m := syncedModel(t)
	press(m, "down")
	e := *m.selectedEntry()
	runID(m, "inbox-attach")
	require.Equal(t, modeOverlay, m.mode)
	assert.Contains(t, m.ov.title, "Attach inbox")
	assert.Len(t, m.ov.shown, 3, "everyone can be searched")
	typeText(m, "cara")
	require.Len(t, m.ov.shown, 1)
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "attached to Cara Cole")
	got, err := m.inbox.Get(context.Background(), e.ID)
	require.NoError(t, err)
	assert.Equal(t, model.InboxAttached, got.Status)
	assert.Len(t, inboxRows(m), 1)
	assert.Len(t, m.people, 3, "attach creates nobody")
	cara := personByID(t, m.pr, 3)
	var emails []string
	for _, em := range cara.Emails {
		emails = append(emails, em.Email)
	}
	assert.Contains(t, emails, "nina@north.org")
}

func TestInboxIgnoreDismissesTheRecord(t *testing.T) {
	m := syncedModel(t)
	press(m, "down", "d") // d is the Delete verb
	assert.Contains(t, m.status, "ignored")
	assert.Len(t, inboxRows(m), 1)
	pending, err := m.inbox.List(context.Background(), model.InboxPending, 0)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "Omar Ode", pending[0].Summary)
}

func TestInboxKeysOnlyApplyToInboxRows(t *testing.T) {
	m := syncedModel(t)
	require.Equal(t, "fake", m.rows[m.cursor].title)
	press(m, "enter")
	assert.Len(t, inboxRows(m), 2, "enter does nothing on a source row")
	runID(m, "inbox-attach")
	assert.Equal(t, modeNormal, m.mode)
	press(m, "d")
	assert.Len(t, inboxRows(m), 2)
}

func TestReadOnlyBlocksSyncWritesButNotDryRun(t *testing.T) {
	m := syncedModel(t)
	m.readOnly = true
	runID(m, "sync-now")
	assert.Contains(t, m.status, "read-only")
	assert.Empty(t, m.busy)
	press(m, "down")
	for _, k := range []string{"enter", "d"} {
		press(m, k)
		assert.Contains(t, m.status, "read-only", k)
		assert.Equal(t, modeNormal, m.mode)
	}
	runID(m, "inbox-attach")
	assert.Contains(t, m.status, "read-only")
	assert.Len(t, inboxRows(m), 2)
	press(m, "up")
	runIDCmd(m, "sync-dry")
	assert.Contains(t, m.status, "dry run", "a dry run is read-only safe")
}

func TestSyncPaletteEntries(t *testing.T) {
	m := syncedModel(t)
	items := func() map[string]overlayItem {
		press(m, "ctrl+k")
		out := map[string]overlayItem{}
		for _, it := range m.ov.items {
			out[it.id] = it
		}
		press(m, "esc")
		return out
	}
	it := items()
	assert.Equal(t, "Sync now: fake", it["sync-run:fake"].label)
	assert.Equal(t, "Dry-run sync: fake", it["sync-dry:fake"].label)
	assert.Equal(t, "", it["sync-run:fake"].disabled)
	assert.Equal(t, "", it["open-inbox"].disabled)
	assert.Equal(t, "", it["sync-now"].disabled)
	assert.Equal(t, "not here", it["inbox-accept"].disabled, "a source row is selected")
	for _, id := range []string{"inbox-accept", "inbox-attach", "inbox-ignore", "sync-now", "sync-dry", "open-inbox", "goto-view", "view-home", "view-sync"} {
		_, ok := it[id]
		assert.True(t, ok, "palette lists %s", id)
	}

	// Run a source from the palette.
	press(m, "1")
	pressRun(m, "ctrl+k")
	typeText(m, "dry-run sync: fake")
	pressRun(m, "enter")
	assert.Contains(t, m.status, "dry run fake")

	// Open inbox puts the cursor on the first record.
	press(m, "2")
	press(m, "ctrl+k")
	typeText(m, "open inbox")
	press(m, "enter")
	assert.Equal(t, viewSync, m.view)
	require.NotNil(t, m.selectedEntry())
	it = items()
	assert.Equal(t, "", it["inbox-accept"].disabled)
	assert.Equal(t, "not here", it["sync-now"].disabled)

	m.readOnly = true
	it = items()
	assert.Equal(t, "read-only", it["sync-run:fake"].disabled)
	assert.Equal(t, "", it["sync-dry:fake"].disabled)
	assert.Equal(t, "read-only", it["inbox-accept"].disabled)

	m.readOnly = false
	press(m, "ctrl+k")
	typeText(m, "sync now: fake")
	pressRun(m, "enter")
	assert.Contains(t, m.status, "fake: ok")
}

func TestOpenInboxWithNothingPending(t *testing.T) {
	m := syncModel(t)
	m.openInbox()
	assert.Equal(t, viewSync, m.view)
	assert.Equal(t, "the inbox is empty", m.status)
}

func TestSyncViewFitsTerminals(t *testing.T) {
	m := syncedModel(t)
	for _, size := range [][2]int{{80, 24}, {60, 12}, {40, 10}, {120, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(m, "0")
		for _, k := range []string{"down", "ctrl+down"} {
			press(m, k)
			lines := strings.Split(m.screen(), "\n")
			assert.LessOrEqual(t, len(lines), size[1], "%v height", size)
			for _, l := range lines {
				assert.LessOrEqual(t, lipgloss.Width(l), size[0], "%v: %q", size, l)
			}
		}
		press(m, "esc", "1")
		for _, l := range strings.Split(m.screen(), "\n") {
			assert.LessOrEqual(t, lipgloss.Width(l), size[0], "home %v: %q", size, l)
		}
	}
}
