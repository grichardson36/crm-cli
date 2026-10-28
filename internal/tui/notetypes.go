package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Note types: the type chosen for new notes and the manage overlay.

// noteTypeNames lists the active note types by name.
func (m *Model) noteTypeNames() []string {
	list, err := m.ntr.List(m.ctx, false)
	if err != nil {
		return nil
	}
	names := make([]string, len(list))
	for i, t := range list {
		names[i] = t.Name
	}
	return names
}

// currentNoteType is the remembered type if it is still active, else note, else the first active type.
func (m *Model) currentNoteType() string {
	names := m.noteTypeNames()
	if len(names) == 0 {
		return m.noteType
	}
	for _, want := range []string{m.noteType, "note"} {
		for _, n := range names {
			if n == want {
				return n
			}
		}
	}
	return names[0]
}

// startNotePrompt opens the note prompt with the remembered type. Tab in the prompt changes it.
func (m *Model) startNotePrompt() tea.Cmd {
	if m.curPerson() == 0 {
		return nil
	}
	m.noteType = m.currentNoteType()
	m.startInput("note", m.noteType+": ", "")
	return m.input.Focus()
}

// cycleNoteType switches the type of the note being typed.
func (m *Model) cycleNoteType(forward bool) {
	names := m.noteTypeNames()
	if len(names) < 2 {
		return
	}
	cur := 0
	for i, n := range names {
		if n == m.noteType {
			cur = i
		}
	}
	if forward {
		cur = (cur + 1) % len(names)
	} else {
		cur = (cur + len(names) - 1) % len(names)
	}
	m.noteType = names[cur]
	m.input.Prompt = m.noteType + ": "
	if m.promptKind == "note-object" {
		m.input.Prompt = m.objectNotePrompt(m.pageRef())
	}
}

// pickNoteType lists the active types (the remembered one first), then opens the note prompt.
func (m *Model) pickNoteType() {
	list, err := m.ntr.List(m.ctx, false)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	cur := m.currentNoteType()
	var items []overlayItem
	add := func(name string, counts bool) {
		it := overlayItem{id: name, label: name, value: name}
		if counts {
			it.desc = "counts as contact"
		}
		items = append(items, it)
	}
	for _, t := range list {
		if t.Name == cur {
			add(t.Name, t.CountsAsContact)
		}
	}
	for _, t := range list {
		if t.Name != cur {
			add(t.Name, t.CountsAsContact)
		}
	}
	m.openPicker("Note type", items, func(m *Model, it overlayItem) {
		m.noteType = it.value
		m.pendingCmd = m.startNotePrompt()
	})
}

// openNoteTypes shows the active note types. Enter on a type archives it, the first row adds one.
func (m *Model) openNoteTypes() tea.Cmd {
	list, err := m.ntr.List(m.ctx, false)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	items := []overlayItem{{id: "+add", label: "+ Add a type", desc: "name, or: name | contact to count it as contact", value: ""}}
	for _, t := range list {
		desc := "enter archives it"
		if t.CountsAsContact {
			desc = "counts as contact, " + desc
		}
		items = append(items, overlayItem{id: t.Name, label: t.Name, value: t.Name, desc: desc})
	}
	o := m.newOverlay("Note types", items, false)
	o.onPick = func(m *Model, it overlayItem) {
		if it.id == "+add" {
			m.startInput("notetype-add", "new note type: ", "")
			m.pendingCmd = m.input.Focus()
			return
		}
		if it.value == "note" {
			m.setStatus("note is the default type and stays", true)
			return
		}
		name := it.value
		m.mode = modeConfirm
		m.confirm = &confirmState{
			prompt: fmt.Sprintf("archive note type %s? old notes keep it. y/n", name),
			yes: func() {
				if err := m.ntr.Archive(m.ctx, name); err != nil {
					m.setStatus(err.Error(), true)
					return
				}
				m.setStatus("archived note type "+name, false)
				m.pendingCmd = m.openNoteTypes()
			},
		}
	}
	return m.showOverlay(o)
}

// addNoteType runs the "new note type" prompt: "name" or "name | contact".
func (m *Model) addNoteType(value string) {
	name, flag, _ := strings.Cut(value, "|")
	counts := strings.EqualFold(strings.TrimSpace(flag), "contact")
	t, err := m.ntr.Add(m.ctx, strings.TrimSpace(name), counts)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus("added note type "+t.Name, false)
	m.pendingCmd = m.openNoteTypes()
}
