package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/jdanielnd/crm-cli/internal/book"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/remind"
	"github.com/spf13/cobra"
)

var dueReminderColumns = []format.ColumnDef{
	{Header: "Event", Field: "event_id"},
	{Header: "Title", Field: "title"},
	{Header: "Starts", Field: "local_start"},
	{Header: "In", Field: "starts_in"},
	{Header: "Method", Field: "method"},
}

func calDueRemindersCmd() *cobra.Command {
	var within, tzName string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "due-reminders",
		Short: "List event reminders that are due; each is listed once (use --dry-run to look without marking)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var w time.Duration
			if within != "" {
				d, err := when.ParseLength(within)
				if err != nil {
					return model.NewExitError(model.ErrValidation, "--within: %v", err)
				}
				w = d
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
			list, err := remind.List(cmd.Context(), db, clockNow(), w, loc)
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintln(os.Stderr, "dry run: nothing was marked as sent")
			} else if err := remind.Mark(cmd.Context(), db, list); err != nil {
				return err
			}
			rows := make([]map[string]any, len(list))
			for i, d := range list {
				rows[i] = toMap(d)
			}
			return format.Output(os.Stdout, resolveFormat(), rows, dueReminderColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&within, "within", "", "also list reminders that fall due within this time, e.g. 15m (default: only what is due now)")
	cmd.Flags().StringVar(&tzName, "tz", "", "zone for local_start and all-day events (default: this device)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list without marking the reminders as sent")
	return cmd
}

var draftColumns = []format.ColumnDef{
	{Header: "Booking", Field: "booking_id"},
	{Header: "Kind", Field: "kind"},
	{Header: "To", Field: "to"},
	{Header: "Subject", Field: "subject"},
}

func bookRemindCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "remind",
		Short: "Draft reminder and follow-up messages for bookings that are due; each is drafted once",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			drafts, err := book.Drafts(cmd.Context(), db, clockNow())
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintln(os.Stderr, "dry run: nothing was marked as sent")
			} else if err := book.Mark(cmd.Context(), db, drafts); err != nil {
				return err
			}
			if resolveFormat() != format.FormatJSON {
				for _, d := range drafts {
					fmt.Fprintf(os.Stdout, "To: %s\nSubject: %s\nSend at: %s\n\n%s\n---\n", d.To, d.Subject, d.SendAt, d.Body)
				}
				if len(drafts) == 0 {
					fmt.Fprintln(os.Stderr, "no reminders or follow-ups are due")
				}
				return nil
			}
			rows := make([]map[string]any, len(drafts))
			for i, d := range drafts {
				rows[i] = toMap(d)
			}
			return format.Output(os.Stdout, format.FormatJSON, rows, draftColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "draft without marking as sent")
	return cmd
}
