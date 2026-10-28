package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/spf13/cobra"
)

const (
	themeListFields = "name,kind,active,description,path,error"
	themeShowFields = "name,kind,active,source,description,path,base,resolved,colors"
)

func registerThemeCommands(rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "List, choose and create color themes for the terminal UI",
		Long: "Color themes for 'crm tui'. Built-in: auto, dark, light, high-contrast, catppuccin, nord, gruvbox, solarized.\n" +
			"Custom themes are TOML files in ~/.crm/themes. The choice is remembered per device in ~/.crm/config.toml.\n" +
			"Inside the TUI use the palette actions 'Theme: choose' (live preview) and 'Theme: reload'. See 'crm help theme'.",
	}
	cmd.AddCommand(themeListCmd(), themeSetCmd(), themeShowCmd(), themePathCmd(), themeInitCmd())
	rootCmd.AddCommand(cmd)
}

// themeContext is the data directory and the loaded catalog.
type themeContext struct {
	dir    string
	cat    theme.Catalog
	active string
	source string
}

func loadThemeContext() (*themeContext, error) {
	dir, err := defaultDataDir()
	if err != nil {
		return nil, err
	}
	name, source := theme.Chosen(dir, os.Getenv)
	dark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	return &themeContext{dir: dir, cat: theme.LoadCatalog(dir, dark), active: name, source: source}, nil
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

func themeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List built-in and custom themes, marking the active one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadThemeContext()
			if err != nil {
				return err
			}
			var rows []map[string]any
			for _, t := range c.cat.Themes {
				rows = append(rows, map[string]any{"id": t.Name, "name": t.Name, "kind": kindOf(t), "active": t.Name == c.active,
					"description": t.Description, "path": t.Path, "error": ""})
			}
			for _, p := range c.cat.Problems {
				rows = append(rows, map[string]any{"id": filepath.Base(p.File), "name": strings.TrimSuffix(filepath.Base(p.File), ".toml"),
					"kind": "invalid", "active": false, "description": "", "path": p.File, "error": p.Err})
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

func themeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show a theme's color roles (default: the active theme)",
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
			colors := map[string]string{}
			for _, rc := range t.Colors() {
				if rc.Color != "" {
					colors[rc.Role] = rc.Color
				}
			}
			resolved := ""
			if t.Name == theme.AutoName {
				resolved = "dark"
				if !lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
					resolved = "light"
				}
			}
			out := map[string]any{"name": t.Name, "kind": kindOf(t), "active": t.Name == c.active, "source": "",
				"description": t.Description, "path": t.Path, "base": t.Base, "resolved": resolved, "colors": colors}
			if t.Name == c.active {
				out["source"] = c.source
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(out)
			}
			w := colorprofile.NewWriter(os.Stdout, os.Environ()) // drops the swatch color when stdout is not a color terminal
			fmt.Fprintf(w, "name:        %s (%s)\n", t.Name, kindOf(t))
			fmt.Fprintf(w, "description: %s\n", t.Description)
			if t.Name == c.active {
				fmt.Fprintf(w, "active:      yes, chosen by %s\n", c.source)
			}
			if t.Path != "" {
				fmt.Fprintf(w, "file:        %s\n", t.Path)
			}
			if t.Base != "" {
				fmt.Fprintf(w, "base:        %s\n", t.Base)
			}
			if resolved != "" {
				fmt.Fprintf(w, "resolves to: %s\n", resolved)
			}
			// The swatch is drawn with the theme's own color. the colorprofile writer drops it when output is
			// not a terminal or NO_COLOR is set, leaving the plain value.
			for _, rc := range t.Colors() {
				if rc.Color == "" {
					fmt.Fprintf(w, "  %-8s (not set: reverse video)\n", rc.Role)
					continue
				}
				swatch := theme.Swatch(rc.Color)
				fmt.Fprintf(w, "  %-8s %s %s\n", rc.Role, swatch, rc.Color)
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
		Short: "Write an example custom theme to ~/.crm/themes/<name>.toml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.ToLower(strings.TrimSpace(args[0]))
			if err := theme.ValidName(name); err != nil {
				return model.NewExitError(model.ErrValidation, "%v", err)
			}
			for _, b := range theme.BuiltinNames() {
				if b == name {
					return model.NewExitError(model.ErrValidation, "%q is a built-in theme name, choose another (for example %s-custom)", name, name)
				}
			}
			dir, err := defaultDataDir()
			if err != nil {
				return err
			}
			td := theme.Dir(dir)
			path := filepath.Join(td, name+".toml")
			if _, err := os.Stat(path); err == nil {
				return model.NewExitError(model.ErrConflict, "%s already exists, edit it or pick another name", path)
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
			fmt.Fprintf(os.Stderr, "edit it, then run: crm theme set %s\n", name)
			return nil
		},
	}
}
