package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// paletteFile is one palette table of a theme file.
type paletteFile struct {
	Colors map[string]string `toml:"colors"`
}

// file is the TOML layout of a custom theme. Both palettes are complete, there is no inheritance.
//
//	name = "Sunset"          # optional, the file name is used without it
//	description = "..."      # optional
//	copied_from = "nord"     # optional: the theme shift+r resets to
//	[dark.colors]
//	fg = "#d8dee9"
//	[light.colors]
//	fg = "#2e3440"
type file struct {
	Name        string      `toml:"name"`
	Description string      `toml:"description"`
	CopiedFrom  string      `toml:"copied_from"`
	Dark        paletteFile `toml:"dark"`
	Light       paletteFile `toml:"light"`
}

// Dir returns the custom theme directory inside the data directory.
func Dir(dataDir string) string { return filepath.Join(dataDir, "themes") }

// Parse reads a custom theme. stem is the file name without .toml and names the theme when the file
// has no name key. path is only used in messages. Errors name the file and the key.
func Parse(stem, path string, data []byte) (Theme, error) {
	fail := func(format string, a ...any) (Theme, error) {
		return Theme{}, fmt.Errorf("theme %q (%s): %s", stem, path, fmt.Sprintf(format, a...))
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
	for _, tbl := range []struct {
		name string
		p    paletteFile
	}{{"dark", f.Dark}, {"light", f.Light}} {
		for role := range tbl.p.Colors {
			if !known[role] {
				msg := fmt.Sprintf("unknown color role %q in %s.colors", role, tbl.name)
				if sg := suggest(role, Roles); sg != "" {
					msg += fmt.Sprintf(", did you mean %q", sg)
				}
				return fail("%s (roles: %s)", msg, strings.Join(Roles, ", "))
			}
		}
	}
	if u := md.Undecoded(); len(u) > 0 {
		var keys []string
		for _, k := range u {
			keys = append(keys, k.String())
		}
		return fail("unknown key %s (allowed: name, description, copied_from, [dark.colors], [light.colors])", strings.Join(keys, ", "))
	}
	name := strings.TrimSpace(f.Name)
	if name == "" {
		name = stem
	}
	if err := ValidName(name); err != nil {
		return fail("%v", err)
	}
	for _, b := range BuiltinNames() {
		if strings.EqualFold(b, name) {
			return fail("the name is taken by a built-in theme, choose another (try %s-custom)", name)
		}
	}
	t := Theme{Name: name, Description: strings.TrimSpace(f.Description), Path: path, CopiedFrom: strings.TrimSpace(f.CopiedFrom)}
	for role, v := range f.Dark.Colors {
		t.Dark.SetColor(role, strings.TrimSpace(v))
	}
	for role, v := range f.Light.Colors {
		t.Light.SetColor(role, strings.TrimSpace(v))
	}
	if err := t.Validate(); err != nil {
		return fail("%v", err)
	}
	return t, nil
}

// Marshal writes a theme as the TOML file format, both palettes in role order.
func Marshal(t Theme) []byte {
	var b strings.Builder
	b.WriteString("# Custom crm theme. Colors are \"#rrggbb\", \"#rgb\" or an ANSI number 0-255.\n")
	fmt.Fprintf(&b, "name = %q\n", t.Name)
	if t.Description != "" {
		fmt.Fprintf(&b, "description = %q\n", t.Description)
	}
	if t.CopiedFrom != "" {
		fmt.Fprintf(&b, "copied_from = %q\n", t.CopiedFrom)
	}
	for _, p := range []struct {
		table string
		pal   Palette
	}{{"dark", t.Dark}, {"light", t.Light}} {
		fmt.Fprintf(&b, "\n[%s.colors]\n", p.table)
		for _, r := range Roles {
			fmt.Fprintf(&b, "%s = %q\n", r, p.pal.Color(r))
		}
	}
	return []byte(b.String())
}

// Example returns the text `crm theme init` writes: the default colors as a complete two-table theme.
func Example(name string) []byte {
	base, _ := Builtin(DefaultName)
	base.Name, base.Builtin, base.CopiedFrom, base.Description = name, false, DefaultName, ""
	return Marshal(base)
}

// Slug turns a theme name into a file name stem: lower case letters, digits, '-' and '_'.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "theme"
	}
	return s
}

// Save writes a custom theme. A theme without a path gets a new file named from its name. It returns
// the theme with its path set.
func Save(dataDir string, t Theme) (Theme, error) {
	if t.Builtin {
		return t, fmt.Errorf("theme %q is built in and cannot be changed", t.Name)
	}
	if err := ValidName(t.Name); err != nil {
		return t, err
	}
	if err := t.Validate(); err != nil {
		return t, err
	}
	if t.Path == "" {
		dir := Dir(dataDir)
		stem := Slug(t.Name)
		for i := 2; ; i++ {
			p := filepath.Join(dir, stem+".toml")
			if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
				t.Path = p
				break
			}
			stem = fmt.Sprintf("%s-%d", Slug(t.Name), i)
		}
	}
	if err := os.MkdirAll(filepath.Dir(t.Path), 0o700); err != nil {
		return t, fmt.Errorf("create %s: %w", filepath.Dir(t.Path), err)
	}
	if err := os.WriteFile(t.Path, Marshal(t), 0o644); err != nil {
		return t, fmt.Errorf("write %s: %w", t.Path, err)
	}
	return t, nil
}

// Remove deletes the file of a custom theme.
func Remove(t Theme) error {
	if t.Builtin || t.Path == "" {
		return fmt.Errorf("theme %q is built in and cannot be deleted", t.Name)
	}
	if err := os.Remove(t.Path); err != nil {
		return fmt.Errorf("delete %s: %w", t.Path, err)
	}
	return nil
}

// Problem is a custom theme file that could not be loaded.
type Problem struct {
	File string `json:"file"`
	Err  string `json:"error"`
}

// LoadDir reads every *.toml file in dir. A missing directory is not an error.
func LoadDir(dir string) ([]Theme, []Problem) {
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
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		data, err := os.ReadFile(path)
		if err != nil {
			probs = append(probs, Problem{File: path, Err: err.Error()})
			continue
		}
		t, err := Parse(stem, path, data)
		if err != nil {
			probs = append(probs, Problem{File: path, Err: err.Error()})
			continue
		}
		if key := strings.ToLower(t.Name); seen[key] {
			probs = append(probs, Problem{File: path, Err: fmt.Sprintf("duplicate theme name %q", t.Name)})
			continue
		} else {
			seen[key] = true
		}
		themes = append(themes, t)
	}
	SortThemes(themes)
	return themes, probs
}

// Catalog is every theme available, sorted by name, plus files that failed to load.
type Catalog struct {
	Themes   []Theme
	Problems []Problem
}

// LoadCatalog lists built-in and custom themes.
func LoadCatalog(dataDir string) Catalog {
	c := Catalog{Themes: Builtins()}
	custom, probs := LoadDir(Dir(dataDir))
	c.Themes = append(c.Themes, custom...)
	SortThemes(c.Themes)
	c.Problems = probs
	return c
}

// Names lists the theme names in order.
func (c Catalog) Names() []string {
	var names []string
	for _, t := range c.Themes {
		names = append(names, t.Name)
	}
	return names
}

// Has reports whether a theme with the name exists, ignoring case.
func (c Catalog) Has(name string) bool {
	_, err := c.Find(name)
	return err == nil
}

// Find returns a theme by name, ignoring case. The error lists the names that exist.
func (c Catalog) Find(name string) (Theme, error) {
	name = strings.TrimSpace(name)
	for _, t := range c.Themes {
		if strings.EqualFold(t.Name, name) {
			return t, nil
		}
	}
	names := c.Names()
	msg := fmt.Sprintf("no theme named %q", name)
	if s := suggest(name, names); s != "" {
		msg += fmt.Sprintf(", did you mean %q", s)
	}
	return Theme{}, fmt.Errorf("%s (available: %s)", msg, strings.Join(names, ", "))
}

// NewName is the name of a new theme: "New theme", then "New theme 2" and so on.
func (c Catalog) NewName() string {
	name := "New theme"
	for i := 2; c.Has(name); i++ {
		name = fmt.Sprintf("New theme %d", i)
	}
	return name
}

// CopyName is the name of a copy of the named theme: "<name> - Copy1", then Copy2 and so on.
func (c Catalog) CopyName(name string) string {
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s - Copy%d", name, i)
		if !c.Has(n) {
			return n
		}
	}
}

// --- remembered choice ---

// ConfigFile is the per-device settings file in the data directory.
func ConfigFile(dataDir string) string { return filepath.Join(dataDir, "config.toml") }

// EnvName overrides the remembered choice for one process (for example the web terminal).
const EnvName = "CRM_THEME"

// Chosen returns the theme name to use and where it came from: "env", "config" or "default".
func Chosen(dataDir string, getenv func(string) string) (name, source string) {
	if v := strings.TrimSpace(getenv(EnvName)); v != "" {
		return v, "env"
	}
	if v := strings.TrimSpace(readConfig(dataDir)["theme"]); v != "" {
		return v, "config"
	}
	return DefaultName, "default"
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
func SaveChoice(dataDir, name string) error { return saveKey(dataDir, "theme", name) }

// saveKey stores one string setting of config.toml and keeps the others.
func saveKey(dataDir, key, value string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}
	path := ConfigFile(dataDir)
	raw := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &raw); err != nil {
			return fmt.Errorf("%s is not valid TOML (%v), fix or remove it before changing %s", path, err, key)
		}
	}
	raw[key] = value
	var b strings.Builder
	b.WriteString("# crm settings for this device. Written by `crm theme set`.\n")
	if err := toml.NewEncoder(&b).Encode(raw); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
