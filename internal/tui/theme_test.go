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
		require.Equal(t, th.Name, m.themeName)
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

func TestThemeChooseLivePreviewKeepAndRemember(t *testing.T) {
	m := themeModel(t)
	dark, _ := theme.Builtin("dark", true)
	m.setTheme(dark)

	runPalette(m, "theme: choose")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Theme", m.ov.title)
	// Built-ins, the custom theme, and the broken file greyed out with its error.
	var ids []string
	var broken *overlayItem
	for i := range m.ov.items {
		ids = append(ids, m.ov.items[i].id)
		if strings.HasPrefix(m.ov.items[i].id, "problem:") {
			broken = &m.ov.items[i]
		}
	}
	assert.Subset(t, ids, []string{"auto", "dark", "light", "high-contrast", "catppuccin", "nord", "gruvbox", "solarized", "sunset"})
	require.NotNil(t, broken)
	assert.Contains(t, broken.disabled, "colors.accent")
	assert.Equal(t, "dark", m.ov.items[m.ov.shown[m.ov.cursor]].id, "cursor starts on the active theme")

	// Moving previews without saving.
	for m.ov.items[m.ov.shown[m.ov.cursor]].id != "nord" {
		press(m, "down")
	}
	assert.Equal(t, "nord", m.themeName)
	assert.Contains(t, m.screen(), "38;2;136;192;208", "the open list is drawn in the previewed theme")
	_, err := os.Stat(theme.ConfigFile(m.env.DataDir))
	assert.True(t, os.IsNotExist(err), "preview does not save")

	// Enter keeps and remembers it.
	press(m, "enter")
	assert.Nil(t, m.ov)
	assert.Equal(t, "nord", m.themeName)
	name, src := theme.Chosen(m.env.DataDir, func(string) string { return "" })
	assert.Equal(t, "nord", name)
	assert.Equal(t, "config", src)
	assert.Contains(t, m.status, "nord")
}

func TestThemeChooseEscRestoresPreviousTheme(t *testing.T) {
	m := themeModel(t)
	light, _ := theme.Builtin("light", true)
	m.setTheme(light)
	runPalette(m, "theme: choose")
	press(m, "down", "down", "down")
	assert.NotEqual(t, "light", m.themeName, "previewing another theme")
	press(m, "esc")
	assert.Nil(t, m.ov)
	assert.Equal(t, "light", m.themeName)
	assert.Equal(t, "light", m.st.Theme.Name)
	_, err := os.Stat(theme.ConfigFile(m.env.DataDir))
	assert.True(t, os.IsNotExist(err))
}

func TestThemeChooseBrokenFileExplainsAndKeepsTheme(t *testing.T) {
	m := themeModel(t)
	runPalette(m, "theme: choose")
	for !strings.HasPrefix(m.ov.items[m.ov.shown[m.ov.cursor]].id, "problem:") {
		press(m, "down")
	}
	assert.Equal(t, "sunset", m.themeName, "a broken file is never previewed: the last valid theme stays")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "colors.accent")
	assert.Equal(t, "dark", m.themeName)
}

func TestThemeReloadPicksUpEditedFile(t *testing.T) {
	m := themeModel(t)
	runPalette(m, "theme: choose")
	for m.ov.items[m.ov.shown[m.ov.cursor]].id != "sunset" {
		press(m, "down")
	}
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
