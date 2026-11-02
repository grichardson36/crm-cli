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

func TestForcedBackgroundPaintsEveryCellOfEveryView(t *testing.T) {
	tests := []struct {
		name     string
		theme    string
		fill     theme.Fill
		termDark bool
		profile  colorprofile.Profile
		paints   bool
	}{
		{"light theme on a dark terminal, auto", "light", theme.FillAuto, true, colorprofile.TrueColor, true},
		{"anthropocene-light on a dark terminal, auto", "anthropocene-light", theme.FillAuto, true, colorprofile.TrueColor, true},
		{"anthropocene-light at 16 colors", "anthropocene-light", theme.FillAuto, true, colorprofile.ANSI, true},
		{"dark theme on a dark terminal, auto", "anthropocene-dark", theme.FillAuto, true, colorprofile.TrueColor, false},
		{"dark theme on a light terminal, auto", "anthropocene-dark", theme.FillAuto, false, colorprofile.TrueColor, true},
		{"dark theme forced on", "nord", theme.FillOn, true, colorprofile.TrueColor, true},
		{"light theme forced off", "light", theme.FillOff, true, colorprofile.TrueColor, false},
		{"auto theme follows the terminal", "auto", theme.FillAuto, false, colorprofile.TrueColor, false},
		{"no color support never paints", "light", theme.FillOn, true, colorprofile.Ascii, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.profile, m.darkBG, m.fillMode = tt.profile, tt.termDark, tt.fill
			th, ok := theme.Builtin(tt.theme, tt.termDark)
			require.True(t, ok)
			m.setTheme(th)
			m.Update(tea.WindowSizeMsg{Width: 90, Height: 28})

			wantFg, wantBg := th.DefaultColors()
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

// The picker previews a theme live: moving to a light theme repaints, moving back unpaints.
func TestPickerPreviewRepaintsTheScreen(t *testing.T) {
	m := themeModel(t)
	m.profile, m.darkBG, m.fillMode = colorprofile.TrueColor, true, theme.FillAuto
	dark, _ := theme.Builtin("anthropocene-dark", true)
	m.setTheme(dark)
	assert.Nil(t, m.View().BackgroundColor)
	light, _ := theme.Builtin("anthropocene-light", true)
	m.setTheme(light)
	v := m.View()
	assert.True(t, rgbaEqual(v.BackgroundColor, mustColor(light.Bg)))
	m.setTheme(dark)
	assert.Nil(t, m.View().BackgroundColor)
}

func mustColor(hex string) color.Color {
	t := theme.Theme{Fg: hex, Bg: hex}
	_, bg := t.DefaultColors()
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
	assert.Equal(t, theme.FillOn, m2.fillMode)
	assert.Equal(t, "nord", m2.themeName)

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
	m.profile, m.darkBG, m.fillMode = colorprofile.TrueColor, true, theme.FillAuto
	th, _ := theme.Builtin("anthropocene-light", true)
	m.setTheme(th)

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
