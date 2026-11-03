package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
)

// Themes. All colors live in internal/theme. The Model holds the active styles in m.st and every
// view draws with them, so switching a theme is one assignment plus a redraw.
//
// A theme is one entry with a dark and a light palette. Theme state has one home. The preview, the
// screen painter and the Settings row all read it:
//
//	themeName   the chosen theme (saved on this device)
//	appearance  the chosen mode: auto, light or dark (saved on this device)
//	previewName the theme shown while the cursor is on it in the Theme pane, "" when none
//	m.st        the styles that are drawn: the preview when there is one, else the chosen theme
//
// Only chooseTheme and setAppearance change what is chosen. Both redraw the Settings rows. The Theme
// pane keeps two more values of its own: themeSel (the theme under the cursor, whose colors are
// listed) and themeEditing (the cursor is in the name and color rows).

// defaultStyles are used until a theme is chosen (tests, and the instant before Run applies the
// remembered theme). It does no terminal I/O.
func defaultStyles() theme.Styles {
	t, _ := theme.Builtin(theme.DefaultName)
	return theme.NewStyles(t.Resolve(true), colorprofile.TrueColor)
}

// setTheme draws with a look. It does not change the chosen theme.
func (m *Model) setTheme(l theme.Look) {
	m.st = theme.NewStyles(l, m.profile)
	m.applyKindColors()
	// The detail viewport holds text drawn with the old styles. Draw it again.
	if m.width > 0 {
		m.syncDetail()
	}
}

// dark says whether the dark palette is used: the mode, else what the terminal reported.
func (m *Model) dark() bool { return m.appearance.Dark(m.darkBG) }

// themeCatalog reads the built-in and custom themes from disk.
func (m *Model) themeCatalog() theme.Catalog { return theme.LoadCatalog(m.env.DataDir) }

// look is a theme as drawn in the current mode.
func (m *Model) look(t theme.Theme) theme.Look { return t.Resolve(m.dark()) }

// applyStartupTheme picks the remembered theme (CRM_THEME, then config.toml, then default). A problem
// never stops the program: an unknown name falls back to default and the status line says why.
func (m *Model) applyStartupTheme() {
	m.appearance = theme.ChosenMode(m.env.DataDir)
	m.darkBG = true // until the terminal answers (tea.BackgroundColorMsg)
	name, _ := theme.Chosen(m.env.DataDir, os.Getenv)
	cat := m.themeCatalog()
	t, err := cat.Find(name)
	if err != nil {
		t, _ = cat.Find(theme.DefaultName)
		m.setStatus("theme: "+err.Error(), true)
	}
	m.setTheme(m.look(t))
	m.themeName = t.Name
}

// resolveTheme draws the chosen theme again, for a change of mode or of the terminal's background.
// A preview that was showing ends.
func (m *Model) resolveTheme() {
	m.previewName = ""
	cat := m.themeCatalog()
	t, err := cat.Find(m.themeName)
	if err != nil {
		t, _ = cat.Find(theme.DefaultName)
	}
	m.setTheme(m.look(t))
}

// previewTheme shows a theme without choosing it.
func (m *Model) previewTheme(name string) {
	t, err := m.themeCatalog().Find(name)
	if err != nil {
		return
	}
	m.previewName = t.Name
	m.setTheme(m.look(t))
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
	m.setTheme(m.look(t))
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
	m.redrawTheme()
	return nil
}

// redrawTheme draws the chosen theme again after the mode, the terminal background or a theme file
// changed. A preview under the cursor of the Theme pane stays, and the pane and the Settings rows
// are rebuilt for the palette now shown.
func (m *Model) redrawTheme() {
	m.resolveTheme()
	if m.appearanceOpen() && m.focus == focusDetail && m.themeSel != "" && !strings.EqualFold(m.themeSel, m.themeName) {
		m.previewTheme(m.themeSel)
	}
	m.refreshAppearance()
}

// refreshAppearance rebuilds the Settings rows and the Theme pane after the theme state changed.
func (m *Model) refreshAppearance() {
	if m.view != viewSettings {
		return
	}
	m.rows = m.settingRows()
	m.refreshDetail(true)
}

// rememberTheme saves the choice for this device and reports a problem in the status line.
func (m *Model) rememberTheme(name string) {
	if m.env.DataDir == "" {
		m.setStatus("theme "+name+" not saved, the data directory is unknown", true)
		return
	}
	if err := theme.SaveChoice(m.env.DataDir, name); err != nil {
		m.setStatus("theme "+name+" not saved: "+err.Error(), true)
		return
	}
	if v := strings.TrimSpace(os.Getenv(theme.EnvName)); v != "" && !strings.EqualFold(v, name) {
		m.setStatus(fmt.Sprintf("%s=%s overrides it at startup", theme.EnvName, v), false)
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
		m.setStatus("theme reload: "+err.Error(), true)
		return
	}
	m.redrawTheme()
	if n := len(cat.Problems); n > 0 {
		m.setStatus(fmt.Sprintf("%d theme file(s) have errors: %s", n, cat.Problems[0].Err), true)
		return
	}
	m.setStatus("theme reloaded: "+t.Name, false)
}

// fillScreen reports whether the frame is painted with the theme background: the mode is light or
// dark and the terminal shows colors at all (NO_COLOR keeps the terminal's own).
func (m *Model) fillScreen() bool {
	return m.profile > colorprofile.Ascii && m.appearance.Paints() && m.st.Look.Bg != ""
}

// paintView gives every cell of the frame the theme background and sets the terminal's default
// colors (OSC 10 and 11) so margins and cleared areas match. Bubble Tea restores the terminal's own
// colors when the program exits, and sets or resets them when the view's fields change.
func (m *Model) paintView(v *tea.View) {
	if !m.fillScreen() || m.width <= 0 {
		// Auto never sets a terminal color. Nil colors make Bubble Tea reset OSC 10 and 11, so a
		// transparent or custom terminal background is kept.
		return
	}
	t := m.st.Look
	v.Content = t.Paint(v.Content, m.width, m.height)
	v.ForegroundColor, v.BackgroundColor = t.DefaultColors()
}

// --- the Theme pane ---

// appearanceID is the Settings row that holds the mode and the theme list.
const appearanceID = "appearance"

// colorLabelWidth is the label column of a color row, wide enough for "selection background".
const colorLabelWidth = 22

// themeShown is the theme whose colors the pane lists: the one under the cursor, else the chosen one.
func (m *Model) themeShown() string {
	if m.themeSel != "" {
		return m.themeSel
	}
	return m.themeName
}

// themeRef marks a row as a field of a theme (saveTyped).
var themeRef = repo.Ref{Kind: "theme"}

// appearanceRows are the rows of the Theme pane: the mode line, the theme list, then the name and
// the colors of the theme under the cursor.
func (m *Model) appearanceRows() []formRow {
	cat := m.themeCatalog()
	m.themeCat = cat
	rows := []formRow{{kind: rkMode, label: "mode", value: string(m.appearance)}, {kind: rkInfo}}
	for _, t := range cat.Themes {
		r := formRow{kind: rkTheme, label: t.Name, value: t.Name}
		if strings.EqualFold(t.Name, m.themeName) {
			r.primary = true
		}
		rows = append(rows, r)
	}
	problems := func() {
		for _, p := range cat.Problems {
			rows = append(rows, formRow{kind: rkInfo, label: baseName(p.File), value: firstProblem(p.Err), dim: true})
		}
	}
	shown, err := cat.Find(m.themeShown())
	if err != nil {
		problems()
		return rows
	}
	rows = append(rows, formRow{kind: rkInfo})
	nameSpec := model.FieldSpec{Key: "name", Label: "name", Type: model.FieldText, Required: true, Max: theme.MaxName}
	name := specRow(themeRef, nameSpec, shown.Name)
	name.kind = rkThemeName
	name.suffix = map[bool]string{true: "dark", false: "light"}[m.dark()]
	name.dim = shown.Builtin
	rows = append(rows, name)
	p := shown.Pick(m.dark())
	for _, rl := range theme.EditableRoles {
		spec := model.FieldSpec{Key: "color:" + rl.Role, Label: rl.Label, Type: model.FieldText, Required: true, Max: 9, Hint: "#rrggbb"}
		r := specRow(themeRef, spec, p.Color(rl.Role))
		r.kind = rkColor
		r.dim = shown.Builtin
		rows = append(rows, r)
	}
	problems()
	return rows
}

// firstProblem is the first complaint of a theme file error, without the file name prefix.
func firstProblem(err string) string {
	if i := strings.Index(err, "): "); i >= 0 {
		err = err[i+3:]
	}
	first, _, _ := strings.Cut(err, "; ")
	return first
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// appearanceOpen reports whether the detail pane shows the Theme pane.
func (m *Model) appearanceOpen() bool {
	return m.view == viewSettings && m.cmp == nil && len(m.form) > 0 && m.form[0].kind == rkMode
}

// selectableRow says whether the cursor may rest on a row of the Theme pane: the themes while
// browsing, the name and the colors while editing.
func (m *Model) selectableRow(r formRow) bool {
	if m.themeEditing {
		return r.kind == rkThemeName || r.kind == rkColor
	}
	return r.kind == rkTheme
}

// themeRowIndex is the form index of a theme row, -1 when there is none.
func (m *Model) themeRowIndex(name string) int {
	for i, r := range m.form {
		if r.kind == rkTheme && strings.EqualFold(r.value, name) {
			return i
		}
	}
	return -1
}

// entryRow is where the cursor starts in the pane: the chosen theme.
func (m *Model) entryRow() int {
	if i := m.themeRowIndex(m.themeName); i >= 0 {
		return i
	}
	for i, r := range m.form {
		if m.selectableRow(r) {
			return i
		}
	}
	return 0
}

// selectedTheme is the theme under the cursor of the pane.
func (m *Model) selectedTheme() (theme.Theme, bool) {
	t, err := m.themeCat.Find(m.themeShown())
	return t, err == nil
}

// whenThemePane is true while the Theme pane has the focus.
func whenThemePane(m *Model) bool { return whenDetail(m) && m.appearanceOpen() }

// whenThemeBrowse is true with the cursor on a theme of the list.
func whenThemeBrowse(m *Model) bool {
	r, ok := m.rowFocused()
	return whenThemePane(m) && !m.themeEditing && ok && r.kind == rkTheme
}

// whenThemeCustom is true with the cursor on a theme that is a file of the user.
func whenThemeCustom(m *Model) bool {
	if !whenThemeBrowse(m) {
		return false
	}
	t, ok := m.selectedTheme()
	return ok && !t.Builtin
}

// whenThemeReset is true on a custom theme that remembers the theme it was copied from.
func whenThemeReset(m *Model) bool {
	if !whenThemeCustom(m) {
		return false
	}
	t, _ := m.selectedTheme()
	return t.CopiedFrom != ""
}

// openAppearance goes to the Theme row of Settings with the cursor in its pane.
func (m *Model) openAppearance() tea.Cmd {
	m.gotoSetting(appearanceID)
	if m.view == viewSettings && m.appearanceOpen() {
		m.themeSel, m.themeEditing = "", false
		m.focus = focusDetail
		m.fcur, m.fcol = m.entryRow(), 0
		m.syncDetail()
	}
	return nil
}

// leaveAppearance ends the preview and puts the pane back to the chosen theme.
func (m *Model) leaveAppearance() {
	m.themeSel, m.themeEditing = "", false
	m.endPreview()
}

// rebuildAppearance rebuilds the pane after a change to the themes or to the theme under the cursor,
// and keeps the cursor on a row that can hold it.
func (m *Model) rebuildAppearance() {
	m.form = m.appearanceRows()
	if m.fcur >= len(m.form) || !m.selectableRow(m.form[m.fcur]) {
		m.fcur = m.entryRow()
		if i := m.themeRowIndex(m.themeShown()); i >= 0 && !m.themeEditing {
			m.fcur = i
		}
	}
	m.syncDetail()
}

// stepAppearance moves the cursor over the rows that can hold it. Over a theme it previews it.
func (m *Model) stepAppearance(delta int) {
	i := m.fcur
	for step := 0; step < abs(delta); step++ {
		dir := 1
		if delta < 0 {
			dir = -1
		}
		for j := i + dir; j >= 0 && j < len(m.form); j += dir {
			if m.selectableRow(m.form[j]) {
				i = j
				break
			}
		}
	}
	m.fcur, m.fcol = i, 0
	if m.themeEditing {
		m.syncDetail()
		return
	}
	if r := m.form[i]; r.kind == rkTheme && !strings.EqualFold(r.value, m.themeShown()) {
		m.themeSel = r.value
		m.previewTheme(r.value)
		m.form = m.appearanceRows()
	}
	m.syncDetail()
}

// themeLoad is enter on a theme: it becomes the chosen one.
func (m *Model) themeLoad() tea.Cmd {
	r, ok := m.rowFocused()
	if !ok || r.kind != rkTheme {
		return nil
	}
	if m.readOnly {
		m.setStatus("read-only: the theme cannot be changed", true)
		return nil
	}
	m.chooseTheme(r.value)
	return nil
}

// themeEdit is e on a custom theme: the cursor moves into its name and colors.
func (m *Model) themeEdit() tea.Cmd {
	m.themeEditing = true
	m.form = m.appearanceRows()
	for i, r := range m.form {
		if m.selectableRow(r) {
			m.fcur = i
			break
		}
	}
	m.syncDetail()
	return nil
}

// themeBack is esc with the cursor in the colors: back to the theme list.
func (m *Model) themeBack() {
	m.themeEditing = false
	m.rebuildAppearance()
	if i := m.themeRowIndex(m.themeShown()); i >= 0 {
		m.fcur = i
	}
	m.syncDetail()
}

// themeSaved writes a theme, rebuilds the pane around it and shows it.
func (m *Model) themeSaved(t theme.Theme) error {
	saved, err := theme.Save(m.env.DataDir, t)
	if err != nil {
		return err
	}
	m.themeSel = saved.Name
	m.rebuildAppearance()
	if !m.themeEditing {
		if i := m.themeRowIndex(saved.Name); i >= 0 {
			m.fcur = i
		}
	}
	m.previewTheme(saved.Name)
	if strings.EqualFold(saved.Name, m.themeName) {
		m.previewName = ""
	}
	m.form = m.appearanceRows()
	m.syncDetail()
	return nil
}

// themeAdd is a on a custom theme: a new theme with the default colors.
func (m *Model) themeAdd() tea.Cmd {
	if m.env.DataDir == "" {
		m.setStatus("no data directory in this session, so a theme cannot be saved", true)
		return nil
	}
	t, _ := theme.Builtin(theme.DefaultName)
	t.Name, t.Builtin, t.CopiedFrom, t.Description = m.themeCat.NewName(), false, theme.DefaultName, ""
	if err := m.themeSaved(t); err != nil {
		m.setStatus(err.Error(), true)
	}
	return nil
}

// themeCopy is c: a copy of the theme under the cursor.
func (m *Model) themeCopy() tea.Cmd {
	src, ok := m.selectedTheme()
	if !ok {
		return nil
	}
	if m.env.DataDir == "" {
		m.setStatus("no data directory in this session, so a theme cannot be saved", true)
		return nil
	}
	c := src
	c.Name, c.Builtin, c.Path, c.CopiedFrom, c.Description = m.themeCat.CopyName(src.Name), false, "", src.Name, ""
	if err := m.themeSaved(c); err != nil {
		m.setStatus(err.Error(), true)
	}
	return nil
}

// themeDelete is d on a custom theme, after a question. The chosen theme falls back to default.
func (m *Model) themeDelete() tea.Cmd {
	t, ok := m.selectedTheme()
	if !ok || t.Builtin {
		return nil
	}
	next := m.neighbor(m.themeCat, t.Name)
	m.mode = modeConfirm
	m.confirm = &confirmState{prompt: fmt.Sprintf("delete %s?", t.Name), yes: func() {
		if err := theme.Remove(t); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		if strings.EqualFold(t.Name, m.themeName) {
			m.chooseTheme(theme.DefaultName)
		}
		m.themeSel = next
		m.previewTheme(next)
		m.rebuildAppearance()
		if i := m.themeRowIndex(next); i >= 0 {
			m.fcur = i
		}
		m.syncDetail()
	}}
	return nil
}

// neighbor is the theme next to a removed one in the list: the one before it, else default.
func (m *Model) neighbor(cat theme.Catalog, removed string) string {
	prev := theme.DefaultName
	for _, t := range cat.Themes {
		if strings.EqualFold(t.Name, removed) {
			break
		}
		prev = t.Name
	}
	return prev
}

// themeReset is shift+r on a custom theme, after a question: both palettes go back to the theme it
// was copied from.
func (m *Model) themeReset() tea.Cmd {
	t, ok := m.selectedTheme()
	if !ok || t.Builtin || t.CopiedFrom == "" {
		return nil
	}
	src, err := m.themeCat.Find(t.CopiedFrom)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{prompt: fmt.Sprintf("reset %s to %s?", t.Name, src.Name), yes: func() {
		t.Dark, t.Light = src.Dark, src.Light
		if err := m.themeSaved(t); err != nil {
			m.setStatus(err.Error(), true)
		}
	}}
	return nil
}

// themeNextMode is tab: auto, light, dark and around.
func (m *Model) themeNextMode() tea.Cmd {
	if m.readOnly {
		m.setStatus("read-only: the mode cannot be changed", true)
		return nil
	}
	next := theme.Modes[0]
	for j, md := range theme.Modes {
		if md == m.appearance {
			next = theme.Modes[(j+1)%len(theme.Modes)]
		}
	}
	if err := m.setAppearance(next); err != nil {
		m.setStatus(err.Error(), true)
	}
	return nil
}

// saveThemeField writes the name or one color of the theme under the cursor, from the typed editor.
func (m *Model) saveThemeField(key, v string) error {
	t, ok := m.selectedTheme()
	if !ok || t.Builtin {
		return fmt.Errorf("this theme cannot be changed: %w", model.ErrValidation)
	}
	if m.env.DataDir == "" {
		return fmt.Errorf("no data directory in this session, so a theme cannot be saved: %w", model.ErrValidation)
	}
	v = strings.TrimSpace(v)
	if key == "name" {
		if err := theme.ValidName(v); err != nil {
			return fmt.Errorf("%v: %w", err, model.ErrValidation)
		}
		if other, err := m.themeCat.Find(v); err == nil && !strings.EqualFold(other.Name, t.Name) {
			return fmt.Errorf("a theme named %q exists: %w", other.Name, model.ErrConflict)
		}
		wasChosen := strings.EqualFold(t.Name, m.themeName)
		t.Name = v
		if err := m.themeSaved(t); err != nil {
			return err
		}
		if wasChosen {
			m.themeName = v
			m.rememberTheme(v)
			m.refreshAppearance()
		}
		return nil
	}
	role := strings.TrimPrefix(key, "color:")
	if isBareHex(v) {
		v = "#" + v
	}
	if err := theme.ValidColor(v); err != nil {
		return fmt.Errorf("%v: %w", err, model.ErrValidation)
	}
	if strings.HasPrefix(v, "#") {
		v = strings.ToUpper(v)
	}
	p := t.Pick(m.dark())
	if !p.SetColor(role, v) {
		return fmt.Errorf("no color role %q: %w", role, model.ErrValidation)
	}
	t.Set(m.dark(), p)
	return m.themeSaved(t)
}

// isBareHex is six or three hex digits without the #.
func isBareHex(s string) bool {
	if len(s) != 6 && len(s) != 3 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// renderModeRow draws "mode  auto | light | dark" with the chosen mode marked.
func (m *Model) renderModeRow(_ formRow, label string, w int) string {
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
	return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(line)
}

// renderThemeRow draws a theme as its name. The chosen theme is marked with >.
func (m *Model) renderThemeRow(r formRow, selected bool, w int) string {
	mark := " "
	if r.primary {
		mark = ">"
	}
	line := fit(mark+r.label, w)
	if selected {
		return m.st.Sel.Render(line)
	}
	return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(line)
}

// renderThemeName draws the name of the theme whose colors follow.
func (m *Model) renderThemeName(r formRow, selected bool, w int) string {
	if selected {
		return m.st.Sel.Render(fit(r.value+"  "+r.suffix, w))
	}
	name := m.st.Head.Render(r.value)
	if r.dim {
		name = m.st.Dim.Render(r.value)
	}
	return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(name + m.st.Dim.Render("  "+r.suffix))
}

// renderColorRow draws a color as its plain name, its hex field and a swatch.
func (m *Model) renderColorRow(r formRow, selected bool, w int) string {
	label := fit(r.label, colorLabelWidth)
	hex := fit(r.value, 10)
	sw := theme.Swatch(r.value)
	switch {
	case selected:
		return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(m.st.Sel.Render(label+hex) + " " + sw)
	case r.dim:
		return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(m.st.Dim.Render(label+hex) + " " + sw)
	}
	return lipgloss.NewStyle().MaxWidth(max(1, w)).Render(m.st.Dim.Render(label) + hex + " " + sw)
}

func appearanceActions() []action {
	return []action{
		{id: "theme-load", title: "Load theme", group: "Edit pane", hint: "enter", keys: []string{"enter"}, desc: "in Settings, Theme: choose the theme under the cursor and save it on this device",
			write: true, foot: 10, footIn: inDetail, footLabel: "load", when: whenThemeBrowse, run: func(m *Model) tea.Cmd { return m.themeLoad() }},
		{id: "theme-edit", title: "Edit theme", group: "Edit pane", hint: "e", keys: []string{"e"}, desc: "in Settings, Theme: move into the name and colors of a custom theme",
			write: true, foot: 11, footIn: inDetail, footLabel: "edit", when: whenThemeCustom, run: func(m *Model) tea.Cmd { return m.themeEdit() }},
		{id: "theme-add", title: "Add theme", group: "Edit pane", hint: "a", keys: []string{"a"}, desc: "in Settings, Theme: add a theme with the default colors, named New theme",
			write: true, foot: 12, footIn: inDetail, footLabel: "add", when: whenThemeCustom, run: func(m *Model) tea.Cmd { return m.themeAdd() }},
		{id: "theme-copy", title: "Copy theme", group: "Edit pane", hint: "c", keys: []string{"c"}, desc: "in Settings, Theme: copy the theme under the cursor as a custom theme",
			write: true, foot: 13, footIn: inDetail, footLabel: "copy", when: whenThemeBrowse, run: func(m *Model) tea.Cmd { return m.themeCopy() }},
		{id: "theme-delete", title: "Delete theme", group: "Edit pane", hint: "d", keys: []string{"d"}, desc: "in Settings, Theme: delete a custom theme (asks first)",
			write: true, foot: 14, footIn: inDetail, footLabel: "delete", when: whenThemeCustom, run: func(m *Model) tea.Cmd { return m.themeDelete() }},
		{id: "theme-reset", title: "Reset theme colors", group: "Edit pane", hint: "shift+r", keys: []string{"R"}, desc: "in Settings, Theme: put the colors of a custom theme back to the theme it was copied from (asks first)",
			write: true, foot: 16, footIn: inDetail, footLabel: "reset", when: whenThemeReset, run: func(m *Model) tea.Cmd { return m.themeReset() }},
		{id: "theme-mode", title: "Next mode", group: "Edit pane", hint: "tab", keys: []string{"tab"}, desc: "in Settings, Theme: next mode (auto, light, dark), saved on this device",
			write: true, foot: 17, footIn: inDetail, footLabel: "mode", when: whenThemePane, run: func(m *Model) tea.Cmd { return m.themeNextMode() }},
	}
}
