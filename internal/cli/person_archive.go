package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// forEachPerson runs fn for every ID argument. It keeps going after a failure and
// returns the first error once all IDs have been tried.
func forEachPerson(cmd *cobra.Command, args []string, verb string, fn func(ctx context.Context, r *repo.PersonRepo, id int64) error) error {
	ids := make([]int64, 0, len(args))
	for _, a := range args {
		id, err := parsePersonID(a)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
		var first error
		for _, id := range ids {
			if err := fn(cmd.Context(), r, id); err != nil {
				// The root command prints the returned (first) error.
				if first == nil {
					first = err
				} else {
					fmt.Fprintf(os.Stderr, "crm: error: %v\n", err)
				}
				continue
			}
			fmt.Fprintf(os.Stderr, "%s person #%d\n", verb, id)
		}
		return first
	})
}

func personArchiveCmd(use, short, verb string, hidden bool) *cobra.Command {
	return &cobra.Command{
		Use:    use,
		Short:  short,
		Args:   cobra.MinimumNArgs(1),
		Hidden: hidden,
		RunE: func(cmd *cobra.Command, args []string) error {
			return forEachPerson(cmd, args, verb, func(ctx context.Context, r *repo.PersonRepo, id int64) error {
				return r.Archive(ctx, id)
			})
		},
	}
}

func personRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <id>...",
		Short: "Restore archived people",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return forEachPerson(cmd, args, "Restored", func(ctx context.Context, r *repo.PersonRepo, id int64) error {
				return r.Restore(ctx, id)
			})
		},
	}
}

func personPurgeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "purge <id>... --yes",
		Short: "Permanently delete archived people (cannot be undone)",
		Long: "Permanently deletes already archived people, their contact methods, relationships, tags and\n" +
			"custom fields, and any interactions or tasks that involve only them. Records a tombstone for sync.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return model.NewExitError(model.ErrValidation, "purge is permanent: pass --yes to confirm")
			}
			return forEachPerson(cmd, args, "Purged", func(ctx context.Context, r *repo.PersonRepo, id int64) error {
				return r.Purge(ctx, id)
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm permanent deletion")
	return cmd
}
