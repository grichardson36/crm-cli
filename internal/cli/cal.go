package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/input"
	"github.com/jdanielnd/crm-cli/internal/cal/recur"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// clockNow is the current time. CRM_NOW (RFC 3339) replaces it, so tests and agents can run
// relative phrases such as "fri 12:30" against a fixed clock.
func clockNow() time.Time {
	if v := os.Getenv("CRM_NOW"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Now()
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// toMap turns a JSON-tagged value into a map so it can go through format.Output.
func toMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

func displayZone(name string) (*time.Location, error) {
	loc, err := tz.Load(name)
	if err != nil {
		return nil, model.NewExitError(model.ErrValidation, "%v", err)
	}
	return loc, nil
}

var calInstanceColumns = []format.ColumnDef{
	{Header: "ID", Field: "event_id"},
	{Header: "Date", Field: "date"},
	{Header: "Time", Field: "time"},
	{Header: "Title", Field: "title"},
	{Header: "Where", Field: "location"},
	{Header: "People", Field: "attendees"},
	{Header: "Repeats", Field: "repeat"},
}

var calEventColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Title", Field: "title"},
	{Header: "Start", Field: "start"},
	{Header: "End", Field: "end"},
	{Header: "Where", Field: "location"},
	{Header: "Repeats", Field: "repeat"},
	{Header: "Status", Field: "status"},
}

// instanceMap renders one listed instance in the display zone.
func instanceMap(in model.EventInstance, disp *time.Location) map[string]any {
	m := toMap(in)
	m["id"] = in.EventID
	m["location"] = in.Location
	if in.AllDay {
		s, _ := tz.ParseDate(in.StartDate)
		e, _ := tz.ParseDate(in.EndDate)
		m["date"] = s.Format("Mon 2 Jan")
		last := e.AddDate(0, 0, -1)
		if last.After(s) {
			m["date"] = s.Format("Mon 2 Jan") + " - " + last.Format("Mon 2 Jan")
		}
		m["time"] = "all day"
	} else {
		s, _ := tz.ParseInstant(in.Start)
		e, _ := tz.ParseInstant(in.End)
		ls, le := s.In(disp), e.In(disp)
		m["date"] = ls.Format("Mon 2 Jan")
		m["time"] = ls.Format("15:04") + "-" + le.Format("15:04")
		if le.YearDay() != ls.YearDay() || le.Year() != ls.Year() {
			m["time"] = ls.Format("15:04") + "-" + le.Format("Mon 15:04")
		}
		m["local_start"] = ls.Format(time.RFC3339)
		m["local_end"] = le.Format(time.RFC3339)
		m["display_tz"] = tz.Name(disp)
	}
	if in.Attendees == 0 {
		m["attendees"] = ""
	}
	if in.Recurring {
		m["repeat"] = "yes"
	}
	return m
}

// eventMap renders one event in the display zone.
func eventMap(e *model.Event, disp *time.Location) map[string]any {
	m := toMap(e)
	if e.AllDay {
		m["start"], m["end"] = derefStr(e.StartDate), derefStr(e.EndDate)
	} else if s, err := tz.ParseInstant(derefStr(e.StartUTC)); err == nil {
		en, _ := tz.ParseInstant(derefStr(e.EndUTC))
		m["start"], m["end"] = s.In(disp).Format("2006-01-02 15:04"), en.In(disp).Format("2006-01-02 15:04")
		m["local_start"], m["local_end"] = s.In(disp).Format(time.RFC3339), en.In(disp).Format(time.RFC3339)
		m["display_tz"] = tz.Name(disp)
	}
	if e.RRule != nil {
		m["repeat"] = recur.Describe(*e.RRule)
	}
	if e.Location == nil {
		m["location"] = ""
	}
	return m
}

// calFlags are the flags shared by cal add and cal edit.
type calFlags struct {
	start, end, duration, tzName, location, notes, repeat, until, status string
	allDay, busy                                                         bool
	count                                                                int
	reminders                                                            []string
	dryRun                                                               bool
}

func (f *calFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.start, "start", "", `when it starts: "2026-10-20 14:00", "fri 12:30", "tomorrow 3pm", or a date for an all-day event`)
	cmd.Flags().StringVar(&f.end, "end", "", "when it ends (an all-day event ends on this day, inclusive)")
	cmd.Flags().StringVar(&f.duration, "duration", "", "length instead of --end, e.g. 30m, 1h, 1h30m, 3d")
	cmd.Flags().BoolVar(&f.allDay, "all-day", false, "all-day event (a date, no time of day)")
	cmd.Flags().StringVar(&f.tzName, "tz", "", "IANA zone of the event, e.g. America/Chicago (default: this device)")
	cmd.Flags().StringVar(&f.location, "location", "", "where")
	cmd.Flags().StringVar(&f.notes, "notes", "", "notes")
	cmd.Flags().StringVar(&f.repeat, "repeat", "", `repeat: "every fri", "weekdays", "every 2w", "last fri", or an RRULE such as FREQ=WEEKLY;BYDAY=MO`)
	cmd.Flags().StringVar(&f.until, "until", "", "last day of the repeat (inclusive)")
	cmd.Flags().IntVar(&f.count, "count", 0, "number of occurrences of the repeat")
	cmd.Flags().StringVar(&f.status, "status", "", "confirmed, tentative or cancelled")
	cmd.Flags().BoolVar(&f.busy, "busy", true, "blocks time in free/busy and find-a-time (--busy=false for transparent)")
	cmd.Flags().StringSliceVar(&f.reminders, "remind", nil, "reminder before the start, e.g. 15m, 1h, 1d (repeatable)")
}

func (f *calFlags) span() input.SpanText {
	return input.SpanText{Start: f.start, End: f.end, Duration: f.duration, AllDay: f.allDay}
}

func parseEntityID(kind, s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, model.NewExitError(model.ErrValidation, "invalid %s ID: %s", kind, s)
	}
	return id, nil
}

func registerCalCommands(rootCmd *cobra.Command) {
	calCmd := &cobra.Command{
		Use:   "cal",
		Short: "Calendar: events, repeats, attendees, ICS files, working hours and free time",
	}
	calCmd.AddCommand(calAddCmd())
	calCmd.AddCommand(calListCmd())
	calCmd.AddCommand(calShowCmd())
	calCmd.AddCommand(calEditCmd())
	calCmd.AddCommand(calRmCmd())
	calCmd.AddCommand(calRestoreCmd())
	calCmd.AddCommand(calPurgeCmd())
	calCmd.AddCommand(calAttendeeCmd())
	calCmd.AddCommand(calInviteCmd())
	registerCalICSCommands(calCmd)
	registerCalHoursCommands(calCmd)
	registerCalFreeCommand(calCmd)
	registerCalPlanCommand(calCmd)
	rootCmd.AddCommand(calCmd)
}

type calDryRun struct {
	DryRun  bool           `json:"dry_run"`
	Command string         `json:"command"`
	EventID int64          `json:"event_id,omitempty"`
	Scope   string         `json:"scope,omitempty"`
	Event   map[string]any `json:"event"`
	Note    string         `json:"note"`
}

func printCalDryRun(p calDryRun) error {
	p.DryRun, p.Note = true, "nothing was written"
	if resolveFormat() == format.FormatJSON {
		return json.NewEncoder(os.Stdout).Encode(p)
	}
	fmt.Fprintf(os.Stdout, "dry run: would run: crm %s\n", p.Command)
	for _, k := range []string{"title", "start", "end", "start_date", "end_date", "tz", "all_day", "rrule", "location", "status"} {
		if v, ok := p.Event[k]; ok && v != nil && v != "" {
			fmt.Fprintf(os.Stdout, "  %s: %v\n", k, v)
		}
	}
	fmt.Fprintln(os.Stdout, "nothing was written")
	return nil
}

func calAddCmd() *cobra.Command {
	var f calFlags
	var personIDs []int64
	var tags []string

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add an event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(f.start) == "" {
				return model.NewExitError(model.ErrValidation, "--start is required, e.g. --start \"fri 12:30\"")
			}
			now := clockNow()
			loc, err := displayZone(f.tzName)
			if err != nil {
				return err
			}
			start, end, allDay, err := input.Span(f.span(), loc, now)
			if err != nil {
				return err
			}
			rule, err := input.Rule(input.RepeatText{Repeat: f.repeat, Changed: cmd.Flags().Changed("repeat"), Until: f.until, Count: f.count}, "", allDay, loc, now)
			if err != nil {
				return err
			}
			reminders, err := input.Reminders(f.reminders)
			if err != nil {
				return err
			}
			in := model.CreateEventInput{
				Title: args[0], Notes: nilIfEmpty(f.notes), Location: nilIfEmpty(f.location),
				AllDay: allDay, Start: start, End: end, TZ: tz.Name(loc), Status: f.status,
				Reminders: reminders, Tags: tags,
			}
			if rule != nil {
				in.RRule = *rule
			}
			if cmd.Flags().Changed("busy") {
				in.Busy = &f.busy
			}
			for _, pid := range personIDs {
				in.Attendees = append(in.Attendees, model.EventAttendee{PersonID: pid})
			}

			if f.dryRun {
				preview := map[string]any{"title": in.Title, "all_day": allDay, "tz": in.TZ, "rrule": in.RRule, "location": f.location, "status": f.status}
				if allDay {
					preview["start_date"] = start.Format(tz.DateLayout)
				} else {
					preview["start"] = tz.FormatInstant(start)
					if !end.IsZero() {
						preview["end"] = tz.FormatInstant(end)
					}
				}
				return printCalDryRun(calDryRun{Command: "cal add", Event: preview})
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			e, err := repo.NewEventRepo(db).Create(cmd.Context(), in)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{eventMap(e, loc)}, calEventColumns, flagQuiet)
		},
	}
	f.bind(cmd)
	cmd.Flags().Int64SliceVar(&personIDs, "person", nil, "attendee person ID (repeatable)")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "tag (repeatable)")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "print the event that would be created and write nothing")
	return cmd
}

func calListCmd() *cobra.Command {
	var from, to, tzName, tag, query string
	var days, limit int
	var personID int64
	var eventTZ, includeCancelled bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events in a window, repeats expanded",
		RunE: func(cmd *cobra.Command, args []string) error {
			disp, err := displayZone(tzName)
			if err != nil {
				return err
			}
			now := clockNow()
			f, t, err := input.Window(from, to, days, disp, now)
			if err != nil {
				return err
			}
			filters := model.EventFilters{From: f, To: t, Loc: disp, Tag: tag, Query: query, Limit: limit, IncludeCancelled: includeCancelled}
			if cmd.Flags().Changed("person") {
				filters.PersonID = &personID
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			list, truncated, err := repo.NewEventRepo(db).List(cmd.Context(), filters)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(list))
			for i, in := range list {
				z := disp
				if eventTZ && in.TZ != "" {
					if l, err := tz.Load(in.TZ); err == nil {
						z = l
					}
				}
				rows[i] = instanceMap(in, z)
			}
			if truncated {
				fmt.Fprintln(os.Stderr, "crm: warning: the list was cut at the instance limit, narrow the window")
			}
			return format.Output(os.Stdout, resolveFormat(), rows, calInstanceColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "window start: a date or phrase (default: today)")
	cmd.Flags().StringVar(&to, "to", "", "window end (exclusive), a date or phrase")
	cmd.Flags().IntVar(&days, "days", 7, "window length in days when --to is not given")
	cmd.Flags().StringVar(&tzName, "tz", "", "zone to show times in (default: this device)")
	cmd.Flags().BoolVar(&eventTZ, "event-tz", false, "show each event in its own zone")
	cmd.Flags().Int64Var(&personID, "person", 0, "only events with this attendee (person ID)")
	cmd.Flags().StringVar(&tag, "tag", "", "only events with this tag")
	cmd.Flags().StringVar(&query, "query", "", "only events matching this text")
	cmd.Flags().BoolVar(&includeCancelled, "include-cancelled", false, "also list cancelled events")
	cmd.Flags().IntVar(&limit, "limit", 0, "max instances (default and maximum 1000)")
	return cmd
}

func calShowCmd() *cobra.Command {
	var tzName string
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show an event with attendees, reminders, tags and repeat",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseEntityID("event", args[0])
			if err != nil {
				return err
			}
			disp, err := displayZone(tzName)
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			e, err := repo.NewEventRepo(db).FindByID(cmd.Context(), id)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{eventMap(e, disp)}, calEventColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&tzName, "tz", "", "zone to show times in (default: this device)")
	return cmd
}

func addScopeFlags(cmd *cobra.Command, scope, at *string, verb string) {
	cmd.Flags().StringVar(scope, "scope", "all", verb+" of a repeating event: all, this (one instance) or following (this and later)")
	cmd.Flags().StringVar(at, "at", "", "start of the instance for --scope this or following: the occurrence value from cal list")
}

func calEditCmd() *cobra.Command {
	var f calFlags
	var title, scope, at string
	var clearReminders bool

	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit an event, one instance of a repeat, or this and later instances",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseEntityID("event", args[0])
			if err != nil {
				return err
			}
			if !model.ValidScope(scope) {
				return model.NewExitError(model.ErrValidation, "invalid scope %q (all, this or following)", scope)
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			er := repo.NewEventRepo(db)
			cur, err := er.FindByID(cmd.Context(), id)
			if err != nil {
				return err
			}
			now := clockNow()
			loc := input.EventLocation(cur)
			if cmd.Flags().Changed("tz") {
				if loc, err = displayZone(f.tzName); err != nil {
					return err
				}
			}

			in := model.UpdateEventInput{}
			ch := cmd.Flags().Changed
			if ch("title") {
				in.Title = &title
			}
			if ch("notes") {
				in.Notes = &f.notes
			}
			if ch("location") {
				in.Location = &f.location
			}
			if ch("status") {
				in.Status = &f.status
			}
			if ch("busy") {
				in.Busy = &f.busy
			}
			if ch("tz") {
				name := tz.Name(loc)
				in.TZ = &name
			}
			allDay := cur.AllDay
			if ch("all-day") {
				allDay = f.allDay
				in.AllDay = &allDay
			}
			if ch("start") || ch("end") || ch("duration") {
				text := input.EditText{Start: f.start, End: f.end, Duration: f.duration, HasStart: ch("start"), HasEnd: ch("end"), HasDuration: ch("duration"), AllDay: allDay}
				if err := input.Edit(text, cur, loc, now, &in); err != nil {
					return err
				}
			}
			if ch("repeat") || ch("until") || ch("count") {
				rule, err := input.Rule(input.RepeatText{Repeat: f.repeat, Changed: ch("repeat"), Until: f.until, Count: f.count}, derefStr(cur.RRule), allDay, loc, now)
				if err != nil {
					return err
				}
				in.RRule = rule
			}
			reminders, err := input.Reminders(f.reminders)
			if err != nil {
				return err
			}
			setReminders := ch("remind") || clearReminders
			anchor, err := input.Anchor(cur, at, now)
			if err != nil {
				return err
			}

			if f.dryRun {
				changes := map[string]any{}
				cmd.Flags().Visit(func(fl *pflag.Flag) {
					if fl.Name != "dry-run" {
						changes[fl.Name] = fl.Value.String()
					}
				})
				if in.Start != nil {
					changes["start_resolved"] = tz.FormatInstant(*in.Start)
				}
				if in.End != nil {
					changes["end_resolved"] = tz.FormatInstant(*in.End)
				}
				if in.RRule != nil {
					changes["rrule"] = *in.RRule
				}
				changes["title"] = cur.Title
				if in.Title != nil {
					changes["title"] = *in.Title
				}
				return printCalDryRun(calDryRun{Command: "cal edit " + args[0], EventID: id, Scope: scope, Event: changes})
			}

			e, err := er.Update(cmd.Context(), id, in, scope, anchor)
			if err != nil {
				return err
			}
			if setReminders {
				if err := er.SetReminders(cmd.Context(), e.ID, reminders); err != nil {
					return err
				}
				if e, err = er.FindByID(cmd.Context(), e.ID); err != nil {
					return err
				}
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{eventMap(e, loc)}, calEventColumns, flagQuiet)
		},
	}
	f.bind(cmd)
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().BoolVar(&clearReminders, "clear-reminders", false, "remove all reminders")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "print the resolved changes and write nothing")
	addScopeFlags(cmd, &scope, &at, "edit")
	return cmd
}

func calRmCmd() *cobra.Command {
	var scope, at string
	cmd := &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete an event (it can be restored), or one instance or the rest of a repeat",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseEntityID("event", args[0])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			er := repo.NewEventRepo(db)
			cur, err := er.FindByID(cmd.Context(), id)
			if err != nil {
				return err
			}
			anchor, err := input.Anchor(cur, at, clockNow())
			if err != nil {
				return err
			}
			if err := er.Archive(cmd.Context(), id, scope, anchor); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Deleted event #%d (%s)\n", id, scope)
			return nil
		},
	}
	addScopeFlags(cmd, &scope, &at, "delete")
	return cmd
}

func calRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <id>",
		Short: "Bring back a deleted event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseEntityID("event", args[0])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := repo.NewEventRepo(db).Restore(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Restored event #%d\n", id)
			return nil
		},
	}
}

func calPurgeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "purge <id>",
		Short: "Permanently delete an already deleted event (writes a tombstone)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseEntityID("event", args[0])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := repo.NewEventRepo(db).Purge(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Purged event #%d\n", id)
			return nil
		},
	}
}

func calAttendeeCmd() *cobra.Command {
	attendee := &cobra.Command{Use: "attendee", Short: "Link people to an event"}

	var role, rsvp string
	add := &cobra.Command{
		Use:   "add <event-id> <person-id>",
		Short: "Add an attendee, or change their role or RSVP",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eid, err := parseEntityID("event", args[0])
			if err != nil {
				return err
			}
			pid, err := parseEntityID("person", args[1])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			er := repo.NewEventRepo(db)
			if err := er.SetAttendee(cmd.Context(), eid, model.EventAttendee{PersonID: pid, Role: role, RSVP: rsvp}); err != nil {
				return err
			}
			e, err := er.FindByID(cmd.Context(), eid)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{eventMap(e, time.Local)}, calEventColumns, flagQuiet)
		},
	}
	add.Flags().StringVar(&role, "role", "attendee", "organizer or attendee")
	add.Flags().StringVar(&rsvp, "rsvp", "needs-action", "needs-action, accepted, declined or tentative")

	rm := &cobra.Command{
		Use:   "rm <event-id> <person-id>",
		Short: "Remove an attendee",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eid, err := parseEntityID("event", args[0])
			if err != nil {
				return err
			}
			pid, err := parseEntityID("person", args[1])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := repo.NewEventRepo(db).RemoveAttendee(cmd.Context(), eid, pid); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Removed person #%d from event #%d\n", pid, eid)
			return nil
		},
	}
	attendee.AddCommand(add, rm)
	return attendee
}
