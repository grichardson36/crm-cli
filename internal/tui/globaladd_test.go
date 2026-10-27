package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kindLabels(m *Model) []string {
	var out []string
	for _, idx := range m.ov.shown {
		out = append(out, m.ov.items[idx].label)
	}
	return out
}

func kindHints(m *Model) []string {
	var out []string
	for _, idx := range m.ov.shown {
		out = append(out, m.ov.items[idx].hint)
	}
	return out
}

func TestAOpensAKindPickerBuiltFromTheRegistry(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "a")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Add", m.ov.title)
	assert.Equal(t, []string{"Task", "Note", "Organization", "Person", "Deal", "Tag"}, kindLabels(m))
	assert.Equal(t, []string{"t", "n", "o", "p", "d", "g"}, kindHints(m), "each kind shows the shortcut its registry entry declares")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode, "esc leaves where you were")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, "a", m.actionByID("add-any").hint)
}

func TestThePickerPreselectsTheViewsOwnKind(t *testing.T) {
	cases := []struct {
		view viewKind
		want string
	}{{viewPeople, "Person"}, {viewHome, "Person"}, {viewBirthdays, "Person"}, {viewTasks, "Task"}, {viewOrgs, "Organization"}, {viewDeals, "Deal"}}
	for _, tc := range cases {
		m, _ := newTestModel(t)
		m.switchView(tc.view)
		m.settle()
		press(m, "a")
		require.NotNil(t, m.ov, viewNames[tc.view])
		assert.Equal(t, tc.want, m.ov.items[m.ov.shown[m.ov.cursor]].label, "%s preselects its own kind", viewNames[tc.view])
		press(m, "esc")
	}
}

func TestAThenEnterAddsTheViewsOwnObject(t *testing.T) {
	m, _ := newTestModel(t)
	m.switchView(viewOrgs)
	m.settle()
	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode, "the organization add flow runs")
	submit(m, "Acme Co | acme.com | | | | | | | ")
	list, err := m.orr.FindAll(m.ctx, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, viewOrgs, m.view)
}

func TestAThenALetterJumpsToThatKind(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "a", "o")
	require.Equal(t, modePrompt, m.mode, "a o adds an organization from the People view")
	submit(m, "Acme Co | acme.com | | | | | | | ")
	list, _ := m.orr.FindAll(m.ctx, 0)
	require.Len(t, list, 1)
	assert.Equal(t, viewPeople, m.view, "the view is not changed")
	assert.Contains(t, m.toast.text, "organization added")

	press(m, "ctrl+z") // one undo step
	list, _ = m.orr.FindAll(m.ctx, 0)
	assert.Empty(t, list)
}

func TestATaskOffersTheHighlightedPersonAsAGrayLinkAndTabAcceptsIt(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "down") // Bob
	bob := m.curPerson()
	press(m, "a", "t")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.promptHint(0), "tab: link Bob Baker", "the suggestion is shown after the cursor")
	assert.NotContains(t, m.input.Value(), "Bob", "and is never put into the text")
	press(m, "tab")
	assert.Contains(t, m.promptHint(0), "linked to Bob Baker")
	assert.NotContains(t, m.promptHint(0), "tab: link")
	typeText(m, "Send deck")
	press(m, "enter")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, []int64{bob}, tasks[0].PersonIDs)
	assert.Equal(t, viewPeople, m.view)
}

func TestATaskWithoutTabStaysUnlinked(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "a", "t")
	typeText(m, "Order flowers")
	press(m, "enter")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Empty(t, tasks[0].PersonIDs, "nothing links until tab is pressed")
}

func TestANoteOfferedAnOrganizationLinksItOnTab(t *testing.T) {
	m, _ := newTestModel(t)
	o, err := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	m.afterWrite()
	m.switchView(viewOrgs)
	m.settle()
	press(m, "a", "n")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.promptHint(0), "tab: link Acme")
	press(m, "tab")
	typeText(m, "met their CEO")
	press(m, "enter")
	notes, err := m.ir.FindAll(m.ctx, model.InteractionFilters{OrgID: &o.ID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "met their CEO", *notes[0].Content)
}

func TestGlobalAddTaskDealPersonAndNote(t *testing.T) {
	m, pr := newTestModel(t)
	m.switchView(viewBirthdays)

	press(m, "a", "t") // task: a standalone task even though a person is highlighted, until tab
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Order flowers")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, "Order flowers", tasks[0].Title)
	assert.Empty(t, tasks[0].PersonIDs)
	assert.Equal(t, viewBirthdays, m.view)

	press(m, "a", "d") // deal
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Big rollout | 5000 | lead")
	deals, _ := m.dlr.FindAll(m.ctx, model.DealFilters{})
	require.Len(t, deals, 1)
	assert.Equal(t, viewBirthdays, m.view)

	press(m, "a", "p") // person: the form opens, a name saves it
	require.Equal(t, modeEdit, m.mode, "the person form takes the screen")
	typeText(m, "Zoe Quinn")
	press(m, "enter")
	assert.Equal(t, viewPeople, m.view, "the view's own kind is added in place and the new person shows in People")
	people, _ := pr.FindAll(m.ctx, model.PersonFilters{})
	var names []string
	for _, p := range people {
		names = append(names, p.FirstName)
	}
	assert.Contains(t, names, "Zoe")
}

func TestANoteWithNothingHighlightedAsksForThePerson(t *testing.T) {
	m, _ := newTestModel(t)
	m.switchView(viewSync)
	m.settle()
	press(m, "a", "n")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Note for", m.ov.title)
	typeText(m, "bob")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode, "the note prompt")
	typeText(m, "likes jazz")
	press(m, "enter")
	bob := int64(2)
	notes, err := m.ir.FindAll(m.ctx, model.InteractionFilters{PersonID: &bob, Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, notes)
	assert.Equal(t, "likes jazz", *notes[0].Content)
	assert.Equal(t, viewSync, m.view)
}

func TestGlobalAddFromTheBarReturnsToTheBar(t *testing.T) {
	m, _ := newTestModel(t)
	startBar(t, m, "bob", "person:2")
	press(m, "tab", "a") // in the bar's results a opens the picker
	require.NotNil(t, m.ov)
	assert.Equal(t, "Add", m.ov.title)
	assert.Equal(t, "Person", m.ov.items[m.ov.shown[m.ov.cursor]].label, "the highlighted row's kind is preselected")
	press(m, "d")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Quartz | 100 | lead")
	assert.Equal(t, modeOverlay, m.mode, "the bar comes back")
	assert.True(t, m.ov.palette)
	assert.Equal(t, "bob", m.ov.filter.Value(), "with the text you had typed")
	deals, _ := m.dlr.FindAll(m.ctx, model.DealFilters{})
	assert.Len(t, deals, 1)

	// Esc in the picker also lands back in the bar.
	press(m, "a", "esc")
	assert.Equal(t, modeOverlay, m.mode)
	assert.True(t, m.ov.palette)
}

func TestTypingAInTheBarInputTypesALetter(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "bob a")
	assert.Equal(t, "bob a", m.ov.filter.Value(), "a is text while the input has the focus")
	assert.Equal(t, "Command bar", m.ov.title)
}

func TestAddIsBlockedInReadOnlyModeAndRebindable(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	press(m, "a")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "read-only")

	m2, _ := newTestModel(t)
	m2.applyKeys(map[string][]string{"add-any": {"ctrl+b"}})
	m2.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	require.NotNil(t, m2.ov)
	assert.Equal(t, "Add", m2.ov.title)
}

func TestTheRetiredAddKeysDoNothing(t *testing.T) {
	for _, k := range []string{"t", "n", "+", "N"} {
		m, _ := newTestModel(t)
		before := len(m.rows)
		press(m, k)
		assert.Equal(t, modeNormal, m.mode, "%s no longer opens anything", k)
		assert.Nil(t, m.ov, k)
		assert.Len(t, m.rows, before, k)
	}
}

// Every kind that can be added has a shortcut in the picker, they are all different, and the picker shows
// exactly the letters the registry gives (owner decision 2026-10-28).
func TestAddShortcutsAreUniqueAndShownInThePicker(t *testing.T) {
	short := addShortcuts()
	seen := map[string]string{}
	for _, o := range objectDefs() {
		if o.add == nil {
			continue
		}
		s := short[o.kind]
		require.NotEmpty(t, s, "%s has no shortcut", o.kind)
		assert.Len(t, s, 1, o.kind)
		other, dup := seen[s]
		assert.False(t, dup, "%s and %s both take %q", other, o.kind, s)
		seen[s] = o.kind
	}
	want := map[string]string{"task": "t", model.NoteKind: "n", "organization": "o", "person": "p", "deal": "d", "tag": "g"}
	for kind, s := range want {
		assert.Equal(t, s, short[kind], kind)
	}
	m, _ := newTestModel(t)
	press(m, "a")
	require.NotNil(t, m.ov)
	shown := map[string]string{}
	for _, idx := range m.ov.shown {
		shown[m.ov.items[idx].value] = m.ov.items[idx].hint
	}
	assert.Equal(t, want, shown)
}

// A kind that declares nothing takes its first letter, or when that is taken the next free letter of its name.
func TestDefaultShortcutIsTheFirstFreeLetterOfTheName(t *testing.T) {
	add := func(m *Model) tea.Cmd { return nil }
	got := shortcutsFor([]*objDef{
		{kind: "task", label: "task", shortcut: "t", add: add},
		{kind: "tag", label: "tag", add: add},     // t is taken, a is free
		{kind: "thing", label: "thing", add: add}, // t taken, h is free
		{kind: "label", label: "label", add: add}, // l is free
		{kind: "lamp", label: "lamp", add: add},   // l taken, a taken by tag, m is free
		{kind: "noadd", label: "noadd"},
	})
	assert.Equal(t, map[string]string{"task": "t", "tag": "a", "thing": "h", "label": "l", "lamp": "m"}, got)
}

func TestATagThroughTheAddPickerCreatesAndTagsTheHighlightedObject(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "a", "g")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "tag-new", m.promptKind)
	typeText(m, "vip")
	assert.Contains(t, m.View(), "tab: link", "the highlighted person is offered")
	press(m, "tab", "enter")
	assert.Equal(t, modeNormal, m.mode)
	p := personByID(t, pr, 1)
	_ = p
	tags, err := repo.NewTagRepo(m.db).GetForEntity(context.Background(), "person", 1)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "vip", tags[0].Name)

	// Without tab the tag is created on its own and a second use of the name finds it.
	press(m, "a", "g")
	typeText(m, "later")
	press(m, "enter")
	assert.False(t, m.statusIsErr, m.status)
	assert.Contains(t, m.status, "created")
	var n int
	require.NoError(t, m.db.QueryRow(`SELECT COUNT(*) FROM tags WHERE name = 'later'`).Scan(&n))
	assert.Equal(t, 1, n)
	press(m, "a", "g")
	typeText(m, "later")
	press(m, "enter")
	require.NoError(t, m.db.QueryRow(`SELECT COUNT(*) FROM tags WHERE name = 'later'`).Scan(&n))
	assert.Equal(t, 1, n, "the same name is found, not duplicated")
}
