package theme

import (
	"image/color"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModeDarkAndPaints(t *testing.T) {
	tests := []struct {
		mode      Mode
		termDark  bool
		wantDark  bool
		wantPaint bool
	}{
		{ModeAuto, true, true, false},
		{ModeAuto, false, false, false},
		{ModeLight, true, false, true},
		{ModeLight, false, false, true},
		{ModeDark, true, true, true},
		{ModeDark, false, true, true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.wantDark, tt.mode.Dark(tt.termDark), "%s on dark=%v", tt.mode, tt.termDark)
		assert.Equal(t, tt.wantPaint, tt.mode.Paints(), tt.mode)
	}
}

func TestParseModeAndRemember(t *testing.T) {
	for in, want := range map[string]Mode{"": ModeAuto, "AUTO": ModeAuto, " light ": ModeLight, "dark": ModeDark} {
		got, err := ParseMode(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}
	_, err := ParseMode("maybe")
	require.Error(t, err)

	dir := t.TempDir()
	assert.Equal(t, ModeAuto, ChosenMode(dir))
	require.NoError(t, SaveChoice(dir, "nord"))
	require.NoError(t, SaveMode(dir, ModeDark))
	assert.Equal(t, ModeDark, ChosenMode(dir))
	name, _ := Chosen(dir, func(string) string { return "" })
	assert.Equal(t, "nord", name, "saving the mode keeps the theme")
	require.Error(t, SaveMode(dir, Mode("sideways")))
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("mode = \"junk\"\n"), 0o644))
	assert.Equal(t, ModeAuto, ChosenMode(dir), "an invalid value falls back to auto")
}

// cells decodes a styled frame into a w x h cell grid, the way the terminal screen holds it.
func cells(frame string, w, h int) *uv.Buffer {
	scr := uv.NewScreenBuffer(w, h)
	uv.NewStyledString(frame).Draw(scr, scr.Bounds())
	return scr.Buffer
}

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestPaintGivesEveryCellTheBackground(t *testing.T) {
	th, _ := Builtin("anthropocene")
	pl := th.Light
	bg := lipgloss.Color(pl.Bg)
	const w, h = 24, 6
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	tests := []struct {
		name, frame string
	}{
		{"empty frame", ""},
		{"plain text and short lines", "hello\nworld"},
		{"a style reset in the middle of a line", "ab" + ansi.ResetStyle + "cd\x1b[1mbold\x1b[0m tail"},
		{"colored text without a background", red.Render("red") + " plain"},
		{"reverse video", lipgloss.NewStyle().Reverse(true).Render("selected") + " rest"},
		{"wide characters", "日本語 ok\nemoji 😀 end"},
		{"full width line", strings.Repeat("x", w)},
		{"a line that is too long is cut", strings.Repeat("y", w+10)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cells(pl.Paint(tt.frame, w, h), w, h)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					c := got.CellAt(x, y)
					require.NotNil(t, c)
					if c.Width == 0 {
						continue // second column of a wide character
					}
					require.NotNil(t, c.Style.Fg, "cell %d,%d has no foreground", x, y)
					assert.True(t, sameColor(c.Style.Bg, bg), "cell %d,%d is not on the background", x, y)
				}
			}
			assert.Equal(t, uv.TrimSpace(cells(tt.frame, w, h).String()), uv.TrimSpace(got.String()), "painting only adds colors")
		})
	}

	explicit := lipgloss.NewStyle().Foreground(lipgloss.Color("#112233")).Background(lipgloss.Color("#445566"))
	got := cells(pl.Paint(explicit.Render("keep")+" rest", w, h), w, h)
	assert.True(t, sameColor(got.CellAt(0, 0).Style.Bg, lipgloss.Color("#445566")), "explicit colors are kept")
	assert.True(t, sameColor(got.CellAt(0, 0).Style.Fg, lipgloss.Color("#112233")))
	assert.True(t, sameColor(got.CellAt(6, 0).Style.Bg, bg))
	assert.True(t, sameColor(got.CellAt(6, 0).Style.Fg, lipgloss.Color(pl.Fg)), "unstyled text gets the foreground")
	rev := cells(pl.Paint(lipgloss.NewStyle().Reverse(true).Render("sel"), w, h), w, h)
	assert.NotZero(t, rev.CellAt(0, 0).Style.Attrs&uv.AttrReverse, "reverse video stays")

	assert.Equal(t, "abc", Palette{Fg: "#fff"}.Paint("abc", 10, 3), "no bg, nothing changes")
	assert.Equal(t, "abc", pl.Paint("abc", 0, 0))
	assert.Equal(t, 5, strings.Count(pl.Paint("1\n2\n3\n4\n5\n6", 5, 2), "\n"), "a frame taller than h keeps its extra lines")
}

func TestDefaultColorsAreTheFgAndBg(t *testing.T) {
	th, _ := Builtin("anthropocene")
	fg, bg := th.Dark.DefaultColors()
	assert.True(t, sameColor(fg, lipgloss.Color("#FAF9F5")))
	assert.True(t, sameColor(bg, lipgloss.Color("#141413")))
	fg, bg = Palette{}.DefaultColors()
	assert.Nil(t, fg)
	assert.Nil(t, bg)
}
