package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func notesOf(t *testing.T, m *Model, pid int64) []*model.Interaction {
	t.Helper()
	list, err := m.ir.FindAll(context.Background(), model.InteractionFilters{PersonID: &pid})
	require.NoError(t, err)
	return list
}

func TestNotePromptDefaultsToNoteAndRemembersType(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "a", "n")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "note: ", m.input.Prompt)
	assert.Contains(t, ansi.Strip(m.screen()), "tab type")
	typeLinked(m, "plain note")
	press(m, "enter")
	assert.Equal(t, "note added", m.status)

	// Tab switches the type (types are sorted by name: call, email, meeting, message, note).
	press(m, "a", "n", "tab")
	assert.Equal(t, "call: ", m.input.Prompt)
	typeLinked(m, "talked about the trip")
	press(m, "enter")
	assert.Equal(t, "call added", m.status)
	ns := notesOf(t, m, 1)
	require.Len(t, ns, 2)
	types := map[string]string{}
	for _, n := range ns {
		types[n.Type] = str(n.Content)
	}
	assert.Equal(t, "talked about the trip", types["call"])
	assert.Equal(t, "plain note", types["note"])

	// The last used type is remembered by the next prompt, and shift+tab goes back.
	press(m, "a", "n")
	assert.Equal(t, "call: ", m.input.Prompt)
	press(m, "shift+tab")
	assert.Equal(t, "note: ", m.input.Prompt)
	press(m, "esc")

	// The detail pane shows the type of each note. The screen is taller than the default so the
	// notes under the longer person form (it has a middle name row) stay in view.
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	press(m, "tab")
	out := m.screen()
	assert.Contains(t, out, "call")
	assert.Contains(t, out, "talked about the trip")
}

func TestNotePickerChoosesType(t *testing.T) {
	m, _ := newTestModel(t)
	runID(m, "note-pick")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Note type", m.ov.title)
	assert.Equal(t, "note", m.ov.items[0].id, "the remembered type is first")
	typeText(m, "meeting")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "meeting: ", m.input.Prompt)
	typeText(m, "kickoff")
	press(m, "enter")
	ns := notesOf(t, m, 1)
	require.Len(t, ns, 1)
	assert.Equal(t, "meeting", ns[0].Type)

	runID(m, "note-pick")
	assert.Equal(t, "meeting", m.ov.items[0].id, "remembered after use")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
}

func TestNotePromptFromDetailAndPaletteUseTypes(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "tab")
	goTo(t, m, "note")
	press(m, "a")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "note: ", m.input.Prompt)
	press(m, "tab")
	typeText(m, "from the detail pane")
	press(m, "enter")
	ns := notesOf(t, m, 1)
	require.Len(t, ns, 1)
	assert.Equal(t, "call", ns[0].Type)
}

func TestArchivedRememberedTypeFallsBackToNote(t *testing.T) {
	m, _ := newTestModel(t)
	m.noteType = "meeting"
	require.NoError(t, m.ntr.Archive(context.Background(), "meeting"))
	press(m, "a", "n")
	assert.Equal(t, "note: ", m.input.Prompt)
}
