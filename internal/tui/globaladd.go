package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/keys"
)

// Global add (ctrl+a). Bare a and + add the view's own object. ctrl+a is the same verb from
// anywhere: it opens a small picker of object kinds built from the objects registry (person,
// organization, deal, task, and note, which needs a person), then runs that kind's normal add flow.
// The screen is borrowed the way the bar's verbs borrow it (barverbs.go): the add flow runs as the
// kind's own empty view, and when the flow ends the view, cursor and selection you came from are put
// back, and the bar too when it was open. One undo step per add comes from the add flow itself.
// ctrl+a is tmux's prefix and readline's line start, so it stays rebindable ([keys], id add-any).

// addReturn remembers where a global add came from.
type addReturn struct {
	bar    *overlay  // the bar to bring back, nil when it was not open
	saved  *barSaved // the screen to put back once the flow ends
	opened bool      // the kind picker was opened
	label  string
}

func globalAddActions() []action {
	return []action{
		{id: "add-any", title: "New: add any object", group: "App", hint: "ctrl+a", keys: []string{"ctrl+a"}, write: true,
			desc: "pick a kind (person, organization, deal, task, note) and run its add flow from any view, then come back to where you were. a or + adds the view's own object",
			run:  func(m *Model) tea.Cmd { return m.openGlobalAdd(nil) }},
	}
}

// addKinds lists the kinds the picker offers: the registered objects, person first, then note.
func addKinds() []overlayItem {
	var items []overlayItem
	seen := map[string]bool{}
	for _, kind := range []string{"person", "organization", "deal", "task"} {
		if o := objectByKind(kind); o != nil && o.add != nil {
			items = append(items, overlayItem{id: "add:" + kind, label: capitalize(o.label), desc: "add " + article(o.label) + " " + o.label, value: kind})
			seen[kind] = true
		}
	}
	for _, o := range objectDefs() { // any kind registered later
		if !seen[o.kind] && o.add != nil {
			items = append(items, overlayItem{id: "add:" + o.kind, label: capitalize(o.label), desc: "add " + article(o.label) + " " + o.label, value: o.kind})
		}
	}
	items = append(items, overlayItem{id: "add:note", label: "Note", desc: "add a note to a person you pick", value: "note"})
	for i := range items {
		items[i].hint = fmt.Sprint(i + 1)
	}
	return items
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// openGlobalAdd shows the kind picker. bar is the bar to bring back afterwards, if it was open.
func (m *Model) openGlobalAdd(bar *overlay) tea.Cmd {
	m.addResume = &addReturn{bar: bar, opened: true}
	m.openPicker("New", addKinds(), func(m *Model, it overlayItem) { m.pendingCmd = m.startGlobalAdd(it.value) })
	m.ov.digits = true
	return nil
}

// startGlobalAdd runs the add flow of a kind in the borrowed empty view of that kind.
func (m *Model) startGlobalAdd(kind string) tea.Cmd {
	ret := m.addResume
	if ret == nil {
		ret = &addReturn{}
		m.addResume = ret
	}
	if kind == "note" {
		var items []overlayItem
		for _, p := range m.people {
			items = append(items, overlayItem{id: fmt.Sprintf("person:%d", p.ID), label: cleanCell(p.Name), desc: cleanCell(p.Company), ref: p.ID})
		}
		if len(items) == 0 {
			m.setStatus("there is nobody to add a note to", true)
			return nil
		}
		m.openPicker("Note for", items, func(m *Model, it overlayItem) {
			saved := m.enterBarContext(barTarget{kind: "person", id: it.ref})
			ret.saved = &saved
			ret.label = "note for " + it.label
			if a := m.actionByID("note"); a != nil {
				m.pendingCmd = m.runAction(a)
			}
		})
		return nil
	}
	o := objectByKind(kind)
	if o == nil || o.add == nil {
		return nil
	}
	saved := m.enterBarContext(barTarget{kind: "blank-" + kind})
	ret.saved = &saved
	ret.label = "new " + o.label
	return o.add(m)
}

// finishGlobalAdd puts the screen back, brings the bar back if it was open and says what happened.
func (m *Model) finishGlobalAdd() tea.Cmd {
	r := m.addResume
	m.addResume = nil
	if r.saved != nil {
		m.leaveBarContext(*r.saved)
	}
	var cmd tea.Cmd
	if r.saved != nil && m.status != "" { // a cancelled picker has nothing to report
		cmd = m.showToast(m.status, m.statusIsErr)
	}
	if r.bar != nil {
		m.ov, m.mode = r.bar, modeOverlay
		r.bar.refilterBar()
	}
	return cmd
}

// globalAddKeyInBar reports whether the key is the global add key, for the bar to act on.
func (m *Model) globalAddKeyInBar(key string) bool {
	a := m.actionByID("add-any")
	return a != nil && a.hasKey(key) && keys.Display(key) != ""
}
