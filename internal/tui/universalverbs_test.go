package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The universal verbs mean the same in every view (patterns/universal-verbs).

func TestEnterAndEAreOneActionInEveryView(t *testing.T) {
	a := buildActions()
	var activate *action
	for i := range a {
		if a[i].id == "activate" {
			activate = &a[i]
		}
	}
	require.NotNil(t, activate)
	assert.Equal(t, []string{"enter", "e"}, activate.keys, "one action id, both keys listed once")

	// Each view: the same key finds the one action, and it opens or edits the highlighted thing.
	m, _ := newTestModel(t)
	o, err := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_, err = m.dlr.Create(m.ctx, model.CreateDealInput{Title: "Big deal", Stage: "lead"})
	require.NoError(t, err)
	_, err = m.tr.Create(m.ctx, model.CreateTaskInput{Title: "Call", Priority: "medium"})
	require.NoError(t, err)
	_ = o
	m.afterWrite()
	for _, tc := range []struct {
		view viewKind
		want string
	}{
		{viewPeople, "detail"}, {viewDue, "detail"}, {viewOrgs, "prompt"}, {viewDeals, "prompt"}, {viewTasks, "prompt"}, {viewHome, ""},
	} {
		for _, k := range []string{"enter", "e"} {
			m.switchView(tc.view)
			m.focus = focusList
			m.mode = modeNormal
			m.settle()
			require.NotNil(t, m.lookupKey(k, keyNormal), "%s in %s", k, viewNames[tc.view])
			assert.Equal(t, "activate", m.lookupKey(k, keyNormal).id, "%s in %s", k, viewNames[tc.view])
			if tc.view == viewHome {
				continue
			}
			press(m, k)
			switch tc.want {
			case "detail":
				assert.Equal(t, focusDetail, m.focus, "%s in %s opens the record", k, viewNames[tc.view])
			case "prompt":
				assert.Equal(t, modePrompt, m.mode, "%s in %s edits the record as one line", k, viewNames[tc.view])
				press(m, "esc")
			}
			m.focus = focusList
		}
	}

	// In the detail form it edits the field under the cursor, with either key.
	for _, k := range []string{"enter", "e"} {
		m.switchView(viewPeople)
		m.focus = focusList
		press(m, "tab")
		goTo(t, m, "company")
		press(m, k)
		assert.Equal(t, modeEdit, m.mode, "%s edits the field", k)
		press(m, "esc")
		press(m, "esc")
	}
}

func TestXMarksTheOneMeaningOfEachType(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "x")
	assert.NotNil(t, personByID(t, pr, 1).LastContactedAt, "a person is contacted")

	_, err := m.tr.Create(m.ctx, model.CreateTaskInput{Title: "Call", Priority: "medium"})
	require.NoError(t, err)
	m.afterWrite()
	press(m, "4", "x")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	assert.Empty(t, tasks, "a task is done")

	// x on a done task opens it again. A done task shows on a task row of a record page.
	all, err := m.tr.FindAll(m.ctx, model.TaskFilters{IncludeCompleted: true})
	require.NoError(t, err)
	require.Len(t, all, 1)
	m.switchView(viewPeople)
	m.focus = focusDetail
	m.form = []formRow{{kind: rkTask, id: all[0].ID, label: "task"}}
	m.fcur = 0
	press(m, "x")
	got, err := m.tr.FindByID(m.ctx, all[0].ID)
	require.NoError(t, err)
	assert.False(t, got.Completed, "x is a toggle on a task")
	press(m, "ctrl+z")
	got, _ = m.tr.FindByID(m.ctx, all[0].ID)
	assert.True(t, got.Completed, "and ctrl+z undoes the reopen")
}


func TestDeleteLinkUnlinkRestoreAreOneKeyInEveryObjectView(t *testing.T) {
	m, _ := newTestModel(t)
	_, err := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_, err = m.dlr.Create(m.ctx, model.CreateDealInput{Title: "Big deal", Stage: "lead"})
	require.NoError(t, err)
	_, err = m.tr.Create(m.ctx, model.CreateTaskInput{Title: "Call", Priority: "medium"})
	require.NoError(t, err)
	m.afterWrite()
	for _, v := range []viewKind{viewPeople, viewOrgs, viewDeals, viewTasks} {
		m.switchView(v)
		m.focus = focusList
		m.mode = modeNormal
		m.settle()
		for k, family := range map[string]string{"d": "delete", "l": "link", "u": "unlink"} {
			a := m.lookupKey(k, keyNormal)
			require.NotNil(t, a, "%s in %s", k, viewNames[v])
			assert.Equal(t, family, auditFamily(a.id), "%s in %s", k, viewNames[v])
		}
	}
	require.NoError(t, m.pr.Archive(m.ctx, 3))
	m.afterWrite()
	m.switchView(viewArchived)
	m.settle()
	assert.Equal(t, "restore", auditFamily(m.lookupKey("r", keyNormal).id), "r restores on the Archived screen")
	assert.Equal(t, "delete", auditFamily(m.lookupKey("d", keyNormal).id), "d is the delete verb there too")
}

func TestMergeKeyActsOnSelectedRowsOrTheHighlightedDuplicate(t *testing.T) {
	m, _ := dupeModel(t)
	// People: two selected rows open the compare.
	m.marked = map[int64]bool{1: true, 3: true}
	press(m, "m")
	require.NotNil(t, m.cmp)
	press(m, "esc")
	// One selected row is not enough, and says so.
	m.marked = map[int64]bool{1: true}
	press(m, "m")
	assert.Nil(t, m.cmp)
	assert.True(t, m.statusIsErr)
	// Merge view: the highlighted duplicate group opens.
	m.marked = map[int64]bool{}
	press(m, "8", "m")
	require.NotNil(t, m.cmp, "m on the highlighted duplicate group")
	press(m, "esc")
	// Organizations: two marked organizations ask for the survivor.
	a, _ := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Orbit"})
	b, _ := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Orbit Labs"})
	m.afterWrite()
	m.switchView(viewOrgs)
	m.settle()
	m.marked = map[int64]bool{a.ID: true, b.ID: true}
	press(m, "m")
	assert.NotEqual(t, modeNormal, m.mode, "the organization merge asks which one to keep")
}

func TestUndoRedoRefreshKeysOnly(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "f")
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	press(m, "z", "ctrl+r") // no bare z, and ctrl+r is refresh and not redo
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	assert.Equal(t, "reloaded", m.status)
	press(m, "ctrl+z")
	assert.False(t, personByID(t, pr, 1).IsFavorite)
	press(m, "ctrl+y")
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	m.setStatus("", false)
	press(m, "r") // r is restore, not reload
	assert.NotEqual(t, "reloaded", m.status)
}

func TestSpaceSelectsRowsInEveryListThatHasPeople(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, " ")
	assert.Len(t, m.marked, 1)
}

// Every function a removed key had stays reachable: a palette entry or a ctrl chord.
func TestEveryMovedFunctionIsStillReachable(t *testing.T) {
	m, _ := newTestModel(t)
	chords := map[string]string{ // action id -> its new ctrl chord
		"sort": "ctrl+o", "dupe-level": "ctrl+l", "archived-type": "ctrl+f", "reload": "ctrl+r",
		"cmp-survivor": "ctrl+e", "cmp-pair": "ctrl+b", "cmp-view": "ctrl+w",
	}
	palette := []string{ // palette entries without a key
		"sync-now", "sync-dry", "inbox-attach", "note-pick", "tag", "task-tag", "go-today",
		"undo-dismiss", "scroll-down", "scroll-up", "contacted", "done", "org-merge", "merge-marked",
		"org-add", "deal-add", "task", "new", "note", "person-edit", "org-edit", "deal-edit", "task-edit",
		"org-restore", "deal-restore", "task-restore", "note-retype", "tag-rename", "compare", "home-open", "edit", "inbox-accept",
	}
	for id, key := range chords {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		assert.Equal(t, []string{key}, a.keys, id)
	}
	for _, id := range palette {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		assert.Empty(t, a.keys, "%s has no bare key now", id)
		assert.NotEmpty(t, a.title, id)
		assert.False(t, a.barOnly, "%s is in the palette", id)
	}
}

// The user can still bind a key to a function that lost its default, and the file format is unchanged.
func TestAnOldBindingOfAPaletteOnlyActionStillWorks(t *testing.T) {
	m, _ := newTestModel(t)
	problems := m.applyKeys(keys.Overrides{"note-pick": {"p"}, "contacted": {"c"}})
	assert.Empty(t, problems)
	assert.Equal(t, []string{"p"}, m.actionByID("note-pick").keys)
	press(m, "c")
	assert.NotNil(t, personByID(t, m.pr, 1).LastContactedAt, "c still marks contacted for someone who bound it")
}

func TestStaleKeyFileIsReportedNotFatal(t *testing.T) {
	m, _ := newTestModel(t)
	problems := m.applyKeys(keys.Overrides{"go-to-today": {"ctrl+t"}, "snooze": {"d"}, "undo": {"ctrl+g"}})
	require.Len(t, problems, 2)
	assert.Contains(t, problems[0], "keys.go-to-today: no such action")
	assert.Contains(t, problems[1], "keys.snooze: d is already used by")
	assert.Equal(t, []string{"s"}, m.actionByID("snooze").keys)
	assert.Equal(t, []string{"ctrl+g"}, m.actionByID("undo").keys, "the valid entry still applies")
	assert.ElementsMatch(t, problems, StaleOverrides(keys.Overrides{"go-to-today": {"ctrl+t"}, "snooze": {"d"}}))
}

// Bubble Tea reports the ctrl+shift arrows as these names where the terminal sends the sequences.
func TestCtrlShiftArrowsHaveTheNamesTheRegistryUses(t *testing.T) {
	assert.Equal(t, "ctrl+shift+up", tea.KeyMsg{Type: tea.KeyCtrlShiftUp}.String())
	assert.Equal(t, "ctrl+shift+down", tea.KeyMsg{Type: tea.KeyCtrlShiftDown}.String())
	assert.Equal(t, "ctrl+down", tea.KeyMsg{Type: tea.KeyCtrlDown}.String())
	assert.Equal(t, "pgdown", tea.KeyMsg{Type: tea.KeyPgDown}.String())
	m, _ := newTestModel(t)
	assert.Equal(t, "home-top", m.lookupKey("ctrl+shift+up", keyNormal).id)
	assert.Equal(t, "home-bottom", m.lookupKey("ctrl+shift+down", keyNormal).id)
	n, err := keys.Normalize("ctrl+shift+up")
	require.NoError(t, err)
	assert.Equal(t, "ctrl+shift+up", n)
	// Without the chord, home and end do the same job.
	press(m, "end")
	last := m.cursor
	press(m, "home")
	assert.Equal(t, 0, m.cursor)
	press(m, "ctrl+shift+down")
	assert.Equal(t, last, m.cursor, "views with no home destination: ctrl+shift+down is end")
	press(m, "ctrl+shift+up")
	assert.Equal(t, 0, m.cursor, "and ctrl+shift+up is home")
}

// A view declares its jump unit and home destination in one place, viewNavs. A view not listed
// inherits a page and no home.
func TestViewsInheritTheDefaultNavigation(t *testing.T) {
	for _, v := range []viewKind{viewHome, viewDue, viewTasks, viewDeals, viewMerge, viewArchived, viewSync} {
		nav := navOf(v)
		assert.Equal(t, unitPage, nav.unit, viewNames[v])
		assert.Nil(t, nav.home, viewNames[v])
	}
	assert.Equal(t, unitDay, navOf(viewBirthdays).unit)
	assert.NotNil(t, navOf(viewBirthdays).home, "Birthdays declares today as its home")
	assert.Equal(t, unitLetter, navOf(viewPeople).unit)
	assert.Equal(t, unitLetter, navOf(viewOrgs).unit)

	// A new view with a declared unit and home gets the behavior without any key code.
	var landed bool
	viewNavs[viewDeals] = viewNav{unit: unitPage, home: func(m *Model) { landed = true }}
	defer delete(viewNavs, viewDeals)
	m, _ := newTestModel(t)
	m.switchView(viewDeals)
	press(m, "ctrl+shift+up")
	assert.True(t, landed, "a declared home destination is used by ctrl+shift+up")
}

func TestOrganizationsJumpAFirstLetterGroup(t *testing.T) {
	m, _ := newTestModel(t)
	for _, n := range []string{"Alpha", "Apex", "Bravo", "Brio", "Cargo"} {
		_, err := m.orr.Create(context.Background(), model.CreateOrgInput{Name: n})
		require.NoError(t, err)
	}
	m.afterWrite()
	m.switchView(viewOrgs)
	m.settle()
	press(m, "home")
	press(m, "ctrl+down")
	assert.Equal(t, "Bravo", m.rows[m.cursor].title)
	press(m, "ctrl+down")
	assert.Equal(t, "Cargo", m.rows[m.cursor].title)
	press(m, "ctrl+up")
	assert.Equal(t, "Bravo", m.rows[m.cursor].title)
}
