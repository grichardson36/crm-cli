package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
	press(m, "down", "tab")
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
	assert.Contains(t, m.screen(), "second draft")

	// Esc cancels without saving.
	press(m, "enter")
	replaceText(m, "never saved")
	press(m, "esc")
	assert.Equal(t, "second draft", str(noteContent(t, m, id).Content))
}

// The note type is a cell of the note row (owner decision 2026-10-28): right moves to it, enter edits it.
func TestNoteTypeIsACellOfTheRow(t *testing.T) {
	m, id := noteModel(t)
	assert.Nil(t, m.actionByID("note-retype"), "no separate change-type action")
	assert.Nil(t, m.actionByID("link-edit"), "E is gone")
	press(m, "E")
	assert.Equal(t, modeNormal, m.mode, "E does nothing now")
	require.Len(t, m.form[m.fcur].cells, 1)
	assert.Equal(t, "type", m.form[m.fcur].cells[0].key)
	setCell(t, m, 1, "meeting")
	assert.Equal(t, "meeting", noteContent(t, m, id).Type)
	assert.Contains(t, m.status, "saved type")
	assert.Equal(t, id, m.form[m.fcur].id)
	assert.Equal(t, "meeting", m.form[m.fcur].typ)
	assert.Equal(t, 1, m.fcol, "the cursor stays on the cell")
	// ctrl+z undoes it and left goes back to the row.
	press(m, "ctrl+z")
	assert.Equal(t, "note", noteContent(t, m, id).Type)
	press(m, "left")
	assert.Equal(t, 0, m.fcol)
	press(m, "left")
	assert.Equal(t, focusList, m.focus, "left from the first cell returns to the list")
}

func TestNoteDeleteAsksThenRestoreFromPalette(t *testing.T) {
	m, id := noteModel(t)
	press(m, "d")
	assert.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, `delete note "note `)
	assert.Contains(t, m.confirm.prompt, "first draft")
	press(m, "n")
	assert.Len(t, notesOf(t, m, 2), 1, "n keeps it")

	press(m, "d", "y")
	assert.Empty(t, notesOf(t, m, 2))
	assert.Equal(t, "note deleted (ctrl+z undoes)", m.status)
	assert.Equal(t, []int64{id}, m.deleted[model.NoteKind])

	press(m, "ctrl+k")
	typeText(m, "restore note")
	press(m, "enter")
	assert.Equal(t, "note restored", m.status)
	require.Len(t, notesOf(t, m, 2), 1)
	assert.Empty(t, m.deleted[model.NoteKind])
	assert.Equal(t, id, m.form[m.fcur].id)

	// Nothing left to restore.
	press(m, "ctrl+k")
	typeText(m, "restore note")
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
	require.Len(t, m.deleted[model.NoteKind], 2)
	last := m.deleted[model.NoteKind][1]
	m.verbRestore(noteDef())
	got := notesOf(t, m, 2)
	require.Len(t, got, 1)
	assert.Equal(t, last, got[0].ID, "the note deleted last comes back first")
	assert.Len(t, m.deleted[model.NoteKind], 1)
}

func TestReadOnlyBlocksNoteEditTypeDeleteRestore(t *testing.T) {
	m, id := noteModel(t)
	press(m, "d", "y")
	require.Empty(t, notesOf(t, m, 2))
	m.verbRestore(noteDef()) // bring it back to test the other keys, then lock
	focusNote(t, m)
	m.readOnly = true

	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "read-only")
	press(m, "a", "t")
	assert.Equal(t, modeNormal, m.mode, "no picker opens")
	assert.Contains(t, m.status, "read-only")
	press(m, "d")
	assert.Equal(t, modeNormal, m.mode, "no confirm opens")
	assert.Equal(t, "first draft", str(noteContent(t, m, id).Content))
	assert.Equal(t, "note", noteContent(t, m, id).Type)
	assert.Len(t, notesOf(t, m, 2), 1)

	m.deleted[model.NoteKind] = []int64{id}
	press(m, "ctrl+k")
	typeText(m, "restore note")
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

	assert.Equal(t, "not here", st["note-restore"], "nothing deleted yet")
	assert.Equal(t, "", st["edit"])
	assert.Equal(t, "", st["delete"])

	press(m, "esc") // back to the list
	st = state()

	// t on a person in the list still adds a task.
	press(m, "a", "t")
	assert.Equal(t, modePrompt, m.mode)
	press(m, "esc")

	// The footer offers the next cell on a note row.
	press(m, "tab")
	focusNote(t, m)
	assert.Contains(t, m.hintsLine(), "cell")

	assert.LessOrEqual(t, lipgloss.Width(strings.Split(m.screen(), "\n")[len(strings.Split(m.screen(), "\n"))-1]), 100)
}
