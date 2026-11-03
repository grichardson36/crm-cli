package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTheme saves a custom theme made from a built-in.
func writeTheme(t *testing.T, dir, name, from string, mutate func(*theme.Theme)) theme.Theme {
	t.Helper()
	th, ok := theme.Builtin(from)
	require.True(t, ok)
	th.Name, th.Builtin, th.CopiedFrom, th.Description = name, false, from, ""
	if mutate != nil {
		mutate(&th)
	}
	saved, err := theme.Save(dir, th)
	require.NoError(t, err)
	return saved
}

// themeModel is a test model whose data directory is a fresh temp dir that holds one custom theme
// (sunset, copied from gruvbox) and one broken theme file.
func themeModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	dir := t.TempDir()
	writeTheme(t, dir, "sunset", "gruvbox", func(th *theme.Theme) { th.Dark.Accent = "#FF8800" })
	require.NoError(t, os.WriteFile(filepath.Join(theme.Dir(dir), "broken.toml"), []byte("[dark.colors]\naccent = \"nope\"\n"), 0o644))
	m.env = Env{DataDir: dir}
	m.themeName = theme.DefaultName
	m.darkBG = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Setenv(theme.EnvName, "")
	return m
}

func TestEveryViewRendersWithEveryThemeInBothModes(t *testing.T) {
	m, _ := dupeModel(t)
	dir := t.TempDir()
	writeTheme(t, dir, "sunset", "nord", func(th *theme.Theme) { th.Dark.SelFg, th.Dark.SelBg = "#FFFFFF", "#333333" })
	m.env = Env{DataDir: dir}
	cat := m.themeCatalog()
	require.Empty(t, cat.Problems)
	require.GreaterOrEqual(t, len(cat.Themes), 8)

	for _, th := range cat.Themes {
		for _, md := range []theme.Mode{theme.ModeDark, theme.ModeLight} {
			m.appearance = md
			m.setTheme(m.look(th))
			m.themeName = th.Name
			who := th.Name + " " + string(md)
			check := func(what string) {
				t.Helper()
				var out string
				require.NotPanics(t, func() { out = m.screen() }, "%s with %s", what, who)
				assert.NotEmpty(t, out, "%s with %s", what, who)
				for _, l := range strings.Split(out, "\n") {
					assert.LessOrEqual(t, lipgloss.Width(l), m.width, "%s with %s", what, who)
				}
			}
			for _, size := range [][2]int{{100, 30}, {40, 10}} {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				for _, v := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
					press(m, v)
					check("view " + v)
				}
				press(m, "2")
				press(m, "space")
				check("marked row")
				press(m, "tab")
				check("detail focus")
				press(m, "esc")
				m.setStatus("something failed", true)
				check("error status")
				m.setStatus("saved", false)
				check("ok status")
				m.readOnly = true
				check("read-only")
				m.readOnly = false
				press(m, "ctrl+k")
				check("palette")
				press(m, "esc", "?")
				check("help")
				press(m, "x")
				press(m, "8", "down", "enter")
				if m.cmp != nil {
					check("compare screen")
					press(m, "esc")
				}
				m.showWhere()
				check("text page")
				press(m, "esc")
			}
		}
	}
}

func TestThemeColorsReachTheScreen(t *testing.T) {
	m := themeModel(t)
	nord, _ := theme.Builtin("nord")
	m.setTheme(nord.Resolve(true))
	out := m.screen()
	assert.Contains(t, out, "48;2;136;192;208", "active tab uses the theme's tab background")

	gruv, _ := theme.Builtin("gruvbox")
	m.setTheme(gruv.Resolve(true))
	out = m.screen()
	assert.Contains(t, out, "48;2;254;128;25")
	assert.NotContains(t, out, "48;2;136;192;208")

	m.setTheme(gruv.Resolve(false))
	assert.Contains(t, m.screen(), "48;2;175;58;3", "the light palette has its own tab background")
}

func TestNoColorKeepsSelectionVisible(t *testing.T) {
	m := themeModel(t)
	m.profile = colorprofile.Ascii // what Bubble Tea reports under NO_COLOR or --no-color
	hc, _ := theme.Builtin("high-contrast")
	m.setTheme(hc.Resolve(true))
	m.syncDetail() // the detail pane keeps the styles it was drawn with
	out := m.screen()
	assert.NotContains(t, out, "38;2;")
	assert.NotContains(t, out, "48;2;")
	assert.Contains(t, out, "\x1b[7m", "the selected row and active tab use reverse video")
}

func TestThemePaletteActionsExistAndAreAllowedReadOnly(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		m := themeModel(t)
		m.readOnly = readOnly
		press(m, "ctrl+k")
		var enabled []string
		for _, it := range m.ov.items {
			if it.disabled == "" {
				enabled = append(enabled, it.label)
			}
		}
		assert.Contains(t, enabled, "Theme: choose", "read-only=%v", readOnly)
		assert.Contains(t, enabled, "Theme: reload", "read-only=%v", readOnly)
	}
}

// openThemePane opens the Theme pane with the cursor on the chosen theme.
func openThemePane(t *testing.T, m *Model) {
	t.Helper()
	runPalette(m, "theme: choose")
	require.Equal(t, viewSettings, m.view)
	require.Equal(t, focusDetail, m.focus)
	require.Equal(t, rkMode, m.form[0].kind, "the pane starts with the mode line")
	require.Equal(t, rkTheme, m.form[m.fcur].kind, "the cursor starts on a theme")
	require.Equal(t, m.themeName, m.form[m.fcur].value, "the cursor starts on the chosen theme")
}

// downToTheme moves the pane cursor to a theme row.
func downToTheme(t *testing.T, m *Model, name string) {
	t.Helper()
	target := m.themeRowIndex(name)
	require.GreaterOrEqual(t, target, 0, name)
	for i := 0; i < 40 && m.fcur != target; i++ {
		if m.fcur < target {
			press(m, "down")
		} else {
			press(m, "up")
		}
	}
	require.Equal(t, name, m.form[m.fcur].value)
}

// downToLabel moves the pane cursor to the row with the label.
func downToLabel(t *testing.T, m *Model, label string) {
	t.Helper()
	for i := 0; i < 40 && m.form[m.fcur].label != label; i++ {
		press(m, "down")
	}
	require.Equal(t, label, m.form[m.fcur].label)
}

func themeNames(m *Model) []string {
	var out []string
	for _, r := range m.form {
		if r.kind == rkTheme {
			out = append(out, r.value)
		}
	}
	return out
}

func footHints(m *Model) []string {
	var out []string
	for _, s := range m.footerSegs() {
		out = append(out, s.hint)
	}
	return out
}

func settingValue(t *testing.T, m *Model, id string) string {
	t.Helper()
	for _, r := range m.rows {
		if r.setting == id {
			return r.sub
		}
	}
	t.Fatalf("no Settings row %s", id)
	return ""
}

func formValue(m *Model, label string) string {
	for _, r := range m.form {
		if r.label == label {
			return r.value
		}
	}
	return ""
}

func TestThemePaneListsEachThemeOnceSortedAndMarksTheChosenOne(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)
	assert.Equal(t, []string{"anthropocene", "catppuccin", "default", "gruvbox", "high-contrast", "nord", "solarized", "sunset"}, themeNames(m))
	var marked []string
	var broken string
	for _, r := range m.form {
		if r.kind == rkTheme && r.primary {
			marked = append(marked, r.value)
		}
		if r.kind == rkInfo && strings.Contains(r.value, "dark.colors.") {
			broken = r.value
		}
	}
	assert.Equal(t, []string{"default"}, marked, "> marks the chosen theme")
	assert.NotEmpty(t, broken, "a broken file is listed with its error")
	assert.Equal(t, "Theme", m.formTitle)
	assert.Contains(t, m.screen(), ">default")
}

func TestThemePaneIsEnteredWithEnterAndNotLiveOnTheCursor(t *testing.T) {
	m := themeModel(t)
	m.gotoSetting(appearanceID)
	require.Equal(t, focusList, m.focus)
	assert.Empty(t, m.previewName, "the cursor on the Theme row previews nothing")
	press(m, "enter")
	assert.Equal(t, focusDetail, m.focus, "enter on the Theme row enters the pane")
	assert.Equal(t, rkTheme, m.form[m.fcur].kind)
}

func TestThemePanePreviewsLoadsAndRemembers(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)

	downToTheme(t, m, "nord")
	assert.Equal(t, "nord", m.st.Look.Name, "the screen shows the previewed theme")
	assert.Equal(t, "default", m.themeName, "the chosen theme did not change")
	assert.Contains(t, m.screen(), "38;2;136;192;208")
	_, err := os.Stat(theme.ConfigFile(m.env.DataDir))
	assert.True(t, os.IsNotExist(err), "preview does not save")

	press(m, "enter")
	assert.Equal(t, "nord", m.themeName)
	assert.Equal(t, "nord", m.st.Look.Name)
	assert.Equal(t, "nord, auto", settingValue(t, m, appearanceID), "the Settings row follows the choice")
	name, src := theme.Chosen(m.env.DataDir, func(string) string { return "" })
	assert.Equal(t, "nord", name)
	assert.Equal(t, "config", src)
	var marked []string
	for _, r := range m.form {
		if r.kind == rkTheme && r.primary {
			marked = append(marked, r.value)
		}
	}
	assert.Equal(t, []string{"nord"}, marked, "> moved to the loaded theme")
}

func TestThemePaneEscRestoresTheChosenTheme(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)
	downToTheme(t, m, "gruvbox")
	require.Equal(t, "gruvbox", m.st.Look.Name)
	press(m, "esc")
	assert.Equal(t, focusList, m.focus)
	assert.Equal(t, "default", m.themeName)
	assert.Equal(t, "default", m.st.Look.Name, "leaving the pane ends the preview")
	_, err := os.Stat(theme.ConfigFile(m.env.DataDir))
	assert.True(t, os.IsNotExist(err))
	press(m, "enter")
	assert.Equal(t, "default", m.form[m.fcur].value, "entering again starts on the chosen theme")
}

func TestThemePaneTabTogglesTheModeAndSavesIt(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)
	require.Equal(t, theme.ModeAuto, m.appearance)
	darkFg := m.st.Look.Fg

	press(m, "tab")
	assert.Equal(t, focusDetail, m.focus, "tab stays in the pane")
	assert.Equal(t, theme.ModeLight, m.appearance)
	assert.Equal(t, "light", m.st.Look.Variant)
	assert.NotEqual(t, darkFg, m.st.Look.Fg, "light is the light palette on a dark terminal")
	assert.True(t, m.fillScreen() || m.profile <= colorprofile.Ascii, "light paints the whole screen")
	assert.Equal(t, theme.ModeLight, theme.ChosenMode(m.env.DataDir))
	assert.Equal(t, "default, light", settingValue(t, m, appearanceID))

	press(m, "tab")
	assert.Equal(t, theme.ModeDark, m.appearance)
	assert.Equal(t, "dark", m.st.Look.Variant)
	press(m, "tab")
	assert.Equal(t, theme.ModeAuto, m.appearance, "tab goes around")
	assert.False(t, theme.ModeAuto.Paints())
	assert.Equal(t, rkTheme, m.form[m.fcur].kind)
}

func TestThemePaneModeKeepsThePreviewedTheme(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)
	downToTheme(t, m, "nord")
	press(m, "tab")
	assert.Equal(t, "nord", m.st.Look.Name, "the preview stays after a mode change")
	assert.Equal(t, "default", m.themeName)
}

func TestThemePaneFooterShowsOnlyKeysThatWork(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)

	downToTheme(t, m, "nord")
	h := footHints(m)
	assert.Contains(t, h, "enter")
	assert.Contains(t, h, "c")
	assert.Contains(t, h, "esc")
	for _, bad := range []string{"e", "a", "d", "shift+r", "ctrl+c"} {
		assert.NotContains(t, h, bad, "a built-in theme cannot use %s", bad)
	}

	downToTheme(t, m, "sunset")
	h = footHints(m)
	for _, want := range []string{"enter", "e", "a", "c", "d", "shift+r", "esc"} {
		assert.Contains(t, h, want, "a custom theme copied from another theme uses %s", want)
	}

	// A custom theme that does not remember a source has no reset.
	th, err := m.themeCat.Find("sunset")
	require.NoError(t, err)
	th.CopiedFrom = ""
	_, err = theme.Save(m.env.DataDir, th)
	require.NoError(t, err)
	m.rebuildAppearance()
	assert.NotContains(t, footHints(m), "shift+r")
}

func TestThemePaneEditsTheColorsOfTheModeShown(t *testing.T) {
	m := themeModel(t)
	m.appearance = theme.ModeDark
	m.resolveTheme()
	openThemePane(t, m)
	downToTheme(t, m, "sunset")
	assert.Equal(t, "#FF8800", formValue(m, "accent"), "the dark palette of the theme is listed")

	press(m, "e")
	require.True(t, m.themeEditing, "e moves the cursor into the name and colors")
	assert.Equal(t, rkThemeName, m.form[m.fcur].kind)
	assert.Equal(t, []string{"enter", "esc"}, func() []string {
		var out []string
		for _, h := range footHints(m) {
			if h == "enter" || h == "esc" {
				out = append(out, h)
			}
		}
		return out
	}(), "the footer shows what the color rows can do")

	downToLabel(t, m, "accent")
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	m.rowInput.SetValue("")
	typeText(m, "00ff00")
	press(m, "enter")
	require.Equal(t, modeNormal, m.mode, "status: %s", m.status)
	assert.Equal(t, "#00FF00", formValue(m, "accent"))
	assert.Equal(t, "#00FF00", m.st.Look.Accent, "the screen shows the edit")
	cat := m.themeCatalog()
	got, err := cat.Find("sunset")
	require.NoError(t, err)
	assert.Equal(t, "#00FF00", got.Dark.Accent)
	assert.NotEqual(t, "#00FF00", got.Light.Accent, "only the dark palette changed")

	// A bad color keeps the editor and says what is wrong.
	press(m, "enter")
	m.rowInput.SetValue("blue")
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode)
	assert.True(t, m.statusIsErr)
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)

	// tab switches which palette is edited.
	lightAccent := got.Light.Accent
	press(m, "tab")
	require.Equal(t, theme.ModeAuto, m.appearance)
	press(m, "tab")
	require.Equal(t, theme.ModeLight, m.appearance)
	assert.Equal(t, lightAccent, formValue(m, "accent"), "the light palette is listed after the mode changed")
	assert.True(t, m.themeEditing, "the cursor stays in the colors")

	press(m, "esc")
	assert.False(t, m.themeEditing, "esc goes back to the theme list")
	assert.Equal(t, focusDetail, m.focus)
	assert.Equal(t, "sunset", m.form[m.fcur].value)
	press(m, "esc")
	assert.Equal(t, focusList, m.focus, "esc again closes the pane")
}

func TestThemePaneRenamesInPlace(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)
	downToTheme(t, m, "sunset")
	press(m, "enter") // loads it, so the saved choice must follow the rename
	require.Equal(t, "sunset", m.themeName)
	before, _ := m.themeCat.Find("sunset")

	press(m, "e", "enter")
	require.Equal(t, modeEdit, m.mode)
	m.rowInput.SetValue("")
	typeText(m, "nord")
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode, "a taken name keeps the editor")
	assert.True(t, m.statusIsErr)

	m.rowInput.SetValue("")
	typeText(m, "Dusk Rose")
	press(m, "enter")
	require.Equal(t, modeNormal, m.mode, "status: %s", m.status)
	cat := m.themeCatalog()
	got, err := cat.Find("dusk rose")
	require.NoError(t, err)
	assert.Equal(t, "Dusk Rose", got.Name)
	assert.Equal(t, before.Path, got.Path, "the same file")
	assert.False(t, cat.Has("sunset"))
	assert.Equal(t, "Dusk Rose", m.themeName)
	name, _ := theme.Chosen(m.env.DataDir, func(string) string { return "" })
	assert.Equal(t, "Dusk Rose", name)
	assert.Contains(t, themeNames(m), "Dusk Rose")
}

func TestThemePaneAddCopyDeleteReset(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)

	downToTheme(t, m, "sunset")
	press(m, "a")
	assert.Equal(t, "New theme", m.form[m.fcur].value, "the cursor lands on the new theme")
	added, err := m.themeCatalog().Find("New theme")
	require.NoError(t, err)
	def, _ := theme.Builtin(theme.DefaultName)
	assert.Equal(t, def.Dark, added.Dark, "a new theme starts from the default colors")
	assert.Equal(t, def.Light, added.Light)
	assert.Equal(t, "default", added.CopiedFrom)
	press(m, "a")
	assert.Equal(t, "New theme 2", m.form[m.fcur].value)

	downToTheme(t, m, "nord")
	press(m, "c")
	assert.Equal(t, "nord - Copy1", m.form[m.fcur].value)
	cp, err := m.themeCatalog().Find("nord - Copy1")
	require.NoError(t, err)
	nord, _ := theme.Builtin("nord")
	assert.Equal(t, nord.Dark, cp.Dark)
	assert.Equal(t, nord.Light, cp.Light)
	assert.Equal(t, "nord", cp.CopiedFrom)
	assert.False(t, cp.Builtin)
	press(m, "c")
	assert.Equal(t, "nord - Copy1 - Copy1", m.form[m.fcur].value, "a copy of a copy is named from the copy")
	downToTheme(t, m, "nord")
	press(m, "c")
	assert.Equal(t, "nord - Copy2", m.form[m.fcur].value)

	// reset: the colors go back to the theme it was copied from, after a question.
	downToTheme(t, m, "nord - Copy1")
	press(m, "e")
	downToLabel(t, m, "accent")
	press(m, "enter")
	m.rowInput.SetValue("")
	typeText(m, "123456")
	press(m, "enter")
	require.Equal(t, "#123456", formValue(m, "accent"))
	press(m, "esc")
	press(m, "R")
	require.Equal(t, modeConfirm, m.mode)
	assert.Equal(t, "reset nord - Copy1 to nord?", m.confirm.prompt)
	press(m, "n")
	assert.Equal(t, "#123456", formValue(m, "accent"), "no keeps the colors")
	press(m, "R", "y")
	assert.Equal(t, nord.Dark.Accent, formValue(m, "accent"), "the colors are back")

	// delete asks first, the chosen theme falls back to default.
	downToTheme(t, m, "sunset")
	press(m, "enter")
	require.Equal(t, "sunset", m.themeName)
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "n")
	assert.Contains(t, themeNames(m), "sunset")
	press(m, "d", "y")
	assert.NotContains(t, themeNames(m), "sunset")
	assert.Equal(t, "default", m.themeName, "deleting the chosen theme chooses default")
	name, _ := theme.Chosen(m.env.DataDir, func(string) string { return "" })
	assert.Equal(t, "default", name)
	assert.Equal(t, rkTheme, m.form[m.fcur].kind)
	_, err = os.Stat(filepath.Join(theme.Dir(m.env.DataDir), "sunset.toml"))
	assert.True(t, os.IsNotExist(err))
}

func TestThemePaneKeysDoNothingOnBuiltIns(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)
	downToTheme(t, m, "nord")
	before := themeNames(m)
	press(m, "e", "d", "R", "a")
	assert.Equal(t, modeNormal, m.mode, "d asks nothing on a built-in")
	assert.False(t, m.themeEditing, "e does not edit a built-in")
	assert.Equal(t, before, themeNames(m), "a adds nothing from a built-in")
	for _, r := range m.form {
		if r.kind == rkColor {
			assert.True(t, r.dim, "the colors of a built-in are shown, not editable")
		}
	}
}

func TestThemePaneReadOnlyChangesNothing(t *testing.T) {
	m := themeModel(t)
	openThemePane(t, m)
	m.readOnly = true
	downToTheme(t, m, "nord")
	press(m, "enter", "tab", "c")
	assert.Equal(t, "default", m.themeName)
	assert.Equal(t, theme.ModeAuto, m.appearance)
	assert.NotContains(t, themeNames(m), "nord - Copy1")
}

func TestThemeReloadPicksUpEditedFile(t *testing.T) {
	m := themeModel(t)
	m.appearance = theme.ModeDark
	openThemePane(t, m)
	downToTheme(t, m, "sunset")
	press(m, "enter")
	require.Equal(t, "sunset", m.themeName)
	assert.Contains(t, m.screen(), "48;2;254;128;25", "gruvbox tab background through the sunset theme")

	got, _ := m.themeCat.Find("sunset")
	nord, _ := theme.Builtin("nord")
	got.Dark = nord.Dark
	require.NoError(t, os.WriteFile(got.Path, theme.Marshal(got), 0o644))
	runPalette(m, "theme: reload")
	assert.Equal(t, "sunset", m.themeName)
	assert.Contains(t, m.screen(), "48;2;136;192;208")

	// A file that no longer parses keeps the current look and shows the error.
	require.NoError(t, os.WriteFile(got.Path, []byte("[dark.colors]\nfg = \"x\"\n"), 0o644))
	runPalette(m, "theme: reload")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.screen(), "48;2;136;192;208", "old colors stay")
}

func TestStartupThemeOrderAndFallback(t *testing.T) {
	m := themeModel(t)
	m.applyStartupTheme()
	assert.Equal(t, "default", m.themeName)
	assert.False(t, m.statusIsErr)

	require.NoError(t, theme.SaveChoice(m.env.DataDir, "sunset"))
	m.applyStartupTheme()
	assert.Equal(t, "sunset", m.themeName)

	t.Setenv(theme.EnvName, "solarized")
	m.applyStartupTheme()
	assert.Equal(t, "solarized", m.themeName, "CRM_THEME beats the saved choice")

	for _, old := range []string{"missing", "auto", "dark", "anthropocene-light"} {
		t.Setenv(theme.EnvName, old)
		m.applyStartupTheme()
		assert.Equal(t, "default", m.themeName, "%s is not a theme, so it falls back to default", old)
		assert.True(t, m.statusIsErr)
		assert.Contains(t, m.status, `no theme named "`+old+`"`)
		m.setStatus("", false)
	}
}
