package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestCtrlAOpensAKindPickerBuiltFromTheRegistry(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+a")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "New", m.ov.title)
	assert.Equal(t, []string{"Person", "Organization", "Deal", "Task", "Note"}, kindLabels(m))
	assert.Equal(t, "1", m.ov.items[0].hint)
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode, "esc leaves where you were")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, "ctrl+a", m.actionByID("add-any").hint)
}

func TestGlobalAddOrganizationFromPeopleReturnsToThePeopleList(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "j")
	cursor, who := m.cursor, m.rows[m.cursor].personID
	press(m, "ctrl+a", "2")
	require.Equal(t, modePrompt, m.mode, "the organization add flow runs")
	submit(m, "Acme Co | acme.com | | | | | | | ")
	assert.Equal(t, modeNormal, m.mode)
	list, err := m.orr.FindAll(m.ctx, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Acme Co", list[0].Name)
	assert.Equal(t, viewPeople, m.view, "the view is not changed")
	assert.Equal(t, cursor, m.cursor)
	assert.Equal(t, who, m.rows[m.cursor].personID, "and the selection is where it was")
	assert.Contains(t, m.toast.text, "organization added")

	press(m, "ctrl+z") // one undo step
	list, _ = m.orr.FindAll(m.ctx, 0)
	assert.Empty(t, list)
}

func TestGlobalAddTaskDealPersonAndNote(t *testing.T) {
	m, pr := newTestModel(t)
	m.switchView(viewBirthdays)

	press(m, "ctrl+a", "4") // task: a standalone task even though a person is selected
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Order flowers")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, "Order flowers", tasks[0].Title)
	assert.Empty(t, tasks[0].PersonIDs, "from anywhere it is the task's own, Link can attach it")
	assert.Equal(t, viewBirthdays, m.view)

	press(m, "ctrl+a", "3") // deal
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Big rollout | 5000 | lead")
	deals, _ := m.dlr.FindAll(m.ctx, model.DealFilters{})
	require.Len(t, deals, 1)
	assert.Equal(t, viewBirthdays, m.view)

	press(m, "ctrl+a", "1") // person: the form opens, a name saves it
	require.Equal(t, modeEdit, m.mode, "the person form takes the screen")
	typeText(m, "Zoe Quinn")
	press(m, "enter")
	assert.Equal(t, viewBirthdays, m.view, "and the Birthdays view is back")
	people, _ := pr.FindAll(m.ctx, model.PersonFilters{})
	var names []string
	for _, p := range people {
		names = append(names, p.FirstName)
	}
	assert.Contains(t, names, "Zoe")

	press(m, "ctrl+a", "5") // note: pick the person first
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
	assert.Equal(t, viewBirthdays, m.view)
}

func TestGlobalAddFromTheBarReturnsToTheBar(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "bob")
	press(m, "ctrl+a")
	require.NotNil(t, m.ov)
	assert.Equal(t, "New", m.ov.title)
	press(m, "3")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Quartz | 100 | lead")
	assert.Equal(t, modeOverlay, m.mode, "the bar comes back")
	assert.True(t, m.ov.palette)
	assert.Equal(t, "bob", m.ov.filter.Value(), "with the text you had typed")
	deals, _ := m.dlr.FindAll(m.ctx, model.DealFilters{})
	assert.Len(t, deals, 1)

	// Esc in the picker also lands back in the bar.
	press(m, "ctrl+a", "esc")
	assert.Equal(t, modeOverlay, m.mode)
	assert.True(t, m.ov.palette)
}

func TestGlobalAddIsBlockedInReadOnlyModeAndRebindable(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	press(m, "ctrl+a")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "read-only")
	press(m, "ctrl+k", "ctrl+a")
	assert.Equal(t, "Command bar", m.ov.title, "the bar stays")
	assert.Contains(t, m.toast.text, "read-only")
	press(m, "esc")

	m2, _ := newTestModel(t)
	m2.applyKeys(map[string][]string{"add-any": {"ctrl+b"}})
	m2.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	require.NotNil(t, m2.ov)
	assert.Equal(t, "New", m2.ov.title)
}

func TestCtrlAAndPromptInputsKeepLineStart(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "t") // task prompt on a person
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "abc")
	press(m, "ctrl+a") // readline line start inside a text input
	assert.Equal(t, modePrompt, m.mode, "the global chord does not fire while typing")
}
