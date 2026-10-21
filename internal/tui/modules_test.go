package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tabBar(m *Model) string { return strings.Split(m.View(), "\n")[0] }

func paletteTitles(m *Model) []string {
	press(m, "ctrl+k")
	var out []string
	for _, it := range m.ov.items {
		out = append(out, it.label)
	}
	press(m, "esc")
	return out
}

func TestModuleOffHidesTabsKeysBracketsAndPalette(t *testing.T) {
	m, _ := newTestModel(t)
	require.NoError(t, m.setModules(mustWith(t, m.mods, "birthdays", false)))

	assert.NotContains(t, tabBar(m), "Birthdays")
	assert.Equal(t, 0, m.viewPosition(viewBirthdays))
	assert.Equal(t, 7, m.viewPosition(viewMerge), "Merge moves up one place")
	press(m, "7")
	assert.Equal(t, viewMerge, m.view, "the number key follows the arrangement")

	// [ and ] skip it.
	m.switchView(viewDeals)
	press(m, "]")
	assert.Equal(t, viewMerge, m.view)

	// The view list and the palette do not offer it.
	press(m, "v")
	for _, it := range m.ov.items {
		assert.NotEqual(t, "Birthdays", it.label)
	}
	press(m, "esc")
	assert.NotContains(t, paletteTitles(m), "Birthdays view")
	assert.Contains(t, paletteTitles(m), "Merge view")

	// The data stays: the view still works when something opens it.
	m.switchView(viewBirthdays)
	assert.Equal(t, viewBirthdays, m.view)
	assert.Contains(t, tabBar(m), "v Birthdays")
	assert.NotEmpty(t, m.rows)
}

func TestOffOrganizationsStillOpensRecordPagesFromLinks(t *testing.T) {
	m, _ := newTestModel(t)
	o, err := m.orr.Create(m.ctx, modelOrg("Acme Holdings"))
	require.NoError(t, err)
	require.NoError(t, m.setModules(mustWith(t, m.mods, "organizations", false)))
	assert.NotContains(t, tabBar(m), "Organizations")
	assert.NotContains(t, paletteTitles(m), "Organizations view")

	m.openObject("organization", o.ID)
	assert.Equal(t, viewOrgs, m.view)
	assert.Equal(t, o.ID, m.selOrg())
	assert.Contains(t, tabBar(m), "v Organizations")
	m.form = m.buildOrgForm(o.ID)
	assert.NotEmpty(t, m.form)
	press(m, "]")
	assert.Equal(t, viewHome, m.view, "] from a view that is off goes to the first one")
}

func TestModulesScreenTogglesMovesResetsAndSaves(t *testing.T) {
	m, _ := newTestModel(t)
	m.env.DataDir = t.TempDir()

	runPalette(m, "modules")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "Modules")
	require.Len(t, m.ov.items, 10)
	assert.Contains(t, m.ov.items[0].label, "Home")
	assert.Equal(t, "1", m.ov.items[0].hint)
	assert.Contains(t, m.ov.items[5].label, "Deals")
	assert.Contains(t, m.ov.items[5].label, "on")
	assert.Equal(t, "0", m.ov.items[9].hint, "the tenth module takes 0")

	// Space turns Home off (People keeps the start screen), K moves the cursor row up.
	press(m, " ")
	assert.Equal(t, 0, m.mods.Position("home"))
	assert.Equal(t, "1", m.ov.items[1].hint, "People is now key 1")
	press(m, "down", "down", "K")
	assert.Equal(t, "follow-up", m.mods.Normalize().Order[1], "the cursor row moved up")
	data, err := os.ReadFile(modules.ConfigFile(m.env.DataDir))
	require.NoError(t, err)
	assert.Contains(t, string(data), "[modules]")

	// Turning off the last start screen is refused with a message.
	press(m, "down", " ") // People is under the cursor now
	assert.NotZero(t, m.mods.Position("people"), "People stays on")
	assert.True(t, m.statusIsErr)

	press(m, "r")
	assert.True(t, m.mods.IsDefault())
	_, err = os.Stat(modules.ConfigFile(m.env.DataDir))
	require.NoError(t, err)
	data, _ = os.ReadFile(modules.ConfigFile(m.env.DataDir))
	assert.NotContains(t, string(data), "[modules]")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
}

func TestStartupReadsTheArrangementAndSurvivesBadConfig(t *testing.T) {
	m, _ := newTestModel(t)
	dir := t.TempDir()
	m.env.DataDir = dir
	require.NoError(t, os.WriteFile(modules.ConfigFile(dir), []byte("[modules]\norder = [\"tasks\", \"home\"]\nhidden = [\"people\", \"deals\"]\n"), 0o644))
	m.applyStartupModules()
	assert.Equal(t, 1, m.viewPosition(viewTasks))
	assert.Equal(t, 0, m.viewPosition(viewPeople))
	press(m, "1")
	assert.Equal(t, viewTasks, m.view)

	require.NoError(t, os.WriteFile(modules.ConfigFile(dir), []byte("[modules\n"), 0o644))
	m.applyStartupModules()
	assert.True(t, m.mods.IsDefault())
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "modules")
}

func TestRebindingAViewKeyKeepsTheUsersKey(t *testing.T) {
	m, _ := newTestModel(t)
	m.applyKeys(map[string][]string{"view-tasks": {"ctrl+t"}})
	require.NoError(t, m.setModules(mustWith(t, m.mods, "follow-up", false)))
	a := m.actionByID("view-tasks")
	assert.Equal(t, []string{"ctrl+t"}, a.keys, "a rebound key is not replaced by the position")
	assert.Equal(t, []string{"4"}, m.actionByID("view-organizations").keys)
}

func TestKeysHelpAndDefaultsMentionModules(t *testing.T) {
	infos := DefaultActions()
	var ids []string
	for _, a := range infos {
		ids = append(ids, a.ID)
	}
	assert.Contains(t, ids, "modules")
	assert.Contains(t, ids, "view-organizations")
	assert.Contains(t, ids, "view-deals")
}
