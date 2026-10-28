package mcp

import (
	"context"
	"database/sql"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerDealStageTools adds the deal stage tools (crm deal stages list|add|rename|remove|reorder|
// outcome). They are thin wrappers over repo.VocabRepo, like the CLI.
func registerDealStageTools(s *server.MCPServer, db *sql.DB) {
	vr := repo.NewVocabRepo(db)
	kind := model.DealStageKind
	missing := func(what string) (*gomcp.CallToolResult, error) {
		return gomcp.NewToolResultError("validation error: " + what + " is required: " + model.ErrValidation.Error()), nil
	}

	s.AddTool(
		gomcp.NewTool("crm_deal_stage_list",
			gomcp.WithDescription("List the deal stages in pipeline order with the outcome of each (open, won or lost) and the number of deals in it. The stage of a deal must be one of these. The outcome, not the name, decides what closes a deal"),
			gomcp.WithBoolean("all", gomcp.Description("Include removed stages")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			l, err := vr.List(ctx, kind, req.GetBool("all", false))
			if err != nil {
				return mcpError(err)
			}
			if l == nil {
				l = []*model.VocabEntry{}
			}
			return jsonResult(l)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_deal_stage_add",
			gomcp.WithDescription("Add an open deal stage at the end of the list. Use crm_deal_stage_outcome to make it won or lost. A name already on the list is E_VOCAB_DUPLICATE"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Stage name, up to 40 characters, no | and no line breaks")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			n, err := req.RequireString("name")
			if err != nil {
				return missing("name")
			}
			e, err := vr.Add(ctx, kind, n)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(e)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_deal_stage_rename",
			gomcp.WithDescription("Rename a deal stage and every deal in it"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Current stage name")),
			gomcp.WithString("new_name", gomcp.Required(), gomcp.Description("New stage name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			n, err1 := req.RequireString("name")
			nn, err2 := req.RequireString("new_name")
			if err1 != nil || err2 != nil {
				return missing("name and new_name")
			}
			res, err := vr.Rename(ctx, kind, n, nn)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_deal_stage_remove",
			gomcp.WithDescription("Remove a deal stage. When deals are in it, replace_with names another stage they move to (E_VOCAB_IN_USE without it). At least one open, one won and one lost stage must stay (E_STAGE_NO_OPEN, E_STAGE_NO_WON, E_STAGE_NO_LOST)"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Stage name")),
			gomcp.WithString("replace_with", gomcp.Description("Another listed stage to move the deals to")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			n, err := req.RequireString("name")
			if err != nil {
				return missing("name")
			}
			res, err := vr.Remove(ctx, kind, n, req.GetString("replace_with", ""), false)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_deal_stage_reorder",
			gomcp.WithDescription("Reorder the deal stages. names puts the named stages first in the given order and keeps the rest after them. Or give one name and position to move that stage to a 1-based position"),
			gomcp.WithArray("names", gomcp.Items(map[string]any{"type": "string"}), gomcp.Description("Stages to put first, in order")),
			gomcp.WithString("name", gomcp.Description("One stage to move (with position)")),
			gomcp.WithNumber("position", gomcp.Description("1-based position for name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			var err error
			if n := strings.TrimSpace(req.GetString("name", "")); n != "" {
				err = vr.Move(ctx, kind, n, req.GetInt("position", 0))
			} else {
				names := req.GetStringSlice("names", nil)
				if len(names) == 0 {
					return missing("names, or name and position")
				}
				err = vr.Reorder(ctx, kind, names)
			}
			if err != nil {
				return mcpError(err)
			}
			l, err := vr.List(ctx, kind, false)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(l)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_deal_stage_outcome",
			gomcp.WithDescription("Set the outcome of a deal stage: open (in the pipeline), won or lost (closed). Deals moved into a won or lost stage get closed_at. At least one open, one won and one lost stage must exist (E_STAGE_NO_OPEN, E_STAGE_NO_WON, E_STAGE_NO_LOST)"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Stage name")),
			gomcp.WithString("outcome", gomcp.Required(), gomcp.Description("open, won or lost")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			n, err1 := req.RequireString("name")
			o, err2 := req.RequireString("outcome")
			if err1 != nil || err2 != nil {
				return missing("name and outcome")
			}
			res, err := vr.SetOutcome(ctx, kind, n, o)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
}
