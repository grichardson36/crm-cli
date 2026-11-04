package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/modules"
)

// Modules (M14). Every view is a module that can be turned on or off and put in any order. The
// arrangement is in config.toml (internal/modules) and per device. An off module disappears from
// the tabs, the number keys, [ and ] and the palette. It stays in the Views screen under Hidden.
// Links, record pages and every other route still reach its data, a hidden view that was opened
// from a link shows as "v Name".

var moduleViews = map[string]viewKind{
	"home": viewHome, "people": viewPeople, "follow-up": viewDue, "tasks": viewTasks, "organizations": viewOrgs,
	"dates": viewDates, "merge": viewMerge, "archived": viewArchived, "sync": viewSync, "deals": viewDeals, "settings": viewSettings,
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
