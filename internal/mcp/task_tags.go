package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerTaskTagTools adds crm_task_tag and crm_task_untag. crm_tag_apply also accepts "task".
func registerTaskTagTools(s *server.MCPServer, db *sql.DB) {
	tagr := repo.NewTagRepo(db)
	tr := repo.NewTaskRepo(db)
	handler := func(remove bool) server.ToolHandlerFunc {
		return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			tags := req.GetStringSlice("tags", nil)
			if len(tags) == 0 {
				return gomcp.NewToolResultError("validation error: pass at least one tag"), nil
			}
			for _, name := range tags {
				var err error
				if remove {
					err = tagr.Remove(ctx, "task", id, name)
				} else {
					err = tagr.Apply(ctx, "task", id, name)
				}
				if err != nil {
					return mcpError(err)
				}
			}
			task, err := tr.FindByID(ctx, id)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(task)
		}
	}
	s.AddTool(
		gomcp.NewTool("crm_task_tag",
			gomcp.WithDescription("Put tags on a task. Unknown tags are created."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Task ID")),
			gomcp.WithArray("tags", gomcp.Required(), gomcp.Description("Tag names"), gomcp.WithStringItems()),
		),
		handler(false),
	)
	s.AddTool(
		gomcp.NewTool("crm_task_untag",
			gomcp.WithDescription("Remove tags from a task."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Task ID")),
			gomcp.WithArray("tags", gomcp.Required(), gomcp.Description("Tag names"), gomcp.WithStringItems()),
		),
		handler(true),
	)
}
