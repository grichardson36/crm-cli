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

// The list editor (patterns/list-editor-standard): the right pane of a Settings row that edits a
// user-managed list (deal stages, deal roles, note types, relation kinds, ignored company texts).
// It lives only in that pane: no popup, no screen of its own, no palette entry and no key outside
// Settings. enter on the row enters the pane, esc leaves it. It is driven by a descriptor (listDesc):
// the title, the word for an item, the storage functions, optional per-item cells, the removal rules
// and the built-in defaults. The keys, each shown in the footer only when it works on the row:
//
//	up, down             select
//	a                    add an item inline (type the name, enter saves)
//	enter, e             rename (the cell under the cursor on a list with text cells)
//	tab                  cycle the value of the row (outcome, mark contacted) or move across text cells
//	shift+up, shift+down move the item in the order
//	d                    remove. Items in use ask where their records go
//	shift+r              reset to the built-in defaults, after a confirm that lists the changes
//	ctrl+z, ctrl+y       undo, redo (a removal or a reset is ONE step)
//	esc, left, shift+tab leave the pane
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
	// noRename says the name cannot change although the item can be removed.
	noRename bool
}

func (it listItem) id() string { return it.group + "\x00" + it.name }

// listGroup is a divider of a grouped list.
type listGroup struct{ key, title string }

// listCellDesc declares a per-item cell. The spec comes from the field vocabulary (model.FieldSpec)
// so the check is the one every other editor uses.
type listCellDesc struct {
	id    string
	label string
	spec  model.FieldSpec
	// cycle makes tab step the cell through its choices and save (outcome, mark contacted).
	cycle bool
	// readonly cells are shown and never edited.
	readonly bool
}

// listRemoveOpts is what the removal of items that are in use may do besides moving them to another
// item. A zero value offers only the other items.
type listRemoveOpts struct {
	blank, keep, entirely             bool
	blankLabel, keepLabel, entirelyLb string
}

// listDest is where the records of removed items go. All false and to empty means nothing uses them.
type listDest struct {
	to                    string
	blank, keep, entirely bool
}

// listDesc declares a list.
type listDesc struct {
	id     string // deal-stages, deal-roles, note-types, relation-kinds, company-ignore (the Settings row ids)
	title  string
	noun   string // one item: "stage"
	plural string // "stages"
	usedBy string // what uses an item: "deals"
	// nameLabel heads the name column, "name" when empty.
	nameLabel string
	// noUsed leaves out the used by column (a list whose items no record uses).
	noUsed bool
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
	// defaults lists the built-in items in their default order with their default cells. A list
	// without it has no reset.
	defaults func(m *Model) []listItem
}

// listRow is a line of the list: a group divider or an item.
type listRow struct {
	divider bool
	title   string
	idx     int // index in items, -1 for a divider
}

// listEditor is the state of the list shown in the pane.
type listEditor struct {
	d      *listDesc
	items  []listItem
	rows   []listRow
	cursor int // index in rows, never on a divider
	offset int // first row drawn
	page   int // rows that fit in the pane, set when it is drawn
	marked map[string]bool
	phase  listPhase
	col    int // 0 is the name, n is cell n-1
	in     textinput.Model
	choice int // cursor of the destination picker
	// addGroup is the group an inline add goes to.
	addGroup string
	pick     []listPick
	pend     *listPending
	msg      string
	msgErr   bool
}

type listPick struct {
	label string
	dest  listDest
}

type listPending struct {
	targets []listItem
	dest    listDest
	prompt  string
	// reset is set for the confirm of a reset: steps run in order, plan is what the pane lists.
	reset bool
	plan  []string
	steps []func() error
}

// listEditorIDs are the ids of the lists, in the order Settings shows them. Each is also the id of its Settings row.
var listEditorIDs = []string{"deal-stages", "deal-roles", "note-types", "relation-kinds", "company-ignore"}

// listPaneID is the list a Settings row shows in its pane, "" for any other row.
func listPaneID(settingID string) string {
	for _, id := range listEditorIDs {
		if id == settingID {
			return id
		}
	}
	return ""
}

// paneList is the list editor of the Settings row under the cursor, nil on any other screen or row.
func (m *Model) paneList() *listEditor {
	if m.lp == nil || m.view != viewSettings || m.cmp != nil {
		return nil
	}
	return m.lp
}

// paneListFocused is the list editor while the cursor is inside its pane.
func (m *Model) paneListFocused() *listEditor {
	if le := m.paneList(); le != nil && m.focus == focusDetail {
		return le
	}
	return nil
}

// ensurePaneList makes m.lp the editor of the list id. The same list is read again and keeps its cursor.
func (m *Model) ensurePaneList(id string) {
	if m.lp != nil && m.lp.d.id == id {
		cur := ""
		if it, ok := m.lp.cur(); ok {
			cur = it.id()
		}
		if err := m.lp.load(m); err != nil {
			m.lp.fail(err)
			return
		}
		m.lp.focus(cur)
		return
	}
	d := m.listDescFor(id)
	if d == nil {
		m.lp = nil
		return
	}
	le := &listEditor{d: d, marked: map[string]bool{}}
	if err := le.load(m); err != nil {
		m.setStatus(err.Error(), true)
		m.lp = nil
		return
	}
	le.focus("")
	m.lp = le
}

// openListPane goes to the Settings row of a list with the cursor in its pane.
func (m *Model) openListPane(id string) tea.Cmd {
	m.gotoSetting(id)
	if m.view == viewSettings && m.paneList() != nil && m.lp.d.id == id {
		m.focus = focusDetail
		m.fcol = 0
		m.syncDetail()
	}
	return nil
}

// leavePane puts the cursor back on the list of settings.
func (m *Model) leavePane() {
	if le := m.paneList(); le != nil {
		le.phase, le.col, le.pend = lpBrowse, 0, nil
	}
	m.focus = focusList
	m.fcol = 0
	m.refreshDetail(true)
	m.syncDetail()
}

// paneListKey takes every key while the cursor is in a list pane. A key the pane does not use goes on
// to the registry (the palette, help, quit).
func (m *Model) paneListKey(le *listEditor, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := keyName(msg)
	if le.phase == lpBrowse && !le.usesKey(k) {
		return m.handleKey(msg)
	}
	m.pendingCmd = nil
	le.key(m, msg)
	if m.paneList() != nil {
		m.setStatus(le.msg, le.msgErr)
		m.syncDetail()
	}
	cmd := m.pendingCmd
	m.pendingCmd = nil
	return m, cmd
}

// usesKey says whether the pane handles a key while browsing.
func (le *listEditor) usesKey(k string) bool {
	switch k {
	case "esc", "left", "shift+tab", "up", "down", "pgup", "pgdown", "home", "end", "shift+up", "shift+down",
		" ", "ctrl+a", "tab", "a", "e", "enter", "d", "R", "ctrl+z", "ctrl+y":
		return true
	}
	return false
}

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

// buildRows makes the rows: a divider per group, then the items.
func (le *listEditor) buildRows() {
	le.rows = le.rows[:0]
	if len(le.d.groups) == 0 {
		for i := range le.items {
			le.rows = append(le.rows, listRow{idx: i})
		}
		return
	}
	for _, g := range le.d.groups {
		le.rows = append(le.rows, listRow{divider: true, title: g.title, idx: -1})
		for i, it := range le.items {
			if it.group == g.key {
				le.rows = append(le.rows, listRow{idx: i})
			}
		}
	}
}

// move steps the cursor over the items, skipping dividers.
func (le *listEditor) move(delta int) {
	step := 1
	if delta < 0 {
		step = -1
	}
	i := le.cursor
	for n := abs(delta); n > 0; n-- {
		j := i + step
		for j >= 0 && j < len(le.rows) && le.rows[j].divider {
			j += step
		}
		if j < 0 || j >= len(le.rows) {
			break
		}
		i = j
	}
	le.cursor = i
	le.fixOffset()
}

// settle puts the cursor in range and off a divider.
func (le *listEditor) settle() {
	le.cursor = clampInt(le.cursor, 0, max(0, len(le.rows)-1))
	if len(le.rows) == 0 || !le.rows[le.cursor].divider {
		return
	}
	for j := le.cursor; j < len(le.rows); j++ {
		if !le.rows[j].divider {
			le.cursor = j
			return
		}
	}
	for j := le.cursor; j >= 0; j-- {
		if !le.rows[j].divider {
			le.cursor = j
			return
		}
	}
}

// fixOffset keeps the cursor row, and the divider above it, in the window.
func (le *listEditor) fixOffset() {
	page := max(1, le.page)
	top := le.cursor
	if top > 0 && le.rows[top-1].divider {
		top--
	}
	if top < le.offset {
		le.offset = top
	}
	if le.cursor >= le.offset+page {
		le.offset = le.cursor - page + 1
	}
	le.offset = max(0, min(le.offset, max(0, len(le.rows)-1)))
}

// focus puts the cursor on the item with id, else keeps it in range and off a divider.
func (le *listEditor) focus(id string) {
	if id != "" {
		for i, r := range le.rows {
			if !r.divider && le.items[r.idx].id() == id {
				le.cursor = i
			}
		}
	}
	le.settle()
	le.fixOffset()
}

// cur is the item under the cursor.
func (le *listEditor) cur() (listItem, bool) {
	if le.cursor < 0 || le.cursor >= len(le.rows) || le.rows[le.cursor].divider {
		return listItem{}, false
	}
	return le.items[le.rows[le.cursor].idx], true
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
	le.focus(focus)
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

// key handles a key of the pane.
func (le *listEditor) key(m *Model, msg tea.KeyPressMsg) {
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
}

// bodyRows is how many item rows the pane shows: its height minus the title and the column heading.
func (le *listEditor) bodyRows(m *Model) int { return max(1, m.detail.Height()-2) }

func (le *listEditor) browseKey(m *Model, k string) {
	le.page = le.bodyRows(m)
	page := le.page
	switch k {
	case "esc", "left", "shift+tab":
		m.leavePane()
	case "up":
		le.move(-1)
	case "down":
		le.move(1)
	case "pgup":
		le.move(-page)
	case "pgdown":
		le.move(page)
	case "home":
		le.move(-len(le.rows))
	case "end":
		le.move(len(le.rows))
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
			le.move(1)
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
	case "tab":
		le.tab(m)
	case "a":
		le.startAdd(m)
	case "enter", "e":
		le.startEdit(m)
	case "d":
		le.startRemove(m)
	case "R":
		le.startReset(m)
	case "ctrl+z":
		m.pendingCmd = m.undoLast()
		le.afterHistory(m)
	case "ctrl+y":
		m.pendingCmd = m.redoLast()
		le.afterHistory(m)
	}
}

// cycleCell is the index of the cell that tab steps through, -1 when the list has none.
func (d *listDesc) cycleCell() int {
	for i, c := range d.cells {
		if c.cycle {
			return i
		}
	}
	return -1
}

// editCols says whether the list has text cells that tab moves across.
func (d *listDesc) editCols() bool {
	for _, c := range d.cells {
		if !c.cycle && !c.readonly {
			return true
		}
	}
	return false
}

// tab steps the cycle cell of the row to its next choice, or moves across the text cells.
func (le *listEditor) tab(m *Model) {
	it, ok := le.cur()
	if !ok {
		return
	}
	if ci := le.d.cycleCell(); ci >= 0 {
		if !le.writeOK(m) {
			return
		}
		c := le.d.cells[ci]
		next := 0
		for i, o := range c.spec.Choices {
			if strings.EqualFold(o.Value, it.cells[ci]) {
				next = (i + 1) % len(c.spec.Choices)
			}
		}
		le.applyCell(m, it, c, c.spec.Choices[next].Value)
		return
	}
	if !le.d.editCols() || it.locked != "" {
		le.col = 0
		return
	}
	n := len(le.d.cells) + 1
	le.col = (le.col + 1) % n
	for le.col > 0 && le.d.cells[le.col-1].readonly {
		le.col = (le.col + 1) % n
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
	if !ok || le.d.reorder == nil || !le.writeOK(m) {
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
	le.startInput("+ ", "", m.detail.Width()-4)
	le.in.Placeholder = le.d.noun
	le.phase = lpAdd
	m.pendingCmd = le.in.Focus()
	le.say("", false)
}

func (le *listEditor) startEdit(m *Model) {
	it, ok := le.cur()
	if !ok || !le.writeOK(m) {
		return
	}
	le.say("", false)
	if it.locked != "" {
		le.say(it.name+": "+it.locked, true)
		return
	}
	if le.col == 0 {
		if it.noRename {
			le.say(it.name+": built-in, stays", true)
			return
		}
		le.startInput("", it.name, 30)
		le.phase = lpRename
		m.pendingCmd = le.in.Focus()
		return
	}
	le.startInput("", it.cells[le.col-1], 30)
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
		le.focus(focus)
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

// cellKey is a key while a text cell is edited: enter saves, esc cancels.
func (le *listEditor) cellKey(m *Model, msg tea.KeyPressMsg, k string) {
	c := le.d.cells[le.col-1]
	switch k {
	case "esc":
		le.phase = lpBrowse
		le.say("", false)
	case "enter":
		it, ok := le.cur()
		if !ok {
			le.phase = lpBrowse
			return
		}
		if le.applyCell(m, it, c, le.in.Value()) {
			le.phase = lpBrowse
		}
	default:
		var cmd tea.Cmd
		le.in, cmd = le.in.Update(msg)
		m.pendingCmd = cmd
	}
}

// applyCell checks the value with the field type, saves it and reports. It returns false when the
// value was refused and the input should stay open.
func (le *listEditor) applyCell(m *Model, it listItem, c listCellDesc, raw string) bool {
	res := validate.Field(c.spec, raw, validate.Options{})
	if errs := res.Errors(); len(errs) > 0 {
		le.say(errs[0].Fix+": "+errs[0].Expected, true)
		return false
	}
	if err := le.d.setCell(m, it, c.id, res.Value); err != nil {
		le.fail(err)
		return true
	}
	m.afterWrite()
	le.say(c.label+" of "+it.name+" is now "+res.Value+" (ctrl+z undoes)", false)
	le.reload(m, it.id())
	return true
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
		le.ask(targets, listDest{}, fmt.Sprintf("Remove %d %s (%s)?", len(targets), d.plural, namesOf(targets)))
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
		le.ask(le.pend.targets, p.dest, fmt.Sprintf("Remove %s (%d %s). %s?", namesOf(le.pend.targets), total, le.d.usedBy, p.label))
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
		if p.reset {
			le.applyReset(m, p)
			return
		}
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

// --- reset to the built-in defaults ---

// findItem finds an item by group and name, ignoring case.
func findItem(items []listItem, group, name string) (listItem, bool) {
	for _, it := range items {
		if it.group == group && strings.EqualFold(it.name, name) {
			return it, true
		}
	}
	return listItem{}, false
}

// resetPlan lists what a reset changes and the steps that do it. It adds the built-in items that are
// missing, sets the cells of built-in items back to their defaults and puts the built-in items first
// in their default order. Items the user added stay, after the built-in ones, and nothing is
// removed, so no record loses its value.
func (le *listEditor) resetPlan(m *Model) (plan []string, steps []func() error) {
	d := le.d
	defs := d.defaults(m)
	var added []listItem
	for _, df := range defs {
		if _, ok := findItem(le.items, df.group, df.name); ok {
			continue
		}
		df := df
		plan = append(plan, "add "+df.name)
		steps = append(steps, func() error {
			if err := d.add(m, df.group, df.name); err != nil {
				return err
			}
			items, err := d.load(m)
			if err != nil {
				return err
			}
			got, ok := findItem(items, df.group, df.name)
			if !ok {
				return nil
			}
			for ci, c := range d.cells {
				if c.readonly || ci >= len(df.cells) || ci >= len(got.cells) || got.cells[ci] == df.cells[ci] || d.setCell == nil {
					continue
				}
				if err := d.setCell(m, got, c.id, df.cells[ci]); err != nil {
					return err
				}
			}
			return nil
		})
		added = append(added, df)
	}
	for _, df := range defs {
		cur, ok := findItem(le.items, df.group, df.name)
		if !ok {
			continue
		}
		for ci, c := range d.cells {
			if c.readonly || ci >= len(df.cells) || ci >= len(cur.cells) || df.cells[ci] == cur.cells[ci] || d.setCell == nil {
				continue
			}
			cur, ci, c, df := cur, ci, c, df
			plan = append(plan, fmt.Sprintf("%s %s %s", cur.name, c.label, df.cells[ci]))
			steps = append(steps, func() error { return d.setCell(m, cur, c.id, df.cells[ci]) })
		}
	}
	if d.reorder != nil {
		groups := []string{""}
		if len(d.groups) > 0 {
			groups = nil
			for _, g := range d.groups {
				groups = append(groups, g.key)
			}
		}
		for _, g := range groups {
			var have []string
			for _, it := range le.items {
				if it.group == g {
					have = append(have, it.name)
				}
			}
			for _, df := range added {
				if df.group == g {
					have = append(have, df.name)
				}
			}
			var want []string
			used := map[string]bool{}
			for _, df := range defs {
				if df.group != g {
					continue
				}
				for _, h := range have {
					if strings.EqualFold(h, df.name) {
						want = append(want, h)
						used[strings.ToLower(h)] = true
					}
				}
			}
			for _, h := range have {
				if !used[strings.ToLower(h)] {
					want = append(want, h)
				}
			}
			if strings.Join(want, "\x00") == strings.Join(have, "\x00") {
				continue
			}
			g, want := g, want
			title := "order"
			for _, gd := range d.groups {
				if gd.key == g {
					title = "order " + strings.ToLower(gd.title)
				}
			}
			plan = append(plan, title)
			steps = append(steps, func() error { return d.reorder(m, g, want) })
		}
	}
	return plan, steps
}

// startReset is shift+r: it asks, with the list of changes, before anything is written.
func (le *listEditor) startReset(m *Model) {
	if le.d.defaults == nil || !le.writeOK(m) {
		return
	}
	plan, steps := le.resetPlan(m)
	if len(plan) == 0 {
		le.say("already the defaults", false)
		return
	}
	le.pend = &listPending{reset: true, plan: plan, steps: steps, prompt: fmt.Sprintf("Reset %s to the defaults?", strings.ToLower(le.d.title))}
	le.phase = lpConfirm
	le.say("", false)
}

// applyReset runs the steps of a reset as ONE undo step.
func (le *listEditor) applyReset(m *Model, p *listPending) {
	le.phase = lpBrowse
	m.beginGroup()
	var failed error
	for _, s := range p.steps {
		if err := s(); err != nil {
			failed = err
			break
		}
	}
	m.endGroup("reset " + strings.ToLower(le.d.title))
	m.afterWrite()
	le.reload(m, "")
	if failed != nil {
		le.fail(failed)
		return
	}
	le.say(fmt.Sprintf("reset %s, %d changes (ctrl+z undoes)", strings.ToLower(le.d.title), len(p.plan)), false)
}

// --- hints and drawing ---

// canEdit says whether e works on the item under the cursor: it renames the name or edits a text cell.
func (le *listEditor) canEdit(it listItem) bool {
	if it.locked != "" {
		return false
	}
	return le.col > 0 || !it.noRename
}

// movable says whether shift+up or shift+down can move the item.
func (le *listEditor) movable(it listItem) bool {
	if le.d.reorder == nil {
		return false
	}
	n := 0
	for _, x := range le.items {
		if x.group == it.group {
			n++
		}
	}
	return n > 1
}

// hints are the footer hints of the pane for its phase: only the keys that work on the row.
func (le *listEditor) hints(m *Model) []hintItem {
	switch le.phase {
	case lpAdd, lpRename, lpCell:
		return []hintItem{fk("enter", hwSave), fk("esc", hwCancel)}
	case lpPick:
		return []hintItem{fk("enter", hwPick), fk("esc", hwCancel), fk("up/down", hwChoose)}
	case lpConfirm:
		return []hintItem{fk("y", hwYes), fk("n", hwNo)}
	}
	var h []hintItem
	if len(le.items) > 1 {
		h = append(h, fk("up/down", hwSelect))
	}
	it, onItem := le.cur()
	if !m.readOnly {
		h = append(h, fk("a", hwAdd))
		if onItem && le.canEdit(it) {
			label := hwRename
			if le.col > 0 {
				label = le.d.cells[le.col-1].label
			}
			h = append(h, fk("e", label))
		}
		if onItem {
			if ci := le.d.cycleCell(); ci >= 0 {
				h = append(h, fk("tab", le.d.cells[ci].label))
			} else if le.d.editCols() && it.locked == "" {
				h = append(h, fk("tab", hwColumn))
			}
			if it.locked == "" {
				h = append(h, fk("d", hwDelete))
			}
			if le.movable(it) {
				h = append(h, fk("shift+up/down", hwMove))
			}
		}
		if le.d.defaults != nil {
			h = append(h, fk("shift+r", hwReset))
		}
	}
	return append(h, fk("esc", hwBack))
}

// promptLines is the confirm question, wrapped to the pane.
func (le *listEditor) promptLines(m *Model, w int) []string {
	var out []string
	for _, l := range strings.Split(ansi.Wrap(le.pend.prompt, w, ""), "\n") {
		out = append(out, m.st.Warn.Render(l))
	}
	return out
}

func width2(on bool) int {
	if on {
		return 2
	}
	return 0
}

// paneLines draws the pane: the title, the column heading, the rows. Every line fits w cells.
func (le *listEditor) paneLines(m *Model, w, h int) []string {
	if w < 1 || h < 1 {
		return nil
	}
	cut := func(s string) string { return ansi.Truncate(s, w, "…") }
	lines := []string{lipgloss.NewStyle().MaxWidth(w).Render(m.headingStyle().Render(cut(le.d.title)))}
	switch {
	case le.phase == lpPick && le.pend != nil:
		return append(lines, le.pickLines(m, w)...)
	case le.phase == lpConfirm && le.pend != nil && le.pend.reset:
		lines = append(lines, le.promptLines(m, w)...)
		plan := le.pend.plan
		if room := max(0, h-len(lines)); room < len(plan) {
			keep := max(0, room-1)
			plan = append(append([]string{}, plan[:keep]...), fmt.Sprintf("and %d more", len(le.pend.plan)-keep))
		}
		for _, p := range plan {
			lines = append(lines, cut("  "+p))
		}
		return lines
	}
	nameLabel := le.d.nameLabel
	if nameLabel == "" {
		nameLabel = "name"
	}
	bracket := le.d.editCols()
	nameW := ansi.StringWidth(nameLabel)
	for _, it := range le.items {
		nameW = max(nameW, ansi.StringWidth(it.name)+width2(bracket))
	}
	nameW += 2
	cellW := make([]int, len(le.d.cells))
	sum := 0
	for i, c := range le.d.cells {
		cellW[i] = ansi.StringWidth(c.label) + 2
		for _, it := range le.items {
			if i < len(it.cells) {
				cellW[i] = max(cellW[i], ansi.StringWidth(it.cells[i])+2+width2(bracket))
			}
		}
		sum += cellW[i]
	}
	usedW := 0
	if !le.d.noUsed {
		usedW = ansi.StringWidth("used by") + 1
	}
	if 2+nameW+sum+usedW > w {
		nameW = max(6, w-2-sum-usedW)
	}
	head := fit("", 2) + fit(nameLabel, nameW)
	for i, c := range le.d.cells {
		head += fit(c.label, cellW[i])
	}
	if usedW > 0 {
		head += "used by"
	}
	if le.phase == lpConfirm && le.pend != nil {
		lines = append(lines, le.promptLines(m, w)...)
	}
	lines = append(lines, m.st.Dim.Render(cut(head)))
	le.page = max(1, h-len(lines))
	le.fixOffset()
	if len(le.rows) == 0 && le.phase != lpAdd {
		return append(lines, m.st.Dim.Render("empty"))
	}
	end := min(len(le.rows), le.offset+le.page)
	focused := m.focus == focusDetail
	addRow := lipgloss.NewStyle().MaxWidth(w).Render("  " + le.in.View())
	for i := le.offset; i < end; i++ {
		r := le.rows[i]
		if r.divider {
			lines = append(lines, m.st.Dim.Render(ruleLine(r.title, "", w)))
		} else {
			lines = append(lines, le.rowLine(m, i, w, nameW, cellW, usedW, bracket, focused))
		}
		if le.phase == lpAdd && i == le.lastOfGroup(end) {
			lines = append(lines, addRow)
		}
	}
	if le.phase == lpAdd && len(le.rows) == 0 {
		lines = append(lines, addRow)
	}
	return lines
}

// lastOfGroup is the row index after which the inline add row is drawn: the last row of the
// group the add goes to, within the visible rows.
func (le *listEditor) lastOfGroup(end int) int {
	last := -1
	for i := le.offset; i < end; i++ {
		r := le.rows[i]
		if !r.divider && le.items[r.idx].group == le.addGroup {
			last = i
		}
	}
	if last < 0 {
		return end - 1
	}
	return last
}

func (le *listEditor) rowLine(m *Model, i, w, nameW int, cellW []int, usedW int, bracket, focused bool) string {
	it := le.items[le.rows[i].idx]
	onRow := i == le.cursor
	mark := "  "
	if le.marked[it.id()] {
		mark = "* "
	}
	nameCell := fit(it.name, nameW)
	switch {
	case onRow && le.phase == lpRename:
		nameCell = fit(le.in.View(), nameW)
	case onRow && focused && le.col == 0 && le.phase == lpBrowse && bracket:
		nameCell = fit("["+it.name+"]", nameW)
	}
	line := mark + nameCell
	for ci, c := range le.d.cells {
		val := ""
		if ci < len(it.cells) {
			val = it.cells[ci]
		}
		text := val
		switch {
		case onRow && le.col == ci+1 && le.phase == lpCell:
			text = le.in.View()
		case onRow && focused && le.col == ci+1 && le.phase == lpBrowse && !c.cycle && !c.readonly:
			text = "[" + val + "]"
		}
		line += fit(text, cellW[ci])
	}
	use := ""
	if usedW > 0 {
		use = fmt.Sprint(it.inUse)
	}
	if onRow && focused {
		return m.st.Sel.Render(fit(ansi.Strip(line+use), w))
	}
	if use != "" {
		line += m.st.Dim.Render(use)
	}
	return ansi.Truncate(line, w, "…")
}

func (le *listEditor) pickLines(m *Model, w int) []string {
	total := 0
	for _, t := range le.pend.targets {
		total += t.inUse
	}
	lines := []string{ansi.Truncate(m.st.Head.Render(fmt.Sprintf("%d %s use %s. Where do they go?", total, le.d.usedBy, namesOf(le.pend.targets))), w, "…")}
	for i, p := range le.pick {
		line := fit("  "+p.label, w)
		if i == le.choice {
			lines = append(lines, m.st.Sel.Render(line))
		} else {
			lines = append(lines, line)
		}
	}
	return lines
}
