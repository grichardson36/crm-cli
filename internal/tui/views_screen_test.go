package tui

import (
	"os"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDefaultModel is a test model with the arrangement the program ships with.
func newDefaultModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	useDefaultViews(m)
	return m
}

// viewsScreen returns the open Views screen as its module ids in order, with "|" for the divider.
func viewsScreen(m *Model) []string {
	var out []string
	for _, idx := range m.ov.shown {
		it := m.ov.items[idx]
		if it.divider {
			out = append(out, "|")
		} else {
			out = append(out, it.value)
		}
	}
	return out
}

func cursorID(m *Model) string {
	if m.ov == nil || m.ov.cursor < 0 || m.ov.cursor >= len(m.ov.shown) {
		return ""
	}
	return m.ov.items[m.ov.shown[m.ov.cursor]].value
}

// cursorTo moves the Views screen cursor to a module.
func cursorTo(t *testing.T, m *Model, id string) {
	t.Helper()
	for i, idx := range m.ov.shown {
		if m.ov.items[idx].value == id {
			m.ov.cursor = i
			return
		}
	}
	t.Fatalf("no row for %s in %v", id, viewsScreen(m))
}

func TestViewsScreenShowsShownThenHiddenUnderADivider(t *testing.T) {
	m := newDefaultModel(t)
	press(m, "v")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Views", m.ov.title)
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "settings", "|", "archived", "sync"}, viewsScreen(m))
	assert.Equal(t, "home", cursorID(m))
	// The shown views carry their number key, the hidden ones have none.
	for _, idx := range m.ov.shown {
		it := m.ov.items[idx]
		switch it.value {
		case "settings":
			assert.Equal(t, "9", it.hint)
		case "archived", "sync":
			assert.Empty(t, it.hint)
		}
	}
	// Down from the last shown view skips the divider.
	cursorTo(t, m, "settings")
	press(m, "down")
	assert.Equal(t, "archived", cursorID(m))
	press(m, "up")
	assert.Equal(t, "settings", cursorID(m), "up skips the divider too")
	press(m, "esc")
	assert.Nil(t, m.ov)
	assert.Equal(t, modeNormal, m.mode)
}

func TestViewsScreenEnterGoesToTheViewHiddenOnesToo(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want viewKind
	}{
		{"tasks", viewTasks}, {"settings", viewSettings}, {"sync", viewSync}, {"archived", viewArchived},
	} {
		t.Run(tc.id, func(t *testing.T) {
			m := newDefaultModel(t)
			press(m, "v")
			cursorTo(t, m, tc.id)
			press(m, "enter")
			assert.Equal(t, tc.want, m.view)
			assert.Nil(t, m.ov)
		})
	}
	m := newDefaultModel(t)
	press(m, "v", "8")
	assert.Equal(t, viewMerge, m.view, "a number key still picks a shown view")
}

func TestViewsScreenXTogglesShowAndHide(t *testing.T) {
	for _, tc := range []struct {
		name   string
		id     string
		wantOn bool
		after  []string
	}{
		{"hide birthdays", "birthdays", false, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "merge", "settings", "|", "birthdays", "archived", "sync"}},
		{"show archived", "archived", true, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "archived", "settings", "|", "sync"}},
		{"show sync", "sync", true, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "sync", "settings", "|", "archived"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newDefaultModel(t)
			m.env.DataDir = t.TempDir()
			press(m, "v")
			cursorTo(t, m, tc.id)
			press(m, "x")
			assert.Equal(t, tc.wantOn, m.mods.Position(tc.id) > 0)
			assert.Equal(t, tc.after, viewsScreen(m))
			assert.Equal(t, tc.id, cursorID(m), "the cursor follows the view")
			assert.False(t, m.statusIsErr)
			_, err := os.Stat(modules.ConfigFile(m.env.DataDir))
			assert.NoError(t, err, "the arrangement is saved")
		})
	}
}

func TestViewsScreenReorderStaysInsideItsGroup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    string
		key   string
		after []string
	}{
		{"down", "home", "shift+down", []string{"people", "home", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "settings", "|", "archived", "sync"}},
		{"up", "tasks", "shift+up", []string{"home", "people", "tasks", "follow-up", "organizations", "deals", "birthdays", "merge", "settings", "|", "archived", "sync"}},
		{"first up does nothing", "home", "shift+up", []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "settings", "|", "archived", "sync"}},
		{"last shown does not cross the divider", "settings", "shift+down", []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "settings", "|", "archived", "sync"}},
		{"hidden group reorders", "sync", "shift+up", []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "settings", "|", "sync", "archived"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newDefaultModel(t)
			press(m, "v")
			cursorTo(t, m, tc.id)
			press(m, tc.key)
			assert.Equal(t, tc.after, viewsScreen(m))
			assert.Equal(t, tc.id, cursorID(m))
		})
	}
	// The tab order follows.
	m := newDefaultModel(t)
	press(m, "v")
	press(m, "shift+down", "esc")
	assert.Equal(t, 2, m.viewPosition(viewHome))
	assert.Equal(t, 1, m.viewPosition(viewPeople))
}

func TestSettingsAndTheViewsScreenCanNeverBeHidden(t *testing.T) {
	m := newDefaultModel(t)
	press(m, "v")
	cursorTo(t, m, "settings")
	press(m, "x")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "stays on")
	assert.NotZero(t, m.mods.Position("settings"))
	assert.Contains(t, viewsScreen(m), "settings")
	press(m, "esc")

	// The v key is not a module, so no arrangement can remove the screen.
	for _, a := range m.actions {
		if a.id == "goto-view" {
			assert.Equal(t, []string{"v"}, a.keys)
		}
	}
	// A hand-edited config cannot hide Settings either.
	m.env.DataDir = t.TempDir()
	require.NoError(t, os.WriteFile(modules.ConfigFile(m.env.DataDir), []byte("[modules]\nhidden = [\"settings\", \"birthdays\"]\n"), 0o644))
	m.applyStartupModules()
	assert.NotZero(t, m.mods.Position("settings"))
	assert.Zero(t, m.mods.Position("birthdays"))
	assert.True(t, m.statusIsErr, "the problem is reported")
}

func TestModulesScreenAndPaletteEntryAreGone(t *testing.T) {
	m := newDefaultModel(t)
	assert.Nil(t, m.actionByID("modules"), "the separate Modules screen is removed")
	assert.NotNil(t, m.actionByID("modules-reset"), "Modules: reset stays")
	assert.NotContains(t, paletteTitles(m), "Modules")
	assert.Contains(t, paletteTitles(m), "Modules: reset")
	require.NoError(t, m.setModules(mustWith(t, m.mods, "birthdays", false)))
	runID(m, "modules-reset")
	assert.True(t, m.mods.IsDefault())
}

func TestArchivedAndSyncStartHiddenAndExplicitConfigIsRespected(t *testing.T) {
	m := newDefaultModel(t)
	bar := tabBar(m)
	assert.NotContains(t, bar, "Archived")
	assert.NotContains(t, bar, "Sync")
	assert.Equal(t, 9, m.viewPosition(viewSettings), "Settings takes the 9 key")

	// Still reachable from Settings, from the bar and from the Views screen.
	var inBar []string
	press(m, "ctrl+k")
	typeText(m, "archived")
	for _, it := range m.ov.items {
		inBar = append(inBar, it.label)
	}
	for _, idx := range m.ov.shown {
		if it := m.ov.items[idx]; it.kind == kindSetting {
			inBar = append(inBar, "shown:"+it.label)
		}
	}
	assert.Contains(t, inBar, "shown:Open Archived")
	press(m, "esc")
	m.switchView(viewArchived)
	assert.Equal(t, viewArchived, m.view)
	assert.Contains(t, tabBar(m), "v Archived")

	// An arrangement the user already wrote keeps Archived and Sync where it put them.
	m2 := newDefaultModel(t)
	m2.env.DataDir = t.TempDir()
	old := "[modules]\norder = [\"home\", \"people\", \"follow-up\", \"tasks\", \"organizations\", \"deals\", \"birthdays\", \"merge\", \"archived\", \"sync\"]\nhidden = []\n"
	require.NoError(t, os.WriteFile(modules.ConfigFile(m2.env.DataDir), []byte(old), 0o644))
	m2.applyStartupModules()
	assert.Equal(t, 9, m2.viewPosition(viewArchived))
	assert.Equal(t, 10, m2.viewPosition(viewSync))
	assert.Equal(t, 11, m2.viewPosition(viewSettings))
}
