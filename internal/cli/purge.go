package cli

import (
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// registerPurgeCommand adds `crm purge --archived`, the bulk form of `crm person purge`, for every record type. It is a dry
// run unless --apply is given.
func registerPurgeCommand(rootCmd *cobra.Command) {
	var archived, apply, yes bool
	var typ string
	var ids []int64
	cmd := &cobra.Command{
		Use:   "purge --archived [--type T] [--ids 1,2,3] [--apply]",
		Short: "Permanently delete archived records in bulk (dry run unless --apply)",
		Long: "Permanently deletes archived records of any type (person, organization, deal, task, note), each one in its own transaction with its dependent rows removed and a tombstone recorded for sync. " +
			"Without --type every archived record of every type is purged. --ids needs a type and defaults to person. This is a dry run that lists what would go, pass --apply to delete. It cannot be undone.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apply = apply || yes
			if !archived {
				return model.NewExitError(model.ErrValidation, "pass --archived: bulk purge only removes archived records")
			}
			typ = repo.NormalizePurgeKind(typ)
			if typ != "" && !repo.IsPurgeKind(typ) {
				return model.NewExitError(model.ErrValidation, "unknown type %q, use person, organization, deal, task or note", typ)
			}
			if len(ids) > 0 && typ == "" {
				typ = "person"
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			return func() error {
				res, err := repo.PurgeArchived(cmd.Context(), db, repo.PurgeArchivedOptions{Kind: typ, IDs: ids, Apply: apply})
				if err != nil {
					return err
				}
				failed := map[string]bool{}
				for _, e := range res.Errors {
					failed[fmt.Sprintf("%s:%d", e.Kind, e.ID)] = true
				}
				rows := make([]map[string]any, 0, len(res.Items)+len(res.Skipped))
				for _, p := range res.Items {
					status := "would purge"
					switch {
					case apply && failed[fmt.Sprintf("%s:%d", p.Kind, p.ID)]:
						status = "error"
					case apply:
						status = "purged"
					}
					rows = append(rows, map[string]any{"type": p.Kind, "id": p.ID, "name": p.Name, "status": status})
				}
				for _, sk := range res.Skipped {
					rows = append(rows, map[string]any{"type": sk.Kind, "id": sk.ID, "name": "", "status": "skipped: " + sk.Reason})
				}
				cols := []format.ColumnDef{{Header: "Type", Field: "type"}, {Header: "ID", Field: "id"}, {Header: "Name", Field: "name"}, {Header: "Status", Field: "status"}}
				if err := format.Output(os.Stdout, resolveFormat(), rows, cols, flagQuiet); err != nil {
					return err
				}
				switch {
				case !apply:
					fmt.Fprintf(os.Stderr, "Dry run: %d archived records would be purged. Pass --apply to delete them.\n", res.Count)
				default:
					fmt.Fprintf(os.Stderr, "Purged %d of %d archived people\n", res.Purged, res.Count)
				}
				if len(res.Errors) > 0 {
					return model.NewExitError(model.ErrDatabase, "%d records could not be purged, first: %s", len(res.Errors), res.Errors[0].Reason)
				}
				return nil
			}()
		},
	}
	cmd.Flags().BoolVar(&archived, "archived", false, "purge archived records (required)")
	cmd.Flags().StringVar(&typ, "type", "", "record type: person, organization, deal, task or note (default: every type)")
	cmd.Flags().Int64SliceVar(&ids, "ids", nil, "only these archived records of --type (default: all of them)")
	cmd.Flags().BoolVar(&apply, "apply", false, "delete for real (default is a dry run)")
	cmd.Flags().BoolVar(&yes, "yes", false, "same as --apply")
	rootCmd.AddCommand(cmd)
}
