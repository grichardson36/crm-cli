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
// the tabs, the number keys, [ and ], the view list and the palette. Links, record pages and every
// other route still reach its data, a hidden view that was opened from a link shows as "v Name".

var moduleViews = map[string]viewKind{
	"home": viewHome, "people": viewPeople, "follow-up": viewDue, "tasks": viewTasks, "organizations": viewOrgs,
	"birthdays": viewBirthdays, "merge": viewMerge, "archived": viewArchived, "sync": viewSync, "deals": viewDeals,
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

// moduleItems lists every module with its state and key.
func (m *Model) moduleItems() []overlayItem {
	var items []overlayItem
	for _, id := range m.mods.Normalize().Order {
		mod, _ := modules.Lookup(id)
		state := "off"
		hint := ""
		if pos := m.mods.Position(id); pos > 0 {
			state = "on"
			hint = modules.Key(pos)
		}
		if a := m.actionByID(viewActionID(moduleViews[id])); a != nil && len(a.keys) > 0 && m.mods.Position(id) > 0 {
			hint = keys.Display(a.keys[0])
		}
		items = append(items, overlayItem{id: "module:" + id, label: fmt.Sprintf("%-14s %s", mod.Name, state), hint: hint, desc: mod.Blurb, value: id})
	}
	return items
}

// openModules shows the Modules screen: space or enter turns the module under the cursor on or off,
// shift+up and shift+down move it up and down, esc closes. The palette entry "Modules: reset" puts the default arrangement back.
func (m *Model) openModules() tea.Cmd {
	return m.showModules("")
}

func (m *Model) showModules(focusID string) tea.Cmd {
	o := m.newOverlay("Modules: space toggles, shift+up / shift+down moves, esc closes", m.moduleItems(), false)
	o.noFilter = true
	if focusID != "" {
		for i, it := range o.items {
			if it.value == focusID {
				o.cursor = i
			}
		}
		o.move(0, m.overlayPage())
	}
	o.onKey = func(m *Model, key string) bool {
		cur := ""
		if o.cursor >= 0 && o.cursor < len(o.shown) {
			cur = o.items[o.shown[o.cursor]].value
		}
		var err error
		next := m.mods
		switch key {
		case " ", "enter":
			if cur == "" {
				return true
			}
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
		o.items = m.moduleItems()
		o.refilter()
		for i, idx := range o.shown {
			if cur != "" && o.items[idx].value == cur {
				o.cursor = i
			}
		}
		o.move(0, m.overlayPage())
		m.setStatus("", false)
		return true
	}
	return m.showOverlay(o)
}

// shiftModule moves a module one place up or down among all modules.
func (m *Model) shiftModule(id string, d int) (modules.Arrangement, error) {
	order := m.mods.Normalize().Order
	for i, x := range order {
		if x == id {
			return m.mods.Moved(id, i+1+d)
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
		{id: "modules", title: "Modules", group: "App", desc: "turn views on or off and put them in order (same as crm modules). Per device, in config.toml",
			run: func(m *Model) tea.Cmd { return m.openModules() }},
		{id: "modules-reset", title: "Modules: reset", group: "App", desc: "put the default module arrangement back: every view on, in the default order (palette, no key)", write: true,
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
