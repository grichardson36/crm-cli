package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// homeModel adds tasks and notes to the standard data and opens Home.
// Today is 2026-10-13. Bob (2) is overdue for a reach out and has a birthday on 10-20.
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
		press(m, "j")
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
	out := m.View()
	assert.Contains(t, out, "1 Home")
	assert.Contains(t, out, "3 people, 3 open tasks, 1 overdue")
	for _, h := range []string{"Due to reach out", "Birthdays next 14 days", "Tasks overdue / this week", "Recent notes"} {
		assert.Contains(t, out, h)
	}
	assert.NotContains(t, out, "plan retreat", "tasks due later stay off Home")
	assert.NotContains(t, rowTitles(m), "Sync", "no sources and an empty inbox hide the sync section")

	titles := rowTitles(m)
	assert.Contains(t, titles, "send deck")
	assert.Contains(t, titles, "call back")
	var recent []string
	in := false
	for _, r := range m.rows {
		if r.skip {
			in = r.title == "Recent notes"
			continue
		}
		if in {
			recent = append(recent, r.title+"|"+r.sub)
		}
	}
	assert.ElementsMatch(t, []string{"Bob Baker|note: talked about the move", "Cara Cole|note: likes tea"}, recent)
}

func TestHomeEmptyDatabaseShowsOnlyTheStatusLine(t *testing.T) {
	d := newTestModelDB(t)
	m, err := New(context.Background(), d)
	require.NoError(t, err)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	require.Len(t, m.rows, 1)
	assert.Equal(t, "0 people, 0 open tasks, 0 overdue", m.rows[0].title)
	assert.True(t, m.rows[0].skip)
}

func TestHomeCursorSkipsHeadingsAndEnterOpensThePerson(t *testing.T) {
	m := homeModel(t)
	r, ok := m.selected()
	require.True(t, ok)
	assert.False(t, r.skip, "the cursor starts on an item, not a heading")
	assert.Equal(t, int64(2), r.personID, "first item is the overdue reach out")

	press(m, "j")
	r, _ = m.selected()
	assert.False(t, r.skip)
	assert.Equal(t, "Bob Baker", r.title, "next item is the birthday, the heading is skipped")
	press(m, "j")
	r, _ = m.selected()
	assert.Equal(t, "send deck", r.title)
	press(m, "k", "k", "k")
	r, _ = m.selected()
	assert.Equal(t, 2, m.cursor, "k stops at the first item")
	assert.False(t, r.skip)

	press(m, "G")
	r, _ = m.selected()
	assert.False(t, r.skip, "G lands on the last item")
	press(m, "g")
	assert.Equal(t, int64(2), m.rows[m.cursor].personID)

	// A task row opens the person it belongs to.
	moveTo(t, m, func(r row) bool { return r.title == "send deck" })
	press(m, "enter")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, int64(1), m.curPerson())
	assert.Equal(t, focusDetail, m.focus, "the detail pane is open")
	assert.Contains(t, m.View(), "zed@acme.com")

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
	assert.Contains(t, jump.title, "all 8 in Due")
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

func TestHomeSyncSectionAppearsWithASource(t *testing.T) {
	m := syncModel(t)
	m.switchView(viewHome)
	titles := rowTitles(m)
	assert.Contains(t, titles, "Sync")
	assert.Contains(t, titles, "fake")
	assert.NotContains(t, titles, "inbox: 2 pending")

	pressRun(m, "8", "s")
	m.switchView(viewHome)
	titles = rowTitles(m)
	assert.Contains(t, titles, "inbox: 2 pending")
	var line string
	for _, r := range m.rows {
		if r.title == "fake" {
			line = r.sub
		}
	}
	assert.True(t, strings.HasPrefix(line, "ok "), line)
	press(m, "G", "enter")
	assert.Equal(t, viewSync, m.view, "the sync rows open the Sync view")
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
	press(m, "c")
	assert.Equal(t, "marked as contacted", m.status)
}

func TestHomeCapsBirthdaysAndLinksToTheView(t *testing.T) {
	m := homeModel(t)
	for i := 0; i < 6; i++ {
		p, err := m.pr.Create(context.Background(), model.CreatePersonInput{FirstName: "Bday" + string(rune('A'+i))})
		require.NoError(t, err)
		_, err = m.pr.Update(context.Background(), p.ID, model.UpdatePersonInput{Birthday: sp("--10-2" + string(rune('0'+i)))})
		require.NoError(t, err)
	}
	require.NoError(t, m.reload())
	people := 0
	var link row
	in := false
	for _, r := range m.rows {
		if r.skip {
			in = strings.HasPrefix(r.title, "Birthdays")
			continue
		}
		if in && r.personID != 0 {
			people++
		}
		if in && r.jump != 0 {
			link = r
		}
	}
	assert.Equal(t, homeBirthdayMax, people)
	assert.Equal(t, "all 7 in Birthdays", link.title)
	assert.Equal(t, int(viewBirthdays)+1, link.jump)
}
