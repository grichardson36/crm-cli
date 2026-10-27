package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/mattn/go-runewidth"
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
	digits  bool // a number key picks the item with that hint (view switcher)
	letters bool // a letter key picks the item whose hint is that letter (the add picker)
	// focusResults is the bar mode where plain keys are verbs of the highlighted row (barverbs.go).
	focusResults bool
	// onKey sees every key first and returns true when it used it (the Modules screen). noFilter
	// turns the filter line off.
	onKey    func(m *Model, key string) bool
	noFilter bool
	onPick  func(m *Model, it overlayItem)
	// onCursor runs when the cursor lands on a different item (live preview). onCancel runs when
	// the overlay is closed without a pick. Both are optional.
	onCursor  func(m *Model, it overlayItem)
	onCancel  func(m *Model)
	previewed string // id of the item onCursor saw last
	listTop   int    // first screen line of the item list
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

// fuzzyScore returns a rank (lower is better) when every word of query matches text.
func fuzzyScore(query, text string) (int, bool) {
	text = strings.ToLower(text)
	score := 0
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if i := strings.Index(text, w); i >= 0 {
			score += min(i, 20) / 4
			continue
		}
		// Subsequence match.
		pos := 0
		for _, r := range w {
			j := strings.IndexRune(text[pos:], r)
			if j < 0 {
				return 0, false
			}
			pos += j + len(string(r))
		}
		score += 30
	}
	return score, true
}

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
		s, ok := fuzzyScore(q, it.label+" "+it.hint+" "+it.id+" "+it.desc)
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

func (o *overlay) move(d, page int) {
	o.cursor = clampInt(o.cursor+d, 0, len(o.shown)-1)
	if o.cursor < o.offset {
		o.offset = o.cursor
	}
	if page > 0 && o.cursor >= o.offset+page {
		o.offset = o.cursor - page + 1
	}
}

func (m *Model) newOverlay(title string, items []overlayItem, palette bool) *overlay {
	in := newInput("> ")
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
	o := m.newOverlay("Command bar", items, true)
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

// openViewSwitcher lists every view with its number key.
func (m *Model) openViewSwitcher() tea.Cmd {
	var items []overlayItem
	for i, v := range m.visibleViews() {
		it := overlayItem{id: viewActionID(v), label: viewNames[v], value: strconv.Itoa(int(v))}
		it.hint = modules.Key(i + 1)
		if v == m.view {
			it.desc = "current view"
		} else {
			it.desc = viewBlurb(v)
		}
		items = append(items, it)
	}
	m.openPicker("Go to view", items, func(m *Model, it overlayItem) {
		n, _ := strconv.Atoi(it.value)
		m.switchView(viewKind(n))
	})
	m.ov.digits = true
	return nil
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

func (m *Model) overlayPage() int { return max(1, m.height-overlayTop-2) }

// pickOverlay runs the item at shown index i.
func (m *Model) pickOverlay(i int) tea.Cmd {
	o := m.ov
	if o == nil || i < 0 || i >= len(o.shown) {
		return nil
	}
	it := o.items[o.shown[i]]
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

func (m *Model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m *Model) overlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.ov
	page := m.overlayPage()
	if o.onKey != nil && o.onKey(m, msg.String()) {
		return m, nil
	}
	if o.palette && o.cmd == nil && m.globalAddKeyInBar(msg.String()) {
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
	switch msg.String() {
	case "esc", "ctrl+c":
		if o.onCancel != nil {
			o.onCancel(m)
		}
		m.closeOverlay()
		return m, nil
	case "enter":
		return m, m.pickOverlay(o.cursor)
	case "up", "ctrl+p", "shift+tab":
		o.move(-1, page)
		return m, nil
	case "down", "ctrl+n", "tab":
		o.move(1, page)
		return m, nil
	case "pgup":
		o.move(-page, page)
		return m, nil
	case "pgdown":
		o.move(page, page)
		return m, nil
	}
	if k := msg.String(); o.digits && len(k) == 1 && k[0] >= '0' && k[0] <= '9' {
		for i, idx := range o.shown {
			if o.items[idx].hint == k {
				return m, m.pickOverlay(i)
			}
		}
		return m, nil
	}
	if k := msg.String(); o.letters && len(k) == 1 {
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

func (m *Model) handleOverlayMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	mm, cmd := m.overlayMouse(msg)
	m.previewCursor()
	return mm, cmd
}

func (m *Model) overlayMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	o := m.ov
	if o.cmd != nil {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		o.move(-3, m.overlayPage())
	case tea.MouseButtonWheelDown:
		o.move(3, m.overlayPage())
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		i := o.offset + msg.Y - overlayTop
		if msg.Y >= overlayTop && msg.Y < overlayTop+m.overlayPage() {
			return m, m.pickOverlay(i)
		}
	}
	return m, nil
}

func (m *Model) overlayView() string {
	o := m.ov
	if o.cmd != nil {
		return m.cmdView()
	}
	var b strings.Builder
	help := "   enter run  esc close  up/down choose"
	if o.palette {
		help = "   enter run  esc close  tab verbs   > actions  @ people  # tags  = settings  ? help"
	}
	b.WriteString(lipgloss.NewStyle().MaxWidth(m.width).Render(m.st.Head.Render(o.title)+m.st.Dim.Render(help)) + "\n")
	b.WriteString(o.filter.View() + "\n")
	page := m.overlayPage()
	if len(o.shown) == 0 {
		b.WriteString(m.st.Dim.Render("No matches."))
		return b.String()
	}
	end := min(len(o.shown), o.offset+page)
	hintW, titleW := 8, 24
	if o.palette {
		b.WriteString(m.barListView(o, end))
		if o.focusResults {
			if it, ok := o.highlighted(); ok {
				b.WriteString("\n" + m.st.Head.Render(runewidth.Truncate(m.barVerbLine(it), m.width, "…")))
			}
		}
		return b.String()
	}
	for i := o.offset; i < end; i++ {
		it := o.items[o.shown[i]]
		desc := it.desc
		if it.disabled != "" {
			desc = "(" + it.disabled + ") " + desc
		}
		left := fit(it.hint, hintW) + fit(it.label, titleW)
		room := m.width - runewidth.StringWidth(left)
		line := left + fit(desc, max(0, room))
		line = fit(line, m.width)
		switch {
		case i == o.cursor:
			b.WriteString(m.st.Sel.Render(line))
		case it.disabled != "":
			b.WriteString(m.st.Dim.Render(line))
		default:
			b.WriteString(m.st.Name.Render(fit(it.hint, hintW)) + fit(it.label, titleW) + m.st.Dim.Render(fit(desc, max(0, room))))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// --- help ---

func (m *Model) openHelp() {
	m.help = true
	m.helpVP.Width = m.width
	m.helpVP.Height = max(1, m.height-2)
	m.helpVP.SetContent(m.helpText())
	m.helpVP.GotoTop()
}

func (m *Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "down", "ctrl+n":
		m.helpVP.LineDown(1)
	case "up", "ctrl+p":
		m.helpVP.LineUp(1)
	case "pgdown":
		m.helpVP.ViewDown()
	case "ctrl+d", " ":
		m.helpVP.HalfViewDown()
	case "pgup":
		m.helpVP.ViewUp()
	case "ctrl+u":
		m.helpVP.HalfViewUp()
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
	b.WriteString(m.st.Dim.Render("Keys that act on a highlighted row need one. Everything else is in the palette, ctrl+k.") + "\n\n")
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
						live = append(live, keys.Display(k))
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

// barListView draws the bar list: the group name at the start of each group, the label, the
// description, and the key on the right.
func (m *Model) barListView(o *overlay, end int) string {
	const groupW, keyW = 8, 15
	var b strings.Builder
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
		desc := it.desc
		if it.disabled != "" {
			desc = "(" + it.disabled + ") " + desc
		}
		titleW := min(34, max(12, m.width/3))
		room := max(0, m.width-groupW-titleW-keyW)
		plainLine := fit(tag, groupW) + fit(it.label, titleW) + fit(desc, room) + runewidth.FillLeft(it.hint, keyW)
		switch {
		case i == o.cursor:
			b.WriteString(m.st.Sel.Render(fit(plainLine, m.width)))
		case it.disabled != "":
			b.WriteString(m.st.Dim.Render(fit(plainLine, m.width)))
		default:
			b.WriteString(m.st.Head.Render(fit(tag, groupW)) + m.st.Name.Render(fit(it.label, titleW)) + m.st.Dim.Render(fit(desc, room)) + m.st.Dim.Render(runewidth.FillLeft(it.hint, keyW)))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
