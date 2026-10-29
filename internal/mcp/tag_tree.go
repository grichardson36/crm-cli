package mcp

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerTagTreeTools adds the nested tag tools (patterns/nested-tags): crm_tag_tree, crm_tag_nest,
// crm_tag_unnest and crm_tag_delete. They call the same repo functions as crm tag tree, nest, unnest and
// delete. crm_tag_rename and crm_tag_merge (tag_merge.go) also move the children of a tag.
func registerTagTreeTools(s *server.MCPServer, db *sql.DB) {
	tr := repo.NewTagRepo(db)
	s.AddTool(
		gomcp.NewTool("crm_tag_tree",
			gomcp.WithDescription("List the tags as a tree. A tag named work/clients/acme is a child of work/clients, which is a child of work. A parent no tag is named after has virtual true. own counts the objects of the tag, total those of the tag and everything below it (an object once). Nodes come in tree order, depth starts at 1"),
			gomcp.WithBoolean("include_unused", gomcp.Description("Include tags no object carries")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			nodes, err := tr.Tree(ctx, req.GetBool("include_unused", false))
			if err != nil {
				return mcpError(err)
			}
			out := make([]map[string]any, len(nodes))
			for i, n := range nodes {
				out[i] = map[string]any{"path": n.Path, "name": n.Name, "depth": n.Depth, "virtual": n.Virtual,
					"own": n.Own, "total": n.Total, "children": n.Children}
			}
			return jsonResult(out)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_tag_nest",
			gomcp.WithDescription("Put tags under a parent tag (a path rename): nesting work under area makes area/work and the children of work follow. The parent needs no tag of its own, an empty parent moves the tags to the top level. A name the parent already holds is a conflict unless merge is true. All or nothing. Use dry_run first"),
			gomcp.WithArray("tags", gomcp.Required(), gomcp.Description("Tags to nest"), gomcp.WithStringItems()),
			gomcp.WithString("parent", gomcp.Description("Parent tag path, empty for the top level")),
			gomcp.WithBoolean("merge", gomcp.Description("Merge into a tag that already has the new name instead of refusing")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report what would change without changing anything")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			tags := req.GetStringSlice("tags", nil)
			parent, merge := req.GetString("parent", ""), req.GetBool("merge", false)
			var res *repo.TagTreeResult
			var err error
			if req.GetBool("dry_run", false) {
				res, err = tr.NestDryRun(ctx, tags, parent, merge)
			} else {
				res, err = tr.Nest(ctx, tags, parent, merge)
			}
			return treeResult(res, err)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_tag_unnest",
			gomcp.WithDescription("Move tags up one level (a/b/c becomes a/c), children follow. A tag at the top level stays. A name that exists is a conflict unless merge is true. Use dry_run first"),
			gomcp.WithArray("tags", gomcp.Required(), gomcp.Description("Tags to unnest"), gomcp.WithStringItems()),
			gomcp.WithBoolean("merge", gomcp.Description("Merge into a tag that already has the new name instead of refusing")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report what would change without changing anything")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			tags, merge := req.GetStringSlice("tags", nil), req.GetBool("merge", false)
			var res *repo.TagTreeResult
			var err error
			if req.GetBool("dry_run", false) {
				res, err = tr.UnnestDryRun(ctx, tags, merge)
			} else {
				res, err = tr.Unnest(ctx, tags, merge)
			}
			return treeResult(res, err)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_tag_delete",
			gomcp.WithDescription("Delete a tag and the tags below it: every object that carries one of them is untagged, as one step (soft deleted, tagging again brings the old tagging back). dry_run reports how many tags and records it would touch. only_this keeps the children"),
			gomcp.WithString("tag", gomcp.Required(), gomcp.Description("Tag path")),
			gomcp.WithBoolean("only_this", gomcp.Description("Delete only this tag, not the tags below it")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report what would be deleted without deleting")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			tag := req.GetString("tag", "")
			if req.GetBool("dry_run", false) {
				st, err := tr.SubtreeStat(ctx, tag)
				if err != nil {
					return mcpError(err)
				}
				return jsonResult(map[string]any{"dry_run": true, "tag": st.Path, "tags": st.Tags, "taggings": st.Taggings, "objects": st.Objects})
			}
			var res *repo.TagDeleteResult
			var err error
			if req.GetBool("only_this", false) {
				res, err = tr.DeleteTag(ctx, tag)
			} else {
				res, err = tr.DeleteTagTree(ctx, tag)
			}
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
}

// treeResult is the result of a tree change as a tool result. A name that is taken is a conflict that
// says to merge.
func treeResult(res *repo.TagTreeResult, err error) (*gomcp.CallToolResult, error) {
	var exists *repo.TagExistsError
	if errors.As(err, &exists) {
		return gomcp.NewToolResultError("conflict: " + exists.Error()), nil
	}
	if err != nil {
		return mcpError(err)
	}
	return jsonResult(res)
}
