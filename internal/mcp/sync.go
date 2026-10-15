package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/jdanielnd/crm-cli/internal/integrations"
	"os"

	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerSyncTools adds the sync runner and inbox tools. Only adapters registered in
// this build can run, and none of them can delete records.
func registerSyncTools(s *server.MCPServer, db *sql.DB) {
	runner := crmsync.NewRunner(db, integrations.DefaultRegistry(os.Getenv))
	inbox := crmsync.NewInbox(db)

	s.AddTool(
		gomcp.NewTool("crm_sync_run",
			gomcp.WithDescription("Pull one external source into the CRM. Matches people by external id, then exact email, phone or handle. Unmatched records go to the sync inbox unless allow_create is set. Never deletes. Use dry_run to see counts without writing."),
			gomcp.WithString("source", gomcp.Required(), gomcp.Description("Source name from crm_sync_status")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report counts, write nothing")),
			gomcp.WithBoolean("allow_create", gomcp.Description("Create people nobody matches instead of queueing them")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			source := req.GetString("source", "")
			if source == "" {
				return gomcp.NewToolResultError("validation error: source is required"), nil
			}
			res, err := runner.Run(ctx, source, crmsync.RunOptions{
				DryRun: req.GetBool("dry_run", false), AllowCreate: req.GetBool("allow_create", false)})
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_sync_status",
			gomcp.WithDescription("Sync sources with their cursor and last run, plus the number of records waiting in the sync inbox"),
		),
		func(ctx context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			o, err := runner.Overview(ctx)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(o)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_sync_inbox_list",
			gomcp.WithDescription("List records the sync runner could not place. Each can be accepted (create a person), attached to a person, or ignored."),
			gomcp.WithString("status", gomcp.Description("pending (default), accepted, attached, ignored, or all")),
			gomcp.WithNumber("limit", gomcp.Description("Max results (default 50)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			status := req.GetString("status", "pending")
			if status == "all" {
				status = ""
			}
			items, err := inbox.List(ctx, status, req.GetInt("limit", 50))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(items)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_sync_inbox_accept",
			gomcp.WithDescription("Create a new person from a pending inbox record"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Inbox item ID")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			personID, err := inbox.Accept(ctx, id)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"inbox_id": id, "person_id": personID, "status": "accepted"})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_sync_inbox_attach",
			gomcp.WithDescription("Attach a pending inbox record to an existing person. Person records fill blank fields and remember the external id. Interaction records are logged on that person."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Inbox item ID")),
			gomcp.WithNumber("person_id", gomcp.Required(), gomcp.Description("Person ID")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			personID, errResult := requireID(req, "person_id")
			if errResult != nil {
				return errResult, nil
			}
			if err := inbox.Attach(ctx, id, personID); err != nil {
				return mcpError(err)
			}
			return gomcp.NewToolResultText(fmt.Sprintf("Inbox item #%d attached to person #%d", id, personID)), nil
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_sync_inbox_ignore",
			gomcp.WithDescription("Dismiss a pending inbox record. Later syncs leave it dismissed."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Inbox item ID")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			if err := inbox.Ignore(ctx, id); err != nil {
				return mcpError(err)
			}
			return gomcp.NewToolResultText(fmt.Sprintf("Inbox item #%d ignored", id)), nil
		},
	)
}
