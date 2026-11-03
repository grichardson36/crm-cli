package tui

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/query"
)

// The saved views in the Views popup (v). A saved view is listed like a built-in one: shown ones in
// tab order after the built-in views, the line, the hidden ones in gray, and below them the deleted
// views. The keys are the ones of the popup: enter opens, shift+up and shift+down move (across the
// line shows or hides), and for a saved view ctrl+e edits, ctrl+d deletes (asks y or n) and ctrl+r
// restores a deleted one. Plain letters are the filter, so these three use ctrl.

const savedPrefix = "saved:"

// savedItem is the row of a saved view in the Views screen.
func (m *Model) savedItem(v repo.SavedView) overlayItem {
	label := v.Name
	if v.Example {
		label += " (example)"
	}
	desc := v.Kind + " view: " + m.svr.Describe(m.ctx, &v)
	if m.sv != nil && m.sv.v.UUID == v.UUID {
		desc = "current view. " + desc
	}
	return overlayItem{id: "saved:" + v.UUID, label: label, value: savedPrefix + v.UUID, desc: desc, isCur: m.sv != nil && m.sv.v.UUID == v.UUID}
}

// savedViewsKey handles the keys of the Views screen on a saved view. It returns true when it used the key.
func (m *Model) savedViewsKey(o *overlay, key, cur string, focus func(string)) bool {
	uuid := strings.TrimPrefix(cur, savedPrefix)
	live, isLive := m.savedByUUID(uuid)
	var dead repo.SavedView
	isDead := false
	for _, v := range m.svDeleted {
		if v.UUID == uuid {
			dead, isDead = v, true
		}
	}
	refresh := func() {
		o.items = m.viewItems()
		o.refilter()
		focus(cur)
	}
	fail := func(err error) bool {
		m.setStatus(err.Error(), true)
		m.pendingCmd = m.showToast(err.Error(), true)
		return true
	}
	switch key {
	case "ctrl+e", "ctrl+d", "ctrl+r", "shift+up", "shift+down":
		if m.readOnly {
			return fail(fmt.Errorf("read-only: changing views is disabled"))
		}
	}
	switch key {
	case "shift+up", "shift+down":
		if !isLive || o.filter.Value() != "" {
			return true
		}
		d := -1
		if key == "shift+down" {
			d = 1
		}
		if err := m.svShift(live, d); err != nil {
			return fail(err)
		}
		refresh()
		return true
	case "ctrl+e":
		if !isLive {
			return fail(fmt.Errorf("restore the view first (ctrl+r)"))
		}
		m.closeOverlay()
		m.pendingCmd = m.openSVEditor(live.ID)
		return true
	case "ctrl+d":
		if !isLive {
			return true
		}
		m.closeOverlay()
		m.mode = modeConfirm
		m.confirm = &confirmState{
			prompt: fmt.Sprintf("delete view %q?", truncateLabel(live.Name)),
			yes: func() {
				if err := m.hDeleteSaved(live.ID); err != nil {
					m.setStatus(err.Error(), true)
					return
				}
				m.setStatus("view deleted (ctrl+z undoes, v then r restores)", false)
				m.pendingCmd = m.showViews("")
			},
		}
		return true
	case "ctrl+r":
		if !isDead {
			return true
		}
		if err := m.hRestoreSaved(dead.ID); err != nil {
			return fail(err)
		}
		m.setStatus(fmt.Sprintf("view %q restored", dead.Name), false)
		cur = savedPrefix + dead.UUID
		refresh()
		return true
	}
	return false
}

// describeFor is the filter of a saved view in words for the editor's title.
func describeFor(kind string, q query.Query) string {
	return query.Describe(kind, q, nil)
}
