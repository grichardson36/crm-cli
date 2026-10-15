package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/jdanielnd/crm-cli/internal/cal/icsio"
	"github.com/jdanielnd/crm-cli/internal/cal/input"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var icsImportColumns = []format.ColumnDef{
	{Header: "Kind", Field: "kind"},
	{Header: "Created", Field: "created"},
	{Header: "Updated", Field: "updated"},
	{Header: "Unchanged", Field: "unchanged"},
	{Header: "Skipped", Field: "skipped"},
}

func registerCalICSCommands(calCmd *cobra.Command) {
	calCmd.AddCommand(calImportCmd())
	calCmd.AddCommand(calExportCmd())
}

func calImportCmd() *cobra.Command {
	var source, tzName string
	var dryRun, noTasks bool

	cmd := &cobra.Command{
		Use:   "import <file.ics | ->",
		Short: "Import events and tasks from an iCalendar file; a second import updates instead of duplicating",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var data []byte
			var err error
			if args[0] == "-" {
				data, err = io.ReadAll(os.Stdin)
			} else {
				data, err = os.ReadFile(args[0])
			}
			if err != nil {
				return model.NewExitError(model.ErrValidation, "read %s: %v", args[0], err)
			}
			loc, err := displayZone(tzName)
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := icsio.Import(cmd.Context(), db, data, icsio.ImportOptions{Source: source, DryRun: dryRun, Loc: loc, SkipTasks: noTasks})
			if err != nil {
				return err
			}
			for _, w := range res.Warnings {
				fmt.Fprintln(os.Stderr, "crm: warning:", w)
			}
			if resolveFormat() == format.FormatJSON {
				return format.Output(os.Stdout, format.FormatJSON, []map[string]any{toMap(res)}, nil, flagQuiet)
			}
			rows := []map[string]any{}
			for _, k := range []struct {
				kind string
				c    icsio.Counts
			}{{"events", res.Events}, {"overrides", res.Overrides}, {"tasks", res.Todos}} {
				rows = append(rows, map[string]any{"kind": k.kind, "created": k.c.Created, "updated": k.c.Updated, "unchanged": k.c.Unchanged, "skipped": k.c.Skipped})
			}
			if dryRun {
				fmt.Fprintln(os.Stderr, "dry run: nothing was written")
			}
			return format.Output(os.Stdout, resolveFormat(), rows, icsImportColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&source, "source", icsio.DefaultSource, "name the file's UIDs are remembered under (use a different name per calendar feed)")
	cmd.Flags().StringVar(&tzName, "tz", "", "zone for times written without a zone (default: this device)")
	cmd.Flags().BoolVar(&noTasks, "no-tasks", false, "skip VTODO tasks")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change and write nothing")
	return cmd
}

func calExportCmd() *cobra.Command {
	var from, to, out, name, source string
	var days int
	var tasks bool

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export events (and optionally tasks) as an iCalendar file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opt := icsio.ExportOptions{Tasks: tasks, Name: name, Source: source}
			if from != "" || to != "" {
				disp := tz.Device()
				f, t, err := input.Window(from, to, days, disp, clockNow())
				if err != nil {
					return err
				}
				opt.From, opt.To = f, t
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := icsio.Export(cmd.Context(), db, opt)
			if err != nil {
				return err
			}
			if res.AttendeesWithoutEmail > 0 {
				fmt.Fprintf(os.Stderr, "crm: warning: %d attendee(s) have no email address and were left out\n", res.AttendeesWithoutEmail)
			}
			if out == "" {
				_, err := os.Stdout.Write(res.Data)
				return err
			}
			if err := os.WriteFile(out, res.Data, 0o600); err != nil {
				return model.NewExitError(model.ErrValidation, "write %s: %v", out, err)
			}
			row := toMap(res)
			row["file"] = out
			row["bytes"] = len(res.Data)
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{row}, []format.ColumnDef{
				{Header: "File", Field: "file"}, {Header: "Events", Field: "events"}, {Header: "Overrides", Field: "overrides"}, {Header: "Tasks", Field: "todos"},
			}, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "only events from this date (a date or phrase)")
	cmd.Flags().StringVar(&to, "to", "", "only events before this date")
	cmd.Flags().IntVar(&days, "days", 365, "window length in days when --from is given without --to")
	cmd.Flags().BoolVar(&tasks, "tasks", false, "include tasks as VTODO")
	cmd.Flags().StringVar(&out, "out", "", "write to this file instead of standard output")
	cmd.Flags().StringVar(&name, "name", "crm", "calendar name")
	cmd.Flags().StringVar(&source, "source", icsio.DefaultSource, "source whose original UIDs are reused")
	return cmd
}
