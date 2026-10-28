package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Manage deal roles (palette, no key): the TUI path of `crm deal roles`. A picker lists the roles with
// the number of links that use each. The first rows add a role and add the roles already in use. A role
// opens its actions: rename, move up, move down, remove. Each change is on the undo stack, except
// removing a role, which asks first (its links are never lost: they move or keep their text).

// dealRoleNames lists the live deal roles in pick order.
func (m *Model) dealRoleNames() []string {
	names, _ := m.vr.Names(m.ctx, model.DealRoleKind)
	return names
}

// openDealRoles shows the deal roles.
func (m *Model) openDealRoles() tea.Cmd {
	list, err := m.vr.List(m.ctx, model.DealRoleKind, false)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	items := []overlayItem{{id: "+add", label: "+ Add a role", desc: "type a name, for example technical evaluator"}}
	if plan, err := m.vr.BackfillPlan(m.ctx, model.DealRoleKind); err == nil && len(plan.ToAdd) > 0 {
		items = append(items, overlayItem{id: "+backfill", label: fmt.Sprintf("+ Add the %d roles already in use", len(plan.ToAdd)),
			desc: "free-text roles on deal links become listed roles"})
	}
	for i, e := range list {
		use := "not used"
		if e.InUse > 0 {
			use = fmt.Sprintf("%d link(s)", e.InUse)
		}
		items = append(items, overlayItem{id: "role:" + e.Name, label: fmt.Sprintf("%d  %s", i+1, e.Name), value: e.Name, desc: use + " · enter: rename, move or remove"})
	}
	o := m.newOverlay("Deal roles", items, false)
	o.onPick = func(m *Model, it overlayItem) {
		switch it.id {
		case "+add":
			m.startInput("vocab-role-add", "new deal role: ", "")
			m.pendingCmd = m.input.Focus()
		case "+backfill":
			m.backfillDealRoles()
		default:
			m.pendingCmd = m.openDealRoleActions(it.value)
		}
	}
	return m.showOverlay(o)
}

// openDealRoleActions lists what can be done with one role.
func (m *Model) openDealRoleActions(name string) tea.Cmd {
	items := []overlayItem{
		{id: "rename", label: "Rename", desc: "rewrites the deal links that use it"},
		{id: "up", label: "Move up"},
		{id: "down", label: "Move down"},
		{id: "remove", label: "Remove", desc: "links that use it move to another role or keep the text"},
	}
	o := m.newOverlay("Deal role "+name, items, false)
	o.onPick = func(m *Model, it overlayItem) {
		switch it.id {
		case "rename":
			m.vocabName = name
			m.startInput("vocab-role-rename", fmt.Sprintf("rename %s to: ", name), name)
			m.pendingCmd = m.input.Focus()
		case "up", "down":
			m.moveDealRole(name, it.id == "up")
		case "remove":
			m.removeDealRole(name)
		}
	}
	return m.showOverlay(o)
}

// addDealRole is the enter of the "new deal role" prompt.
func (m *Model) addDealRole(value string) {
	m.beginGroup()
	err := m.createVocabValue(model.DealRoleKind, value)
	m.endGroup("")
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus("added deal role "+model.CleanVocabName(value)+" (ctrl+z undoes)", false)
	m.pendingCmd = m.openDealRoles()
}

// renameDealRole is the enter of the rename prompt. The links that used the role follow.
func (m *Model) renameDealRole(to string) {
	from := m.vocabName
	m.vocabName = ""
	res, err := m.vr.Rename(m.ctx, model.DealRoleKind, from, to)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.record("rename deal role "+res.Old+" to "+res.New,
		func() error { _, err := m.vr.Rename(m.ctx, model.DealRoleKind, res.New, res.Old); return err },
		func() error { _, err := m.vr.Rename(m.ctx, model.DealRoleKind, res.Old, res.New); return err })
	m.afterWrite()
	m.setStatus(fmt.Sprintf("renamed deal role %s to %s, %d link(s) updated (ctrl+z undoes)", res.Old, res.New, res.Rows), false)
	m.pendingCmd = m.openDealRoles()
}

// moveDealRole moves a role one place up or down.
func (m *Model) moveDealRole(name string, up bool) {
	before := m.dealRoleNames()
	pos := -1
	for i, n := range before {
		if n == name {
			pos = i + 1
		}
	}
	to := pos + 1
	if up {
		to = pos - 1
	}
	if pos < 0 || to < 1 || to > len(before) {
		m.setStatus(name+" is already there", true)
		return
	}
	if err := m.vr.Move(m.ctx, model.DealRoleKind, name, to); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	after := m.dealRoleNames()
	m.record("move deal role "+name,
		func() error { return m.vr.Reorder(m.ctx, model.DealRoleKind, before) },
		func() error { return m.vr.Reorder(m.ctx, model.DealRoleKind, after) })
	m.setStatus("moved deal role "+name+" (ctrl+z undoes)", false)
	m.pendingCmd = m.openDealRoles()
}

// removeDealRole removes a role. Links that still use it move to a role picked from the list or keep
// the text as plain text.
func (m *Model) removeDealRole(name string) {
	list, err := m.vr.List(m.ctx, model.DealRoleKind, false)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	used := 0
	var others []overlayItem
	for _, e := range list {
		if strings.EqualFold(e.Name, name) {
			used = e.InUse
			continue
		}
		others = append(others, overlayItem{id: "to:" + e.Name, label: e.Name, value: e.Name})
	}
	finish := func(replace string, keep bool) {
		res, err := m.vr.Remove(m.ctx, model.DealRoleKind, name, replace, keep)
		if err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWrite()
		msg := "removed deal role " + res.Name
		switch {
		case res.Moved > 0:
			msg += fmt.Sprintf(", %d link(s) moved to %s", res.Moved, res.ReplacedBy)
		case res.Kept > 0:
			msg += fmt.Sprintf(", %d link(s) keep it as plain text", res.Kept)
		}
		m.setStatus(msg+" (add it again to undo)", false)
		m.pendingCmd = m.openDealRoles()
	}
	if used == 0 {
		m.mode = modeConfirm
		m.confirm = &confirmState{prompt: fmt.Sprintf("remove deal role %s? y/n", name), yes: func() { finish("", false) }}
		return
	}
	items := append([]overlayItem{{id: "keep", label: "Keep the text", desc: fmt.Sprintf("%d link(s) keep %q as plain text", used, name)}}, others...)
	m.openPicker(fmt.Sprintf("%d link(s) use %s: move them to", used, name), items, func(m *Model, it overlayItem) {
		if it.id == "keep" {
			finish("", true)
			return
		}
		finish(it.value, false)
	})
}

// backfillDealRoles adds the roles already on deal links to the list, as one undo step.
func (m *Model) backfillDealRoles() {
	_, ch, err := m.vr.BackfillApply(m.ctx, model.DealRoleKind)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if len(ch.Added) == 0 {
		m.setStatus("every role in use is already on the list", false)
		return
	}
	m.record(fmt.Sprintf("add %d deal roles in use", len(ch.Added)),
		func() error { return m.vr.UndoBackfill(m.ctx, ch) },
		func() error { return m.vr.RedoBackfill(m.ctx, ch) })
	m.setStatus(fmt.Sprintf("added %d deal role(s) from the links (ctrl+z undoes)", len(ch.Added)), false)
	m.pendingCmd = m.openDealRoles()
}

// dealRolesAction is the palette entry.
func dealRolesAction() action {
	return action{id: "deal-roles", title: "Manage deal roles", group: "Contact",
		desc:  "the roles a person has on a deal (champion, decision maker, ...): add, rename, reorder, remove, and add the free-text roles already in use. The deal role cell of a deal's people edits one link",
		write: true, run: func(m *Model) tea.Cmd { return m.openDealRoles() }}
}

