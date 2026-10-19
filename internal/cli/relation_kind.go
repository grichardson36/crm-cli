package cli

import (
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var relationKindColumns = []format.ColumnDef{
	{Header: "Type", Field: "object_type"},
	{Header: "Name", Field: "name"},
	{Header: "Inverse", Field: "inverse_name"},
	{Header: "Mutual", Field: "symmetric"},
	{Header: "Built-in", Field: "builtin"},
}

const relationKindFields = "id,uuid,object_type,name,inverse_name,symmetric,builtin,created_at,updated_at"

func kindToMap(k *model.RelationKind) map[string]any {
	return map[string]any{
		"id": k.ID, "uuid": k.UUID, "object_type": k.ObjectType, "name": k.Name, "inverse_name": k.InverseName,
		"symmetric": k.Symmetric, "builtin": k.Builtin, "created_at": k.CreatedAt, "updated_at": k.UpdatedAt,
	}
}

// objectTypeOf turns the --type flag (person, org, organization) into the stored object type.
func objectTypeOf(s string) (string, error) {
	switch s {
	case "person", "people":
		return "person", nil
	case "org", "organization":
		return "organization", nil
	}
	return "", model.NewExitError(model.ErrValidation, "type must be person or org, not %q", s)
}

// registerRelationCommands adds `crm relation-kind` and `crm roles`.
func registerRelationCommands(rootCmd *cobra.Command) {
	kind := &cobra.Command{
		Use:   "relation-kind",
		Short: "Manage the kinds of person and organization links (friend, parent, manager, ...)",
		Long: "A link kind names how two people (or two organizations) are related. 'a is parent b' also reads 'b is child a'. A kind without an inverse is mutual (friend). " +
			"Built-in kinds cannot be removed. See 'crm help relations'.",
	}
	kind.AddCommand(relationKindListCmd(), relationKindAddCmd(), relationKindRmCmd())
	rootCmd.AddCommand(kind)
	rootCmd.AddCommand(rolesCmd())
}

func relationKindListCmd() *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List link kinds",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			objType := ""
			if typ != "" {
				var err error
				if objType, err = objectTypeOf(typ); err != nil {
					return err
				}
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			kinds, err := repo.NewKindRepo(db).List(cmd.Context(), objType)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(kinds))
			for i, k := range kinds {
				rows[i] = kindToMap(k)
			}
			return format.Output(os.Stdout, resolveFormat(), rows, relationKindColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "only kinds for person or org links")
	return cmd
}

func relationKindAddCmd() *cobra.Command {
	var typ, inverse string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a link kind, mutual or with an inverse name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			objType, err := objectTypeOf(typ)
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			k, err := repo.NewKindRepo(db).Add(cmd.Context(), objType, args[0], inverse)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{kindToMap(k)}, relationKindColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "person", "person or org")
	cmd.Flags().StringVar(&inverse, "inverse", "", "name seen from the other end (leave out for a mutual kind)")
	return cmd
}

func relationKindRmCmd() *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a user-defined link kind that no link uses",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			objType, err := objectTypeOf(typ)
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := repo.NewKindRepo(db).Remove(cmd.Context(), objType, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Removed %s kind %q\n", objType, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "person", "person or org")
	return cmd
}

// rolesCmd lists the roles already in use, for suggestions.
func rolesCmd() *cobra.Command {
	var relation string
	cmd := &cobra.Command{
		Use:   "roles",
		Short: "List the roles already used on links, most used first (roles are free text)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			roles, err := repo.NewLinkRepo(db).Roles(cmd.Context(), relation, 50)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(roles))
			for i, r := range roles {
				rows[i] = map[string]any{"role": r}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{{Header: "Role", Field: "role"}}, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&relation, "relation", "membership", "membership, deal_person or deal_org")
	return cmd
}
