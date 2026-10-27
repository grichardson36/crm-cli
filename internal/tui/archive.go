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

// Bulk purge (owner decision 2026-10-28). Two palette actions, no bare key: "Purge selected forever"
// for the space-selected rows and "Purge all remaining" for every archived person in the current type
// filter. Both ask for the count typed back ("purge 37") and then write exactly what the single purge
// writes, one person at a time, tombstones included. Only people have a purge, so other archived
// kinds in the list are left alone and the prompt counts people only.

// bulkPurgeWord is what the user types to confirm: "purge N".
func bulkPurgeWord(n int) string { return fmt.Sprintf("purge %d", n) }

// whenBulkPurgeSelected is true in the Archived view while archived people are space-selected.
func whenBulkPurgeSelected(m *Model) bool {
	return m.view == viewArchived && len(m.bulkPurgeSelected()) > 0
}

// whenBulkPurgeAll is true in the Archived view when the type filter can hold people.
func whenBulkPurgeAll(m *Model) bool {
	return m.view == viewArchived && (m.archKind == "" || m.archKind == "person")
}

// bulkPurgeSelected lists the space-selected archived people.
func (m *Model) bulkPurgeSelected() []int64 {
	var ids []int64
	for _, id := range m.markedIDs() {
		if m.arch[id] != nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// bulkPurgeAll lists every archived person in the current type filter.
func (m *Model) bulkPurgeAll() []int64 {
	var ids []int64
	for _, r := range m.rows {
		if r.akind == "person" && r.personID != 0 {
			ids = append(ids, r.personID)
		}
	}
	return ids
}

// askPurgeMany opens the typed confirmation for the selected rows or for all remaining people.
func (m *Model) askPurgeMany(all bool) tea.Cmd {
	ids := m.bulkPurgeSelected()
	what := "selected"
	if all {
		ids, what = m.bulkPurgeAll(), "remaining"
	}
	if len(ids) == 0 {
		m.setStatus("no archived people to purge in this list (only people can be purged)", true)
		return nil
	}
	m.purgeIDs = ids
	m.startInput("purge-bulk", fmt.Sprintf("type %q to delete %d %s people forever: ", bulkPurgeWord(len(ids)), len(ids), what), "")
	return m.input.Focus()
}

// purgeMany deletes the people chosen by askPurgeMany when the typed words match the count.
func (m *Model) purgeMany(typed string) {
	ids := m.purgeIDs
	m.purgeIDs = nil
	if len(ids) == 0 {
		return
	}
	if !sameName(typed, bulkPurgeWord(len(ids))) {
		m.setStatus(fmt.Sprintf("did not match %q, nothing deleted", bulkPurgeWord(len(ids))), true)
		return
	}
	res, err := m.pr.PurgeArchived(m.ctx, repo.PurgeArchivedOptions{Kind: "person", IDs: ids, Apply: true})
	m.marked = map[int64]bool{}
	m.afterWrite()
	switch {
	case err != nil:
		m.setStatus(err.Error(), true)
	case len(res.Errors) > 0:
		m.setStatus(fmt.Sprintf("deleted %d of %d forever, %d failed: %s", res.Purged, res.Count, len(res.Errors), res.Errors[0].Reason), true)
	default:
		m.setStatus(fmt.Sprintf("deleted %d people forever", res.Purged), false)
	}
}
