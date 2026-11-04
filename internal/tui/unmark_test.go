package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-026: esc clears every mark of the view, as the last layer esc peels.

func spaceN(m *Model, n int) {
	for i := 0; i < n; i++ {
		press(m, "space")
	}
}

func marksOf(m *Model) int { return len(m.markedRefs()) }

func toastText(m *Model) string {
	if m.toast == nil {
		return ""
	}
	return m.toast.text
}

func orgsModel(t *testing.T) *Model {
	m, _ := newTestModel(t)
	for _, n := range []string{"Acme", "Bolt", "Core"} {
		_, err := m.orr.Create(context.Background(), model.CreateOrgInput{Name: n})
		require.NoError(t, err)
	}
	require.NoError(t, m.reload())
	m.switchView(viewOrgs)
	require.GreaterOrEqual(t, len(m.rows), 3)
	return m
}

func dealsModel(t *testing.T) *Model {
	m, _ := newTestModel(t)
	dr := repo.NewDealRepo(m.db)
	for _, n := range []string{"Site", "Big", "Small"} {
		_, err := dr.Create(context.Background(), model.CreateDealInput{Title: n, Stage: "lead"})
		require.NoError(t, err)
	}
	require.NoError(t, m.reload())
	m.switchView(viewDeals)
	require.GreaterOrEqual(t, len(m.rows), 3)
	return m
}

func tasksModel(t *testing.T) *Model {
	m, _ := newTestModel(t)
	tr := repo.NewTaskRepo(m.db)
	for _, n := range []string{"Call", "Write", "Send"} {
		_, err := tr.Create(context.Background(), model.CreateTaskInput{Title: n, Priority: "medium"})
		require.NoError(t, err)
	}
	require.NoError(t, m.reload())
	m.switchView(viewTasks)
	require.GreaterOrEqual(t, len(m.rows), 3)
	return m
}

func markTagsModel(t *testing.T) *Model {
	m, _ := newTestModel(t)
	tg := repo.NewTagRepo(m.db)
	for _, n := range []string{"family", "work", "fun"} {
		require.NoError(t, tg.Apply(context.Background(), "person", 1, n))
	}
	require.NoError(t, m.reload())
	m.switchView(viewTags)
	require.GreaterOrEqual(t, len(m.rows), 3)
	return m
}

func savedPeopleModel(t *testing.T) *Model {
	m := svModel(t)
	v := svCreate(t, m, "All people", "person", query.Query{Filter: []query.Condition{cond("company", query.OpContains, "e")}}, false)
	m.openSaved(v.UUID)
	require.Equal(t, viewPeople, m.view)
	require.NotNil(t, m.sv)
	require.GreaterOrEqual(t, len(m.rows), 2)
	return m
}

// S1
func TestEscUnmarksEveryMarkedPerson(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 3)
	require.Len(t, m.markedIDs(), 3)
	cursor, rows, hist := m.cursor, len(m.rows), len(m.hist)
	press(m, "esc")
	assert.Empty(t, m.marked)
	assert.Equal(t, cursor, m.cursor, "the cursor stays")
	assert.Len(t, m.rows, rows)
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, focusList, m.focus)
	assert.Equal(t, "unmarked 3 (ctrl+z undoes)", toastText(m))
	require.NotNil(t, m.toast)
	assert.False(t, m.toast.isErr)
	assert.Len(t, m.hist, hist+1, "one undo step")
	assert.Equal(t, "unmark 3", m.hist[len(m.hist)-1].label)
	assert.Contains(t, plainScreen(m), "unmarked 3 (ctrl+z undoes)")
	assert.NotContains(t, plainScreen(m), "marked  ")
}

// S2
func TestEscWithoutMarksDoesWhatItDidBefore(t *testing.T) {
	for name, mk := range map[string]func() *Model{
		"people": func() *Model { m, _ := newTestModel(t); return m },
		"tasks":  func() *Model { return tasksModel(t) },
	} {
		m := mk()
		press(m, "down")
		view, focus, cursor, status, hist := m.view, m.focus, m.cursor, m.status, len(m.hist)
		press(m, "esc")
		assert.Equal(t, view, m.view, name)
		assert.Equal(t, focus, m.focus, name)
		assert.Equal(t, cursor, m.cursor, name)
		assert.Equal(t, status, m.status, "%s: no status", name)
		assert.Empty(t, m.marked, name)
		assert.Nil(t, m.toast, name)
		assert.Len(t, m.hist, hist, "%s: nothing to undo", name)
	}
}

// S3
func TestEscClearsTheSearchBeforeTheMarks(t *testing.T) {
	m, _ := newTestModel(t)
	m.showPeopleQuery("a")
	require.GreaterOrEqual(t, len(m.rows), 2)
	spaceN(m, 2)
	require.Len(t, m.markedIDs(), 2)
	press(m, "esc")
	assert.Empty(t, m.query, "the first esc clears the search")
	assert.Len(t, m.markedIDs(), 2, "and keeps the marks")
	press(m, "esc")
	assert.Empty(t, m.marked, "the second esc clears the marks")
	assert.Equal(t, "unmarked 2 (ctrl+z undoes)", toastText(m))
}

// S4
func TestEscUnmarksInEveryList(t *testing.T) {
	for name, mk := range map[string]func(*testing.T) *Model{
		"organizations": orgsModel, "deals": dealsModel, "tasks": tasksModel, "tags": markTagsModel, "saved view": savedPeopleModel,
	} {
		t.Run(name, func(t *testing.T) {
			m := mk(t)
			view, cursor := m.view, 0
			spaceN(m, 2)
			require.Equal(t, 2, marksOf(m))
			cursor = m.cursor
			press(m, "esc")
			assert.Equal(t, 0, marksOf(m))
			assert.Empty(t, m.marked)
			assert.Equal(t, view, m.view, "esc stays in the list")
			assert.Equal(t, cursor, m.cursor)
			assert.Equal(t, "unmarked 2 (ctrl+z undoes)", toastText(m))
		})
	}
}

func TestEscClearsTheSearchBeforeTheMarksInASavedView(t *testing.T) {
	m := savedPeopleModel(t)
	m.query = "a"
	m.report(m.rebuild(false))
	require.GreaterOrEqual(t, len(m.rows), 1)
	spaceN(m, 1)
	require.Equal(t, 1, marksOf(m))
	press(m, "esc")
	assert.Empty(t, m.query)
	assert.Equal(t, 1, marksOf(m))
	press(m, "esc")
	assert.Equal(t, 0, marksOf(m))
	assert.Equal(t, "unmarked 1 (ctrl+z undoes)", toastText(m))
}

func TestEscClearsTheTagSearchBeforeTheMarks(t *testing.T) {
	m := markTagsModel(t)
	m.showTagQuery("f")
	require.GreaterOrEqual(t, len(m.rows), 2)
	spaceN(m, 2)
	require.Equal(t, 2, marksOf(m))
	press(m, "esc")
	assert.Empty(t, m.tagQuery, "the first esc clears the tag search")
	assert.Equal(t, 2, marksOf(m))
	press(m, "esc")
	assert.Equal(t, 0, marksOf(m))
}

func TestEscOnATagPageLeavesThePageFirst(t *testing.T) {
	m := markTagsModel(t)
	press(m, "enter")
	require.True(t, m.onTagPage())
	m.cursorOn(func(r row) bool { _, ok := m.rowRef(r); return ok })
	spaceN(m, 1)
	require.Equal(t, 1, marksOf(m))
	press(m, "esc")
	assert.False(t, m.onTagPage(), "esc goes back to the tag list as before")
}

// S5
func TestEscLeavesTheDetailPaneKeepingTheMarks(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	press(m, "tab")
	require.Equal(t, focusDetail, m.focus)
	press(m, "esc")
	assert.Equal(t, focusList, m.focus, "esc goes back to the list")
	assert.Len(t, m.markedIDs(), 2, "and keeps the marks")
	press(m, "esc")
	assert.Empty(t, m.marked)
}

func TestOverlaysAndModesTakeEscBeforeTheMarks(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	press(m, "ctrl+k")
	require.Equal(t, modeOverlay, m.mode)
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Len(t, m.markedIDs(), 2, "the palette closed, the marks stay")

	press(m, "?")
	press(m, "esc")
	assert.Len(t, m.markedIDs(), 2, "help closed, the marks stay")

	press(m, "v")
	press(m, "esc")
	assert.Len(t, m.markedIDs(), 2, "the Views popup closed, the marks stay")

	press(m, "ctrl+o")
	press(m, "esc")
	assert.Len(t, m.markedIDs(), 2, "the options popup closed, the marks stay")

	press(m, "s")
	require.Equal(t, modePrompt, m.mode)
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Len(t, m.markedIDs(), 2, "the prompt closed, the marks stay")

	press(m, "esc")
	assert.Empty(t, m.marked)
}

// S6
func TestFooterShowsEscUnmarkOnlyWhileMarksExist(t *testing.T) {
	for name, mk := range map[string]func(*testing.T) *Model{
		"people":        func(t *testing.T) *Model { m, _ := newTestModel(t); return m },
		"organizations": orgsModel, "deals": dealsModel, "tasks": tasksModel, "tags": markTagsModel, "saved view": savedPeopleModel,
	} {
		t.Run(name, func(t *testing.T) {
			m := mk(t)
			assert.NotContains(t, hintsText(m), "unmark", "no marks")
			assert.NotContains(t, footHints(m), "esc")
			press(m, "space")
			assert.Contains(t, hintsText(m), "esc unmark")
			assert.Contains(t, footHints(m), "esc")
			press(m, "esc")
			assert.NotContains(t, hintsText(m), "esc unmark", "after unmarking")
		})
	}
}

func TestFooterEscUnmarkIsNotShownInTheDetailPaneOrAPopup(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	press(m, "tab")
	assert.NotContains(t, hintsText(m), "esc unmark")
	assert.Contains(t, hintsText(m), "esc back")
}

func TestMarkCountStaysInTheStatusLine(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	assert.Contains(t, plainScreen(m), "2 marked")
	press(m, "esc")
	assert.NotContains(t, plainScreen(m), "2 marked")
}

func TestEscUnmarkOutlastsTheOtherContextualHints(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	var seenFav, seenAll bool
	for w := 40; w <= 130; w++ {
		m.width = w
		labels := map[string]bool{}
		for _, s := range m.footerSegs() {
			labels[s.label] = true
		}
		if labels[hwFavorite] || labels[hwSnooze] {
			assert.True(t, labels[hwUnmark], "width %d: unmark must outlast favorite and snooze", w)
			seenFav = true
		}
		if labels[hwFavorite] && labels[hwSnooze] && labels[hwUnmark] {
			seenAll = true
		}
	}
	assert.True(t, seenFav, "favorite shows at some width")
	assert.True(t, seenAll, "all three fit at a wide width")
}

// S7
func TestVerbsActOnTheHighlightedRowAfterUnmarking(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	require.Len(t, m.bulkRefs(), 2)
	press(m, "esc")
	refs := m.bulkRefs()
	require.Len(t, refs, 1, "only the highlighted row")
	cur, ok := m.cursorRef()
	require.True(t, ok)
	assert.Equal(t, cur, refs[0])
	assert.Equal(t, []int64{cur.ID}, m.targets())
}

func TestUnmarkIsOneRegistryActionOnEsc(t *testing.T) {
	m, _ := newTestModel(t)
	a := m.actionByID("unmark")
	require.NotNil(t, a)
	assert.True(t, a.hasKey("esc"))
	assert.True(t, a.footCtx)
	assert.Greater(t, a.footKeep, m.actionByID("favorite").footKeep)
	assert.Greater(t, a.footKeep, m.actionByID("snooze").footKeep)
	assert.Equal(t, hwUnmark, a.footLabel)
	assert.False(t, a.write, "marks are not data")
}

// S8
func TestCtrlZRestoresTheMarksAndCtrlYClearsThemAgain(t *testing.T) {
	for name, mk := range map[string]func(*testing.T) *Model{
		"people":        func(t *testing.T) *Model { m, _ := newTestModel(t); return m },
		"organizations": orgsModel, "deals": dealsModel, "tasks": tasksModel, "tags": markTagsModel, "saved view": savedPeopleModel,
	} {
		t.Run(name, func(t *testing.T) {
			m := mk(t)
			spaceN(m, 2)
			before := map[int64]bool{}
			for k := range m.marked {
				before[k] = true
			}
			require.Len(t, before, 2)
			press(m, "esc")
			require.Empty(t, m.marked)
			press(m, "down", "up") // the cursor moved since
			press(m, "ctrl+z")
			assert.Equal(t, before, m.marked, "exactly the same marks")
			assert.Contains(t, toastText(m), "Undone: unmark 2")
			press(m, "ctrl+y")
			assert.Empty(t, m.marked, "redo clears them again")
			assert.Contains(t, toastText(m), "Redone: unmark 2")
			press(m, "ctrl+z")
			assert.Equal(t, before, m.marked)
			press(m, "esc")
			assert.Equal(t, "unmarked 2 (ctrl+z undoes)", toastText(m))
		})
	}
}

func TestUndoUnmarkSkipsRowsThatLeftTheList(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 3)
	gone := m.rows[0].personID
	press(m, "esc")
	_, err := m.db.Exec(`UPDATE people SET archived = 1 WHERE id = ?`, gone)
	require.NoError(t, err)
	require.NoError(t, m.reload())
	press(m, "ctrl+z")
	assert.Len(t, m.markedIDs(), 2, "the archived person is not marked again")
	for _, id := range m.markedIDs() {
		assert.NotEqual(t, gone, id)
	}
}

func TestUndoUnmarkIsRefusedInAnotherListAndKeepsTheEntry(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	press(m, "esc")
	hist := len(m.hist)
	m.switchView(viewTasks)
	press(m, "ctrl+z")
	assert.Contains(t, toastText(m), "Cannot undo unmark 2: the marks belong to People, open it first")
	assert.True(t, m.toast.isErr)
	assert.Len(t, m.hist, hist, "the entry stays so the undo works once People is open")
	assert.Empty(t, m.marked)
	m.switchView(viewPeople)
	press(m, "ctrl+z")
	assert.Len(t, m.markedIDs(), 2)
}

func TestUnmarkIsInTheUndoHistory(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	press(m, "esc")
	assert.Equal(t, "unmark 2", m.hist[len(m.hist)-1].label)
}

func TestEscUnmarkSitsAfterTheVerbsAndBeforeNavigation(t *testing.T) {
	m, _ := newTestModel(t)
	spaceN(m, 2)
	m.width = 130
	idx := map[string]int{}
	for i, s := range m.footerSegs() {
		idx[s.label] = i
	}
	require.Contains(t, idx, hwUnmark)
	require.Contains(t, idx, hwFavorite)
	require.Contains(t, idx, hwMove)
	assert.Greater(t, idx[hwUnmark], idx[hwFavorite], "after the other verbs")
	assert.Less(t, idx[hwUnmark], idx[hwMove], "before navigation")
}

func TestUndoUnmarkIsRefusedInAnotherSavedViewOfTheSameKind(t *testing.T) {
	m := svModel(t)
	a := svCreate(t, m, "A people", "person", query.Query{Filter: []query.Condition{cond("company", query.OpContains, "e")}}, false)
	b := svCreate(t, m, "B people", "person", query.Query{Filter: []query.Condition{cond("company", query.OpContains, "e")}}, false)
	m.openSaved(a.UUID)
	spaceN(m, 2)
	press(m, "esc")
	m.openSaved(b.UUID)
	press(m, "ctrl+z")
	assert.Contains(t, toastText(m), "Cannot undo unmark 2: the marks belong to A people")
	assert.Empty(t, m.marked)
	m.openSaved(a.UUID)
	press(m, "ctrl+z")
	assert.Len(t, m.markedIDs(), 2)
}

func TestUnmarkToastNamesTheReboundUndoKey(t *testing.T) {
	m, _ := newTestModel(t)
	m.applyKeys(keys.Overrides{"undo": {"alt+u"}})
	spaceN(m, 1)
	press(m, "esc")
	assert.Equal(t, "unmarked 1 (alt+u undoes)", toastText(m))
}
