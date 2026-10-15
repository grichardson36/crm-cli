package mcp

import (
	"context"
	"database/sql"
	"time"

	"github.com/jdanielnd/crm-cli/internal/book"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/remind"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerRemindTools(s *server.MCPServer, db *sql.DB) {
	s.AddTool(
		gomcp.NewTool("crm_cal_due_reminders",
			gomcp.WithDescription("Event reminders that are due now: the fire time (start minus the reminder) has passed and the event has not started. Each reminder is returned once, then marked as handed out, so a second call returns nothing. Deliver them yourself (notify the owner, or for method email write to the people and guests). dry_run lists without marking."),
			gomcp.WithString("within", gomcp.Description("Also include reminders falling due within this time, e.g. 15m")),
			gomcp.WithBoolean("dry_run", gomcp.Description("List without marking as handed out")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			var w time.Duration
			if v := req.GetString("within", ""); v != "" {
				d, err := when.ParseLength(v)
				if err != nil {
					return validationError(err)
				}
				w = d
			}
			list, err := remind.List(ctx, db, calNow(), w, tz.Device())
			if err != nil {
				return mcpError(err)
			}
			if !req.GetBool("dry_run", false) {
				if err := remind.Mark(ctx, db, list); err != nil {
					return mcpError(err)
				}
			}
			return jsonResult(list)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_book_remind",
			gomcp.WithDescription("Draft the booking messages that are due: reminders before a confirmed booking (the link's reminders) and a follow-up after it (the link's followup). Each draft has to, reply_to, subject and body, with how to cancel. Each is returned once, then marked. Nothing is sent: send them with a mail tool. dry_run drafts without marking."),
			gomcp.WithBoolean("dry_run", gomcp.Description("Draft without marking as handed out")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			drafts, err := book.Drafts(ctx, db, calNow())
			if err != nil {
				return mcpError(err)
			}
			if !req.GetBool("dry_run", false) {
				if err := book.Mark(ctx, db, drafts); err != nil {
					return mcpError(err)
				}
			}
			return jsonResult(drafts)
		},
	)
}
