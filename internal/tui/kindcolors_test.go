package tui

import (
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveKindColor(t *testing.T) {
	b, ok := theme.Builtin("default")
	require.True(t, ok)
	th := b.Resolve(true)
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

func TestColorsPaneEditsAndResetsAKindColor(t *testing.T) {
	m := settingsModel(t)
	m.profile = colorprofile.TrueColor
	m.setTheme(m.st.Look)
	before := m.st.KindStyle("person").Render("x")

	cursorToSetting(t, m, "colors")
	require.NotNil(t, m.paneSP())
	press(m, "enter")
	require.Equal(t, focusDetail, m.focus)
	out := paneText(m)
	for _, k := range []string{"person", "organization", "deal", "task", "note", "tag"} {
		assert.Contains(t, out, k)
	}
	assert.NotContains(t, out, "view ", "the saved view kind is not listed")

	// one row per kind, in the order of the Views (people, tasks, organizations, deals, tags), note last
	var got []string
	for _, ck := range m.paneColorKinds() {
		got = append(got, ck.kind)
	}
	assert.Equal(t, []string{"person", "task", "organization", "deal", "tag", model.NoteKind}, got)

	// footer: only keys that work on the row. Nothing is custom yet, so no reset.
	h := hintsText(m)
	assert.Contains(t, h, "e edit")
	assert.Contains(t, h, "esc back")
	assert.NotContains(t, h, "shift+r reset")

	// edit in place: a bad value is refused, a hex is saved to [colors]
	press(m, "e")
	assert.Contains(t, hintsText(m), "enter save")
	m.sp.(*colorsPane).in.SetValue("zz")
	press(m, "enter")
	assert.True(t, m.statusIsErr, "a bad color is refused")
	assert.Empty(t, m.kindColors)
	m.sp.(*colorsPane).in.SetValue("#112233")
	press(m, "enter")
	assert.False(t, m.statusIsErr, m.status)
	assert.Equal(t, "#112233", m.kindColors["person"])
	assert.Equal(t, "#112233", loadKindColors(m.env.DataDir)["person"])
	assert.NotEqual(t, before, m.st.KindStyle("person").Render("x"))
	assert.Contains(t, paneText(m), "#112233")
	assert.Contains(t, hintsText(m), "shift+r reset", "reset shows once the row is custom")

	// esc while editing cancels and keeps the saved color
	press(m, "e")
	m.sp.(*colorsPane).in.SetValue("#ffffff")
	press(m, "esc")
	assert.Equal(t, focusDetail, m.focus, "esc closed the input only")
	assert.Equal(t, "#112233", m.kindColors["person"])

	// shift+r resets the row
	press(m, "R")
	_, has := m.kindColors["person"]
	assert.False(t, has)
	assert.Empty(t, loadKindColors(m.env.DataDir))
	assert.Equal(t, before, m.st.KindStyle("person").Render("x"))
	assert.NotContains(t, hintsText(m), "shift+r reset")

	// the other rows move with down, and esc leaves the pane
	press(m, "down")
	assert.Equal(t, 1, m.sp.(*colorsPane).cursor)
	press(m, "esc")
	assert.Equal(t, focusList, m.focus)
}

func TestColorsAreOneSettingsRow(t *testing.T) {
	m := settingsModel(t)
	assert.NotNil(t, m.settingByID("colors"))
	assert.Nil(t, m.settingByID("color-person"), "the per-kind rows and their picker are gone")
	assert.Len(t, colorKinds(), 7)
}

func TestKindColorsKeepNoColorPrefixes(t *testing.T) {
	m := settingsModel(t)
	m.profile = colorprofile.Ascii
	m.kindColors = map[string]string{"person": "danger"}
	m.setTheme(m.st.Look)
	assert.Equal(t, "p: Ann", m.st.KindStyle("person").Render("p: Ann"), "no color support leaves the text and its prefix plain")
}

func TestRecordHeadingTakesTheKindColor(t *testing.T) {
	m, _ := newTestModel(t)
	m.profile = colorprofile.TrueColor
	m.kindColors = map[string]string{"person": "#123456"}
	m.setTheme(m.st.Look)
	assert.Equal(t, "person", m.headingKind())
	assert.Equal(t, m.st.KindStyle("person").Bold(true).Render("Ann"), m.headingStyle().Render("Ann"))
	m.switchView(viewSettings)
	assert.Equal(t, m.st.Name.Render("Ann"), m.headingStyle().Render("Ann"), "a non record screen keeps the accent")
}
