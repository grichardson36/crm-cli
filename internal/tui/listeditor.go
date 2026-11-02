package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// The list editor (patterns/list-editor-standard): ONE screen that edits every user-managed list
// (deal stages, deal roles, note types, relation kinds). It is driven by a descriptor (listDesc):
// the title, the word for an item, the storage functions, optional per-item cells and the removal
// rules. The keys are the canonical ones:
//
//	a                    add an item inline (type the name, enter saves)
//	enter, e             edit the cell under the cursor (the name by default, rename inline)
//	tab, shift+tab       move across the cells of the row
//	shift+up, shift+down move the item in the order
//	d                    remove (several with space or ctrl+a). Items in use ask where their records go
//	space, ctrl+a        select, select all
//	ctrl+z, ctrl+y       undo, redo (a removal is ONE step)
//	esc                  close
//
// A new list is one more descriptor in listeditor_lists.go.

type listPhase int

const (
	lpBrowse listPhase = iota
	lpAdd
	lpRename
	lpCell
	lpPick
	lpConfirm
)

// listItem is one row of a list.
type listItem struct {
	name  string
	group string   // key of the group the item sits in, "" for a list without groups
	cells []string // value of each cell of the descriptor, in order
	inUse int      // records that use the item
	// locked is the reason the item cannot be renamed or removed, empty when it can.
	locked string
}

func (it listItem) id() string { return it.group + "\x00" + it.name }

// listGroup is a divider of a grouped list.
type listGroup struct{ key, title string }

// listCellDesc declares a per-item cell edited with tab and enter. The spec comes from the field
// vocabulary (model.FieldSpec) so the check is the one every other editor uses.
type listCellDesc struct {
	id    string
	label string
	spec  model.FieldSpec
}

// listRemoveOpts is what the removal of items that are in use may do besides moving them to another
// item. A zero value offers only the other items.
type listRemoveOpts struct {
	blank, keep, entirely             bool
	blankLabel, keepLabel, entirelyLb string
}

// listDest is where the records of removed items go. All false and to empty means nothing uses them.
type listDest struct {
	to                     string
	blank, keep, entirely  bool
}

// listDesc declares a list.
type listDesc struct {
	id     string // the id of openListEditor: deal-stages, deal-roles, note-types, relation-kinds
	title  string
	noun   string // one item: "stage"
	plural string // "stages"
	usedBy string // what uses an item: "deals"
	groups []listGroup
	cells  []listCellDesc
	// load lists the items in order, groups in the order of groups.
	load func(m *Model) ([]listItem, error)
	// add appends an item to a group and records the undo step.
	add func(m *Model, group, name string) error
	// rename changes the name and records the undo step. It returns what it did for the status line.
	rename func(m *Model, it listItem, to string) (string, error)
	// setCell sets one cell and records the undo step.
	setCell func(m *Model, it listItem, cell, value string) error
	// reorder gives the new order of the item names of one group and records the undo step.
	reorder func(m *Model, group string, names []string) error
	// removeOpts checks the rules for removing these items (all is the whole list) and says what the
	// records of items in use may do. An error blocks the removal with its message.
	removeOpts func(m *Model, targets, all []listItem) (listRemoveOpts, error)
	// remove removes the items as ONE undo step and returns what it did for the status line.
	remove func(m *Model, targets []listItem, dest listDest) (string, error)
}

// listEditor is the state of an open editor.
type listEditor struct {
	d      *listDesc
	items  []listItem
	at     []int // for each overlay row, the index in items, -1 for a divider
	marked map[string]bool
	phase  listPhase
	col    int // 0 is the name, n is cell n-1
	in     textinput.Model
	choice int // option index of a choice cell, cursor of the destination picker
	// addGroup is the group an inline add goes to.
	addGroup string
	pick     []listPick
	pend     *listPending
	msg      string
	msgErr   bool
	o        *overlay
	// rowsCache are the overlay rows built by buildRows.
	rowsCache []overlayItem
}

type listPick struct {
	label string
	dest  listDest
}

type listPending struct {
	targets []listItem
	dest    listDest
	prompt  string
}

// openListEditor opens the editor of a list by id: "deal-stages", "deal-roles", "note-types" or
// "relation-kinds". The Settings rows, the bar and the palette call it.
func (m *Model) openListEditor(id string) tea.Cmd {
	d := m.listDescFor(id)
	if d == nil {
		m.setStatus("unknown list "+id, true)
		return nil
	}
	le := &listEditor{d: d, marked: map[string]bool{}}
	if err := le.load(m); err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	return le.show(m, "")
}

// listEditorIDs are the ids openListEditor takes, in the order Settings shows them.
var listEditorIDs = []string{"deal-stages", "deal-roles", "note-types", "relation-kinds"}

func (le *listEditor) load(m *Model) error {
	items, err := le.d.load(m)
	if err != nil {
		return err
	}
	le.items = items
	keep := map[string]bool{}
	for _, it := range items {
		if le.marked[it.id()] {
			keep[it.id()] = true
		}
	}
	le.marked = keep
	le.buildRows()
	return nil
}

// buildRows makes the overlay rows: a divider per group, then the items.
func (le *listEditor) buildRows() {
	var rows []overlayItem
	le.at = le.at[:0]
	add := func(i int) {
		it := le.items[i]
		rows = append(rows, overlayItem{id: it.id(), label: it.name, value: it.name})
		le.at = append(le.at, i)
	}
	if len(le.d.groups) == 0 {
		for i := range le.items {
			add(i)
		}
	} else {
		for _, g := range le.d.groups {
			rows = append(rows, overlayItem{id: "divider:" + g.key, label: g.title, divider: true})
			le.at = append(le.at, -1)
			for i, it := range le.items {
				if it.group == g.key {
					add(i)
				}
			}
		}
	}
	if le.o != nil {
		le.o.items = rows
		le.o.refilter()
	}
	le.rowsCache = rows
}

// show opens (or reopens) the overlay on the item with id focus.
func (le *listEditor) show(m *Model, focus string) tea.Cmd {
	le.buildRows()
	o := m.newOverlay(le.d.title, le.rowsCache, false)
	o.noFilter = true
	o.le = le
	o.onMsg = le.key
	o.draw = le.view
	o.extraTop = 1 // the column heading line
	o.onPick = func(m *Model, it overlayItem) { m.pendingCmd = le.show(m, it.id) }
	le.o = o
	le.focus(m, focus)
	return m.showOverlay(o)
}

// focus puts the cursor on the item with id, else keeps it in range and off a divider.
func (le *listEditor) focus(m *Model, id string) {
	o := le.o
	if id != "" {
		for i, idx := range o.shown {
			if o.items[idx].id == id {
				o.cursor = i
			}
		}
	}
	o.cursor = clampInt(o.cursor, 0, max(0, len(o.shown)-1))
	o.move(0, m.overlayPage())
}

// cur is the item under the cursor.
func (le *listEditor) cur() (listItem, bool) {
	o := le.o
	if o == nil || o.cursor < 0 || o.cursor >= len(o.shown) {
		return listItem{}, false
	}
	i := le.at[o.shown[o.cursor]]
	if i < 0 || i >= len(le.items) {
		return listItem{}, false
	}
	return le.items[i], true
}

// targets are the selected items, else the item under the cursor.
func (le *listEditor) targets() []listItem {
	var out []listItem
	for _, it := range le.items {
		if le.marked[it.id()] {
			out = append(out, it)
		}
	}
	if len(out) == 0 {
		if it, ok := le.cur(); ok {
			out = append(out, it)
		}
	}
	return out
}

func (le *listEditor) say(msg string, isErr bool) { le.msg, le.msgErr = msg, isErr }

func (le *listEditor) fail(err error) { le.say(err.Error(), true) }

// reload re-reads the list after a change and keeps the cursor on focus.
func (le *listEditor) reload(m *Model, focus string) {
	if err := le.load(m); err != nil {
		le.fail(err)
		return
	}
	le.focus(m, focus)
}

// writeOK says whether the list may change, and why not.
func (le *listEditor) writeOK(m *Model) bool {
	if m.readOnly {
		le.say("read-only: lists cannot be changed", true)
		return false
	}
	return true
}

func (le *listEditor) startInput(prompt, value string, width int) {
	le.in = newInput(prompt)
	le.in.SetValue(value)
	le.in.CursorEnd()
	le.in.SetWidth(max(8, width))
}

// key handles every key while the editor is open. It always uses the key.
func (le *listEditor) key(m *Model, msg tea.KeyPressMsg) bool {
	k := keyName(msg)
	switch le.phase {
	case lpAdd, lpRename:
		le.textKey(m, msg, k)
	case lpCell:
		le.cellKey(m, msg, k)
	case lpPick:
		le.pickKey(m, k)
	case lpConfirm:
		le.confirmKey(m, k)
	default:
		le.browseKey(m, k)
	}
	return true
}

func (le *listEditor) browseKey(m *Model, k string) {
	o := le.o
	page := m.overlayPage()
	switch k {
	case "esc", "ctrl+c":
		m.closeOverlay()
	case "up":
		o.move(-1, page)
	case "down":
		o.move(1, page)
	case "pgup":
		o.move(-page, page)
	case "pgdown":
		o.move(page, page)
	case "home":
		o.move(-len(o.shown), page)
	case "end":
		o.move(len(o.shown), page)
	case "shift+up":
		le.shift(m, -1)
	case "shift+down":
		le.shift(m, 1)
	case " ":
		if it, ok := le.cur(); ok {
			if le.marked[it.id()] {
				delete(le.marked, it.id())
			} else {
				le.marked[it.id()] = true
			}
			o.move(1, page)
		}
	case "ctrl+a":
		all := len(le.items) > 0
		for _, it := range le.items {
			if !le.marked[it.id()] {
				all = false
			}
		}
		le.marked = map[string]bool{}
		if !all {
			for _, it := range le.items {
				le.marked[it.id()] = true
			}
		}
	case "tab", "shift+tab":
		if n := len(le.d.cells) + 1; n > 1 {
			if k == "tab" {
				le.col = (le.col + 1) % n
			} else {
				le.col = (le.col + n - 1) % n
			}
		}
	case "a":
		le.startAdd(m)
	case "enter", "e":
		le.startEdit(m)
	case "d":
		le.startRemove(m)
	case "ctrl+z":
		m.pendingCmd = m.undoLast()
		le.afterHistory(m)
	case "ctrl+y":
		m.pendingCmd = m.redoLast()
		le.afterHistory(m)
	}
}

// afterHistory re-reads the list after an undo or redo, which may have changed anything.
func (le *listEditor) afterHistory(m *Model) {
	id := ""
	if it, ok := le.cur(); ok {
		id = it.id()
	}
	le.say("", false)
	le.reload(m, id)
}

// shift moves the item under the cursor one place inside its group.
func (le *listEditor) shift(m *Model, d int) {
	it, ok := le.cur()
	if !ok || !le.writeOK(m) {
		return
	}
	var names []string
	pos := -1
	for _, x := range le.items {
		if x.group == it.group {
			if x.name == it.name {
				pos = len(names)
			}
			names = append(names, x.name)
		}
	}
	to := pos + d
	if pos < 0 || to < 0 || to >= len(names) {
		return
	}
	names[pos], names[to] = names[to], names[pos]
	if le.d.reorder == nil {
		return
	}
	if err := le.d.reorder(m, it.group, names); err != nil {
		le.fail(err)
		return
	}
	m.afterWrite()
	le.say("moved "+it.name+" (ctrl+z undoes)", false)
	le.reload(m, it.id())
}

func (le *listEditor) startAdd(m *Model) {
	if !le.writeOK(m) {
		return
	}
	le.addGroup = ""
	if it, ok := le.cur(); ok {
		le.addGroup = it.group
	} else if len(le.d.groups) > 0 {
		le.addGroup = le.d.groups[0].key
	}
	le.startInput("+ ", "", m.width/2)
	le.in.Placeholder = "name of the new " + le.d.noun
	le.phase = lpAdd
	m.pendingCmd = le.in.Focus()
	le.say("", false)
}

func (le *listEditor) startEdit(m *Model) {
	it, ok := le.cur()
	if !ok || !le.writeOK(m) {
		return
	}
	if le.col == 0 {
		if it.locked != "" {
			le.say(it.name+": "+it.locked, true)
			return
		}
		le.startInput("", it.name, 30)
		le.phase = lpRename
		m.pendingCmd = le.in.Focus()
		le.say("", false)
		return
	}
	c := le.d.cells[le.col-1]
	val := it.cells[le.col-1]
	le.say("", false)
	if c.spec.Type == model.FieldChoice {
		le.choice = 0
		for i, o := range c.spec.Choices {
			if strings.EqualFold(o.Value, val) || strings.EqualFold(o.Label, val) {
				le.choice = i
			}
		}
		le.phase = lpCell
		return
	}
	le.startInput("", val, 30)
	le.phase = lpCell
	m.pendingCmd = le.in.Focus()
}

// textKey is a key while the inline input of an add or a rename is open.
func (le *listEditor) textKey(m *Model, msg tea.KeyPressMsg, k string) {
	switch k {
	case "esc":
		le.phase = lpBrowse
		le.say("", false)
	case "enter":
		le.commitText(m)
	default:
		var cmd tea.Cmd
		le.in, cmd = le.in.Update(msg)
		m.pendingCmd = cmd
	}
}

func (le *listEditor) commitText(m *Model) {
	name := strings.TrimSpace(le.in.Value())
	if name == "" {
		le.phase = lpBrowse
		return
	}
	if le.phase == lpAdd {
		if err := le.d.add(m, le.addGroup, name); err != nil {
			le.fail(err)
			return
		}
		m.afterWrite()
		le.phase = lpBrowse
		le.say("added "+le.d.noun+" "+model.CleanVocabName(name)+" (ctrl+z undoes)", false)
		focus := ""
		if err := le.load(m); err == nil {
			for _, it := range le.items {
				if it.group == le.addGroup && strings.EqualFold(it.name, model.CleanVocabName(name)) {
					focus = it.id()
				}
			}
		}
		le.focus(m, focus)
		return
	}
	it, ok := le.cur()
	if !ok {
		le.phase = lpBrowse
		return
	}
	if name == it.name {
		le.phase = lpBrowse
		return
	}
	msg, err := le.d.rename(m, it, name)
	if err != nil {
		le.fail(err)
		return
	}
	m.afterWrite()
	le.phase = lpBrowse
	le.say(msg+" (ctrl+z undoes)", false)
	le.reload(m, (listItem{group: it.group, name: model.CleanVocabName(name)}).id())
}

// cellKey is a key while a cell is edited: left and right choose an option, enter saves.
func (le *listEditor) cellKey(m *Model, msg tea.KeyPressMsg, k string) {
	c := le.d.cells[le.col-1]
	choice := c.spec.Type == model.FieldChoice
	switch {
	case k == "esc":
		le.phase = lpBrowse
		le.say("", false)
	case k == "enter":
		le.commitCell(m, c)
	case choice && (k == "left" || k == "up" || k == "shift+tab"):
		n := len(c.spec.Choices)
		le.choice = (le.choice + n - 1) % n
	case choice && (k == "right" || k == "down" || k == "tab"):
		le.choice = (le.choice + 1) % len(c.spec.Choices)
	case !choice:
		var cmd tea.Cmd
		le.in, cmd = le.in.Update(msg)
		m.pendingCmd = cmd
	}
}

func (le *listEditor) commitCell(m *Model, c listCellDesc) {
	it, ok := le.cur()
	if !ok {
		le.phase = lpBrowse
		return
	}
	raw := le.in.Value()
	if c.spec.Type == model.FieldChoice {
		raw = c.spec.Choices[le.choice].Value
	}
	res := validate.Field(c.spec, raw, validate.Options{})
	if errs := res.Errors(); len(errs) > 0 {
		le.say(errs[0].Fix+": "+errs[0].Expected, true)
		return
	}
	if err := le.d.setCell(m, it, c.id, res.Value); err != nil {
		le.fail(err)
		le.phase = lpBrowse
		return
	}
	m.afterWrite()
	le.phase = lpBrowse
	le.say(c.label+" of "+it.name+" is now "+res.Value+" (ctrl+z undoes)", false)
	le.reload(m, it.id())
}

// --- removal ---

func (le *listEditor) isTarget(ts []listItem, it listItem) bool {
	for _, t := range ts {
		if t.id() == it.id() {
			return true
		}
	}
	return false
}

func namesOf(ts []listItem) string {
	var n []string
	for i, t := range ts {
		if i == 3 {
			n = append(n, fmt.Sprintf("and %d more", len(ts)-3))
			break
		}
		n = append(n, t.name)
	}
	return strings.Join(n, ", ")
}

// startRemove is d. An item nothing uses goes at once (several ask first). Items in use ask where
// their records go, then ask for a y.
func (le *listEditor) startRemove(m *Model) {
	targets := le.targets()
	if len(targets) == 0 || !le.writeOK(m) {
		return
	}
	d := le.d
	total := 0
	for _, t := range targets {
		if t.group != targets[0].group {
			le.say("select items from one group to remove them together", true)
			return
		}
		if t.locked != "" {
			le.say(t.name+": "+t.locked, true)
			return
		}
		total += t.inUse
	}
	opts, err := d.removeOpts(m, targets, le.items)
	if err != nil {
		le.fail(err)
		return
	}
	if total == 0 {
		if len(targets) == 1 {
			le.apply(m, targets, listDest{})
			return
		}
		le.ask(targets, listDest{}, fmt.Sprintf("Remove %d %s (%s)? y/n", len(targets), d.plural, namesOf(targets)))
		return
	}
	var picks []listPick
	for _, it := range le.items {
		if it.group == targets[0].group && !le.isTarget(targets, it) {
			picks = append(picks, listPick{label: "Move them to " + it.name, dest: listDest{to: it.name}})
		}
	}
	if opts.blank {
		picks = append(picks, listPick{label: opts.blankLabel, dest: listDest{blank: true}})
	}
	if opts.keep {
		picks = append(picks, listPick{label: opts.keepLabel, dest: listDest{keep: true}})
	}
	if opts.entirely {
		picks = append(picks, listPick{label: opts.entirelyLb, dest: listDest{entirely: true}})
	}
	if len(picks) == 0 {
		le.say(fmt.Sprintf("%d %s use %s and there is no other %s to move them to", total, d.usedBy, namesOf(targets), d.noun), true)
		return
	}
	le.pick, le.choice = picks, 0
	le.pend = &listPending{targets: targets}
	le.phase = lpPick
	le.say("", false)
}

func (le *listEditor) pickKey(m *Model, k string) {
	n := len(le.pick)
	switch k {
	case "esc":
		le.phase = lpBrowse
		le.pend = nil
		le.say("cancelled", false)
	case "up":
		le.choice = (le.choice + n - 1) % n
	case "down":
		le.choice = (le.choice + 1) % n
	case "enter":
		total := 0
		for _, t := range le.pend.targets {
			total += t.inUse
		}
		p := le.pick[le.choice]
		le.ask(le.pend.targets, p.dest, fmt.Sprintf("Remove %s (%d %s). %s? y/n", namesOf(le.pend.targets), total, le.d.usedBy, p.label))
	}
}

// ask shows the one-key confirmation.
func (le *listEditor) ask(targets []listItem, dest listDest, prompt string) {
	le.pend = &listPending{targets: targets, dest: dest, prompt: prompt}
	le.phase = lpConfirm
}

func (le *listEditor) confirmKey(m *Model, k string) {
	switch k {
	case "y", "Y":
		p := le.pend
		le.pend = nil
		le.apply(m, p.targets, p.dest)
	case "n", "N", "esc":
		le.phase = lpBrowse
		le.pend = nil
		le.say("cancelled", false)
	}
}

// apply removes the items as one undo step.
func (le *listEditor) apply(m *Model, targets []listItem, dest listDest) {
	le.phase = lpBrowse
	msg, err := le.d.remove(m, targets, dest)
	if err != nil {
		le.fail(err)
		return
	}
	m.afterWrite()
	for _, t := range targets {
		delete(le.marked, t.id())
	}
	le.say(msg+" (ctrl+z undoes)", false)
	le.reload(m, "")
}

// --- drawing ---

// hints are the footer hints of the list editor for its phase (hints.go): verbs, navigation, specifics.
func (le *listEditor) hints() []hintItem {
	switch le.phase {
	case lpAdd, lpRename:
		return []hintItem{fk("enter", "save"), fk("esc", "cancel")}
	case lpCell:
		if le.d.cells[le.col-1].spec.Type == model.FieldChoice {
			return []hintItem{fk("enter", "save"), fk("esc", "cancel"), fk("left/right", "choose")}
		}
		return []hintItem{fk("enter", "save"), fk("esc", "cancel")}
	case lpPick:
		return []hintItem{fk("enter", "ok"), fk("esc", "cancel"), fk("up/down", "choose")}
	case lpConfirm:
		return []hintItem{fk("y", "yes"), fk("n", "no")}
	}
	h := []hintItem{fk("a", "add"), fk("enter", "edit"), fk("d", "remove"), fk("space", "select"), ak("undo", "undo"), fk("esc", "close"), fk("shift+up/down", "move")}
	if len(le.d.cells) > 0 {
		h = append(h, fk("tab", "cell"))
	}
	return h
}

// view draws the list editor with its key hints on the last line.
func (le *listEditor) view(m *Model) string {
	return le.body(m) + "\n" + m.renderHintItems(le.hints())
}

func (le *listEditor) body(m *Model) string {
	o := le.o
	w := m.width
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().MaxWidth(w).Render(m.st.Head.Render(le.d.title)) + "\n")
	switch {
	case le.phase == lpConfirm && le.pend != nil:
		b.WriteString(m.st.Warn.Render(ansi.Truncate(le.pend.prompt, w, "…")) + "\n")
	case le.msg != "" && le.msgErr:
		b.WriteString(m.st.Err.Render(ansi.Truncate(le.msg, w, "…")) + "\n")
	case le.msg != "":
		b.WriteString(m.st.Dim.Render(ansi.Truncate(le.msg, w, "…")) + "\n")
	default:
		b.WriteString(m.st.Dim.Render(ansi.Truncate(le.defaultNote(), w, "…")) + "\n")
	}
	if le.phase == lpPick {
		b.WriteString(le.pickView(m))
		return b.String()
	}
	nameW := 12
	for _, it := range le.items {
		nameW = max(nameW, ansi.StringWidth(it.name)+2)
	}
	nameW = min(nameW, max(12, w/3))
	cellW := make([]int, len(le.d.cells))
	for i, c := range le.d.cells {
		cellW[i] = ansi.StringWidth(c.label) + 2
		for _, it := range le.items {
			if i < len(it.cells) {
				cellW[i] = max(cellW[i], ansi.StringWidth(it.cells[i])+4)
			}
		}
	}
	head := fit("", 2) + fit("name", nameW)
	for i, c := range le.d.cells {
		head += fit(c.label, cellW[i])
	}
	b.WriteString(m.st.Dim.Render(ansi.Truncate(head, w, "")) + "\n")
	page := m.overlayPage() - 2
	if len(o.shown) == 0 && le.phase != lpAdd {
		b.WriteString(m.st.Dim.Render("The list is empty."))
		return b.String()
	}
	end := min(len(o.shown), o.offset+max(1, page))
	var lines []string
	for i := o.offset; i < end; i++ {
		row := o.items[o.shown[i]]
		if row.divider {
			lines = append(lines, m.st.Dim.Render(ruleLine(row.label, "", w)))
		} else {
			lines = append(lines, le.rowLine(m, i, nameW, cellW))
		}
		if le.phase == lpAdd && i == le.lastOfGroup(end) {
			lines = append(lines, le.addLine(m))
		}
	}
	if le.phase == lpAdd && len(o.shown) == 0 {
		lines = append(lines, le.addLine(m))
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}

// lastOfGroup is the shown index after which the inline add row is drawn: the last row of the
// group the add goes to, within the visible rows.
func (le *listEditor) lastOfGroup(end int) int {
	o := le.o
	last := -1
	for i := o.offset; i < end; i++ {
		idx := le.at[o.shown[i]]
		if idx >= 0 && le.items[idx].group == le.addGroup {
			last = i
		}
	}
	if last < 0 {
		return end - 1
	}
	return last
}

func (le *listEditor) addLine(m *Model) string {
	return lipgloss.NewStyle().MaxWidth(m.width).Render("  " + le.in.View())
}

func (le *listEditor) defaultNote() string {
	n := len(le.marked)
	if n > 0 {
		return fmt.Sprintf("%d selected", n)
	}
	if len(le.d.cells) > 0 {
		return "each row has cells that are edited in place"
	}
	return "renaming updates the records that use the name"
}

func (le *listEditor) rowLine(m *Model, i, nameW int, cellW []int) string {
	o := le.o
	it := le.items[le.at[o.shown[i]]]
	onRow := i == o.cursor
	mark := "  "
	if le.marked[it.id()] {
		mark = "* "
	}
	name := it.name
	nameCell := fit(name, nameW)
	if onRow && le.phase == lpRename {
		nameCell = fit(le.in.View(), nameW)
	} else if onRow && le.col == 0 && le.phase == lpBrowse && len(le.d.cells) > 0 {
		nameCell = fit("["+name+"]", nameW)
	}
	line := mark + nameCell
	for ci, c := range le.d.cells {
		val := ""
		if ci < len(it.cells) {
			val = it.cells[ci]
		}
		text := val
		active := onRow && le.col == ci+1
		switch {
		case active && le.phase == lpCell && c.spec.Type == model.FieldChoice:
			var opts []string
			for oi, op := range c.spec.Choices {
				if oi == le.choice {
					opts = append(opts, "["+op.Label+"]")
				} else {
					opts = append(opts, op.Label)
				}
			}
			text = strings.Join(opts, " ")
		case active && le.phase == lpCell:
			text = le.in.View()
		case active:
			text = "[" + val + "]"
		}
		line += fit(text, max(cellW[ci], ansi.StringWidth(text)+1))
	}
	use := ""
	switch {
	case it.locked != "":
		use = it.locked
	case it.inUse > 0:
		use = fmt.Sprintf("%d %s", it.inUse, le.d.usedBy)
	default:
		use = "not used"
	}
	room := m.width - ansi.StringWidth(line) - 1
	if room > 4 {
		line += " " + m.st.Dim.Render(ansi.Truncate(use, room, "…"))
		if onRow {
			line = fit(ansi.Strip(line), m.width)
		}
	}
	if onRow {
		return m.st.Sel.Render(fit(ansi.Strip(line), m.width))
	}
	return ansi.Truncate(line, m.width, "…")
}

func (le *listEditor) pickView(m *Model) string {
	total := 0
	for _, t := range le.pend.targets {
		total += t.inUse
	}
	var b strings.Builder
	b.WriteString(m.st.Head.Render(fmt.Sprintf("%d %s use %s. Where do they go?", total, le.d.usedBy, namesOf(le.pend.targets))) + "\n")
	for i, p := range le.pick {
		line := fit("  "+p.label, m.width)
		if i == le.choice {
			b.WriteString(m.st.Sel.Render(line))
		} else {
			b.WriteString(line)
		}
		if i < len(le.pick)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
