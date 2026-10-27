package cli

import (
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/spf13/cobra"
)

var aliasColumns = []format.ColumnDef{
	{Header: "Alias", Field: "alias"},
	{Header: "Name", Field: "canonical"},
	{Header: "Source", Field: "source"},
	{Header: "Active", Field: "active"},
}

// registerAliasCommands adds `crm alias add|list|remove`: the shared given-name aliases (Zach and
// Zachary). They are read by the duplicate finder, person search, the command bar and the typeaheads.
func registerAliasCommands(rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage given-name aliases (Zach = Zachary, Rich = Richard)",
		Long: "Name aliases tell the duplicate finder, person search, the command bar and the relation typeahead that two given names are one name. " +
			"There is a built-in set and your additions. Turning off a built-in is a removal. Search expands a typed name through the aliases, so 'crm search Zach' finds Zachary.",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List the aliases in effect", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				db, err := openDB()
				if err != nil {
					return err
				}
				defer db.Close()
				list, err := repo.NewAliasRepo(db).List(cmd.Context())
				if err != nil {
					return err
				}
				rows := make([]map[string]any, len(list))
				for i, e := range list {
					src := "added"
					if e.Builtin {
						src = "built-in"
					}
					rows[i] = map[string]any{"alias": e.Alias, "canonical": e.Canonical, "source": src, "active": e.Active}
				}
				return format.Output(os.Stdout, resolveFormat(), rows, aliasColumns, flagQuiet)
			},
		},
		&cobra.Command{
			Use: "add <alias> <name>", Short: "Make a word another name for a given name", Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				db, err := openDB()
				if err != nil {
					return err
				}
				defer db.Close()
				changed, err := repo.NewAliasRepo(db).Add(cmd.Context(), args[0], args[1])
				if err != nil {
					return err
				}
				if changed {
					fmt.Fprintf(os.Stderr, "%s is now an alias of %s\n", args[0], args[1])
				} else {
					fmt.Fprintf(os.Stderr, "%s is already an alias of %s\n", args[0], args[1])
				}
				return nil
			},
		},
		&cobra.Command{
			Use: "remove <alias>", Short: "Stop treating a word as an alias", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				db, err := openDB()
				if err != nil {
					return err
				}
				defer db.Close()
				if err := repo.NewAliasRepo(db).Remove(cmd.Context(), args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "%s is no longer an alias\n", args[0])
				return nil
			},
		},
	)
	rootCmd.AddCommand(cmd)
}
