package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/modules"
)

// Modules (M14). Every view is a module that can be turned on or off and put in any order. The
// arrangement is in config.toml (internal/modules) and per device. An off module disappears from
// the tabs, the number keys, [ and ] and the palette. It stays in the Views screen under Hidden.
// Links, record pages and every other route still reach its data, a hidden view that was opened
// from a link shows as "v Name".

var moduleViews = map[string]viewKind{
	"home": viewHome, "people": viewPeople, "follow-up": viewDue, "tasks": viewTasks, "organizations": viewOrgs,
	"birthdays": viewBirthdays, "merge": viewMerge, "archived": viewArchived, "sync": viewSync, "deals": viewDeals, "settings": viewSettings,
	"tags": viewTags,
}

// moduleOf is the module id of a view. Every view here is a module or opens from a link.
func moduleOf(v viewKind) (string, bool) {
	for id, vv := range moduleViews {
		if vv == v {
			return id, true
		}
	}
	return "", false
}

func moduleName(id string) string {
	if mod, ok := modules.Lookup(id); ok {
		return mod.Name
	}
	return id
}

// visibleViews lists the views that are on, in order.
func (m *Model) visibleViews() []viewKind {
	var out []viewKind
	for _, id := range m.mods.Visible() {
		out = append(out, moduleViews[id])
	}
	return out
}

// viewPosition is the 1-based place of a view among the visible ones, 0 when it is off or not a module.
func (m *Model) viewPosition(v viewKind) int {
	if id, ok := moduleOf(v); ok {
		return m.mods.Position(id)
	}
	return 0
}

// nextView is the visible view after v in the cycle of [ and ]. From a view that is off (opened
// from a link) ] goes to the first visible view and [ to the last.
func (m *Model) nextView(v viewKind, dir int) viewKind {
	vis := m.visibleViews()
	if len(vis) == 0 {
		return viewHome
	}
	for i, x := range vis {
		if x == v {
			return vis[(i+len(vis)+dir)%len(vis)]
		}
	}
	if dir > 0 {
		return vis[0]
	}
	return vis[len(vis)-1]
}

func viewBlurb(v viewKind) string {
	if id, ok := moduleOf(v); ok {
		if mod, ok := modules.Lookup(id); ok {
			return mod.Blurb
		}
	}
	return ""
}

// patchModuleActions gives the view actions the key of their place in the arrangement: the number
// of the position, none when the module is off. A key the user rebound is left alone.
func (m *Model) patchModuleActions() {
	for i := range m.actions {
		a := &m.actions[i]
		if a.module == "" {
			continue
		}
		if _, custom := m.keyOverrides[a.id]; custom || !a.isView {
			continue
		}
		if k := modules.Key(m.mods.Position(a.module)); k != "" {
			a.keys, a.hint = []string{k}, k
		} else {
			a.keys, a.hint = nil, ""
		}
	}
}

// moduleOn reports whether the action's module is on (true for an action without a module).
func (m *Model) moduleOn(a *action) bool {
	return a.module == "" || m.mods.Position(a.module) > 0
}

// applyStartupModules reads config.toml. A problem never stops the program, it shows in the status line.
func (m *Model) applyStartupModules() {
	if m.env.DataDir == "" {
		return
	}
	a, problems := modules.Load(m.env.DataDir)
	m.mods = a
	m.patchModuleActions()
	if len(problems) > 0 {
		m.setStatus("modules: "+problems[0], true)
	}
}

// setModules installs a new arrangement for this session and saves it when there is a data directory.
func (m *Model) setModules(a modules.Arrangement) error {
	if err := a.Check(); err != nil {
		return err
	}
	if m.env.DataDir != "" {
		if err := modules.Save(m.env.DataDir, a); err != nil {
			return err
		}
	}
	m.mods = a.Normalize()
	m.patchModuleActions()
	return nil
}

// The Views screen (v) is the one place that lists every view, shows or hides it and orders it. It
// is an overlay with a grouped list (patterns/views-screen): the views that are shown first, then the
// hidden ones under a divider. enter goes to the view, x (the universal mark) shows or hides it,
// shift+up and shift+down move it inside its group, esc closes. The screen itself is not a module,
// and Settings cannot be hidden, so the user can always get back to both.

// viewItems lists every module as the Views screen shows it: shown views in order, then a divider and
// the hidden views.
func (m *Model) viewItems() []overlayItem {
	arr := m.mods.Normalize()
	var shown, hidden []overlayItem
	for _, id := range arr.Order {
		mod, _ := modules.Lookup(id)
		v := moduleViews[id]
		it := overlayItem{id: "view:" + id, label: mod.Name, value: id, desc: mod.Blurb}
		if v == m.view {
			it.desc = "current view. " + mod.Blurb
		}
		if mod.Always {
			it.desc += " (always shown)"
		}
		if pos := arr.Position(id); pos > 0 {
			it.hint = modules.Key(pos)
			if a := m.actionByID(viewActionID(v)); a != nil && len(a.keys) > 0 {
				it.hint = keys.Display(a.keys[0])
			}
			shown = append(shown, it)
		} else {
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
			hidden = append(hidden, it)
		}
	}
	items := shown
	if len(hidden) > 0 {
		items = append(items, overlayItem{id: "divider:hidden", label: "Hidden", desc: fmt.Sprintf("%d, x shows", len(hidden)), divider: true})
		items = append(items, hidden...)
	}
	if len(m.svDeleted) > 0 {
		items = append(items, overlayItem{id: "divider:deleted", label: "Deleted views", desc: fmt.Sprintf("%d, r restores", len(m.svDeleted)), divider: true})
		for _, v := range m.svDeleted {
			it := m.savedItem(v)
			it.disabled = ""
			items = append(items, it)
		}
	}
	return items
}

// openViews shows the Views screen.
func (m *Model) openViews() tea.Cmd { return m.showViews("") }

func (m *Model) showViews(focusID string) tea.Cmd {
	o := m.newOverlay("Views", m.viewItems(), false)
	o.noFilter = true
	o.digits = true
	o.help = "enter go  x show/hide  shift+up/down move  e edit  d delete  r restore  esc close"
	o.note = "Number keys go to a shown view. Settings and this screen (v) are always available."
	focus := func(id string) {
		for i, idx := range o.shown {
			if id != "" && o.items[idx].value == id {
				o.cursor = i
			}
		}
		o.move(0, m.overlayPage())
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
		if cur == "" {
			return false
		}
		if strings.HasPrefix(cur, savedPrefix) {
			return m.savedViewsKey(o, key, cur, focus)
		}
		switch key {
		case "e", "d", "r":
			m.setStatus("a built-in view cannot be edited, deleted or restored, x hides it", true)
			return true
		}
		var err error
		next := m.mods
		switch key {
		case "x":
			next, err = m.mods.With(cur, m.mods.Position(cur) == 0)
		case "shift+up":
			next, err = m.shiftModule(cur, -1)
		case "shift+down":
			next, err = m.shiftModule(cur, 1)
		default:
			return false
		}
		if err == nil {
			err = m.setModules(next)
		}
		if err != nil {
			m.setStatus(err.Error(), true)
			m.pendingCmd = m.showToast(err.Error(), true)
			return true
		}
		o.items = m.viewItems()
		o.refilter()
		focus(cur)
		m.setStatus("", false)
		return true
	}
	return m.showOverlay(o)
}

// shiftModule moves a module one place up or down among the modules of its own group (shown or
// hidden), so a view never jumps across the divider.
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
	return m.mods, nil
}

func viewActionID(v viewKind) string {
	if v == viewDue {
		return "view-due"
	}
	return "view-" + strings.ToLower(viewNames[v])
}

func moduleActions() []action {
	return []action{
		{id: "modules-reset", title: "Modules: reset", group: "App", desc: "put the default view arrangement back: Archived and Sync hidden, the rest shown in the default order (palette, no key)", write: true,
			run: func(m *Model) tea.Cmd {
				if err := m.setModules(modules.Default()); err != nil {
					m.setStatus(err.Error(), true)
					return m.showToast(err.Error(), true)
				}
				m.setStatus("modules: default arrangement restored", false)
				return nil
			}},
	}
}
