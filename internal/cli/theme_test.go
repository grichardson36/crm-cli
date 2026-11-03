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

func TestThemeListShowsEachThemeOnceWithBothPalettes(t *testing.T) {
	home := t.TempDir()
	stdout, _, code := crmEnv(t, home, []string{"CRM_THEME="}, "theme", "list", "-f", "json")
	require.Equal(t, 0, code)
	var rows []struct {
		Name, Kind string
		Active     bool
		Dark       map[string]string
		Light      map[string]string
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
		assert.Equal(t, "builtin", r.Kind)
		assert.Equal(t, r.Name == "default", r.Active, "default is the default")
		assert.Len(t, r.Dark, 18, r.Name)
		assert.Len(t, r.Light, 18, r.Name)
		assert.NotEqual(t, r.Dark["bg"], r.Light["bg"], r.Name)
	}
	assert.Equal(t, []string{"anthropocene", "catppuccin", "default", "gruvbox", "high-contrast", "nord", "solarized"}, names)
}

func TestThemeSetShowModePathAndPersistence(t *testing.T) {
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
		Mode   string            `json:"mode"`
		Dark   map[string]string `json:"dark"`
		Light  map[string]string `json:"light"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &show))
	assert.Equal(t, "nord", show.Name)
	assert.True(t, show.Active)
	assert.Equal(t, "config", show.Source)
	assert.Equal(t, "auto", show.Mode)
	assert.Equal(t, "#88C0D0", show.Dark["accent"])
	assert.Equal(t, "#5E81AC", show.Light["accent"])

	// The mode is saved next to the theme.
	stdout, _, code = crmEnv(t, home, noEnv, "theme", "mode", "dark", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, `"mode":"dark"`)
	data, err = os.ReadFile(filepath.Join(home, ".crm", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `mode = "dark"`)
	assert.Contains(t, string(data), `theme = "nord"`, "the mode keeps the theme")
	stdout, _, _ = crmEnv(t, home, noEnv, "theme", "show", "-f", "json")
	require.NoError(t, json.Unmarshal([]byte(stdout), &show))
	assert.Equal(t, "dark", show.Mode)
	_, stderr, code := crmEnv(t, home, noEnv, "theme", "mode", "sepia")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "auto, light or dark")
	_, _, code = crmEnv(t, home, noEnv, "theme", "mode", "auto")
	assert.Equal(t, 0, code)

	// CRM_THEME beats the saved choice, and matching ignores case.
	stdout, _, _ = crmEnv(t, home, []string{"CRM_THEME=Gruvbox"}, "theme", "show", "-f", "json")
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

	for _, old := range []string{"auto", "dark", "light", "anthropocene-dark"} {
		_, _, code = crmEnv(t, home, nil, "theme", "set", old)
		assert.Equal(t, 3, code, "%s is not a theme name any more", old)
	}
}

func TestThemeUnknownNameInConfigFallsBackToDefault(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".crm"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".crm", "config.toml"), []byte("theme = \"auto\"\n"), 0o644))
	stdout, _, code := crmEnv(t, home, []string{"CRM_THEME="}, "theme", "show", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, `"name":"default"`)
	assert.Contains(t, stdout, `"source":"fallback"`)
}

func TestThemeInitWritesTwoTableTemplateThenCustomThemeWorks(t *testing.T) {
	home := t.TempDir()
	noEnv := []string{"CRM_THEME="}

	stdout, _, code := crmEnv(t, home, noEnv, "theme", "init", "mine", "-f", "json")
	require.Equal(t, 0, code)
	path := filepath.Join(home, ".crm", "themes", "mine.toml")
	assert.Contains(t, stdout, "mine.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[dark.colors]")
	assert.Contains(t, string(data), "[light.colors]")
	assert.Contains(t, string(data), `copied_from = "default"`)

	// A second init refuses to overwrite.
	_, stderr, code := crmEnv(t, home, noEnv, "theme", "init", "mine")
	assert.Equal(t, 4, code)
	assert.Contains(t, stderr, "already exists")

	// Built-in names and bad names are rejected.
	_, stderr, code = crmEnv(t, home, noEnv, "theme", "init", "Nord")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "built-in")
	_, _, code = crmEnv(t, home, noEnv, "theme", "init", "")
	assert.Equal(t, 2, code)

	// The custom theme is listed once, selectable and shown.
	stdout, _, _ = crmEnv(t, home, noEnv, "theme", "list", "-f", "json")
	assert.Contains(t, stdout, `"name":"mine"`)
	assert.Contains(t, stdout, `"copied_from":"default"`)
	_, _, code = crmEnv(t, home, noEnv, "theme", "set", "mine")
	assert.Equal(t, 0, code)
	stdout, _, code = crmEnv(t, home, noEnv, "theme", "show", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, `"name":"mine"`)

	// A name with spaces gets a file name made from it.
	_, _, code = crmEnv(t, home, noEnv, "theme", "init", "Warm Sand")
	assert.Equal(t, 0, code)
	_, err = os.Stat(filepath.Join(home, ".crm", "themes", "warm-sand.toml"))
	assert.NoError(t, err)
	_, _, code = crmEnv(t, home, noEnv, "theme", "set", "warm sand")
	assert.Equal(t, 0, code, "names match ignoring case")

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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "oops.toml"), []byte("[dark.colors]\naccent = \"#12\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "old.toml"), []byte("base = \"dark\"\n[colors]\naccent = \"#123456\"\n"), 0o644))

	stdout, _, code := crmEnv(t, home, []string{"CRM_THEME="}, "theme", "list", "-f", "json")
	require.Equal(t, 0, code, "a broken theme file does not break the list")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	errs := map[string]string{}
	for _, r := range rows {
		if r["kind"] == "invalid" {
			errs[r["name"].(string)], _ = r["error"].(string)
		}
	}
	require.Len(t, errs, 2, "the single table format is not read")
	assert.Contains(t, errs["oops"], "dark.colors.accent")
	assert.Contains(t, errs["oops"], "oops.toml")
	assert.Contains(t, errs["oops"], "#rrggbb")
	assert.Contains(t, errs["old"], "unknown key")
}

func TestThemeHelpTopicAndSchema(t *testing.T) {
	home := t.TempDir()
	stdout, _, code := crmEnv(t, home, nil, "help", "theme")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "See also")
	assert.Contains(t, stdout, "crm theme init")
	assert.Contains(t, stdout, "crm theme mode")

	stdout, _, code = crmEnv(t, home, nil, "schema", "-f", "json")
	require.Equal(t, 0, code)
	for _, c := range []string{`"theme list"`, `"theme set"`, `"theme mode"`, `"theme show"`, `"theme path"`, `"theme init"`} {
		assert.Contains(t, stdout, c)
	}
}
