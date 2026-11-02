package theme

import (
	"image/color"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func contrast(t *testing.T, a, b string) float64 {
	la, lb := luminance(t, a), luminance(t, b)
	hi, lo := max(la, lb), min(la, lb)
	return (hi + 0.05) / (lo + 0.05)
}

// textRoles are drawn as text on the screen background.
var textRoles = []string{RoleFg, RoleDim, RoleAccent, RoleDanger, RoleGood, RoleWarn, RoleMark}

func TestEveryBuiltinHasABackgroundAndConsistentVariant(t *testing.T) {
	for _, name := range BuiltinNames() {
		for _, termDark := range []bool{true, false} {
			th, ok := Builtin(name, termDark)
			require.True(t, ok)
			require.NotEmpty(t, th.Bg, "%s needs a bg so it can be painted", name)
			require.NoError(t, ValidColor(th.Bg))
			dark, known := th.IsDark()
			require.True(t, known, name)
			assert.Equal(t, luminance(t, th.Bg) < 0.18, dark, "%s: variant %q disagrees with bg %s", name, th.Variant, th.Bg)
			for _, role := range textRoles {
				assert.GreaterOrEqual(t, contrast(t, th.Color(role), th.Bg), 3.0, "%s %s on its bg", name, role)
			}
		}
	}
}

func TestAnthropoceneVariants(t *testing.T) {
	tests := []struct {
		name, bg, fg, accent string
		dark                 bool
	}{
		{"anthropocene-dark", "#141413", "#FAF9F5", "#D97757", true},
		{"anthropocene-light", "#FAF9F5", "#141413", "#B4532F", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The terminal background does not matter for a fixed variant.
			th, ok := Builtin(tt.name, !tt.dark)
			require.True(t, ok)
			require.NoError(t, th.Validate())
			assert.Equal(t, tt.bg, th.Bg)
			assert.Equal(t, tt.fg, th.Fg)
			assert.Equal(t, tt.accent, th.Accent)
			dark, known := th.IsDark()
			assert.True(t, known)
			assert.Equal(t, tt.dark, dark)
			for _, role := range textRoles {
				assert.GreaterOrEqual(t, contrast(t, th.Color(role), th.Bg), 4.5, "%s %s on the background (WCAG AA)", tt.name, role)
			}
			assert.GreaterOrEqual(t, contrast(t, th.TabFg, th.TabBg), 4.5, "active tab")
			assert.GreaterOrEqual(t, contrast(t, th.SelFg, th.SelBg), 4.5, "selected row")
			assert.GreaterOrEqual(t, contrast(t, th.SelBg, th.Bg), 1.1, "the selected row must be visible against the background")
			for kind := range KindRoles {
				assert.GreaterOrEqual(t, contrast(t, th.KindColor(kind), th.Bg), 4.5, kind)
			}
			// 16 colors: after the terminal maps the theme to the basic palette no text role may
			// land on the background's own color, or it would vanish.
			at16 := func(c string) color.Color { return colorprofile.ANSI.Convert(lipgloss.Color(c)) }
			for _, role := range textRoles {
				assert.NotEqual(t, at16(th.Bg), at16(th.Color(role)), "%s %s vanishes at 16 colors", tt.name, role)
			}
			assert.NotEqual(t, at16(th.SelFg), at16(th.SelBg), "selection at 16 colors")
			assert.NotEqual(t, at16(th.TabFg), at16(th.TabBg), "tab at 16 colors")
		})
	}
}

func TestAnthropoceneFollowsTheTerminalAndIsAValidBase(t *testing.T) {
	d, _ := Builtin("anthropocene", true)
	l, _ := Builtin("anthropocene", false)
	assert.Equal(t, "anthropocene", d.Name)
	assert.Equal(t, "#141413", d.Bg)
	assert.Equal(t, "#FAF9F5", l.Bg)
	assert.True(t, d.Follows())
	fixed, _ := Builtin("anthropocene-dark", true)
	assert.False(t, fixed.Follows())

	th, err := Parse("mine", "mine.toml", []byte("base = \"anthropocene-light\"\n[colors]\naccent = \"#aa3300\"\n"), true)
	require.NoError(t, err)
	assert.Equal(t, "#FAF9F5", th.Bg, "bg is inherited from the base")
	assert.Equal(t, "light", th.Variant)
}

func TestParseVariantAndBg(t *testing.T) {
	tests := []struct {
		name, src   string
		wantVariant string
		wantDark    bool
		wantErr     string
	}{
		{"inherits", "base = \"nord\"\n", "dark", true, ""},
		{"declared light", "base = \"nord\"\nvariant = \"light\"\n", "light", false, ""},
		{"new light bg replaces the inherited variant", "base = \"nord\"\n[colors]\nbg = \"#fafafa\"\n", "", false, ""},
		{"new dark bg", "base = \"light\"\n[colors]\nbg = \"#101010\"\n", "", true, ""},
		{"ansi bg with a declared variant", "base = \"light\"\nvariant = \"light\"\n[colors]\nbg = \"15\"\n", "light", false, ""},
		{"bad variant", "base = \"nord\"\nvariant = \"grey\"\n", "", false, `variant "grey" is not valid`},
		{"bad bg", "base = \"nord\"\n[colors]\nbg = \"white\"\n", "", false, "colors.bg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th, err := Parse("x", "x.toml", []byte(tt.src), true)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantVariant, th.Variant)
			dark, known := th.IsDark()
			assert.True(t, known)
			assert.Equal(t, tt.wantDark, dark)
		})
	}
	// A theme without bg stays valid and never paints.
	src := "[colors]\nfg=\"#111\"\ndim=\"#222\"\naccent=\"#333\"\ndanger=\"#444\"\ngood=\"#555\"\nwarn=\"#666\"\nmark=\"#777\"\ntab_fg=\"#888\"\ntab_bg=\"#999\"\n"
	th, err := Parse("nobg", "nobg.toml", []byte(src), true)
	require.NoError(t, err)
	assert.Empty(t, th.Bg)
	assert.False(t, th.Fills(FillOn, true))
}

func TestFillDecision(t *testing.T) {
	get := func(name string, termDark bool) Theme { th, _ := Builtin(name, termDark); return th }
	nobg := Theme{Name: "x", Variant: "light"}
	tests := []struct {
		name     string
		th       Theme
		mode     Fill
		termDark bool
		want     bool
	}{
		{"explicit light on a dark terminal, auto", get("light", true), FillAuto, true, true},
		{"explicit light on a light terminal, auto", get("light", false), FillAuto, false, true},
		{"anthropocene-light, auto", get("anthropocene-light", true), FillAuto, true, true},
		{"dark theme on a dark terminal, auto", get("nord", true), FillAuto, true, false},
		{"dark theme on a light terminal, auto", get("nord", false), FillAuto, false, true},
		{"anthropocene-dark on a dark terminal, auto", get("anthropocene-dark", true), FillAuto, true, false},
		{"the auto theme never needs it", get("auto", false), FillAuto, false, false},
		{"anthropocene follows the terminal", get("anthropocene", false), FillAuto, false, false},
		{"on forces a dark theme", get("nord", true), FillOn, true, true},
		{"on forces a following theme too", get("auto", true), FillOn, true, true},
		{"off wins over a light theme", get("light", true), FillOff, true, false},
		{"no bg never paints", nobg, FillOn, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.th.Fills(tt.mode, tt.termDark))
		})
	}
}

func TestParseFillAndRemember(t *testing.T) {
	for in, want := range map[string]Fill{"": FillAuto, "AUTO": FillAuto, " on ": FillOn, "off": FillOff} {
		got, err := ParseFill(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}
	_, err := ParseFill("maybe")
	require.Error(t, err)

	dir := t.TempDir()
	assert.Equal(t, FillAuto, ChosenFill(dir))
	require.NoError(t, SaveChoice(dir, "nord"))
	require.NoError(t, SaveFill(dir, FillOn))
	assert.Equal(t, FillOn, ChosenFill(dir))
	name, _ := Chosen(dir, func(string) string { return "" })
	assert.Equal(t, "nord", name, "saving the fill choice keeps the theme")
	require.Error(t, SaveFill(dir, Fill("sideways")))
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("fill_background = \"junk\"\n"), 0o644))
	assert.Equal(t, FillAuto, ChosenFill(dir), "an invalid value falls back to auto")
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

func TestPaintGivesEveryCellTheThemeBackground(t *testing.T) {
	th, _ := Builtin("anthropocene-light", true)
	bg := lipgloss.Color(th.Bg)
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
			got := cells(th.Paint(tt.frame, w, h), w, h)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					c := got.CellAt(x, y)
					require.NotNil(t, c)
					if c.Width == 0 {
						continue // second column of a wide character
					}
					require.NotNil(t, c.Style.Fg, "cell %d,%d has no foreground", x, y)
					assert.True(t, sameColor(c.Style.Bg, bg), "cell %d,%d is not on the theme background", x, y)
				}
			}
			// Painting only adds colors: the text is the same.
			assert.Equal(t, uv.TrimSpace(cells(tt.frame, w, h).String()), uv.TrimSpace(got.String()))
		})
	}

	explicit := lipgloss.NewStyle().Foreground(lipgloss.Color("#112233")).Background(lipgloss.Color("#445566"))
	got := cells(th.Paint(explicit.Render("keep")+" rest", w, h), w, h)
	assert.True(t, sameColor(got.CellAt(0, 0).Style.Bg, lipgloss.Color("#445566")), "explicit colors are kept")
	assert.True(t, sameColor(got.CellAt(0, 0).Style.Fg, lipgloss.Color("#112233")))
	assert.True(t, sameColor(got.CellAt(6, 0).Style.Bg, bg))
	assert.True(t, sameColor(got.CellAt(6, 0).Style.Fg, lipgloss.Color(th.Fg)), "unstyled text gets the theme foreground")
	rev := cells(th.Paint(lipgloss.NewStyle().Reverse(true).Render("sel"), w, h), w, h)
	assert.NotZero(t, rev.CellAt(0, 0).Style.Attrs&uv.AttrReverse, "reverse video stays")

	// Without a bg or a size nothing changes.
	assert.Equal(t, "abc", Theme{Fg: "#fff"}.Paint("abc", 10, 3))
	assert.Equal(t, "abc", th.Paint("abc", 0, 0))
	// A frame taller than h keeps its extra lines.
	assert.Equal(t, 5, strings.Count(th.Paint("1\n2\n3\n4\n5\n6", 5, 2), "\n"))
}

func TestDefaultColorsAreTheThemeFgAndBg(t *testing.T) {
	th, _ := Builtin("anthropocene-dark", true)
	fg, bg := th.DefaultColors()
	assert.True(t, sameColor(fg, lipgloss.Color("#FAF9F5")))
	assert.True(t, sameColor(bg, lipgloss.Color("#141413")))
	fg, bg = Theme{}.DefaultColors()
	assert.Nil(t, fg)
	assert.Nil(t, bg)
}
