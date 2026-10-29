package tui

import (
	"os"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveKindColor(t *testing.T) {
	th, ok := theme.Builtin("dark", true)
	require.True(t, ok)
	tests := []struct {
		name string
		ov   map[string]string
		kind string
		want string
	}{
		{"no override uses the theme", nil, "person", th.KindColor("person")},
		{"palette name follows the theme", map[string]string{"person": "warn"}, "person", th.Warn},
		{"hex", map[string]string{"deal": "#E8A33D"}, "deal", "#e8a33d"},
		{"invalid override is ignored", map[string]string{"task": "chartreuse"}, "task", th.KindColor("task")},
		{"override of another kind does not leak", map[string]string{"deal": "#112233"}, "task", th.KindColor("task")},
		{"unknown kind has no color", nil, "widget", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveKindColor(th, tt.ov, tt.kind))
		})
	}
}

func TestLoadKindColorsDropsBadEntries(t *testing.T) {
	dir := t.TempDir()
	writeCfg(t, dir, "[colors]\nperson = \"accent\"\ndeal = \"#ABCDEF\"\ntask = \"nope\"\nwidget = \"#fff\"\ntag = 5\n")
	assert.Equal(t, map[string]string{"person": "accent", "deal": "#abcdef"}, loadKindColors(dir))
	assert.Empty(t, loadKindColors(""))
}

func TestColorsSettingsOverrideAndReset(t *testing.T) {
	m := settingsModel(t)
	m.profile = colorprofile.TrueColor
	m.setTheme(m.st.Theme)
	before := m.st.KindStyle("person").Render("x")

	// the Colors section lists every registered kind
	for _, ck := range colorKinds() {
		require.NotNil(t, m.settingByID("color-"+ck.kind), ck.kind)
	}
	assert.Len(t, colorKinds(), 7)

	// pick a palette color from the picker
	cursorToSetting(t, m, "color-person")
	pressRun(m, "enter")
	require.NotNil(t, m.ov)
	typeText(m, "danger")
	press(m, "enter")
	assert.Equal(t, "danger", m.kindColors["person"])
	assert.NotEqual(t, before, m.st.KindStyle("person").Render("x"), "the style changed")
	data, err := os.ReadFile(modules.ConfigFile(m.env.DataDir))
	require.NoError(t, err)
	assert.Contains(t, string(data), "[colors]")
	assert.Contains(t, m.rows[m.cursor].sub, "danger")

	// a custom hex color, with a live preview while typing and validation on enter
	pressRun(m, "enter")
	typeText(m, "Custom")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "zz")
	press(m, "enter")
	assert.True(t, m.statusIsErr, "a bad color is refused")
	assert.Equal(t, "danger", m.kindColors["person"])

	pressRun(m, "enter")
	typeText(m, "Custom")
	press(m, "enter")
	m.input.SetValue("#112233")
	m.previewKindColorHex()
	assert.Equal(t, "#112233", resolveKindColor(m.st.Theme, m.kindColors, "person"), "live preview")
	press(m, "enter")
	assert.False(t, m.statusIsErr, m.status)
	assert.Equal(t, "#112233", m.kindColors["person"])
	assert.Equal(t, "#112233", loadKindColors(m.env.DataDir)["person"])

	// esc on the prompt restores the saved color
	pressRun(m, "enter")
	typeText(m, "Custom")
	press(m, "enter")
	m.input.SetValue("#ffffff")
	m.previewKindColorHex()
	press(m, "esc")
	assert.Equal(t, "#112233", m.kindColors["person"])

	// reset one kind from the picker
	pressRun(m, "enter")
	typeText(m, "Reset")
	press(m, "enter")
	_, has := m.kindColors["person"]
	assert.False(t, has)
	assert.Equal(t, before, m.st.KindStyle("person").Render("x"))
	_, err = os.Stat(modules.ConfigFile(m.env.DataDir))
	require.NoError(t, err)
	assert.Empty(t, loadKindColors(m.env.DataDir))

	// reset all after a yes
	require.NoError(t, m.setKindColor("deal", "mark"))
	require.NoError(t, m.setKindColor("task", "#010203"))
	cursorToSetting(t, m, "colors-reset")
	pressRun(m, "enter")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "y")
	assert.Empty(t, m.kindColors)
	assert.Empty(t, loadKindColors(m.env.DataDir))
}

func TestKindColorsKeepNoColorPrefixes(t *testing.T) {
	m := settingsModel(t)
	m.profile = colorprofile.Ascii
	m.kindColors = map[string]string{"person": "danger"}
	m.setTheme(m.st.Theme)
	assert.Equal(t, "p: Ann", m.st.KindStyle("person").Render("p: Ann"), "no color support leaves the text and its prefix plain")
}

func TestRecordHeadingTakesTheKindColor(t *testing.T) {
	m, _ := newTestModel(t)
	m.profile = colorprofile.TrueColor
	m.kindColors = map[string]string{"person": "#123456"}
	m.setTheme(m.st.Theme)
	assert.Equal(t, "person", m.headingKind())
	assert.Equal(t, m.st.KindStyle("person").Bold(true).Render("Ann"), m.headingStyle().Render("Ann"))
	m.switchView(viewSettings)
	assert.Equal(t, m.st.Name.Render("Ann"), m.headingStyle().Render("Ann"), "a non record screen keeps the accent")
}
