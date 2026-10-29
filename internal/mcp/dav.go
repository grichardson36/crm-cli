package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/dav"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerDavTools adds the CardDAV server's read tools and the conflict undo. The server
// itself runs as `crm dav serve`, not inside the MCP process.
func registerDavTools(s *server.MCPServer, db *sql.DB) {
	davRepo := repo.NewDavRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_dav_info",
			gomcp.WithDescription("How to connect a phone to this database's CardDAV address book: URLs, user name and step-by-step setup for Android (DAVx5), iOS and macOS. The server runs with `crm dav serve`."),
			gomcp.WithString("base_url", gomcp.Description("Public base URL of the server, such as https://contacts.example.com. Defaults to the local address.")),
			gomcp.WithString("user", gomcp.Description("Account user name (default crm)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			return jsonResult(dav.BuildInfo(req.GetString("base_url", ""), req.GetString("user", ""), dav.BookNameFromDB(ctx, db)))
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_dav_status",
			gomcp.WithDescription("State of the CardDAV sync: contacts exposed to devices, how many a phone has fetched, last request, last fetch and last edit time, and conflict counts."),
		),
		func(ctx context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			st, err := davRepo.Status(ctx)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(st)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_dav_conflicts",
			gomcp.WithDescription("List fields where the phone and the CRM both changed and which side won (the CRM value is kept, the phone value is stored), newest first. Link rows (field _link) record a phone contact attached to an existing person. Undo one with crm_dav_conflict_undo."),
			gomcp.WithBoolean("include_undone", gomcp.Description("Also list conflicts already undone")),
			gomcp.WithNumber("limit", gomcp.Description("Max results (default 50)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			list, err := davRepo.Conflicts(ctx, req.GetBool("include_undone", false), req.GetInt("limit", 50))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(list)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_dav_issues",
			gomcp.WithDescription("List phone vCards the CardDAV server could not read. Such a card is skipped, never read as an emptied contact, and flagged here with the failing text. Open issues only unless include_resolved is set."),
			gomcp.WithBoolean("include_resolved", gomcp.Description("Also list issues already resolved")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			list, err := davRepo.Issues(ctx, req.GetBool("include_resolved", false))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(list)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_dav_conflict_undo",
			gomcp.WithDescription("Apply the stored phone value of a CardDAV conflict (the CRM value was kept). Fails if the field changed again since, or if the conflict is a link. The change reaches the phone on its next sync."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Conflict ID from crm_dav_conflicts")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			c, err := dav.UndoConflict(ctx, db, id)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(c)
		},
	)
}
