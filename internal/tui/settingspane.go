package tui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
)

// Panes of the Settings screen that are not the list editor (listeditor.go): the Views row shows the
// Views list and the Colors row shows one color per object kind. Like the list editor they live in
// the right pane of their row, enter on the row enters the pane and esc leaves it
// (patterns/navigation-standard). The pane has no filter line, so bare keys are free: the footer
// shows a key only when it works on the highlighted row.

const (
	viewsPaneID  = "views"
	colorsPaneID = "colors"
)

// settingsPane is a pane of a Settings row. key returns false for a key the pane does not use, which
// then goes on to the registry (the palette, help, quit).
type settingsPane interface {
	id() string
	lines(m *Model, w, h int) []string
	hints(m *Model) []hintItem
	key(m *Model, msg tea.KeyPressMsg) bool
}

// paneSPID is the pane a Settings row shows, "" for any other row.
func paneSPID(settingID string) string {
	if settingID == viewsPaneID || settingID == colorsPaneID {
		return settingID
	}
	return ""
}

// paneSP is the pane of the Settings row under the cursor, nil on any other screen or row.
func (m *Model) paneSP() settingsPane {
	if m.sp == nil || m.view != viewSettings || m.cmp != nil {
		return nil
	}
	return m.sp
}

// paneSPFocused is the pane while the cursor is inside it.
func (m *Model) paneSPFocused() settingsPane {
	if p := m.paneSP(); p != nil && m.focus == focusDetail {
		return p
	}
	return nil
}

// ensureSettingsPane makes m.sp the pane of the row id. The same pane is read again and keeps its cursor.
func (m *Model) ensureSettingsPane(id string) {
	switch id {
	case viewsPaneID:
		if p, ok := m.sp.(*viewsPane); ok {
			cur := ""
			if it, ok := p.o.highlighted(); ok {
				cur = it.value
			}
			p.o.items = m.viewItems()
			p.o.refilter()
			p.focus(m, cur)
			return
		}
		m.sp = &viewsPane{o: m.newViewsListFor("", true)}
	case colorsPaneID:
		if _, ok := m.sp.(*colorsPane); ok {
			return
		}
		m.sp = &colorsPane{}
	default:
		m.sp = nil
	}
}

// openSettingsPane goes to the Settings row of a pane with the cursor in it.
func (m *Model) openSettingsPane(id string) tea.Cmd {
	m.gotoSetting(id)
	if m.view == viewSettings && m.paneSP() != nil && m.sp.id() == id {
		m.focus = focusDetail
		m.fcol = 0
		m.syncDetail()
	}
	return nil
}

// settingsPaneKey takes every key while the cursor is in a pane.
func (m *Model) settingsPaneKey(p settingsPane, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.pendingCmd = nil
	if !p.key(m, msg) {
		return m.handleKey(msg)
	}
	if m.paneSP() != nil {
		m.syncDetail()
	}
	cmd := m.pendingCmd
	m.pendingCmd = nil
	return m, cmd
}

// --- Views ---

// viewsPane is the Views list embedded in the Settings pane.
type viewsPane struct {
	o    *overlay
	page int
}

func (p *viewsPane) id() string { return viewsPaneID }

func (p *viewsPane) focus(m *Model, value string) {
	for i, idx := range p.o.shown {
		if value != "" && p.o.items[idx].value == value {
			p.o.cursor = i
		}
	}
	p.o.move(0, max(1, p.page))
}

func (p *viewsPane) lines(m *Model, w, h int) []string {
	if w < 1 || h < 1 {
		return nil
	}
	lines := []string{lipgloss.NewStyle().MaxWidth(w).Render(m.headingStyle().Render("Views"))}
	p.page = max(1, h-1)
	p.o.move(0, p.page)
	return append(lines, m.popupListRows(p.o, w, p.page)...)
}

func (p *viewsPane) hints(m *Model) []hintItem { return m.viewsHints(p.o, true) }

func (p *viewsPane) key(m *Model, msg tea.KeyPressMsg) bool {
	k := keyName(msg)
	o := p.o
	page := max(1, p.page)
	switch k {
	case "esc", "left", "shift+tab":
		m.leavePane()
	case "up":
		o.move(-1, page)
	case "down":
		o.move(1, page)
	case "pgup":
		o.move(-page, page)
	case "pgdown":
		o.move(page, page)
	case "home":
		o.cursor = 0
		o.move(0, page)
	case "end":
		o.cursor = len(o.shown) - 1
		o.move(0, page)
	case "enter":
		if it, ok := o.highlighted(); ok && !it.divider && o.onPick != nil {
			o.onPick(m, it)
		}
	default:
		return o.onKey(m, k)
	}
	return true
}

// --- Colors ---

// colorsPane is one row per object kind, in the order of the Views, each with a swatch and a hex
// field. The values are the per-kind overrides of kindcolors.go (the [colors] table of config.toml).
type colorsPane struct {
	cursor, offset, page int
	editing              bool
	in                   textinput.Model
}

func (p *colorsPane) id() string { return colorsPaneID }

// paneColorKinds are the kinds the pane lists: person, organization, deal, task, note and tag, in the
// order of their views among the modules (a kind without a view of its own goes last).
func (m *Model) paneColorKinds() []colorKind {
	rank := map[string]string{"person": "people", "organization": "organizations", "deal": "deals", "task": "tasks", tagKind: "tags"}
	order := m.mods.Normalize().Order
	pos := func(kind string) int {
		for i, id := range order {
			if id == rank[kind] {
				return i
			}
		}
		return len(order)
	}
	var out []colorKind
	for _, ck := range colorKinds() {
		if ck.kind != model.SavedViewKind {
			out = append(out, ck)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return pos(out[a].kind) < pos(out[b].kind) })
	return out
}

// kindHex is the color of a kind as shown: the override or the theme's own.
func (m *Model) kindHex(kind string) string {
	return resolveKindColor(m.st.Look, m.kindColors, kind)
}

func (p *colorsPane) cur(m *Model) (colorKind, bool) {
	ks := m.paneColorKinds()
	if p.cursor < 0 || p.cursor >= len(ks) {
		return colorKind{}, false
	}
	return ks[p.cursor], true
}

func (p *colorsPane) lines(m *Model, w, h int) []string {
	if w < 1 || h < 1 {
		return nil
	}
	cut := func(s string) string { return ansi.Truncate(s, w, "…") }
	lines := []string{lipgloss.NewStyle().MaxWidth(w).Render(m.headingStyle().Render("Colors"))}
	lines = append(lines, m.st.Dim.Render(cut(fit("", 5)+fit("kind", 15)+fit("hex", 11)+"source")))
	ks := m.paneColorKinds()
	p.page = max(1, h-len(lines))
	p.cursor = clampInt(p.cursor, 0, max(0, len(ks)-1))
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.page {
		p.offset = p.cursor - p.page + 1
	}
	focused := m.focus == focusDetail
	for i := p.offset; i < min(len(ks), p.offset+p.page); i++ {
		ck := ks[i]
		hex := m.kindHex(ck.kind)
		if hex == "" {
			hex = "-"
		}
		source := "theme"
		if _, ok := m.kindColors[ck.kind]; ok {
			source = "custom"
		}
		swatch := m.st.KindStyle(ck.kind).Render("██")
		if i == p.cursor && p.editing {
			if v, ok := validKindColor(p.in.Value()); ok {
				if c := paletteColor(m.st.Look, strings.ToLower(v)); c != "" {
					v = c
				}
				swatch = themeSwatch(m, v)
			}
			lines = append(lines, cut(" "+swatch+"  "+fit(ck.label, 15)+p.in.View()))
			continue
		}
		if i == p.cursor && focused {
			lines = append(lines, cut(" "+swatch+"  "+m.st.Sel.Render(fit(ck.label, 15)+fit(hex, 11)+fit(source, max(0, w-5-15-11)))))
			continue
		}
		lines = append(lines, cut(" "+swatch+"  "+fit(ck.label, 15)+fit(hex, 11)+m.st.Dim.Render(source)))
	}
	return lines
}

// themeSwatch is the swatch in a hex color, or plain when the terminal has no color.
func themeSwatch(m *Model, hex string) string {
	return theme.ForegroundStyle(hex).Render("██")
}

func (p *colorsPane) hints(m *Model) []hintItem {
	if p.editing {
		return []hintItem{fk("enter", "save"), fk("esc", "cancel")}
	}
	var h []hintItem
	ks := m.paneColorKinds()
	if len(ks) > 1 {
		h = append(h, fk("up/down", "select"))
	}
	if !m.readOnly {
		h = append(h, fk("e", "edit"))
		if ck, ok := p.cur(m); ok {
			if _, custom := m.kindColors[ck.kind]; custom {
				h = append(h, fk("shift+r", "reset"))
			}
		}
	}
	return append(h, fk("esc", "close"))
}

func (p *colorsPane) startEdit(m *Model) {
	ck, ok := p.cur(m)
	if !ok {
		return
	}
	in := newTextInput()
	in.SetValue(m.kindHex(ck.kind))
	in.CursorEnd()
	_ = in.Focus()
	p.in, p.editing = in, true
}

func (p *colorsPane) save(m *Model) {
	ck, ok := p.cur(m)
	if !ok {
		return
	}
	v, valid := validKindColor(p.in.Value())
	if !valid {
		m.setStatus(ck.label+" color: use #rrggbb or a palette name (accent good warn danger mark fg dim tab)", true)
		return
	}
	if err := m.setKindColor(ck.kind, v); err != nil {
		m.setStatus(ck.label+" color: "+err.Error(), true)
		return
	}
	p.editing = false
	m.setStatus(ck.label+" color: "+v, false)
}

func (p *colorsPane) key(m *Model, msg tea.KeyPressMsg) bool {
	k := keyName(msg)
	if p.editing {
		switch k {
		case "enter":
			p.save(m)
		case "esc":
			p.editing = false
			m.setStatus("", false)
		default:
			var cmd tea.Cmd
			p.in, cmd = p.in.Update(msg)
			m.pendingCmd = cmd
		}
		return true
	}
	ks := m.paneColorKinds()
	page := max(1, p.page)
	switch k {
	case "esc", "left", "shift+tab":
		m.leavePane()
	case "up":
		p.cursor = max(0, p.cursor-1)
	case "down":
		p.cursor = min(len(ks)-1, p.cursor+1)
	case "pgup":
		p.cursor = max(0, p.cursor-page)
	case "pgdown":
		p.cursor = min(len(ks)-1, p.cursor+page)
	case "home":
		p.cursor = 0
	case "end":
		p.cursor = len(ks) - 1
	case "e", "enter":
		if m.readOnly {
			m.setStatus("read-only: colors cannot be changed", true)
			return true
		}
		p.startEdit(m)
	case "R":
		if m.readOnly {
			m.setStatus("read-only: colors cannot be changed", true)
			return true
		}
		ck, ok := p.cur(m)
		if !ok {
			return true
		}
		if _, custom := m.kindColors[ck.kind]; !custom {
			return true
		}
		if err := m.setKindColor(ck.kind, ""); err != nil {
			m.setStatus(ck.label+" color: "+err.Error(), true)
			return true
		}
		m.setStatus(ck.label+" color is back to the theme", false)
	default:
		return false
	}
	return true
}

// colorsSetting is the one Settings row of the Colors section.
func (m *Model) colorsSetting() setting {
	return setting{id: colorsPaneID, section: secColors, name: "Colors", scope: scopeDevice,
		words: "color colour highlight kind chip palette hex person organization deal task note tag",
		desc:  "the highlight color of each kind",
		value: func(m *Model) string {
			if n := len(m.kindColors); n > 0 {
				return countText(n, "custom color")
			}
			return "theme colors"
		},
		run: func(m *Model) tea.Cmd { return m.openSettingsPane(colorsPaneID) }}
}

