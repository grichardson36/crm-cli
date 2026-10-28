package theme

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinsAreValidAndNamed(t *testing.T) {
	want := []string{"auto", "dark", "light", "high-contrast", "catppuccin", "nord", "gruvbox", "solarized"}
	assert.Equal(t, want, BuiltinNames())
	for _, name := range want {
		for _, dark := range []bool{true, false} {
			th, ok := Builtin(name, dark)
			require.True(t, ok, name)
			assert.True(t, th.Builtin)
			assert.Equal(t, name, th.Name)
			assert.NoError(t, th.Validate(), name)
			assert.NoError(t, ValidName(name), name)
		}
	}
	d, _ := Builtin("auto", true)
	l, _ := Builtin("auto", false)
	assert.NotEqual(t, d.Fg, l.Fg, "auto follows the terminal background")
	_, ok := Builtin("nope", true)
	assert.False(t, ok)
}

func TestValidColor(t *testing.T) {
	for _, ok := range []string{"#fff", "#FFAA00", "0", "255", "42"} {
		assert.NoError(t, ValidColor(ok), ok)
	}
	for bad, msg := range map[string]string{"256": "out of range", "-1": "out of range", "red": "not a color", "#12": "not a color", "#gggggg": "not a color", "": "not a color"} {
		err := ValidColor(bad)
		if assert.Error(t, err, bad) {
			assert.Contains(t, err.Error(), msg, bad)
		}
	}
}

func TestParseFullThemeWithoutBase(t *testing.T) {
	src := `description = "mine"
[colors]
fg = "#ffffff"
dim = "8"
accent = "#0af"
danger = "#f00"
good = "#0f0"
warn = "#ff0"
mark = "#f0f"
tab_fg = "#000"
tab_bg = "#fff"
`
	th, err := Parse("mine", "mine.toml", []byte(src), true)
	require.NoError(t, err)
	assert.Equal(t, "mine", th.Name)
	assert.Equal(t, "mine", th.Description)
	assert.False(t, th.Builtin)
	assert.Equal(t, "8", th.Dim)
	assert.Empty(t, th.SelBg, "selection colors are optional")
}

func TestParseBaseInheritsAndOverrides(t *testing.T) {
	th, err := Parse("tweak", "tweak.toml", []byte("base = \"nord\"\n[colors]\naccent = \"#ff8800\"\n"), true)
	require.NoError(t, err)
	nord, _ := Builtin("nord", true)
	assert.Equal(t, "#ff8800", th.Accent)
	assert.Equal(t, nord.Danger, th.Danger)
	assert.Equal(t, nord.SelBg, th.SelBg)
	assert.Equal(t, "nord", th.Base)
	assert.False(t, th.Builtin)
}

func TestParseErrorsAreUseful(t *testing.T) {
	tests := []struct {
		name, file, src string
		want            []string
	}{
		{"bad toml", "x", "[colors\nfg = 1", []string{`theme "x"`, "x.toml", "not valid TOML"}},
		{"unknown role", "x", "base = \"dark\"\n[colors]\naccnt = \"#fff\"\n", []string{`unknown color role "accnt"`, `did you mean "accent"`, "roles:"}},
		{"unknown key", "x", "base = \"dark\"\ncolour = 1\n", []string{"unknown key colour", "allowed:"}},
		{"bad color", "x", "base = \"dark\"\n[colors]\nfg = \"blue\"\n", []string{"colors.fg", `"blue" is not a color`, "#rrggbb"}},
		{"range", "x", "base = \"dark\"\n[colors]\nfg = \"300\"\n", []string{"colors.fg", "out of range"}},
		{"bad base", "x", "base = \"nrd\"\n", []string{`base "nrd" is not a built-in theme`, `did you mean "nord"`}},
		{"missing roles", "x", "[colors]\nfg = \"#fff\"\n", []string{"colors.dim is missing", "colors.tab_bg is missing", `base = "dark"`}},
		{"half selection", "x", "base = \"dark\"\n[colors]\nsel_bg = \"#333\"\n", []string{"sel_fg and colors.sel_bg must be set together"}},
		{"builtin name", "nord", "base = \"dark\"\n", []string{"taken by a built-in theme"}},
		{"auto name", "auto", "base = \"dark\"\n", []string{"taken by a built-in theme"}},
		{"bad name", "My Theme", "base = \"dark\"\n", []string{"invalid", "rename the file"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.file, tt.file+".toml", []byte(tt.src), true)
			require.Error(t, err)
			for _, w := range tt.want {
				assert.Contains(t, err.Error(), w)
			}
		})
	}
}

func TestLoadDirAndCatalog(t *testing.T) {
	data := t.TempDir()
	dir := Dir(data)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "good.toml"), []byte("base = \"dark\"\n[colors]\naccent = \"#123456\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.toml"), []byte("base = \"dark\"\n[colors]\naccent = \"nope\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignored"), 0o644))

	cat := LoadCatalog(data, true)
	require.Len(t, cat.Problems, 1)
	assert.Contains(t, cat.Problems[0].File, "bad.toml")
	assert.Contains(t, cat.Problems[0].Err, "colors.accent")

	g, err := cat.Find("GOOD")
	require.NoError(t, err)
	assert.Equal(t, "#123456", g.Accent)
	assert.Equal(t, filepath.Join(dir, "good.toml"), g.Path)
	assert.Len(t, cat.Themes, len(BuiltinNames())+1)

	_, err = cat.Find("nrod")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `did you mean "nord"`)
	assert.Contains(t, err.Error(), "available:")

	// A missing directory is fine.
	none := LoadCatalog(t.TempDir(), true)
	assert.Empty(t, none.Problems)
}

func TestExampleParsesAndRoundTrips(t *testing.T) {
	src := Example("sample")
	th, err := Parse("sample", "sample.toml", src, true)
	require.NoError(t, err)
	dark, _ := Builtin("dark", true)
	assert.Equal(t, dark.Accent, th.Accent)
	assert.Contains(t, string(src), "crm theme set sample")
	// Every required role is spelled out, so the example also parses with base removed.
	noBase := strings.Replace(string(src), "base = \"dark\"\n", "", 1)
	_, err = Parse("sample", "sample.toml", []byte(noBase), true)
	assert.NoError(t, err)
}

func TestChosenOrderAndSave(t *testing.T) {
	dir := t.TempDir()
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == EnvName {
				return v
			}
			return ""
		}
	}
	name, src := Chosen(dir, env(""))
	assert.Equal(t, "auto", name)
	assert.Equal(t, "default", src)

	require.NoError(t, SaveChoice(dir, "nord"))
	name, src = Chosen(dir, env(""))
	assert.Equal(t, "nord", name)
	assert.Equal(t, "config", src)

	name, src = Chosen(dir, env("Gruvbox"))
	assert.Equal(t, "gruvbox", name)
	assert.Equal(t, "env", src)
}

func TestSaveChoiceKeepsOtherSettingsAndRejectsBrokenFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("other = \"keep\"\ntheme = \"dark\"\n"), 0o644))
	require.NoError(t, SaveChoice(dir, "light"))
	data, err := os.ReadFile(ConfigFile(dir))
	require.NoError(t, err)
	assert.Contains(t, string(data), `other = "keep"`)
	assert.Contains(t, string(data), `theme = "light"`)

	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("[broken"), 0o644))
	err = SaveChoice(dir, "light")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid TOML")
	// A broken settings file must not stop the program from choosing a theme.
	name, _ := Chosen(dir, func(string) string { return "" })
	assert.Equal(t, "auto", name)
}

func rendererFor(p termenv.Profile) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(p)
	return r
}

func TestStylesDegradeByProfile(t *testing.T) {
	th, _ := Builtin("nord", true)

	tc := rendererFor(termenv.TrueColor)
	out := NewStyles(th, tc).Name.Render("x")
	assert.Contains(t, out, "38;2;136;192;208", "truecolor writes the exact color")

	c256 := rendererFor(termenv.ANSI256)
	out = NewStyles(th, c256).Name.Render("x")
	assert.Contains(t, out, "38;5;", "256 color terminals get the nearest palette color")
	assert.NotContains(t, out, "38;2;")

	c16 := rendererFor(termenv.ANSI)
	out = NewStyles(th, c16).Name.Render("x")
	assert.NotContains(t, out, "38;2;")
	assert.NotContains(t, out, "38;5;")
	assert.Contains(t, out, "\x1b[")
}

func TestNoColorKeepsAttributesNotColors(t *testing.T) {
	th, _ := Builtin("catppuccin", true)
	st := NewStyles(th, rendererFor(termenv.Ascii))
	for name, s := range map[string]lipgloss.Style{"name": st.Name, "dim": st.Dim, "sel": st.Sel, "tab": st.TabOn, "err": st.Err, "mark": st.Mark} {
		out := s.Render("x")
		assert.NotContains(t, out, "38;", name)
		assert.NotContains(t, out, "48;", name)
	}
	assert.Contains(t, st.Sel.Render("row"), "\x1b[7m", "the selected row stays visible as reverse video")
	assert.Contains(t, st.TabOn.Render("tab"), "7m", "the active tab stays visible")
}

func TestSelectionColorsOrReverse(t *testing.T) {
	tc := rendererFor(termenv.TrueColor)
	dark, _ := Builtin("dark", true) // no sel colors
	assert.Contains(t, NewStyles(dark, tc).Sel.Render("x"), "\x1b[7m")
	hc, _ := Builtin("high-contrast", true)
	out := NewStyles(hc, tc).Sel.Render("x")
	assert.Contains(t, out, "48;2;255;255;255")
	assert.NotContains(t, out, ";7m")
}

// luminance is the WCAG relative luminance of "#rrggbb".
func luminance(t *testing.T, hex string) float64 {
	t.Helper()
	require.Len(t, hex, 7, hex)
	ch := func(s string) float64 {
		v, err := strconv.ParseUint(s, 16, 8)
		require.NoError(t, err)
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(hex[1:3]) + 0.7152*ch(hex[3:5]) + 0.0722*ch(hex[5:7])
}

// Every built-in has six distinct kind colors that stay readable (contrast of at least 3:1) on a dark
// background, or on a white one for the light theme.
func TestKindColorsAreReadableAndDistinctInEveryBuiltin(t *testing.T) {
	for _, name := range BuiltinNames() {
		th, _ := Builtin(name, name != "light")
		bg := 0.0
		if name == "light" {
			bg = 1.0
		}
		seen := map[string]string{}
		for kind, role := range KindRoles {
			c := th.KindColor(kind)
			require.NotEmpty(t, c, "%s %s", name, role)
			require.NoError(t, ValidColor(c))
			l := luminance(t, c)
			hi, lo := math.Max(l, bg), math.Min(l, bg)
			assert.GreaterOrEqual(t, (hi+0.05)/(lo+0.05), 3.0, "%s %s %s", name, kind, c)
			if prev, dup := seen[c]; dup {
				t.Errorf("%s: %s and %s share %s", name, prev, kind, c)
			}
			seen[c] = kind
		}
	}
}

// A custom theme that sets no kind colors still validates and gets fallbacks from its other roles.
func TestKindColorsFallBackWhenAThemeLeavesThemOut(t *testing.T) {
	th, err := Parse("plain", "plain.toml", []byte("[colors]\nfg=\"#111111\"\ndim=\"#222222\"\naccent=\"#333333\"\ndanger=\"#444444\"\ngood=\"#555555\"\nwarn=\"#666666\"\nmark=\"#777777\"\ntab_fg=\"#888888\"\ntab_bg=\"#999999\"\n"), true)
	require.NoError(t, err)
	assert.Equal(t, "#333333", th.KindColor("person"))
	assert.Equal(t, "#555555", th.KindColor("organization"))
	assert.Equal(t, "", th.KindColor("nonsense"))
	th2, err := Parse("kinds", "kinds.toml", []byte("base = \"nord\"\n[colors]\nkind_tag = \"#abcdef\"\n"), true)
	require.NoError(t, err)
	assert.Equal(t, "#abcdef", th2.KindColor("tag"))
}
