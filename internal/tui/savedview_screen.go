package tui

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/query"
)

// The saved views in the Views popup (v). A saved view is listed like a built-in one: shown ones in
// tab order after the built-in views, the line, the hidden ones in gray, and below them the deleted
// views. The popup opens a view, moves it (across the line shows or hides) and filters by typing.
// A saved view is edited, deleted (asks y or n) and restored in the Settings Views pane (settingspane.go).

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

// savedViewsKey handles the keys of the Views list on a saved view. It returns true when it used the key.
// The popup only moves a view (shift+up and shift+down), plain letters are its filter. The Settings
// pane (pane true) has no filter, so e edits, d deletes and r restores. after redraws the list the
// key was used in once a delete is confirmed.
func (m *Model) savedViewsKey(o *overlay, key, cur string, pane bool, focus func(string), after func()) bool {
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
	act := ""
	switch key {
	case "shift+up", "shift+down":
		act = key
	case "e", "d", "r":
		if pane {
			act = key
		}
	}
	if act == "" {
		return false
	}
	if m.readOnly {
		return fail(fmt.Errorf("read-only: changing views is disabled"))
	}
	switch act {
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
	case "e":
		if !isLive {
			return fail(fmt.Errorf("restore the view first (r)"))
		}
		m.pendingCmd = m.openSVEditor(live.ID)
		return true
	case "d":
		if !isLive {
			return true
		}
		m.mode = modeConfirm
		m.confirm = &confirmState{
			prompt: fmt.Sprintf("delete view %q?", truncateLabel(live.Name)),
			yes: func() {
				if err := m.hDeleteSaved(live.ID); err != nil {
					m.setStatus(err.Error(), true)
					return
				}
				m.setStatus("view deleted (ctrl+z undoes, r restores)", false)
				after()
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
