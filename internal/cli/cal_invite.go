package cli

import (
	"os"

	"github.com/jdanielnd/crm-cli/internal/book"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

func calInviteCmd() *cobra.Command {
	var to []string
	var from, fromName, out string
	var cancel bool
	cmd := &cobra.Command{
		Use:   "invite <event id>",
		Short: "Write an invitation (.ics, METHOD:REQUEST) for an event to attach to an email",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseEntityID("event", args[0])
			if err != nil {
				return err
			}
			if len(to) == 0 {
				return model.NewExitError(model.ErrValidation, "--to needs at least one address")
			}
			method := "REQUEST"
			if cancel {
				method = "CANCEL"
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			data, err := book.Invite(cmd.Context(), db, id, from, fromName, bookInviteAttendees(to), method)
			if err != nil {
				return err
			}
			if out == "" {
				_, err := os.Stdout.Write(data)
				return err
			}
			if err := os.WriteFile(out, data, 0o600); err != nil {
				return model.NewExitError(model.ErrValidation, "write %s: %v", out, err)
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"file": out, "event_id": id, "method": method, "bytes": len(data)}},
				[]format.ColumnDef{{Header: "File", Field: "file"}, {Header: "Method", Field: "method"}}, flagQuiet)
		},
	}
	cmd.Flags().StringArrayVar(&to, "to", nil, "invitee address (repeat)")
	cmd.Flags().StringVar(&from, "from", "", "your address, written as the organizer")
	cmd.Flags().StringVar(&fromName, "from-name", "", "your name, written with the organizer")
	cmd.Flags().StringVar(&out, "out", "", "write to this file instead of standard output")
	cmd.Flags().BoolVar(&cancel, "cancel", false, "make it a cancellation (METHOD:CANCEL)")
	return cmd
}
