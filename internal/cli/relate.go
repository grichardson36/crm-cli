package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// person relate, relationships and unrelate are the older names of `crm person link person`,
// `crm person show` (links) and `crm person unlink person`. They call the same core.

var relationshipColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Person", Field: "person_id"},
	{Header: "Related", Field: "related_person_id"},
	{Header: "Type", Field: "type"},
	{Header: "Notes", Field: "notes"},
}

func registerRelateCommands(personCmd *cobra.Command) {
	personCmd.AddCommand(personRelateCmd())
	personCmd.AddCommand(personRelationshipsCmd())
	personCmd.AddCommand(personUnrelateCmd())
}

// relateDeprecated prints the pointer from an older relate name to the link verb (stderr).
func relateDeprecated(old, use string) {
	fmt.Fprintf(os.Stderr, "crm: %s is deprecated, use: %s\n", old, use)
}

func personRelateCmd() *cobra.Command {
	var relType, notes string

	cmd := &cobra.Command{
		Use:   "relate <person_id> <related_person_id>",
		Short: "Deprecated, use 'person link person' (creates a relationship between two people)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			relateDeprecated("person relate", "crm person link person <id> <id> --kind <kind>")
			personID, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			relatedID, err := parsePersonID(args[1])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			attrs := model.LinkAttrs{Kind: &relType, Notes: nilIfEmpty(notes)}
			res, err := repo.NewLinkRepo(db).Link(cmd.Context(), repo.Ref{Kind: "person", ID: personID}, repo.Ref{Kind: "person", ID: relatedID}, attrs)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Related person #%d to person #%d as %q (link %d)\n", personID, relatedID, relType, res.LinkID)
			return nil
		},
	}

	cmd.Flags().StringVar(&relType, "type", model.DefaultPersonKind, "relationship kind, see `crm relation-kind list`")
	cmd.Flags().StringVar(&notes, "notes", "", "notes about the relationship")

	return cmd
}

func personRelationshipsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "relationships <person_id>",
		Aliases: []string{"rels"},
		Short:   "Deprecated, use 'person show' (lists the people related to a person)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			relateDeprecated("person relationships", "crm person show <id>")
			personID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid person ID: %s", args[0])
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			items, err := repo.NewLinkRepo(db).Linked(cmd.Context(), repo.Ref{Kind: "person", ID: personID}, "person")
			if err != nil {
				return err
			}
			data := make([]map[string]any, len(items))
			for i, li := range items {
				m := map[string]any{
					"id":                li.LinkID,
					"person_id":         personID,
					"related_person_id": li.ID,
					"name":              li.Name,
					"type":              li.Label,
				}
				if li.Notes != nil {
					m["notes"] = *li.Notes
				}
				data[i] = m
			}
			return format.Output(os.Stdout, resolveFormat(), data, relationshipColumns, flagQuiet)
		},
	}
}

func personUnrelateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unrelate <link_id>",
		Short: "Deprecated, use 'person unlink person --id' (removes a relationship by its link id)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			relateDeprecated("person unrelate", "crm person unlink person --id <link id>")
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid relationship ID: %s", args[0])
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			lr := repo.NewLinkRepo(db)
			rec, err := lr.Get(cmd.Context(), "person_relation", id)
			if err != nil || rec.Deleted {
				return model.NewExitError(model.ErrNotFound, "relationship %d not found", id)
			}
			if _, err := lr.Unlink(cmd.Context(), repo.Ref{Kind: "person", ID: rec.A}, repo.Ref{Kind: "person", ID: rec.B}, repo.UnlinkOptions{LinkID: id}); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Removed relationship #%d\n", id)
			return nil
		},
	}
}
