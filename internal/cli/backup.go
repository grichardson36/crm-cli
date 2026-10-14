package cli

import (
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

func registerBackupCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "backup <dest.db>",
		Short: "Write a consistent, compacted copy of the database",
		Long:  "Writes a consistent snapshot of the database to dest.db using SQLite's VACUUM INTO. Safe to run while other crm commands are using the database. The destination must not exist.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dest := args[0]
			if _, err := os.Stat(dest); err == nil {
				return model.NewExitError(model.ErrConflict, "%s already exists", dest)
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			if _, err := db.ExecContext(cmd.Context(), "VACUUM INTO ?", dest); err != nil {
				return fmt.Errorf("backup to %s: %w", dest, err)
			}
			fmt.Fprintf(os.Stderr, "Wrote %s\n", dest)
			return nil
		},
	})
}
