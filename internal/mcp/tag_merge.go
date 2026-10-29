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
			gomcp.WithDescription("Rename a tag on every record that has it. The children follow: renaming work to job turns work/clients into job/clients. A name another tag already has, in any case, is refused as a conflict and the message says to use crm_tag_merge, or pass merge true to merge into it (children with the same names merge too)"),
			gomcp.WithString("tag", gomcp.Required(), gomcp.Description("Current tag name")),
			gomcp.WithString("new_name", gomcp.Required(), gomcp.Description("New tag name")),
			gomcp.WithBoolean("merge", gomcp.Description("When the new name exists, merge into that tag instead of refusing")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			from, to := repo.NormalizeTagName(req.GetString("tag", "")), repo.NormalizeTagName(req.GetString("new_name", ""))
			res, err := tr.RenameTree(ctx, from, to, req.GetBool("merge", false))
			if err != nil {
				return treeResult(nil, err)
			}
			return jsonResult(map[string]any{"old": from, "name": to, "renamed": res.Renamed, "merged": res.Merged})
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_tag_merge",
			gomcp.WithDescription("Merge a tag into another: every record tagged source gets target (records that already had it count once) and source is removed. The children of source follow: they become children of target and children with the same names merge. Use dry_run first"),
			gomcp.WithString("source", gomcp.Required(), gomcp.Description("Tag to merge away")),
			gomcp.WithString("target", gomcp.Required(), gomcp.Description("Tag that stays")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report what would change without changing anything")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			src, tgt := req.GetString("source", ""), req.GetString("target", "")
			// A tag with children, or a parent that is no tag, merges as a tree.
			if virtual, _ := tr.IsVirtual(ctx, src); virtual {
				return mergeTreeTool(ctx, tr, src, tgt, req.GetBool("dry_run", false))
			}
			if st, err := tr.SubtreeStat(ctx, src); err == nil && st.Tags > 1 {
				return mergeTreeTool(ctx, tr, src, tgt, req.GetBool("dry_run", false))
			}
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

func mergeTreeTool(ctx context.Context, tr *repo.TagRepo, src, tgt string, dry bool) (*gomcp.CallToolResult, error) {
	if dry {
		return treeResult(tr.MergeTreeDryRun(ctx, src, tgt))
	}
	return treeResult(tr.MergeTree(ctx, src, tgt))
}
