package mcp

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPurgeTools adds the bulk purge of archived records of any type (crm purge --archived).
func registerPurgeTools(s *server.MCPServer, db *sql.DB) {
	s.AddTool(
		gomcp.NewTool("crm_purge_archived",
			gomcp.WithDescription("Permanently delete archived records of any type in bulk (person, organization, deal, task, note), each with its dependent rows removed and a tombstone recorded for sync. Dry run by default: returns what would be purged. Set apply to true to delete. Cannot be undone"),
			gomcp.WithString("type", gomcp.Description("Record type: person, organization, deal, task or note (default: every type)")),
			gomcp.WithArray("ids", gomcp.Description("Only these archived ids of type (default: every archived record of the type). ids without a type mean people"), gomcp.WithNumberItems()),
			gomcp.WithBoolean("apply", gomcp.Description("Delete for real (default false: dry run)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			var ids []int64
			if raw, ok := req.GetArguments()["ids"].([]any); ok {
				for _, v := range raw {
					f, isNum := v.(float64)
					if !isNum || f <= 0 || f != float64(int64(f)) {
						return gomcp.NewToolResultError(fmt.Sprintf("validation error: ids must be positive integers, got %v", v)), nil
					}
					ids = append(ids, int64(f))
				}
			}
			kind := req.GetString("type", "")
			if len(ids) > 0 && kind == "" {
				kind = "person"
			}
			res, err := repo.PurgeArchived(ctx, db, repo.PurgeArchivedOptions{Kind: kind, IDs: ids, Apply: req.GetBool("apply", false)})
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
}
