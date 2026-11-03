package theme

import (
	"bytes"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bothPalettes runs f for the dark and the light palette of a theme.
func bothPalettes(th Theme, f func(variant string, p Palette)) {
	f("dark", th.Dark)
	f("light", th.Light)
}

func TestBuiltinsAreSortedValidAndHaveBothPalettes(t *testing.T) {
	want := []string{"anthropocene", "catppuccin", "default", "gruvbox", "high-contrast", "nord", "solarized"}
	assert.Equal(t, want, BuiltinNames())
	for _, name := range want {
		th, ok := Builtin(name)
		require.True(t, ok, name)
		assert.True(t, th.Builtin)
		assert.Equal(t, name, th.Name)
		assert.NoError(t, th.Validate(), name)
		assert.NotEqual(t, th.Dark, th.Light, "%s has two different palettes", name)
	}
	for _, gone := range []string{"auto", "dark", "light", "anthropocene-dark", "anthropocene-light"} {
		_, ok := Builtin(gone)
		assert.False(t, ok, "%s is not a theme any more", gone)
	}
	_, ok := Builtin("NORD")
	assert.True(t, ok, "names match ignoring case")
}

func TestDefaultKeepsTheOldDarkAndLightPalettes(t *testing.T) {
	d, _ := Builtin(DefaultName)
	assert.Equal(t, "#0D1117", d.Dark.Bg)
	assert.Equal(t, "#E6E6E6", d.Dark.Fg)
	assert.Equal(t, "#FFFFFF", d.Light.Bg)
	assert.Equal(t, "#1F2328", d.Light.Fg)
}

func TestResolvePicksThePalette(t *testing.T) {
	n, _ := Builtin("nord")
	d, l := n.Resolve(true), n.Resolve(false)
	assert.Equal(t, "nord", d.Name)
	assert.Equal(t, "dark", d.Variant)
	assert.Equal(t, "light", l.Variant)
	assert.Equal(t, n.Dark.Bg, d.Bg)
	assert.Equal(t, n.Light.Bg, l.Bg)
	assert.NotEqual(t, d.Fg, l.Fg)
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

func TestValidName(t *testing.T) {
	for _, ok := range []string{"x", "New theme", "nord - Copy1", strings.Repeat("a", MaxName)} {
		assert.NoError(t, ValidName(ok), ok)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("a", MaxName+1), "a\nb"} {
		assert.Error(t, ValidName(bad), bad)
	}
}

// customSrc is a complete two-table theme made from a built-in.
func customSrc(name string, mutate func(*Theme)) []byte {
	base, _ := Builtin(DefaultName)
	base.Name, base.Builtin, base.CopiedFrom = name, false, DefaultName
	if mutate != nil {
		mutate(&base)
	}
	return Marshal(base)
}

func TestParseTwoTablesAndMarshalRoundTrip(t *testing.T) {
	src := customSrc("mine", func(th *Theme) { th.Description = "mine"; th.Dark.Accent = "#ff8800"; th.Light.Accent = "5" })
	th, err := Parse("mine", "mine.toml", src)
	require.NoError(t, err)
	assert.Equal(t, "mine", th.Name)
	assert.Equal(t, "mine", th.Description)
	assert.Equal(t, DefaultName, th.CopiedFrom)
	assert.False(t, th.Builtin)
	assert.Equal(t, "#ff8800", th.Dark.Accent)
	assert.Equal(t, "5", th.Light.Accent)
	assert.Equal(t, th.Light.Bg, "#FFFFFF")
	assert.Equal(t, src, Marshal(th), "marshal and parse round trip")
}

func TestParseNameDefaultsToTheFileName(t *testing.T) {
	src := strings.Replace(string(customSrc("x", nil)), "name = \"x\"\n", "", 1)
	th, err := Parse("from-file", "from-file.toml", []byte(src))
	require.NoError(t, err)
	assert.Equal(t, "from-file", th.Name)
}

func TestParseErrorsAreUseful(t *testing.T) {
	full := string(customSrc("x", nil))
	tests := []struct {
		name, file, src string
		want            []string
	}{
		{"bad toml", "x", "[dark.colors\nfg = 1", []string{`theme "x"`, "x.toml", "not valid TOML"}},
		{"unknown role", "x", strings.Replace(full, "[dark.colors]\n", "[dark.colors]\naccnt = \"#fff\"\n", 1), []string{`unknown color role "accnt"`, "dark.colors", `did you mean "accent"`, "roles:"}},
		{"unknown key", "x", "colour = 1\n" + full, []string{"unknown key colour", "allowed:"}},
		{"old single table", "x", "[colors]\nfg = \"#fff\"\n", []string{"unknown key colors", "[dark.colors]"}},
		{"bad color", "x", strings.Replace(full, "fg = \"#E6E6E6\"", "fg = \"blue\"", 1), []string{"dark.colors.fg", `"blue" is not a color`, "#rrggbb"}},
		{"range", "x", strings.Replace(full, "fg = \"#1F2328\"", "fg = \"300\"", 1), []string{"light.colors.fg", "out of range"}},
		{"missing light table", "x", strings.Split(full, "[light.colors]")[0], []string{"light.colors.fg is missing", "light.colors.bg is missing"}},
		{"builtin name", "nord", strings.Replace(full, `name = "x"`, "", 1), []string{"taken by a built-in theme"}},
		{"builtin name in the file", "x", strings.Replace(full, `name = "x"`, `name = "Nord"`, 1), []string{"taken by a built-in theme"}},
		{"bad name", "x", strings.Replace(full, `name = "x"`, `name = ""`, 1) + "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.file, tt.file+".toml", []byte(tt.src))
			if tt.want == nil {
				assert.NoError(t, err, "an empty name falls back to the file name")
				return
			}
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "good.toml"), customSrc("Good one", func(th *Theme) { th.Dark.Accent = "#123456" }), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.toml"), []byte("[dark.colors]\naccent = \"nope\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zdup.toml"), customSrc("good ONE", nil), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignored"), 0o644))

	cat := LoadCatalog(data)
	require.Len(t, cat.Problems, 2)
	var all string
	for _, p := range cat.Problems {
		all += p.File + " " + p.Err + "\n"
	}
	assert.Contains(t, all, "bad.toml")
	assert.Contains(t, all, "dark.colors.accent")
	assert.Contains(t, all, "duplicate theme name")

	g, err := cat.Find("GOOD ONE")
	require.NoError(t, err)
	assert.Equal(t, "#123456", g.Dark.Accent)
	assert.Equal(t, filepath.Join(dir, "good.toml"), g.Path, "files load in name order, the first file with a name wins")
	assert.Len(t, cat.Themes, len(BuiltinNames())+1)

	names := cat.Names()
	for i := 1; i < len(names); i++ {
		assert.LessOrEqual(t, strings.ToLower(names[i-1]), strings.ToLower(names[i]), "sorted")
	}

	_, err = cat.Find("nrod")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `did you mean "nord"`)
	assert.Contains(t, err.Error(), "available:")

	none := LoadCatalog(t.TempDir())
	assert.Empty(t, none.Problems, "a missing directory is fine")
}

func TestExampleParsesAsADefaultCopy(t *testing.T) {
	src := Example("sample")
	th, err := Parse("sample", "sample.toml", src)
	require.NoError(t, err)
	d, _ := Builtin(DefaultName)
	assert.Equal(t, d.Dark, th.Dark)
	assert.Equal(t, d.Light, th.Light)
	assert.Equal(t, DefaultName, th.CopiedFrom)
	assert.Contains(t, string(src), "[dark.colors]")
	assert.Contains(t, string(src), "[light.colors]")
}

func TestNewAndCopyNames(t *testing.T) {
	data := t.TempDir()
	cat := LoadCatalog(data)
	assert.Equal(t, "New theme", cat.NewName())
	assert.Equal(t, "nord - Copy1", cat.CopyName("nord"))

	d, _ := Builtin(DefaultName)
	for _, n := range []string{"New theme", "nord - Copy1"} {
		c := d
		c.Name, c.Builtin = n, false
		_, err := Save(data, c)
		require.NoError(t, err)
	}
	cat = LoadCatalog(data)
	assert.Equal(t, "New theme 2", cat.NewName())
	assert.Equal(t, "nord - Copy2", cat.CopyName("nord"))
	assert.Equal(t, "other - Copy1", cat.CopyName("other"))
}

func TestSaveRemoveAndSlug(t *testing.T) {
	data := t.TempDir()
	d, _ := Builtin(DefaultName)
	a := d
	a.Name, a.Builtin = "My Theme!", false
	saved, err := Save(data, a)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(Dir(data), "my-theme.toml"), saved.Path)
	b := a
	b.Name = "my theme"
	saved2, err := Save(data, b)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(Dir(data), "my-theme-2.toml"), saved2.Path, "a taken file name gets a number")

	saved.Name = "Renamed"
	_, err = Save(data, saved)
	require.NoError(t, err)
	got, err := LoadCatalog(data).Find("renamed")
	require.NoError(t, err)
	assert.Equal(t, saved.Path, got.Path, "a rename rewrites the same file")

	require.NoError(t, Remove(got))
	_, err = LoadCatalog(data).Find("renamed")
	assert.Error(t, err)

	assert.Error(t, Remove(d), "a built-in cannot be deleted")
	_, err = Save(data, d)
	assert.Error(t, err, "a built-in cannot be saved")
	bad := a
	bad.Dark.Accent = "nope"
	_, err = Save(data, bad)
	assert.Error(t, err, "an invalid color is not written")

	assert.Equal(t, "theme", Slug("!!!"))
	assert.Equal(t, "a-b_c", Slug("  A  b_c "))
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
	assert.Equal(t, "default", name)
	assert.Equal(t, "default", src)

	require.NoError(t, SaveChoice(dir, "nord"))
	name, src = Chosen(dir, env(""))
	assert.Equal(t, "nord", name)
	assert.Equal(t, "config", src)

	name, src = Chosen(dir, env("Gruvbox"))
	assert.Equal(t, "Gruvbox", name)
	assert.Equal(t, "env", src)
}

func TestSaveChoiceKeepsOtherSettingsAndRejectsBrokenFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("other = \"keep\"\ntheme = \"nord\"\n"), 0o644))
	require.NoError(t, SaveChoice(dir, "gruvbox"))
	data, err := os.ReadFile(ConfigFile(dir))
	require.NoError(t, err)
	assert.Contains(t, string(data), `other = "keep"`)
	assert.Contains(t, string(data), `theme = "gruvbox"`)

	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("[broken"), 0o644))
	err = SaveChoice(dir, "nord")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid TOML")
	name, _ := Chosen(dir, func(string) string { return "" })
	assert.Equal(t, "default", name, "a broken settings file must not stop the program from choosing a theme")
}

// render draws s the way Bubble Tea writes a frame: the style carries the exact color and the
// writer for the terminal profile downsamples it.
func render(p colorprofile.Profile, s lipgloss.Style, text string) string {
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: p}
	_, _ = w.WriteString(s.Render(text))
	return buf.String()
}

func nordLook() Look {
	th, _ := Builtin("nord")
	return th.Resolve(true)
}

func TestStylesDegradeByProfile(t *testing.T) {
	th := nordLook()

	out := render(colorprofile.TrueColor, NewStyles(th, colorprofile.TrueColor).Name, "x")
	assert.Contains(t, out, "38;2;136;192;208", "truecolor writes the exact color")

	out = render(colorprofile.ANSI256, NewStyles(th, colorprofile.ANSI256).Name, "x")
	assert.Contains(t, out, "38;5;", "256 color terminals get the nearest palette color")
	assert.NotContains(t, out, "38;2;")

	out = render(colorprofile.ANSI, NewStyles(th, colorprofile.ANSI).Name, "x")
	assert.NotContains(t, out, "38;2;")
	assert.NotContains(t, out, "38;5;")
	assert.Contains(t, out, "\x1b[")
}

func TestNoColorKeepsAttributesNotColors(t *testing.T) {
	c, _ := Builtin("catppuccin")
	st := NewStyles(c.Resolve(true), colorprofile.Ascii)
	for name, s := range map[string]lipgloss.Style{"name": st.Name, "dim": st.Dim, "sel": st.Sel, "tab": st.TabOn, "err": st.Err, "mark": st.Mark} {
		out := render(colorprofile.Ascii, s, "x")
		assert.NotContains(t, out, "38;", name)
		assert.NotContains(t, out, "48;", name)
	}
	assert.Contains(t, render(colorprofile.Ascii, st.Sel, "row"), "\x1b[7m", "the selected row stays visible as reverse video")
	assert.Contains(t, render(colorprofile.Ascii, st.TabOn, "tab"), "7m", "the active tab stays visible")
	assert.Contains(t, render(colorprofile.Ascii, st.Name, "x"), "\x1b[1m")
}

func TestSelectionColorsReachTheStyle(t *testing.T) {
	hc, _ := Builtin("high-contrast")
	out := NewStyles(hc.Resolve(true), colorprofile.TrueColor).Sel.Render("x")
	assert.Contains(t, out, "48;2;255;255;255")
	assert.NotContains(t, out, ";7m")
	out = NewStyles(hc.Resolve(false), colorprofile.TrueColor).Sel.Render("x")
	assert.Contains(t, out, "48;2;0;0;0", "the light palette has its own selection")
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

func contrast(t *testing.T, a, b string) float64 {
	la, lb := luminance(t, a), luminance(t, b)
	hi, lo := max(la, lb), min(la, lb)
	return (hi + 0.05) / (lo + 0.05)
}

// textRoles are drawn as text on the screen background.
var textRoles = []string{RoleFg, RoleDim, RoleAccent, RoleDanger, RoleGood, RoleWarn, RoleMark}

// Every palette of every built-in has a background of its own kind, readable text roles and six
// distinct kind colors that stay readable (3:1) on the background.
func TestEveryBuiltinPaletteIsReadableAndDistinct(t *testing.T) {
	for _, th := range Builtins() {
		bothPalettes(th, func(variant string, p Palette) {
			who := th.Name + " " + variant
			require.NoError(t, ValidColor(p.Bg), who)
			isDark := luminance(t, p.Bg) < 0.18
			assert.Equal(t, variant == "dark", isDark, "%s: the background %s is the wrong kind", who, p.Bg)
			for _, role := range textRoles {
				assert.GreaterOrEqual(t, contrast(t, p.Color(role), p.Bg), 3.0, "%s %s on its bg", who, role)
			}
			assert.NotEqual(t, p.SelFg, p.SelBg, who)
			assert.NotEqual(t, p.TabFg, p.TabBg, who)
			seen := map[string]string{}
			for kind := range KindRoles {
				c := p.KindColor(kind)
				require.NotEmpty(t, c, "%s %s", who, kind)
				assert.GreaterOrEqual(t, contrast(t, c, p.Bg), 3.0, "%s %s %s", who, kind, c)
				if prev, dup := seen[c]; dup {
					t.Errorf("%s: %s and %s share %s", who, prev, kind, c)
				}
				seen[c] = kind
			}
		})
	}
}

func TestAnthropocenePalettes(t *testing.T) {
	th, ok := Builtin("anthropocene")
	require.True(t, ok)
	tests := []struct {
		name, bg, fg, accent string
		p                    Palette
	}{
		{"dark", "#141413", "#FAF9F5", "#D97757", th.Dark},
		{"light", "#FAF9F5", "#141413", "#B4532F", th.Light},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p
			assert.Equal(t, tt.bg, p.Bg)
			assert.Equal(t, tt.fg, p.Fg)
			assert.Equal(t, tt.accent, p.Accent)
			for _, role := range textRoles {
				assert.GreaterOrEqual(t, contrast(t, p.Color(role), p.Bg), 4.5, "%s on the background (WCAG AA)", role)
			}
			assert.GreaterOrEqual(t, contrast(t, p.TabFg, p.TabBg), 4.5, "active tab")
			assert.GreaterOrEqual(t, contrast(t, p.SelFg, p.SelBg), 4.5, "selected row")
			assert.GreaterOrEqual(t, contrast(t, p.SelBg, p.Bg), 1.1, "the selected row must be visible against the background")
			for kind := range KindRoles {
				assert.GreaterOrEqual(t, contrast(t, p.KindColor(kind), p.Bg), 4.5, kind)
			}
			// 16 colors: no text role may land on the background's own color.
			at16 := func(c string) color.Color { return colorprofile.ANSI.Convert(lipgloss.Color(c)) }
			for _, role := range textRoles {
				assert.NotEqual(t, at16(p.Bg), at16(p.Color(role)), "%s vanishes at 16 colors", role)
			}
			assert.NotEqual(t, at16(p.SelFg), at16(p.SelBg), "selection at 16 colors")
			assert.NotEqual(t, at16(p.TabFg), at16(p.TabBg), "tab at 16 colors")
		})
	}
}

func TestEditableRolesAreRolesWithPlainNames(t *testing.T) {
	var labels []string
	for _, r := range EditableRoles {
		assert.Contains(t, Roles, r.Role)
		labels = append(labels, r.Label)
	}
	assert.Equal(t, []string{"text", "secondary text", "accent", "error", "success", "warning", "marked", "tab text", "tab background", "selection text", "selection background", "background"}, labels)
}
