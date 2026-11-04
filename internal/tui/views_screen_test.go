package tui

import (
	"github.com/charmbracelet/x/ansi"
	"os"
	"strings"
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
		if strings.HasPrefix(it.value, savedPrefix) {
			continue // the saved views are tested in savedview_test.go
		}
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
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "dates", "merge", "settings", "|", "tags", "archived", "sync"}, viewsScreen(m))
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
	assert.Equal(t, "tags", cursorID(m))
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

func TestViewsListShiftAcrossTheLineHidesAndShows(t *testing.T) {
	m := newDefaultModel(t)
	m.env.DataDir = t.TempDir()
	press(m, "v")
	// Merge swaps with Settings, then moves past the last shown view and is hidden.
	cursorTo(t, m, "merge")
	press(m, "shift+down")
	assert.Equal(t, "merge", cursorID(m))
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "dates", "settings", "merge", "|", "tags", "archived", "sync"}, viewsScreen(m), "swapped with Settings")
	press(m, "shift+down")
	assert.Zero(t, m.mods.Position("merge"), "moving past the last shown view crosses the line and hides it")
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "dates", "settings", "|", "merge", "tags", "archived", "sync"}, viewsScreen(m), "it lands first under the line")
	assert.Equal(t, "merge", cursorID(m), "the cursor follows the view")
	_, err := os.Stat(modules.ConfigFile(m.env.DataDir))
	assert.NoError(t, err, "the arrangement is saved")

	press(m, "shift+up")
	assert.Equal(t, 9, m.mods.Position("merge"), "moving up over the line shows it as the last shown view")
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "dates", "settings", "merge", "|", "tags", "archived", "sync"}, viewsScreen(m))

	// A hidden view moves inside the hidden group, and away from the line at the edge does nothing.
	cursorTo(t, m, "tags")
	press(m, "shift+down")
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "dates", "settings", "merge", "|", "archived", "tags", "sync"}, viewsScreen(m))
	press(m, "shift+up", "shift+up")
	assert.Equal(t, 10, m.mods.Position("tags"), "the top hidden view crosses up and shows")
	cursorTo(t, m, "sync")
	press(m, "shift+down")
	assert.Zero(t, m.mods.Position("sync"), "the bottom hidden view stays")
	// The number key follows the position among the shown views.
	press(m, "esc")
	assert.Equal(t, 10, m.viewPosition(viewTags))
}

func TestViewsListHasNoHideKey(t *testing.T) {
	m := newDefaultModel(t)
	press(m, "v")
	cursorTo(t, m, "dates")
	press(m, "x")
	assert.Equal(t, "x", m.ov.filter.Value(), "x is typed into the filter now")
	assert.NotZero(t, m.mods.Position("dates"))
	for _, h := range m.viewsHints(m.ov, false) {
		assert.NotEqual(t, "x", h.key)
	}
}

func TestViewsListFilterNarrowsAndEnterOpens(t *testing.T) {
	m := newDefaultModel(t)
	press(m, "v")
	typeText(m, "arch")
	assert.Equal(t, []string{"archived"}, viewsScreen(m), "the hidden view that matches is listed, the line is not")
	assert.Equal(t, "archived", cursorID(m))
	press(m, "enter")
	assert.Equal(t, viewArchived, m.view)

	m = newDefaultModel(t)
	press(m, "v")
	typeText(m, "tas")
	assert.Equal(t, "tasks", cursorID(m))
	assert.Contains(t, ansi.Strip(m.screen()), "> tas")
	press(m, "enter")
	assert.Equal(t, viewTasks, m.view)
	// Digits type into a filter, they only pick while it is empty.
	m = newDefaultModel(t)
	press(m, "v", "t", "1")
	assert.Equal(t, "t1", m.ov.filter.Value())
}

func TestViewsListShowsHiddenRowsGrayBelowTheLineAndTheKeysOfTheRow(t *testing.T) {
	m := newDefaultModel(t)
	press(m, "v")
	out := ansi.Strip(m.screen())
	assert.Contains(t, out, strings.Repeat("─", 20), "a horizontal line")
	hidden := 0
	for _, idx := range m.ov.shown {
		if it := m.ov.items[idx]; it.dim && !strings.HasPrefix(it.value, savedPrefix) {
			hidden++
			assert.Empty(t, it.hint, "a hidden view has no number key")
		}
	}
	assert.Equal(t, 3, hidden)
	for _, want := range []string{"↑/↓ select", "enter open", "shift+↑/↓ move", "shift+r reset", "esc close"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "hide")
	assert.NotContains(t, out, "x show")
	typeText(m, "t")
	assert.NotContains(t, ansi.Strip(m.screen()), "shift+↑/↓ move", "a move needs the whole list")
}

func TestViewsListResetPutsTheDefaultBack(t *testing.T) {
	m := newDefaultModel(t)
	m.env.DataDir = t.TempDir()
	require.NoError(t, m.setModules(mustWith(t, m.mods, "dates", false)))
	press(m, "v")
	cursorTo(t, m, "home")
	press(m, "shift+down", "R")
	assert.True(t, m.mods.IsDefault())
	assert.Equal(t, "home", cursorID(m))
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "tags", "dates", "merge", "settings", "|", "archived", "sync"}, viewsScreen(m), "the shipped order and hidden set")
}

func TestSettingsAndTheViewsScreenCanNeverBeHidden(t *testing.T) {
	m := newDefaultModel(t)
	press(m, "v")
	cursorTo(t, m, "settings")
	press(m, "shift+down")
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
	require.NoError(t, os.WriteFile(modules.ConfigFile(m.env.DataDir), []byte("[modules]\nhidden = [\"settings\", \"dates\"]\n"), 0o644))
	m.applyStartupModules()
	assert.NotZero(t, m.mods.Position("settings"))
	assert.Zero(t, m.mods.Position("dates"))
	assert.True(t, m.statusIsErr, "the problem is reported")
}

func TestModulesScreenAndPaletteEntryAreGone(t *testing.T) {
	m := newDefaultModel(t)
	assert.Nil(t, m.actionByID("modules"), "the separate Modules screen is removed")
	assert.NotNil(t, m.actionByID("modules-reset"), "Modules: reset stays")
	assert.NotContains(t, paletteTitles(m), "Modules")
	assert.Contains(t, paletteTitles(m), "Modules: reset")
	require.NoError(t, m.setModules(mustWith(t, m.mods, "dates", false)))
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
	old := "[modules]\norder = [\"home\", \"people\", \"follow-up\", \"tasks\", \"organizations\", \"deals\", \"dates\", \"merge\", \"archived\", \"sync\"]\nhidden = []\n"
	require.NoError(t, os.WriteFile(modules.ConfigFile(m2.env.DataDir), []byte(old), 0o644))
	m2.applyStartupModules()
	assert.Equal(t, 9, m2.viewPosition(viewArchived))
	assert.Equal(t, 10, m2.viewPosition(viewSync))
	assert.Equal(t, 11, m2.viewPosition(viewTags), "a module the config does not list is appended")
	assert.Equal(t, 12, m2.viewPosition(viewSettings))
}

func TestSettingsViewsPaneKeysAndFooter(t *testing.T) {
	m := settingsModel(t)
	cursorToSetting(t, m, "views")
	require.NotNil(t, m.paneSP(), "the pane shows the list as soon as the row is under the cursor")
	press(m, "enter")
	require.Equal(t, focusDetail, m.focus)
	assert.Nil(t, m.ov, "no popup")
	out := paneText(m)
	assert.NotContains(t, out, "> Views", "the pane has no filter line")
	assert.Contains(t, out, strings.Repeat("─", 10), "the line between shown and hidden views")

	// bare letters are not a filter here
	press(m, "t")
	assert.Equal(t, "", m.sp.(*viewsPane).o.filter.Value())

	// footer: keys of the highlighted built-in view only
	h := hintsText(m)
	for _, want := range []string{"↑/↓ select", "enter open", "shift+↑/↓ move", "shift+r reset", "esc back"} {
		assert.Contains(t, h, want)
	}
	for _, bad := range []string{"e edit", "d delete", "r restore", "hide", "x "} {
		assert.NotContains(t, h, bad)
	}

	// shift+down on Merge: past the last shown view crosses the line and hides it
	p := m.sp.(*viewsPane)
	for p.o.items[p.o.shown[p.o.cursor]].value != "merge" {
		press(m, "down")
	}
	press(m, "shift+down", "shift+down")
	assert.Zero(t, m.mods.Position("merge"), "crossing the line hides the view")
	press(m, "shift+up")
	assert.NotZero(t, m.mods.Position("merge"), "crossing back shows it")

	// shift+r puts the shipped order back
	press(m, "R")
	assert.True(t, m.mods.IsDefault())

	press(m, "esc")
	assert.Equal(t, focusList, m.focus)
}
