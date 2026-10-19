package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Organization merge (open discrepancy X18) and the duplicate prompt for new organizations.
// Marks in the Organizations view use the same m.marked map as people (it is cleared when the view
// changes), space marks, M merges the marked organizations.

// errPromptOpened is returned by a save that opened a follow-up prompt instead of finishing.
var errPromptOpened = errors.New("prompt opened")

func whenOrgsList(m *Model) bool { return m.view == viewOrgs && m.focus == focusList }

// markedOrgIDs lists the marked organizations in list order.
func (m *Model) markedOrgIDs() []int64 {
	if m.view != viewOrgs || len(m.marked) == 0 {
		return nil
	}
	var ids []int64
	for _, r := range m.rows {
		if r.orgID != 0 && m.marked[r.orgID] {
			ids = append(ids, r.orgID)
		}
	}
	return ids
}

func whenOrgMarks(m *Model) bool { return whenOrgsList(m) && len(m.markedOrgIDs()) >= 2 }

func (m *Model) toggleOrgMark() {
	id := m.selOrg()
	if id == 0 {
		return
	}
	if m.marked[id] {
		delete(m.marked, id)
	} else {
		m.marked[id] = true
	}
	m.move(1)
}

func orgMergeActions() []action {
	return []action{
		{id: "org-mark", title: "Mark organization", group: "Organizations", hint: "space", keys: []string{" "},
			desc: "mark or unmark the organization, then M merges the marked ones",
			foot: 25, footIn: inList, footLabel: "mark", when: func(m *Model) bool { return whenOrgsList(m) && m.selOrg() != 0 },
			run: func(m *Model) tea.Cmd { m.toggleOrgMark(); return nil }},
		{id: "org-merge", title: "Merge marked organizations", group: "Organizations", hint: "M", keys: []string{"M"},
			desc: "merge the marked organizations into one you pick: people, parent links, deals, notes and tags move, the others are archived and ctrl+z undoes it. Same as crm org merge",
			write: true, foot: 34, footIn: inList, footLabel: "merge", footIf: func(m *Model) bool { return len(m.markedOrgIDs()) >= 2 },
			when: whenOrgsList, run: func(m *Model) tea.Cmd { m.askOrgMerge(); return nil }},
	}
}

// askOrgMerge asks which marked organization survives, then shows the preview and confirms.
func (m *Model) askOrgMerge() {
	ids := m.markedOrgIDs()
	if len(ids) < 2 {
		m.setStatus("mark two or more organizations with space, then M merges them", true)
		return
	}
	var items []overlayItem
	for _, id := range ids {
		o, err := m.orr.FindByID(m.ctx, id)
		if err != nil {
			continue
		}
		items = append(items, overlayItem{id: "pick", label: cleanCell(o.Name), desc: cleanCell(str(o.Domain)), ref: o.ID, value: o.Name})
	}
	m.openPicker("Keep which organization?", items, func(m *Model, it overlayItem) {
		var drops []int64
		for _, id := range ids {
			if id != it.ref {
				drops = append(drops, id)
			}
		}
		m.confirmOrgMerge(it.ref, drops)
	})
}

func (m *Model) confirmOrgMerge(keep int64, drops []int64) {
	res, err := m.orr.MergeOrgsDryRun(m.ctx, keep, drops)
	if err != nil {
		m.setStatus(err.Error(), true)
		m.pendingCmd = m.showToast(err.Error(), true)
		return
	}
	var parts []string
	for _, k := range []string{"memberships", "relations", "deals", "tasks", "interactions", "tags"} {
		if n := res.Moved[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if n := res.Moved["duplicates_dropped"]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d duplicate rows dropped", n))
	}
	moved := "nothing to move"
	if len(parts) > 0 {
		moved = strings.Join(parts, ", ")
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("merge %s into %s (%s)? y/n", strings.Join(res.DropNames, ", "), res.KeepName, moved),
		yes:    func() { m.runOrgMerge(keep, drops) },
	}
}

func (m *Model) runOrgMerge(keep int64, drops []int64) {
	res, err := m.orr.MergeOrgs(m.ctx, keep, drops)
	if err != nil {
		m.setStatus(err.Error(), true)
		m.pendingCmd = m.showToast(err.Error(), true)
		return
	}
	m.record(res.Label(),
		func() error { return m.orr.UndoMerge(m.ctx, res) },
		func() error { return m.orr.RedoMerge(m.ctx, res) })
	m.marked = map[int64]bool{}
	m.afterWrite()
	m.cursorOn(func(r row) bool { return r.orgID == keep })
	msg := fmt.Sprintf("merged %d organizations into %s", len(drops), res.KeepName)
	m.setStatus(msg+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(msg+" · ctrl+z undoes", false)
}

// askOrgDuplicate opens the pick-or-create prompt for a new organization that matches a live one:
// use the existing one, create anyway, or cancel. Esc cancels.
func (m *Model) askOrgDuplicate(dup *model.Organization, reason string, use func(o *model.Organization), anyway func()) {
	items := []overlayItem{
		{id: "use", label: "Use " + cleanCell(dup.Name), desc: fmt.Sprintf("#%d has the same %s", dup.ID, reason), ref: dup.ID},
		{id: "anyway", label: "Create anyway", desc: "a second organization with the same " + reason},
		{id: "cancel", label: "Cancel"},
	}
	m.openPicker("Organization already exists", items, func(m *Model, it overlayItem) {
		switch it.id {
		case "use":
			use(dup)
		case "anyway":
			anyway()
		}
	})
}
