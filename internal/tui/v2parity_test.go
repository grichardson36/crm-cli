package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bubble Tea v2 spells some keys differently from v1. keyName maps every v2 key press to the v1
// vocabulary the registry, the config file and internal/keys use.
func TestKeyNameKeepsTheV1Vocabulary(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{"plain letter", tea.KeyPressMsg{Code: 'a', Text: "a"}, "a"},
		{"capital letter", tea.KeyPressMsg{Code: 'a', ShiftedCode: 'A', Text: "A", Mod: tea.ModShift}, "A"},
		{"punctuation", tea.KeyPressMsg{Code: '?', Text: "?"}, "?"},
		{"space is a blank, not the word", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, " "},
		{"space without text", tea.KeyPressMsg{Code: tea.KeySpace}, " "},
		{"space with alt keeps the word", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModAlt}, "alt+space"},
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}, "enter"},
		{"alt+enter", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}, "alt+enter"},
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}, "esc"},
		{"backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}, "backspace"},
		{"shift+tab", tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, "shift+tab"},
		{"pgdown", tea.KeyPressMsg{Code: tea.KeyPgDown}, "pgdown"},
		{"pgup", tea.KeyPressMsg{Code: tea.KeyPgUp}, "pgup"},
		{"home", tea.KeyPressMsg{Code: tea.KeyHome}, "home"},
		{"end", tea.KeyPressMsg{Code: tea.KeyEnd}, "end"},
		{"f5", tea.KeyPressMsg{Code: tea.KeyF5}, "f5"},
		{"ctrl letter", tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl}, "ctrl+f"},
		{"ctrl+shift letter stays distinct", tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl | tea.ModShift}, "ctrl+shift+f"},
		{"alt letter", tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt}, "alt+d"},
		{"alt letter with text is still alt", tea.KeyPressMsg{Code: 'd', Text: "d", Mod: tea.ModAlt}, "alt+d"},
		{"alt+shift letter is the capital", tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt | tea.ModShift}, "alt+D"},
		{"ctrl+up", tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}, "ctrl+up"},
		{"ctrl+left", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl}, "ctrl+left"},
		{"shift+down", tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}, "shift+down"},
		{"ctrl+shift+up", tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl | tea.ModShift}, "ctrl+shift+up"},
		{"ctrl+shift+down", tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl | tea.ModShift}, "ctrl+shift+down"},
		{"alt before ctrl", tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl | tea.ModAlt}, "alt+ctrl+up"},
		{"caps lock is not a modifier", tea.KeyPressMsg{Code: 'a', Text: "a", Mod: tea.ModCapsLock}, "a"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, keyName(c.msg), c.name)
	}
}

// Every key of every action survives the test helper and keyName unchanged, so a binding that works
// in the registry is a binding the model can receive.
func TestEveryRegisteredKeyRoundTrips(t *testing.T) {
	m, _ := newTestModel(t)
	seen := 0
	for _, a := range m.actions {
		for _, k := range a.keys {
			assert.Equal(t, k, keyName(keyPress(k)), "action %s key %q", a.id, k)
			seen++
		}
	}
	require.Greater(t, seen, 50)
}

func TestViewAsksForAltScreenAndCellMotionMouse(t *testing.T) {
	m, _ := newTestModel(t)
	v := m.View()
	assert.True(t, v.AltScreen, "the program runs on the alternate screen")
	assert.Equal(t, tea.MouseModeCellMotion, v.MouseMode)
	assert.Contains(t, v.Content, "People")
}

// Bracketed paste is its own message in v2. It lands in the text input that has focus.
func TestPasteGoesToTheFocusedInput(t *testing.T) {
	m, _ := newTestModel(t)
	m.startInput("org-add", orgPrompt(), "")
	_ = m.input.Focus()
	m.Update(tea.PasteMsg{Content: "Acme Corp"})
	assert.Equal(t, "Acme Corp", m.input.Value(), "the footer prompt takes a paste")

	m, _ = newTestModel(t)
	press(m, "ctrl+k")
	require.NotNil(t, m.ov)
	m.Update(tea.PasteMsg{Content: "theme"})
	assert.Equal(t, "theme", m.ov.filter.Value(), "the palette filter takes a paste")
	first := m.ov.items[m.ov.shown[0]]
	assert.Contains(t, strings.ToLower(first.label+first.desc+first.id), "theme", "and filters by it")
}

func TestPasteOnTheListChangesNothing(t *testing.T) {
	m, _ := newTestModel(t)
	before := m.cursor
	m.Update(tea.PasteMsg{Content: "jjjj"})
	assert.Equal(t, before, m.cursor, "a paste is not a stream of key presses")
}

// A mouse release or motion is not a click. Only the press and the wheel act, as in v1.
func TestMouseReleaseAndMotionDoNothing(t *testing.T) {
	m, _ := newTestModel(t)
	view := m.view
	x := m.tabLayout()[len(m.tabLayout())-1].x0
	m.Update(tea.MouseReleaseMsg{X: x, Y: 0, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: x, Y: 0, Button: tea.MouseLeft})
	assert.Equal(t, view, m.view)
	m.Update(tea.MouseClickMsg{X: m.tabLayout()[0].x0, Y: 0, Button: tea.MouseLeft})
}

func TestKeyReleaseIsIgnored(t *testing.T) {
	m, _ := newTestModel(t)
	start := m.view
	m.Update(tea.KeyReleaseMsg{Code: ']'})
	assert.Equal(t, start, m.view)
}

func TestWideCharactersFitTheirColumns(t *testing.T) {
	for _, s := range []string{"日本語のテスト", "Zoë Müller", "a😀b", "Ａｂｃ"} {
		for _, w := range []int{1, 2, 5, 8, 12} {
			got := fit(s, w)
			assert.Equal(t, w, ansi.StringWidth(got), "fit(%q, %d) = %q", s, w, got)
		}
	}
	assert.Equal(t, "日本…", fit("日本語のテスト", 5))
	assert.Equal(t, 4, ansi.StringWidth("日本"))
}

func TestColorProfileMessageSwitchesToAttributeStyles(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Contains(t, m.st.Name.Render("x"), "38;", "truecolor by default")
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.Ascii})
	out := m.st.Name.Render("x")
	assert.NotContains(t, out, "38;")
	assert.Contains(t, m.st.Sel.Render("x"), "\x1b[7m", "the selection stays visible without color")
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	assert.Contains(t, m.st.Name.Render("x"), "38;")
}

func TestBackgroundColorMessageResolvesTheAutoTheme(t *testing.T) {
	m := themeModel(t)
	m.darkBG = true
	auto, err := m.themeCatalog().Find(theme.AutoName)
	require.NoError(t, err)
	m.setTheme(auto)
	dark := m.st.Theme.Fg
	m.Update(tea.BackgroundColorMsg{Color: darkColor{}})
	assert.Equal(t, dark, m.st.Theme.Fg)
	m.Update(tea.BackgroundColorMsg{Color: lightColor{}})
	assert.NotEqual(t, dark, m.st.Theme.Fg, "a light terminal gets the light colors")
	assert.False(t, m.darkBG)
}

type lightColor struct{}

func (lightColor) RGBA() (r, g, b, a uint32) { return 0xffff, 0xffff, 0xffff, 0xffff }

type darkColor struct{}

func (darkColor) RGBA() (r, g, b, a uint32) { return 0, 0, 0, 0xffff }
