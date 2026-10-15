package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
}

// overlay is a filterable list shown instead of the main screen: the command palette or a picker.
type overlay struct {
	title    string
	items    []overlayItem
	filter   textinput.Model
	shown    []int // indexes into items that match the filter
	cursor   int
	offset   int
	palette  bool
	digits   bool // a number key picks the item with that hint (view switcher)
	onPick   func(m *Model, it overlayItem)
	listTop  int // first screen line of the item list
	pageSize int
	// cmd is the command-bar state while the palette text is a command line (verb and a space).
	cmd *cmdState
	// cycle remembers the last Tab completion, so Tab again moves to the next candidate.
	cycle *cycleState
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
	type scored struct{ idx, score int }
	var list []scored
	q := o.filter.Value()
	for i, it := range o.items {
		s, ok := fuzzyScore(q, it.label+" "+it.hint+" "+it.id+" "+it.desc)
		if !ok {
			continue
		}
		if it.disabled != "" {
			s += 1000
		}
		list = append(list, scored{i, s})
	}
	sort.SliceStable(list, func(a, b int) bool { return list[a].score < list[b].score })
	o.shown = o.shown[:0]
	for _, s := range list {
		o.shown = append(o.shown, s.idx)
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
	o.refilter()
	return o
}

// openPalette lists every registry action. Actions that are blocked or out of context are greyed out.
func (m *Model) openPalette() tea.Cmd {
	m.cmdPerson = m.palettePerson()
	items := make([]overlayItem, 0, len(m.actions))
	for i := range m.actions {
		a := &m.actions[i]
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
	o := m.newOverlay("Command palette", items, true)
	o.onPick = func(m *Model, it overlayItem) {
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
	items := make([]overlayItem, 0, numViews)
	for v := viewKind(0); v < numViews; v++ {
		it := overlayItem{id: "view-" + strings.ToLower(viewNames[v]), label: viewNames[v], hint: strconv.Itoa(int(v) + 1), value: strconv.Itoa(int(v))}
		if v == m.view {
			it.desc = "current view"
		} else {
			it.desc = viewBlurbs[v]
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

// viewBlurbs say what each view is for.
var viewBlurbs = [numViews]string{
	"what needs attention today", "everyone, search with /", "people to reach out to", "open tasks",
	"upcoming birthdays", "probable duplicates", "archived people, restore or purge", "sources and the inbox",
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
	o := m.ov
	page := m.overlayPage()
	if o.palette {
		if mdl, c, ok := m.cmdKey(msg); ok {
			return mdl, c
		}
	}
	switch msg.String() {
	case "esc", "ctrl+c":
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
	if k := msg.String(); o.digits && len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
		for i, idx := range o.shown {
			if o.items[idx].hint == k {
				return m, m.pickOverlay(i)
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	before := o.filter.Value()
	o.filter, cmd = o.filter.Update(msg)
	if o.filter.Value() != before {
		o.refilter()
	}
	m.syncCmd()
	return m, cmd
}

func (m *Model) handleOverlayMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
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
	b.WriteString(lipgloss.NewStyle().MaxWidth(m.width).Render(headStyle.Render(o.title)+dimStyle.Render("   enter run  esc close  up/down choose")) + "\n")
	b.WriteString(o.filter.View() + "\n")
	page := m.overlayPage()
	if len(o.shown) == 0 {
		b.WriteString(dimStyle.Render("No matches."))
		return b.String()
	}
	end := min(len(o.shown), o.offset+page)
	hintW, titleW := 8, 24
	if o.palette {
		hintW = 8
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
			b.WriteString(selStyle.Render(line))
		case it.disabled != "":
			b.WriteString(dimStyle.Render(line))
		default:
			b.WriteString(nameStyle.Render(fit(it.hint, hintW)) + fit(it.label, titleW) + dimStyle.Render(fit(desc, max(0, room))))
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
	case "j", "down":
		m.helpVP.LineDown(1)
	case "k", "up":
		m.helpVP.LineUp(1)
	case "pgdown", "ctrl+d", " ":
		m.helpVP.HalfViewDown()
	case "pgup", "ctrl+u":
		m.helpVP.HalfViewUp()
	default:
		m.help = false
	}
	return m, nil
}

// helpText is generated from the action registry so it matches the palette.
func (m *Model) helpText() string {
	order := []string{"Home", "Contact", "Edit pane", "Bulk", "Sync", "Merge", "Compare", "Archive", "Move", "Views", "App"}
	var b strings.Builder
	for _, g := range order {
		b.WriteString(headStyle.Render(g) + "\n")
		for i := range m.actions {
			a := &m.actions[i]
			if a.group != g {
				continue
			}
			key := a.hint
			if len(a.keys) > 1 {
				key = strings.Join(a.keys, " ")
				key = strings.ReplaceAll(key, " ", ", ")
			}
			if key == "" {
				key = "palette"
			}
			if a.hasKey(" ") {
				key = "space"
			}
			line := nameStyle.Render(fit(key, 22)) + a.desc
			if a.write {
				line += dimStyle.Render("  [edits data]")
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(headStyle.Render("Mouse") + "\n")
	b.WriteString(nameStyle.Render(fit("click", 22)) + "select a row, click again on a detail row to edit it\n")
	b.WriteString(nameStyle.Render(fit("click footer", 22)) + "tap a footer key to run it, tap the status line for the palette\n")
	b.WriteString(nameStyle.Render(fit("wheel, tabs", 22)) + "scroll, switch view (the More tab opens the view list)\n")
	return strings.TrimRight(b.String(), "\n")
}
