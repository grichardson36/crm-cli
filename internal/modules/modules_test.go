package modules_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultArrangement(t *testing.T) {
	a := modules.Default()
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "settings"}, a.Visible(), "Archived and Sync start hidden, Settings is last")
	assert.Equal(t, 5, a.Position("organizations"))
	assert.Equal(t, 6, a.Position("deals"))
	assert.Equal(t, "9", modules.Key(a.Position("settings")))
	assert.Equal(t, "0", modules.Key(10), "the tenth module takes the 0 key")
	assert.Equal(t, "", modules.Key(11))
	assert.Equal(t, []string{"archived", "sync"}, a.Hidden)
	last := modules.All[len(modules.All)-1]
	assert.Equal(t, "settings", last.ID, "Settings is last in the default order")
	assert.True(t, last.Always)
	assert.True(t, a.IsDefault())
	assert.NoError(t, a.Check())
}

func TestLookupNames(t *testing.T) {
	for in, want := range map[string]string{"Orgs": "organizations", "due": "follow-up", "Follow-up": "follow-up", "DEALS": "deals", "dupes": "merge"} {
		m, ok := modules.Lookup(in)
		require.True(t, ok, in)
		assert.Equal(t, want, m.ID, in)
	}
	_, ok := modules.Lookup("calendar")
	assert.False(t, ok)
}

func TestOnOffMoveAndGuards(t *testing.T) {
	a := modules.Default()
	var err error
	a, err = a.With("birthdays", false)
	require.NoError(t, err)
	assert.Equal(t, 0, a.Position("birthdays"))
	assert.Equal(t, 7, a.Position("merge"), "the next module takes its place")
	a, err = a.With("deals", false)
	require.NoError(t, err)
	assert.Equal(t, 0, a.Position("deals"))
	a, err = a.With("deals", true)
	require.NoError(t, err)
	assert.Equal(t, 6, a.Position("deals"), "turning it on again restores its place")

	a, err = a.Moved("deals", 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"deals", "home", "people"}, a.Visible()[:3])
	_, err = a.Moved("deals", 12)
	assert.Error(t, err)
	_, err = a.Moved("nope", 1)
	assert.Error(t, err)

	// Home or People must stay on, and at least one module.
	b, err := modules.Default().With("home", false)
	require.NoError(t, err)
	_, err = b.With("people", false)
	assert.ErrorContains(t, err, "Home or People")
	var c modules.Arrangement = modules.Default()
	for _, id := range []string{"home", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "archived", "sync", "people"} {
		c, err = c.With(id, false)
		if err != nil {
			break
		}
	}
	require.Error(t, err)
	assert.Equal(t, []string{"people", "settings"}, c.Visible(), "People keeps the start screen and Settings never goes")
	_, err = c.With("people", false)
	assert.Error(t, err)
}

func TestConfigRoundTripKeepsOtherSettings(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(modules.ConfigFile(dir), []byte("theme = \"dark\"\n\n[keys]\nundo = \"ctrl+z\"\n"), 0o644))
	a, err := modules.Default().With("tasks", false)
	require.NoError(t, err)
	a, err = a.Moved("organizations", 2)
	require.NoError(t, err)
	require.NoError(t, modules.Save(dir, a))

	got, problems := modules.Load(dir)
	assert.Empty(t, problems)
	assert.Equal(t, a.Visible(), got.Visible())
	assert.Equal(t, "organizations", got.Visible()[1])
	data, _ := os.ReadFile(modules.ConfigFile(dir))
	assert.Contains(t, string(data), "[keys]")
	assert.Contains(t, string(data), "theme")
	assert.Contains(t, string(data), "[modules]")

	// The default arrangement removes the table and keeps the rest.
	require.NoError(t, modules.Save(dir, modules.Default()))
	data, _ = os.ReadFile(modules.ConfigFile(dir))
	assert.NotContains(t, string(data), "[modules]")
	assert.Contains(t, string(data), "[keys]")
}

func TestLoadNeverFails(t *testing.T) {
	dir := t.TempDir()
	got, problems := modules.Load(dir)
	assert.Empty(t, problems, "no file is the default")
	assert.True(t, got.IsDefault())

	write := func(s string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(s), 0o644))
	}
	write("this is [not toml")
	got, problems = modules.Load(dir)
	assert.NotEmpty(t, problems)
	assert.True(t, got.IsDefault())

	write("[modules]\norder = [\"tasks\", \"nope\", \"home\"]\nhidden = [\"birthdays\", \"alsonope\"]\n")
	got, problems = modules.Load(dir)
	assert.Len(t, problems, 2, "each unknown name is reported")
	assert.Equal(t, []string{"tasks", "home"}, got.Visible()[:2])
	assert.Equal(t, 0, got.Position("birthdays"))
	assert.Equal(t, 6, got.Position("deals"), "a module missing from order is appended in default order, and hidden names only birthdays")
	assert.NotZero(t, got.Position("archived"), "an explicit table is respected: Archived is on because it does not list it")
	assert.NotZero(t, got.Position("settings"))

	write("[modules]\nhidden = [\"home\", \"people\"]\n")
	got, problems = modules.Load(dir)
	assert.NotEmpty(t, problems, "no start screen")
	assert.True(t, got.IsDefault())

	write("[modules]\norder = \"home\"\n")
	_, problems = modules.Load(dir)
	assert.NotEmpty(t, problems)

	write("modules = 3\n")
	_, problems = modules.Load(dir)
	assert.NotEmpty(t, problems)
}

func TestSettingsAndViewsCannotBeHidden(t *testing.T) {
	_, err := modules.Default().With("settings", false)
	assert.ErrorContains(t, err, "stays on")
	_, err = modules.Default().With("Setting", false)
	assert.Error(t, err, "an alias is refused too")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(modules.ConfigFile(dir), []byte("[modules]\nhidden = [\"settings\", \"birthdays\"]\n"), 0o644))
	got, problems := modules.Load(dir)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "cannot be hidden")
	assert.NotZero(t, got.Position("settings"), "a hand-edited config cannot hide it")
	assert.Zero(t, got.Position("birthdays"))
	assert.False(t, got.Normalize().IsHidden("settings"))
}

func TestArchivedAndSyncHiddenByDefaultButExplicitConfigIsRespected(t *testing.T) {
	d := modules.Default()
	assert.True(t, d.IsHidden("archived"))
	assert.True(t, d.IsHidden("sync"))
	rows := d.Rows()
	for _, r := range rows {
		switch r.ID {
		case "archived", "sync":
			assert.False(t, r.On, r.ID)
			assert.False(t, r.Default, "%s is not on by default", r.ID)
		default:
			assert.True(t, r.On, r.ID)
			assert.True(t, r.Default, r.ID)
		}
	}

	dir := t.TempDir()
	got, problems := modules.Load(dir)
	assert.Empty(t, problems)
	assert.True(t, got.IsHidden("archived"), "a new setup has them hidden")

	// An existing arrangement written before the change lists every module on.
	old := "[modules]\norder = [\"home\", \"people\", \"follow-up\", \"tasks\", \"organizations\", \"deals\", \"birthdays\", \"merge\", \"archived\", \"sync\"]\nhidden = []\n"
	require.NoError(t, os.WriteFile(modules.ConfigFile(dir), []byte(old), 0o644))
	got, problems = modules.Load(dir)
	assert.Empty(t, problems)
	assert.NotZero(t, got.Position("archived"))
	assert.NotZero(t, got.Position("sync"))
	assert.Equal(t, "settings", got.Visible()[len(got.Visible())-1], "Settings joins at the end")

	// Saving it keeps it explicit.
	require.NoError(t, modules.Save(dir, got))
	again, _ := modules.Load(dir)
	assert.NotZero(t, again.Position("archived"))
}

func TestSetValueKeepsOtherSettingsAndRemovesEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, modules.Save(dir, mustArrangement(t)))
	require.NoError(t, modules.SetValue(dir, "open", "search_url", "https://duckduckgo.com/?q={query}"))
	data, _ := os.ReadFile(modules.ConfigFile(dir))
	assert.Contains(t, string(data), "[open]")
	assert.Contains(t, string(data), "duckduckgo")
	assert.Contains(t, string(data), "[modules]")
	require.NoError(t, modules.SetValue(dir, "open", "search_url", ""))
	data, _ = os.ReadFile(modules.ConfigFile(dir))
	assert.NotContains(t, string(data), "[open]")
	assert.Contains(t, string(data), "[modules]")
}

func mustArrangement(t *testing.T) modules.Arrangement {
	a, err := modules.Default().With("tasks", false)
	require.NoError(t, err)
	return a
}
