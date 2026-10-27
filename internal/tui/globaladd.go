package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Add (a). The one add verb of every view. a opens a picker of object kinds with the kind of the
// current view preselected, so a then enter adds the view's own object. Each kind shows its first
// letter, and typing the letter picks it: a t adds a task, a n a note, a o an organization, a p a
// person, a d a deal. The picker is built from the objects registry (objects.go).
//
// When an object is highlighted, a task or a note offers it as a link: the prompt shows the name in
// grayscale after the cursor, and tab accepts it. Nothing is linked until tab is pressed.
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

// addSuggestion is the highlighted object offered as the link of the new object, for any pair of
// kinds the relation registry (model.Relations) joins, and a highlighted tag as a tag for any kind.
// reason is set when nothing can be offered: it is shown in the grayscale hint instead.
type addSuggestion struct {
	ref      repo.Ref
	tag      string // set instead of ref when the highlighted object is a tag
	label    string
	accepted bool
	reason   string
	// generic suggestions are linked after the new object is created (afterCreate). The task or note
	// offered a person, organization or deal keeps the older flow that borrows that object's page.
	generic bool
}

func globalAddActions() []action {
	return []action{
		{id: "add-any", title: "Add", group: "Verbs", hint: "a", keys: []string{"a"}, write: true,
			desc: "add an object: a picker of kinds with this view's own kind preselected (a then enter adds it), or type a letter to jump: t task, n note, o organization, p person, d deal. A highlighted object is offered as the link of a task or note, tab accepts it",
			foot: footAdd, footIn: inList, footLabel: "add",
			keyIf: func(m *Model) bool { return m.focus != focusDetail }, // in the detail pane a adds a row (the form's own add)
			run:   func(m *Model) tea.Cmd { return m.openGlobalAdd(nil) }},
	}
}

// addKindOrder is the order of the picker.
var addKindOrder = []string{taskKindName, model.NoteKind, "organization", "person", "deal"}

// addKindLetter is the first letter of each kind's word in the picker.
func addKindLetter(kind string) string {
	if o := objectByKind(kind); o != nil {
		return strings.ToLower(o.label[:1])
	}
	return ""
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
		items = append(items, overlayItem{id: "add:" + o.kind, label: capitalize(o.label), desc: desc, value: o.kind, hint: addKindLetter(o.kind)})
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
	m.addLinkPending = nil
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

// suggestionFor settles what the highlighted object offers to a new object of this kind. It returns
// the suggestion to link, or nil when nothing can be linked, and then m.addSugg holds a reason the
// prompt shows in grayscale ("nothing highlighted", "a deal has no link to a note").
func (m *Model) suggestionFor(kind string) *addSuggestion {
	s := m.addSugg
	if s == nil {
		m.addSugg = &addSuggestion{reason: "nothing highlighted to link"}
		return nil
	}
	if s.reason != "" {
		return nil
	}
	cp := *s
	switch {
	case cp.tag != "":
		if !model.ValidTaggable(kind) {
			cp = addSuggestion{reason: "a " + relationLabel(kind) + " cannot take a tag"}
		}
	default:
		if _, ok := model.RelationBetween(kind, cp.ref.Kind); !ok {
			cp = addSuggestion{reason: "no link between " + article(relationLabel(kind)) + " " + relationLabel(kind) + " and " + article(relationLabel(cp.ref.Kind)) + " " + relationLabel(cp.ref.Kind)}
		}
	}
	legacy := (kind == taskKindName || kind == model.NoteKind) && cp.tag == "" && cp.ref.Kind != "" &&
		(cp.ref.Kind == "person" || cp.ref.Kind == "organization" || cp.ref.Kind == "deal")
	cp.generic = !legacy
	m.addSugg = &cp
	if cp.reason != "" {
		return nil
	}
	return &cp
}

// startGlobalAdd runs the add flow of a kind in the borrowed empty view of that kind.
func (m *Model) startGlobalAdd(kind string) tea.Cmd {
	ret := m.addResume
	if ret == nil {
		ret = &addReturn{}
		m.addResume = ret
	}
	sugg := m.suggestionFor(kind)
	m.addLinkPending = nil
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

// acceptSuggestion links the new task or note to the highlighted object. The screen is borrowed as
// that object's own view, and the prompt becomes the one the object's own add uses, so the existing
// flows do the linking.
func (m *Model) acceptSuggestion() tea.Cmd {
	s := m.addSugg
	if s == nil || s.accepted {
		return nil
	}
	if s.reason != "" {
		m.setStatus(s.reason, true)
		return nil
	}
	s.accepted = true
	if s.generic {
		m.addLinkPending = s // linked once the new object exists (afterCreate)
		return nil
	}
	bk := s.ref.Kind
	if bk == "organization" {
		bk = "org"
	}
	m.enterBarContext(barTarget{kind: bk, id: s.ref.ID})
	value := m.input.Value()
	task := m.promptKind == "task-new"
	onPage := s.ref.Kind != "person"
	switch {
	case task && !onPage:
		m.promptKind = "task"
		m.input.Prompt = taskPersonPrompt
	case task:
		m.promptKind = "task-page"
		m.input.Prompt = taskPersonPrompt
	case !onPage:
		m.promptKind = "note"
		m.input.Prompt = m.noteType + ": "
	default:
		m.promptKind = "note-object"
		m.input.Prompt = m.objectNotePrompt(m.pageRef())
	}
	m.input.SetValue(value)
	m.input.CursorEnd()
	return nil
}

// saveFreeNote adds a note that is linked to nothing yet.
func (m *Model) saveFreeNote(text string) {
	typ := m.currentNoteType()
	if _, err := m.hCreateNote(model.CreateInteractionInput{Type: typ, Content: &text}); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.noteType = typ
	m.afterWrite()
	m.setStatus(typ+" added, not linked (l links it) (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(typ+" added · ctrl+z undoes", false)
}

// suggestionHint is the grayscale text after the prompt while a link is offered.
func (m *Model) suggestionHint() string {
	s := m.addSugg
	if s == nil {
		return ""
	}
	if s.reason != "" {
		return s.reason
	}
	if s.accepted {
		return "linked to " + s.label
	}
	return "tab: link " + s.label
}

// finishGlobalAdd puts the screen back, brings the bar back if it was open and says what happened.
func (m *Model) finishGlobalAdd() tea.Cmd {
	r := m.addResume
	m.addResume = nil
	m.addSugg, m.addLinkPending = nil, nil
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

// afterCreate links a new object to the suggestion the user accepted with tab in the add picker
// (owner decision 2026-10-28). The pair comes from the relation registry, a tag from the tag
// relation. The create and the link are one undo step. A failed link leaves the object created and
// says so.
func (m *Model) afterCreate(kind string, id int64) {
	p := m.addLinkPending
	if p == nil || !p.accepted {
		return
	}
	m.addLinkPending = nil
	// Fold the entry the create just recorded into the same step as the link.
	var created *histEntry
	if m.grpDepth == 0 && len(m.hist) > 0 {
		e := m.hist[len(m.hist)-1]
		m.hist = m.hist[:len(m.hist)-1]
		created = &e
	}
	m.beginGroup()
	if created != nil {
		m.grp = append(m.grp, *created)
	}
	ref := repo.Ref{Kind: kind, ID: id}
	var err error
	if p.tag != "" {
		err = m.hTagApply(m.ctx, kind, id, p.tag)
	} else {
		err = m.recordLinkChange("link new "+relationLabel(kind)+" to "+p.label, ref, p.ref, func() error {
			_, e := m.links.Link(m.ctx, ref, p.ref)
			return e
		})
	}
	label := "add " + relationLabel(kind) + " and link it to " + p.label
	m.endGroup(label)
	if err != nil {
		m.setStatus("created, but could not link to "+p.label+": "+err.Error(), true)
		return
	}
}
