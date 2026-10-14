package cli

import (
	"github.com/jdanielnd/crm-cli/internal/tui"
	"github.com/spf13/cobra"
)

func registerTUICommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "tui",
		Short: "Browse and update contacts in a full-screen terminal UI",
		Long:  "A keyboard-first terminal UI (mouse optional). Press ? inside for the key list.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			return tui.Run(cmd.Context(), db)
		},
	})
}
