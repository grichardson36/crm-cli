package modules_test

import (
	"os"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(modules.ConfigFile(dir), []byte(body), 0o644))
	return dir
}

func readConfig(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(modules.ConfigFile(dir))
	require.NoError(t, err)
	var raw map[string]any
	_, err = toml.Decode(string(b), &raw)
	require.NoError(t, err)
	return raw
}

// CRM-022, S8. The Birthdays view is now Dates. There is no alias for the old key: a stored
// arrangement is rewritten once, and the old name resolves to nothing.
func TestOldBirthdaysKeyIsGoneFromTheRegistry(t *testing.T) {
	for _, name := range []string{"birthdays", "Birthdays", "birthday"} {
		_, ok := modules.Lookup(name)
		assert.False(t, ok, name)
	}
	m, ok := modules.Lookup("Dates")
	require.True(t, ok)
	assert.Equal(t, "dates", m.ID)
	for _, row := range modules.Default().Rows() {
		assert.NotEqual(t, "birthdays", row.ID)
	}
	_, err := modules.Default().With("birthdays", false)
	assert.Error(t, err)
}

func TestLoadMovesAStoredBirthdaysArrangementToDates(t *testing.T) {
	dir := writeConfig(t, "[modules]\norder = [\"home\", \"people\", \"birthdays\", \"tasks\"]\nhidden = [\"birthdays\", \"merge\"]\n")
	a, problems := modules.Load(dir)
	assert.Empty(t, problems, "the old key is not reported as unknown")
	assert.Equal(t, 3, a.Position("tasks"), "home, people, tasks are shown: dates took the old place and is hidden")
	assert.Zero(t, a.Position("dates"), "it stays hidden")
	assert.True(t, a.IsHidden("dates"))
	assert.True(t, a.IsHidden("merge"))

	raw := readConfig(t, dir)
	table := raw["modules"].(map[string]any)
	assert.Equal(t, []any{"home", "people", "dates", "tasks"}, table["order"])
	assert.Equal(t, []any{"dates", "merge"}, table["hidden"])
	b, _ := os.ReadFile(modules.ConfigFile(dir))
	assert.NotContains(t, string(b), "birthdays", "the old key is gone from the file")
}

func TestMigrateConfigMovesKeyBindingsAndKeepsOtherSettings(t *testing.T) {
	dir := writeConfig(t, "[theme]\nname = \"night\"\n\n[keys]\nview-birthdays = \"7\"\nundo = \"ctrl+z\"\n")
	require.NoError(t, modules.MigrateConfig(dir))
	raw := readConfig(t, dir)
	k := raw["keys"].(map[string]any)
	assert.Equal(t, "7", k["view-dates"])
	assert.NotContains(t, k, "view-birthdays")
	assert.Equal(t, "ctrl+z", k["undo"])
	assert.Equal(t, "night", raw["theme"].(map[string]any)["name"], "other settings stay")

	// keys.Load does the same on a file nothing else has touched.
	dir = writeConfig(t, "[keys]\nview-birthdays = \"f9\"\n")
	o, problems := keys.Load(dir)
	assert.Empty(t, problems)
	assert.Equal(t, []string{"f9"}, o["view-dates"])
	assert.NotContains(t, o, "view-birthdays")
}

func TestMigrateConfigKeepsAnExistingDatesBindingAndIsQuietWhenNothingMoves(t *testing.T) {
	dir := writeConfig(t, "[keys]\nview-birthdays = \"7\"\nview-dates = \"8\"\n")
	require.NoError(t, modules.MigrateConfig(dir))
	k := readConfig(t, dir)["keys"].(map[string]any)
	assert.Equal(t, "8", k["view-dates"], "a binding already written for Dates wins")
	assert.NotContains(t, k, "view-birthdays")

	body := "# my own comment\n[modules]\nhidden = [\"merge\"]\n"
	dir = writeConfig(t, body)
	require.NoError(t, modules.MigrateConfig(dir))
	b, _ := os.ReadFile(modules.ConfigFile(dir))
	assert.Equal(t, body, string(b), "the file is not rewritten when nothing moved")

	require.NoError(t, modules.MigrateConfig(t.TempDir()), "no file is fine")
	require.NoError(t, modules.MigrateConfig(writeConfig(t, "not = [valid")), "a broken file is left alone")
}

func TestMigrateConfigMovesTheOldNameWhateverItsCase(t *testing.T) {
	dir := writeConfig(t, "[modules]\norder = [\"home\", \"Birthdays\", \"Birthday\"]\n")
	require.NoError(t, modules.MigrateConfig(dir))
	assert.Equal(t, []any{"home", "dates"}, readConfig(t, dir)["modules"].(map[string]any)["order"])
}

func TestMigrateConfigDropsDuplicateAfterTheMove(t *testing.T) {
	dir := writeConfig(t, "[modules]\norder = [\"home\", \"dates\", \"birthdays\"]\n")
	require.NoError(t, modules.MigrateConfig(dir))
	assert.Equal(t, []any{"home", "dates"}, readConfig(t, dir)["modules"].(map[string]any)["order"])
}
