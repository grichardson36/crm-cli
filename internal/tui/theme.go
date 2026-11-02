package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
)

// Themes. All colors live in internal/theme. The Model holds the active styles in m.st and every
// view draws with them, so switching a theme is one assignment plus a redraw.
//
// Theme state has one home. The preview, the screen painter and the Settings row all read it:
//
//	themeName   the chosen theme (saved on this device)
//	appearance  the chosen mode: auto, light or dark (saved on this device)
//	previewName the theme shown while the cursor is on it in the Appearance pane, "" when none
//	m.st        the styles that are drawn: the preview when there is one, else the chosen theme
//
// Only chooseTheme and setAppearance change what is chosen. Both redraw the Settings rows.

// defaultStyles are used until a theme is chosen (tests, and the instant before Run applies the
// remembered theme). It does no terminal I/O.
func defaultStyles() theme.Styles {
	t, _ := theme.Builtin("dark", true)
	return theme.NewStyles(t, colorprofile.TrueColor)
}

// setTheme draws with t. It does not change the chosen theme.
func (m *Model) setTheme(t theme.Theme) {
	m.st = theme.NewStyles(t, m.profile)
	m.applyKindColors()
}

// dark says whether the themes that follow the terminal take their dark variant.
func (m *Model) dark() bool { return m.appearance.Dark(m.darkBG) }

// themeCatalog reads the built-in and custom themes from disk.
func (m *Model) themeCatalog() theme.Catalog {
	return theme.LoadCatalog(m.env.DataDir, m.dark())
}

// applyStartupTheme picks the remembered theme (CRM_THEME, then config.toml, then auto).
// A problem never stops the program: it falls back to auto and says why in the status line.
func (m *Model) applyStartupTheme() {
	m.appearance = theme.ChosenMode(m.env.DataDir)
	m.fillMode = m.appearance.Fill()
	m.darkBG = true // until the terminal answers (tea.BackgroundColorMsg)
	name, _ := theme.Chosen(m.env.DataDir, os.Getenv)
	cat := m.themeCatalog()
	t, err := cat.Find(name)
	if err != nil {
		t, _ = cat.Find(theme.AutoName)
		m.setStatus("theme: "+err.Error()+". Using auto.", true)
	}
	m.setTheme(t)
	m.themeName = t.Name
}

// resolveTheme draws the chosen theme again, for a change of mode or of the terminal's background.
// A preview that was showing ends.
func (m *Model) resolveTheme() {
	m.previewName = ""
	cat := m.themeCatalog()
	t, err := cat.Find(m.themeName)
	if err != nil {
		t, _ = cat.Find(theme.AutoName)
	}
	m.setTheme(t)
}

// previewTheme shows a theme without choosing it.
func (m *Model) previewTheme(name string) {
	t, err := m.themeCatalog().Find(name)
	if err != nil {
		return
	}
	m.previewName = name
	m.setTheme(t)
}

// endPreview goes back to the chosen theme when a preview is showing.
func (m *Model) endPreview() {
	if m.previewName != "" {
		m.resolveTheme()
	}
}

// chooseTheme makes a theme the chosen one and remembers it on this device.
func (m *Model) chooseTheme(name string) {
	t, err := m.themeCatalog().Find(name)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.previewName = ""
	m.setTheme(t)
	m.themeName = t.Name
	m.rememberTheme(t.Name)
	m.refreshAppearance()
}

// setAppearance chooses the mode and remembers it on this device.
func (m *Model) setAppearance(md theme.Mode) error {
	if m.env.DataDir == "" {
		return fmt.Errorf("no data directory in this session, so the mode cannot be saved: %w", model.ErrValidation)
	}
	if err := theme.SaveMode(m.env.DataDir, md); err != nil {
		return err
	}
	m.appearance = md
	m.fillMode = md.Fill()
	m.resolveTheme()
	m.refreshAppearance()
	m.setStatus("mode: "+string(md), false)
	return nil
}

// refreshAppearance rebuilds the Settings rows and the Appearance pane after the theme state changed.
func (m *Model) refreshAppearance() {
	if m.view != viewSettings {
		return
	}
	m.rows = m.settingRows()
	m.refreshDetail(true)
}

// rememberTheme saves the choice for this device and reports it in the status line.
func (m *Model) rememberTheme(name string) {
	if m.env.DataDir == "" {
		m.setStatus("theme: "+name+" (not saved, the data directory is unknown)", false)
		return
	}
	if err := theme.SaveChoice(m.env.DataDir, name); err != nil {
		m.setStatus("theme "+name+" applied but not saved: "+err.Error(), true)
		return
	}
	if v := strings.TrimSpace(os.Getenv(theme.EnvName)); v != "" && !strings.EqualFold(v, name) {
		m.setStatus(fmt.Sprintf("theme: %s saved, but %s=%s overrides it at startup", name, theme.EnvName, v), false)
		return
	}
	m.setStatus("theme: "+name, false)
}

// reloadTheme re-reads the theme files and re-applies the chosen theme, so edits to a custom file
// show without restarting. A broken file keeps the current look and shows the error.
func (m *Model) reloadTheme() {
	cat := m.themeCatalog()
	t, err := cat.Find(m.themeName)
	if err != nil {
		m.setStatus("theme reload: "+err.Error()+" (keeping the current look)", true)
		return
	}
	m.previewName = ""
	m.setTheme(t)
	if n := len(cat.Problems); n > 0 {
		m.setStatus(fmt.Sprintf("theme reloaded: %s. %d theme file(s) have errors: %s", t.Name, n, cat.Problems[0].Err), true)
		return
	}
	m.setStatus("theme reloaded: "+t.Name, false)
}

// fillScreen reports whether the frame is painted with the theme background: the mode allows it for
// this theme and the terminal shows colors at all (NO_COLOR keeps the terminal's own).
func (m *Model) fillScreen() bool {
	return m.profile > colorprofile.Ascii && m.st.Theme.Fills(m.fillMode, m.darkBG)
}

// paintView gives every cell of the frame the theme background and sets the terminal's default
// colors (OSC 10 and 11) so margins and cleared areas match. Bubble Tea restores the terminal's own
// colors when the program exits, and sets or resets them when the view's fields change.
func (m *Model) paintView(v *tea.View) {
	if !m.fillScreen() || m.width <= 0 {
		// Not every terminal honors the reset sequence. A screen that was painted gives the
		// terminal's own background back explicitly.
		if m.oscPainted && m.termBG != nil {
			v.BackgroundColor = m.termBG
		}
		return
	}
	m.oscPainted = true
	t := m.st.Theme
	v.Content = t.Paint(v.Content, m.width, m.height)
	v.ForegroundColor, v.BackgroundColor = t.DefaultColors()
}

// --- the Appearance pane ---

// appearanceID is the Settings row that holds the mode and the theme list.
const appearanceID = "appearance"

// appearanceRows are the rows of the Appearance pane: the mode selector, then one row per theme.
func (m *Model) appearanceRows() []formRow {
	rows := []formRow{{kind: rkMode, label: "mode", value: string(m.appearance), head: "Mode"}}
	cat := m.themeCatalog()
	for i, t := range cat.Themes {
		r := formRow{kind: rkTheme, label: t.Name, value: t.Name, suffix: "  " + t.Description}
		if i == 0 {
			r.head = "Theme"
		}
		if t.Name == m.themeName {
			r.label += " *"
		}
		if !t.Builtin {
			r.suffix += " (custom: " + t.Path + ")"
		}
		rows = append(rows, r)
	}
	for _, p := range cat.Problems {
		rows = append(rows, formRow{kind: rkInfo, label: baseName(p.File), value: "cannot load: " + p.Err, dim: true})
	}
	return rows
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// appearanceOpen reports whether the detail pane shows the Appearance pane.
func (m *Model) appearanceOpen() bool {
	return m.view == viewSettings && m.cmp == nil && len(m.form) > 0 && m.form[0].kind == rkMode
}

// whenModeRow is true with the cursor on the mode selector.
func whenModeRow(m *Model) bool {
	r, ok := m.rowFocused()
	return whenDetail(m) && ok && r.kind == rkMode
}

// whenThemeRow is true with the cursor on a theme of the Appearance pane.
func whenThemeRow(m *Model) bool {
	r, ok := m.rowFocused()
	return whenDetail(m) && ok && r.kind == rkTheme
}

// whenAppearanceRow is true on a row that has its own keys: the mode selector or a theme.
func whenAppearanceRow(m *Model) bool { return whenModeRow(m) || whenThemeRow(m) }

// stepMode moves the mode selector and saves the new mode.
func (m *Model) stepMode(delta int) tea.Cmd {
	if m.readOnly {
		m.setStatus("read-only: the mode cannot be changed", true)
		return nil
	}
	i := 0
	for j, md := range theme.Modes {
		if md == m.appearance {
			i = j
		}
	}
	if i+delta < 0 { // left of the first mode is the edge of the pane: back to the Settings list
		m.focus, m.fcol = focusList, 0
		m.endPreview()
		m.syncDetail()
		return nil
	}
	if i+delta >= len(theme.Modes) { // right of the last mode stays
		return nil
	}
	if err := m.setAppearance(theme.Modes[i+delta]); err != nil {
		m.setStatus(err.Error(), true)
	}
	return nil
}

// previewRow shows the theme under the cursor of the Appearance pane. Any other row ends the preview.
func (m *Model) previewRow() {
	if !m.appearanceOpen() {
		return
	}
	if r, ok := m.rowFocused(); ok && r.kind == rkTheme && m.focus == focusDetail {
		if r.value != m.previewName && !(m.previewName == "" && r.value == m.themeName) {
			m.previewTheme(r.value)
		}
		return
	}
	m.endPreview()
}

// chooseRow is enter on a theme row.
func (m *Model) chooseRow() tea.Cmd {
	if r, ok := m.rowFocused(); ok && r.kind == rkTheme {
		if m.readOnly {
			m.setStatus("read-only: the theme cannot be changed", true)
			return nil
		}
		m.chooseTheme(r.value)
	}
	return nil
}

// renderModeRow draws "mode  auto | light | dark" with the chosen mode marked.
func (m *Model) renderModeRow(r formRow, label string, selected bool, w int) string {
	var parts []string
	for _, md := range theme.Modes {
		name := string(md)
		if md == m.appearance {
			parts = append(parts, m.st.Sel.Render(" "+name+" "))
		} else {
			parts = append(parts, m.st.Dim.Render(" "+name+" "))
		}
	}
	line := m.st.Dim.Render(label) + strings.Join(parts, m.st.Dim.Render("|"))
	if selected {
		line += m.st.Dim.Render("  left and right change it")
	}
	return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(line)
}

// renderThemeRow draws a theme as its name and description. The chosen theme is marked with *.
func (m *Model) renderThemeRow(r formRow, selected bool, w int) string {
	name := fit(r.label, 20)
	if selected {
		return m.st.Sel.Render(fit(name+ansi.Truncate(r.suffix, max(1, w-20), "…"), w))
	}
	return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(name + m.st.Dim.Render(r.suffix))
}

func appearanceActions() []action {
	return []action{
		{id: "theme-pick", title: "Choose theme", group: "Edit pane", hint: "enter", keys: []string{"enter"}, desc: "in Settings, Appearance: choose the theme under the cursor and save it on this device",
			foot: 16, footIn: inDetail, footLabel: "choose", when: whenThemeRow, run: func(m *Model) tea.Cmd { return m.chooseRow() }},
		{id: "mode-next", title: "Next mode", group: "Edit pane", hint: "right", keys: []string{"right"}, desc: "in Settings, Appearance: next mode (auto, light, dark), saved on this device",
			foot: 16, footIn: inDetail, footLabel: "mode", when: whenModeRow, run: func(m *Model) tea.Cmd { return m.stepMode(1) }},
		{id: "mode-prev", title: "Previous mode", group: "Edit pane", hint: "left", keys: []string{"left"}, desc: "in Settings, Appearance: previous mode (auto, light, dark), saved on this device",
			when: whenModeRow, run: func(m *Model) tea.Cmd { return m.stepMode(-1) }},
	}
}
