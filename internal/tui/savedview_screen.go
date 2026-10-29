package tui

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/query"
)

// The saved views in the Views screen (v). A saved view is listed like a built-in one: shown ones in
// tab order after the built-in views, a Hidden group, and below them the deleted views. The keys are the
// ones of the screen: enter opens, x shows or hides, shift+up and shift+down move, and for a saved
// view e edits, d deletes (asks y or n) and r restores a deleted one.

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
	return overlayItem{id: "saved:" + v.UUID, label: label, value: savedPrefix + v.UUID, desc: desc}
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
	if m.readOnly && (key == "x" || key == "e" || key == "d" || key == "r" || key == "shift+up" || key == "shift+down") {
		return fail(fmt.Errorf("read-only: changing views is disabled"))
	}
	switch key {
	case "x":
		if !isLive {
			return fail(fmt.Errorf("restore the view first (r)"))
		}
		on := !live.ShowInTabs
		if _, err := m.hUpdateSaved(live.ID, repo.SavedViewPatch{ShowInTabs: &on}); err != nil {
			return fail(err)
		}
		refresh()
		return true
	case "shift+up", "shift+down":
		if !isLive {
			return true
		}
		d := -1
		if key == "shift+down" {
			d = 1
		}
		m.svMove(live.ID, d)
		refresh()
		return true
	case "e":
		if !isLive {
			return fail(fmt.Errorf("restore the view first (r)"))
		}
		m.closeOverlay()
		m.pendingCmd = m.openSVEditor(live.ID)
		return true
	case "d":
		if !isLive {
			return true
		}
		m.closeOverlay()
		m.mode = modeConfirm
		m.confirm = &confirmState{
			prompt: fmt.Sprintf("delete view %q? y/n", truncateLabel(live.Name)),
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
	case "r":
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
