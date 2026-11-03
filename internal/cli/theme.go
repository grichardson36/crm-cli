package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/spf13/cobra"
)

const (
	themeListFields = "name,kind,active,description,path,copied_from,dark,light,error"
	themeShowFields = "name,kind,active,source,mode,description,path,copied_from,dark,light"
)

func registerThemeCommands(rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "List, choose and create color themes for the terminal UI",
		Long: "Color themes for 'crm tui'. A theme is one name with a dark and a light palette. Built-in: anthropocene, catppuccin, default, gruvbox, high-contrast, nord, solarized.\n" +
			"The mode (auto, light or dark) picks the palette: crm theme mode. Custom themes are TOML files in ~/.crm/themes. The choice is remembered per device in ~/.crm/config.toml.\n" +
			"Inside the TUI use Settings > Theme. See 'crm help theme'.",
	}
	cmd.AddCommand(themeListCmd(), themeSetCmd(), themeModeCmd(), themeShowCmd(), themePathCmd(), themeInitCmd())
	rootCmd.AddCommand(cmd)
}

// themeContext is the data directory and the loaded catalog.
type themeContext struct {
	dir    string
	cat    theme.Catalog
	active string
	source string
	mode   theme.Mode
}

func loadThemeContext() (*themeContext, error) {
	dir, err := defaultDataDir()
	if err != nil {
		return nil, err
	}
	name, source := theme.Chosen(dir, os.Getenv)
	cat := theme.LoadCatalog(dir)
	if t, err := cat.Find(name); err == nil {
		name = t.Name
	} else {
		name, source = theme.DefaultName, "fallback"
	}
	return &themeContext{dir: dir, cat: cat, active: name, source: source, mode: theme.ChosenMode(dir)}, nil
}

func (c *themeContext) find(name string) (theme.Theme, error) {
	t, err := c.cat.Find(name)
	if err != nil {
		return t, model.NewExitError(model.ErrNotFound, "%v", err)
	}
	return t, nil
}

func kindOf(t theme.Theme) string {
	if t.Builtin {
		return "builtin"
	}
	return "custom"
}

// themeRow is a theme as one record: its name once, with both palettes.
func (c *themeContext) themeRow(t theme.Theme) map[string]any {
	return map[string]any{"id": t.Name, "name": t.Name, "kind": kindOf(t), "active": t.Name == c.active,
		"description": t.Description, "path": t.Path, "copied_from": t.CopiedFrom,
		"dark": t.Dark.Map(), "light": t.Light.Map(), "error": ""}
}

func themeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List built-in and custom themes once each, with both palettes, marking the active one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadThemeContext()
			if err != nil {
				return err
			}
			var rows []map[string]any
			for _, t := range c.cat.Themes {
				rows = append(rows, c.themeRow(t))
			}
			for _, p := range c.cat.Problems {
				rows = append(rows, map[string]any{"id": filepath.Base(p.File), "name": strings.TrimSuffix(filepath.Base(p.File), ".toml"),
					"kind": "invalid", "active": false, "description": "", "path": p.File, "copied_from": "", "error": p.Err})
			}
			if resolveFormat() != format.FormatJSON {
				for _, p := range c.cat.Problems {
					fmt.Fprintf(os.Stderr, "warning: %s\n", p.Err)
				}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{
				{Header: "Name", Field: "name"}, {Header: "Kind", Field: "kind"}, {Header: "Active", Field: "active"},
				{Header: "Description", Field: "description"}}, flagQuiet)
		},
	}
}

func themeSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name>",
		Short: "Choose the theme for this device (used by crm tui)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadThemeContext()
			if err != nil {
				return err
			}
			t, err := c.find(args[0])
			if err != nil {
				return err
			}
			if err := theme.SaveChoice(c.dir, t.Name); err != nil {
				return model.NewExitError(model.ErrValidation, "%v", err)
			}
			if v := strings.TrimSpace(os.Getenv(theme.EnvName)); v != "" && !strings.EqualFold(v, t.Name) {
				fmt.Fprintf(os.Stderr, "note: %s=%s is set and overrides the saved choice in this shell\n", theme.EnvName, v)
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"theme": t.Name, "saved": true, "config": theme.ConfigFile(c.dir)})
			}
			fmt.Fprintf(os.Stdout, "theme: %s (saved in %s)\n", t.Name, theme.ConfigFile(c.dir))
			return nil
		},
	}
}

func themeModeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mode <auto|light|dark>",
		Short: "Choose the mode for this device: auto follows the terminal, light and dark paint the whole screen",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			md, err := theme.ParseMode(args[0])
			if err != nil || strings.TrimSpace(args[0]) == "" {
				return model.NewExitError(model.ErrValidation, "mode %q is not valid, use auto, light or dark", args[0])
			}
			dir, err := defaultDataDir()
			if err != nil {
				return err
			}
			if err := theme.SaveMode(dir, md); err != nil {
				return model.NewExitError(model.ErrValidation, "%v", err)
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"mode": string(md), "saved": true, "config": theme.ConfigFile(dir)})
			}
			fmt.Fprintf(os.Stdout, "mode: %s (saved in %s)\n", md, theme.ConfigFile(dir))
			return nil
		},
	}
}

func themeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show a theme's color roles in both palettes (default: the active theme)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadThemeContext()
			if err != nil {
				return err
			}
			name := c.active
			if len(args) == 1 {
				name = args[0]
			}
			t, err := c.find(name)
			if err != nil {
				return err
			}
			out := c.themeRow(t)
			delete(out, "id")
			delete(out, "error")
			out["source"] = ""
			out["mode"] = string(c.mode)
			if t.Name == c.active {
				out["source"] = c.source
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(out)
			}
			w := colorprofile.NewWriter(os.Stdout, os.Environ()) // drops the swatch color when stdout is not a color terminal
			fmt.Fprintf(w, "name:        %s (%s)\n", t.Name, kindOf(t))
			if t.Description != "" {
				fmt.Fprintf(w, "description: %s\n", t.Description)
			}
			if t.Name == c.active {
				fmt.Fprintf(w, "active:      yes, chosen by %s\n", c.source)
			}
			fmt.Fprintf(w, "mode:        %s\n", c.mode)
			if t.Path != "" {
				fmt.Fprintf(w, "file:        %s\n", t.Path)
			}
			if t.CopiedFrom != "" {
				fmt.Fprintf(w, "copied from: %s\n", t.CopiedFrom)
			}
			// The swatch is drawn with the theme's own color. the colorprofile writer drops it when output is
			// not a terminal or NO_COLOR is set, leaving the plain value.
			for _, p := range []struct {
				name string
				pal  theme.Palette
			}{{"dark", t.Dark}, {"light", t.Light}} {
				fmt.Fprintf(w, "\n[%s]\n", p.name)
				for _, rc := range p.pal.Colors() {
					fmt.Fprintf(w, "  %-12s %s %s\n", rc.Role, theme.Swatch(rc.Color), rc.Color)
				}
			}
			return nil
		},
	}
}

func themePathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the custom themes directory and the settings file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := defaultDataDir()
			if err != nil {
				return err
			}
			td := theme.Dir(dir)
			_, statErr := os.Stat(td)
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"themes_dir": td, "exists": statErr == nil, "config": theme.ConfigFile(dir)})
			}
			fmt.Fprintln(os.Stdout, td)
			if flagVerbose {
				fmt.Fprintf(os.Stderr, "settings file: %s\n", theme.ConfigFile(dir))
			}
			return nil
		},
	}
}

func themeInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <name>",
		Short: "Write a custom theme template (a dark and a light palette) to ~/.crm/themes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if err := theme.ValidName(name); err != nil {
				return model.NewExitError(model.ErrValidation, "%v", err)
			}
			for _, b := range theme.BuiltinNames() {
				if strings.EqualFold(b, name) {
					return model.NewExitError(model.ErrValidation, "%q is a built-in theme name, choose another (for example %s-custom)", name, name)
				}
			}
			dir, err := defaultDataDir()
			if err != nil {
				return err
			}
			td := theme.Dir(dir)
			path := filepath.Join(td, theme.Slug(name)+".toml")
			if _, err := os.Stat(path); err == nil {
				return model.NewExitError(model.ErrConflict, "%s already exists, edit it or pick another name", path)
			}
			if theme.LoadCatalog(dir).Has(name) {
				return model.NewExitError(model.ErrConflict, "a theme named %q already exists, pick another name", name)
			}
			if err := os.MkdirAll(td, 0o700); err != nil {
				return fmt.Errorf("create %s: %w", td, err)
			}
			if err := os.WriteFile(path, theme.Example(name), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"name": name, "path": path, "created": true})
			}
			fmt.Fprintf(os.Stdout, "%s\n", path)
			fmt.Fprintf(os.Stderr, "edit it, then run: crm theme set %q\n", name)
			return nil
		},
	}
}
