package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Archive, restore and purge.

// askArchive archives the person or every selected person. One person asks y/n, several ask for the
// count typed back.
func (m *Model) askArchive() tea.Cmd {
	ids := m.targets()
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 1 {
		word := bulkWord("archive", len(ids))
		return m.askTyped(fmt.Sprintf("archive %d people", len(ids)), word, func() {
			m.bulk(ids, "archived", "archived", func(id int64) error { return m.hArchivePerson(m.ctx, id) })
		})
	}
	what := m.nameOf(ids[0])
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("archive %s? y/n", what),
		yes: func() {
			m.bulk(ids, "archived "+what+" (restore from the Archived view)", "archived", func(id int64) error {
				return m.hArchivePerson(m.ctx, id)
			})
		},
	}
	return nil
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
		r.sub = cliNoun(it.Kind)
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
		case "interaction":
			r.noteID = it.ID
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
	case "interaction":
		return r.noteID
	}
	return r.taskID
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

func sameName(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}
