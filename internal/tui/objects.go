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

// verbKeys are the standard keys. They are the same in every object view (patterns/same-key-same-verb).
// Mark is space, undo and redo are ctrl+z and ctrl+y, and open is enter.
var verbKeys = map[verb][]string{
	vAdd:     {"a", "+"},
	vOpen:    {"enter"},
	vEdit:    {"enter", "e"},
	vLink:    {"L"},
	vUnlink:  {"U"},
	vDelete:  {"d"},
	vRestore: {"u"},
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
	return o.label + " (" + strings.Join(o.fields, " | ") + "): "
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
	return []*objDef{orgDef(), dealDef(), taskDef(), personDef()}
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
		fields:   []string{"name", "domain", "industry", "notes"},
		cols:     []string{"name", "industry", "domain"},
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
			o, err := m.hCreateOrg(model.CreateOrgInput{Name: text})
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
				return m.view == viewTasks && m.focus == focusList || (key == "t" && whenPerson(m))
			}
			return m.view == viewTasks && m.focus == focusList || m.focus == focusDetail && v == vDelete
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
			if !whenPerson(m) || m.view == viewCalendar || m.view == viewSync || m.view == viewMerge {
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
				if key == "+" {
					return m.view < numViews && m.view != viewCalendar && m.view != viewTasks
				}
				// a is the Add row key in the detail form, so it adds a person only from a people list.
				switch m.view {
				case viewHome, viewPeople, viewDue, viewBirthdays:
					return m.focus == focusList
				}
				return false
			case vDelete:
				// # archives from any pane, d only in the list (in the detail form it deletes a row).
				return key == "#" || m.focus == focusList
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
		if o.kind == "deal" {
			out = append(out, dealStageAction())
		}
	}
	return append(out, linkActions()...)
}

func (o *objDef) keysFor(v verb) []string {
	switch {
	case o.kind == "task" && v == vAdd:
		return []string{"a", "+", "t"}
	case o.kind == "person" && v == vDelete:
		return []string{"d", "#"}
	case v == vEdit && o.kind != "organization" && o.kind != "deal":
		return []string{"enter"}
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
		id: o.actionID(vAdd), title: "Add " + o.label, group: o.group, hint: "a", keys: o.keysFor(vAdd),
		desc: desc + ". a or + in the list, or from the palette", write: true,
		keyFor: o.keyFor(vAdd), foot: footAdd, footIn: inList, footLabel: "add",
		footIf: func(m *Model) bool { return o.keyHere(m, vAdd, "a") },
		run:    func(m *Model) tea.Cmd { return o.add(m) },
	}
	if o.kind == "person" {
		a.title, a.hint, a.desc = "Add person", "a", "create a contact: opens a blank form, saved when you enter a name. a or + in a people list, or from the palette"
	}
	if o.kind == "task" {
		a.desc = "add a task: title | optional due date. a or + in the Tasks view, t on a person, or from the palette (same as crm task add)"
	}
	return a
}

func (o *objDef) editAction() action {
	a := action{
		id: o.actionID(vEdit), title: "Edit " + o.label, group: o.group, hint: "enter", keys: o.keysFor(vEdit),
		desc:   "edit the selected " + o.label + ", fields you empty are cleared (same as crm " + cliNoun(o.kind) + " edit). enter",
		write:  true,
		keyFor: o.keyFor(vEdit), foot: footEdit, footIn: inList, footLabel: "edit",
		when: func(m *Model) bool { return o.selected(m) != 0 },
		run:  func(m *Model) tea.Cmd { return o.edit(m, o.selected(m)) },
	}
	if len(a.keys) > 1 {
		a.desc += " or e"
	}
	if o.kind == "person" {
		a.write = false
		a.desc = "move into the detail form of the selected person to edit it (enter)"
		a.foot = 0
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
	if o.kind == "person" {
		a.desc = "delete (archive) the person or all marked (asks y/n, restore from the Archived view). # also"
		a.footLabel = "archive"
		a.foot = 35
		a.footIf = whenMarks
	}
	return a
}

func (o *objDef) restoreAction() action {
	a := action{
		id: o.actionID(vRestore), title: "Restore " + o.label, group: o.group, hint: "u", keys: o.keysFor(vRestore),
		desc:  "bring back the " + o.label + " deleted last in this session (u in its list, ctrl+z does the same for the newest change; same as crm " + cliNoun(o.kind) + " restore)",
		write: true, keyFor: o.keyFor(vRestore), foot: footRestore, footIn: inList, footLabel: "restore",
		when: func(m *Model) bool { return len(m.deleted[o.kind]) > 0 },
		run:  func(m *Model) tea.Cmd { m.verbRestore(o); return m.takePending() },
	}
	if o.restoreIn != nil {
		a.when = o.restoreIn
		a.desc = "restore the archived person or all marked (u in the Archived view)"
		a.foot = 20
	} else {
		a.footIf = func(m *Model) bool { return len(m.deleted[o.kind]) > 0 && o.keyHere(m, vRestore, "u") }
	}
	return a
}

func dealStageAction() action {
	return action{id: "deal-stage", title: "Set deal stage", group: "Deals", hint: "s", keys: []string{"s"},
		desc:  "pick the stage of the selected deal: lead, prospect, proposal, negotiation, won, lost",
		write: true, foot: 21, footIn: inList, footLabel: "stage",
		when: whenDealSel, run: func(m *Model) tea.Cmd { m.pickDealStage(m.selDeal()); return nil }}
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
