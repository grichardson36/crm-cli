package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Add (a). The one add verb of every view. a opens a picker of object kinds with the kind of the
// current view preselected, so a then enter adds the view's own object. Each kind shows its first
// letter, and typing the letter picks it: a t adds a task, a n a note, a o an organization, a p a
// person, a d a deal. The picker is built from the objects registry (objects.go).
//
// Links are pipe segments (owner decision 2026-10-29). When an object is highlighted, it is the first
// suggestion of the segment that takes its kind (a task's people segment, a deal's organization segment,
// the "on" segment of a tag), and tab accepts it. Nothing is linked until the line is entered
// (pipelink.go).
//
// The screen is borrowed the way the bar's verbs borrow it (barverbs.go): the add flow runs as the
// kind's own empty view, and when the flow ends the view, cursor and selection you came from are put
// back, and the bar too when it was open. One undo step per add comes from the add flow itself.
// The per-object add actions (Add organization, Add task, ...) stay in the palette without a key.

// addReturn remembers where a global add came from.
type addReturn struct {
	bar    *overlay  // the bar to bring back, nil when it was not open
	saved  *barSaved // the screen to put back once the flow ends
	opened bool      // the kind picker was opened
	label  string
}

// addSuggestion is the highlighted object (or tag) the add flow started from. The pipe line offers it as
// the first suggestion of the segment that takes its kind (pipecomplete.go).
type addSuggestion struct {
	ref   repo.Ref
	tag   string // set instead of ref when the highlighted object is a tag
	label string
}

func globalAddActions() []action {
	return []action{
		{id: "add-any", title: "Add", group: "Verbs", hint: "a", keys: []string{"a"}, write: true,
			desc: "add an object: a picker of kinds with this view's own kind preselected (a then enter adds it), or type a letter to jump: t task, n note, o organization, p person, d deal. A highlighted object is the first suggestion of the matching link segment of the pipe line, tab accepts it",
			foot: footAdd, footIn: inList, footLabel: "add",
			keyIf: func(m *Model) bool { return m.focus != focusDetail }, // in the detail pane a adds a row (the form's own add)
			run:   func(m *Model) tea.Cmd { return m.openGlobalAdd(nil) }},
	}
}

// addKindOrder is the order of the picker.
var addKindOrder = []string{taskKindName, model.NoteKind, "organization", "person", "deal", tagKind}

// addShortcut is the letter of a kind in the add picker.
func addShortcut(kind string) string { return addShortcuts()[kind] }

// addShortcuts gives every kind that has an add its letter in the picker. A kind declares its own in
// the registry (objDef.shortcut). One that does not takes the first letter of its label, or when that is
// taken the next letter of the label that is free. Letters are unique (TestAddShortcutsAreUnique).
func addShortcuts() map[string]string { return shortcutsFor(objectDefs()) }

// shortcutsFor is addShortcuts over a list of objects.
func shortcutsFor(all []*objDef) map[string]string {
	out := map[string]string{}
	taken := map[string]bool{}
	var defs []*objDef
	for _, o := range all {
		if o.add != nil {
			defs = append(defs, o)
			if o.shortcut != "" {
				out[o.kind] = strings.ToLower(o.shortcut)
				taken[strings.ToLower(o.shortcut)] = true
			}
		}
	}
	for _, o := range defs {
		if _, ok := out[o.kind]; ok {
			continue
		}
		for _, r := range strings.ToLower(o.label) {
			if r >= 'a' && r <= 'z' && !taken[string(r)] {
				out[o.kind] = string(r)
				taken[string(r)] = true
				break
			}
		}
	}
	return out
}

// addKinds lists the kinds the picker offers, in the owner's order, then any kind registered later.
func addKinds() []overlayItem {
	var items []overlayItem
	seen := map[string]bool{}
	add := func(o *objDef) {
		if o == nil || o.add == nil || seen[o.kind] {
			return
		}
		seen[o.kind] = true
		desc := "add " + article(o.label) + " " + o.label
		if o.kind == model.NoteKind {
			desc = "add a note"
		}
		items = append(items, overlayItem{id: "add:" + o.kind, label: capitalize(o.label), desc: desc, value: o.kind, hint: addShortcut(o.kind)})
	}
	for _, kind := range addKindOrder {
		add(objectByKind(kind))
	}
	for _, o := range objectDefs() { // any kind registered later
		add(o)
	}
	return items
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// viewAddKind is the kind a view's own object is, for the preselection.
func viewAddKind(v viewKind) string {
	switch v {
	case viewTasks:
		return taskKindName
	case viewOrgs:
		return "organization"
	case viewDeals:
		return "deal"
	case viewTags:
		return tagKind
	}
	return "person"
}

// barKindAddKind maps the kind of a bar row to an add kind.
func barKindAddKind(kind string) string {
	switch kind {
	case "org":
		return "organization"
	case "task":
		return taskKindName
	case "deal", "person":
		return kind
	}
	return "person"
}

// suggestableKind says whether a highlighted object of this kind can be offered as a link.
func suggestableKind(kind string) bool {
	return kind == "person" || kind == "organization" || kind == "deal" || kind == taskKindName || kind == model.NoteKind
}

// addContext reads what the picker needs from the screen: the kind to preselect and the highlighted
// object. In the bar the highlighted row stands for the object.
func (m *Model) addContext(bar *overlay) (string, *addSuggestion) {
	if bar != nil {
		it, ok := bar.highlighted()
		if !ok {
			return "person", nil
		}
		t, has := barTargetOf(it)
		if !has {
			return "person", nil
		}
		def := barKindAddKind(t.kind)
		if t.kind == "tag" {
			return def, &addSuggestion{tag: t.name, label: "#" + t.name}
		}
		ref := repo.Ref{Kind: def, ID: t.id}
		if !suggestableKind(def) {
			return def, nil
		}
		return def, &addSuggestion{ref: ref, label: it.label}
	}
	def := viewAddKind(m.view)
	ref, ok := m.cursorObject()
	if !ok || !suggestableKind(ref.Kind) {
		return def, nil
	}
	return def, &addSuggestion{ref: ref, label: m.refName(ref)}
}

// openGlobalAdd shows the kind picker. bar is the bar to bring back afterwards, if it was open.
func (m *Model) openGlobalAdd(bar *overlay) tea.Cmd {
	def, sugg := m.addContext(bar)
	m.addResume = &addReturn{bar: bar, opened: true}
	m.addSugg = sugg
	items := addKinds()
	m.openPicker("Add", items, func(m *Model, it overlayItem) { m.pendingCmd = m.startGlobalAdd(it.value) })
	m.ov.letters, m.ov.noFilter = true, true
	m.ov.onCancel = func(m *Model) { m.addSugg = nil }
	for pos, idx := range m.ov.shown {
		if m.ov.items[idx].value == def {
			m.ov.cursor = pos
		}
	}
	return nil
}

// startGlobalAdd runs the add flow of a kind in the borrowed empty view of that kind.
func (m *Model) startGlobalAdd(kind string) tea.Cmd {
	ret := m.addResume
	if ret == nil {
		ret = &addReturn{}
		m.addResume = ret
	}
	sugg := m.addSugg
	if kind == model.NoteKind {
		if sugg == nil {
			// Nothing is highlighted: a note needs a person, so ask which.
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
		saved := m.enterBarContext(barTarget{kind: "blank-note"})
		ret.saved = &saved
		ret.label = "new note"
		m.noteType = m.currentNoteType()
		m.startInput("note-free", m.noteType+": ", "")
		return m.input.Focus()
	}
	o := objectByKind(kind)
	if o == nil || o.add == nil {
		return nil
	}
	if kind == viewAddKind(m.view) && ret.bar == nil && m.focus == focusList {
		// The view's own kind: a then enter works as the old per-view add did, the cursor lands on
		// the new object and nothing is borrowed.
		m.addResume = nil
		return o.add(m)
	}
	saved := m.enterBarContext(barTarget{kind: "blank-" + kind})
	ret.saved = &saved
	ret.label = "new " + o.label
	return o.add(m)
}

// saveFreeNote adds a note that is linked to nothing yet.
func (m *Model) saveFreeNote(text string) {
	// The first person in the people segment is the note's subject, so the line reads the same short
	// syntax as the command bar (#tag, @Name, remind fri). Everything else in the link segments is
	// linked after the note exists (pipelink.go).
	for i, l := range m.pipeLinks {
		if l.ref.Kind == "person" && l.ref.ID != 0 {
			m.pipeLinks = append(m.pipeLinks[:i:i], m.pipeLinks[i+1:]...)
			m.runNoteAs(l.ref.ID, text, "note-free")
			return
		}
	}
	typ := m.currentNoteType()
	linked := len(m.pipeLinks) > 0
	if _, err := m.hCreateNote(model.CreateInteractionInput{Type: typ, Content: &text}); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.noteType = typ
	m.afterWrite()
	if linked {
		m.setStatus(typ+" added and linked (ctrl+z undoes)", false)
	} else {
		m.setStatus(typ+" added, not linked (l links it) (ctrl+z undoes)", false)
	}
	m.pendingCmd = m.showToast(typ+" added · ctrl+z undoes", false)
}

// finishGlobalAdd puts the screen back, brings the bar back if it was open and says what happened.
func (m *Model) finishGlobalAdd() tea.Cmd {
	r := m.addResume
	m.addResume = nil
	m.addSugg = nil
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

// globalAddKeyInBar reports whether the key is the add key in the bar's results, for the bar to act
// on. In the bar's input line the key types a letter.
func (m *Model) globalAddKeyInBar(key string) bool {
	a := m.actionByID("add-any")
	return a != nil && a.hasKey(key) && m.ov != nil && m.ov.focusResults
}

// taskKindName is the object kind of a task.
const taskKindName = "task"
