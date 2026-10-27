package tui

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Note rows in the detail pane of every record page: edit (Enter, in form.go), change type (t) and
// delete (d). Add, edit, delete, restore, link and unlink come from the note object in the registry
// (objects.go). Edits go through InteractionRepo.Update/Archive/Restore.

// whenNoteRow is true when the detail cursor is on a saved note.
func whenNoteRow(m *Model) bool {
	if !whenDetail(m) {
		return false
	}
	r := m.form[m.fcur]
	return r.kind == rkNote && !r.empty && r.id != 0
}

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

// noteTarget is the note under the detail cursor, 0 when the cursor is not on a saved note. It is
// the selection of the note object in the registry (objects.go), the same on every record page.
func (m *Model) noteTarget() int64 {
	if !whenNoteRow(m) {
		return 0
	}
	return m.form[m.fcur].id
}

// noteLabel names a note: its subject, else the start of its text, else its type.
func noteLabel(n *model.Interaction) string {
	if s := str(n.Subject); s != "" {
		return s
	}
	if c := strings.TrimSpace(str(n.Content)); c != "" {
		return c
	}
	return n.Type
}

func shortDate(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func (m *Model) noteName(id int64) string {
	if n, err := m.ir.FindByID(m.ctx, id); err == nil {
		return truncateLabel(n.Type + " " + shortDate(n.OccurredAt) + ": " + noteLabel(n))
	}
	return fmt.Sprintf("note %d", id)
}

// hCreateNote adds a note and records it for undo.
func (m *Model) hCreateNote(in model.CreateInteractionInput) (*model.Interaction, error) {
	n, err := m.ir.Create(m.ctx, in)
	if err != nil {
		return nil, err
	}
	id := n.ID
	m.record("add "+n.Type+" "+truncateLabel(noteLabel(n)),
		func() error { return m.ir.Archive(m.ctx, id) },
		func() error { return m.ir.Restore(m.ctx, id) })
	return n, nil
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
		if _, err := m.hUpdateNote(m.ctx, id, model.UpdateInteractionInput{Type: &name}); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWrite()
		m.selectNote(id)
		m.setStatus("note type set to "+name, false)
	})
}
