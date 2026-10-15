package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/book"
	"github.com/jdanielnd/crm-cli/internal/cal/ics"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var bookLinkColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Slug", Field: "slug"},
	{Header: "Title", Field: "title"},
	{Header: "Min", Field: "durations"},
	{Header: "Days", Field: "horizon_days"},
	{Header: "Notice", Field: "min_notice_min"},
	{Header: "Active", Field: "active"},
}

var bookingColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Link", Field: "link"},
	{Header: "Start", Field: "start"},
	{Header: "Who", Field: "invitee_email"},
	{Header: "Status", Field: "status"},
}

func registerBookCommands(rootCmd *cobra.Command) {
	bookCmd := &cobra.Command{
		Use:   "book",
		Short: "Booking links without a server: offer times as text, a web page or mail links, and confirm replies",
	}
	bookCmd.AddCommand(bookAddCmd(), bookListCmd(), bookOfferCmd(), bookExportCmd(), bookConfirmCmd(), bookCancelCmd(),
		bookPauseCmd(false), bookPauseCmd(true), bookRemoveCmd())
	rootCmd.AddCommand(bookCmd)
}

func bookLinkRow(l *model.BookingLink) map[string]any {
	m := toMap(l)
	parts := make([]string, len(l.Durations))
	for i, d := range l.Durations {
		parts[i] = fmt.Sprint(d)
	}
	m["durations"] = strings.Join(parts, ",")
	return m
}

func bookAddCmd() *cobra.Command {
	var sp book.LinkSpec
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a booking link: lengths, hours, notice, buffers, caps and questions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sp.Name = args[0]
			in, err := sp.Input(clockNow(), tz.Device())
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintln(os.Stderr, "dry run: nothing was written")
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"dry_run": true, "link": in})
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			l, err := repo.NewBookingRepo(db).CreateLink(cmd.Context(), in)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{bookLinkRow(l)}, bookLinkColumns, flagQuiet)
		},
	}
	f := cmd.Flags()
	f.StringVar(&sp.Title, "title", "", "name the invitee sees (default: the name)")
	f.StringVar(&sp.Slug, "slug", "", "short id used in commands and slot lines (default: from the name)")
	f.StringVar(&sp.Description, "description", "", "text shown under the title")
	f.StringVar(&sp.Email, "email", "", "your address: where mail links send requests, and the organizer of invitations")
	f.StringVar(&sp.Lens, "len", "30m", "meeting lengths offered, comma separated, the first is the default (30m,45m,1h)")
	f.StringVar(&sp.Rule, "rule", "", "working hours rule to offer times from (default: the default rule)")
	f.IntVar(&sp.Horizon, "horizon", 14, "offer times this many days ahead")
	f.StringVar(&sp.From, "from", "", "offer nothing before this date")
	f.StringVar(&sp.Notice, "notice", "4h", "do not offer anything sooner than this from now")
	f.IntVar(&sp.Step, "step", 30, "slots start every this many minutes")
	f.IntVar(&sp.Before, "buffer-before", 0, "minutes to keep free before a meeting")
	f.IntVar(&sp.After, "buffer-after", 0, "minutes to keep free after a meeting")
	f.IntVar(&sp.Both, "buffer", 0, "same minutes before and after")
	f.IntVar(&sp.MaxDay, "max-day", 0, "at most this many meetings per day")
	f.IntVar(&sp.MaxWeek, "max-week", 0, "at most this many meetings per week")
	f.IntVar(&sp.MaxMonth, "max-month", 0, "at most this many meetings per month")
	f.BoolVar(&sp.BusyTasks, "busy-tasks", false, "also keep time that planned tasks use free")
	f.StringVar(&sp.Where, "where", "", "address, phone number or room link shown to the invitee")
	f.StringVar(&sp.WhereKind, "where-kind", "", "phone, in_person or url (default: guessed from --where)")
	f.StringArrayVar(&sp.Questions, "question", nil, "ask the invitee this (repeat; end with ! to require an answer)")
	f.StringArrayVar(&sp.Reminds, "remind", nil, "event reminder before each meeting, e.g. 1d (repeat)")
	f.StringVar(&sp.Followup, "followup", "", "due this long after the meeting for the follow-up task, e.g. 1d")
	f.IntVar(&sp.Uses, "uses", 0, "close the link after this many bookings (1 = one-time link)")
	f.StringVar(&sp.Expires, "expires", "", "close the link at this date or time")
	f.BoolVar(&dryRun, "dry-run", false, "show the link and write nothing")
	return cmd
}

func bookListCmd() *cobra.Command {
	var link string
	var bookings, all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List booking links, or with --bookings the booked meetings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			br := repo.NewBookingRepo(db)
			if !bookings {
				links, err := br.ListLinks(cmd.Context())
				if err != nil {
					return err
				}
				rows := make([]map[string]any, len(links))
				for i, l := range links {
					rows[i] = bookLinkRow(l)
				}
				return format.Output(os.Stdout, resolveFormat(), rows, bookLinkColumns, flagQuiet)
			}
			f := repo.BookingFilters{All: all}
			if link != "" {
				l, err := br.FindLink(cmd.Context(), link)
				if err != nil {
					return err
				}
				f.LinkID = l.ID
			}
			list, err := br.ListBookings(cmd.Context(), f)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(list))
			for i, b := range list {
				rows[i] = toMap(b)
			}
			return format.Output(os.Stdout, resolveFormat(), rows, bookingColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&bookings, "bookings", false, "list booked meetings instead of links")
	cmd.Flags().StringVar(&link, "link", "", "only bookings of this link")
	cmd.Flags().BoolVar(&all, "all", false, "include cancelled bookings")
	return cmd
}

// bookSlots finds the link and the offered times.
func bookSlots(cmd *cobra.Command, ref, length string, count int) (*book.Result, func(), error) {
	db, err := openDB()
	if err != nil {
		return nil, nil, err
	}
	l, err := repo.NewBookingRepo(db).FindLink(cmd.Context(), ref)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	n := 0
	if length != "" {
		d, err := when.ParseLength(length)
		if err != nil {
			db.Close()
			return nil, nil, model.NewExitError(model.ErrValidation, "--len: %v", err)
		}
		n = int(d / time.Minute)
	}
	res, err := book.Slots(cmd.Context(), db, l, n, clockNow())
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	res.Limit(count)
	return res, func() { db.Close() }, nil
}

func bookOfferCmd() *cobra.Command {
	var length, inviteeTZ string
	var count int
	cmd := &cobra.Command{
		Use:   "offer <link>",
		Short: "Print the next open times of a link as text you can paste into a message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, closeDB, err := bookSlots(cmd, args[0], length, count)
			if err != nil {
				return err
			}
			defer closeDB()
			var inv *time.Location
			if inviteeTZ != "" {
				if inv, err = displayZone(inviteeTZ); err != nil {
					return err
				}
			}
			text, err := book.Offer(res, inv)
			if err != nil {
				return err
			}
			if flagFormat == "json" {
				slots := make([]map[string]any, len(res.Slots))
				loc, _ := tz.Load(res.TZ)
				for i, s := range res.Slots {
					slots[i] = map[string]any{"start": tz.FormatInstant(s.Start), "end": tz.FormatInstant(s.End), "local_start": s.Start.In(loc).Format(time.RFC3339)}
				}
				return format.Output(os.Stdout, format.FormatJSON, []map[string]any{{
					"link": res.Link.Slug, "title": res.Link.Title, "tz": res.TZ, "length_min": res.LengthMin, "reason": res.Reason, "slots": slots, "text": text,
				}}, nil, flagQuiet)
			}
			fmt.Fprint(os.Stdout, text)
			return nil
		},
	}
	cmd.Flags().StringVar(&length, "len", "", "meeting length (default: the link's first length)")
	cmd.Flags().IntVar(&count, "count", 10, "how many times to list (0 for all)")
	cmd.Flags().StringVar(&inviteeTZ, "tz", "", "also show every time in this zone, e.g. America/New_York")
	return cmd
}

func bookExportCmd() *cobra.Command {
	var length, out string
	var count int
	cmd := &cobra.Command{
		Use:   "export <link>",
		Short: "Write a self-contained web page with one mail button per open time",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, closeDB, err := bookSlots(cmd, args[0], length, count)
			if err != nil {
				return err
			}
			defer closeDB()
			if err := book.NeedsHost(res.Link); err != nil {
				return err
			}
			page, err := book.HTML(res, clockNow())
			if err != nil {
				return err
			}
			if out == "" {
				_, err := os.Stdout.Write(page)
				return err
			}
			path := out
			if !strings.HasSuffix(strings.ToLower(out), ".html") {
				if err := os.MkdirAll(out, 0o755); err != nil {
					return model.NewExitError(model.ErrValidation, "create %s: %v", out, err)
				}
				path = filepath.Join(out, "index.html")
			}
			if err := os.WriteFile(path, page, 0o644); err != nil {
				return model.NewExitError(model.ErrValidation, "write %s: %v", path, err)
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"file": path, "slots": len(res.Slots), "bytes": len(page)}},
				[]format.ColumnDef{{Header: "File", Field: "file"}, {Header: "Slots", Field: "slots"}}, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&length, "len", "", "meeting length (default: the link's first length)")
	cmd.Flags().IntVar(&count, "count", 60, "most times on the page (0 for all)")
	cmd.Flags().StringVar(&out, "out", "", "folder (writes index.html) or .html file; default is standard output")
	return cmd
}

func bookConfirmCmd() *cobra.Command {
	var at, length, name, email, inviteeTZ, inviteOut string
	var answers []string
	var person int64
	var followUp, dryRun bool
	cmd := &cobra.Command{
		Use:   "confirm <link | slug/instant/len>",
		Short: "Book a time the invitee asked for: creates the event, links the contact, logs a meeting",
		Long: "Book a time the invitee asked for. Pass the link and --at, or paste the Slot line from the invitee's mail " +
			"(slug/2026-10-12T15:00:00Z/30) as the argument. The time must still be open. The invitation (.ics) goes to --invite.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			br := repo.NewBookingRepo(db)
			ref := args[0]
			var start time.Time
			lenMin := 0
			if slug, t, n, ok := book.SplitToken(ref); ok {
				ref, start, lenMin = slug, t, n
			}
			l, err := br.FindLink(cmd.Context(), ref)
			if err != nil {
				return err
			}
			if length != "" {
				d, err := when.ParseLength(length)
				if err != nil {
					return model.NewExitError(model.ErrValidation, "--len: %v", err)
				}
				lenMin = int(d / time.Minute)
			}
			if at != "" {
				if start, err = book.ResolveStart(cmd.Context(), db, l, at, clockNow()); err != nil {
					return err
				}
			}
			if start.IsZero() {
				return model.NewExitError(model.ErrValidation, "say which time: --at \"tue 10:00\" or pass the slot line")
			}
			ans, err := book.Answers(answers)
			if err != nil {
				return err
			}
			res, err := book.Confirm(cmd.Context(), db, book.ConfirmInput{
				Link: l, Start: start, LengthMin: lenMin, Name: name, Email: email, TZ: inviteeTZ, Answers: ans,
				PersonID: person, FollowUp: followUp, Now: clockNow(), DryRun: dryRun,
			})
			if err != nil {
				return err
			}
			if res.Note != "" {
				fmt.Fprintln(os.Stderr, "crm: note:", res.Note)
			}
			if dryRun {
				fmt.Fprintln(os.Stderr, "dry run: nothing was written")
			} else if inviteOut != "" {
				if err := os.WriteFile(inviteOut, res.Invite, 0o600); err != nil {
					return model.NewExitError(model.ErrValidation, "write %s: %v", inviteOut, err)
				}
				fmt.Fprintf(os.Stderr, "invitation written to %s: attach it to your reply\n", inviteOut)
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{toMap(res)}, []format.ColumnDef{
				{Header: "Booking", Field: "booking.id"}, {Header: "Start", Field: "booking.start"}, {Header: "Who", Field: "booking.invitee_email"}, {Header: "Contact", Field: "person"},
			}, flagQuiet)
		},
	}
	f := cmd.Flags()
	f.StringVar(&at, "at", "", "the time, in the link's zone, such as \"tue 10:00\" or an instant")
	f.StringVar(&length, "len", "", "meeting length (default: the link's first length)")
	f.StringVar(&name, "name", "", "invitee name")
	f.StringVar(&email, "email", "", "invitee email (required)")
	f.StringVar(&inviteeTZ, "tz", "", "invitee zone, if known")
	f.StringArrayVar(&answers, "answer", nil, "answer to a question, Question=answer (repeat)")
	f.Int64Var(&person, "person", 0, "contact ID to attach (default: matched by email)")
	f.BoolVar(&followUp, "follow-up", false, "also add a follow-up task")
	f.StringVar(&inviteOut, "invite", "", "write the invitation (.ics, METHOD:REQUEST) to this file")
	f.BoolVar(&dryRun, "dry-run", false, "check the time and show what would happen, write nothing")
	return cmd
}

func bookCancelCmd() *cobra.Command {
	var inviteOut string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "cancel <booking id>",
		Short: "Cancel a booking and delete its event; --invite writes the cancellation (.ics) to send",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseEntityID("booking", args[0])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			b, cancel, err := book.Cancel(cmd.Context(), db, id, dryRun)
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintln(os.Stderr, "dry run: nothing was written")
			} else if inviteOut != "" && cancel != nil {
				if err := os.WriteFile(inviteOut, cancel, 0o600); err != nil {
					return model.NewExitError(model.ErrValidation, "write %s: %v", inviteOut, err)
				}
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{toMap(b)}, bookingColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&inviteOut, "invite", "", "write the cancellation (.ics, METHOD:CANCEL) to this file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the booking and write nothing")
	return cmd
}

func bookPauseCmd(resume bool) *cobra.Command {
	use, short := "pause <link>", "Stop offering times from a link (bookings stay)"
	if resume {
		use, short = "resume <link>", "Start offering times from a paused link again"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			br := repo.NewBookingRepo(db)
			l, err := br.FindLink(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := br.SetLinkActive(cmd.Context(), l.ID, resume); err != nil {
				return err
			}
			l.Active = resume
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{bookLinkRow(l)}, bookLinkColumns, flagQuiet)
		},
	}
}

func bookRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <link>",
		Short: "Delete a booking link (its bookings and events stay)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			br := repo.NewBookingRepo(db)
			l, err := br.FindLink(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := br.ArchiveLink(cmd.Context(), l.ID); err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{bookLinkRow(l)}, bookLinkColumns, flagQuiet)
		},
	}
}

// bookInviteAttendees is used by cal invite.
func bookInviteAttendees(list []string) []ics.Person {
	var out []ics.Person
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, ics.Person{Email: s})
		}
	}
	return out
}
