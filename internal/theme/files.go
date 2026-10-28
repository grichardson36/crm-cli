package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// file is the TOML layout of a custom theme.
//
//	description = "..."
//	base = "nord"          # optional built-in to start from
//	[colors]
//	accent = "#ff8800"
type file struct {
	Description string            `toml:"description"`
	Base        string            `toml:"base"`
	Colors      map[string]string `toml:"colors"`
}

// Dir returns the custom theme directory inside the data directory.
func Dir(dataDir string) string { return filepath.Join(dataDir, "themes") }

// Parse reads a custom theme. name comes from the file name. path is only used in messages.
// Errors name the file and the key, and say how to fix them.
func Parse(name, path string, data []byte, darkBackground bool) (Theme, error) {
	fail := func(format string, a ...any) (Theme, error) {
		return Theme{}, fmt.Errorf("theme %q (%s): %s", name, path, fmt.Sprintf(format, a...))
	}
	if err := ValidName(name); err != nil {
		return fail("%v (rename the file)", err)
	}
	for _, b := range BuiltinNames() {
		if b == name {
			return fail("the name is taken by a built-in theme, rename the file (try %s-custom.toml)", name)
		}
	}
	var f file
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return fail("not valid TOML: %v", err)
	}
	known := map[string]bool{}
	for _, r := range Roles {
		known[r] = true
	}
	var unknown []string
	for _, k := range md.Undecoded() {
		unknown = append(unknown, k.String())
	}
	for role := range f.Colors {
		if !known[role] {
			msg := fmt.Sprintf("unknown color role %q", role)
			if s := suggest(role, Roles); s != "" {
				msg += fmt.Sprintf(", did you mean %q", s)
			}
			return fail("%s (roles: %s)", msg, strings.Join(Roles, ", "))
		}
	}
	if len(unknown) > 0 {
		return fail("unknown key %s (allowed: description, base, [colors])", strings.Join(unknown, ", "))
	}

	t := Theme{Name: name, Description: f.Description, Path: path, Base: f.Base}
	if f.Base != "" {
		base, ok := Builtin(f.Base, darkBackground)
		if !ok {
			msg := fmt.Sprintf("base %q is not a built-in theme", f.Base)
			if s := suggest(f.Base, BuiltinNames()); s != "" {
				msg += fmt.Sprintf(", did you mean %q", s)
			}
			return fail("%s (built-ins: %s)", msg, strings.Join(BuiltinNames(), ", "))
		}
		t = base
		t.Name, t.Builtin, t.Path, t.Base = name, false, path, f.Base
		t.Description = f.Description
	}
	for role, v := range f.Colors {
		*t.slot(role) = strings.TrimSpace(v)
	}
	if err := t.Validate(); err != nil {
		hint := ""
		if f.Base == "" {
			hint = ` (add base = "dark" to inherit missing roles)`
		}
		return fail("%v%s", err, hint)
	}
	if t.Description == "" {
		t.Description = "custom theme"
	}
	return t, nil
}

// Problem is a custom theme file that could not be loaded.
type Problem struct {
	File string `json:"file"`
	Err  string `json:"error"`
}

// LoadDir reads every *.toml file in dir. A missing directory is not an error.
func LoadDir(dir string, darkBackground bool) ([]Theme, []Problem) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []Problem{{File: dir, Err: err.Error()}}
	}
	var themes []Theme
	var probs []Problem
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".toml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		name := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		data, err := os.ReadFile(path)
		if err != nil {
			probs = append(probs, Problem{File: path, Err: err.Error()})
			continue
		}
		t, err := Parse(name, path, data, darkBackground)
		if err != nil {
			probs = append(probs, Problem{File: path, Err: err.Error()})
			continue
		}
		if seen[name] {
			probs = append(probs, Problem{File: path, Err: fmt.Sprintf("duplicate theme name %q", name)})
			continue
		}
		seen[name] = true
		themes = append(themes, t)
	}
	sort.Slice(themes, func(i, j int) bool { return themes[i].Name < themes[j].Name })
	return themes, probs
}

// Catalog is every theme available: built-ins then custom ones, plus files that failed to load.
type Catalog struct {
	Themes   []Theme
	Problems []Problem
}

// LoadCatalog lists built-in and custom themes.
func LoadCatalog(dataDir string, darkBackground bool) Catalog {
	c := Catalog{}
	for _, b := range Builtins() {
		t, _ := Builtin(b.Name, darkBackground)
		c.Themes = append(c.Themes, t)
	}
	custom, probs := LoadDir(Dir(dataDir), darkBackground)
	c.Themes = append(c.Themes, custom...)
	c.Problems = probs
	return c
}

// Find returns a theme by name. The error lists the names that exist.
func (c Catalog) Find(name string) (Theme, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, t := range c.Themes {
		if t.Name == name {
			return t, nil
		}
	}
	var names []string
	for _, t := range c.Themes {
		names = append(names, t.Name)
	}
	msg := fmt.Sprintf("no theme named %q", name)
	if s := suggest(name, names); s != "" {
		msg += fmt.Sprintf(", did you mean %q", s)
	}
	return Theme{}, fmt.Errorf("%s (available: %s)", msg, strings.Join(names, ", "))
}

// Example returns the text `crm theme init` writes: a complete theme with every role.
func Example(name string) []byte {
	base, _ := Builtin("dark", true)
	var b strings.Builder
	fmt.Fprintf(&b, "# Custom crm theme. Save as ~/.crm/themes/%s.toml, then pick it with: crm theme set %s\n", name, name)
	b.WriteString("# Colors are \"#rrggbb\", \"#rgb\" or an ANSI number 0-255. Terminals without truecolor get the nearest color.\n")
	b.WriteString("# Remove a line to inherit it from the base theme. Delete \"base\" to define every required role yourself.\n")
	b.WriteString("description = \"my custom theme\"\n")
	b.WriteString("base = \"dark\"\n\n[colors]\n")
	notes := map[string]string{
		RoleFg: "headings and strong text", RoleDim: "secondary text and hints", RoleAccent: "names, key hints",
		RoleDanger: "errors and destructive prompts", RoleGood: "success messages", RoleWarn: "cautions, READ-ONLY badge",
		RoleMark: "marked rows", RoleTabFg: "active tab text", RoleTabBg: "active tab background",
		RoleSelFg: "selected row text (optional, set with sel_bg)", RoleSelBg: "selected row background (optional)",
		RoleKindPerson: "people in the related segment (optional)", RoleKindOrg: "organizations (optional)", RoleKindDeal: "deals (optional)",
		RoleKindTask: "tasks (optional)", RoleKindNote: "notes (optional)", RoleKindTag: "tags (optional)",
	}
	for _, r := range Roles {
		v := base.Color(r)
		if v == "" {
			fmt.Fprintf(&b, "# %s = \"#334455\"  # %s\n", r, notes[r])
			continue
		}
		fmt.Fprintf(&b, "%s = %q  # %s\n", r, v, notes[r])
	}
	return []byte(b.String())
}

// --- remembered choice ---

// ConfigFile is the per-device settings file in the data directory.
func ConfigFile(dataDir string) string { return filepath.Join(dataDir, "config.toml") }

// EnvName overrides the remembered choice for one process (for example the web terminal).
const EnvName = "CRM_THEME"

// Chosen returns the theme name to use and where it came from: "env", "config" or "default".
func Chosen(dataDir string, getenv func(string) string) (name, source string) {
	if v := strings.TrimSpace(getenv(EnvName)); v != "" {
		return strings.ToLower(v), "env"
	}
	if v := readConfig(dataDir)["theme"]; v != "" {
		return strings.ToLower(v), "config"
	}
	return AutoName, "default"
}

func readConfig(dataDir string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(ConfigFile(dataDir))
	if err != nil {
		return out
	}
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return out
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// SaveChoice remembers the theme for this device. Other string settings in the file are kept.
func SaveChoice(dataDir, name string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}
	path := ConfigFile(dataDir)
	raw := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &raw); err != nil {
			return fmt.Errorf("%s is not valid TOML (%v), fix or remove it before changing the theme", path, err)
		}
	}
	raw["theme"] = name
	var b strings.Builder
	b.WriteString("# crm settings for this device. Written by `crm theme set`.\n")
	if err := toml.NewEncoder(&b).Encode(raw); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
