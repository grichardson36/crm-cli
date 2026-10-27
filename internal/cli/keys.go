package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/tui"
	"github.com/spf13/cobra"
)

const keysListFields = "id,title,group,layer,scope,default,keys,custom,changes_data"

func registerKeysCommands(rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "List and rebind the keys of the terminal UI",
		Long: "Every action of 'crm tui' has default keys. Rebind one with 'crm keys set ACTION KEY' or in the TUI palette (Keys: customize).\n" +
			"Bindings live in the [keys] table of ~/.crm/config.toml. See 'crm help keys'.",
	}
	cmd.AddCommand(keysListCmd(), keysSetCmd(), keysResetCmd(), keysPathCmd())
	rootCmd.AddCommand(cmd)
}

type keysContext struct {
	dir       string
	overrides keys.Overrides
	problems  []string
}

func loadKeysContext() (*keysContext, error) {
	dir, err := defaultDataDir()
	if err != nil {
		return nil, err
	}
	o, problems := keys.Load(dir)
	return &keysContext{dir: dir, overrides: o, problems: problems}, nil
}

func keyList(ks []string) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = keys.Display(k)
	}
	return out
}

func keysListCmd() *cobra.Command {
	var group string
	var customOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every action with its default and effective keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadKeysContext()
			if err != nil {
				return err
			}
			eff, problems := tui.ResolveOverrides(c.overrides)
			problems = append(c.problems, problems...)
			var rows []map[string]any
			for _, a := range tui.DefaultActions() {
				if group != "" && !strings.EqualFold(a.Group, group) {
					continue
				}
				_, custom := c.overrides[a.ID]
				applied := custom && strings.Join(eff[a.ID], "\x00") == strings.Join(c.overrides[a.ID], "\x00")
				if customOnly && !custom {
					continue
				}
				rows = append(rows, map[string]any{
					"id": a.ID, "title": a.Title, "group": a.Group, "layer": a.Layer, "scope": a.Scope, "default": keyList(a.Keys),
					"keys": keyList(eff[a.ID]), "custom": applied, "changes_data": a.Write,
				})
			}
			for _, p := range problems {
				fmt.Fprintln(os.Stderr, "warning: "+p)
			}
			cols := []format.ColumnDef{{Header: "ID", Field: "id"}, {Header: "Action", Field: "title"}, {Header: "Group", Field: "group"},
				{Header: "Keys", Field: "keys"}, {Header: "Default", Field: "default"}, {Header: "Custom", Field: "custom"}}
			return format.Output(os.Stdout, resolveFormat(), rows, cols, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "only this group (Contact, Views, App, ...)")
	cmd.Flags().BoolVar(&customOnly, "custom", false, "only actions with a custom key")
	return cmd
}

// findAction returns the action info for an id, or a not-found error that suggests close ids.
func findAction(id string) (tui.ActionInfo, error) {
	all := tui.DefaultActions()
	var near []string
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
		if strings.Contains(a.ID, id) || strings.Contains(id, a.ID) {
			near = append(near, a.ID)
		}
	}
	sort.Strings(near)
	hint := ""
	if len(near) > 0 {
		if len(near) > 5 {
			near = near[:5]
		}
		hint = " (did you mean " + strings.Join(near, ", ") + "?)"
	}
	return tui.ActionInfo{}, model.NewExitError(model.ErrNotFound, "no action %q. 'crm keys list' shows the ids%s", id, hint)
}

func keysSetCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "set <action> <key>...",
		Short: "Bind an action to one or more keys (replaces its default keys)",
		Long: "Keys: a letter (a, T), a symbol (?, /), enter, esc, tab, shift+tab, space, up, down, left, right, home, end, pgup, pgdown, f1 to f12, ctrl+<letter>, alt+<key>.\n" +
			"A key already used by another action in the same context is refused and names that action.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := findAction(args[0])
			if err != nil {
				return err
			}
			var norm []string
			for _, k := range args[1:] {
				n, err := keys.Normalize(k)
				if err != nil {
					return model.NewExitError(model.ErrValidation, "%v", err)
				}
				norm = append(norm, n)
			}
			c, err := loadKeysContext()
			if err != nil {
				return err
			}
			if why := tui.ConflictFor(c.overrides, a.ID, norm); why != "" {
				return model.NewExitError(model.ErrConflict, "%s", why)
			}
			for _, k := range norm {
				if why := tui.SwallowedChordWarning(k); why != "" {
					fmt.Fprintf(os.Stderr, "warning: %s may never reach the program: %s. The binding is saved, pick another key if it does not work\n", keys.Display(k), why)
				}
			}
			c.overrides[a.ID] = norm
			if !dryRun {
				if err := keys.Save(c.dir, c.overrides); err != nil {
					return model.NewExitError(model.ErrValidation, "%v", err)
				}
			}
			return printKeysChange(a, keyList(a.Keys), keyList(norm), dryRun, !dryRun, keys.ConfigFile(c.dir))
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "check the binding and print it, and write nothing")
	return cmd
}

func keysResetCmd() *cobra.Command {
	var all, dryRun bool
	cmd := &cobra.Command{
		Use:   "reset [action]",
		Short: "Put one action (or every action with --all) back to its default keys",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadKeysContext()
			if err != nil {
				return err
			}
			path := keys.ConfigFile(c.dir)
			if len(args) == 0 {
				if !all {
					return model.NewExitError(model.ErrUsage, "name an action to reset, or pass --all")
				}
				if !dryRun {
					if err := keys.Save(c.dir, keys.Overrides{}); err != nil {
						return model.NewExitError(model.ErrValidation, "%v", err)
					}
				}
				return printKeysChange(tui.ActionInfo{ID: "*", Title: "all actions"}, nil, nil, dryRun, !dryRun, path)
			}
			a, err := findAction(args[0])
			if err != nil {
				return err
			}
			if _, ok := c.overrides[a.ID]; !ok {
				fmt.Fprintf(os.Stderr, "%s already uses its default keys\n", a.ID)
				return printKeysChange(a, keyList(a.Keys), keyList(a.Keys), dryRun, false, path)
			}
			delete(c.overrides, a.ID)
			if !dryRun {
				if err := keys.Save(c.dir, c.overrides); err != nil {
					return model.NewExitError(model.ErrValidation, "%v", err)
				}
			}
			return printKeysChange(a, keyList(a.Keys), keyList(a.Keys), dryRun, !dryRun, path)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "reset every action")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would change and write nothing")
	return cmd
}

func printKeysChange(a tui.ActionInfo, def, now []string, dry, saved bool, path string) error {
	if resolveFormat() == format.FormatJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"id": a.ID, "title": a.Title, "default": def, "keys": now, "saved": saved, "dry_run": dry, "config": path,
		})
	}
	switch {
	case dry:
		fmt.Fprintf(os.Stdout, "dry run: %s would use %s (nothing was written)\n", a.ID, strings.Join(now, ", "))
	case a.ID == "*":
		fmt.Fprintf(os.Stdout, "every action is back to its default keys (saved in %s)\n", path)
	default:
		fmt.Fprintf(os.Stdout, "%s: %s (saved in %s)\n", a.ID, strings.Join(now, ", "), path)
	}
	return nil
}

func keysPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the file that holds the key bindings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := defaultDataDir()
			if err != nil {
				return err
			}
			path := keys.ConfigFile(dir)
			_, statErr := os.Stat(path)
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"config": path, "exists": statErr == nil})
			}
			fmt.Fprintln(os.Stdout, path)
			return nil
		},
	}
}
