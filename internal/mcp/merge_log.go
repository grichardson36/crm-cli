package mcp

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerMergeLogTools adds the persistent merge log (open discrepancy X9) and the "not a
// duplicate" action for organization merge candidates.
func registerMergeLogTools(s *server.MCPServer, db *sql.DB) {
	logs := repo.NewMergeLogRepo(db)
	orgs := repo.NewOrgRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_merge_log",
			gomcp.WithDescription("List logged merges of people and organizations, newest first. Each entry has the survivor, the merged-in records, a summary of what moved and a status (merged or undone). Undo one with crm_merge_undo"),
			gomcp.WithString("kind", gomcp.Description("Only person or organization merges")),
			gomcp.WithNumber("limit", gomcp.Description("Max entries (default 50, 0 for all)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			list, err := logs.List(ctx, req.GetString("kind", ""), req.GetInt("limit", 50))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"count": len(list), "merges": list})
		},
	)

	replay := func(redo bool) server.ToolHandlerFunc {
		return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			var res *repo.MergeReplayResult
			var err error
			if redo {
				res, err = logs.Redo(ctx, id)
			} else {
				res, err = logs.Undo(ctx, id)
			}
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		}
	}
	s.AddTool(
		gomcp.NewTool("crm_merge_undo",
			gomcp.WithDescription("Undo a logged merge (id from crm_merge_log or the log_id of crm_person_merge and crm_org_merge). Works in any later session: the merged records come back, their links move back, the alias is removed and overwritten fields get their old values. A value edited after the merge is left as it is (skipped counts them)"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Merge log id")),
		),
		replay(false),
	)
	s.AddTool(
		gomcp.NewTool("crm_merge_redo",
			gomcp.WithDescription("Repeat a merge that crm_merge_undo reversed"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Merge log id")),
		),
		replay(true),
	)

	s.AddTool(
		gomcp.NewTool("crm_org_dupes_dismiss",
			gomcp.WithDescription("Mark two organizations as not duplicates so the pair stops appearing in crm_org_dupes. Stored as data and reversible: set undo to true to bring the pair back"),
			gomcp.WithNumber("org_a", gomcp.Required(), gomcp.Description("First organization ID")),
			gomcp.WithNumber("org_b", gomcp.Required(), gomcp.Description("Second organization ID")),
			gomcp.WithBoolean("undo", gomcp.Description("Undismiss the pair")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			a, errResult := requireID(req, "org_a")
			if errResult != nil {
				return errResult, nil
			}
			b, errResult := requireID(req, "org_b")
			if errResult != nil {
				return errResult, nil
			}
			verb := "dismissed"
			var err error
			if req.GetBool("undo", false) {
				err = orgs.UndismissOrgPair(ctx, a, b)
				verb = "undismissed"
			} else {
				_, err = orgs.DismissOrgPair(ctx, a, b)
			}
			if err != nil {
				return mcpError(err)
			}
			return gomcp.NewToolResultText(fmt.Sprintf("Organizations #%d and #%d %s", a, b, verb)), nil
		},
	)
}
