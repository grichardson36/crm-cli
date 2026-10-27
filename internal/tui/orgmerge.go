package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Organization merge (open discrepancy X18) and the duplicate prompt for new organizations.
// Selected organizations use the same m.marked set as every other object (selection.go): space or
// shift+arrows select, m merges the selected organizations.

// errPromptOpened is returned by a save that opened a follow-up prompt instead of finishing.
var errPromptOpened = errors.New("prompt opened")

func whenOrgsList(m *Model) bool { return m.view == viewOrgs && m.focus == focusList }

// markedOrgIDs lists the selected organizations in list order.
func (m *Model) markedOrgIDs() []int64 { return m.markedOfKind("organization") }

func whenOrgMarks(m *Model) bool { return whenOrgsList(m) && len(m.markedOrgIDs()) >= 2 }

func orgMergeActions() []action {
	return []action{
		{id: "org-merge", title: "Merge marked organizations", group: "Organizations",
			desc:  "merge the marked organizations into one you pick: people, parent links, deals, notes and tags move, the others are archived and ctrl+z undoes it. Same as crm org merge",
			write: true,
			when:  whenOrgsList, run: func(m *Model) tea.Cmd { m.askOrgMerge(); return nil }},
		// Palette only, no key (the owner is redesigning keys).
		{id: "org-not-duplicate", title: "Not a duplicate (organization)", group: "Organizations",
			desc:  "tell the program the selected organization and the one with a close name are different organizations: the pair stops showing as a merge candidate. Stored as data, ctrl+z undoes it. Same as crm org dupes dismiss",
			write: true, when: func(m *Model) bool { return whenOrgsList(m) && len(m.similarOrgs()[m.selOrg()]) > 0 },
			run: func(m *Model) tea.Cmd { m.askNotDuplicate(); return nil }},
		{id: "org-undismiss", title: "Show dismissed organization pairs", group: "Organizations",
			desc:  "list the organization pairs marked as not duplicates and pick one to make a merge candidate again. Same as crm org dupes undismiss",
			write: true, when: whenOrgsList, run: func(m *Model) tea.Cmd { m.askUndismissOrgs(); return nil }},
	}
}

// askNotDuplicate dismisses the selected organization against the one it looks like. When it looks
// like several, a picker asks which.
func (m *Model) askNotDuplicate() {
	id := m.selOrg()
	near := m.similarOrgs()[id]
	switch len(near) {
	case 0:
		m.setStatus("this organization has no similar one", true)
	case 1:
		m.dismissOrgPair(id, near[0].id)
	default:
		items := make([]overlayItem, 0, len(near))
		for _, n := range near {
			items = append(items, overlayItem{id: "pick", label: cleanCell(n.name), desc: n.reason, ref: n.id})
		}
		m.openPicker("Not a duplicate of which organization?", items, func(m *Model, it overlayItem) { m.dismissOrgPair(id, it.ref) })
	}
}

// dismissOrgPair marks a pair as not duplicates and records it for undo and redo.
func (m *Model) dismissOrgPair(a, b int64) {
	changed, err := m.orr.DismissOrgPair(m.ctx, a, b)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	label := "not a duplicate: " + m.orgName(a) + " and " + m.orgName(b)
	if changed {
		m.record(truncateLabel(label),
			func() error { return m.orr.UndismissOrgPair(m.ctx, a, b) },
			func() error { _, err := m.orr.DismissOrgPair(m.ctx, a, b); return err })
	}
	m.afterWrite()
	m.setStatus(label+" (ctrl+z undoes)", false)
}

// askUndismissOrgs lists the dismissed pairs and brings the picked one back as a candidate.
func (m *Model) askUndismissOrgs() {
	pairs, err := m.orr.DismissedOrgCandidates(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if len(pairs) == 0 {
		m.setStatus("no organization pairs are dismissed", false)
		return
	}
	items := make([]overlayItem, 0, len(pairs))
	for i, p := range pairs {
		items = append(items, overlayItem{id: fmt.Sprintf("pair:%d", i), label: cleanCell(p.AName) + " | " + cleanCell(p.BName), desc: p.Reason, ref: int64(i)})
	}
	m.openPicker("Make a pair a merge candidate again", items, func(m *Model, it overlayItem) {
		p := pairs[it.ref]
		if err := m.orr.UndismissOrgPair(m.ctx, p.AID, p.BID); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.record(truncateLabel("show again: "+p.AName+" and "+p.BName),
			func() error { _, err := m.orr.DismissOrgPair(m.ctx, p.AID, p.BID); return err },
			func() error { return m.orr.UndismissOrgPair(m.ctx, p.AID, p.BID) })
		m.afterWrite()
		m.setStatus(p.AName+" and "+p.BName+" are merge candidates again (ctrl+z undoes)", false)
	})
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
	m.recordMerge(res.LogID)
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
