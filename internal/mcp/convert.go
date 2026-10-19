package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerConvertTools adds the company text to organization review (owner decision Q21). Looking
// creates nothing. crm_org_convert_apply needs an explicit decision per group.
func registerConvertTools(s *server.MCPServer, db *sql.DB) {
	cr := repo.NewConvertRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_org_convert_list",
			gomcp.WithDescription("List the review groups of people's company text, biggest first, with the suggested existing organization (match kind exact, alias or fuzzy; only exact and alias are safe to accept in bulk). Each group also has domain, the dominant non-free email domain of its people, and domain_match, an existing organization with that domain (a suggestion only, never part of accept_exact). Returns stats (people with company text, groups, matched) and the groups. Creates nothing"),
			gomcp.WithNumber("limit", gomcp.Description("Max groups (default 50, 0 for all)")),
			gomcp.WithNumber("min_people", gomcp.Description("Only groups with at least this many people")),
			gomcp.WithBoolean("include_skipped", gomcp.Description("Also list groups that were skipped")),
			gomcp.WithBoolean("review_only", gomcp.Description("Only groups with people tagged org-review (a phone ORG that matched nothing)")),
			gomcp.WithBoolean("with_people", gomcp.Description("List the people of each group")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			groups, stats, err := cr.Groups(ctx, req.GetBool("include_skipped", false))
			if err != nil {
				return mcpError(err)
			}
			limit := req.GetInt("limit", 50)
			minP := req.GetInt("min_people", 0)
			rows := []map[string]any{}
			for _, g := range groups {
				if g.Count < minP || (req.GetBool("review_only", false) && g.Review == 0) {
					continue
				}
				rows = append(rows, g.Record(req.GetBool("with_people", false)))
				if limit > 0 && len(rows) >= limit {
					break
				}
			}
			return jsonResult(map[string]any{"stats": stats, "line": stats.Line(), "groups": rows})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_org_convert_apply",
			gomcp.WithDescription("Apply conversion decisions in one batch. Each decision is {key, action, org_id, name, domain}: action create makes an organization (name defaults to the most common spelling, domain to the group's dominant non-free email domain) and links the group, link links to org_id, skip hides the group until its text changes, unskip brings it back. A person gets a membership with their title as the role, primary when they have none, and loses the org-review tag. people.company stays. accept_exact adds a link decision for every exact or alias match. dry_run previews without writing. Nothing is created without a decision"),
			gomcp.WithArray("decisions", gomcp.Description("Decisions, one per group key")),
			gomcp.WithBoolean("accept_exact", gomcp.Description("Also link every group that matches an existing organization exactly")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Preview only")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			var decisions []repo.ConvertDecision
			if raw, ok := req.GetArguments()["decisions"]; ok && raw != nil {
				b, err := json.Marshal(raw)
				if err != nil {
					return gomcp.NewToolResultError("validation error: decisions must be a list of objects"), nil
				}
				if err := json.Unmarshal(b, &decisions); err != nil {
					return gomcp.NewToolResultError("validation error: decisions must be a list of {key, action, org_id, name, domain}: " + err.Error()), nil
				}
			}
			if req.GetBool("accept_exact", false) {
				groups, _, err := cr.Groups(ctx, false)
				if err != nil {
					return mcpError(err)
				}
				decisions = append(decisions, repo.ExactDecisions(groups)...)
			}
			if len(decisions) == 0 {
				return gomcp.NewToolResultError(fmt.Sprintf("validation error: give decisions or accept_exact: %v", model.ErrValidation)), nil
			}
			plan, err := cr.Plan(ctx, decisions)
			if err != nil {
				return mcpError(err)
			}
			out := map[string]any{"plan": plan, "applied": false}
			if !req.GetBool("dry_run", false) {
				ch, err := cr.Apply(ctx, decisions)
				if err != nil {
					return mcpError(err)
				}
				out["applied"], out["result"] = true, ch
			}
			return jsonResult(out)
		},
	)
}
