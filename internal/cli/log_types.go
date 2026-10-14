package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var noteTypeColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Name", Field: "name"},
	{Header: "Counts as contact", Field: "counts_as_contact"},
	{Header: "Archived", Field: "archived"},
}

func noteTypesToMaps(types []*model.NoteType) []map[string]any {
	out := make([]map[string]any, len(types))
	for i, n := range types {
		out[i] = map[string]any{
			"id":                n.ID,
			"uuid":              n.UUID,
			"name":              n.Name,
			"counts_as_contact": n.CountsAsContact,
			"archived":          n.Archived,
			"created_at":        n.CreatedAt,
			"updated_at":        n.UpdatedAt,
		}
	}
	return out
}

// logAddCmd is the generic path for any note type: crm log add --type standup 3 "text".
func logAddCmd() *cobra.Command {
	var typeName, subject, content, direction, at string

	cmd := &cobra.Command{
		Use:   "add --type <type> <person_id> [text...]",
		Short: "Log an interaction of any defined type",
		Long:  "Log an interaction of any active type from 'crm log types'. Text after the person ID becomes the content.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if typeName == "" {
				return model.NewExitError(model.ErrValidation, "--type is required (see 'crm log types')")
			}
			personID, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			text := strings.Join(args[1:], " ")
			if text != "" && content != "" {
				return model.NewExitError(model.ErrValidation, "give the text as arguments or --content, not both")
			}
			if text == "" {
				text = content
			}
			if direction != "" && !model.ValidInteractionDirection(direction) {
				return model.NewExitError(model.ErrValidation, "invalid direction: %s (must be one of: %s)", direction, strings.Join(model.InteractionDirections, ", "))
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			input := model.CreateInteractionInput{
				Type:      typeName,
				Subject:   nilIfEmpty(subject),
				Content:   nilIfEmpty(text),
				Direction: nilIfEmpty(direction),
				PersonIDs: []int64{personID},
			}
			if at != "" {
				input.OccurredAt = &at
			}
			interaction, err := repo.NewInteractionRepo(db).Create(cmd.Context(), input)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{interactionToMap(interaction)}, interactionColumns, flagQuiet)
		},
	}

	cmd.Flags().StringVar(&typeName, "type", "", "interaction type (see 'crm log types')")
	cmd.Flags().StringVar(&subject, "subject", "", "subject/title")
	cmd.Flags().StringVar(&content, "content", "", "content/body")
	cmd.Flags().StringVar(&direction, "direction", "", "direction: inbound or outbound")
	cmd.Flags().StringVar(&at, "at", "", "when the interaction occurred (ISO 8601)")
	return cmd
}

func logTypesCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "types",
		Short: "List, add, or archive interaction types",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			types, err := repo.NewNoteTypeRepo(db).List(cmd.Context(), all)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), noteTypesToMaps(types), noteTypeColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include archived types")

	var countsAsContact bool
	addCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add an interaction type (restores it if archived)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			nt, err := repo.NewNoteTypeRepo(db).Add(cmd.Context(), args[0], countsAsContact)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Added interaction type %q\n", nt.Name)
			return format.Output(os.Stdout, resolveFormat(), noteTypesToMaps([]*model.NoteType{nt}), noteTypeColumns, flagQuiet)
		},
	}
	addCmd.Flags().BoolVar(&countsAsContact, "counts-as-contact", true, "whether logging this type resets the keep-in-touch timer")

	rmCmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Archive an interaction type (existing interactions keep it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := repo.NewNoteTypeRepo(db).Archive(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Archived interaction type %q\n", args[0])
			return nil
		},
	}

	cmd.AddCommand(addCmd, rmCmd)
	return cmd
}
