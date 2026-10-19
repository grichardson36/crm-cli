package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerOrgMergeTool adds crm_org_merge (open discrepancy X18).
func registerOrgMergeTool(s *server.MCPServer, db *sql.DB) {
	or := repo.NewOrgRepo(db)
	s.AddTool(
		gomcp.NewTool("crm_org_merge",
			gomcp.WithDescription("Merge duplicate organizations into a survivor. The survivor keeps its fields and empty ones are filled from the duplicates. Memberships move (only an exact duplicate row is dropped), parent links, deal links, task and interaction links and tags move, a merge that would make an organization its own parent is refused. Duplicates are archived and their ids keep resolving to the survivor. Use dry_run first"),
			gomcp.WithNumber("keep_id", gomcp.Required(), gomcp.Description("Survivor organization ID")),
			gomcp.WithArray("drop_ids", gomcp.Required(), gomcp.Description("Organization IDs to merge into the survivor"), gomcp.WithNumberItems()),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report what would change without changing anything")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			keep := int64(req.GetInt("keep_id", 0))
			var drops []int64
			if raw, ok := req.GetArguments()["drop_ids"].([]any); ok {
				for _, v := range raw {
					if f, ok := v.(float64); ok {
						drops = append(drops, int64(f))
					}
				}
			}
			var res *repo.OrgMergeResult
			var err error
			if req.GetBool("dry_run", false) {
				res, err = or.MergeOrgsDryRun(ctx, keep, drops)
			} else {
				res, err = or.MergeOrgs(ctx, keep, drops)
			}
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
}
