package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModulesCommands(t *testing.T) {
	home := t.TempDir()
	run := func(args ...string) (string, string, int) {
		return keysRun(t, home, append([]string{"modules"}, args...)...)
	}
	stdout, stderr, code := run("list", "-f", "json")
	require.Equal(t, 0, code, stderr)
	rows := keyRows(t, stdout)
	assert.Equal(t, true, rows["organizations"]["on"])
	assert.EqualValues(t, 5, rows["organizations"]["position"])
	assert.Equal(t, "5", rows["organizations"]["key"])
	assert.Equal(t, true, rows["deals"]["on"])
	assert.EqualValues(t, 6, rows["deals"]["position"])
	assert.Equal(t, false, rows["archived"]["on"], "Archived and Sync start hidden")
	assert.Equal(t, false, rows["sync"]["on"])
	assert.Equal(t, "", rows["sync"]["key"])
	assert.Equal(t, "9", rows["settings"]["key"], "Settings is last and always on")
	assert.Equal(t, false, rows["archived"]["default"])
	for _, col := range []string{"order", "id", "name", "on", "position", "key", "default", "description"} {
		assert.Contains(t, rows["home"], col, "the row shape is unchanged")
	}

	_, stderr, code = run("off", "deals")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = run("on", "deals")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = run("off", "Birthdays", "merge")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ = run("move", "deals", "1", "-f", "json")
	rows = keyRows(t, stdout)
	assert.EqualValues(t, 1, rows["deals"]["position"])
	assert.EqualValues(t, 2, rows["home"]["position"])
	assert.Equal(t, false, rows["birthdays"]["on"])
	data, err := os.ReadFile(filepath.Join(home, ".crm", "config.toml"))
	if err != nil { // the test helper may point the data directory elsewhere
		data, err = os.ReadFile(filepath.Join(home, "config.toml"))
	}
	require.NoError(t, err)
	assert.Contains(t, string(data), "[modules]")

	// Guards and bad input.
	_, stderr, code = run("off", "home", "people")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "Home or People")
	_, stderr, code = run("off", "settings")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "stays on")
	_, stderr, code = run("on", "archived", "sync")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = run("off", "settings")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "stays on")
	_, stderr, code = run("on", "archived", "sync")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = run("on", "calendar")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "no module")
	_, _, code = run("move", "tasks", "99")
	assert.NotEqual(t, 0, code)
	_, _, code = run("move", "tasks", "x")
	assert.NotEqual(t, 0, code)

	stdout, _, code = run("reset", "-f", "json")
	require.Equal(t, 0, code)
	rows = keyRows(t, stdout)
	assert.Equal(t, true, rows["deals"]["on"])
	assert.Equal(t, true, rows["birthdays"]["on"])
	assert.EqualValues(t, 1, rows["home"]["position"])
}
