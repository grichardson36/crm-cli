package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	crmdoctor "github.com/jdanielnd/crm-cli/internal/doctor"
	"github.com/jdanielnd/crm-cli/internal/format"
	crmmcp "github.com/jdanielnd/crm-cli/internal/mcp"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/spf13/cobra"
)

func registerDoctorCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check the installation and say how to fix problems",
		Long: "Check binary version, data directory, database, integrity, schema version, write-ahead log, backups,\n" +
			"disk space, cloud-synced folders (OneDrive, Dropbox, iCloud), sync sources and MCP. Each check prints\n" +
			"ok, warn or fail with a one-line fix. Exit code 0 when every check is ok or warn, 1 when any check fails.\n" +
			"Read-only: the database is opened read-only.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			loc, err := resolveDBLocation()
			if err != nil {
				return err
			}
			path := loc.Path
			if abs, err := filepath.Abs(path); err == nil {
				path = abs
			}
			rep := crmdoctor.Run(cmd.Context(), crmdoctor.Options{
				DBPath:      path,
				Version:     Version,
				MCPTools:    len(crmmcp.ToolNames()),
				SyncSources: crmsync.DefaultRegistry(os.Getenv).Names(),
			})
			if err := printDoctor(rep); err != nil {
				return err
			}
			if !rep.OK {
				return fmt.Errorf("doctor: %d check(s) failed", rep.Summary[crmdoctor.Fail])
			}
			return nil
		},
	})
}

func printDoctor(rep crmdoctor.Report) error {
	switch resolveFormat() {
	case format.FormatJSON:
		return json.NewEncoder(os.Stdout).Encode(rep)
	case format.FormatTable:
		for _, c := range rep.Checks {
			fmt.Fprintf(os.Stdout, "%-4s  %-13s %s\n", c.Status, c.Name, c.Message)
			if c.Fix != "" {
				fmt.Fprintf(os.Stdout, "      fix: %s\n", c.Fix)
			}
		}
		fmt.Fprintf(os.Stdout, "\n%d ok, %d warn, %d fail\n", rep.Summary[crmdoctor.OK], rep.Summary[crmdoctor.Warn], rep.Summary[crmdoctor.Fail])
		return nil
	default:
		data := make([]map[string]any, len(rep.Checks))
		for i, c := range rep.Checks {
			data[i] = map[string]any{"name": c.Name, "status": c.Status, "message": c.Message, "fix": c.Fix}
		}
		return format.Output(os.Stdout, resolveFormat(), data, []format.ColumnDef{
			{Header: "Name", Field: "name"}, {Header: "Status", Field: "status"},
			{Header: "Message", Field: "message"}, {Header: "Fix", Field: "fix"}}, false)
	}
}
