package tui

import (
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Note rows in the detail pane: edit (Enter, in form.go), change type (t), delete (d) and
// restore the last deleted note. Edits go through InteractionRepo.Update/Archive/Restore.

// whenNoteRow is true when the detail cursor is on a saved note.
func whenNoteRow(m *Model) bool {
	if !whenDetail(m) {
		return false
	}
	r := m.form[m.fcur]
	return r.kind == rkNote && !r.empty && r.id != 0
}

func whenDeletedNote(m *Model) bool { return len(m.deletedNotes) > 0 }

// selectNote puts the detail cursor on the note with the id.
func (m *Model) selectNote(id int64) {
	for i, r := range m.form {
		if r.kind == rkNote && r.id == id {
			m.fcur = i
			m.syncDetail()
			return
		}
	}
}

func (m *Model) askDeleteNote(r formRow) {
	if r.empty || r.id == 0 {
		m.setStatus("nothing to delete here", true)
		return
	}
	id := r.id
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("delete the %s note from %s? y/n", r.typ, r.label),
		yes: func() {
			if err := m.ir.Archive(m.ctx, id); err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			m.deletedNotes = append(m.deletedNotes, id)
			m.afterWrite()
			m.setStatus("note deleted (palette: Restore last deleted note)", false)
		},
	}
}

// retypeNote lists the active note types and sets the chosen one on the note under the cursor.
func (m *Model) retypeNote() {
	r, ok := m.rowFocused()
	if !ok || r.kind != rkNote || r.empty {
		return
	}
	list, err := m.ntr.List(m.ctx, false)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	id := r.id
	var items []overlayItem
	for _, t := range list {
		if t.Name == r.typ {
			items = append(items, overlayItem{id: t.Name, label: t.Name, value: t.Name, desc: "current"})
		}
	}
	for _, t := range list {
		if t.Name != r.typ {
			items = append(items, overlayItem{id: t.Name, label: t.Name, value: t.Name})
		}
	}
	m.openPicker("Note type", items, func(m *Model, it overlayItem) {
		if it.value == r.typ {
			return
		}
		name := it.value
		if _, err := m.ir.Update(m.ctx, id, model.UpdateInteractionInput{Type: &name}); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWrite()
		m.selectNote(id)
		m.setStatus("note type set to "+name, false)
	})
}

// restoreLastNote brings back the note deleted most recently in this session.
func (m *Model) restoreLastNote() {
	if len(m.deletedNotes) == 0 {
		m.setStatus("no deleted note to restore in this session", true)
		return
	}
	id := m.deletedNotes[len(m.deletedNotes)-1]
	if err := m.ir.Restore(m.ctx, id); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.deletedNotes = m.deletedNotes[:len(m.deletedNotes)-1]
	m.afterWrite()
	m.selectNote(id)
	m.setStatus("note restored", false)
}
