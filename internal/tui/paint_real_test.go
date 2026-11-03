package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rig runs a model under the real Bubble Tea renderer and returns the bytes written after each step.
type rig struct {
	p    *tea.Program
	out  *syncBuffer
	seen int
	done chan error
}

func startRig(t *testing.T, m *Model) *rig {
	t.Helper()
	r := &rig{out: &syncBuffer{}, done: make(chan error, 1)}
	r.p = tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(r.out), tea.WithWindowSize(100, 30), tea.WithColorProfile(colorprofile.TrueColor), tea.WithoutSignals())
	go func() { _, err := r.p.Run(); r.done <- err }()
	t.Cleanup(func() {
		r.p.Quit()
		select {
		case <-r.done:
		case <-time.After(10 * time.Second):
		}
	})
	return r
}

// settle waits for the renderer to flush and returns what was written since the last call.
func (r *rig) settle() string {
	time.Sleep(300 * time.Millisecond)
	s := r.out.String()
	d := s[r.seen:]
	r.seen = len(s)
	return d
}

func (r *rig) press(keys ...string) string {
	for _, k := range keys {
		r.p.Send(keyPress(k))
	}
	return r.settle()
}

func oscColors(s string) bool {
	return strings.Contains(s, "\x1b]10;") || strings.Contains(s, "\x1b]11;") || strings.Contains(s, "\x1b]110") || strings.Contains(s, "\x1b]111")
}

// auto, then light, dark and auto again: the terminal colors are set only while painted and reset
// when the mode goes back to auto.
func TestRealRendererModeCycleSetsAndResetsDefaultColors(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG = colorprofile.TrueColor, true
	openThemePane(t, m)
	r := startRig(t, m)
	first := r.settle()
	assert.False(t, oscColors(first), "auto sets no color at startup: %q", first)

	light := r.press("tab") // auto -> light
	assert.Contains(t, light, "\x1b]11;#", "light sets the terminal background")
	assert.Contains(t, light, "\x1b]10;#", "light sets the terminal foreground")

	dark := r.press("tab") // light -> dark
	assert.Contains(t, dark, "\x1b]11;#", "dark sets the colors again")
	assert.NotContains(t, dark, "\x1b]111", "no reset between two painted modes")

	back := r.press("tab") // dark -> auto
	assert.Contains(t, back, "\x1b]111", "auto resets the background (OSC 111)")
	assert.Contains(t, back, "\x1b]110", "auto resets the foreground (OSC 110)")
	assert.NotContains(t, back, "\x1b]11;#", "auto sets no background")
	assert.NotContains(t, back, "\x1b]10;#", "auto sets no foreground")

	for _, k := range []string{"down", "up", "down", "esc"} {
		assert.False(t, oscColors(r.press(k)), "auto, key %s", k)
	}
}

// In auto, previewing, choosing, copying and leaving never write an OSC color sequence.
func TestRealRendererThemeChangesInAutoWriteNoOSC(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG, m.appearance = colorprofile.TrueColor, true, theme.ModeAuto
	openThemePane(t, m)
	r := startRig(t, m)
	r.settle()
	for _, k := range []string{"down", "down", "enter", "up", "c", "esc"} {
		s := r.press(k)
		assert.False(t, oscColors(s), "key %s wrote %q", k, s)
	}
}

// Previewing with the cursor and pressing esc restores the chosen look, in every mode.
func TestPreviewThenEscRestoresTheChosenLook(t *testing.T) {
	for _, md := range []theme.Mode{theme.ModeAuto, theme.ModeLight, theme.ModeDark} {
		t.Run(string(md), func(t *testing.T) {
			m := themeModel(t)
			m.profile, m.darkBG, m.appearance = colorprofile.TrueColor, true, md
			m.resolveTheme()
			before := m.View()
			openThemePane(t, m)
			chosen := m.st.Look
			press(m, "down", "down")
			require.NotEmpty(t, m.previewName)
			require.NotEqual(t, chosen, m.st.Look, "a preview is showing")
			pv := m.View()
			press(m, "esc")
			assert.Empty(t, m.previewName)
			assert.Equal(t, chosen, m.st.Look)
			after := m.View()
			if md == theme.ModeAuto {
				assert.Nil(t, pv.BackgroundColor, "a preview in auto sets no color")
				assert.Nil(t, pv.ForegroundColor)
				assert.Nil(t, after.BackgroundColor)
				return
			}
			assert.True(t, rgbaEqual(before.BackgroundColor, after.BackgroundColor), "the terminal background is the chosen one again")
			assert.True(t, rgbaEqual(before.ForegroundColor, after.ForegroundColor))
		})
	}
}

// Going from painted to unpainted leaves no painted cell behind in the frame.
func TestPaintedToUnpaintedLeavesNoPaintedCells(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG, m.appearance = colorprofile.TrueColor, false, theme.ModeAuto
	openThemePane(t, m)
	press(m, "tab") // light
	painted := m.st.Look.Bg
	require.NotEmpty(t, painted)
	press(m, "tab", "tab") // dark, auto
	require.Equal(t, theme.ModeAuto, m.appearance)
	v := m.View()
	assert.Nil(t, v.BackgroundColor)
	buf := frameCells(v, m.width, m.height)
	bg := mustColor(painted)
	left := 0
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x++ {
			if c := buf.CellAt(x, y); c != nil && rgbaEqual(c.Style.Bg, bg) {
				left++
			}
		}
	}
	assert.Zero(t, left, "cells still painted with the old background")
}

func themeNameSuffix(m *Model) string {
	for _, r := range m.form {
		if r.kind == rkThemeName {
			return r.suffix
		}
	}
	return ""
}

// Every piece of drawn state follows the mode and theme at once, after every key of the pane.
func TestThemePaneStateFollowsEveryKey(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG, m.appearance = colorprofile.TrueColor, true, theme.ModeAuto
	m.resolveTheme()
	openThemePane(t, m)
	check := func(step string) {
		t.Helper()
		want := m.themeName
		if m.previewName != "" {
			want = m.previewName
		}
		th, err := m.themeCatalog().Find(want)
		require.NoError(t, err, step)
		look := m.look(th)
		assert.Equal(t, look.Fg, m.st.Look.Fg, step+": styles follow theme and mode")
		assert.Equal(t, look.Accent, m.st.Look.Accent, step)
		assert.Equal(t, string(m.appearance), m.form[0].value, step+": mode line")
		assert.Equal(t, map[bool]string{true: "dark", false: "light"}[m.dark()], themeNameSuffix(m), step+": palette label")
		assert.Contains(t, m.screen(), m.st.Sel.Render(" "+string(m.appearance)+" "), step+": mode drawn with the current styles")
		if c := resolveKindColor(m.st.Look, m.kindColors, "person"); c != "" {
			assert.Equal(t, theme.ForegroundStyle(c).Render("x"), m.st.Kind["person"].Render("x"), step)
		}
	}
	check("start")
	for _, k := range []string{"tab", "down", "tab", "up", "enter", "tab", "c", "tab", "esc"} {
		press(m, k)
		check("after " + k)
	}
}

// A terminal that reports a new background in auto mode redraws the pane, not only the styles.
func TestBackgroundReportRebuildsThePane(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG, m.appearance = colorprofile.TrueColor, true, theme.ModeAuto
	m.resolveTheme()
	openThemePane(t, m)
	require.Equal(t, "dark", themeNameSuffix(m))
	before := m.screen()
	m.Update(tea.BackgroundColorMsg{Color: lightColor{}})
	assert.Equal(t, "light", themeNameSuffix(m), "the palette label follows the terminal")
	assert.NotEqual(t, before, m.screen(), "the detail pane is drawn again with the new styles")
}

// The mode changes while the cursor is in the color rows: the rows are rebuilt for the other palette.
func TestModeChangeWhileEditingAColorRebuildsRows(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG, m.appearance = colorprofile.TrueColor, true, theme.ModeAuto
	m.resolveTheme()
	openThemePane(t, m)
	downToTheme(t, m, "sunset")
	press(m, "e")
	require.True(t, m.themeEditing)
	downToLabel(t, m, "accent")
	darkAccent := m.form[m.fcur].value
	press(m, "tab")
	require.Equal(t, theme.ModeLight, m.appearance)
	assert.Equal(t, "light", themeNameSuffix(m))
	assert.Equal(t, "accent", m.form[m.fcur].label, "the cursor stays on the row")
	assert.NotEqual(t, darkAccent, m.form[m.fcur].value, "the row shows the light palette")
}

// A popup over the dimmed screen is painted like the rest: no cell is left without a background in
// the painted modes, and the terminal color is not set in auto.
func TestPopupOverPaintedScreenHasNoUnpaintedCells(t *testing.T) {
	for _, md := range []theme.Mode{theme.ModeLight, theme.ModeDark, theme.ModeAuto} {
		m := themeModel(t)
		m.profile, m.darkBG, m.appearance = colorprofile.TrueColor, true, md
		m.resolveTheme()
		press(m, "ctrl+k")
		require.NotNil(t, m.ov)
		v := m.View()
		if md == theme.ModeAuto {
			assert.Nil(t, v.BackgroundColor)
			continue
		}
		buf := frameCells(v, m.width, m.height)
		unthemed := 0
		for y := 0; y < m.height; y++ {
			for x := 0; x < m.width; x++ {
				if c := buf.CellAt(x, y); c != nil && c.Width > 0 && c.Style.Bg == nil {
					unthemed++
				}
			}
		}
		assert.Zero(t, unthemed, "mode %s", md)
		assert.NotNil(t, v.BackgroundColor)
	}
}