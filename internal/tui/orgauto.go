package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// Company text creates or reuses an organization on its own (owner decision 2026-10-27), so there is
// no review screen. This palette entry is the TUI side of crm org backfill. It has no key. The ignore
// list is the pane of the Ignored company texts row of Settings. See the convert help topic.

func orgAutoActions() []action {
	return []action{
		{id: "org-backfill", title: "Backfill organizations from company text", group: "Organizations",
			desc:  "create or reuse an organization for every person's company text, after one confirm that shows the counts. One undo step, safe to repeat. Same as crm org backfill --apply",
			write: true, run: func(m *Model) tea.Cmd { m.askBackfill(); return nil }},
	}
}

// askBackfill previews the backfill and asks before it writes.
func (m *Model) askBackfill() {
	rep, err := m.oa.BackfillPlan(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if rep.PeopleToLink == 0 {
		m.setStatus("nothing to backfill: "+rep.Line(), false)
		return
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("create %d organizations, reuse %d, link %d people (%d ignored)?", rep.OrgsToCreate, rep.OrgsReused, rep.PeopleToLink, rep.IgnoredPeople),
		yes:    m.runBackfill,
	}
}

// runBackfill writes the backfill and records it as one undo step.
func (m *Model) runBackfill() {
	_, ch, err := m.oa.BackfillApply(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		m.pendingCmd = m.showToast(err.Error(), true)
		return
	}
	if !ch.Empty() {
		m.record(fmt.Sprintf("backfill organizations: %d created, %d people", ch.OrgsCreated, ch.Linked),
			func() error { return m.oa.UndoBackfill(m.ctx, ch) },
			func() error { return m.oa.RedoBackfill(m.ctx, ch) })
	}
	m.afterWrite()
	msg := fmt.Sprintf("created %d organizations, linked %d people", ch.OrgsCreated, ch.Linked)
	m.setStatus(msg+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(msg+" · ctrl+z undoes", false)
}
