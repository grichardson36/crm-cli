package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/icsio"
	"github.com/jdanielnd/crm-cli/internal/cal/input"
	"github.com/jdanielnd/crm-cli/internal/cal/recur"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// calNow is the current time. CRM_NOW (RFC 3339) replaces it for tests and agents.
func calNow() time.Time {
	if v := os.Getenv("CRM_NOW"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Now()
}

func argStrings(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func argIDs(args map[string]any, key string) []int64 {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	var out []int64
	for _, v := range raw {
		if f, ok := v.(float64); ok && f == math.Trunc(f) && f > 0 {
			out = append(out, int64(f))
		}
	}
	return out
}

func validationError(err error) (*gomcp.CallToolResult, error) {
	return gomcp.NewToolResultError("validation error: " + err.Error()), nil
}

// eventView is an event with the repeat in words.
type eventView struct {
	*model.Event
	Repeat string `json:"repeat,omitempty"`
}

func viewOf(e *model.Event) eventView {
	v := eventView{Event: e}
	if e.RRule != nil {
		v.Repeat = recur.Describe(*e.RRule)
	}
	return v
}

// registerEventTools adds the calendar tools. They call the same repository and the same text
// resolution (package input) as `crm cal`.
func registerEventTools(s *server.MCPServer, db *sql.DB) {
	er := repo.NewEventRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_event_create",
			gomcp.WithDescription("Create a calendar event. start accepts \"2026-10-20 14:00\", \"fri 12:30\", \"tomorrow 3pm\" or a date for an all-day event; times are in tz (default: this device). Give end or duration, else it lasts one hour. repeat accepts \"daily\", \"weekdays\", \"every fri\", \"every 2w\", \"last fri\" or an RRULE."),
			gomcp.WithString("title", gomcp.Required(), gomcp.Description("Event title")),
			gomcp.WithString("start", gomcp.Required(), gomcp.Description("Start date and time")),
			gomcp.WithString("end", gomcp.Description("End (an all-day event ends on this day, inclusive)")),
			gomcp.WithString("duration", gomcp.Description("Length instead of end: 30m, 1h, 1h30m, 3d")),
			gomcp.WithBoolean("all_day", gomcp.Description("All-day event")),
			gomcp.WithString("tz", gomcp.Description("IANA zone of the event, such as America/Chicago")),
			gomcp.WithString("location", gomcp.Description("Where")),
			gomcp.WithString("notes", gomcp.Description("Notes")),
			gomcp.WithString("repeat", gomcp.Description("Repeat rule or phrase")),
			gomcp.WithString("until", gomcp.Description("Last day of the repeat (inclusive)")),
			gomcp.WithNumber("count", gomcp.Description("Number of occurrences of the repeat")),
			gomcp.WithArray("person_ids", gomcp.Description("Attendee person IDs"), gomcp.WithNumberItems()),
			gomcp.WithArray("reminders", gomcp.Description("Reminders before the start, such as 15m, 1h, 1d:email"), gomcp.WithStringItems()),
			gomcp.WithArray("tags", gomcp.Description("Tags"), gomcp.WithStringItems()),
			gomcp.WithString("status", gomcp.Description("confirmed (default), tentative or cancelled")),
			gomcp.WithBoolean("busy", gomcp.Description("Blocks time in free/busy (default true)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			args := req.GetArguments()
			now := calNow()
			loc, err := tz.Load(req.GetString("tz", ""))
			if err != nil {
				return validationError(err)
			}
			start, end, allDay, err := input.Span(input.SpanText{
				Start: req.GetString("start", ""), End: req.GetString("end", ""),
				Duration: req.GetString("duration", ""), AllDay: req.GetBool("all_day", false),
			}, loc, now)
			if err != nil {
				return validationError(err)
			}
			count, _ := argInt64(args, "count")
			_, hasRepeat := args["repeat"]
			rule, err := input.Rule(input.RepeatText{
				Repeat: req.GetString("repeat", ""), Changed: hasRepeat, Until: req.GetString("until", ""), Count: int(count),
			}, "", allDay, loc, now)
			if err != nil {
				return validationError(err)
			}
			reminders, err := input.Reminders(argStrings(args, "reminders"))
			if err != nil {
				return validationError(err)
			}
			in := model.CreateEventInput{
				Title: req.GetString("title", ""), Notes: strPtr(req.GetString("notes", "")), Location: strPtr(req.GetString("location", "")),
				AllDay: allDay, Start: start, End: end, TZ: tz.Name(loc), Status: req.GetString("status", ""),
				Reminders: reminders, Tags: argStrings(args, "tags"),
			}
			if rule != nil {
				in.RRule = *rule
			}
			if v, ok := args["busy"].(bool); ok {
				in.Busy = &v
			}
			for _, pid := range argIDs(args, "person_ids") {
				in.Attendees = append(in.Attendees, model.EventAttendee{PersonID: pid})
			}
			e, err := er.Create(ctx, in)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(viewOf(e))
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_event_list",
			gomcp.WithDescription("List calendar events in a window with repeats expanded. Each item has start and end in UTC (or start_date and end_date for all-day events), tz, and for repeating events an occurrence value to pass as at to crm_event_update or crm_event_delete."),
			gomcp.WithString("from", gomcp.Description("Window start: a date or phrase (default: today)")),
			gomcp.WithString("to", gomcp.Description("Window end, exclusive")),
			gomcp.WithNumber("days", gomcp.Description("Window length in days when to is not given (default 7)")),
			gomcp.WithString("tz", gomcp.Description("Zone that decides where days start (default: this device)")),
			gomcp.WithNumber("person_id", gomcp.Description("Only events with this attendee")),
			gomcp.WithString("tag", gomcp.Description("Only events with this tag")),
			gomcp.WithString("query", gomcp.Description("Only events matching this text")),
			gomcp.WithBoolean("include_cancelled", gomcp.Description("Also list cancelled events")),
			gomcp.WithNumber("limit", gomcp.Description("Max instances (default and maximum 1000)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			args := req.GetArguments()
			disp, err := tz.Load(req.GetString("tz", ""))
			if err != nil {
				return validationError(err)
			}
			days := req.GetInt("days", 7)
			from, to, err := input.Window(req.GetString("from", ""), req.GetString("to", ""), days, disp, calNow())
			if err != nil {
				return validationError(err)
			}
			f := model.EventFilters{
				From: from, To: to, Loc: disp, Tag: req.GetString("tag", ""), Query: req.GetString("query", ""),
				IncludeCancelled: req.GetBool("include_cancelled", false), Limit: req.GetInt("limit", 0),
			}
			if pid, ok := argInt64(args, "person_id"); ok && pid > 0 {
				f.PersonID = &pid
			}
			list, truncated, err := er.List(ctx, f)
			if err != nil {
				return mcpError(err)
			}
			if list == nil {
				list = []model.EventInstance{}
			}
			return jsonResult(map[string]any{
				"from": tz.FormatInstant(from), "to": tz.FormatInstant(to), "events": list, "truncated": truncated,
			})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_event_get",
			gomcp.WithDescription("Get one event with attendees, reminders, tags, exceptions and the repeat in words"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Event ID")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			e, err := er.FindByID(ctx, id)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(viewOf(e))
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_event_update",
			gomcp.WithDescription("Change an event. Only the fields you pass change. For a repeating event, scope is all (default), this (one instance) or following (this and later); this and following need at, the occurrence value from crm_event_list. Passing reminders replaces them."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Event ID (the series ID for a repeat)")),
			gomcp.WithString("title", gomcp.Description("Title")),
			gomcp.WithString("start", gomcp.Description("New start. A date alone moves the day and keeps the time of day")),
			gomcp.WithString("end", gomcp.Description("New end")),
			gomcp.WithString("duration", gomcp.Description("New length")),
			gomcp.WithBoolean("all_day", gomcp.Description("Make the event all-day or timed (needs start)")),
			gomcp.WithString("tz", gomcp.Description("Zone of the event")),
			gomcp.WithString("location", gomcp.Description("Where (empty clears)")),
			gomcp.WithString("notes", gomcp.Description("Notes (empty clears)")),
			gomcp.WithString("repeat", gomcp.Description("Repeat rule or phrase (empty makes it a single event)")),
			gomcp.WithString("until", gomcp.Description("Last day of the repeat")),
			gomcp.WithNumber("count", gomcp.Description("Number of occurrences")),
			gomcp.WithString("status", gomcp.Description("confirmed, tentative or cancelled")),
			gomcp.WithBoolean("busy", gomcp.Description("Blocks time in free/busy")),
			gomcp.WithArray("reminders", gomcp.Description("Replace reminders, such as 15m, 1d:email"), gomcp.WithStringItems()),
			gomcp.WithString("scope", gomcp.Description("all, this or following")),
			gomcp.WithString("at", gomcp.Description("Start of the instance for scope this or following")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			args := req.GetArguments()
			cur, err := er.FindByID(ctx, id)
			if err != nil {
				return mcpError(err)
			}
			now := calNow()
			loc := input.EventLocation(cur)
			var in model.UpdateEventInput
			has := func(k string) bool { _, ok := args[k]; return ok }
			if has("tz") {
				if loc, err = tz.Load(req.GetString("tz", "")); err != nil {
					return validationError(err)
				}
				name := tz.Name(loc)
				in.TZ = &name
			}
			if v, ok := argString(args, "title"); ok {
				in.Title = &v
			}
			if v, ok := argString(args, "location"); ok {
				in.Location = &v
			}
			if v, ok := argString(args, "notes"); ok {
				in.Notes = &v
			}
			if v, ok := argString(args, "status"); ok {
				in.Status = &v
			}
			if v, ok := args["busy"].(bool); ok {
				in.Busy = &v
			}
			allDay := cur.AllDay
			if v, ok := args["all_day"].(bool); ok {
				allDay = v
				in.AllDay = &v
			}
			if has("start") || has("end") || has("duration") {
				text := input.EditText{
					Start: req.GetString("start", ""), End: req.GetString("end", ""), Duration: req.GetString("duration", ""),
					HasStart: has("start"), HasEnd: has("end"), HasDuration: has("duration"), AllDay: allDay,
				}
				if err := input.Edit(text, cur, loc, now, &in); err != nil {
					return validationError(err)
				}
			}
			if has("repeat") || has("until") || has("count") {
				count, _ := argInt64(args, "count")
				base := ""
				if cur.RRule != nil {
					base = *cur.RRule
				}
				rule, err := input.Rule(input.RepeatText{
					Repeat: req.GetString("repeat", ""), Changed: has("repeat"), Until: req.GetString("until", ""), Count: int(count),
				}, base, allDay, loc, now)
				if err != nil {
					return validationError(err)
				}
				in.RRule = rule
			}
			anchor, err := input.Anchor(cur, req.GetString("at", ""), now)
			if err != nil {
				return validationError(err)
			}
			reminders, err := input.Reminders(argStrings(args, "reminders"))
			if err != nil {
				return validationError(err)
			}
			e, err := er.Update(ctx, id, in, req.GetString("scope", ""), anchor)
			if err != nil {
				return mcpError(err)
			}
			if has("reminders") {
				if err := er.SetReminders(ctx, e.ID, reminders); err != nil {
					return mcpError(err)
				}
				if e, err = er.FindByID(ctx, e.ID); err != nil {
					return mcpError(err)
				}
			}
			return jsonResult(viewOf(e))
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_event_delete",
			gomcp.WithDescription("Delete an event (it can be restored), or with scope this one instance of a repeat, or with scope following that instance and every later one. Set restore to true to bring a deleted event back. Set purge to true to permanently remove an already deleted event."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Event ID")),
			gomcp.WithString("scope", gomcp.Description("all (default), this or following")),
			gomcp.WithString("at", gomcp.Description("Start of the instance for scope this or following")),
			gomcp.WithBoolean("restore", gomcp.Description("Restore a deleted event instead")),
			gomcp.WithBoolean("purge", gomcp.Description("Permanently delete an already deleted event")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			verb := "deleted"
			switch {
			case req.GetBool("restore", false):
				if err := er.Restore(ctx, id); err != nil {
					return mcpError(err)
				}
				verb = "restored"
			case req.GetBool("purge", false):
				if err := er.Purge(ctx, id); err != nil {
					return mcpError(err)
				}
				verb = "purged"
			default:
				cur, err := er.FindByID(ctx, id)
				if err != nil {
					return mcpError(err)
				}
				anchor, err := input.Anchor(cur, req.GetString("at", ""), calNow())
				if err != nil {
					return validationError(err)
				}
				if err := er.Archive(ctx, id, req.GetString("scope", ""), anchor); err != nil {
					return mcpError(err)
				}
			}
			return gomcp.NewToolResultText(fmt.Sprintf("Event #%d %s", id, verb)), nil
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_event_attendee",
			gomcp.WithDescription("Add a person to an event (or change their role or RSVP), or remove them"),
			gomcp.WithNumber("event_id", gomcp.Required(), gomcp.Description("Event ID")),
			gomcp.WithNumber("person_id", gomcp.Required(), gomcp.Description("Person ID")),
			gomcp.WithBoolean("remove", gomcp.Description("Remove the attendee instead of adding")),
			gomcp.WithString("role", gomcp.Description("organizer or attendee (default)")),
			gomcp.WithString("rsvp", gomcp.Description("needs-action (default), accepted, declined or tentative")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			eid, errResult := requireID(req, "event_id")
			if errResult != nil {
				return errResult, nil
			}
			pid, errResult := requireID(req, "person_id")
			if errResult != nil {
				return errResult, nil
			}
			if req.GetBool("remove", false) {
				if err := er.RemoveAttendee(ctx, eid, pid); err != nil {
					return mcpError(err)
				}
			} else if err := er.SetAttendee(ctx, eid, model.EventAttendee{
				PersonID: pid, Role: req.GetString("role", ""), RSVP: req.GetString("rsvp", ""),
			}); err != nil {
				return mcpError(err)
			}
			e, err := er.FindByID(ctx, eid)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(viewOf(e))
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_cal_import_ics",
			gomcp.WithDescription("Import iCalendar text (events, repeats, exceptions, per-instance changes, tasks). Matched by UID, so importing again updates instead of duplicating. Attendees are matched to people by email and never created; unmatched addresses are listed. Use dry_run to see counts and warnings without writing."),
			gomcp.WithString("ics", gomcp.Required(), gomcp.Description("The iCalendar file content")),
			gomcp.WithString("source", gomcp.Description("Name the UIDs are remembered under (default ics). Use one name per calendar feed")),
			gomcp.WithString("tz", gomcp.Description("Zone for times written without a zone (default: this device)")),
			gomcp.WithBoolean("include_tasks", gomcp.Description("Import VTODO tasks (default true)")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report what would change, write nothing")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			text := req.GetString("ics", "")
			if text == "" {
				return gomcp.NewToolResultError("validation error: ics is required"), nil
			}
			loc, err := tz.Load(req.GetString("tz", ""))
			if err != nil {
				return validationError(err)
			}
			res, err := icsio.Import(ctx, db, []byte(text), icsio.ImportOptions{
				Source: req.GetString("source", ""), DryRun: req.GetBool("dry_run", false), Loc: loc,
				SkipTasks: !req.GetBool("include_tasks", true),
			})
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_cal_export_ics",
			gomcp.WithDescription("Export the calendar as iCalendar text. Events keep the UID they were imported with. Attendees without an email address are left out."),
			gomcp.WithString("from", gomcp.Description("Only events from this date (a date or phrase)")),
			gomcp.WithString("to", gomcp.Description("Only events before this date")),
			gomcp.WithBoolean("include_tasks", gomcp.Description("Include tasks as VTODO")),
			gomcp.WithString("name", gomcp.Description("Calendar name (default crm)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			opt := icsio.ExportOptions{Tasks: req.GetBool("include_tasks", false), Name: req.GetString("name", "crm")}
			from, to := req.GetString("from", ""), req.GetString("to", "")
			if from != "" || to != "" {
				f, t, err := input.Window(from, to, 365, tz.Device(), calNow())
				if err != nil {
					return validationError(err)
				}
				opt.From, opt.To = f, t
			}
			res, err := icsio.Export(ctx, db, opt)
			if err != nil {
				return mcpError(err)
			}
			return gomcp.NewToolResultText(string(res.Data)), nil
		},
	)
}
