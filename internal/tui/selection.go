package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
)

// Selection and bulk actions (patterns/bulk-actions-always). Every verb acts on the selected rows when
// rows are selected and on the highlighted row otherwise, for every object type. The selection is the
// m.marked set. The keys are space (toggle a row), shift+up and shift+down (extend the selection by a
// row, shift+up back over a row takes it out again) and ctrl+a (select every visible row). The view's
// sort and filter decide what is visible, so there is no "select all matching".

// groupKind is the pseudo kind of a row of the Merge view: a group of duplicate candidates.
const groupKind = "group"

// selStep keeps the keys of different kinds apart in one set. The primary kind of a view (people in the
// People view, tasks in the Tasks view) is stored as its plain id, every other kind as base*selStep+id.
const selStep = int64(1) << 40

func kindBase(kind string) int64 {
	switch kind {
	case "organization":
		return 1
	case "deal":
		return 2
	case "task":
		return 3
	case "interaction":
		return 4
	case groupKind:
		return 5
	}
	return 0
}

// primaryKind is the kind whose ids are stored plain in the selection of the current view.
func (m *Model) primaryKind() string {
	switch m.view {
	case viewTasks:
		return "task"
	case viewOrgs:
		return "organization"
	case viewDeals:
		return "deal"
	case viewMerge:
		return groupKind
	case viewTags:
		if m.onTagPage() {
			return "person"
		}
		return tagKind
	}
	return "person"
}

// selKey is the key of an object in m.marked.
func (m *Model) selKey(ref repo.Ref) int64 {
	if ref.Kind == m.primaryKind() || (m.view == viewArchived && ref.Kind == "person") {
		return ref.ID
	}
	return kindBase(ref.Kind)*selStep + ref.ID
}

// rowRef is the object a list row stands for in the current view, if it stands for one.
func (m *Model) rowRef(r row) (repo.Ref, bool) {
	if r.skip {
		return repo.Ref{}, false
	}
	switch m.view {
	case viewTasks:
		if r.taskID != 0 {
			return repo.Ref{Kind: "task", ID: r.taskID}, true
		}
	case viewOrgs:
		if r.orgID != 0 {
			return repo.Ref{Kind: "organization", ID: r.orgID}, true
		}
	case viewDeals:
		if r.dealID != 0 {
			return repo.Ref{Kind: "deal", ID: r.dealID}, true
		}
	case viewMerge:
		if r.dupe != 0 {
			return repo.Ref{Kind: groupKind, ID: int64(r.dupe)}, true
		}
	case viewArchived:
		if r.akind != "" {
			if id := archivedRowID(r); id != 0 {
				return repo.Ref{Kind: r.akind, ID: id}, true
			}
		}
	case viewTags:
		return m.tagRowRef(r)
	case viewSync:
	default:
		if r.personID != 0 {
			return repo.Ref{Kind: "person", ID: r.personID}, true
		}
	}
	return repo.Ref{}, false
}

// cursorRef is the highlighted object of the list.
func (m *Model) cursorRef() (repo.Ref, bool) {
	r, ok := m.selected()
	if !ok {
		return repo.Ref{}, false
	}
	return m.rowRef(r)
}

// markedRefs lists the selected objects in list order.
func (m *Model) markedRefs() []repo.Ref {
	if len(m.marked) == 0 {
		return nil
	}
	var out []repo.Ref
	seen := map[int64]bool{}
	for _, r := range m.rows {
		ref, ok := m.rowRef(r)
		if !ok {
			continue
		}
		k := m.selKey(ref)
		if m.marked[k] && !seen[k] {
			seen[k] = true
			out = append(out, ref)
		}
	}
	return out
}

// bulkRefs are the objects a verb acts on: the selected ones, else the highlighted one. Selected rows
// only count in the list pane.
func (m *Model) bulkRefs() []repo.Ref {
	if m.focus == focusList {
		if refs := m.markedRefs(); len(refs) > 0 {
			return refs
		}
		if ref, ok := m.cursorRef(); ok {
			return []repo.Ref{ref}
		}
	}
	return nil
}

// bulkOfKind is bulkRefs limited to one kind, as ids.
func (m *Model) bulkOfKind(kind string) []int64 {
	var ids []int64
	for _, r := range m.bulkRefs() {
		if r.Kind == kind {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// markedOfKind lists the selected objects of one kind, as ids.
func (m *Model) markedOfKind(kind string) []int64 {
	var ids []int64
	for _, r := range m.markedRefs() {
		if r.Kind == kind {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

func whenSelectable(m *Model) bool {
	_, ok := m.cursorRef()
	return m.focus == focusList && ok
}

// toggleMark is space: select or unselect the highlighted row and move down.
func (m *Model) toggleMark() {
	ref, ok := m.cursorRef()
	if !ok {
		return
	}
	k := m.selKey(ref)
	if m.marked[k] {
		delete(m.marked, k)
	} else {
		m.marked[k] = true
	}
	m.move(1)
}

// selectAll is ctrl+a: select every visible row, or clear the selection when every visible row is selected.
func (m *Model) selectAll() {
	all := map[int64]bool{}
	for _, r := range m.rows {
		if ref, ok := m.rowRef(r); ok {
			all[m.selKey(ref)] = true
		}
	}
	if len(all) > 0 && len(m.markedRefs()) == len(all) {
		m.marked = map[int64]bool{}
		return
	}
	m.marked = all
}

// extendState remembers an extension of the selection that is in progress: the row it started on and
// the selection before it began, so shift+up after shift+down takes the rows back out.
type extendState struct {
	anchor int
	base   map[int64]bool
}

// extendSelection is shift+up and shift+down: select the rows between where the extension began and
// the cursor, which moves one row.
func (m *Model) extendSelection(dir int) {
	if _, ok := m.cursorRef(); !ok {
		return
	}
	if m.extend == nil {
		base := map[int64]bool{}
		for k := range m.marked {
			base[k] = true
		}
		m.extend = &extendState{anchor: m.cursor, base: base}
	}
	m.move(dir)
	lo, hi := m.extend.anchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	marked := map[int64]bool{}
	for k := range m.extend.base {
		marked[k] = true
	}
	for i := lo; i <= hi && i < len(m.rows); i++ {
		if ref, ok := m.rowRef(m.rows[i]); ok {
			marked[m.selKey(ref)] = true
		}
	}
	m.marked = marked
}

// selectionActions are the framework actions of the selection.
func selectionActions() []action {
	return []action{
		setFieldAction(),
		{id: "mark", title: "Select row", group: "Bulk", hint: "space", keys: []string{" "},
			desc: "select or unselect the row, then every verb (d, x, f, l, u, r, m, the palette) acts on all selected rows. With nothing selected a verb acts on the highlighted row",
			foot: 25, footIn: inList, footLabel: "select", when: whenSelectable, run: func(m *Model) tea.Cmd { m.toggleMark(); return nil }},
		{id: "select-all", title: "Select all visible", group: "Bulk", hint: "ctrl+a", keys: []string{"ctrl+a"},
			desc: "select every visible row, again to clear. The view's sort and filter (ctrl+o) decide what is visible",
			when: whenList, run: func(m *Model) tea.Cmd { m.selectAll(); return nil }},
		{id: "extend-down", title: "Extend selection down", group: "Bulk", hint: "shift+down", keys: []string{"shift+down"},
			desc: "select the highlighted row and the one below, again to extend by one more row (shift+up takes rows back out)",
			when: whenSelectable, run: func(m *Model) tea.Cmd { m.extendSelection(1); return nil }},
		{id: "extend-up", title: "Extend selection up", group: "Bulk", hint: "shift+up", keys: []string{"shift+up"},
			desc: "select the highlighted row and the one above, again to extend by one more row (shift+down takes rows back out)",
			when: whenSelectable, run: func(m *Model) tea.Cmd { m.extendSelection(-1); return nil }},
	}
}
