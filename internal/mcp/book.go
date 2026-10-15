package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jdanielnd/crm-cli/internal/book"
	"github.com/jdanielnd/crm-cli/internal/cal/ics"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerBookTools(s *server.MCPServer, db *sql.DB) {
	br := repo.NewBookingRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_book_add",
			gomcp.WithDescription("Create a booking link (Tier 0, no server). A link says which times may be offered: lengths, working hours rule, notice, horizon, step, buffers, per day/week/month caps, questions. Offer times with crm_book_offer, export a page with crm_book_export, book a reply with crm_book_confirm. Use dry_run to check the settings."),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Private name")),
			gomcp.WithString("title", gomcp.Description("Name the invitee sees (default: the name)")),
			gomcp.WithString("slug", gomcp.Description("Short id (default: from the name)")),
			gomcp.WithString("description", gomcp.Description("Text under the title")),
			gomcp.WithString("email", gomcp.Description("Host address for mail links and invitations")),
			gomcp.WithString("lengths", gomcp.Description("Comma separated meeting lengths, the first is the default, e.g. 30m,45m (default 30m)")),
			gomcp.WithString("rule", gomcp.Description("Working hours rule (default: the default rule)")),
			gomcp.WithNumber("horizon_days", gomcp.Description("Days ahead (default 14)")),
			gomcp.WithString("from", gomcp.Description("Offer nothing before this date, YYYY-MM-DD")),
			gomcp.WithString("notice", gomcp.Description("Minimum notice, e.g. 4h (default 4h)")),
			gomcp.WithNumber("step", gomcp.Description("Slot start step in minutes (default 30)")),
			gomcp.WithNumber("buffer", gomcp.Description("Minutes kept free before and after")),
			gomcp.WithNumber("buffer_before", gomcp.Description("Minutes kept free before")),
			gomcp.WithNumber("buffer_after", gomcp.Description("Minutes kept free after")),
			gomcp.WithNumber("max_per_day", gomcp.Description("Cap per day")),
			gomcp.WithNumber("max_per_week", gomcp.Description("Cap per week")),
			gomcp.WithNumber("max_per_month", gomcp.Description("Cap per month")),
			gomcp.WithBoolean("busy_tasks", gomcp.Description("Also keep planned task time free")),
			gomcp.WithString("where", gomcp.Description("Address, phone or room link")),
			gomcp.WithString("where_kind", gomcp.Description("phone, in_person or url")),
			gomcp.WithArray("questions", gomcp.Description("Questions for the invitee, a trailing ! makes one required"), gomcp.WithStringItems()),
			gomcp.WithArray("reminders", gomcp.Description("Event reminders before each meeting, e.g. 1d, 1h"), gomcp.WithStringItems()),
			gomcp.WithString("followup", gomcp.Description("Follow-up task due this long after the meeting, e.g. 1d")),
			gomcp.WithNumber("max_uses", gomcp.Description("Close the link after this many bookings (1 = one-time)")),
			gomcp.WithString("expires", gomcp.Description("Close the link at this date or time")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Validate and show the link, write nothing")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			args := req.GetArguments()
			num := func(k string) int { v, _ := argInt64(args, k); return int(v) }
			sp := book.LinkSpec{
				Name: req.GetString("name", ""), Title: req.GetString("title", ""), Slug: req.GetString("slug", ""),
				Description: req.GetString("description", ""), Email: req.GetString("email", ""), Lens: req.GetString("lengths", ""),
				Rule: req.GetString("rule", ""), Notice: req.GetString("notice", ""), Followup: req.GetString("followup", ""),
				From: req.GetString("from", ""), Expires: req.GetString("expires", ""), Where: req.GetString("where", ""),
				WhereKind: req.GetString("where_kind", ""), Horizon: num("horizon_days"), Step: num("step"),
				Before: num("buffer_before"), After: num("buffer_after"), Both: num("buffer"), MaxDay: num("max_per_day"),
				MaxWeek: num("max_per_week"), MaxMonth: num("max_per_month"), Uses: num("max_uses"), BusyTasks: req.GetBool("busy_tasks", false),
				Questions: argStrings(args, "questions"), Reminds: argStrings(args, "reminders"),
			}
			in, err := sp.Input(calNow(), tz.Device())
			if err != nil {
				return mcpError(err)
			}
			if req.GetBool("dry_run", false) {
				return jsonResult(map[string]any{"dry_run": true, "link": in})
			}
			l, err := br.CreateLink(ctx, in)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(l)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_book_list",
			gomcp.WithDescription("List booking links, or with bookings true the booked meetings (optionally of one link)."),
			gomcp.WithBoolean("bookings", gomcp.Description("List booked meetings instead of links")),
			gomcp.WithString("link", gomcp.Description("Only bookings of this link (slug or ID)")),
			gomcp.WithBoolean("include_cancelled", gomcp.Description("Include cancelled bookings")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			if !req.GetBool("bookings", false) {
				links, err := br.ListLinks(ctx)
				if err != nil {
					return mcpError(err)
				}
				return jsonResult(links)
			}
			f := repo.BookingFilters{All: req.GetBool("include_cancelled", false)}
			if ref := req.GetString("link", ""); ref != "" {
				l, err := br.FindLink(ctx, ref)
				if err != nil {
					return mcpError(err)
				}
				f.LinkID = l.ID
			}
			list, err := br.ListBookings(ctx, f)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(list)
		},
	)

	slots := func(ctx context.Context, req gomcp.CallToolRequest, defCount int) (*book.Result, error) {
		l, err := br.FindLink(ctx, req.GetString("link", ""))
		if err != nil {
			return nil, err
		}
		n := 0
		if v := req.GetString("length", ""); v != "" {
			d, err := when.ParseLength(v)
			if err != nil {
				return nil, err
			}
			n = int(d / time.Minute)
		}
		res, err := book.Slots(ctx, db, l, n, calNow())
		if err != nil {
			return nil, err
		}
		res.Limit(req.GetInt("count", defCount))
		return res, nil
	}

	s.AddTool(
		gomcp.NewTool("crm_book_offer",
			gomcp.WithDescription("The next open times of a booking link, as data and as plain text ready to paste into a message. Times honour working hours, notice, horizon, step, buffers, caps and existing bookings. Event titles are never shown."),
			gomcp.WithString("link", gomcp.Required(), gomcp.Description("Link slug or ID")),
			gomcp.WithString("length", gomcp.Description("Meeting length (default: the link's first)")),
			gomcp.WithNumber("count", gomcp.Description("How many times (default 10, 0 for all)")),
			gomcp.WithString("tz", gomcp.Description("Also show times in this invitee zone")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			res, err := slots(ctx, req, 10)
			if err != nil {
				return mcpError(err)
			}
			var inv *time.Location
			if name := req.GetString("tz", ""); name != "" {
				if inv, err = tz.Load(name); err != nil {
					return validationError(err)
				}
			}
			text, err := book.Offer(res, inv)
			if err != nil {
				return mcpError(err)
			}
			out := make([]map[string]any, len(res.Slots))
			for i, s := range res.Slots {
				out[i] = map[string]any{"start": tz.FormatInstant(s.Start), "end": tz.FormatInstant(s.End)}
			}
			return jsonResult(map[string]any{"link": res.Link.Slug, "tz": res.TZ, "length_min": res.LengthMin, "reason": res.Reason, "slots": out, "text": text})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_book_export",
			gomcp.WithDescription("Render a booking link as one self-contained HTML page with a mail button per open time. Returns the HTML text. The page has free times only, never event titles. Needs the link's host email."),
			gomcp.WithString("link", gomcp.Required(), gomcp.Description("Link slug or ID")),
			gomcp.WithString("length", gomcp.Description("Meeting length (default: the link's first)")),
			gomcp.WithNumber("count", gomcp.Description("Most times on the page (default 60, 0 for all)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			res, err := slots(ctx, req, 60)
			if err != nil {
				return mcpError(err)
			}
			if err := book.NeedsHost(res.Link); err != nil {
				return mcpError(err)
			}
			page, err := book.HTML(res, calNow())
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"slots": len(res.Slots), "html": string(page)})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_book_confirm",
			gomcp.WithDescription("Book a time an invitee asked for. The time must still be open on the link. It takes the time (two confirmations cannot both get it), creates the event, links the contact matched by email, logs a meeting, optionally adds a follow-up task, and returns the invitation (METHOD:REQUEST .ics text) to attach to the reply. Pass slot (the Slot line from the invitee's mail, slug/instant/length) or link plus at. Use dry_run to check without writing."),
			gomcp.WithString("slot", gomcp.Description("Slot line: slug/2026-10-12T15:00:00Z/30")),
			gomcp.WithString("link", gomcp.Description("Link slug or ID (when no slot)")),
			gomcp.WithString("at", gomcp.Description("Time in the link's zone, e.g. tue 10:00, or an instant (when no slot)")),
			gomcp.WithString("length", gomcp.Description("Meeting length (default: the link's first)")),
			gomcp.WithString("name", gomcp.Description("Invitee name")),
			gomcp.WithString("email", gomcp.Required(), gomcp.Description("Invitee email")),
			gomcp.WithString("tz", gomcp.Description("Invitee zone")),
			gomcp.WithObject("answers", gomcp.Description("Answers to the link's questions, label to text")),
			gomcp.WithNumber("person_id", gomcp.Description("Contact to attach (default: matched by email)")),
			gomcp.WithBoolean("follow_up", gomcp.Description("Also add a follow-up task")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Check and show, write nothing")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			args := req.GetArguments()
			ref := req.GetString("link", "")
			var start time.Time
			lenMin := 0
			if slug, t, n, ok := book.SplitToken(req.GetString("slot", "")); ok {
				ref, start, lenMin = slug, t, n
			}
			l, err := br.FindLink(ctx, ref)
			if err != nil {
				return mcpError(err)
			}
			if v := req.GetString("length", ""); v != "" {
				d, err := when.ParseLength(v)
				if err != nil {
					return validationError(err)
				}
				lenMin = int(d / time.Minute)
			}
			if at := req.GetString("at", ""); at != "" {
				if start, err = book.ResolveStart(ctx, db, l, at, calNow()); err != nil {
					return mcpError(err)
				}
			}
			if start.IsZero() {
				return validationError(errString("pass slot, or link and at"))
			}
			answers := map[string]string{}
			if raw, ok := args["answers"].(map[string]any); ok {
				for k, v := range raw {
					if s, ok := v.(string); ok {
						answers[k] = s
					}
				}
			}
			pid, _ := argInt64(args, "person_id")
			res, err := book.Confirm(ctx, db, book.ConfirmInput{
				Link: l, Start: start, LengthMin: lenMin, Name: req.GetString("name", ""), Email: req.GetString("email", ""),
				TZ: req.GetString("tz", ""), Answers: answers, PersonID: pid, FollowUp: req.GetBool("follow_up", false),
				Now: calNow(), DryRun: req.GetBool("dry_run", false),
			})
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{
				"dry_run": res.DryRun, "booking": res.Booking, "event": res.Event, "person": res.Person, "person_id": res.PersonID,
				"note": res.Note, "task_id": res.TaskID, "invite_ics": string(res.Invite),
			})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_book_cancel",
			gomcp.WithDescription("Cancel a booking and delete its event. Returns the cancellation (METHOD:CANCEL .ics text) to send. Use dry_run to preview."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Booking ID")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Show, write nothing")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, ok := argInt64(req.GetArguments(), "id")
			if !ok {
				return validationError(errString("id must be a number"))
			}
			b, cancel, err := book.Cancel(ctx, db, id, req.GetBool("dry_run", false))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"booking": b, "cancel_ics": string(cancel), "dry_run": req.GetBool("dry_run", false)})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_book_link_update",
			gomcp.WithDescription("Pause or resume a booking link (active), or delete it (delete true). Bookings and events stay."),
			gomcp.WithString("link", gomcp.Required(), gomcp.Description("Link slug or ID")),
			gomcp.WithBoolean("active", gomcp.Description("false pauses, true resumes")),
			gomcp.WithBoolean("delete", gomcp.Description("Delete the link")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			l, err := br.FindLink(ctx, req.GetString("link", ""))
			if err != nil {
				return mcpError(err)
			}
			if req.GetBool("delete", false) {
				if err := br.ArchiveLink(ctx, l.ID); err != nil {
					return mcpError(err)
				}
				return jsonResult(l)
			}
			if _, ok := req.GetArguments()["active"]; ok {
				if err := br.SetLinkActive(ctx, l.ID, req.GetBool("active", true)); err != nil {
					return mcpError(err)
				}
				l.Active = req.GetBool("active", true)
			}
			return jsonResult(l)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_cal_invite",
			gomcp.WithDescription("Write an iCalendar invitation (METHOD:REQUEST, or CANCEL) for an event, to attach to an email. Nothing is stored or sent."),
			gomcp.WithNumber("event_id", gomcp.Required(), gomcp.Description("Event ID")),
			gomcp.WithArray("to", gomcp.Required(), gomcp.Description("Invitee addresses"), gomcp.WithStringItems()),
			gomcp.WithString("from", gomcp.Description("Your address, the organizer")),
			gomcp.WithString("from_name", gomcp.Description("Your name")),
			gomcp.WithBoolean("cancel", gomcp.Description("Make it a cancellation")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			args := req.GetArguments()
			id, ok := argInt64(args, "event_id")
			to := argStrings(args, "to")
			if !ok || len(to) == 0 {
				return validationError(errString("event_id and at least one address in to are required"))
			}
			var inv []ics.Person
			for _, a := range to {
				inv = append(inv, ics.Person{Email: a})
			}
			method := "REQUEST"
			if req.GetBool("cancel", false) {
				method = "CANCEL"
			}
			data, err := book.Invite(ctx, db, id, req.GetString("from", ""), req.GetString("from_name", ""), inv, method)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"method": method, "ics": string(data)})
		},
	)
}

func errString(msg string) error { return fmt.Errorf("%s: %w", msg, model.ErrValidation) }
