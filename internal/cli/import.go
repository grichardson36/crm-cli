package cli

import (
	"encoding/json"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/importer"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

func registerImportCommand(rootCmd *cobra.Command) {
	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import data from other tools",
	}

	var contacts, notes string
	var dryRun bool
	var skipTags []string
	dex := &cobra.Command{
		Use:   "dex",
		Short: "Import a Dex CSV export (contacts, and optionally notes)",
		Long:  "Import the contacts and notes CSV files exported from Dex (Settings > Import/Export). Safe to re-run: records already imported are skipped.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if contacts == "" {
				return model.NewExitError(model.ErrValidation, "--contacts is required")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			rep, err := importer.ImportDex(cmd.Context(), db, contacts, notes, importer.DexOptions{DryRun: dryRun, SkipTags: skipTags})
			if err != nil {
				return err
			}
			out, _ := json.MarshalIndent(rep, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			if dryRun {
				fmt.Fprintln(cmd.ErrOrStderr(), "dry run: nothing was written")
			}
			return nil
		},
	}
	dex.Flags().StringVar(&contacts, "contacts", "", "path to dex_contacts_*.csv")
	dex.Flags().StringVar(&notes, "notes", "", "path to dex_notes_*.csv")
	dex.Flags().StringSliceVar(&skipTags, "skip-tag", nil, "group/tag name to leave out (repeatable)")
	dex.Flags().BoolVar(&dryRun, "dry-run", false, "count what would be imported without writing")

	importCmd.AddCommand(dex)
	rootCmd.AddCommand(importCmd)
}
