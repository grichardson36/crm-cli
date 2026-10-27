package keys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	good := map[string]string{
		"a": "a", "T": "T", "?": "?", "+": "+", "/": "/", "é": "é", " ": " ",
		"enter": "enter", "Enter": "enter", "ESC": "esc", "escape": "esc", "tab": "tab", "shift+tab": "shift+tab",
		"up": "up", "PgDown": "pgdown", "pageup": "pgup", "space": " ", "backspace": "backspace", "delete": "delete",
		"f1": "f1", "F12": "f12", "ctrl+z": "ctrl+z", "Ctrl+Y": "ctrl+y", "control+k": "ctrl+k",
		"alt+x": "alt+x", "alt+ctrl+a": "alt+ctrl+a", "ctrl+alt+a": "alt+ctrl+a", "ctrl+up": "ctrl+up",
		"shift+up": "shift+up", "shift+f5": "shift+f5", "alt+enter": "alt+enter", "alt+/": "alt+/", "alt++": "alt++",
		"meta+q": "alt+q",
	}
	for in, want := range good {
		got, err := Normalize(in)
		assert.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	bad := []string{"", "banana", "ctrl+", "ctrl+1", "ctrl+enter", "shift+t", "super+a", "f99", "f0", "ctrl+ctrl+", "alt+banana", "+++", "ctrl++"}
	for _, in := range bad {
		_, err := Normalize(in)
		assert.Error(t, err, "%q should be rejected", in)
	}
	_, err := Normalize("banana")
	assert.Contains(t, err.Error(), "is not a key name")
	_, err = Normalize("shift+t")
	assert.Contains(t, err.Error(), "capital letter")
	_, err = Normalize("hyper+a")
	assert.Contains(t, err.Error(), "unknown modifier")
	assert.Equal(t, "space", Display(" "))
	assert.Equal(t, "ctrl+z", Display("ctrl+z"))
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	o, problems := Load(t.TempDir())
	assert.Empty(t, o)
	assert.Empty(t, problems)
}

func TestLoadReadsStringsAndLists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte(`
theme = "nord"

[keys]
undo = "Ctrl+Z"
favorite = ["f", "*"]
snooze = "space"
`), 0o600))
	o, problems := Load(dir)
	assert.Empty(t, problems)
	assert.Equal(t, Overrides{"undo": {"ctrl+z"}, "favorite": {"f", "*"}, "snooze": {" "}}, o)
}

func TestLoadReportsProblemsAndKeepsTheGoodEntries(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte(`
[keys]
undo = "ctrl+z"
redo = 5
tier = ["p", 3]
note = "banana"
tag = []
favorite = ["f", "nonsense+key"]
`), 0o600))
	o, problems := Load(dir)
	assert.Equal(t, Overrides{"undo": {"ctrl+z"}}, o)
	require.Len(t, problems, 5)
	joined := ""
	for _, p := range problems {
		joined += p + "\n"
	}
	assert.Contains(t, joined, "keys.redo: use a string")
	assert.Contains(t, joined, "keys.tier: every key must be a string")
	assert.Contains(t, joined, "keys.note")
	assert.Contains(t, joined, "keys.tag: the list is empty")
	assert.Contains(t, joined, "keys.favorite")
}

func TestLoadSurvivesBrokenTomlAndAWrongTable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("this is [not toml"), 0o600))
	o, problems := Load(dir)
	assert.Empty(t, o)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "not valid TOML")

	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte(`keys = "oops"`), 0o600))
	o, problems = Load(dir)
	assert.Empty(t, o)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "must be a table")
}

func TestSaveKeepsOtherSettingsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("theme = \"gruvbox\"\n"), 0o600))
	require.NoError(t, Save(dir, Overrides{"undo": {"ctrl+u"}, "favorite": {"f", " "}}))
	data, err := os.ReadFile(ConfigFile(dir))
	require.NoError(t, err)
	assert.Contains(t, string(data), `theme = "gruvbox"`)
	assert.Contains(t, string(data), `undo = "ctrl+u"`)
	assert.Contains(t, string(data), `"space"`)
	o, problems := Load(dir)
	assert.Empty(t, problems)
	assert.Equal(t, Overrides{"undo": {"ctrl+u"}, "favorite": {"f", " "}}, o)

	// Removing every override removes the table and keeps the theme.
	require.NoError(t, Save(dir, Overrides{}))
	data, _ = os.ReadFile(ConfigFile(dir))
	assert.Contains(t, string(data), `theme = "gruvbox"`)
	assert.NotContains(t, string(data), "[keys]")
}

func TestSaveRefusesBrokenToml(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(ConfigFile(dir), []byte("[broken"), 0o600))
	err := Save(dir, Overrides{"undo": {"ctrl+u"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fix or remove it")
	data, _ := os.ReadFile(ConfigFile(dir))
	assert.Equal(t, "[broken", string(data), "the file is left alone")
}

func TestSaveCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", ".crm")
	require.NoError(t, Save(dir, Overrides{"undo": {"ctrl+u"}}))
	_, err := os.Stat(ConfigFile(dir))
	assert.NoError(t, err)
}

func TestNormalizeAcceptsCtrlPunctuationAndCtrlShiftArrows(t *testing.T) {
	for in, want := range map[string]string{
		"ctrl+[": "ctrl+[", "ctrl+]": "ctrl+]", "ctrl+shift+up": "ctrl+shift+up", "Ctrl+Shift+Down": "ctrl+shift+down", "shift+ctrl+up": "ctrl+shift+up",
	} {
		got, err := Normalize(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
}
