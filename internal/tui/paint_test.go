package tui

import (
	"bytes"
	"image/color"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frameCells decodes the frame Bubble Tea would receive into the cell grid of the screen.
func frameCells(v tea.View, w, h int) *uv.Buffer {
	scr := uv.NewScreenBuffer(w, h)
	uv.NewStyledString(v.Content).Draw(scr, scr.Bounds())
	return scr.Buffer
}

func rgbaEqual(a, b color.Color) bool {
	if a == nil || b == nil {
		return false
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// setMode makes a mode and theme the drawn state of a model.
func setMode(t *testing.T, m *Model, md theme.Mode, name string, termDark bool, p colorprofile.Profile) theme.Look {
	t.Helper()
	th, ok := theme.Builtin(name)
	require.True(t, ok)
	m.profile, m.darkBG, m.appearance = p, termDark, md
	look := m.look(th)
	m.setTheme(look)
	return look
}

func TestForcedBackgroundPaintsEveryCellOfEveryView(t *testing.T) {
	tests := []struct {
		name     string
		theme    string
		mode     theme.Mode
		termDark bool
		profile  colorprofile.Profile
		paints   bool
	}{
		{"light on a dark terminal", "default", theme.ModeLight, true, colorprofile.TrueColor, true},
		{"light on a light terminal", "anthropocene", theme.ModeLight, false, colorprofile.TrueColor, true},
		{"light at 16 colors", "anthropocene", theme.ModeLight, true, colorprofile.ANSI, true},
		{"dark on a light terminal", "nord", theme.ModeDark, false, colorprofile.TrueColor, true},
		{"dark on a dark terminal", "anthropocene", theme.ModeDark, true, colorprofile.TrueColor, true},
		{"auto on a dark terminal never paints", "anthropocene", theme.ModeAuto, true, colorprofile.TrueColor, false},
		{"auto on a light terminal never paints", "nord", theme.ModeAuto, false, colorprofile.TrueColor, false},
		{"no color support never paints", "default", theme.ModeLight, true, colorprofile.Ascii, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			look := setMode(t, m, tt.mode, tt.theme, tt.termDark, tt.profile)
			m.Update(tea.WindowSizeMsg{Width: 90, Height: 28})

			wantFg, wantBg := look.DefaultColors()
			for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "?"} {
				press(m, key)
				v := m.View()
				if !tt.paints {
					assert.Nil(t, v.BackgroundColor, "view %s: the terminal background is left alone", key)
					assert.Nil(t, v.ForegroundColor)
					continue
				}
				assert.True(t, rgbaEqual(v.BackgroundColor, wantBg), "view %s sets the terminal default background (OSC 11)", key)
				assert.True(t, rgbaEqual(v.ForegroundColor, wantFg), "view %s sets the terminal default foreground (OSC 10)", key)
				buf := frameCells(v, m.width, m.height)
				unthemed := 0
				for y := 0; y < m.height; y++ {
					for x := 0; x < m.width; x++ {
						if c := buf.CellAt(x, y); c.Width > 0 && c.Style.Bg == nil {
							unthemed++
						}
					}
				}
				assert.Zero(t, unthemed, "view %s: cells without a background", key)
				press(m, "esc")
			}
		})
	}
}

// Changing the mode repaints the screen: light on a dark terminal paints, auto unpaints.
func TestModeChangeRepaintsTheScreen(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG = colorprofile.TrueColor, true
	m.appearance = theme.ModeAuto
	m.resolveTheme()
	assert.Nil(t, m.View().BackgroundColor)
	require.NoError(t, m.setAppearance(theme.ModeLight))
	v := m.View()
	assert.True(t, rgbaEqual(v.BackgroundColor, mustColor(m.st.Look.Bg)))
	require.NoError(t, m.setAppearance(theme.ModeAuto))
	assert.Nil(t, m.View().BackgroundColor)
}

func mustColor(hex string) color.Color {
	_, bg := theme.Palette{Fg: hex, Bg: hex}.DefaultColors()
	return bg
}

// The mode is read again at startup and does not drop the remembered theme.
func TestModeIsReadAtStartup(t *testing.T) {
	m := themeModel(t)
	require.NoError(t, theme.SaveChoice(m.env.DataDir, "nord"))
	require.NoError(t, theme.SaveMode(m.env.DataDir, theme.ModeDark))
	m2, _ := newTestModel(t)
	m2.env = m.env
	m2.applyStartupTheme()
	assert.Equal(t, theme.ModeDark, m2.appearance)
	assert.True(t, m2.appearance.Paints())
	assert.Equal(t, "nord", m2.themeName)
	assert.Equal(t, "dark", m2.st.Look.Variant)

	m.env = Env{}
	require.Error(t, m.setAppearance(theme.ModeLight), "no data directory, nothing to save to")
}

// syncBuffer is an output a program goroutine can write while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *syncBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }

// Runs the model under the real Bubble Tea v2 renderer with a captured output and checks the bytes
// written to the terminal: the default colors are set (OSC 10 and 11) and every painted cell carries
// the background, and the terminal's own colors are restored when the program quits. What a real
// terminal then shows cannot be checked here.
func TestRealRendererWritesThemeColorsAndRestoresThem(t *testing.T) {
	m, _ := newTestModel(t)
	setMode(t, m, theme.ModeLight, "anthropocene", true, colorprofile.TrueColor)

	out := &syncBuffer{}
	p := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(out), tea.WithWindowSize(90, 28), tea.WithColorProfile(colorprofile.TrueColor), tea.WithoutSignals())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	time.Sleep(600 * time.Millisecond)
	p.Quit()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the program did not quit")
	}
	s := out.String()
	assert.Contains(t, s, "\x1b]11;#faf9f5", "OSC 11 sets the terminal background to the theme bg")
	assert.Contains(t, s, "\x1b]10;#141413", "OSC 10 sets the terminal foreground")
	assert.Contains(t, s, "48;2;250;249;245", "cells carry the theme background as a truecolor SGR")
	set, reset := strings.LastIndex(s, "\x1b]11;#faf9f5"), strings.LastIndex(s, "\x1b]111")
	assert.Greater(t, reset, set, "OSC 111 gives the terminal its own background back on exit")
	assert.Contains(t, s[reset:], "\x1b]110", "OSC 110 gives the terminal its own foreground back")
}
