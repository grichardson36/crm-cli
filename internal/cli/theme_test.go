package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// All theme tests run with HOME and USERPROFILE pointing at a temp dir, so ~/.crm is never the real one.

func TestThemeListShowsBuiltinsAndMarksActive(t *testing.T) {
	home := t.TempDir()
	stdout, _, code := crmEnv(t, home, []string{"CRM_THEME="}, "theme", "list", "-f", "json")
	require.Equal(t, 0, code)
	var rows []struct {
		Name, Kind string
		Active     bool
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
		assert.Equal(t, "builtin", r.Kind)
		assert.Equal(t, r.Name == "auto", r.Active, "auto is the default")
	}
	assert.Equal(t, []string{"auto", "dark", "light", "high-contrast", "catppuccin", "nord", "gruvbox", "solarized"}, names)
}

func TestThemeSetShowPathAndPersistence(t *testing.T) {
	home := t.TempDir()
	noEnv := []string{"CRM_THEME="}

	stdout, _, code := crmEnv(t, home, noEnv, "theme", "set", "nord", "-f", "json")
	require.Equal(t, 0, code)
	var set map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &set))
	assert.Equal(t, "nord", set["theme"])
	data, err := os.ReadFile(filepath.Join(home, ".crm", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `theme = "nord"`)

	stdout, _, code = crmEnv(t, home, noEnv, "theme", "show", "-f", "json")
	require.Equal(t, 0, code)
	var show struct {
		Name   string            `json:"name"`
		Active bool              `json:"active"`
		Source string            `json:"source"`
		Colors map[string]string `json:"colors"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &show))
	assert.Equal(t, "nord", show.Name)
	assert.True(t, show.Active)
	assert.Equal(t, "config", show.Source)
	assert.Equal(t, "#88C0D0", show.Colors["accent"])

	// CRM_THEME beats the saved choice.
	stdout, _, _ = crmEnv(t, home, []string{"CRM_THEME=gruvbox"}, "theme", "show", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(stdout), &show))
	assert.Equal(t, "gruvbox", show.Name)
	assert.Equal(t, "env", show.Source)

	stdout, _, code = crmEnv(t, home, noEnv, "theme", "path", "-f", "json")
	require.Equal(t, 0, code)
	var p struct {
		ThemesDir string `json:"themes_dir"`
		Exists    bool   `json:"exists"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &p))
	assert.Equal(t, filepath.Join(home, ".crm", "themes"), p.ThemesDir)
	assert.False(t, p.Exists)
}

func TestThemeUnknownNameIsNotFoundWithSuggestion(t *testing.T) {
	home := t.TempDir()
	_, stderr, code := crmEnv(t, home, nil, "theme", "set", "nrod")
	assert.Equal(t, 3, code)
	assert.Contains(t, stderr, `did you mean "nord"`)
	assert.Contains(t, stderr, "available:")
	_, err := os.Stat(filepath.Join(home, ".crm", "config.toml"))
	assert.True(t, os.IsNotExist(err), "nothing is saved for an unknown name")
}

func TestThemeInitWritesExampleThenCustomThemeWorks(t *testing.T) {
	home := t.TempDir()
	noEnv := []string{"CRM_THEME="}

	stdout, _, code := crmEnv(t, home, noEnv, "theme", "init", "mine", "-f", "json")
	require.Equal(t, 0, code)
	path := filepath.Join(home, ".crm", "themes", "mine.toml")
	assert.Contains(t, stdout, "mine.toml")
	_, err := os.Stat(path)
	require.NoError(t, err)

	// A second init refuses to overwrite.
	_, stderr, code := crmEnv(t, home, noEnv, "theme", "init", "mine")
	assert.Equal(t, 4, code)
	assert.Contains(t, stderr, "already exists")

	// Built-in names and bad names are rejected.
	_, stderr, code = crmEnv(t, home, noEnv, "theme", "init", "nord")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "built-in")
	_, _, code = crmEnv(t, home, noEnv, "theme", "init", "Bad Name")
	assert.Equal(t, 2, code)

	// The custom theme is listed, selectable and shown.
	stdout, _, _ = crmEnv(t, home, noEnv, "theme", "list", "-f", "json")
	assert.Contains(t, stdout, `"name":"mine"`)
	_, _, code = crmEnv(t, home, noEnv, "theme", "set", "mine")
	assert.Equal(t, 0, code)
	stdout, _, code = crmEnv(t, home, noEnv, "theme", "show", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, `"name":"mine"`)
	assert.Contains(t, stdout, `"base":"dark"`)

	// --dry-run on init writes nothing.
	_, _, code = crmEnv(t, home, noEnv, "theme", "init", "other", "--dry-run")
	assert.Equal(t, 0, code)
	_, err = os.Stat(filepath.Join(home, ".crm", "themes", "other.toml"))
	assert.True(t, os.IsNotExist(err))
}

func TestThemeBrokenFileIsListedAsInvalidWithUsefulError(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".crm", "themes")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "oops.toml"), []byte("base = \"dark\"\n[colors]\naccent = \"#12\"\n"), 0o644))

	stdout, _, code := crmEnv(t, home, []string{"CRM_THEME="}, "theme", "list", "-f", "json")
	require.Equal(t, 0, code, "a broken theme file does not break the list")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	var found map[string]any
	for _, r := range rows {
		if r["kind"] == "invalid" {
			found = r
		}
	}
	require.NotNil(t, found)
	assert.Contains(t, found["error"], "colors.accent")
	assert.Contains(t, found["error"], "oops.toml")
	assert.Contains(t, found["error"], "#rrggbb")
}

func TestThemeHelpTopicAndSchema(t *testing.T) {
	home := t.TempDir()
	stdout, _, code := crmEnv(t, home, nil, "help", "theme")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "See also")
	assert.Contains(t, stdout, "crm theme init")

	stdout, _, code = crmEnv(t, home, nil, "schema", "-f", "json")
	require.Equal(t, 0, code)
	for _, c := range []string{`"theme list"`, `"theme set"`, `"theme show"`, `"theme path"`, `"theme init"`} {
		assert.Contains(t, stdout, c)
	}
}
