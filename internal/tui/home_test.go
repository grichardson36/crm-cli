package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// homeModel adds tasks and notes to the standard data and opens Home.
// Today is 2026-10-13. Bob (2) is overdue for a reach out.
func homeModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	ctx := context.Background()
	add := func(title, due string, pid int64) {
		_, err := m.tr.Create(ctx, model.CreateTaskInput{Title: title, Priority: "medium", PersonID: &pid, DueAt: sp(due)})
		require.NoError(t, err)
	}
	add("send deck", "2026-10-10", 1)    // overdue
	add("call back", "2026-10-15", 3)    // this week
	add("plan retreat", "2026-12-01", 1) // later: not on Home
	for _, n := range []struct {
		pid    int64
		typ, c string
	}{{2, "note", "talked about the move"}, {3, "note", "likes tea"}} {
		_, err := m.ir.Create(ctx, model.CreateInteractionInput{Type: n.typ, Content: sp(n.c), PersonIDs: []int64{n.pid}})
		require.NoError(t, err)
	}
	m.switchView(viewHome)
	require.NoError(t, m.reload())
	return m
}

// moveTo presses j until the cursor row satisfies ok.
func moveTo(t *testing.T, m *Model, ok func(r row) bool) {
	t.Helper()
	for i := 0; i < len(m.rows)+1; i++ {
		if ok(m.rows[m.cursor]) {
			return
		}
		press(m, "down")
	}
	t.Fatalf("no row matched, rows: %v", rowTitles(m))
}

func rowTitles(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		out = append(out, r.title)
	}
	return out
}

func TestHomeIsTheDefaultView(t *testing.T) {
	d := newTestModelDB(t)
	m, err := New(context.Background(), d)
	require.NoError(t, err)
	assert.Equal(t, viewHome, m.view)
}

func TestHomeShowsOnlySectionsWithContent(t *testing.T) {
	m := homeModel(t)
	out := m.screen()
	assert.Contains(t, out, "1 Home")
	for _, h := range []string{"Due to reach out", "Tasks due today"} {
		assert.Contains(t, out, h)
	}
	assert.NotContains(t, out, "plan retreat", "tasks due later stay off Home")
	assert.NotContains(t, out, "call back", "tasks due later this week stay off Home")
	assert.NotContains(t, out, "Birthdays", "birthdays belong in their own view")
	assert.NotContains(t, out, "Recent notes", "recent context belongs on the person record")
	assert.NotContains(t, out, "Sync", "sync belongs in its own view")

	titles := rowTitles(m)
	assert.Contains(t, titles, "send deck")
}

func TestHomeEmptyDatabaseShowsNoDashboardCounts(t *testing.T) {
	d := newTestModelDB(t)
	m, err := New(context.Background(), d)
	require.NoError(t, err)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	assert.Empty(t, m.rows)
	assert.Contains(t, m.screen(), "Nothing needs attention today.")
}

func TestHomeCursorSkipsHeadingsAndEnterOpensThePerson(t *testing.T) {
	m := homeModel(t)
	r, ok := m.selected()
	require.True(t, ok)
	assert.False(t, r.skip, "the cursor starts on an item, not a heading")
	assert.Equal(t, int64(2), r.personID, "first item is the overdue reach out")

	press(m, "down")
	r, _ = m.selected()
	assert.False(t, r.skip)
	assert.Equal(t, "send deck", r.title)
	press(m, "up", "up")
	r, _ = m.selected()
	assert.Equal(t, "Bob Baker", r.title, "k stops at the first action")
	assert.False(t, r.skip)

	press(m, "end")
	r, _ = m.selected()
	assert.False(t, r.skip, "G lands on the last item")
	press(m, "home")
	assert.Equal(t, int64(2), m.rows[m.cursor].personID)

	// A task row opens the person it belongs to.
	moveTo(t, m, func(r row) bool { return r.title == "send deck" })
	press(m, "enter")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, int64(1), m.curPerson())
	assert.Equal(t, focusDetail, m.focus, "the detail pane is open")
	assert.Contains(t, m.screen(), "zed@acme.com")

	// Back on Home, the first item jumps to Bob.
	press(m, "esc", "1", "enter")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, int64(2), m.curPerson())
}

func TestHomeViewAllRowsJumpToTheirView(t *testing.T) {
	m := homeModel(t)
	ctx := context.Background()
	for i := 0; i < 7; i++ {
		p, err := m.pr.Create(ctx, model.CreatePersonInput{FirstName: "Due" + string(rune('A'+i))})
		require.NoError(t, err)
		_, err = m.pr.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: sp("7 days"), LastContactedAt: sp("2020-01-01")})
		require.NoError(t, err)
	}
	require.NoError(t, m.reload())
	var jump *row
	for i := range m.rows {
		if m.rows[i].jump == int(viewDue)+1 {
			jump = &m.rows[i]
		}
	}
	require.NotNil(t, jump, "only the top 5 show, the rest are one Enter away")
	assert.Contains(t, jump.title, "all 8 in Follow-up")
	due := 0
	for _, r := range m.rows {
		if r.personID != 0 && r.meta != "" && strings.HasSuffix(r.meta, "overdue") {
			due++
		}
	}
	assert.Equal(t, homeDueMax, due)
	moveTo(t, m, func(r row) bool { return r.jump == int(viewDue)+1 })
	press(m, "enter")
	assert.Equal(t, viewDue, m.view)
}

func TestHomeHidesSyncState(t *testing.T) {
	m := syncModel(t)
	m.switchView(viewHome)
	titles := rowTitles(m)
	assert.NotContains(t, titles, "Sync")
	assert.NotContains(t, titles, "fake")
	assert.NotContains(t, titles, "inbox: 2 pending")
}

func TestHomeActionsInPaletteAndFooter(t *testing.T) {
	m := homeModel(t)
	press(m, "ctrl+k")
	var found bool
	for _, it := range m.ov.items {
		if it.id == "home-open" {
			found = true
			assert.Equal(t, "", it.disabled)
		}
	}
	assert.True(t, found)
	press(m, "esc")
	assert.Contains(t, m.hintsLine(), "open")
	assert.Contains(t, m.hintsLine(), "contacted")

	// Contact keys work on Home items.
	press(m, "x")
	assert.Equal(t, "marked as contacted", m.status)
}

func TestHomeHidesBirthdays(t *testing.T) {
	m := homeModel(t)
	for i := 0; i < 6; i++ {
		p, err := m.pr.Create(context.Background(), model.CreatePersonInput{FirstName: "Bday" + string(rune('A'+i))})
		require.NoError(t, err)
		_, err = m.pr.Update(context.Background(), p.ID, model.UpdatePersonInput{Birthday: sp("--10-2" + string(rune('0'+i)))})
		require.NoError(t, err)
	}
	require.NoError(t, m.reload())
	assert.NotContains(t, rowTitles(m), "Birthdays next 14 days")
	for _, r := range m.rows {
		assert.False(t, strings.HasPrefix(r.title, "Bday"))
	}
}
