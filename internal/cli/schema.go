package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/help"
	crmmcp "github.com/jdanielnd/crm-cli/internal/mcp"
	"github.com/jdanielnd/crm-cli/internal/schema"
	"github.com/spf13/cobra"
)

// buildSchema describes the command tree below root.
func buildSchema(root *cobra.Command) schema.Manifest {
	return schema.Build(root, schema.Options{
		Version:    Version,
		MCPTools:   crmmcp.ToolNames(),
		HelpTopics: help.Names(),
	})
}

func registerSchemaCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "schema",
		Short: "Print every command, flag, example, exit code and MCP tool",
		Long: "Print the machine-readable manifest of the CLI: every command with its path, description, arguments, flags\n" +
			"(name, type, default, usage), examples, whether it changes data (mutates), whether it cannot be undone\n" +
			"(destructive), the JSON output fields, the exit code table, the JSON error format and the MCP tool names.\n" +
			"An agent runs this once and knows the whole surface. Use -f json for the full manifest.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := buildSchema(cmd.Root())
			if resolveFormat() == format.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(m)
			}
			data := make([]map[string]any, len(m.Commands))
			for i, c := range m.Commands {
				data[i] = map[string]any{"path": c.Path, "mutates": c.Mutates, "destructive": c.Destructive, "short": c.Short, "id": c.Path}
			}
			if err := format.Output(os.Stdout, resolveFormat(), data, []format.ColumnDef{
				{Header: "Command", Field: "path"}, {Header: "Mutates", Field: "mutates"},
				{Header: "Destructive", Field: "destructive"}, {Header: "Description", Field: "short"}}, flagQuiet); err != nil {
				return err
			}
			if resolveFormat() == format.FormatTable {
				fmt.Fprintln(os.Stderr, "Use -f json for flags, examples, exit codes and MCP tools.")
			}
			return nil
		},
	})
}
