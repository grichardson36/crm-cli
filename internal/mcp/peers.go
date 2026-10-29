package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPeersTools adds the read-only view of device sync. Pulling, pushing and deciding review
// items change data and stay in the CLI (`crm peers`).
func registerPeersTools(s *server.MCPServer, db *sql.DB) {
	s.AddTool(
		gomcp.NewTool("crm_peers_status",
			gomcp.WithDescription("State of device-to-device sync: whether it is on, this device id, the cursor, stored changes, open review items, blocked records and the known devices with their last contact. Read only."),
		),
		func(ctx context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			if !replicasql.IsEnabled(db) {
				return jsonResult(map[string]any{"enabled": false, "hint": "run `crm peers init`"})
			}
			a, err := replicasql.Open(ctx, db, replicasql.DefaultRegistry())
			if err != nil {
				return mcpError(err)
			}
			st, err := a.Status(ctx)
			if err != nil {
				return mcpError(err)
			}
			devs, err := a.Devices(ctx)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"enabled": true, "status": st, "devices": devs})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_peers_review",
			gomcp.WithDescription("Records that one device deleted and another edited. They stay visible with the edits until a person decides with `crm peers resolve ID --keep` or `--delete`. Read only."),
			gomcp.WithBoolean("include_resolved", gomcp.Description("Also list items already decided")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			if !replicasql.IsEnabled(db) {
				return jsonResult([]any{})
			}
			a, err := replicasql.Open(ctx, db, replicasql.DefaultRegistry())
			if err != nil {
				return mcpError(err)
			}
			list, err := a.Reviews(ctx, req.GetBool("include_resolved", false))
			if err != nil {
				return mcpError(err)
			}
			if list == nil {
				list = []replicasql.Review{}
			}
			return jsonResult(list)
		},
	)
}
