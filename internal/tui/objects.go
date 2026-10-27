package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// The object registry. Objects first, verbs second (patterns/objects-first, one-verb-set).
//
// Each object registers itself once in objectDefs: name, label, fields, relations, list columns and
// the repository hooks. The palette entries ("Add organization"), the keys, the footer hints and
// the help text are generated from the registry and the one verb set below. A verb is written
// once and takes the object as a parameter. A new object gets every verb by registering here.

type verb string

const (
	vAdd     verb = "add"
	vOpen    verb = "open"
	vEdit    verb = "edit"
	vLink    verb = "link"
	vUnlink  verb = "unlink"
	vDelete  verb = "delete"
	vRestore verb = "restore"
	vMark    verb = "mark"
	vUndo    verb = "undo"
	vRedo    verb = "redo"
)

// verbSet is the fixed verb list. Titles read "<Verb> <object>". A verb an object cannot do is
// absent, never renamed.
var verbSet = []verb{vAdd, vOpen, vEdit, vLink, vUnlink, vDelete, vRestore, vMark, vUndo, vRedo}

// verbKeys are the standard keys. They are the same in every object view (patterns/universal-verbs).
// Add is a (the picker in globaladd.go) and Open and Edit are one action, activate (enter, e, in
// verbs.go), so the per-object Add and Edit actions have no key of their own. Mark is space (the
// multi-select), undo and redo are ctrl+z and ctrl+y, and restore is r on the Archived screen.
var verbKeys = map[verb][]string{
	vLink:    {"l"},
	vUnlink:  {"u"},
	vDelete:  {"d"},
	vRestore: {"r"},
	vMark:    {" "},
	vUndo:    {"ctrl+z"},
	vRedo:    {"ctrl+y"},
}

// objDef is one object type.
type objDef struct {
	kind   string   // model kind: person, organization, deal, task
	label  string   // the word in palette titles
	short  string   // prefix of action ids: org-add
	group  string   // help group
	view   viewKind // the list view of this object
	fields []string // fields of the one-line form, in order. Empty when the detail form edits it
	cols   []string // list columns, left to right

	// prompts are the prompt kinds of this object's pipe line (org-add, org-edit). The inline hint
	// shows fields for them. barVerb and barFields give the command bar its hint for the verb that
	// adds this object (task, note), see hint.go.
	prompts   []string
	barVerb   string
	barFields []hintField

	can map[verb]bool // verbs this object supports

	// ids keeps the action ids that existed before the registry, so rebound keys still work.
	ids map[verb]string

	// Hooks. id 0 means none.
	selected  func(m *Model) int64                       // the object the cursor is on in this context
	nameOf    func(m *Model, id int64) string            // label in prompts and toasts
	add       func(m *Model) tea.Cmd                     // start the Add form
	edit      func(m *Model, id int64) tea.Cmd           // start the Edit form
	archive   func(m *Model, id int64) error             // recorded soft delete
	restore   func(m *Model, id int64) error             // recorded restore
	showRow   func(m *Model, id int64)                   // put the cursor on the object after a restore
	deleteAsk func(m *Model)                             // replaces the single y/n delete (people delete many)
	restoreIn func(m *Model) bool                        // Restore applies here (nil: the session stack is not empty)
	restoreDo func(m *Model)                             // replaces the stack restore (people come from the Archived view)
	items     func(m *Model) []overlayItem               // every live object, for the link picker
	create    func(m *Model, text string) (int64, error) // make an object from typed text, for the link picker
	keyHere   func(m *Model, v verb, key string) bool    // limits the keys of a verb to the views that mean this object
}

// prompt is the one-line form prompt of an object with fields.
func (o *objDef) prompt() string {
	return o.label + ": " // the fields show as the hint after the cursor (hint.go)
}

func (o *objDef) has(v verb) bool { return o.can[v] }

func (o *objDef) actionID(v verb) string {
	if id, ok := o.ids[v]; ok {
		return id
	}
	return o.short + "-" + string(v)
}

// objectDefs registers every object, in key-resolution order.
func objectDefs() []*objDef {
	return []*objDef{orgDef(), dealDef(), taskDef(), noteDef(), personDef()}
}

func objectByKind(kind string) *objDef {
	for _, o := range objectDefs() {
		if o.kind == kind {
			return o
		}
	}
	return nil
}

// --- the objects ---

func orgDef() *objDef {
	return &objDef{
		kind: "organization", label: "organization", short: "org", group: "Organizations", view: viewOrgs,
		fields:   []string{"name", "domain", "industry", "size", "location", "website", "linkedin", "description", "notes"},
		cols:     []string{"name", "industry", "domain"},
		prompts:  []string{"org-add", "org-edit"},
		can:      map[verb]bool{vAdd: true, vEdit: true, vLink: true, vUnlink: true, vDelete: true, vRestore: true},
		selected: func(m *Model) int64 { return m.selOrg() },
		nameOf:   func(m *Model, id int64) string { return m.orgName(id) },
		add:      func(m *Model) tea.Cmd { return m.startOrgAdd() },
		edit:     func(m *Model, id int64) tea.Cmd { return m.startOrgEdit(id) },
		archive:  func(m *Model, id int64) error { return m.hArchiveOrg(id) },
		restore:  func(m *Model, id int64) error { return m.hRestoreOrg(id) },
		showRow: func(m *Model, id int64) {
			if m.view == viewOrgs {
				m.cursorOn(func(r row) bool { return r.orgID == id })
			}
		},
		items: func(m *Model) []overlayItem {
			var out []overlayItem
			list, _ := m.orr.FindAll(m.ctx, 0)
			for _, o := range list {
				out = append(out, overlayItem{id: "pick", label: cleanCell(o.Name), desc: cleanCell(str(o.Industry)), ref: o.ID, value: o.Name})
			}
			return out
		},
		create: func(m *Model, text string) (int64, error) {
			o, err := m.hCreateOrg(model.CreateOrgInput{Name: text, AllowDuplicate: m.orgCreateAnyway})
			if err != nil {
				return 0, err
			}
			return o.ID, nil
		},
		keyHere: func(m *Model, v verb, key string) bool { return m.view == viewOrgs && m.focus == focusList },
	}
}

func dealDef() *objDef {
	return &objDef{
		kind: "deal", label: "deal", short: "deal", group: "Deals", view: viewDeals,
		fields:   []string{"title", "value", "stage", "organization", "person", "notes"},
		cols:     []string{"title", "stage and organization", "value"},
		prompts:  []string{"deal-add", "deal-edit"},
		can:      map[verb]bool{vAdd: true, vEdit: true, vLink: true, vUnlink: true, vDelete: true, vRestore: true},
		selected: func(m *Model) int64 { return m.selDeal() },
		nameOf:   func(m *Model, id int64) string { return m.dealTitle(id) },
		add:      func(m *Model) tea.Cmd { return m.startDealAdd() },
		edit:     func(m *Model, id int64) tea.Cmd { return m.startDealEdit(id) },
		archive:  func(m *Model, id int64) error { return m.hArchiveDeal(id) },
		restore:  func(m *Model, id int64) error { return m.hRestoreDeal(id) },
		showRow: func(m *Model, id int64) {
			if m.view == viewDeals {
				m.cursorOn(func(r row) bool { return r.dealID == id })
			}
		},
		items: func(m *Model) []overlayItem {
			var out []overlayItem
			list, _ := m.dlr.FindAll(m.ctx, model.DealFilters{})
			for _, d := range list {
				out = append(out, overlayItem{id: "pick", label: cleanCell(d.Title), desc: d.Stage, ref: d.ID, value: d.Title})
			}
			return out
		},
		create: func(m *Model, text string) (int64, error) {
			d, err := m.hCreateDeal(model.CreateDealInput{Title: text, Stage: "lead"})
			if err != nil {
				return 0, err
			}
			return d.ID, nil
		},
		keyHere: func(m *Model, v verb, key string) bool { return m.view == viewDeals && m.focus == focusList },
	}
}

func taskDef() *objDef {
	return &objDef{
		kind: "task", label: "task", short: "task", group: "Tasks", view: viewTasks,
		fields:   []string{"title", "due"},
		cols:     []string{"title", "repeat and tags", "due"},
		prompts:  []string{"task-new", "task-edit", "task", "task-page"},
		barVerb:  "task",
		barFields: []hintField{{Label: "title"}, {Label: "by date", Marker: "by "}, {Label: "@person", Marker: "@"}, {Label: "#tag", Marker: "#"},
			{Label: "!priority", Marker: "!"}, {Label: "every 3m", Marker: "every "}},
		can:      map[verb]bool{vAdd: true, vEdit: true, vLink: true, vUnlink: true, vDelete: true, vRestore: true, vMark: true},
		ids:      map[verb]string{vAdd: "task", vDelete: "task-delete", vRestore: "task-restore"},
		selected: func(m *Model) int64 { return m.taskTarget() },
		nameOf: func(m *Model, id int64) string {
			if t, err := m.tr.FindByID(m.ctx, id); err == nil {
				return truncateLabel(t.Title)
			}
			return fmt.Sprintf("task %d", id)
		},
		add:     func(m *Model) tea.Cmd { return m.startTaskAdd() },
		edit:    func(m *Model, id int64) tea.Cmd { return m.startTaskEdit(id) },
		archive: func(m *Model, id int64) error { return m.hArchiveTask(m.ctx, id) },
		restore: func(m *Model, id int64) error { return m.hRestoreTask(m.ctx, id) },
		showRow: func(m *Model, id int64) {
			if m.view == viewTasks {
				m.cursorOn(func(r row) bool { return r.taskID == id })
			}
		},
		items: func(m *Model) []overlayItem {
			var out []overlayItem
			list, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
			for _, t := range list {
				out = append(out, overlayItem{id: "pick", label: cleanCell(t.Title), desc: str(t.DueAt), ref: t.ID, value: t.Title})
			}
			return out
		},
		create: func(m *Model, text string) (int64, error) {
			t, err := m.hCreateTask(model.CreateTaskInput{Title: text, Priority: "medium"})
			if err != nil {
				return 0, err
			}
			return t.ID, nil
		},
		// Task keys work in the Tasks view. In a person's detail form they work on a task row.
		keyHere: func(m *Model, v verb, key string) bool {
			if v == vAdd {
				return m.view == viewTasks && m.focus == focusList || (whenPerson(m) || whenObjectPage(m))
			}
			return m.view == viewTasks && m.focus == focusList || m.focus == focusDetail && v == vDelete
		},
	}
}

// noteDef registers notes (the interactions table) like every other object. A note is added with n
// on a person, organization or deal, edited and deleted as a row of its page, restored from the
// palette, and linked to any object with Link (patterns/objects-first). It has no list view of its own.
func noteDef() *objDef {
	return &objDef{
		kind: model.NoteKind, label: "note", short: "note", group: "Contact", view: viewPeople,
		fields:  []string{"text"},
		cols:    []string{"date", "type and subject", "text"},
		prompts: []string{"note", "note-object"},
		barVerb: "note",
		barFields: []hintField{{Label: "text"}, {Label: "@person", Marker: "@"}, {Label: "#tag", Marker: "#"}, {Label: "remind fri", Marker: "remind "}},
		can:    map[verb]bool{vAdd: true, vEdit: true, vLink: true, vUnlink: true, vDelete: true, vRestore: true},
		// The action ids that existed before the registry keep working for rebound keys.
		ids:      map[verb]string{vAdd: "note", vRestore: "note-restore"},
		selected: func(m *Model) int64 { return m.noteTarget() },
		nameOf:   func(m *Model, id int64) string { return m.noteName(id) },
		add: func(m *Model) tea.Cmd {
			if m.onObjectPage() {
				m.startObjectNote()
				return m.takePending()
			}
			return m.startNotePrompt()
		},
		edit: func(m *Model, id int64) tea.Cmd {
			m.selectNote(id)
			return m.beginEdit(m.fcur, false)
		},
		archive: func(m *Model, id int64) error { return m.hArchiveNote(m.ctx, id) },
		restore: func(m *Model, id int64) error { return m.hRestoreNote(m.ctx, id) },
		showRow: func(m *Model, id int64) { m.selectNote(id) },
		items: func(m *Model) []overlayItem {
			var out []overlayItem
			list, _ := m.ir.FindAll(m.ctx, model.InteractionFilters{Limit: 500})
			for _, n := range list {
				out = append(out, overlayItem{id: "pick", label: cleanCell(noteLabel(n)), desc: cleanCell(n.Type + " " + shortDate(n.OccurredAt)), ref: n.ID, value: noteLabel(n)})
			}
			return out
		},
		create: func(m *Model, text string) (int64, error) {
			n, err := m.hCreateNote(model.CreateInteractionInput{Type: m.currentNoteType(), Content: &text})
			if err != nil {
				return 0, err
			}
			return n.ID, nil
		},
		// n adds a note to the person, organization or deal on screen. The other note verbs work on a
		// note row of a page and have no key of their own: enter and d on the row, the palette.
		keyHere: func(m *Model, v verb, key string) bool {
			return v == vAdd && (whenPerson(m) || whenObjectPage(m))
		},
	}
}

func personDef() *objDef {
	return &objDef{
		kind: "person", label: "person", short: "person", group: "Contact", view: viewPeople,
		cols: []string{"name", "company or title", "keep in touch"},
		can:  map[verb]bool{vAdd: true, vEdit: true, vLink: true, vUnlink: true, vDelete: true, vRestore: true, vMark: true},
		ids:  map[verb]string{vAdd: "new", vDelete: "archive", vRestore: "restore", vEdit: "person-edit"},
		selected: func(m *Model) int64 {
			if !whenPerson(m) || m.view == viewSync || m.view == viewMerge {
				return 0
			}
			return m.curPerson()
		},
		nameOf:    func(m *Model, id int64) string { return m.personLabel(id) },
		add:       func(m *Model) tea.Cmd { return m.startCreate() },
		edit:      func(m *Model, id int64) tea.Cmd { m.focus = focusDetail; m.syncDetail(); return nil },
		archive:   func(m *Model, id int64) error { return m.hArchivePerson(m.ctx, id) },
		restore:   func(m *Model, id int64) error { return m.hRestorePerson(m.ctx, id) },
		deleteAsk: func(m *Model) { m.askArchive() },
		restoreIn: whenArchivedRows,
		restoreDo: func(m *Model) { m.restoreTargets() },
		items: func(m *Model) []overlayItem {
			var out []overlayItem
			for _, p := range m.people {
				out = append(out, overlayItem{id: "pick", label: cleanCell(p.Name), desc: cleanCell(p.Company), ref: p.ID, value: p.Name})
			}
			return out
		},
		create: func(m *Model, text string) (int64, error) {
			p, err := m.createNamed(text)
			if err != nil {
				return 0, err
			}
			return p.ID, nil
		},
		keyHere: func(m *Model, v verb, key string) bool {
			switch v {
			case vAdd:
				// the detail form's own a adds a row, so a person is added from a people list only.
				switch m.view {
				case viewHome, viewPeople, viewDue, viewBirthdays:
					return m.focus == focusList
				}
				return false
			case vDelete:
				// d only in the list (in the detail form it deletes a row).
				return m.focus == focusList
			case vEdit:
				return m.focus == focusList && (m.view == viewPeople || m.view == viewDue || m.view == viewBirthdays)
			}
			return m.focus == focusList
		},
	}
}

// --- generated actions ---

// footer positions of the generated actions
const (
	footAdd     = 45
	footEdit    = 20
	footDelete  = 46
	footRestore = 47
	footLink    = 60
	footUnlink  = 61
)

// objectActions generates the "<Verb> <object>" actions of every object from the registry.
func objectActions() []action {
	var out []action
	for _, o := range objectDefs() {
		o := o
		for _, v := range verbSet {
			if !o.has(v) {
				continue
			}
			switch v {
			case vAdd:
				out = append(out, o.addAction())
			case vEdit:
				out = append(out, o.editAction())
			case vDelete:
				out = append(out, o.deleteAction())
			case vRestore:
				out = append(out, o.restoreAction())
			}
		}
	}
	return append(out, linkActions()...)
}

func (o *objDef) keysFor(v verb) []string {
	switch {
	case v == vAdd || v == vEdit:
		return nil // a opens the add picker, enter and e are activate
	case v == vRestore && o.kind != "person":
		return nil // the session restore of a deleted object is ctrl+z or the palette; r restores on the Archived screen
	case o.kind == model.NoteKind:
		return nil
	}
	return append([]string{}, verbKeys[v]...)
}

func (o *objDef) keyFor(v verb) func(m *Model, key string) bool {
	return func(m *Model, key string) bool { return o.keyHere == nil || o.keyHere(m, v, key) }
}

func (o *objDef) addAction() action {
	desc := "add " + article(o.label) + " " + o.label
	if len(o.fields) > 0 {
		desc += ": " + strings.Join(o.fields, " | ") + " (same as crm " + cliNoun(o.kind) + " add)"
	}
	a := action{
		id: o.actionID(vAdd), title: "Add " + o.label, group: o.group, keys: o.keysFor(vAdd),
		desc: desc + ". From the palette, or a then " + addKindLetter(o.kind), write: true,
		keyFor: o.keyFor(vAdd),
		run:    func(m *Model) tea.Cmd { return o.add(m) },
	}
	if o.kind == "person" {
		a.title, a.desc = "Add person", "create a contact: opens a blank form, saved when you enter a name. From the palette, or a then p"
	}
	if o.kind == "task" {
		a.desc = "add a task: title | optional due date. From the palette, or a then t (same as crm task add). A highlighted object is offered as the link, tab accepts it"
	}
	if o.kind == model.NoteKind {
		a.when = func(m *Model) bool { return whenPerson(m) || whenObjectPage(m) }
		a.desc = "add a note of the last used type to the person, organization or deal on screen (tab in the prompt changes the type; same as crm note add). From the palette, or a then n"
	}
	return a
}

func (o *objDef) editAction() action {
	a := action{
		id: o.actionID(vEdit), title: "Edit " + o.label, group: o.group, keys: o.keysFor(vEdit),
		desc:   "edit the selected " + o.label + ", fields you empty are cleared (same as crm " + cliNoun(o.kind) + " edit). enter or e does this in its list",
		write:  true,
		keyFor: o.keyFor(vEdit),
		when:   func(m *Model) bool { return o.selected(m) != 0 },
		run:    func(m *Model) tea.Cmd { return o.edit(m, o.selected(m)) },
	}
	if o.kind == model.NoteKind {
		a.desc = "edit the note under the cursor inline (enter or e on the note row; same as crm note edit)"
	}
	if o.kind == "person" {
		a.write = false
		a.desc = "move into the detail form of the selected person to edit it (enter or e)"
	}
	return a
}

func (o *objDef) deleteAction() action {
	a := action{
		id: o.actionID(vDelete), title: "Delete " + o.label, group: o.group, hint: "d", keys: o.keysFor(vDelete),
		desc:  "delete the selected " + o.label + " (asks y/n, the same soft delete as crm " + cliNoun(o.kind) + " delete). ctrl+z undoes it",
		write: true, keyFor: o.keyFor(vDelete), foot: footDelete, footIn: inList, footLabel: "delete",
		when: func(m *Model) bool { return o.selected(m) != 0 },
		run:  func(m *Model) tea.Cmd { m.verbDelete(o); return nil },
	}
	if o.kind == "task" {
		a.footIf = func(m *Model) bool { return m.view == viewTasks }
	}
	if o.kind == model.NoteKind {
		a.hint, a.foot = "d", 0
		a.desc = "delete the note under the cursor (asks y/n, the same soft delete as crm note delete). d on the note row, ctrl+z undoes it"
	}
	if o.kind == "person" {
		a.desc = "delete (archive) the person or all marked (asks y/n, restore from the Archived view)"
		a.footLabel = "archive"
		a.foot = 35
		a.footIf = whenMarks
	}
	return a
}

func (o *objDef) restoreAction() action {
	a := action{
		id: o.actionID(vRestore), title: "Restore " + o.label, group: o.group, keys: o.keysFor(vRestore),
		desc:  "bring back the " + o.label + " deleted last in this session (palette; ctrl+z does the same for the newest change; same as crm " + cliNoun(o.kind) + " restore)",
		write: true, keyFor: o.keyFor(vRestore),
		when: func(m *Model) bool { return len(m.deleted[o.kind]) > 0 },
		run:  func(m *Model) tea.Cmd { m.verbRestore(o); return m.takePending() },
	}
	if o.restoreIn != nil {
		a.when = o.restoreIn
		a.hint = "r"
		a.desc = "restore the archived object under the cursor, or every marked person (r on the Archived screen)"
		a.foot, a.footIn, a.footLabel = 20, inList, "restore"
	}
	if o.kind == model.NoteKind {
		a.desc = "bring back the note deleted last in this session (ctrl+z does the same for the newest change; same as crm note restore)"
	}
	return a
}

func article(label string) string {
	if strings.ContainsAny(label[:1], "aeiou") {
		return "an"
	}
	return "a"
}

// cliNoun is the word the CLI and MCP use for an object kind.
func cliNoun(kind string) string {
	if kind == "organization" {
		return "org"
	}
	if kind == model.NoteKind {
		return "note"
	}
	return kind
}

// --- generic verb handlers ---

// verbDelete asks y/n, then deletes the selected object through its recorded archive hook.
func (m *Model) verbDelete(o *objDef) {
	if o.deleteAsk != nil {
		o.deleteAsk(m)
		return
	}
	id := o.selected(m)
	if id == 0 {
		return
	}
	name := truncateLabel(o.nameOf(m, id))
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("delete %s %q? y/n", o.label, name),
		yes: func() {
			if err := o.archive(m, id); err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			m.deleted[o.kind] = append(m.deleted[o.kind], id)
			m.afterWrite()
			m.setStatus(o.label+" deleted (ctrl+z undoes)", false)
			m.pendingCmd = m.showToast(o.label+" deleted · ctrl+z undoes", false)
		},
	}
}

// verbRestore brings back the object of this kind deleted last in the session. Objects that came
// back through ctrl+z are skipped.
func (m *Model) verbRestore(o *objDef) {
	if o.restoreDo != nil {
		o.restoreDo(m)
		return
	}
	for len(m.deleted[o.kind]) > 0 {
		stack := m.deleted[o.kind]
		id := stack[len(stack)-1]
		m.deleted[o.kind] = stack[:len(stack)-1]
		if err := o.restore(m, id); err != nil {
			continue
		}
		m.afterWrite()
		if o.showRow != nil {
			o.showRow(m, id)
		}
		m.setStatus(o.label+" restored", false)
		m.pendingCmd = m.showToast(o.label+" restored", false)
		return
	}
	m.setStatus("no deleted "+o.label+" to restore in this session", true)
}
