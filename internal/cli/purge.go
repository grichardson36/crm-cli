package cli

import (
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// registerPurgeCommand adds `crm purge --archived`, the bulk form of `crm person purge`. It is a dry
// run unless --apply is given.
func registerPurgeCommand(rootCmd *cobra.Command) {
	var archived, apply, yes bool
	var typ string
	var ids []int64
	cmd := &cobra.Command{
		Use:   "purge --archived [--type person] [--ids 1,2,3] [--apply]",
		Short: "Permanently delete archived records in bulk (dry run unless --apply)",
		Long: "Permanently deletes archived people, each one exactly as 'crm person purge' does (dependent rows removed, a tombstone recorded for sync). " +
			"Without --ids every archived person in the type is purged. Only people can be purged. This is a dry run that lists who would go, pass --apply to delete. It cannot be undone.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apply = apply || yes
			if !archived {
				return model.NewExitError(model.ErrValidation, "pass --archived: bulk purge only removes archived records")
			}
			switch typ {
			case "", "person", "people":
				typ = "person"
			default:
				return model.NewExitError(model.ErrValidation, "only archived people can be purged, not %q", typ)
			}
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				res, err := r.PurgeArchived(cmd.Context(), repo.PurgeArchivedOptions{Kind: typ, IDs: ids, Apply: apply})
				if err != nil {
					return err
				}
				failed := map[int64]bool{}
				for _, e := range res.Errors {
					failed[e.ID] = true
				}
				rows := make([]map[string]any, 0, len(res.People)+len(res.Skipped))
				for _, p := range res.People {
					status := "would purge"
					switch {
					case apply && failed[p.ID]:
						status = "error"
					case apply:
						status = "purged"
					}
					rows = append(rows, map[string]any{"id": p.ID, "name": p.Name, "status": status})
				}
				for _, sk := range res.Skipped {
					rows = append(rows, map[string]any{"id": sk.ID, "name": "", "status": "skipped: " + sk.Reason})
				}
				cols := []format.ColumnDef{{Header: "ID", Field: "id"}, {Header: "Name", Field: "name"}, {Header: "Status", Field: "status"}}
				if err := format.Output(os.Stdout, resolveFormat(), rows, cols, flagQuiet); err != nil {
					return err
				}
				switch {
				case !apply:
					fmt.Fprintf(os.Stderr, "Dry run: %d archived people would be purged. Pass --apply to delete them.\n", res.Count)
				default:
					fmt.Fprintf(os.Stderr, "Purged %d of %d archived people\n", res.Purged, res.Count)
				}
				if len(res.Errors) > 0 {
					return model.NewExitError(model.ErrDatabase, "%d people could not be purged, first: %s", len(res.Errors), res.Errors[0].Reason)
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&archived, "archived", false, "purge archived records (required)")
	cmd.Flags().StringVar(&typ, "type", "", "record type: person (the only type with a purge)")
	cmd.Flags().Int64SliceVar(&ids, "ids", nil, "only these archived people (default: all of them)")
	cmd.Flags().BoolVar(&apply, "apply", false, "delete for real (default is a dry run)")
	cmd.Flags().BoolVar(&yes, "yes", false, "same as --apply")
	rootCmd.AddCommand(cmd)
}
