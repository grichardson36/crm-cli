package mcp

import (
	"context"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerLogEditTools adds edit and delete for logged interactions.
func registerLogEditTools(s *server.MCPServer, ir *repo.InteractionRepo) {
	s.AddTool(
		gomcp.NewTool("crm_log_update",
			gomcp.WithDescription("Edit a logged interaction. Only the fields you pass change. Empty subject, content or direction clears it."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Interaction ID")),
			gomcp.WithString("type", gomcp.Description("New type, must be an active type from crm_note_types")),
			gomcp.WithString("subject", gomcp.Description("Subject/title")),
			gomcp.WithString("content", gomcp.Description("Content/body")),
			gomcp.WithString("direction", gomcp.Description("inbound or outbound")),
			gomcp.WithString("occurred_at", gomcp.Description("When it occurred (ISO 8601)")),
		),
		logUpdateHandler(ir),
	)

	s.AddTool(
		gomcp.NewTool("crm_log_delete",
			gomcp.WithDescription("Delete (archive) a logged interaction. It leaves lists and search and stops counting toward keep-in-touch. Set restore to true to bring it back."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Interaction ID")),
			gomcp.WithBoolean("restore", gomcp.Description("Restore an archived interaction instead")),
		),
		logDeleteHandler(ir),
	)
}

func logUpdateHandler(ir *repo.InteractionRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		args := req.GetArguments()
		var in model.UpdateInteractionInput
		if v, ok := argString(args, "type"); ok {
			in.Type = &v
		}
		if v, ok := argString(args, "subject"); ok {
			in.Subject = &v
		}
		if v, ok := argString(args, "content"); ok {
			in.Content = &v
		}
		if v, ok := argString(args, "direction"); ok {
			in.Direction = &v
		}
		if v, ok := argString(args, "occurred_at"); ok {
			in.OccurredAt = &v
		}
		if in == (model.UpdateInteractionInput{}) {
			return gomcp.NewToolResultError("validation error: pass at least one field to change"), nil
		}
		updated, err := ir.Update(ctx, id, in)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(updated)
	}
}

func logDeleteHandler(ir *repo.InteractionRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		verb := "deleted"
		var err error
		if req.GetBool("restore", false) {
			err = ir.Restore(ctx, id)
			verb = "restored"
		} else {
			err = ir.Archive(ctx, id)
		}
		if err != nil {
			return mcpError(err)
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Interaction #%d %s", id, verb)), nil
	}
}
