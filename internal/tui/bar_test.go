package tui

import (
	"github.com/jdanielnd/crm-cli/internal/model"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shownKinds lists "kind:label" of the bar rows in order.
func shownKinds(m *Model) []string {
	var out []string
	for _, i := range m.ov.shown {
		it := m.ov.items[i]
		out = append(out, it.kind+":"+it.label)
	}
	return out
}

func kindsOf(m *Model) map[string]int {
	n := map[string]int{}
	for _, i := range m.ov.shown {
		n[m.ov.items[i].kind]++
	}
	return n
}

func TestBarGroupsActionsAndPeople(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "bob")
	k := kindsOf(m)
	assert.Equal(t, 1, k[kindPerson], "Bob Baker is a People hit: %v", shownKinds(m))
	// Groups come in a fixed order: objects, then actions, then settings.
	last := -1
	for _, i := range m.ov.shown {
		r := barRank[m.ov.items[i].kind]
		assert.GreaterOrEqual(t, r, last)
		last = r
	}
	out := plain(m.View())
	assert.Contains(t, out, "People")
	assert.Contains(t, out, "Bob Baker")

	// Picking the person shows them in the People view.
	for i, idx := range m.ov.shown {
		if m.ov.items[idx].kind == kindPerson {
			m.ov.cursor = i
		}
	}
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, int64(2), m.curPerson())
}

func TestBarPrefixScopes(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, ">favorite")
	for _, i := range m.ov.shown {
		assert.Contains(t, []string{kindAction, kindVerb}, m.ov.items[i].kind)
	}
	require.NotEmpty(t, m.ov.shown)
	press(m, "esc")

	press(m, "ctrl+k")
	typeText(m, "@cara")
	assert.Equal(t, map[string]int{kindPerson: 1}, kindsOf(m))
	press(m, "esc")

	press(m, "ctrl+k")
	typeText(m, "#fri")
	assert.Equal(t, map[string]int{kindTag: 1}, kindsOf(m))
	press(m, "enter")
	assert.Equal(t, "#friend", m.query)
	assert.Equal(t, 1, len(m.rows))

	press(m, "ctrl+k")
	typeText(m, "?short")
	require.NotEmpty(t, m.ov.shown)
	assert.Equal(t, map[string]int{kindHelp: len(m.ov.shown)}, kindsOf(m))
	assert.Equal(t, "short-syntax", m.ov.items[m.ov.shown[0]].value)
	press(m, "enter")
	assert.NotNil(t, m.tv, "the help topic opens")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
}

func TestBarRecentAndAliases(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	assert.Zero(t, kindsOf(m)[kindRecent], "nothing recent in a new session")
	var labels []string
	for _, i := range m.ov.shown {
		labels = append(labels, m.ov.items[i].label)
	}
	assert.Contains(t, labels, "Add note (n)")
	assert.Contains(t, labels, "Add task (t)")
	typeText(m, "favor")
	press(m, "enter")

	press(m, "ctrl+k")
	require.NotEmpty(t, m.ov.shown)
	first := m.ov.items[m.ov.shown[0]]
	assert.Equal(t, kindRecent, first.kind)
	assert.Equal(t, "favorite", first.id)
}

func TestBarVerbItemEntersCommandMode(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "snooze")
	var found bool
	for i, idx := range m.ov.shown {
		if m.ov.items[idx].id == "verb:snooze" {
			m.ov.cursor, found = i, true
		}
	}
	require.True(t, found, shownKinds(m))
	press(m, "enter")
	require.NotNil(t, m.ov)
	require.NotNil(t, m.ov.cmd, "the verb and a space open the command line")
	assert.Equal(t, "snooze ", m.ov.filter.Value())
}

func TestBarNoMatchOffersCreatePersonAndNote(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "Qwerty Zxcvb")
	assert.Equal(t, map[string]int{kindCreate: 2}, kindsOf(m))
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, plain(m.View()), "Created Qwerty Zxcvb")
	list, err := pr.Search(m.ctx, "Qwerty", 5)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, list[0].ID, m.curPerson())
	press(m, "ctrl+z")
	list, _ = pr.Search(m.ctx, "Qwerty", 5)
	assert.Empty(t, list, "the contact is undone")

	// The second offer turns the text into a note command.
	press(m, "ctrl+k")
	typeText(m, "Qwerty Zxcvb")
	press(m, "down", "enter")
	require.NotNil(t, m.ov)
	assert.Equal(t, "n Qwerty Zxcvb", m.ov.filter.Value())
	require.NotNil(t, m.ov.cmd)
}

func TestBarReadOnlyBlocksCreate(t *testing.T) {
	m, pr := newTestModel(t)
	m.readOnly = true
	press(m, "ctrl+k")
	typeText(m, "Qwerty Zxcvb")
	press(m, "enter")
	list, _ := pr.Search(m.ctx, "Qwerty", 5)
	assert.Empty(t, list)
}

func TestBarShowsKeysOnTheRight(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "toggle favorite")
	lines := strings.Split(plain(m.View()), "\n")
	var row string
	for _, l := range lines {
		if len(l) > 0 && strings.Contains(l, "Toggle favorite") {
			row = l
		}
	}
	require.NotEmpty(t, row)
	assert.Equal(t, "f", strings.TrimSpace(row[len(row)-3:]), "the key sits at the right edge: %q", row)
}
func settingLabels(m *Model) []string { return barNames(m, kindSetting) }

func TestBarFindsSettingsByTypingWithTheirCurrentValue(t *testing.T) {
	m, _ := newTestModel(t)
	m.themeName = "nord"
	press(m, "/")
	typeText(m, "theme")
	labels := settingLabels(m)
	require.NotEmpty(t, labels, shownKinds(m))
	assert.Equal(t, "Theme: nord", labels[0], "the current theme is shown")
	assert.Contains(t, barNames(m, kindAction), "Theme: choose", "the action is still listed after the setting")
	assert.Contains(t, plain(m.View()), "Settings")
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Theme", m.ov.title, "enter opens the theme chooser")
	press(m, "esc")
}

func TestBarEqualsScopeListsEverySetting(t *testing.T) {
	m, _ := newTestModel(t)
	m.themeName = "nord"
	press(m, "ctrl+k")
	typeText(m, "=")
	assert.Equal(t, map[string]int{kindSetting: len(m.settingList())}, kindsOf(m))
	got := strings.Join(settingLabels(m), "\n")
	for _, want := range []string{"Theme: ", "Keys: defaults", "Short syntax: on", "Read-only: off", "Database: ", "Doctor",
		"Sync sources: 0"} {
		assert.Contains(t, got, want)
	}
}

func TestBarSettingToggleRunsTheAction(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "/")
	typeText(m, "short syntax")
	assert.Equal(t, []string{"Short syntax: on"}, settingLabels(m))
	press(m, "enter")
	assert.True(t, m.noSyntax)
	press(m, "/")
	typeText(m, "=syntax")
	assert.Equal(t, []string{"Short syntax: off"}, settingLabels(m))
	press(m, "esc")
}

func TestBarSettingsFollowReadOnly(t *testing.T) {
	m := toolsModel(t)
	m.readOnly = true
	press(m, "/")
	typeText(m, "=read")
	assert.Equal(t, []string{"Read-only: on"}, settingLabels(m))
	press(m, "esc")

	press(m, "/")
	typeText(m, "=keys: reset")
	require.Equal(t, []string{"Keys: reset"}, settingLabels(m))
	press(m, "enter")
	assert.Contains(t, m.status, "read-only")
	assert.Equal(t, modeNormal, m.mode, "nothing opens")

	// Looking is allowed.
	press(m, "/")
	typeText(m, "=doctor")
	press(m, "enter")
	assert.NotNil(t, m.tv)
}

func TestBarSettingsDoNotCrowdOutPeople(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "/")
	typeText(m, "bob")
	assert.Empty(t, settingLabels(m))
	press(m, "esc")
	press(m, "/")
	typeText(m, "t")
	assert.Empty(t, settingLabels(m), "one letter is not enough")
	press(m, "esc")
}

func TestBarActionScopeListsNoRecords(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "/")
	typeText(m, ">bob")
	for _, i := range m.ov.shown {
		assert.Contains(t, []string{kindAction, kindVerb}, m.ov.items[i].kind)
	}
}

func TestBarOffersToShowAllPeopleWhenMany(t *testing.T) {
	m, pr := newTestModel(t)
	for i := 0; i < 5; i++ {
		_, err := pr.Create(m.ctx, model.CreatePersonInput{FirstName: "Pat", LastName: sp("Smith" + string(rune('a'+i)))})
		require.NoError(t, err)
	}
	require.NoError(t, m.reload())
	press(m, "/")
	typeText(m, "pat")
	assert.Len(t, barNames(m, kindPerson), barHitCap)
	assert.Equal(t, []string{`Show all 5 people matching "pat"`}, barNames(m, kindAll))
	for i, idx := range m.ov.shown {
		if m.ov.items[idx].kind == kindAll {
			m.ov.cursor = i
		}
	}
	press(m, "enter")
	assert.Len(t, m.rows, 5, "the People list is filtered")
	press(m, "esc")
	assert.Len(t, m.rows, 8, "esc clears the filter")
}

func TestOldSearchPromptIsGone(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Nil(t, m.actionByID("search"))
	assert.True(t, m.actionByID("palette").hasKey("/"))
	assert.NotContains(t, m.helpText(), "search people")
}
