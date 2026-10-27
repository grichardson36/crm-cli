package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func keysRun(t *testing.T, home string, args ...string) (string, string, int) {
	t.Helper()
	return crmEnv(t, home, nil, append([]string{"--db", filepath.Join(home, "x.db")}, args...)...)
}

func keyRows(t *testing.T, s string) map[string]map[string]any {
	t.Helper()
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &rows), s)
	out := map[string]map[string]any{}
	for _, r := range rows {
		out[r["id"].(string)] = r
	}
	return out
}

func TestKeysListShowsDefaults(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, code := keysRun(t, home, "keys", "list", "-f", "json")
	require.Equal(t, 0, code, stderr)
	rows := keyRows(t, stdout)
	assert.Equal(t, []any{"ctrl+z"}, rows["undo"]["keys"])
	assert.Equal(t, []any{"ctrl+y"}, rows["redo"]["keys"])
	assert.Equal(t, []any{"space"}, rows["mark"]["keys"])
	assert.Equal(t, false, rows["undo"]["custom"])
	assert.Equal(t, true, rows["favorite"]["changes_data"])
	assert.Equal(t, "App", rows["undo"]["group"])
	assert.Equal(t, "framework", rows["undo"]["layer"])
	assert.Equal(t, "app", rows["undo"]["scope"])
	assert.Equal(t, "app", rows["snooze"]["layer"], "snooze is specific to this CRM")
	assert.Equal(t, "type:person", rows["snooze"]["scope"])
	assert.Equal(t, []any{"enter", "e"}, rows["activate"]["keys"], "enter and e are one action")
	assert.Equal(t, []any{"a"}, rows["add-any"]["keys"])
	assert.Equal(t, []any{}, rows["tier"]["keys"], "tier moved to the palette")

	stdout, _, _ = keysRun(t, home, "keys", "list", "--group", "contact", "-f", "json")
	for _, r := range keyRows(t, stdout) {
		assert.Equal(t, "Contact", r["group"])
	}
}

func TestKeysSetListAndReset(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, code := keysRun(t, home, "keys", "set", "favorite", "ctrl+f", "ctrl+e", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	assert.Equal(t, true, out["saved"])
	assert.Equal(t, []any{"ctrl+f", "ctrl+e"}, out["keys"])
	assert.Equal(t, []any{"f"}, out["default"])

	data, err := os.ReadFile(filepath.Join(home, ".crm", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "[keys]")
	assert.Contains(t, string(data), `"ctrl+f"`)

	stdout, _, _ = keysRun(t, home, "keys", "list", "--custom", "-f", "json")
	rows := keyRows(t, stdout)
	require.Len(t, rows, 1)
	assert.Equal(t, true, rows["favorite"]["custom"])
	assert.Equal(t, []any{"ctrl+f", "ctrl+e"}, rows["favorite"]["keys"])

	_, _, code = keysRun(t, home, "keys", "reset", "favorite")
	require.Equal(t, 0, code)
	stdout, _, _ = keysRun(t, home, "keys", "list", "--custom", "-f", "json")
	assert.Len(t, keyRows(t, stdout), 0)

	// reset --all and the no-argument error.
	keysRun(t, home, "keys", "set", "undo", "ctrl+g")
	keysRun(t, home, "keys", "set", "tier", "ctrl+e")
	_, stderr, code = keysRun(t, home, "keys", "reset")
	assert.Equal(t, 2, code, stderr)
	_, _, code = keysRun(t, home, "keys", "reset", "--all")
	require.Equal(t, 0, code)
	stdout, _, _ = keysRun(t, home, "keys", "list", "--custom", "-f", "json")
	assert.Len(t, keyRows(t, stdout), 0)
}

func TestKeysSetKeepsOtherConfigSettings(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".crm"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".crm", "config.toml"), []byte("theme = \"nord\"\n"), 0o600))
	_, stderr, code := keysRun(t, home, "keys", "set", "undo", "ctrl+g")
	require.Equal(t, 0, code, stderr)
	data, _ := os.ReadFile(filepath.Join(home, ".crm", "config.toml"))
	assert.Contains(t, string(data), `theme = "nord"`)
	assert.Contains(t, string(data), `undo = "ctrl+g"`)
	stdout, _, code := keysRun(t, home, "theme", "show", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, `"name":"nord"`)
}

func TestKeysSetErrors(t *testing.T) {
	home := t.TempDir()
	_, stderr, code := keysRun(t, home, "keys", "set", "nope", "ctrl+g")
	assert.Equal(t, 3, code)
	assert.Contains(t, stderr, `no action "nope"`)

	_, stderr, code = keysRun(t, home, "keys", "set", "undo", "banana")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "is not a key name")

	_, stderr, code = keysRun(t, home, "keys", "set", "undo", "ctrl+y")
	assert.Equal(t, 4, code, "a clash is a conflict")
	assert.Contains(t, stderr, `already used by "Redo"`)

	_, stderr, code = keysRun(t, home, "keys", "set", "fav", "ctrl+g")
	assert.Equal(t, 3, code)
	assert.Contains(t, stderr, "did you mean")
	assert.Contains(t, stderr, "favorite")

	_, _, code = keysRun(t, home, "keys", "set", "undo")
	assert.Equal(t, 2, code, "a key is required")
	_, _, code = keysRun(t, home, "keys", "reset", "nope")
	assert.Equal(t, 3, code)

	// Nothing was written by the failed attempts.
	_, err := os.Stat(filepath.Join(home, ".crm", "config.toml"))
	assert.True(t, os.IsNotExist(err))
}

func TestKeysDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, code := keysRun(t, home, "keys", "set", "undo", "ctrl+g", "--dry-run", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, `"dry_run":true`)
	assert.Contains(t, stdout, `"saved":false`)
	_, err := os.Stat(filepath.Join(home, ".crm", "config.toml"))
	assert.True(t, os.IsNotExist(err))
	_, _, code = keysRun(t, home, "keys", "set", "undo", "banana", "--dry-run")
	assert.Equal(t, 2, code, "a dry run still validates")
	_, _, code = keysRun(t, home, "keys", "reset", "--all", "--dry-run")
	assert.Equal(t, 0, code)
}

func TestKeysListReportsBadConfigWithoutFailing(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".crm"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".crm", "config.toml"), []byte(`
[keys]
undo = "banana"
favorite = "ctrl+f"
bogus = "ctrl+g"
redo = "ctrl+z"
`), 0o600))
	stdout, stderr, code := keysRun(t, home, "keys", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "keys.undo")
	assert.Contains(t, stderr, "keys.bogus: no such action")
	assert.Contains(t, stderr, "keys.redo: ctrl+z is already used")
	rows := keyRows(t, stdout)
	assert.Equal(t, []any{"ctrl+f"}, rows["favorite"]["keys"])
	assert.Equal(t, true, rows["favorite"]["custom"])
	assert.Equal(t, []any{"ctrl+z"}, rows["undo"]["keys"], "the bad entry falls back to the default")
	assert.Equal(t, []any{"ctrl+y"}, rows["redo"]["keys"])
}

func TestKeysPath(t *testing.T) {
	home := t.TempDir()
	stdout, _, code := keysRun(t, home, "keys", "path", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, `"exists":false`)
	assert.Contains(t, stdout, "config.toml")
	keysRun(t, home, "keys", "set", "undo", "ctrl+g")
	stdout, _, _ = keysRun(t, home, "keys", "path", "-f", "json")
	assert.Contains(t, stdout, `"exists":true`)
}

func TestKeysSetWarnsAboutChordsTerminalsSwallow(t *testing.T) {
	for k, action := range map[string]string{"ctrl+i": "tier", "ctrl+m": "tier", "ctrl+h": "tier", "ctrl+[": "tier", "ctrl+j": "tier", "ctrl+shift+up": "home-top", "ctrl+shift+down": "home-bottom"} {
		home := t.TempDir()
		_, stderr, code := keysRun(t, home, "keys", "set", action, k)
		require.Equal(t, 0, code, "%s: %s", k, stderr)
		assert.Contains(t, stderr, "warning: "+k+" may never reach the program", k)
	}
	home := t.TempDir()
	_, stderr, code := keysRun(t, home, "keys", "set", "tier", "ctrl+g")
	require.Equal(t, 0, code, stderr)
	assert.NotContains(t, stderr, "warning", "an ordinary chord has no warning")
}

func TestKeysListReportsRemovedActionsAndTakenKeysWithoutFailing(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".crm"), 0o700))
	// An old file: an action that never existed, and keys the framework owns now.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".crm", "config.toml"), []byte(`
[keys]
go-to-today = "ctrl+t"
snooze = "d"
tier = "p"
sync-now = "s"
`), 0o600))
	stdout, stderr, code := keysRun(t, home, "keys", "list", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "keys.go-to-today: no such action")
	assert.Contains(t, stderr, "keys.snooze: d is already used by")
	rows := keyRows(t, stdout)
	assert.Equal(t, []any{"p"}, rows["tier"]["keys"], "an old binding of a palette-only action still works")
	assert.Equal(t, []any{"s"}, rows["sync-now"]["keys"], "and s is free on the Sync screen")
	assert.Equal(t, []any{"s"}, rows["snooze"]["keys"], "the taken key falls back to the default")
}
