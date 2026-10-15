package cli

import (
	"fmt"
	"github.com/jdanielnd/crm-cli/internal/integrations"
	"os"
	"strconv"

	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/spf13/cobra"
)

func registerSyncCommands(rootCmd *cobra.Command) {
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull records from external sources",
		Long:  "Pull records from external sources. Matches people by external id, then exact email, phone or handle. Never deletes. Records that match nobody wait in the inbox (crm sync inbox).",
	}
	syncCmd.AddCommand(syncListCmd(), syncRunCmd(), syncStatusCmd(), syncInboxCmd())
	rootCmd.AddCommand(syncCmd)
}

func newRunner() (*crmsync.Runner, func(), error) {
	db, err := openDB()
	if err != nil {
		return nil, nil, err
	}
	return crmsync.NewRunner(db, integrations.DefaultRegistry(os.Getenv)), func() { db.Close() }, nil
}

func syncListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available sync sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := integrations.DefaultRegistry(os.Getenv)
			data := []map[string]any{}
			for _, n := range registry.Names() {
				data = append(data, map[string]any{"source": n})
			}
			return format.Output(os.Stdout, resolveFormat(), data, []format.ColumnDef{{Header: "Source", Field: "source"}}, flagQuiet)
		},
	}
}

var syncRunColumns = []format.ColumnDef{
	{Header: "Source", Field: "source"},
	{Header: "Status", Field: "status"},
	{Header: "Created", Field: "created"},
	{Header: "Updated", Field: "updated"},
	{Header: "Skipped", Field: "skipped"},
	{Header: "Inbox", Field: "inbox"},
	{Header: "Errors", Field: "errors"},
	{Header: "Dry Run", Field: "dry_run"},
	{Header: "Note", Field: "note"},
}

func syncRunCmd() *cobra.Command {
	var dryRun, allowCreate bool
	cmd := &cobra.Command{
		Use:   "run <source>",
		Short: "Run one sync source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, closeDB, err := newRunner()
			if err != nil {
				return err
			}
			defer closeDB()

			res, err := runner.Run(cmd.Context(), args[0], crmsync.RunOptions{DryRun: dryRun, AllowCreate: allowCreate})
			if err != nil {
				return err
			}
			row := map[string]any{
				"source": res.Source, "status": res.Status, "created": res.Created, "updated": res.Updated,
				"skipped": res.Skipped, "inbox": res.Inbox, "errors": res.Errors, "dry_run": res.DryRun,
				"note": res.Note, "run_id": res.RunID, "since": res.Since, "next_cursor": res.NextCursor,
			}
			if err := format.Output(os.Stdout, resolveFormat(), []map[string]any{row}, syncRunColumns, flagQuiet); err != nil {
				return err
			}
			if res.Status == model.SyncFailed {
				return fmt.Errorf("sync %s failed: %s", res.Source, res.Note)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report counts without writing anything")
	cmd.Flags().BoolVar(&allowCreate, "allow-create", false, "create people nobody matches instead of queueing them in the inbox")
	return cmd
}

func syncStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show each source's last run and the inbox size",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, closeDB, err := newRunner()
			if err != nil {
				return err
			}
			defer closeDB()
			o, err := runner.Overview(cmd.Context())
			if err != nil {
				return err
			}
			data := make([]map[string]any, 0, len(o.Sources))
			for _, st := range o.Sources {
				row := map[string]any{"source": st.Source, "registered": st.Registered, "cursor": st.Cursor, "inbox_pending": o.InboxPending}
				if st.LastRun != nil {
					row["last_run"] = st.LastRun
					row["status"] = st.LastRun.Status
					row["finished_at"] = st.LastRun.FinishedAt
					row["created"], row["updated"], row["skipped"], row["errors"] = st.LastRun.Created, st.LastRun.Updated, st.LastRun.Skipped, st.LastRun.Errors
				}
				data = append(data, row)
			}
			cols := []format.ColumnDef{
				{Header: "Source", Field: "source"}, {Header: "Registered", Field: "registered"},
				{Header: "Last Status", Field: "status"}, {Header: "Finished", Field: "finished_at"},
				{Header: "Created", Field: "created"}, {Header: "Updated", Field: "updated"},
				{Header: "Skipped", Field: "skipped"}, {Header: "Errors", Field: "errors"},
				{Header: "Cursor", Field: "cursor"}, {Header: "Inbox", Field: "inbox_pending"},
			}
			return format.Output(os.Stdout, resolveFormat(), data, cols, flagQuiet)
		},
	}
}

var inboxColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Source", Field: "source"},
	{Header: "External ID", Field: "external_id"},
	{Header: "Kind", Field: "kind"},
	{Header: "Reason", Field: "reason"},
	{Header: "Summary", Field: "summary"},
	{Header: "Status", Field: "status"},
}

func syncInboxCmd() *cobra.Command {
	inbox := &cobra.Command{
		Use:   "inbox",
		Short: "Review records the sync runner could not place",
	}

	var status string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List inbox records (pending by default)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withInbox(func(b *crmsync.Inbox) error {
				st := status
				if st == "all" {
					st = ""
				}
				items, err := b.List(cmd.Context(), st, limit)
				if err != nil {
					return err
				}
				data := make([]map[string]any, len(items))
				for i, it := range items {
					data[i] = map[string]any{"id": it.ID, "uuid": it.UUID, "source": it.Source, "external_id": it.ExternalID,
						"kind": it.Kind, "reason": it.Reason, "candidates": it.Candidates, "summary": it.Summary,
						"status": it.Status, "payload": it.Payload, "created_at": it.CreatedAt, "updated_at": it.UpdatedAt}
				}
				return format.Output(os.Stdout, resolveFormat(), data, inboxColumns, flagQuiet)
			})
		},
	}
	list.Flags().StringVar(&status, "status", "pending", "pending, accepted, attached, ignored or all")
	list.Flags().IntVar(&limit, "limit", 0, "max results")

	accept := &cobra.Command{
		Use:   "accept <id>",
		Short: "Create a person from an inbox record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInboxID(args[0])
			if err != nil {
				return err
			}
			return withInbox(func(b *crmsync.Inbox) error {
				personID, err := b.Accept(cmd.Context(), id)
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Inbox #%d accepted as person #%d\n", id, personID)
				return nil
			})
		},
	}

	attach := &cobra.Command{
		Use:   "attach <id> <person_id>",
		Short: "Attach an inbox record to an existing person",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInboxID(args[0])
			if err != nil {
				return err
			}
			personID, err := parsePersonID(args[1])
			if err != nil {
				return err
			}
			return withInbox(func(b *crmsync.Inbox) error {
				if err := b.Attach(cmd.Context(), id, personID); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Inbox #%d attached to person #%d\n", id, personID)
				return nil
			})
		},
	}

	ignore := &cobra.Command{
		Use:   "ignore <id>",
		Short: "Dismiss an inbox record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInboxID(args[0])
			if err != nil {
				return err
			}
			return withInbox(func(b *crmsync.Inbox) error {
				if err := b.Ignore(cmd.Context(), id); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Inbox #%d ignored\n", id)
				return nil
			})
		},
	}

	inbox.AddCommand(list, accept, attach, ignore)
	// `crm sync inbox` alone lists, like `list`.
	inbox.Args = cobra.NoArgs
	inbox.RunE = list.RunE
	inbox.Flags().AddFlagSet(list.Flags())
	return inbox
}

func parseInboxID(arg string) (int64, error) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id <= 0 {
		return 0, model.NewExitError(model.ErrValidation, "invalid inbox ID: %s", arg)
	}
	return id, nil
}

func withInbox(fn func(b *crmsync.Inbox) error) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	return fn(crmsync.NewInbox(db))
}
