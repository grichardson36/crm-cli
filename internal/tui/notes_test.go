package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noteModel opens Bob (2) with one note and puts the detail cursor on it.
func noteModel(t *testing.T) (*Model, int64) {
	t.Helper()
	m, _ := newTestModel(t)
	n, err := m.ir.Create(context.Background(), model.CreateInteractionInput{Type: "note", Content: sp("first draft"), PersonIDs: []int64{2}})
	require.NoError(t, err)
	press(m, "j", "tab")
	require.Equal(t, focusDetail, m.focus)
	focusNote(t, m)
	return m, n.ID
}

func focusNote(t *testing.T, m *Model) {
	t.Helper()
	for i, r := range m.form {
		if r.kind == rkNote && !r.empty {
			m.fcur = i
			m.syncDetail()
			return
		}
	}
	t.Fatal("no note row")
}

func noteContent(t *testing.T, m *Model, id int64) *model.Interaction {
	t.Helper()
	n, err := m.ir.FindByID(context.Background(), id)
	require.NoError(t, err)
	return n
}

func TestNoteEnterEditsContentInline(t *testing.T) {
	m, id := noteModel(t)
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode)
	replaceText(m, "second draft")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, "note saved", m.status)
	assert.Equal(t, "second draft", str(noteContent(t, m, id).Content))
	assert.Equal(t, id, m.form[m.fcur].id, "the cursor stays on the note")
	assert.Contains(t, m.View(), "second draft")

	// Esc cancels without saving.
	press(m, "enter")
	replaceText(m, "never saved")
	press(m, "esc")
	assert.Equal(t, "second draft", str(noteContent(t, m, id).Content))
}

func TestNoteTChangesTypeWithAPicker(t *testing.T) {
	m, id := noteModel(t)
	press(m, "t")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Note type", m.ov.title)
	assert.Equal(t, "note", m.ov.items[0].label, "the current type is first")
	typeText(m, "meeting")
	press(m, "enter")
	assert.Equal(t, "meeting", noteContent(t, m, id).Type)
	assert.Equal(t, "note type set to meeting", m.status)
	assert.Equal(t, id, m.form[m.fcur].id)
	assert.Equal(t, "meeting", m.form[m.fcur].typ)
}

func TestNoteDeleteAsksThenRestoreFromPalette(t *testing.T) {
	m, id := noteModel(t)
	press(m, "d")
	assert.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "delete the note note")
	press(m, "n")
	assert.Len(t, notesOf(t, m, 2), 1, "n keeps it")

	press(m, "d", "y")
	assert.Empty(t, notesOf(t, m, 2))
	assert.Contains(t, m.status, "Restore last deleted note")
	assert.Equal(t, []int64{id}, m.deletedNotes)

	press(m, "ctrl+k")
	typeText(m, "restore last deleted")
	press(m, "enter")
	assert.Equal(t, "note restored", m.status)
	require.Len(t, notesOf(t, m, 2), 1)
	assert.Empty(t, m.deletedNotes)
	assert.Equal(t, id, m.form[m.fcur].id)

	// Nothing left to restore.
	press(m, "ctrl+k")
	typeText(m, "restore last deleted")
	var it overlayItem
	for _, i := range m.ov.shown {
		if m.ov.items[i].id == "note-restore" {
			it = m.ov.items[i]
		}
	}
	assert.Equal(t, "not here", it.disabled)
}

func TestNoteDeleteRestoreStackIsLastInFirstOut(t *testing.T) {
	m, _ := noteModel(t)
	_, err := m.ir.Create(context.Background(), model.CreateInteractionInput{Type: "note", Content: sp("other"), PersonIDs: []int64{2}})
	require.NoError(t, err)
	m.afterWrite()
	focusNote(t, m)
	press(m, "d", "y")
	focusNote(t, m)
	press(m, "d", "y")
	require.Empty(t, notesOf(t, m, 2))
	require.Len(t, m.deletedNotes, 2)
	last := m.deletedNotes[1]
	m.restoreLastNote()
	got := notesOf(t, m, 2)
	require.Len(t, got, 1)
	assert.Equal(t, last, got[0].ID, "the note deleted last comes back first")
	assert.Len(t, m.deletedNotes, 1)
}

func TestReadOnlyBlocksNoteEditTypeDeleteRestore(t *testing.T) {
	m, id := noteModel(t)
	press(m, "d", "y")
	require.Empty(t, notesOf(t, m, 2))
	m.restoreLastNote() // bring it back to test the other keys, then lock
	focusNote(t, m)
	m.readOnly = true

	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "read-only")
	press(m, "t")
	assert.Equal(t, modeNormal, m.mode, "no picker opens")
	assert.Contains(t, m.status, "read-only")
	press(m, "d")
	assert.Equal(t, modeNormal, m.mode, "no confirm opens")
	assert.Equal(t, "first draft", str(noteContent(t, m, id).Content))
	assert.Equal(t, "note", noteContent(t, m, id).Type)
	assert.Len(t, notesOf(t, m, 2), 1)

	m.deletedNotes = []int64{id}
	press(m, "ctrl+k")
	typeText(m, "restore last deleted")
	for _, i := range m.ov.shown {
		if m.ov.items[i].id == "note-restore" {
			assert.Equal(t, "read-only", m.ov.items[i].disabled)
		}
	}
}

func TestNoteActionsInPalette(t *testing.T) {
	m, _ := noteModel(t)
	state := func() map[string]string {
		press(m, "ctrl+k")
		out := map[string]string{}
		for _, it := range m.ov.items {
			out[it.id] = it.disabled
		}
		press(m, "esc")
		return out
	}
	st := state()
	assert.Equal(t, "", st["note-retype"], "on a note row")
	assert.Equal(t, "not here", st["note-restore"], "nothing deleted yet")
	assert.Equal(t, "", st["edit"])
	assert.Equal(t, "", st["delete"])

	press(m, "esc") // back to the list
	st = state()
	assert.Equal(t, "not here", st["note-retype"], "needs the detail pane")

	// t on a person in the list still adds a task.
	press(m, "t")
	assert.Equal(t, modePrompt, m.mode)
	press(m, "esc")

	// The footer offers the type key on a note row.
	press(m, "tab")
	focusNote(t, m)
	assert.Contains(t, m.hintsLine(), "type")
	assert.LessOrEqual(t, lipgloss.Width(strings.Split(m.View(), "\n")[len(strings.Split(m.View(), "\n"))-1]), 100)
}
