package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
)

// Company text creates or reuses an organization on its own (owner decision 2026-10-27), so there is
// no review screen. These palette entries are the TUI side of crm org backfill and crm org ignore. They
// have no keys. See the convert help topic.

func orgAutoActions() []action {
	return []action{
		{id: "org-backfill", title: "Backfill organizations from company text", group: "Organizations",
			desc:  "create or reuse an organization for every person's company text, after one confirm that shows the counts. One undo step, safe to repeat. Same as crm org backfill --apply",
			write: true, run: func(m *Model) tea.Cmd { m.askBackfill(); return nil }},
		{id: "org-ignore", title: "Ignored company texts", group: "Organizations",
			desc:  "the strings that never create an organization (N/A, none, retired ...): enter turns one off or on, type a new text to ignore it. Same as crm org ignore",
			write: true, run: func(m *Model) tea.Cmd { m.openIgnoreList(); return nil }},
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
		prompt: fmt.Sprintf("create %d organizations, reuse %d, link %d people (%d ignored)? y/n", rep.OrgsToCreate, rep.OrgsReused, rep.PeopleToLink, rep.IgnoredPeople),
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

// openIgnoreList shows the ignore list. Enter on a line turns it off or on, a typed text is added.
func (m *Model) openIgnoreList() {
	ig := repo.NewIgnoreRepo(m.db)
	list, err := ig.List(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	var items []overlayItem
	for _, e := range list {
		state, src := "ignored", "added"
		if !e.Active {
			state = "off"
		}
		if e.Builtin {
			src = "built-in"
		}
		items = append(items, overlayItem{id: "pick", label: cleanCell(e.Text), desc: state + ", " + src, value: e.Text, ref: boolRef(e.Active)})
	}
	m.openPicker("Ignored company texts", items, func(m *Model, it overlayItem) {
		if it.id == "create" {
			m.setIgnore(it.value, true)
			return
		}
		m.setIgnore(it.value, it.ref == 0) // an active entry turns off, an off one turns on
	})
	m.ov.create = func(q string) (overlayItem, bool) {
		q = strings.TrimSpace(q)
		if q == "" {
			return overlayItem{}, false
		}
		return overlayItem{id: "create", label: fmt.Sprintf("Ignore %q", q), desc: "never creates an organization", value: q}, true
	}
	m.ov.refilter()
}

func boolRef(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// setIgnore turns a text on or off the ignore list and records it for undo.
func (m *Model) setIgnore(text string, on bool) {
	ig := repo.NewIgnoreRepo(m.db)
	var err error
	if on {
		_, err = ig.Add(m.ctx, text)
	} else {
		err = ig.Remove(m.ctx, text)
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	verb := map[bool]string{true: "ignore", false: "stop ignoring"}[on]
	m.record(verb+" company text "+cleanCell(text),
		func() error {
			if on {
				return ig.Remove(m.ctx, text)
			}
			_, err := ig.Add(m.ctx, text)
			return err
		},
		func() error {
			if on {
				_, err := ig.Add(m.ctx, text)
				return err
			}
			return ig.Remove(m.ctx, text)
		})
	m.setStatus(verb+" "+cleanCell(text)+" (ctrl+z undoes)", false)
}
