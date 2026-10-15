package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverrideReplacesTheDefaultKeysAndTheHint(t *testing.T) {
	m, pr := newTestModel(t)
	problems := m.applyKeys(keys.Overrides{"favorite": {"ctrl+f"}})
	assert.Empty(t, problems)
	a := m.actionByID("favorite")
	assert.Equal(t, []string{"ctrl+f"}, a.keys)
	assert.Equal(t, "ctrl+f", a.hint)

	press(m, "f")
	assert.False(t, personByID(t, pr, 1).IsFavorite, "the old key no longer runs it")
	m.Update(keyMsg("ctrl+f"))
	assert.True(t, personByID(t, pr, 1).IsFavorite)

	// The palette and the help screen show the effective key.
	press(m, "ctrl+k")
	typeText(m, "toggle favorite")
	assert.Equal(t, "ctrl+f", paletteItem(t, m, "favorite").hint)
	press(m, "esc")
	assert.Contains(t, m.helpText(), "ctrl+f")
}

func TestOverrideCanBindSeveralKeysAndSpace(t *testing.T) {
	m, pr := newTestModel(t)
	assert.Empty(t, m.applyKeys(keys.Overrides{"favorite": {"f", "*", " "}}))
	assert.Equal(t, "f", m.actionByID("favorite").hint)
	press(m, "f")
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	assert.Contains(t, m.helpText(), "space")
}

func TestConflictingOverrideIsSkippedAndNamesTheOtherAction(t *testing.T) {
	m, _ := newTestModel(t)
	// q quits, in the App group that works everywhere.
	problems := m.applyKeys(keys.Overrides{"undo": {"q"}})
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "keys.undo: q is already used by")
	assert.Equal(t, []string{"ctrl+z"}, m.actionByID("undo").keys, "the default stays")

	// Two actions of one group cannot share a key either; a different group may.
	problems = m.applyKeys(keys.Overrides{"favorite": {"c"}})
	require.Len(t, problems, 1, "c is Mark contacted, also in the Contact group")
	assert.Contains(t, problems[0], "Mark contacted")
	problems = m.applyKeys(keys.Overrides{"sync-now": {"c"}})
	assert.Empty(t, problems, "Sync actions work on another screen than Contact actions")

	// The built-in keys that overlap across screens never produce a message.
	assert.Empty(t, m.applyKeys(keys.Overrides{}))
}

func TestTwoOverridesCannotClaimTheSameKey(t *testing.T) {
	m, _ := newTestModel(t)
	problems := m.applyKeys(keys.Overrides{"favorite": {"ctrl+g"}, "tier": {"ctrl+g"}})
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "keys.tier")
	assert.Contains(t, problems[0], "ctrl+g is already used")
	assert.Equal(t, []string{"ctrl+g"}, m.actionByID("favorite").keys)
}

func TestUnknownActionIsReportedWithASuggestion(t *testing.T) {
	m, _ := newTestModel(t)
	problems := m.applyKeys(keys.Overrides{"fav": {"ctrl+g"}, "nonsense-action": {"ctrl+h"}})
	require.Len(t, problems, 2)
	assert.Contains(t, problems[0], "keys.fav: no such action")
	assert.Contains(t, problems[0], "did you mean")
	assert.Contains(t, problems[0], "favorite")
	assert.Contains(t, problems[1], "keys.nonsense-action: no such action")
}

func TestStartupReadsConfigAndReportsProblemsWithoutCrashing(t *testing.T) {
	m, _ := newTestModel(t)
	dir := t.TempDir()
	m.env.DataDir = dir
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`
theme = "dark"

[keys]
favorite = "ctrl+f"
tier = "banana"
bogus = "ctrl+g"
`), 0o600))
	m.applyStartupKeys()
	assert.Equal(t, []string{"ctrl+f"}, m.actionByID("favorite").keys)
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "keys: ")
	assert.Contains(t, m.status, "+1 more")

	// Broken TOML is a message too.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[oops"), 0o600))
	m2, _ := newTestModel(t)
	m2.env.DataDir = dir
	m2.applyStartupKeys()
	assert.Contains(t, m2.status, "not valid TOML")
	assert.NotNil(t, m2.actionByID("favorite"))
}

func TestKeysCustomizeFlowSavesTheBinding(t *testing.T) {
	m, pr := newTestModel(t)
	dir := t.TempDir()
	m.env.DataDir = dir
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte("theme = \"nord\"\n"), 0o600))

	press(m, "ctrl+k")
	typeText(m, "keys: customize")
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode, "a picker lists the actions")
	typeText(m, "toggle favorite")
	press(m, "enter")
	require.Equal(t, modeCapture, m.mode)
	assert.Contains(t, m.View(), "press the new key for Toggle favorite")

	// A key that is taken is refused and the capture stays open.
	m.Update(keyMsg("c"))
	assert.Equal(t, modeCapture, m.mode)
	assert.True(t, m.toast.isErr)
	assert.Contains(t, m.toast.text, "Mark contacted")

	m.Update(keyMsg("ctrl+f"))
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "bind Toggle favorite to ctrl+f (now f)")
	press(m, "y")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.toast.text, "Toggle favorite is now ctrl+f")
	assert.Equal(t, []string{"ctrl+f"}, m.actionByID("favorite").keys)

	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `favorite = "ctrl+f"`)
	assert.Contains(t, string(data), `theme = "nord"`, "other settings are kept")
	o, problems := keys.Load(dir)
	assert.Empty(t, problems)
	assert.Equal(t, keys.Overrides{"favorite": {"ctrl+f"}}, o)

	m.Update(keyMsg("ctrl+f"))
	assert.True(t, personByID(t, pr, 1).IsFavorite)

	// Esc cancels a capture without changes; n declines the confirmation.
	press(m, "ctrl+k")
	typeText(m, "keys: customize")
	press(m, "enter")
	typeText(m, "undo")
	press(m, "enter")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	press(m, "ctrl+k")
	typeText(m, "keys: customize")
	press(m, "enter")
	typeText(m, "undo")
	press(m, "enter")
	m.Update(keyMsg("ctrl+g"))
	press(m, "n")
	assert.Equal(t, []string{"ctrl+z"}, m.actionByID("undo").keys)
}

func TestKeysResetFlow(t *testing.T) {
	m, _ := newTestModel(t)
	dir := t.TempDir()
	m.env.DataDir = dir
	require.NoError(t, keys.Save(dir, keys.Overrides{"favorite": {"ctrl+f"}, "tier": {"ctrl+g"}}))
	m.applyStartupKeys()
	require.Equal(t, []string{"ctrl+g"}, m.actionByID("tier").keys)

	press(m, "ctrl+k")
	typeText(m, "keys: reset")
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode)
	typeText(m, "favorite")
	press(m, "enter")
	assert.Equal(t, []string{"f"}, m.actionByID("favorite").keys)
	assert.Equal(t, []string{"ctrl+g"}, m.actionByID("tier").keys)
	o, _ := keys.Load(dir)
	assert.Equal(t, keys.Overrides{"tier": {"ctrl+g"}}, o)

	press(m, "ctrl+k")
	typeText(m, "keys: reset")
	press(m, "enter")
	typeText(m, "all actions")
	press(m, "enter")
	assert.Equal(t, []string{"p"}, m.actionByID("tier").keys)
	o, _ = keys.Load(dir)
	assert.Empty(t, o)

	// Nothing left to reset.
	press(m, "ctrl+k")
	typeText(m, "keys: reset")
	press(m, "enter")
	assert.Contains(t, m.toast.text, "no custom keys")
}

func TestKeysActionsAreBlockedInReadOnlyMode(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	press(m, "ctrl+k")
	typeText(m, "keys: customize")
	assert.Equal(t, "read-only", paletteItem(t, m, "keys-customize").disabled)
}

func TestSessionOnlyBindingWithoutADataDirectory(t *testing.T) {
	m, _ := newTestModel(t)
	m.env.DataDir = ""
	press(m, "ctrl+k")
	typeText(m, "keys: customize")
	press(m, "enter")
	typeText(m, "toggle favorite")
	press(m, "enter")
	m.Update(keyMsg("ctrl+f"))
	press(m, "y")
	assert.True(t, m.toast.isErr)
	assert.Contains(t, m.toast.text, "session only")
	assert.Equal(t, []string{"ctrl+f"}, m.actionByID("favorite").keys)
}

func TestDefaultActionsAndResolve(t *testing.T) {
	all := DefaultActions()
	assert.Greater(t, len(all), 60)
	byID := map[string]ActionInfo{}
	for _, a := range all {
		byID[a.ID] = a
	}
	assert.Equal(t, []string{"ctrl+z"}, byID["undo"].Keys)
	assert.Equal(t, []string{"ctrl+y"}, byID["redo"].Keys)
	assert.True(t, byID["undo"].Write)
	eff, problems := ResolveOverrides(keys.Overrides{"undo": {"ctrl+g"}})
	assert.Empty(t, problems)
	assert.Equal(t, []string{"ctrl+g"}, eff["undo"])
	assert.Equal(t, "", ConflictFor(nil, "undo", []string{"ctrl+g"}))
	assert.Contains(t, ConflictFor(nil, "undo", []string{"ctrl+y"}), "Redo")
}

func TestNoLegacyUndoRedoKeys(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "f")
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	press(m, "U")
	assert.True(t, personByID(t, pr, 1).IsFavorite, "U is not undo any more")
	m.Update(keyMsg("ctrl+r"))
	assert.Nil(t, m.toast)
	m.Update(keyMsg("ctrl+z"))
	assert.False(t, personByID(t, pr, 1).IsFavorite)
	assert.Contains(t, m.toast.text, "· ctrl+y redo")
	m.Update(keyMsg("ctrl+y"))
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	assert.Contains(t, m.toast.text, "· ctrl+z undo")
}

// keyMsg builds the key message for "ctrl+f" style names and plain characters.
func keyMsg(k string) tea.KeyMsg {
	if strings.HasPrefix(k, "ctrl+") && len(k) == 6 {
		return tea.KeyMsg{Type: tea.KeyType(int(k[5]-'a') + 1)}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
