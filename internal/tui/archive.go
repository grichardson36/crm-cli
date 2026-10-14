package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Archive, restore and purge.

// askArchive confirms, then archives the person or every marked person.
func (m *Model) askArchive() {
	ids := m.targets()
	if len(ids) == 0 {
		return
	}
	what := m.nameOf(ids[0])
	if len(ids) > 1 {
		what = fmt.Sprintf("%d people", len(ids))
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("archive %s? y/n", what),
		yes: func() {
			m.bulk(ids, "archived "+what+" (restore from the Archived view)", "archived", func(id int64) error {
				return m.pr.Archive(m.ctx, id)
			})
		},
	}
}

// restoreTargets restores the archived person or every marked one.
func (m *Model) restoreTargets() {
	ids := m.targets()
	if len(ids) == 0 {
		return
	}
	m.bulk(ids, "restored "+m.nameOf(ids[0]), "restored", func(id int64) error {
		return m.pr.Restore(m.ctx, id)
	})
}

// askPurge asks for the person's name before deleting them for good.
func (m *Model) askPurge() tea.Cmd {
	id := m.curPerson()
	p := m.arch[id]
	if p == nil {
		return nil
	}
	m.startInput("purge", fmt.Sprintf("type %q to delete forever: ", fullName(p)), "")
	return m.input.Focus()
}

func sameName(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

// purgeCurrent deletes the selected archived person when the typed name matches.
func (m *Model) purgeCurrent(typed string) {
	id := m.curPerson()
	p := m.arch[id]
	if p == nil {
		m.setStatus("select an archived person", true)
		return
	}
	name := fullName(p)
	if !sameName(typed, name) {
		m.setStatus("name did not match, nothing deleted", true)
		return
	}
	if err := m.pr.Purge(m.ctx, id); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus("deleted "+name+" forever", false)
}
