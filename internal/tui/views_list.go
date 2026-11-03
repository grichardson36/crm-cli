package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/modules"
)

// The Views popup (v) is the one place that lists every view and orders it. The list is the shown
// views numbered 1 to 0, a horizontal line, and the hidden views in gray below the line. shift+up and
// shift+down move the highlighted view. A view that crosses the line is hidden or shown, so there is
// no separate hide key. Typing filters the list by name, enter opens the highlighted view, shift+r
// puts the default order and hidden set back, esc closes. The number key of a view is its place among
// the shown views. The data is the [modules] table of config.toml and the saved views' ShowInTabs.
// The screen itself is not a module and Settings cannot be hidden, so the user can always get back to both.

// viewItems lists every module as the Views list shows it: shown views in order, the line, the hidden
// views, and the deleted saved views under their own heading.
func (m *Model) viewItems() []overlayItem {
	arr := m.mods.Normalize()
	var shown, hidden []overlayItem
	for _, id := range arr.Order {
		mod, _ := modules.Lookup(id)
		v := moduleViews[id]
		it := overlayItem{id: "view:" + id, label: mod.Name, value: id, desc: mod.Blurb}
		it.isCur = v == m.view && m.sv == nil
		if pos := arr.Position(id); pos > 0 {
			it.hint = modules.Key(pos)
			if a := m.actionByID(viewActionID(v)); a != nil && len(a.keys) > 0 {
				it.hint = keys.Display(a.keys[0])
			}
			shown = append(shown, it)
		} else {
			it.dim = true
			hidden = append(hidden, it)
		}
	}
	// The saved views follow the built-in ones, shown first, then hidden (patterns/saved-views).
	nShown := 0
	for _, v := range m.svs {
		it := m.savedItem(v)
		if v.ShowInTabs {
			nShown++
			it.hint = modules.Key(len(arr.Visible()) + nShown)
			shown = append(shown, it)
		} else {
			it.dim = true
			hidden = append(hidden, it)
		}
	}
	items := shown
	items = append(items, overlayItem{id: "divider:hidden", divider: true})
	items = append(items, hidden...)
	if len(m.svDeleted) > 0 {
		items = append(items, overlayItem{id: "divider:deleted", label: "Deleted views", desc: fmt.Sprint(len(m.svDeleted)), divider: true})
		for _, v := range m.svDeleted {
			it := m.savedItem(v)
			it.disabled = ""
			it.dim = true
			items = append(items, it)
		}
	}
	return items
}

// openViews shows the Views popup.
func (m *Model) openViews() tea.Cmd { return m.showViews("") }

func (m *Model) showViews(focusID string) tea.Cmd { return m.showOverlay(m.newViewsList(focusID)) }

// viewsHints are the keys that work on the highlighted row of the Views list, one word each. The
// popup (pane false) has a filter, so it only opens and moves. The Settings pane (pane true) also
// edits, deletes and restores saved views.
func (m *Model) viewsHints(o *overlay, pane bool) []hintItem {
	it, ok := o.highlighted()
	if !ok {
		return []hintItem{fk("esc", "close")}
	}
	typed := o.filter.Value() != ""
	h := []hintItem{fk("up/down", "select")}
	dead := false
	if uuid, saved := strings.CutPrefix(it.value, savedPrefix); saved {
		_, live := m.savedByUUID(uuid)
		dead = !live
		switch {
		case dead && pane && !m.readOnly:
			h = append(h, fk("r", "restore"))
		case dead:
		case pane && !m.readOnly:
			h = append(h, fk("enter", "open"), fk("e", "edit"), fk("d", "delete"))
		default:
			h = append(h, fk("enter", "open"))
		}
	} else {
		h = append(h, fk("enter", "open"))
	}
	if !typed && !dead {
		h = append(h, fk("shift+up/down", "move"))
	}
	return append(h, fk("shift+r", "reset"), fk("esc", "close"))
}

// newViewsList is the Views popup list as an overlay that is not shown yet.
func (m *Model) newViewsList(focusID string) *overlay { return m.newViewsListFor(focusID, false) }

// newViewsListFor builds the Views list. The popup (pane false) shows it with a filter line. The
// Settings pane (pane true) embeds the same overlay with no filter: draw its rows with popupListRows,
// give it keys through onKey and onPick, and the bare keys e, d and r work on saved views.
func (m *Model) newViewsListFor(focusID string, pane bool) *overlay {
	o := m.newOverlay("Views", m.viewItems(), false)
	o.nameOnly = true
	o.noDesc = true
	o.minW = 80
	o.digits = !pane
	o.hintsFor = func(m *Model) []hintItem { return m.viewsHints(o, pane) }
	focus := func(id string) {
		for i, idx := range o.shown {
			if id != "" && o.items[idx].value == id {
				o.cursor = i
			}
		}
		o.move(0, m.overlayPage())
	}
	refresh := func(cur string) {
		o.items = m.viewItems()
		o.refilter()
		focus(cur)
	}
	focus(focusID)
	o.onPick = func(m *Model, it overlayItem) {
		if uuid, ok := strings.CutPrefix(it.value, savedPrefix); ok {
			m.openSaved(uuid)
			return
		}
		if v, ok := moduleViews[it.value]; ok {
			m.switchView(v)
		}
	}
	o.onKey = func(m *Model, key string) bool {
		cur := ""
		if o.cursor >= 0 && o.cursor < len(o.shown) {
			cur = o.items[o.shown[o.cursor]].value
		}
		if key == "R" { // shift+r
			if err := m.setModules(modules.Default()); err != nil {
				return m.viewsFail(err)
			}
			refresh(cur)
			m.setStatus("", false)
			return true
		}
		if cur == "" {
			return false
		}
		if strings.HasPrefix(cur, savedPrefix) {
			after := func() { m.pendingCmd = m.showViews("") }
			if pane {
				after = func() { refresh("") }
			}
			return m.savedViewsKey(o, key, cur, pane, focus, after)
		}
		var d int
		switch key {
		case "shift+up":
			d = -1
		case "shift+down":
			d = 1
		case "e", "d", "r":
			if pane {
				m.setStatus("a built-in view cannot be edited, deleted or restored", true)
				return true
			}
			return false
		default:
			return false
		}
		if o.filter.Value() != "" {
			return true // the order is changed on the whole list, not on a filtered one
		}
		next, err := m.shiftModule(cur, d)
		if err == nil {
			err = m.setModules(next)
		}
		if err != nil {
			return m.viewsFail(err)
		}
		refresh(cur)
		m.setStatus("", false)
		return true
	}
	return o
}

// viewsFail shows an error of the Views list in the status line and as a toast.
func (m *Model) viewsFail(err error) bool {
	m.setStatus(err.Error(), true)
	m.pendingCmd = m.showToast(err.Error(), true)
	return true
}

// shiftModule moves a module one place up or down among the modules of its own group (shown or
// hidden). At the edge of its group, a move toward the line crosses it: a shown module is hidden and
// lands first among the hidden ones, a hidden module is shown and lands last among the shown ones.
// A move away from the line at the edge does nothing.
func (m *Model) shiftModule(id string, d int) (modules.Arrangement, error) {
	n := m.mods.Normalize()
	idx := -1
	for i, x := range n.Order {
		if x == id {
			idx = i
		}
	}
	if idx < 0 {
		return m.mods, nil
	}
	hidden := n.IsHidden(id)
	for j := idx + d; j >= 0 && j < len(n.Order); j += d {
		if n.IsHidden(n.Order[j]) == hidden {
			return m.mods.Moved(id, j+1)
		}
	}
	if (!hidden && d < 0) || (hidden && d > 0) {
		return m.mods, nil
	}
	next, err := m.mods.With(id, hidden)
	if err != nil {
		return m.mods, err
	}
	var others []string
	for _, x := range next.Order {
		if x != id {
			others = append(others, x)
		}
	}
	pos := len(others) + 1
	if hidden { // now shown: after the last shown module
		last := -1
		for i, x := range others {
			if !next.IsHidden(x) {
				last = i
			}
		}
		pos = last + 2
	} else { // now hidden: before the first hidden module
		for i, x := range others {
			if next.IsHidden(x) {
				pos = i + 1
				break
			}
		}
	}
	return next.Moved(id, pos)
}
