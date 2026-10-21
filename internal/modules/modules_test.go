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
	assert.Equal(t, []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "archived", "sync"}, a.Visible(), "the default shows every module")
	assert.Equal(t, 5, a.Position("organizations"))
	assert.Equal(t, 6, a.Position("deals"))
	assert.Equal(t, "0", modules.Key(a.Position("sync")), "the tenth module takes the 0 key")
	assert.Equal(t, "", modules.Key(11))
	assert.Empty(t, a.Hidden)
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
	_, err = a.Moved("deals", 11)
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
	assert.Equal(t, 1, len(c.Visible()), "the last module that keeps the program usable stays on")
	assert.Equal(t, "people", c.Visible()[0])
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
