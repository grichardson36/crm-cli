package cli

import (
	"github.com/jdanielnd/crm-cli/internal/tui"
	"github.com/spf13/cobra"
)

func registerTUICommand(rootCmd *cobra.Command) {
	var readOnly bool
	cmd := &cobra.Command{
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
			return tui.Run(cmd.Context(), db, readOnly)
		},
	}
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "browse only: disable the keys that change data")
	rootCmd.AddCommand(cmd)
}
