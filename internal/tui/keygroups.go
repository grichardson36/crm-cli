package tui

import (
	"fmt"
	"sort"
	"strings"
)

// The grouped key list (patterns/settings-view, patterns/grouped-lists-with-dividers). The Keys
// section of Settings and the rebind pickers group every action under two levels of divider headings:
// first the layer (framework verbs, navigation, view control, app-specific), then the area (People,
// Orgs, Deals, Tasks, Notes, Tags, Merge, Views and Settings, App). One function decides the group of
// an action, from its id, scope and layer, so a new action lands in a group without any list edit.

// Layer headings, in display order.
const (
	keyLayerVerbs = "Framework verbs"
	keyLayerNav   = "Navigation"
	keyLayerView  = "View control"
	keyLayerApp   = "App-specific"
)

// Area headings, in display order.
const (
	keyAreaPeople = "People"
	keyAreaOrgs   = "Orgs"
	keyAreaDeals  = "Deals"
	keyAreaTasks  = "Tasks"
	keyAreaNotes  = "Notes"
	keyAreaTags   = "Tags"
	keyAreaMerge  = "Merge"
	keyAreaViews  = "Views and Settings"
	keyAreaApp    = "App"
)

var keyLayerOrder = []string{keyLayerVerbs, keyLayerNav, keyLayerView, keyLayerApp}
var keyAreaOrder = []string{keyAreaPeople, keyAreaOrgs, keyAreaDeals, keyAreaTasks, keyAreaNotes, keyAreaTags, keyAreaMerge, keyAreaViews, keyAreaApp}

// navigationIDs are navigation actions that are not in the Move group.
var navigationIDs = map[string]bool{
	"cell-next": true, "cell-prev": true, "focus-detail": true, "back": true,
	"theme-mode": true,
	"tag-tree-expand": true, "tag-tree-collapse": true,
}

// viewControlIDs are the actions that control the view or the app chrome.
var viewControlIDs = map[string]bool{
	"palette": true, "options": true, "help": true, "quit": true, "goto-view": true, "next-view": true,
	"prev-view": true, "clear-search": true, "reload": true,
}

// keyLayerOf is the layer heading of an action.
func keyLayerOf(a *action) string {
	switch {
	case a.group == "Move" || navigationIDs[a.id]:
		return keyLayerNav
	case viewControlIDs[a.id] || strings.HasPrefix(a.id, "view-"):
		return keyLayerView
	case a.layerOf() == layerFramework:
		return keyLayerVerbs
	}
	return keyLayerApp
}

// keyAreaOf is the area heading of an action.
func keyAreaOf(a *action) string {
	id := a.id
	has := func(prefixes ...string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(id, p) {
				return true
			}
		}
		return false
	}
	switch {
	case has("link-person", "unlink-person"):
		return keyAreaPeople
	case has("link-org", "unlink-org"):
		return keyAreaOrgs
	case has("link-deal", "unlink-deal"):
		return keyAreaDeals
	case has("link-task", "unlink-task"):
		return keyAreaTasks
	case has("link-note", "unlink-note"):
		return keyAreaNotes
	case has("link-tag", "unlink-tag"):
		return keyAreaTags
	case has("cmp-"):
		return keyAreaMerge
	case has("note"):
		return keyAreaNotes
	case id == "done" || has("task"):
		return keyAreaTasks
	case has("tag", "tags-"):
		return keyAreaTags
	case has("org-merge", "org-not-duplicate", "org-undismiss", "find-dupes", "compare", "dismiss", "undo-dismiss", "merge"):
		return keyAreaMerge
	case has("org"):
		return keyAreaOrgs
	case has("deal"):
		return keyAreaDeals
	case has("view-", "sview-", "goto-view", "next-view", "prev-view", "options", "modules-", "keys-", "theme-", "syntax-", "setting-", "go-today"):
		return keyAreaViews
	}
	switch a.scopeOf() {
	case "type:person":
		return keyAreaPeople
	case "type:organization":
		return keyAreaOrgs
	case "type:deal":
		return keyAreaDeals
	case "type:task":
		return keyAreaTasks
	case "type:tag":
		return keyAreaTags
	case "type:dupes", "compare":
		return keyAreaMerge
	}
	if a.group == "Contact" {
		return keyAreaPeople
	}
	return keyAreaApp
}

func orderIndex(order []string, v string) int {
	for i, o := range order {
		if o == v {
			return i
		}
	}
	return len(order)
}

// keyGroup is the layer and area heading of an action.
func keyGroup(a *action) (layer, area string) { return keyLayerOf(a), keyAreaOf(a) }

// defaultKeysByID is the default keys of every action, before any override.
func defaultKeysByID() map[string][]string {
	out := map[string][]string{}
	for _, a := range buildActions() {
		out[a.id] = a.keys
	}
	return out
}

// keyChanged says whether the action's keys differ from the default, and returns the default as text.
func keyChanged(a *action, defaults map[string][]string) (string, bool) {
	d := defaults[a.id]
	if strings.Join(d, " ") == strings.Join(a.keys, " ") {
		return "", false
	}
	if len(d) == 0 {
		return "none", true
	}
	return joinKeys(d), true
}

// keyCurrent is the current key text of an action.
func keyCurrent(a *action) string {
	if len(a.keys) == 0 {
		return "no key"
	}
	return joinKeys(a.keys)
}

// sortedActions returns the actions in group order: layer, area, then title.
func sortedActions(acts []*action) []*action {
	out := append([]*action{}, acts...)
	sort.SliceStable(out, func(i, j int) bool {
		li, ai := keyGroup(out[i])
		lj, aj := keyGroup(out[j])
		if x, y := orderIndex(keyLayerOrder, li), orderIndex(keyLayerOrder, lj); x != y {
			return x < y
		}
		if x, y := orderIndex(keyAreaOrder, ai), orderIndex(keyAreaOrder, aj); x != y {
			return x < y
		}
		return strings.ToLower(out[i].title) < strings.ToLower(out[j].title)
	})
	return out
}

// keyHeads gives the two divider levels of an action for the grouped list component. counts says how
// many actions each heading holds.
func keyHeads(a *action, layerN, areaN map[string]int) []groupHead {
	layer, area := keyGroup(a)
	return []groupHead{
		{key: layer, title: layer, meta: countText(layerN[layer], "key"), rule: true},
		{key: area, title: area, meta: countText(areaN[layer+"/"+area], "key")},
	}
}

func keyCounts(acts []*action) (layerN, areaN map[string]int) {
	layerN, areaN = map[string]int{}, map[string]int{}
	for _, a := range acts {
		l, ar := keyGroup(a)
		layerN[l]++
		areaN[l+"/"+ar]++
	}
	return
}

// groupedKeyItems builds the items of a rebind picker: dividers by layer then area, one item per
// action made by mk. It uses groupRows, the same component as every grouped list.
func groupedKeyItems(acts []*action, mk func(a *action) overlayItem) []overlayItem {
	acts = sortedActions(acts)
	layerN, areaN := keyCounts(acts)
	g := groupRows(acts,
		func(a *action) []groupHead { return keyHeads(a, layerN, areaN) },
		func(a *action) row { return row{title: a.id} })
	var items []overlayItem
	next := 0
	for _, r := range g.rows {
		if r.skip {
			items = append(items, overlayItem{divider: true, label: r.title, desc: r.meta})
			continue
		}
		items = append(items, mk(acts[next]))
		next++
	}
	return items
}

// keyRows are the rows of the Keys section of Settings under the two divider levels. The section
// divider above them is added by settingRows.
func (m *Model) keyActions() []*action {
	var acts []*action
	for i := range m.actions {
		acts = append(acts, &m.actions[i])
	}
	return sortedActions(acts)
}

// keyRowFor is the Settings row of an action: its name, the default key when it was changed, and the
// current key at the right.
func keyRowFor(a *action, defaults map[string][]string) row {
	sub := ""
	if d, changed := keyChanged(a, defaults); changed {
		sub = "default " + d
	}
	return row{setting: "key:" + a.id, title: a.title, sub: sub, meta: keyCurrent(a)}
}

// keyDetail is the detail pane of an action row in Settings.
func (m *Model) keyDetail(id string) []formRow {
	m.formTitle = ""
	a := m.actionByID(id)
	if a == nil {
		return nil
	}
	m.formTitle = a.title
	layer, area := keyGroup(a)
	rows := []formRow{infoRow("layer", layer), infoRow("area", area)}
	rows[0].head = "Action"
	rows = append(rows, infoRow("about", a.desc), infoRow("id", a.id))
	cur := infoRow("key", keyCurrent(a))
	cur.head = "Key"
	rows = append(rows, cur)
	if d, changed := keyChanged(a, defaultKeysByID()); changed {
		rows = append(rows, infoRow("default", d))
	}
	scope := a.scopeOf()
	rows = append(rows, infoRow("works in", fmt.Sprintf("%s (%s)", scope, a.mode)))
	return rows
}

// curKeyAction is the action of the key row under the cursor of the Settings view.
func (m *Model) curKeyAction() *action {
	if m.view != viewSettings {
		return nil
	}
	r, ok := m.selected()
	if !ok || !strings.HasPrefix(r.setting, "key:") {
		return nil
	}
	return m.actionByID(strings.TrimPrefix(r.setting, "key:"))
}
