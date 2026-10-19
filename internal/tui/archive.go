package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
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
				return m.hArchivePerson(m.ctx, id)
			})
		},
	}
}

// archivedRows lists every archived object, or one kind when the type filter is set.
func (m *Model) archivedRows() ([]row, error) {
	items, err := repo.ListArchived(m.ctx, m.db, m.archKind)
	if err != nil {
		return nil, err
	}
	m.arch = map[int64]*model.Person{}
	m.archItems = map[string]repo.ArchivedItem{}
	if m.archKind == "" || m.archKind == "person" {
		people, err := m.pr.FindAll(m.ctx, model.PersonFilters{Archived: true})
		if err != nil {
			return nil, err
		}
		for _, p := range people {
			m.arch[p.ID] = p
		}
	}
	var rows []row
	for _, it := range items {
		m.archItems[fmt.Sprintf("%s:%d", it.Kind, it.ID)] = it
		r := row{akind: it.Kind, title: cleanCell(it.Name), meta: dateOnly(it.UpdatedAt)}
		r.sub = it.Kind
		if d := cleanCell(it.Detail); d != "" {
			r.sub += "  " + d
		}
		switch it.Kind {
		case "person":
			r.personID = it.ID
		case "organization":
			r.orgID = it.ID
		case "deal":
			r.dealID = it.ID
		case "task":
			r.taskID = it.ID
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// archivedRowID is the id of the object under the cursor in the Archived view.
func archivedRowID(r row) int64 {
	switch r.akind {
	case "person":
		return r.personID
	case "organization":
		return r.orgID
	case "deal":
		return r.dealID
	}
	return r.taskID
}

// pickArchivedType is the type filter of the Archived view.
func (m *Model) pickArchivedType() {
	types := []struct{ kind, label string }{{"", "all types"}, {"person", "people"}, {"organization", "organizations"}, {"deal", "deals"}, {"task", "tasks"}}
	var items []overlayItem
	for i, t := range types {
		it := overlayItem{id: "arch-" + t.kind, label: t.label, value: t.kind, hint: fmt.Sprint(i + 1)}
		if t.kind == m.archKind {
			it.desc = "showing now"
		}
		items = append(items, it)
	}
	m.openPicker("Archived: show", items, func(m *Model, it overlayItem) {
		m.archKind = it.value
		m.report(m.rebuild(false))
		m.setStatus("archived: "+it.label, false)
	})
	m.ov.digits = true
}

// restoreArchivedObject restores the archived organization, deal or task under the cursor. People
// use restoreTargets (they can be marked and restored together).
func (m *Model) restoreArchivedObject(r row) {
	o := objectByKind(r.akind)
	if o == nil {
		return
	}
	id := archivedRowID(r)
	name := truncateLabel(m.archItems[fmt.Sprintf("%s:%d", r.akind, id)].Name)
	if err := o.restore(m, id); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus("restored "+o.label+" "+name+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast("restored "+o.label+" "+name+" · ctrl+z undoes", false)
}

// restoreTargets restores the archived object under the cursor, or every marked person.
func (m *Model) restoreTargets() {
	if r, ok := m.selected(); ok && m.view == viewArchived && r.akind != "" && r.akind != "person" {
		m.restoreArchivedObject(r)
		return
	}
	ids := m.targets()
	if len(ids) == 0 {
		return
	}
	m.bulk(ids, "restored "+m.nameOf(ids[0]), "restored", func(id int64) error {
		return m.hRestorePerson(m.ctx, id)
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
