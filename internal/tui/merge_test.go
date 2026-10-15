package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dupeModel adds two duplicate groups to the standard test data:
// HIGH Cara Cole (3) and Karen Cole (5) share an email, MED Bob Baker (2) and bob baker (4) share a name.
func dupeModel(t *testing.T) (*Model, *repo.PersonRepo) {
	t.Helper()
	m, pr := newTestModel(t)
	ctx := context.Background()
	bob2, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "bob", LastName: sp("baker"), Company: sp("Globex Inc"), Title: sp("CTO"), Location: sp("Austin")})
	require.NoError(t, err)
	karen, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Karen", LastName: sp("Cole"), Email: sp("karen@other.org")})
	require.NoError(t, err)
	require.NoError(t, pr.AddEmail(ctx, karen.ID, "cara@initech.com", nil, false))
	require.Equal(t, int64(4), bob2.ID)
	require.Equal(t, int64(5), karen.ID)
	require.NoError(t, m.reload())
	return m, pr
}

func archivedIDs(t *testing.T, pr *repo.PersonRepo) map[int64]bool {
	t.Helper()
	list, err := pr.FindAll(context.Background(), model.PersonFilters{Archived: true})
	require.NoError(t, err)
	out := map[int64]bool{}
	for _, p := range list {
		out[p.ID] = true
	}
	return out
}

func rowIndex(c *compareState, label string) int {
	for i, r := range c.rows {
		if r.label == label {
			return i
		}
	}
	return -1
}

func TestMergeViewListsGroupsHighFirstAndCyclesLevel(t *testing.T) {
	m, _ := dupeModel(t)
	press(m, "6")
	assert.Equal(t, viewMerge, m.view)
	require.Len(t, m.rows, 2)
	assert.True(t, strings.HasPrefix(m.rows[0].title, "HIGH"), m.rows[0].title)
	assert.Contains(t, m.rows[0].title, "Cara Cole | Karen Cole")
	assert.True(t, strings.HasPrefix(m.rows[1].title, "MED"), m.rows[1].title)
	out := m.View()
	assert.Contains(t, out, "2 groups, medium+")
	assert.Contains(t, out, "v Merge")
	assert.Contains(t, out, "email cara@initech.com", "the detail pane explains the group")

	// m lowers the level, wraps to high, then back to medium.
	press(m, "m")
	assert.Equal(t, dedupe.Low, m.minLevel)
	press(m, "m")
	assert.Equal(t, dedupe.Fuzzy, m.minLevel)
	press(m, "m")
	assert.Equal(t, dedupe.High, m.minLevel)
	require.Len(t, m.rows, 1)
	assert.Contains(t, m.rows[0].title, "HIGH")
	assert.Contains(t, m.View(), "1 groups, high+")
	press(m, "m")
	assert.Equal(t, dedupe.Medium, m.minLevel)
	assert.Len(t, m.rows, 2)

	// Person keys do nothing on a group row.
	press(m, "c", "f", "#")
	assert.Equal(t, modeNormal, m.mode)
}

func TestMergeViewEmptyMessage(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "6")
	assert.Empty(t, m.rows)
	assert.Contains(t, m.View(), "No duplicates")
}

func TestCompareFlowPickAndApply(t *testing.T) {
	m, pr := dupeModel(t)
	ctx := context.Background()
	press(m, "6", "j", "enter")
	require.NotNil(t, m.cmp)
	c := m.cmp
	require.Len(t, c.members, 2)
	assert.Equal(t, int64(2), c.members[0].id, "the lowest id is the default survivor")
	assert.Equal(t, 0, c.surv)
	for _, label := range []string{"name", "title", "company", "location", "birthday", "keep in touch"} {
		assert.GreaterOrEqual(t, rowIndex(c, label), 0, label)
	}
	assert.Equal(t, -1, rowIndex(c, "notes"), "equal fields are not shown")
	out := m.View()
	assert.Contains(t, out, "#2 Bob Baker")
	assert.Contains(t, out, "#4 bob baker")
	assert.Contains(t, out, "keeps the record")
	assert.Contains(t, out, "Combined, always")
	assert.Contains(t, out, "interactions")
	assert.Contains(t, out, "result:")

	// The survivor's company wins by default. Tab flips it to the other person.
	press(m, "j", "j") // name, title, company
	require.Equal(t, "company", c.rows[c.cur].label)
	assert.Equal(t, 0, c.resolved(c.cur))
	press(m, "tab")
	assert.Equal(t, 1, c.resolved(c.cur))
	press(m, " ")
	assert.Equal(t, 0, c.resolved(c.cur), "space flips back")
	press(m, "tab")
	assert.Equal(t, 1, c.resolved(c.cur))

	// An empty survivor value defaults to the first person that has one.
	assert.Equal(t, 1, c.resolved(rowIndex(c, "title")))

	press(m, "m")
	assert.Nil(t, m.cmp, "compare closes after the merge")
	assert.Contains(t, m.status, "merged 1 into #2")
	assert.False(t, m.statusIsErr)
	keep := personByID(t, pr, 2)
	assert.Equal(t, "Globex Inc", str(keep.Company), "picked value won")
	assert.Equal(t, "CTO", str(keep.Title))
	assert.Equal(t, "Bob", keep.FirstName, "survivor name kept")
	assert.True(t, archivedIDs(t, pr)[4])
	_ = ctx

	// The merged group is gone from the list, the HIGH group is still there.
	require.Len(t, m.rows, 1)
	assert.Contains(t, m.rows[0].title, "HIGH")
}

func TestCompareChangeSurvivor(t *testing.T) {
	m, pr := dupeModel(t)
	press(m, "6", "j", "enter", "s")
	require.NotNil(t, m.cmp)
	assert.Equal(t, 1, m.cmp.surv)
	assert.Contains(t, m.View(), "keeps the record")
	press(m, "m")
	assert.Nil(t, m.cmp)
	assert.True(t, archivedIDs(t, pr)[2], "the old survivor is archived")
	assert.False(t, archivedIDs(t, pr)[4])
	assert.Equal(t, "bob", personByID(t, pr, 4).FirstName)
}

func TestCompareEscAndQLeaveWithoutMerging(t *testing.T) {
	m, pr := dupeModel(t)
	for _, k := range []string{"esc", "q"} {
		press(m, "6", "enter")
		require.NotNil(t, m.cmp, k)
		press(m, k)
		assert.Nil(t, m.cmp, k)
		assert.Empty(t, archivedIDs(t, pr))
	}
	assert.NotEqual(t, modeOverlay, m.mode)
}

func TestCompareSwallowsOtherKeys(t *testing.T) {
	m, pr := dupeModel(t)
	press(m, "6", "enter")
	require.NotNil(t, m.cmp)
	for _, k := range []string{"c", "f", "n", "t", "#", "T", "1", "2", "x", "+", "u", "M"} {
		press(m, k)
		assert.NotNil(t, m.cmp, k)
		assert.Equal(t, modeNormal, m.mode, k)
		assert.Equal(t, viewMerge, m.view, k)
	}
	assert.False(t, personByID(t, pr, 3).IsFavorite)
	assert.Empty(t, archivedIDs(t, pr))
}

func TestCompareClickPicksValue(t *testing.T) {
	m, _ := dupeModel(t)
	press(m, "6", "j", "enter")
	c := m.cmp
	row := rowIndex(c, "company")
	require.GreaterOrEqual(t, row, 0)
	colW, _ := c.geometry(m.width)
	// Body line 2 is the first field row, and the screen has the title line above it.
	click(m, cmpLabelW+colW+1, 1+2+row)
	assert.Equal(t, row, c.cur)
	assert.Equal(t, 1, c.resolved(row))
	click(m, cmpLabelW+1, 1+2+row)
	assert.Equal(t, 0, c.resolved(row))
	m.Update(tea.MouseMsg{X: 3, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	assert.Equal(t, row+1, c.cur)
}

func TestDismissAndUndoDismiss(t *testing.T) {
	m, _ := dupeModel(t)
	press(m, "6", "j")
	assert.Contains(t, m.rows[m.cursor].title, "Bob")
	press(m, "D")
	require.Len(t, m.rows, 1)
	assert.Contains(t, m.rows[0].title, "HIGH")
	assert.Contains(t, m.status, "dismissed 2 people")
	assert.Len(t, m.dismissed, 1)

	press(m, "D")
	assert.Empty(t, m.rows)
	press(m, "u")
	require.Len(t, m.rows, 1, "undo brings back the last dismissed group")
	assert.Contains(t, m.rows[0].title, "HIGH")
	press(m, "u")
	assert.Len(t, m.rows, 2)
	press(m, "u")
	assert.True(t, m.statusIsErr, "nothing left to undo")
	assert.Len(t, m.rows, 2)

	// Dismiss from inside the compare screen.
	press(m, "enter", "D")
	assert.Nil(t, m.cmp)
	assert.Len(t, m.rows, 1)
}

func TestManualMergeOfMarkedPeople(t *testing.T) {
	m, pr := dupeModel(t)
	// One mark is not enough.
	press(m, " ")
	press(m, "M")
	assert.Nil(t, m.cmp)
	assert.True(t, m.statusIsErr)

	m.marked = map[int64]bool{1: true, 3: true}
	press(m, "M")
	require.NotNil(t, m.cmp)
	assert.Equal(t, []int64{1, 3}, []int64{m.cmp.members[0].id, m.cmp.members[1].id})
	assert.True(t, m.cmp.manual)
	assert.Contains(t, m.View(), "marked by you")
	press(m, "esc")
	assert.Nil(t, m.cmp)
	assert.Len(t, m.markedIDs(), 2, "marks survive a cancelled compare")

	press(m, "M")
	row := rowIndex(m.cmp, "company")
	require.GreaterOrEqual(t, row, 0)
	m.cmp.cur = row
	press(m, "tab")
	press(m, "m")
	assert.Nil(t, m.cmp)
	assert.Empty(t, m.marked)
	assert.Contains(t, m.status, "merged 1 into #1")
	assert.Equal(t, "Initech", str(personByID(t, pr, 1).Company))
	assert.True(t, archivedIDs(t, pr)[3])
	assert.NotContains(t, names(m), "Cara Cole")
}

func TestReadOnlyBlocksMergeArchiveAndNoteTypes(t *testing.T) {
	m, pr := dupeModel(t)
	m.readOnly = true
	press(m, "#")
	assert.True(t, m.statusIsErr)
	assert.Equal(t, modeNormal, m.mode)
	m.marked = map[int64]bool{1: true, 3: true}
	press(m, "M")
	assert.Nil(t, m.cmp)
	assert.True(t, m.statusIsErr)
	m.marked = map[int64]bool{}

	// Reading is fine: the compare screen opens, but merge, dismiss and undo are blocked.
	press(m, "6", "enter")
	require.NotNil(t, m.cmp)
	for _, k := range []string{"m", "D"} {
		m.setStatus("", false)
		press(m, k)
		assert.True(t, m.statusIsErr, k)
		assert.NotNil(t, m.cmp, k)
	}
	press(m, "esc")
	for _, k := range []string{"D", "u"} {
		m.setStatus("", false)
		press(m, k)
		assert.True(t, m.statusIsErr, k)
	}
	require.NoError(t, pr.Archive(context.Background(), 1))
	require.NoError(t, m.reload())
	press(m, "7")
	require.Len(t, m.rows, 1)
	for _, k := range []string{"u", "X"} {
		m.setStatus("", false)
		press(m, k)
		assert.True(t, m.statusIsErr, k)
		assert.Equal(t, modeNormal, m.mode)
	}
	press(m, "2", "N")
	assert.True(t, m.statusIsErr)
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, map[int64]bool{1: true}, archivedIDs(t, pr))
	assert.Empty(t, m.dismissed)
}

func TestArchiveRestoreFlow(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "#")
	assert.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.View(), "archive Zed Adams? y/n")
	press(m, "n")
	assert.Empty(t, archivedIDs(t, pr))
	assert.Len(t, m.rows, 3)

	press(m, "#", "y")
	assert.Equal(t, modeNormal, m.mode)
	assert.True(t, archivedIDs(t, pr)[1])
	assert.Equal(t, []string{"Bob Baker", "Cara Cole"}, names(m))
	assert.Contains(t, m.status, "archived Zed Adams")

	press(m, "7")
	assert.Equal(t, viewArchived, m.view)
	require.Len(t, m.rows, 1)
	assert.Equal(t, "Zed Adams", m.rows[0].title)
	out := m.View()
	assert.Contains(t, out, "1 archived")
	assert.Contains(t, out, "zed@acme.com", "the detail pane describes the archived person")

	// Edit keys do not reach archived people.
	press(m, "c", "f", "n", "t", "tab")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, focusList, m.focus)
	archived, err := pr.FindAll(context.Background(), model.PersonFilters{Archived: true})
	require.NoError(t, err)
	require.Len(t, archived, 1)
	assert.False(t, archived[0].IsFavorite)

	press(m, "u")
	assert.Empty(t, m.rows)
	assert.Contains(t, m.status, "restored Zed Adams")
	assert.Empty(t, archivedIDs(t, pr))
	press(m, "2")
	assert.Len(t, m.rows, 3)
}

func TestArchiveMarkedAndRestoreMarked(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "*", "#")
	assert.Contains(t, m.View(), "archive 3 people? y/n")
	press(m, "y")
	assert.Len(t, archivedIDs(t, pr), 3)
	assert.Empty(t, m.rows)
	assert.Empty(t, m.marked)

	press(m, "7")
	require.Len(t, m.rows, 3)
	press(m, " ", " ")
	assert.Len(t, m.markedIDs(), 2)
	press(m, "u")
	assert.Len(t, archivedIDs(t, pr), 1)
	assert.Contains(t, m.status, "restored: 2 people")
}

func TestArchivedPeopleStayOutOfOtherViews(t *testing.T) {
	m, pr := dupeModel(t)
	// Bob (2) is due, has a birthday and is in a duplicate group.
	press(m, "3")
	require.Equal(t, []string{"Bob Baker"}, names(m))
	press(m, "#", "y")
	assert.Empty(t, m.rows, "gone from Due")
	press(m, "5")
	assert.Empty(t, m.rows, "gone from Birthdays")
	press(m, "6")
	require.Len(t, m.rows, 1, "gone from Merge")
	assert.Contains(t, m.rows[0].title, "HIGH")
	press(m, "2")
	assert.NotContains(t, names(m), "Bob Baker")
	assert.True(t, archivedIDs(t, pr)[2])
}

func TestPurgeNeedsTheTypedName(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "#", "y", "7", "X")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.View(), `type "Zed Adams" to delete forever`)

	typeText(m, "zed")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "did not match")
	assert.True(t, archivedIDs(t, pr)[1], "still there")

	press(m, "esc") // no-op in normal mode
	press(m, "X")
	typeText(m, "  zed   ADAMS ")
	press(m, "enter")
	assert.False(t, m.statusIsErr, m.status)
	assert.Contains(t, m.status, "deleted Zed Adams forever")
	assert.Empty(t, archivedIDs(t, pr))
	_, err := pr.FindByID(context.Background(), 1)
	assert.Error(t, err)
	assert.Empty(t, m.rows)

	// Esc at the prompt purges nothing.
	press(m, "2", "#", "y", "7", "X", "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Len(t, archivedIDs(t, pr), 1)
}

func TestNewActionsInPaletteWithCorrectGreying(t *testing.T) {
	m, _ := dupeModel(t)
	state := func() map[string]string {
		press(m, "ctrl+k")
		out := map[string]string{}
		for _, it := range m.ov.items {
			out[it.id] = it.disabled
		}
		press(m, "esc")
		return out
	}
	newIDs := []string{"find-dupes", "compare", "dupe-level", "dismiss", "undo-dismiss", "merge-marked", "archive", "restore", "purge",
		"cmp-pick", "cmp-pick-back", "cmp-survivor", "cmp-apply", "cmp-back", "note-pick", "note-types", "view-merge", "view-archived"}

	st := state()
	for _, id := range newIDs {
		_, ok := st[id]
		assert.True(t, ok, "palette lists %s", id)
	}
	assert.Equal(t, "", st["find-dupes"])
	assert.Equal(t, "", st["archive"])
	assert.Equal(t, "", st["note-types"])
	assert.Equal(t, "", st["note-pick"])
	for _, id := range []string{"compare", "dupe-level", "dismiss", "undo-dismiss", "merge-marked", "restore", "purge", "cmp-apply", "cmp-pick", "cmp-survivor", "cmp-back"} {
		assert.Equal(t, "not here", st[id], id+" in People")
	}

	press(m, " ", " ") // two marks
	assert.Equal(t, "", func() string { return state()["merge-marked"] }())

	press(m, "6")
	st = state()
	assert.Equal(t, "", st["compare"])
	assert.Equal(t, "", st["dupe-level"])
	assert.Equal(t, "", st["dismiss"])
	assert.Equal(t, "", st["undo-dismiss"], "undo is offered in the Merge view")
	assert.Equal(t, "not here", st["archive"], "group rows are not people")
	assert.Equal(t, "not here", st["restore"])

	press(m, "enter")
	st = state()
	for _, id := range []string{"cmp-pick", "cmp-pick-back", "cmp-survivor", "cmp-apply", "cmp-back", "dismiss", "palette", "help"} {
		assert.Equal(t, "", st[id], id+" in compare")
	}
	for _, id := range []string{"archive", "note", "contacted", "compare", "find-dupes", "view-people", "note-types"} {
		assert.Equal(t, "not here", st[id], id+" in compare")
	}
	press(m, "esc")

	press(m, "7")
	st = state()
	assert.Equal(t, "not here", st["restore"], "nothing archived yet")
	press(m, "2", "#", "y", "7")
	st = state()
	assert.Equal(t, "", st["restore"])
	assert.Equal(t, "", st["purge"])
	assert.Equal(t, "not here", st["archive"])

	// The palette runs them.
	press(m, "ctrl+k")
	typeText(m, "find dup")
	press(m, "enter")
	assert.Equal(t, viewMerge, m.view)
	m.readOnly = true
	st = state()
	for _, id := range []string{"dismiss", "undo-dismiss", "merge-marked", "archive", "restore", "purge", "cmp-apply", "note-pick", "note-types"} {
		assert.Equal(t, "read-only", st[id], id+" read-only")
	}
	assert.Equal(t, "", st["compare"])
	assert.Equal(t, "", st["dupe-level"])
}

func TestTabBarFitsAndClicksWork(t *testing.T) {
	m, _ := dupeModel(t)
	for _, w := range []int{80, 100, 120} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for v := 1; v <= int(numViews); v++ {
			press(m, string(rune('0'+v)))
			line := strings.Split(m.View(), "\n")[0]
			assert.LessOrEqual(t, lipgloss.Width(line), w, "tab bar at width %d view %d", w, v)
			for _, name := range []string{"Home", "People", "Due", "Tasks"} {
				assert.Contains(t, line, name, "main tabs at width %d view %d", w, v)
			}
			if !isMainTab(viewKind(v - 1)) {
				assert.Contains(t, line, "v "+viewNames[v-1], "More shows the hidden current view at width %d", w)
			} else {
				assert.Contains(t, line, "v More")
			}
		}
	}

	// Click each tab at 80 columns. The More tab opens the view switcher.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	press(m, "1")
	for _, seg := range m.tabLayout() {
		click(m, seg.x0+1, 0)
		if seg.more {
			assert.Equal(t, modeOverlay, m.mode, "More opens the switcher")
			require.NotNil(t, m.ov)
			assert.Equal(t, "Go to view", m.ov.title)
			press(m, "esc")
			continue
		}
		assert.Equal(t, seg.view, m.view, "tab %q", seg.label)
	}

	// Narrow terminals shrink the other tabs to their key and clicks still land.
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 30})
	press(m, "2")
	segs := m.tabLayout()
	assert.Equal(t, "1", segs[0].label)
	assert.Equal(t, "3", segs[2].label)
	assert.LessOrEqual(t, segs[len(segs)-1].x1, 30)
	click(m, segs[2].x0, 0)
	assert.Equal(t, viewDue, m.view)
	assert.LessOrEqual(t, lipgloss.Width(strings.Split(m.View(), "\n")[0]), 30)
}

func TestNumberAndBracketKeysCoverEveryView(t *testing.T) {
	m, _ := newTestModel(t)
	for v := 1; v <= int(numViews); v++ {
		press(m, string(rune('0'+v)))
		assert.Equal(t, viewKind(v-1), m.view)
	}
	assert.Equal(t, viewCalendar, m.view)
	press(m, "]")
	assert.Equal(t, viewHome, m.view, "] wraps around, also from the Calendar")
	press(m, "[")
	assert.Equal(t, viewCalendar, m.view, "[ wraps back")
	press(m, "[")
	assert.Equal(t, viewSync, m.view)
}

func TestCompareAndNewViewsFitTerminals(t *testing.T) {
	m, _ := dupeModel(t)
	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 12}, {40, 10}, {120, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		check := func(what string) {
			lines := strings.Split(m.View(), "\n")
			assert.LessOrEqual(t, len(lines), size[1], "%v %s height", size, what)
			for _, l := range lines {
				assert.LessOrEqual(t, lipgloss.Width(l), size[0], "%v %s: %q", size, what, l)
			}
		}
		press(m, "6")
		check("merge list")
		press(m, "enter")
		check("compare")
		press(m, "j", "j", "j", "tab", "s")
		check("compare after keys")
		press(m, "esc", "7")
		check("archived")
		press(m, "2")
	}
}
