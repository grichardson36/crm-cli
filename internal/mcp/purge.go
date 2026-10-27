package mcp

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPurgeTools adds the bulk purge of archived people (crm purge --archived).
func registerPurgeTools(s *server.MCPServer, db *sql.DB) {
	pr := repo.NewPersonRepo(db)
	s.AddTool(
		gomcp.NewTool("crm_purge_archived",
			gomcp.WithDescription("Permanently delete archived people in bulk, each exactly as crm_person_purge does (dependent rows removed, a tombstone recorded for sync). Dry run by default: returns who would be purged. Set apply to true to delete. Only people can be purged. Cannot be undone"),
			gomcp.WithString("type", gomcp.Description("Record type, person is the only one with a purge")),
			gomcp.WithArray("ids", gomcp.Description("Only these archived person ids (default: every archived person)"), gomcp.WithNumberItems()),
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
			res, err := pr.PurgeArchived(ctx, repo.PurgeArchivedOptions{Kind: req.GetString("type", ""), IDs: ids, Apply: req.GetBool("apply", false)})
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
}
