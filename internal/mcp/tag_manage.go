package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerTagManageTools adds crm_tag_unused and crm_tag_duplicates (the Tags module).
func registerTagManageTools(s *server.MCPServer, db *sql.DB) {
	tr := repo.NewTagRepo(db)
	s.AddTool(
		gomcp.NewTool("crm_tag_unused",
			gomcp.WithDescription("List the tags no object carries. A dry run unless apply is true, which deletes them"),
			gomcp.WithBoolean("apply", gomcp.Description("Delete the unused tags instead of only listing them")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			res, err := tr.DeleteUnused(ctx, !req.GetBool("apply", false))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_tag_duplicates",
			gomcp.WithDescription("List groups of tags that differ only by case, spaces, hyphens or underscores. Each group names the most used spelling to merge into. A dry run unless apply is true, which merges every group into its most used spelling"),
			gomcp.WithBoolean("apply", gomcp.Description("Merge every group instead of only listing them")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			groups, err := tr.Duplicates(ctx)
			if err != nil {
				return mcpError(err)
			}
			if groups == nil {
				groups = []repo.TagDupGroup{}
			}
			out := map[string]any{"groups": groups, "applied": false}
			if req.GetBool("apply", false) {
				res, err := tr.MergeDuplicates(ctx, false)
				if err != nil {
					return mcpError(err)
				}
				out["applied"], out["merge"] = true, res
			}
			return jsonResult(out)
		},
	)
}
