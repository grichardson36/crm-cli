package cli

import (
	"path/filepath"

	crmmcp "github.com/jdanielnd/crm-cli/internal/mcp"
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
			env := tui.Env{Version: Version, MCPTools: len(crmmcp.ToolNames())}
			if loc, err := resolveDBLocation(); err == nil {
				env.DBPath, env.Source = loc.Path, loc.Source
				if abs, err := filepath.Abs(loc.Path); err == nil {
					env.DBPath = abs
				}
			}
			if dir, err := defaultDataDir(); err == nil {
				env.DataDir = dir
			}
			return tui.Run(cmd.Context(), db, readOnly, env)
		},
	}
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "browse only: disable the keys that change data")
	rootCmd.AddCommand(cmd)
}
