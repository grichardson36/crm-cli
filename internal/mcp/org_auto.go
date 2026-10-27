package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const deprecatedConvert = "deprecated: use crm_org_backfill. Company text now creates or reuses organizations automatically, there is no review."

// registerOrgAutoTools adds the company text tools (owner decision 2026-10-27): the backfill of
// existing people, the ignore list for junk company strings and the organization merge candidates.
func registerOrgAutoTools(s *server.MCPServer, db *sql.DB) {
	oa := repo.NewOrgAutoRepo(db)
	ig := repo.NewIgnoreRepo(db)
	orgs := repo.NewOrgRepo(db)

	backfill := func(ctx context.Context, req gomcp.CallToolRequest, apply bool, limit int, note string) (*gomcp.CallToolResult, error) {
		out := map[string]any{"applied": false}
		if note != "" {
			out["note"] = note
		}
		var rep repo.BackfillReport
		if apply {
			r, ch, err := oa.BackfillApply(ctx)
			if err != nil {
				return mcpError(err)
			}
			rep = r
			out["applied"] = true
			out["result"] = map[string]any{"orgs_created": ch.OrgsCreated, "people_linked": ch.Linked, "created": ch.Created}
		} else {
			r, err := oa.BackfillPlan(ctx)
			if err != nil {
				return mcpError(err)
			}
			rep = r
		}
		groups := rep.List
		if limit > 0 && len(groups) > limit {
			groups = groups[:limit]
		}
		rows := make([]map[string]any, len(groups))
		for i, g := range groups {
			rows[i] = g.Record()
		}
		rep.List = nil
		out["counts"], out["line"], out["groups"] = rep, rep.Line(), rows
		return jsonResult(out)
	}

	s.AddTool(
		gomcp.NewTool("crm_org_backfill",
			gomcp.WithDescription("Create or reuse organizations for existing people's company text. Dry run by default: returns counts (people with company text, distinct groups, orgs to create, orgs reused, ignored, people to link) and the groups, groups of 3 or more people first. Set apply to true to write it as one batch: each missing organization is created, each person gets a membership (primary when they have none, title as role), people.company is not changed. Matching is the normalized name (case, punctuation, Inc/LLC endings) or a merge alias, close names never merge. Junk strings on the ignore list (crm_org_ignore_list) create nothing. Idempotent"),
			gomcp.WithBoolean("apply", gomcp.Description("Write the changes (default false: dry run)")),
			gomcp.WithNumber("limit", gomcp.Description("Max groups listed (default 50, 0 for all)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			return backfill(ctx, req, req.GetBool("apply", false), req.GetInt("limit", 50), "")
		},
	)

	// Deprecated names of the removed review. They map to the backfill and keep a dry-run default.
	s.AddTool(
		gomcp.NewTool("crm_org_convert_list",
			gomcp.WithDescription("Deprecated alias of crm_org_backfill (dry run). "+deprecatedConvert),
			gomcp.WithNumber("limit", gomcp.Description("Max groups (default 50, 0 for all)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			return backfill(ctx, req, false, req.GetInt("limit", 50), deprecatedConvert)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_org_convert_apply",
			gomcp.WithDescription("Deprecated alias of crm_org_backfill. Per-group decisions are ignored. Dry run unless dry_run is false. "+deprecatedConvert),
			gomcp.WithBoolean("dry_run", gomcp.Description("Preview only (default true)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			return backfill(ctx, req, !req.GetBool("dry_run", true), 50, deprecatedConvert)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_org_ignore_list",
			gomcp.WithDescription("List the company strings that never create an organization: built-in defaults (N/A, none, self-employed, retired, unemployed, -, ...) and the user's additions. A built-in the user turned off shows active=false"),
		),
		func(ctx context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			list, err := ig.List(ctx)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(list)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_org_ignore_add",
			gomcp.WithDescription("Ignore a company string so it never creates an organization (matched by normalized text, case and punctuation do not matter). Turns a removed built-in back on. Existing organizations and memberships are not touched"),
			gomcp.WithString("text", gomcp.Required(), gomcp.Description("The company text to ignore")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			text, err := req.RequireString("text")
			if err != nil {
				return gomcp.NewToolResultError("validation error: text is required: " + model.ErrValidation.Error()), nil
			}
			changed, err := ig.Add(ctx, text)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"text": text, "changed": changed})
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_org_ignore_remove",
			gomcp.WithDescription("Stop ignoring a company string: a user addition is deleted, a built-in is turned off"),
			gomcp.WithString("text", gomcp.Required(), gomcp.Description("The company text")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			text, err := req.RequireString("text")
			if err != nil {
				return gomcp.NewToolResultError("validation error: text is required: " + model.ErrValidation.Error()), nil
			}
			if err := ig.Remove(ctx, text); err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"text": text, "removed": true})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_org_dupes",
			gomcp.WithDescription("List pairs of organizations that may be the same one: same normalized name, close spelling, one name starting with the other, or the same domain. Best first. Never merged automatically: use crm_org_merge (dry run first)"),
			gomcp.WithNumber("limit", gomcp.Description("Max pairs (default 50, 0 for all)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			c, err := orgs.FindOrgCandidates(ctx, req.GetInt("limit", 50))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"count": len(c), "candidates": c})
		},
	)
}
