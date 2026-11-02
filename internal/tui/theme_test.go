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

// themeModel is a test model whose data directory is a fresh temp dir that holds one custom
// theme and one broken theme file.
func themeModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	dir := t.TempDir()
	themes := theme.Dir(dir)
	require.NoError(t, os.MkdirAll(themes, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(themes, "sunset.toml"), []byte("base = \"gruvbox\"\n[colors]\naccent = \"#ff8800\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(themes, "broken.toml"), []byte("base = \"dark\"\n[colors]\naccent = \"nope\"\n"), 0o644))
	m.env = Env{DataDir: dir}
	m.themeName = "dark"
	t.Setenv(theme.EnvName, "")
	return m
}

func TestEveryViewRendersWithEveryTheme(t *testing.T) {
	m, _ := dupeModel(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(theme.Dir(dir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(theme.Dir(dir), "sunset.toml"), []byte("base = \"nord\"\n[colors]\nsel_fg = \"#fff\"\nsel_bg = \"#333\"\n"), 0o644))
	m.env = Env{DataDir: dir}
	cat := m.themeCatalog()
	require.Empty(t, cat.Problems)
	require.GreaterOrEqual(t, len(cat.Themes), 9)

	for _, th := range cat.Themes {
		m.setTheme(th)
		m.themeName = th.Name
		check := func(what string) {
			t.Helper()
			var out string
			require.NotPanics(t, func() { out = m.screen() }, "%s with %s", what, th.Name)
			assert.NotEmpty(t, out, "%s with %s", what, th.Name)
			for _, l := range strings.Split(out, "\n") {
				assert.LessOrEqual(t, lipgloss.Width(l), m.width, "%s with %s", what, th.Name)
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

func TestThemeColorsReachTheScreen(t *testing.T) {
	m := themeModel(t)
	nord, _ := theme.Builtin("nord", true)
	m.setTheme(nord)
	out := m.screen()
	assert.Contains(t, out, "48;2;136;192;208", "active tab uses the theme's tab background")

	gruv, _ := theme.Builtin("gruvbox", true)
	m.setTheme(gruv)
	out = m.screen()
	assert.Contains(t, out, "48;2;254;128;25")
	assert.NotContains(t, out, "48;2;136;192;208")
}

func TestNoColorKeepsSelectionVisible(t *testing.T) {
	m := themeModel(t)
	m.profile = colorprofile.Ascii // what Bubble Tea reports under NO_COLOR or --no-color
	hc, _ := theme.Builtin("high-contrast", true)
	m.setTheme(hc)
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

// openAppearanceFor opens the Appearance pane with the cursor on the mode selector.
func openAppearanceFor(t *testing.T, m *Model) {
	t.Helper()
	runPalette(m, "theme: choose")
	require.Equal(t, viewSettings, m.view)
	require.Equal(t, focusDetail, m.focus)
	require.Equal(t, rkMode, m.form[m.fcur].kind, "the cursor starts on the mode selector")
}

// downToTheme moves the pane cursor to a theme row.
func downToTheme(t *testing.T, m *Model, name string) {
	t.Helper()
	for i := 0; i < len(m.form) && !(m.form[m.fcur].kind == rkTheme && m.form[m.fcur].value == name); i++ {
		press(m, "down")
	}
	require.Equal(t, name, m.form[m.fcur].value)
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

func TestAppearancePreviewChooseAndRemember(t *testing.T) {
	m := themeModel(t)
	dark, _ := theme.Builtin("dark", true)
	m.setTheme(dark)
	openAppearanceFor(t, m)

	// Built-ins, the custom theme, and the broken file with its error.
	var names []string
	var broken string
	for _, r := range m.form {
		if r.kind == rkTheme {
			names = append(names, r.value)
		}
		if r.kind == rkInfo && strings.Contains(r.value, "colors.accent") {
			broken = r.value
		}
	}
	assert.Subset(t, names, []string{"auto", "dark", "light", "high-contrast", "catppuccin", "nord", "gruvbox", "solarized", "sunset"})
	assert.NotEmpty(t, broken)

	// Moving previews without choosing or saving.
	downToTheme(t, m, "nord")
	assert.Equal(t, "nord", m.st.Theme.Name, "the screen shows the previewed theme")
	assert.Equal(t, "dark", m.themeName, "the chosen theme did not change")
	assert.Contains(t, m.screen(), "38;2;136;192;208")
	_, err := os.Stat(theme.ConfigFile(m.env.DataDir))
	assert.True(t, os.IsNotExist(err), "preview does not save")

	// Enter chooses it, and the Settings row reads the same state.
	press(m, "enter")
	assert.Equal(t, "nord", m.themeName)
	assert.Equal(t, "nord", m.st.Theme.Name)
	assert.Equal(t, "nord, auto", settingValue(t, m, appearanceID), "the Settings row follows the choice")
	name, src := theme.Chosen(m.env.DataDir, func(string) string { return "" })
	assert.Equal(t, "nord", name)
	assert.Equal(t, "config", src)
	assert.Contains(t, m.status, "nord")
}

func TestAppearanceEscRestoresTheChosenTheme(t *testing.T) {
	m := themeModel(t)
	light, _ := theme.Builtin("light", true)
	m.setTheme(light)
	m.themeName = "light"
	openAppearanceFor(t, m)
	downToTheme(t, m, "gruvbox")
	require.Equal(t, "gruvbox", m.st.Theme.Name)
	press(m, "esc")
	assert.Equal(t, focusList, m.focus)
	assert.Equal(t, "light", m.themeName)
	assert.Equal(t, "light", m.st.Theme.Name, "leaving the pane ends the preview")
	_, err := os.Stat(theme.ConfigFile(m.env.DataDir))
	assert.True(t, os.IsNotExist(err))
}

func TestAppearanceModeKeysChangeAndSaveTheMode(t *testing.T) {
	m := themeModel(t)
	m.themeName = "auto"
	m.darkBG = true
	openAppearanceFor(t, m)
	require.Equal(t, theme.ModeAuto, m.appearance)

	press(m, "right")
	assert.Equal(t, theme.ModeLight, m.appearance)
	assert.Equal(t, "light", m.st.Theme.Variant, "auto takes its light variant in light mode on a dark terminal")
	assert.Equal(t, theme.FillOn, m.fillMode, "light and dark paint the whole screen")
	assert.Equal(t, theme.ModeLight, theme.ChosenMode(m.env.DataDir))
	assert.Equal(t, "auto, light", settingValue(t, m, appearanceID))

	press(m, "right")
	assert.Equal(t, theme.ModeDark, m.appearance)
	assert.Equal(t, "dark", m.st.Theme.Variant)
	press(m, "right")
	assert.Equal(t, theme.ModeAuto, m.appearance, "the selector wraps")
	assert.Equal(t, theme.FillAuto, m.fillMode)
	press(m, "left")
	assert.Equal(t, theme.ModeDark, m.appearance)
	assert.Equal(t, focusDetail, m.focus, "left changes the mode, it does not leave the pane on this row")
}

// A light theme previewed on a dark terminal paints the screen. Choosing a dark theme afterwards sets
// the terminal background explicitly instead of relying on the terminal to honor a reset.
func TestAppearanceDarkThemeAfterLightRestoresTheTerminalBackground(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG, m.width, m.height = colorprofile.TrueColor, true, 80, 24
	m.termBG = lipgloss.Color("#101010")
	openAppearanceFor(t, m)
	downToTheme(t, m, "light")
	assert.NotNil(t, m.View().BackgroundColor, "a light theme paints on a dark terminal")
	downToTheme(t, m, "nord")
	press(m, "enter")
	v := m.View()
	require.NotNil(t, v.BackgroundColor, "the terminal background is set, not left to a reset")
	assert.True(t, rgbaEqual(v.BackgroundColor, m.termBG) || rgbaEqual(v.BackgroundColor, mustColor("#2E3440")))
}

func TestThemeReloadPicksUpEditedFile(t *testing.T) {
	m := themeModel(t)
	openAppearanceFor(t, m)
	downToTheme(t, m, "sunset")
	press(m, "enter")
	require.Equal(t, "sunset", m.themeName)
	assert.Contains(t, m.screen(), "48;2;254;128;25", "gruvbox tab background through the sunset theme")

	path := filepath.Join(theme.Dir(m.env.DataDir), "sunset.toml")
	require.NoError(t, os.WriteFile(path, []byte("base = \"nord\"\n"), 0o644))
	runPalette(m, "theme: reload")
	assert.Equal(t, "sunset", m.themeName)
	assert.Contains(t, m.status, "theme reloaded")
	assert.Contains(t, m.screen(), "48;2;136;192;208")

	// A file that no longer parses keeps the current look and shows the error.
	require.NoError(t, os.WriteFile(path, []byte("base = \"nrd\"\n"), 0o644))
	runPalette(m, "theme: reload")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "sunset")
	assert.Contains(t, m.screen(), "48;2;136;192;208", "old colors stay")
}

func TestStartupThemeOrderAndFallback(t *testing.T) {
	m := themeModel(t)
	m.applyStartupTheme()
	assert.Equal(t, "auto", m.themeName)

	require.NoError(t, theme.SaveChoice(m.env.DataDir, "sunset"))
	m.applyStartupTheme()
	assert.Equal(t, "sunset", m.themeName)

	t.Setenv(theme.EnvName, "solarized")
	m.applyStartupTheme()
	assert.Equal(t, "solarized", m.themeName, "CRM_THEME beats the saved choice")

	t.Setenv(theme.EnvName, "missing")
	m.applyStartupTheme()
	assert.Equal(t, "auto", m.themeName, "an unknown name falls back to auto")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, `no theme named "missing"`)
}
