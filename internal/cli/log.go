package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/spf13/cobra"
)

var interactionColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Type", Field: "type"},
	{Header: "Subject", Field: "subject"},
	{Header: "People", Field: "person_ids"},
	{Header: "Orgs", Field: "org_ids"},
	{Header: "Date", Field: "occurred_at"},
	{Header: "Direction", Field: "direction"},
}

func interactionToMap(i *model.Interaction) map[string]any {
	m := map[string]any{
		"id":          i.ID,
		"uuid":        i.UUID,
		"type":        i.Type,
		"occurred_at": i.OccurredAt,
		"person_ids":  i.PersonIDs,
		"org_ids":     nonNilIDs(i.OrgIDs),
		"deal_ids":    nonNilIDs(i.DealIDs),
		"created_at":  i.CreatedAt,
		"updated_at":  i.UpdatedAt,
	}
	if i.Subject != nil {
		m["subject"] = *i.Subject
	}
	if i.Content != nil {
		m["content"] = *i.Content
	}
	if i.Direction != nil {
		m["direction"] = *i.Direction
	}
	return m
}

func interactionsToMaps(interactions []*model.Interaction) []map[string]any {
	result := make([]map[string]any, len(interactions))
	for i, inter := range interactions {
		result[i] = interactionToMap(inter)
	}
	return result
}

func registerLogCommands(rootCmd *cobra.Command) {
	logCmd := &cobra.Command{
		Use:   "log",
		Short: "Log interactions (calls, emails, meetings, notes, messages)",
	}

	for _, iType := range model.InteractionTypes {
		logCmd.AddCommand(logTypeCmd(iType))
	}

	logCmd.AddCommand(logAddCmd())
	logCmd.AddCommand(logTypesCmd())
	logCmd.AddCommand(logListCmd())
	logCmd.AddCommand(logEditCmd())
	logCmd.AddCommand(logRmCmd())

	rootCmd.AddCommand(logCmd)
}

func logTypeCmd(interactionType string) *cobra.Command {
	var subject, content, direction, at string
	var orgIDs, dealIDs []int64

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <person_id> [person_id...]", interactionType),
		Short: fmt.Sprintf("Log a %s", interactionType),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var personIDs []int64
			for _, arg := range args {
				id, err := strconv.ParseInt(arg, 10, 64)
				if err != nil {
					return model.NewExitError(model.ErrValidation, "invalid person ID: %s", arg)
				}
				personIDs = append(personIDs, id)
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()


			input := model.CreateInteractionInput{
				Type:      interactionType,
				Subject:   nilIfEmpty(subject),
				Content:   nilIfEmpty(content),
				Direction: nilIfEmpty(direction),
				PersonIDs: personIDs,
				OrgIDs:    orgIDs,
				DealIDs:   dealIDs,
			}
			if at != "" {
				input.OccurredAt = &at
			}

			r := repo.NewInteractionRepo(db)
			reportHints(cmd, validate.InteractionCreate(&input))
			interaction, err := r.Create(cmd.Context(), input)
			if err != nil {
				return err
			}

			data := []map[string]any{interactionToMap(interaction)}
			return format.Output(os.Stdout, resolveFormat(), data, interactionColumns, flagQuiet)
		},
	}

	cmd.Flags().StringVar(&subject, "subject", "", "subject/title")
	cmd.Flags().StringVar(&content, "content", "", "content/body")
	cmd.Flags().StringVar(&direction, "direction", "", "direction: inbound or outbound")
	cmd.Flags().StringVar(&at, "at", "", "when the interaction occurred (ISO 8601)")
	cmd.Flags().Int64SliceVar(&orgIDs, "org", nil, "also attach it to this organization (repeatable)")
	cmd.Flags().Int64SliceVar(&dealIDs, "deal", nil, "also attach it to this deal (repeatable)")

	return cmd
}

func logListCmd() *cobra.Command {
	var personID, orgID, dealID int64
	var iType string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List interactions",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			filters := model.InteractionFilters{Limit: limit}
			if cmd.Flags().Changed("person") {
				filters.PersonID = &personID
			}
			if cmd.Flags().Changed("org") {
				filters.OrgID = &orgID
			}
			if cmd.Flags().Changed("deal") {
				filters.DealID = &dealID
			}
			if iType != "" {
				filters.Type = &iType
			}

			r := repo.NewInteractionRepo(db)
			interactions, err := r.FindAll(cmd.Context(), filters)
			if err != nil {
				return err
			}

			return format.Output(os.Stdout, resolveFormat(), interactionsToMaps(interactions), interactionColumns, flagQuiet)
		},
	}

	cmd.Flags().Int64Var(&personID, "person", 0, "filter by person ID")
	cmd.Flags().Int64Var(&orgID, "org", 0, "filter by organization ID")
	cmd.Flags().Int64Var(&dealID, "deal", 0, "filter by deal ID")
	cmd.Flags().StringVar(&iType, "type", "", "filter by interaction type")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results")

	return cmd
}
