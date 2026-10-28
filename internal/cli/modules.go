package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/spf13/cobra"
)

const modulesFields = "order,id,name,on,position,key,default,description"

var modulesColumns = []format.ColumnDef{
	{Header: "Order", Field: "order"}, {Header: "Module", Field: "id"}, {Header: "On", Field: "on"},
	{Header: "Key", Field: "key"}, {Header: "Description", Field: "description"},
}

func registerModulesCommands(rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "modules",
		Short: "Turn views on or off and put them in order (per device)",
		Long: "Every view of the terminal UI is a module: " + "home, people, follow-up, tasks, organizations, birthdays, merge, archived, sync, deals, settings" + ".\n" +
			"Off hides a module from the tabs, the number keys, [ and ] and the palette, and lists it under Hidden in the Views screen (v). Its data and its links stay. Settings never turns off. Archived and Sync start off.\n" +
			"The arrangement is the [modules] table of ~/.crm/config.toml. See 'crm help modules'.",
	}
	cmd.AddCommand(modulesListCmd(), modulesSwitchCmd("on", true), modulesSwitchCmd("off", false), modulesMoveCmd(), modulesResetCmd())
	rootCmd.AddCommand(cmd)
}

func loadModules() (string, modules.Arrangement, error) {
	dir, err := defaultDataDir()
	if err != nil {
		return "", modules.Default(), err
	}
	a, problems := modules.Load(dir)
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "warning: "+p)
	}
	return dir, a, nil
}

func modulesOutput(a modules.Arrangement) error {
	rows := make([]map[string]any, 0, len(a.Rows()))
	for _, r := range a.Rows() {
		rows = append(rows, map[string]any{"order": r.Order, "id": r.ID, "name": r.Name, "on": r.On, "position": r.Position, "key": r.Key, "default": r.Default, "description": r.Blurb})
	}
	return format.Output(os.Stdout, resolveFormat(), rows, modulesColumns, flagQuiet)
}

func modulesError(err error) error {
	return model.NewExitError(model.ErrValidation, "%v", err)
}

func modulesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the modules in order with their state and number key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, a, err := loadModules()
			if err != nil {
				return err
			}
			return modulesOutput(a)
		},
	}
}

func modulesSwitchCmd(verb string, on bool) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " <module>...",
		Short: "Turn modules " + verb,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, a, err := loadModules()
			if err != nil {
				return err
			}
			for _, name := range args {
				if a, err = a.With(name, on); err != nil {
					return modulesError(err)
				}
			}
			if err := modules.Save(dir, a); err != nil {
				return err
			}
			return modulesOutput(a)
		},
	}
}

func modulesMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move <module> <position>",
		Short: "Put a module at a place in the order (1 is first)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pos, err := strconv.Atoi(args[1])
			if err != nil {
				return model.NewExitError(model.ErrValidation, "position must be a number, got %q", args[1])
			}
			dir, a, err := loadModules()
			if err != nil {
				return err
			}
			if a, err = a.Moved(args[0], pos); err != nil {
				return modulesError(err)
			}
			if err := modules.Save(dir, a); err != nil {
				return err
			}
			return modulesOutput(a)
		},
	}
}

func modulesResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Put the default arrangement back",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := loadModules()
			if err != nil {
				return err
			}
			if err := modules.Save(dir, modules.Default()); err != nil {
				return err
			}
			return modulesOutput(modules.Default())
		},
	}
}
