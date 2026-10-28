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

// registerDealRoleTools adds the deal role vocabulary tools (crm deal roles list|add|rename|remove|
// reorder|backfill). They are thin wrappers over repo.VocabRepo, like the CLI.
func registerDealRoleTools(s *server.MCPServer, db *sql.DB) {
	vr := repo.NewVocabRepo(db)
	kind := model.DealRoleKind
	missing := func(what string) (*gomcp.CallToolResult, error) {
		return gomcp.NewToolResultError("validation error: " + what + " is required: " + model.ErrValidation.Error()), nil
	}

	s.AddTool(
		gomcp.NewTool("crm_deal_role_list",
			gomcp.WithDescription("List the deal roles in pick order (champion, decision maker, economic buyer ...) with the number of deal links that use each. The role of a person on a deal must be one of these (or created with create). The role at a company (a membership) is free text and not on this list"),
			gomcp.WithBoolean("all", gomcp.Description("Include removed roles")),
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
		gomcp.NewTool("crm_deal_role_add",
			gomcp.WithDescription("Add a deal role at the end of the list. A name already on the list is E_VOCAB_DUPLICATE"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Role name, up to 80 characters, no | and no line breaks")),
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
		gomcp.NewTool("crm_deal_role_rename",
			gomcp.WithDescription("Rename a deal role and rewrite every deal link that uses it"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Current role name")),
			gomcp.WithString("new_name", gomcp.Required(), gomcp.Description("New role name")),
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
		gomcp.NewTool("crm_deal_role_remove",
			gomcp.WithDescription("Remove a deal role from the list. When deal links still use it, pass replace_with (they move to that role) or keep=true (they keep the text as plain text). Neither is E_VOCAB_IN_USE"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Role name")),
			gomcp.WithString("replace_with", gomcp.Description("Another listed role to move the links to")),
			gomcp.WithBoolean("keep", gomcp.Description("Leave the links as plain text")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			n, err := req.RequireString("name")
			if err != nil {
				return missing("name")
			}
			res, err := vr.Remove(ctx, kind, n, req.GetString("replace_with", ""), req.GetBool("keep", false))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_deal_role_reorder",
			gomcp.WithDescription("Reorder the deal roles. names puts the named roles first in the given order and keeps the rest after them. Or give one name and position to move that role to a 1-based position"),
			gomcp.WithArray("names", gomcp.Items(map[string]any{"type": "string"}), gomcp.Description("Roles to put first, in order")),
			gomcp.WithString("name", gomcp.Description("One role to move (with position)")),
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
		gomcp.NewTool("crm_deal_role_backfill",
			gomcp.WithDescription("Add the free-text roles already on deal links to the deal role list. Dry run by default: returns counts (rows, distinct, already_listed, removed_skipped, invalid_skipped) and the roles to add. apply=true adds them in one step. Deal links are not changed and a second run adds nothing"),
			gomcp.WithBoolean("apply", gomcp.Description("Write the changes (default false)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			apply := req.GetBool("apply", false)
			var rep repo.VocabBackfillReport
			var err error
			if apply {
				rep, _, err = vr.BackfillApply(ctx, kind)
			} else {
				rep, err = vr.BackfillPlan(ctx, kind)
			}
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"applied": apply, "counts": map[string]any{
				"rows": rep.Rows, "distinct": rep.Distinct, "already_listed": rep.Listed, "removed_skipped": rep.Removed,
				"invalid_skipped": rep.Invalid, "to_add": len(rep.ToAdd)}, "roles": rep.ToAdd})
		},
	)
}
