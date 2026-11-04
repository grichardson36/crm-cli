package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	sg "github.com/jdanielnd/crm-cli/internal/suggest"
)

// overlayItem is one choice in the palette or a picker.
type overlayItem struct {
	id       string
	label    string
	hint     string
	desc     string
	value    string
	disabled string // reason shown greyed out, empty when usable
	// kind says what a bar item is (see bar.go): "" is an action. ref is a record id and origin
	// the kind a recent item had.
	kind   string
	ref    int64
	origin string
	// link is the id of a stored link, for the Unlink picker.
	link int64
	// exact marks a tag row whose name is the whole query: it ranks above the other objects.
	exact bool
	// divider marks a heading row of a grouped list (label at the left, desc at the right). The
	// cursor never lands on it and enter does nothing on it (patterns/grouped-lists-with-dividers).
	divider bool
	// selector marks a row whose value left and right change. current is the value shown at the
	// right of the row. isCur marks the row of what is open now (drawn in the accent style).
	selector bool
	current  string
	isCur    bool
	// dim draws the row gray (a hidden view). The row stays usable.
	dim bool
}

// overlay is a filterable list shown instead of the main screen: the command palette or a picker.
type overlay struct {
	title   string
	items   []overlayItem
	filter  textinput.Model
	shown   []int // indexes into items that match the filter
	cursor  int
	offset  int
	palette bool
	digits  bool // a number key picks the item with that hint (view switcher), while the filter is empty
	// nameOnly makes the filter match the label only (the Views list).
	nameOnly bool
	letters bool // a letter key picks the item whose hint is that letter (the add picker)
	// focusResults is the bar mode where plain keys are verbs of the highlighted row (barverbs.go).
	focusResults bool
	// onKey sees every key first and returns true when it used it (the Modules screen). noFilter
	// turns the filter line off.
	onKey    func(m *Model, key string) bool
	noFilter bool
	// noDesc hides the desc column of a popup (desc stays searchable). hintsFor builds the hint row
	// from the highlighted row, and replaces hints.
	noDesc   bool
	minW     int // text width the popup keeps at least, when its hints need more room
	hintsFor func(m *Model) []hintItem
	// onMsg sees the whole key event before onKey (a screen with its own text input) and draw
	// replaces the list drawing. The list editor uses both (listeditor.go), le is its state.
	onMsg func(m *Model, msg tea.KeyPressMsg) bool
	draw  func(m *Model) string
	le    *listEditor
	// hints replaces the default footer hints of the list (hints.go), note replaces the filter line of a noFilter list.
	note  string
	hints []hintItem
	onPick     func(m *Model, it overlayItem)
	// onCursor runs when the cursor lands on a different item (live preview). onCancel runs when
	// the overlay is closed without a pick. Both are optional.
	onCursor  func(m *Model, it overlayItem)
	onCancel  func(m *Model)
	previewed string // id of the item onCursor saw last
	listTop   int    // first screen line of the item list
	extraTop  int    // lines between the filter line and the list that a custom draw adds (mouse rows)
	pageSize  int
	// cmd is the command-bar state while the palette text is a command line (verb and a space).
	cmd *cmdState
	// cycle remembers the last Tab completion, so Tab again moves to the next candidate.
	cycle *cycleState
	// dismissed are the chips turned back into plain text with alt+d while typing a command.
	dismissed []string
	// The bar (palette) lists base plus what search finds and recent returns. See bar.go.
	base   []overlayItem
	search func(scope byte, q string) []overlayItem
	recent func() []overlayItem
	// create adds a last row "Create <typed text>" to a picker (pick-or-create). It returns false
	// when the text cannot make an object. baseLen is the number of real items.
	create  func(q string) (overlayItem, bool)
	baseLen int
}

const overlayTop = 2 // title line, filter line

func (o *overlay) refilter() {
	if o.palette {
		o.refilterBar()
		return
	}
	type scored struct{ idx, score, length int }
	var list []scored
	q := o.filter.Value()
	if o.create != nil {
		if o.baseLen == 0 {
			o.baseLen = len(o.items)
		}
		o.items = o.items[:o.baseLen]
	}
	for i, it := range o.items {
		hay := it.label + " " + it.hint + " " + it.id + " " + it.desc
		if o.nameOnly {
			hay = it.label
		}
		s, ok := sg.Fuzzy(q, hay)
		if !ok {
			continue
		}
		if it.disabled != "" {
			s += 1000
		}
		list = append(list, scored{i, s, len(it.label)})
	}
	sort.SliceStable(list, func(a, b int) bool {
		if list[a].score != list[b].score {
			return list[a].score < list[b].score
		}
		return q != "" && list[a].length < list[b].length // an exact short title beats a longer one that starts the same
	})
	o.shown = o.shown[:0]
	for _, s := range list {
		o.shown = append(o.shown, s.idx)
	}
	if o.create != nil {
		if it, ok := o.create(q); ok {
			o.items = append(o.items, it)
			o.shown = append(o.shown, len(o.items)-1)
		}
	}
	o.cursor, o.offset = 0, 0
}

// skipDividers moves the cursor off a divider row, in the direction of the last move first.
func (o *overlay) skipDividers(d int) {
	step := 1
	if d < 0 {
		step = -1
	}
	for range len(o.shown) {
		if o.cursor < 0 || o.cursor >= len(o.shown) || !o.items[o.shown[o.cursor]].divider {
			return
		}
		next := o.cursor + step
		if next < 0 || next >= len(o.shown) {
			step = -step
			next = o.cursor + step
		}
		o.cursor = next
	}
}

func (o *overlay) move(d, page int) {
	o.cursor = clampInt(o.cursor+d, 0, len(o.shown)-1)
	o.skipDividers(d)
	if o.cursor < o.offset {
		o.offset = o.cursor
	}
	if page > 0 && o.cursor >= o.offset+page {
		o.offset = o.cursor - page + 1
	}
}

func (m *Model) newOverlay(title string, items []overlayItem, palette bool) *overlay {
	in := newInput("> ")
	in.Placeholder = title
	o := &overlay{title: title, items: items, filter: in, palette: palette}
	if palette {
		o.base = items
	}
	o.refilter()
	return o
}

// openPalette lists every registry action. Actions that are blocked or out of context are greyed out.
func (m *Model) openPalette() tea.Cmd {
	m.cmdPerson = m.palettePerson()
	items := make([]overlayItem, 0, len(m.actions))
	for i := range m.actions {
		a := &m.actions[i]
		if !m.moduleOn(a) || a.barOnly || a.mode == keyText {
			continue // an off module, the verbs of a tag row and the keys of a text prompt are not in the palette
		}
		it := overlayItem{id: a.id, label: a.title, hint: a.hint, desc: a.desc}
		switch {
		case a.write && m.readOnly:
			it.disabled = "read-only"
		case !a.enabled(m):
			it.disabled = "not here"
		}
		items = append(items, it)
	}
	// One entry per registered adapter. A dry run writes nothing, so read-only mode allows it.
	for _, name := range m.registry.Names() {
		run := overlayItem{id: "sync-run:" + name, label: "Sync now: " + name, desc: "run the " + name + " source in the background, shows the result line"}
		dry := overlayItem{id: "sync-dry:" + name, label: "Dry-run sync: " + name, desc: "preview what " + name + " would change, writes nothing"}
		if m.readOnly {
			run.disabled = "read-only"
		}
		if m.cmp != nil {
			run.disabled, dry.disabled = "not here", "not here"
		}
		items = append(items, run, dry)
	}
	for i := range items {
		if v, ok := verbAliases[items[i].id]; ok {
			items[i].label += " (" + v + ")"
		}
	}
	o := m.newOverlay("Select a command…", items, true)
	o.search = m.barSearch
	o.recent = m.recentItems
	o.refilter()
	o.onPick = func(m *Model, it overlayItem) {
		if ok, cmd := m.pickBar(it); ok {
			m.pendingCmd = cmd
			return
		}
		m.pushRecent(it)
		if kind, name, ok := strings.Cut(it.id, ":"); ok && (kind == "sync-run" || kind == "sync-dry") {
			if it.disabled != "" {
				m.setStatus(strings.ToLower(it.label)+": "+it.disabled, true)
				return
			}
			m.pendingCmd = m.startSync(name, kind == "sync-dry")
			return
		}
		a := m.actionByID(it.id)
		if a == nil {
			return
		}
		if it.disabled != "" {
			m.setStatus(strings.ToLower(a.title)+": "+it.disabled, true)
			return
		}
		m.pendingCmd = m.runAction(a)
	}
	return m.showOverlay(o)
}

// openPicker shows a small list of choices. onPick runs after the overlay closes.
func (m *Model) openPicker(title string, items []overlayItem, onPick func(m *Model, it overlayItem)) {
	_ = m.showOverlay(m.newOverlay(title, items, false))
	m.ov.onPick = onPick
}

func (m *Model) showOverlay(o *overlay) tea.Cmd {
	m.ov = o
	m.mode = modeOverlay
	return o.filter.Focus()
}

func (m *Model) closeOverlay() {
	m.mode = modeNormal
	m.ov = nil
}

// overlayPage is the number of list rows a popup shows. A custom drawn screen (the list editor) has
// the whole terminal.
func (m *Model) overlayPage() int {
	if m.ov != nil && m.ov.draw != nil {
		return max(1, m.height-overlayTop-2)
	}
	return m.popupPage()
}

// pickOverlay runs the item at shown index i.
func (m *Model) pickOverlay(i int) tea.Cmd {
	o := m.ov
	if o == nil || i < 0 || i >= len(o.shown) {
		return nil
	}
	it := o.items[o.shown[i]]
	if it.divider {
		return nil
	}
	pick := o.onPick
	m.lastFilter = o.filter.Value()
	m.closeOverlay()
	m.pendingCmd = nil
	if pick != nil {
		pick(m, it)
	}
	cmd := m.pendingCmd
	m.pendingCmd = nil
	return cmd
}

func (m *Model) handleOverlayKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	mm, cmd := m.overlayKey(msg)
	m.previewCursor()
	return mm, cmd
}

// previewCursor tells the open overlay which item the cursor is on, once per change.
func (m *Model) previewCursor() {
	o := m.ov
	if o == nil || o.onCursor == nil || o.cursor < 0 || o.cursor >= len(o.shown) {
		return
	}
	it := o.items[o.shown[o.cursor]]
	if it.id == o.previewed {
		return
	}
	o.previewed = it.id
	o.onCursor(m, it)
}

func (m *Model) overlayKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	o := m.ov
	page := m.overlayPage()
	if o.onMsg != nil && o.onMsg(m, msg) {
		cmd := m.pendingCmd
		m.pendingCmd = nil
		return m, cmd
	}
	if o.onKey != nil && o.onKey(m, keyName(msg)) {
		return m, nil
	}
	if o.palette && o.cmd == nil && m.globalAddKeyInBar(keyName(msg)) {
		if m.readOnly {
			return m, m.showToast("read-only: new object is disabled", true)
		}
		m.closeOverlay()
		return m, m.openGlobalAdd(o)
	}
	if o.palette && o.cmd == nil {
		if mdl, c, ok := m.barFocusKey(msg); ok {
			return mdl, c
		}
	}
	if o.palette {
		if mdl, c, ok := m.cmdKey(msg); ok {
			return mdl, c
		}
	}
	switch keyName(msg) {
	case "esc", "ctrl+c":
		if o.onCancel != nil {
			o.onCancel(m)
		}
		if m.ov == o { // onCancel may have closed the overlay itself and opened a prompt (pipelink.go)
			m.closeOverlay()
		}
		return m, nil
	case "enter":
		return m, m.pickOverlay(o.cursor)
	case "up", "shift+tab":
		o.move(-1, page)
		return m, nil
	case "down", "tab":
		o.move(1, page)
		return m, nil
	case "pgup":
		o.move(-page, page)
		return m, nil
	case "pgdown":
		o.move(page, page)
		return m, nil
	}
	if k := keyName(msg); o.digits && o.filter.Value() == "" && len(k) == 1 && k[0] >= '0' && k[0] <= '9' {
		for i, idx := range o.shown {
			if o.items[idx].hint == k {
				return m, m.pickOverlay(i)
			}
		}
		return m, nil
	}
	if k := keyName(msg); o.letters && len(k) == 1 {
		for i, idx := range o.shown {
			if strings.EqualFold(o.items[idx].hint, k) {
				return m, m.pickOverlay(i)
			}
		}
		return m, nil
	}
	if o.noFilter {
		return m, nil
	}
	var cmd tea.Cmd
	before := o.filter.Value()
	o.filter, cmd = o.filter.Update(msg)
	if o.filter.Value() != before {
		o.refilter()
		o.move(0, page)
	}
	m.syncCmd()
	return m, cmd
}

func (m *Model) handleOverlayMouse(msg mouseEvent) (tea.Model, tea.Cmd) {
	mm, cmd := m.overlayMouse(msg)
	m.previewCursor()
	return mm, cmd
}

func (m *Model) overlayMouse(msg mouseEvent) (tea.Model, tea.Cmd) {
	o := m.ov
	if o.cmd != nil {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		o.move(-3, m.overlayPage())
	case tea.MouseWheelDown:
		o.move(3, m.overlayPage())
	case tea.MouseLeft:
		if o.draw != nil {
			top := overlayTop + o.extraTop
			i := o.offset + msg.Y - top
			if msg.Y >= top && msg.Y < top+m.overlayPage() {
				return m, m.pickOverlay(i)
			}
			break
		}
		b := m.overlayBox(o)
		if msg.X >= b.x && msg.X < b.x+b.w && msg.Y >= b.listY && msg.Y < b.listY+b.rows {
			return m, m.pickOverlay(o.offset + msg.Y - b.listY)
		}
	}
	return m, nil
}

func (m *Model) overlayView() string {
	o := m.ov
	if o.draw != nil {
		return o.draw(m)
	}
	return m.popupOver(m.underView(), m.overlayPopup(o))
}

// hasFilter says whether the popup shows a filter line: the palette and pick-or-create lists always
// do, a picker when it is long or once something is typed. Typing filters in every popup but a noFilter one.
func (o *overlay) hasFilter() bool {
	if o.noFilter {
		return false
	}
	if o.palette || o.create != nil {
		return true
	}
	// A short picker shows its title until the first letter is typed. Typing filters either way.
	return len(o.items) > popupFilterMin || o.filter.Value() != ""
}

// popupWidth is the row width the popup wants: the widest row, within the popup limits.
func (o *overlay) popupWidth() int {
	if o.palette {
		return popupPaletteW
	}
	w := ansi.StringWidth(o.title) + 2
	hintW := o.hintWidth()
	for _, it := range o.items {
		line := hintW + ansi.StringWidth(it.label) + 2
		switch {
		case it.divider:
			line = ansi.StringWidth(it.label) + ansi.StringWidth(it.desc) + 6
		case it.selector:
			line += ansi.StringWidth(it.current) + 2
		case it.disabled != "":
			line += ansi.StringWidth(it.disabled) + 2
		case !o.noDesc && it.desc != "":
			line += min(30, ansi.StringWidth(it.desc)) + 2
		}
		w = max(w, line)
	}
	w = max(w, o.minW)
	return clampInt(w, popupMinW, max(popupMaxW, o.minW))
}

// hintWidth is the width of the key column at the left of a picker row, 0 when no row has a key.
func (o *overlay) hintWidth() int {
	w := 0
	for _, it := range o.items {
		w = max(w, ansi.StringWidth(showKey(it.hint)))
	}
	if w == 0 {
		return 0
	}
	return min(w, 8) + 2
}

// popupRowCount is the number of list lines the box has: the whole list up to the page, and the
// palette always a full page, so the box does not change size while typing.
func (o *overlay) popupRowCount(page int) int {
	if o.palette {
		return page
	}
	n := len(o.items)
	if o.create != nil && o.baseLen > 0 {
		n = o.baseLen + 1
	}
	return min(max(1, n), page)
}

// overlayBox is where the popup of o sits.
func (m *Model) overlayBox(o *overlay) popupBox {
	return m.popupBoxWith(m.popupTextW(o.popupWidth()), o.popupRowCount(m.popupPage()), o.palette)
}

// paletteHelpers are the scope prefixes of the palette, as the dim helper row shows them.
const paletteHelpers = "> actions   @ people   # tags   = settings   ? help"

// paletteHelper is the helper row of the palette: the prefixes, while the box is empty or holds
// only a prefix. It is not a sentence and it is not shown once a query is typed.
func (o *overlay) paletteHelper() string {
	v := strings.TrimSpace(o.filter.Value())
	if v == "" || (len(v) == 1 && strings.ContainsAny(v, ">@#?=")) {
		return paletteHelpers
	}
	return ""
}

// overlayPopup is what the popup of o draws now.
func (m *Model) overlayPopup(o *overlay) popupSpec {
	box := m.overlayBox(o)
	spec := popupSpec{title: o.title, box: box}
	if o.cmd != nil {
		return m.cmdPopup(o, spec)
	}
	if o.hasFilter() {
		o.filter.SetWidth(max(1, box.textW-3))
		spec.input = o.filter.View()
	}
	spec.rows = m.popupListRows(o, box.textW, box.rows)
	if box.helper {
		spec.helper = m.st.Dim.Render(o.paletteHelper())
	}
	if o.focusResults {
		if it, ok := o.highlighted(); ok {
			spec.hints = m.st.Head.Render(ansi.Truncate(m.barVerbLine(it), box.textW, "…"))
		}
	}
	if spec.hints == "" {
		spec.hints = m.renderPopupHints(m.overlayHints(), box.textW)
	}
	spec.empty = "No matches"
	return spec
}

// popupListRows draws the rows from o.offset on, n at most, each textW cells wide. The Views list
// and the Settings pane that embeds it draw their rows with this.
func (m *Model) popupListRows(o *overlay, textW, n int) []string {
	end := min(len(o.shown), o.offset+n)
	if o.palette {
		return m.barListView(o, end, textW)
	}
	hintW := o.hintWidth()
	labelMax := 0
	for _, it := range o.items {
		if !it.divider {
			labelMax = max(labelMax, ansi.StringWidth(it.label))
		}
	}
	var out []string
	for i := o.offset; i < end; i++ {
		it := o.items[o.shown[i]]
		if it.divider {
			line := ruleLine(it.label, it.desc, textW)
			if it.label == "" && it.desc == "" {
				line = strings.Repeat("─", textW) // the line between shown and hidden views
			}
			out = append(out, m.st.Dim.Render(line))
			continue
		}
		right := ""
		switch {
		case it.disabled != "":
			right = it.disabled
		case it.selector:
			right = it.current
		case !o.noDesc:
			right = it.desc
		}
		labelW := textW - hintW
		if right != "" {
			rightW := min(ansi.StringWidth(right), max(8, textW/2))
			labelW = max(4, min(labelMax+2, textW-hintW-rightW-1))
		}
		rightW := max(0, textW-hintW-labelW)
		keyCol, labelCol := fit(showKey(it.hint), hintW), fit(it.label, labelW)
		rightCol := fit(right, rightW)
		if it.selector {
			rightCol = padLeft(ansi.Truncate(right, rightW, "…"), rightW)
		}
		switch {
		case i == o.cursor:
			out = append(out, m.st.Sel.Render(keyCol+labelCol+rightCol))
		case it.disabled != "" || it.dim:
			out = append(out, m.st.Dim.Render(keyCol+labelCol+rightCol))
		default:
			label := labelCol
			if it.isCur {
				label = m.st.Head.Render(labelCol)
			}
			rst := m.st.Dim
			if it.selector {
				rst = m.st.Name
			}
			out = append(out, m.st.Name.Render(keyCol)+label+rst.Render(rightCol))
		}
	}
	return out
}

// --- help ---

func (m *Model) openHelp() {
	m.help = true
	m.helpVP.SetWidth(m.width)
	m.helpVP.SetHeight(max(1, m.pageRows(m.helpHints())))
	m.helpVP.SetContent(m.helpText())
	m.helpVP.GotoTop()
}

func (m *Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch keyName(msg) {
	case "down":
		m.helpVP.ScrollDown(1)
	case "up":
		m.helpVP.ScrollUp(1)
	case "pgdown":
		m.helpVP.PageDown()
	case "ctrl+d", " ":
		m.helpVP.HalfPageDown()
	case "pgup":
		m.helpVP.PageUp()
	case "ctrl+u":
		m.helpVP.HalfPageUp()
	case "home", "ctrl+shift+up":
		m.helpVP.GotoTop()
	case "end", "ctrl+shift+down":
		m.helpVP.GotoBottom()
	default:
		m.help = false
	}
	return m, nil
}

// helpText is generated from the action registry. It lists the keys that work in the current view
// and mode, grouped by layer (framework, then app). Actions without a key are in the palette.
func (m *Model) helpText() string {
	order := []string{"Verbs", "Home", "Contact", "Tasks", "Link", "Edit pane", "Bulk", "Sync", "Merge", "Compare", "Archive", "Organizations", "Tags", "Deals", "Move", "Views", "App"}
	chain := m.scopeChain()
	var b strings.Builder
	b.WriteString(m.st.Head.Render(fmt.Sprintf("Keys in %s (normal mode)", viewNames[m.view])) + "\n")
	b.WriteString(m.st.Dim.Render("Keys that act on a highlighted row need one. Everything else is in the palette.") + "\n\n")
	for _, layer := range []string{layerFramework, layerApp} {
		title := "Framework: the same in every view"
		if layer == layerApp {
			title = "App: specific to this CRM"
		}
		var body strings.Builder
		for _, g := range order {
			var lines []string
			for i := range m.actions {
				a := &m.actions[i]
				if a.group != g || a.barOnly || a.layerOf() != layer || a.mode != keyNormal || !inScopes(a, chain) {
					continue
				}
				var live []string
				for _, k := range a.keys {
					if (a.keyIf == nil || a.keyIf(m)) && (a.keyFor == nil || a.keyFor(m, k)) && (m.cmp == nil || a.inCompare) {
						live = append(live, showKey(k))
					}
				}
				if len(live) == 0 {
					continue
				}
				line := m.st.Name.Render(fit(strings.Join(live, ", "), 22)) + a.desc
				if a.write {
					line += m.st.Dim.Render("  [edits data]")
				}
				lines = append(lines, line)
			}
			if len(lines) == 0 {
				continue
			}
			body.WriteString(m.st.Dim.Render(g) + "\n" + strings.Join(lines, "\n") + "\n")
		}
		if body.Len() == 0 {
			continue
		}
		b.WriteString(m.st.Head.Render(title) + "\n" + body.String() + "\n")
	}
	var text []string
	for i := range m.actions {
		a := &m.actions[i]
		if a.mode == keyText && len(a.keys) > 0 {
			text = append(text, m.st.Name.Render(fit(joinKeys(a.keys), 22))+a.desc)
		}
	}
	if len(text) > 0 {
		b.WriteString(m.st.Head.Render("Typing in a prompt (text mode)") + "\n" + strings.Join(text, "\n") + "\n\n")
	}
	b.WriteString(m.st.Head.Render("Mouse") + "\n")
	b.WriteString(m.st.Name.Render(fit("click", 22)) + "select a row, click again on a detail row to edit it\n")
	b.WriteString(m.st.Name.Render(fit("click footer", 22)) + "tap a footer key to run it, tap the status line for the palette\n")
	b.WriteString(m.st.Name.Render(fit("wheel, tabs", 22)) + "scroll, switch view (the More tab opens the view list)\n")
	return strings.TrimRight(b.String(), "\n")
}

// barListView draws the bar list as lines: the group name at the start of each group, the label,
// the description of a record and the key on the right.
func (m *Model) barListView(o *overlay, end, width int) []string {
	const groupW = 8
	keyW := 0
	for _, it := range o.items {
		keyW = max(keyW, ansi.StringWidth(showKey(it.hint)))
	}
	keyW = min(keyW, 15)
	if keyW > 0 {
		keyW++
	}
	var out []string
	prev := ""
	if o.offset > 0 {
		prev = barGroup[o.items[o.shown[o.offset-1]].kind]
	}
	for i := o.offset; i < end; i++ {
		it := o.items[o.shown[i]]
		group := barGroup[it.kind]
		tag := ""
		if group != prev {
			tag = group
		}
		prev = group
		desc := ""
		switch it.kind {
		case kindPerson, kindOrg, kindDeal, kindTask, kindNote, kindTag, kindRecent, kindAll:
			desc = it.desc
		}
		if it.disabled != "" {
			desc = it.disabled
		}
		titleW := min(34, max(12, width/2))
		room := max(0, width-groupW-titleW-keyW)
		plainLine := fit(tag, groupW) + fit(it.label, titleW) + fit(desc, room) + padLeft(showKey(it.hint), keyW)
		switch {
		case i == o.cursor:
			out = append(out, m.st.Sel.Render(fit(plainLine, width)))
		case it.disabled != "":
			out = append(out, m.st.Dim.Render(fit(plainLine, width)))
		default:
			out = append(out, m.st.Head.Render(fit(tag, groupW))+m.st.Name.Render(fit(it.label, titleW))+m.st.Dim.Render(fit(desc, room))+m.st.Dim.Render(padLeft(showKey(it.hint), keyW)))
		}
	}
	return out
}
