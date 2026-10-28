package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func settingsModel(t *testing.T) *Model {
	t.Helper()
	m := newDefaultModel(t)
	m.env.DataDir = t.TempDir()
	press(m, "9")
	require.Equal(t, viewSettings, m.view, "Settings is key 9 in the default arrangement")
	return m
}

func settingRowIDs(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		if r.skip {
			out = append(out, "|"+r.title)
		} else {
			out = append(out, r.setting)
		}
	}
	return out
}

func cursorToSetting(t *testing.T, m *Model, id string) {
	t.Helper()
	for i, r := range m.rows {
		if r.setting == id {
			m.cursor = i
			m.refreshDetail(true)
			return
		}
	}
	t.Fatalf("no setting %s in %v", id, settingRowIDs(m))
}

func TestSettingsViewIsAGroupedListWithASectionDividerEach(t *testing.T) {
	m := settingsModel(t)
	var sections []string
	for _, r := range m.rows {
		if r.skip {
			assert.True(t, r.rule, "a section divider is drawn as a rule")
			sections = append(sections, r.title)
		}
	}
	assert.Equal(t, settingSections, sections)
	assert.False(t, m.rows[m.cursor].skip, "the cursor starts on a setting")
	assert.Equal(t, "Theme", m.rows[m.cursor].title)
	assert.Contains(t, m.screen(), "Appearance")
	assert.Equal(t, "Settings", viewNames[viewSettings])
}

func TestSettingsRowsShowTheScopeTag(t *testing.T) {
	m := settingsModel(t)
	want := map[string]string{
		"theme": "device", "keys": "device", "views": "device", "search-url": "device", "backups": "device",
		"deal-stages": "synced", "deal-roles": "synced", "note-types": "synced", "aliases": "synced", "company-ignore": "synced", "archived": "synced",
		"syntax": "session", "read-only": "session",
	}
	got := map[string]string{}
	for _, r := range m.rows {
		if r.setting != "" {
			got[r.setting] = r.meta
			assert.Contains(t, []string{"device", "synced", "session"}, r.meta, r.setting)
		}
	}
	for id, scope := range want {
		assert.Equal(t, scope, got[id], id)
	}
	assert.Contains(t, m.screen(), "synced")
	assert.Contains(t, m.screen(), "device")
}

func TestTheBarAndTheViewReadTheSameRegistry(t *testing.T) {
	m := settingsModel(t)
	var registry, view, bar []string
	for _, s := range m.settingList() {
		registry = append(registry, s.id)
	}
	for _, r := range m.rows {
		if r.setting != "" {
			view = append(view, r.setting)
		}
	}
	for _, it := range m.settingItems("") {
		bar = append(bar, strings.TrimPrefix(it.id, "setting:"))
	}
	assert.ElementsMatch(t, registry, view, "the Settings view lists the registry")
	assert.ElementsMatch(t, registry, bar, "the = list of the bar lists the registry")
	// The bar shows the same value and the scope as the hint.
	for _, it := range m.settingItems("theme") {
		if it.id == "setting:theme" {
			assert.Contains(t, it.label, m.themeName)
			assert.Equal(t, "device", it.hint)
		}
	}
	// Every setting declares what the view needs.
	seen := map[string]bool{}
	for _, s := range m.settingList() {
		assert.False(t, seen[s.id], "duplicate id %s", s.id)
		seen[s.id] = true
		assert.Contains(t, settingSections, s.section, s.id)
		assert.NotEmpty(t, s.name, s.id)
		assert.NotEmpty(t, s.desc, s.id)
		assert.NotEmpty(t, s.words, s.id)
		assert.Contains(t, []settingScope{scopeDevice, scopeSynced, scopeSession}, s.scope, s.id)
		assert.True(t, s.field != nil || s.run != nil || s.action != "", "%s does something on enter", s.id)
		if s.action != "" {
			assert.NotNil(t, m.actionByID(s.action), "%s opens a real action", s.id)
		}
	}
}

func TestSettingsEnterOpensTheThingBehindARow(t *testing.T) {
	for _, tc := range []struct {
		id   string
		view viewKind
		ov   string
	}{
		{"archived", viewArchived, ""},
		{"sync", viewSync, ""},
		{"views", viewSettings, "Views"},
		{"deal-stages", viewSettings, "Deal stages"},
		{"deal-roles", viewSettings, "Deal roles"},
		{"theme", viewSettings, "Theme"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			m := settingsModel(t)
			cursorToSetting(t, m, tc.id)
			pressRun(m, "enter")
			assert.Equal(t, tc.view, m.view)
			if tc.ov != "" {
				require.NotNil(t, m.ov)
				assert.Contains(t, m.ov.title, tc.ov)
			}
		})
	}
}

func TestSettingsEditInPlaceWithTheFieldEditors(t *testing.T) {
	m := settingsModel(t)

	// A text setting of this device: the web search address, saved in config.toml [open].
	cursorToSetting(t, m, "search-url")
	assert.Contains(t, m.screen(), defaultSearchURL)
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	assert.Equal(t, defaultSearchURL, m.rowInput.Value())
	m.rowInput.SetValue("https://duckduckgo.com/?q={query}")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.False(t, m.statusIsErr, m.status)
	data, err := os.ReadFile(modules.ConfigFile(m.env.DataDir))
	require.NoError(t, err)
	assert.Contains(t, string(data), "search_url")
	assert.Equal(t, "https://duckduckgo.com/?q=hello", searchURLFor(m.env.DataDir, "hello"))
	assert.Contains(t, m.rows[m.cursor].sub, "duckduckgo", "the list shows the new value")

	// d resets it to the default after a yes.
	cursorToSetting(t, m, "search-url")
	if m.focus != focusDetail {
		press(m, "tab")
	}
	goTo(t, m, "Web search")
	press(m, "d", "y")
	data, _ = os.ReadFile(modules.ConfigFile(m.env.DataDir))
	assert.NotContains(t, string(data), "search_url")
	assert.Equal(t, defaultSearchURL, m.settingByID("search-url").valueOf(m))

	// A choice setting: short syntax, edited with the options in the row (session scope).
	cursorToSetting(t, m, "syntax")
	assert.False(t, m.noSyntax)
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	press(m, "down", "enter")
	assert.True(t, m.noSyntax)
	assert.Equal(t, "off", m.rows[m.cursor].sub)
}

func TestSettingsEditIsBlockedInReadOnlyMode(t *testing.T) {
	m := settingsModel(t)
	m.readOnly = true
	cursorToSetting(t, m, "search-url")
	press(m, "enter")
	assert.NotEqual(t, modeEdit, m.mode)
	assert.True(t, m.statusIsErr)
	_, err := os.Stat(modules.ConfigFile(m.env.DataDir))
	assert.True(t, os.IsNotExist(err))
}

func TestBarSettingThatIsOnlyEditedInPlaceGoesToItsRow(t *testing.T) {
	m := newDefaultModel(t)
	m.env.DataDir = t.TempDir()
	press(m, "ctrl+k")
	typeText(m, "=web search")
	press(m, "enter")
	assert.Equal(t, viewSettings, m.view)
	assert.Equal(t, "search-url", m.rows[m.cursor].setting)
}

func TestBackupsListsThePreMigrationCopies(t *testing.T) {
	m := settingsModel(t)
	dir := t.TempDir()
	m.env.DBPath = dir + string(os.PathSeparator) + "personal.db"
	require.NoError(t, os.MkdirAll(dir+string(os.PathSeparator)+"backups", 0o700))
	require.NoError(t, os.WriteFile(dir+string(os.PathSeparator)+"backups"+string(os.PathSeparator)+"personal-pre-v13-20261029-101500.db", []byte("x"), 0o600))
	require.NoError(t, m.rebuild(true))
	cursorToSetting(t, m, "backups")
	assert.Equal(t, "1 copy", m.rows[m.cursor].sub)
	press(m, "enter")
	require.NotNil(t, m.tv)
	body := m.tv.vp.GetContent()
	assert.Contains(t, body, "2026-10-29 10:15:00")
	assert.Contains(t, body, "schema v13")
}
