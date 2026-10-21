package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerTagMergeTools adds crm_tag_rename and crm_tag_merge (open discrepancy X25).
func registerTagMergeTools(s *server.MCPServer, db *sql.DB) {
	tr := repo.NewTagRepo(db)
	s.AddTool(
		gomcp.NewTool("crm_tag_rename",
			gomcp.WithDescription("Rename a tag on every record that has it. A name another tag already has, in any case, is refused as a conflict and the message says to use crm_tag_merge"),
			gomcp.WithString("tag", gomcp.Required(), gomcp.Description("Current tag name")),
			gomcp.WithString("new_name", gomcp.Required(), gomcp.Description("New tag name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			from, to := repo.NormalizeTagName(req.GetString("tag", "")), repo.NormalizeTagName(req.GetString("new_name", ""))
			if err := tr.Rename(ctx, from, to); err != nil {
				if e, ok := err.(*repo.TagExistsError); ok {
					return gomcp.NewToolResultError("conflict: " + e.Error()), nil
				}
				return mcpError(err)
			}
			return jsonResult(map[string]any{"old": from, "name": to})
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_tag_merge",
			gomcp.WithDescription("Merge a tag into another: every record tagged source gets target (records that already had it count once) and source is removed. Use dry_run first"),
			gomcp.WithString("source", gomcp.Required(), gomcp.Description("Tag to merge away")),
			gomcp.WithString("target", gomcp.Required(), gomcp.Description("Tag that stays")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report what would change without changing anything")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			src, tgt := req.GetString("source", ""), req.GetString("target", "")
			var res *repo.TagMergeResult
			var err error
			if req.GetBool("dry_run", false) {
				res, err = tr.MergeDryRun(ctx, src, tgt)
			} else {
				res, err = tr.Merge(ctx, src, tgt)
			}
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
}
