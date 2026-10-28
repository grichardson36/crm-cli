package tui

import (
	"context"
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
	assert.Contains(t, m.screen(), "tab type")
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

	// The detail pane shows the type of each note.
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

func TestManageNoteTypesAddAndArchive(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "manage note")
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Note types", m.ov.title)
	assert.Equal(t, "+add", m.ov.items[m.ov.shown[0]].id, "the first row adds a type")

	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "coffee | contact")
	press(m, "enter")
	assert.Contains(t, m.status, "added note type coffee")
	require.Equal(t, modeOverlay, m.mode, "the list reopens for the next change")
	nt, err := m.ntr.FindByName(context.Background(), "coffee")
	require.NoError(t, err)
	assert.True(t, nt.CountsAsContact)
	assert.False(t, nt.Archived)

	// A bad name shows the error.
	press(m, "enter")
	typeText(m, "bad name!")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.Equal(t, modeNormal, m.mode)

	// The new type can be used for a note.
	press(m, "ctrl+k")
	typeText(m, "manage note")
	press(m, "enter")
	typeText(m, "coffee")
	assert.Equal(t, "coffee", m.ov.items[m.ov.shown[0]].id)
	press(m, "enter")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.screen(), "archive note type coffee")
	press(m, "n")
	nt, _ = m.ntr.FindByName(context.Background(), "coffee")
	assert.False(t, nt.Archived)

	press(m, "ctrl+k")
	typeText(m, "manage note")
	press(m, "enter")
	typeText(m, "coffee")
	press(m, "enter", "y")
	nt, _ = m.ntr.FindByName(context.Background(), "coffee")
	assert.True(t, nt.Archived)
	require.Equal(t, modeOverlay, m.mode)
	for _, it := range m.ov.items {
		assert.NotEqual(t, "coffee", it.id)
	}
	press(m, "esc")

	// The default type cannot be archived, and an archived type leaves the picker.
	press(m, "ctrl+k")
	typeText(m, "manage note")
	press(m, "enter")
	typeText(m, "note")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.Equal(t, modeNormal, m.mode)
	names := m.noteTypeNames()
	assert.Contains(t, names, "note")
	assert.NotContains(t, names, "coffee")
}

func TestArchivedRememberedTypeFallsBackToNote(t *testing.T) {
	m, _ := newTestModel(t)
	m.noteType = "meeting"
	require.NoError(t, m.ntr.Archive(context.Background(), "meeting"))
	press(m, "a", "n")
	assert.Equal(t, "note: ", m.input.Prompt)
}
