package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

func parseInteractionID(arg string) (int64, error) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id <= 0 {
		return 0, model.NewExitError(model.ErrValidation, "invalid interaction ID: %s", arg)
	}
	return id, nil
}

// normalizeOccurredAt accepts RFC 3339, "YYYY-MM-DD HH:MM[:SS]" or "YYYY-MM-DD".
func normalizeOccurredAt(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format("2006-01-02 15:04:05"), nil
		}
	}
	return "", model.NewExitError(model.ErrValidation, "invalid date %q (use YYYY-MM-DD or an ISO 8601 timestamp)", s)
}

func logEditCmd() *cobra.Command {
	var typeName, subject, content, direction, date string

	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a logged interaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInteractionID(args[0])
			if err != nil {
				return err
			}
			var in model.UpdateInteractionInput
			f := cmd.Flags()
			if f.Changed("type") {
				in.Type = &typeName
			}
			if f.Changed("subject") {
				in.Subject = &subject
			}
			if f.Changed("content") {
				in.Content = &content
			}
			if f.Changed("direction") {
				in.Direction = &direction
			}
			if f.Changed("date") {
				d, err := normalizeOccurredAt(date)
				if err != nil {
					return err
				}
				in.OccurredAt = &d
			}
			if in == (model.UpdateInteractionInput{}) {
				return model.NewExitError(model.ErrValidation, "nothing to change: give --type, --content, --subject, --direction or --date")
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			updated, err := repo.NewInteractionRepo(db).Update(cmd.Context(), id, in)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{interactionToMap(updated)}, interactionColumns, flagQuiet)
		},
	}

	cmd.Flags().StringVar(&typeName, "type", "", "interaction type (see 'crm log types')")
	cmd.Flags().StringVar(&subject, "subject", "", "subject/title (empty clears)")
	cmd.Flags().StringVar(&content, "content", "", "content/body (empty clears)")
	cmd.Flags().StringVar(&direction, "direction", "", "inbound or outbound (empty clears)")
	cmd.Flags().StringVar(&date, "date", "", "when it occurred (YYYY-MM-DD or ISO 8601)")
	return cmd
}

func logRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete (archive) a logged interaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInteractionID(args[0])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := repo.NewInteractionRepo(db).Archive(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Deleted interaction #%d\n", id)
			return nil
		},
	}
}
