package mcp

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerDedupeTools adds the duplicate detection and merge tools.
func registerDedupeTools(s *server.MCPServer, db *sql.DB) {
	dr := repo.NewDupeRepo(db)
	pr := repo.NewPersonRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_dupes",
			gomcp.WithDescription("List groups of probable duplicate people, highest confidence first. Levels: high (shared email/phone/social handle), medium (same normalized name), low (nickname), fuzzy (similar spelling)."),
			gomcp.WithString("min", gomcp.Description("Lowest level: high, medium, low, fuzzy (default medium)")),
			gomcp.WithNumber("limit", gomcp.Description("Max groups (default 50)")),
		),
		dupesHandler(dr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_merge",
			gomcp.WithDescription("Merge duplicate people into a survivor. Lists are combined, records move to the survivor, dropped people are archived with merged_into set. The result has a log_id: crm_merge_undo reverses the merge later. Use dry_run first."),
			gomcp.WithNumber("keep_id", gomcp.Required(), gomcp.Description("Survivor person ID")),
			gomcp.WithArray("drop_ids", gomcp.Required(), gomcp.Description("Person IDs to merge into the survivor"), gomcp.WithNumberItems()),
			gomcp.WithObject("choices", gomcp.Description("Per-field person ID to take the value from, e.g. {\"title\": 7, \"linkedin\": 7}. Fields: first_name, last_name, title, company, location, notes, summary, birthday, keep_in_touch, keep_in_touch_snoozed_until, last_contacted_at, is_favorite, priority_tier, email, phone, or a handle kind")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Report what would change without changing anything")),
		),
		personMergeHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_dupes_dismiss",
			gomcp.WithDescription("Mark two people as not duplicates so they stop appearing in crm_dupes. Set undo to true to reverse."),
			gomcp.WithNumber("person_a", gomcp.Required(), gomcp.Description("First person ID")),
			gomcp.WithNumber("person_b", gomcp.Required(), gomcp.Description("Second person ID")),
			gomcp.WithBoolean("undo", gomcp.Description("Undismiss the pair")),
		),
		dupesDismissHandler(dr),
	)
}

func dupesHandler(dr *repo.DupeRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		min, err := dedupe.ParseLevel(req.GetString("min", "medium"))
		if err != nil {
			return gomcp.NewToolResultError("validation error: " + err.Error()), nil
		}
		groups, err := dr.Find(ctx, min)
		if err != nil {
			return mcpError(err)
		}
		limit := req.GetInt("limit", 50)
		if limit > 0 && len(groups) > limit {
			groups = groups[:limit]
		}
		out := make([]map[string]any, len(groups))
		for i, g := range groups {
			out[i] = map[string]any{"level": g.Level.String(), "ids": g.IDs(), "members": g.Members, "reasons": g.Reasons}
		}
		return jsonResult(out)
	}
}

func personMergeHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		keep, errResult := requireID(req, "keep_id")
		if errResult != nil {
			return errResult, nil
		}
		args := req.GetArguments()
		rawDrops, _ := args["drop_ids"].([]any)
		if len(rawDrops) == 0 {
			return gomcp.NewToolResultError("drop_ids must be a non-empty array of person IDs"), nil
		}
		drops := make([]int64, 0, len(rawDrops))
		for _, d := range rawDrops {
			f, ok := d.(float64)
			if !ok || f != float64(int64(f)) || f <= 0 {
				return gomcp.NewToolResultError("drop_ids must contain positive integers"), nil
			}
			drops = append(drops, int64(f))
		}
		choices := map[string]int64{}
		if raw, ok := args["choices"].(map[string]any); ok {
			for k, v := range raw {
				id, ok := argInt64(map[string]any{"v": v}, "v")
				if !ok || id <= 0 {
					return gomcp.NewToolResultError(fmt.Sprintf("choices.%s must be a person ID", k)), nil
				}
				choices[k] = id
			}
		}
		var res *model.MergeResult
		var err error
		if req.GetBool("dry_run", false) {
			res, err = pr.MergeDryRun(ctx, keep, drops, choices)
		} else {
			res, err = pr.Merge(ctx, keep, drops, choices)
		}
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(res)
	}
}

func dupesDismissHandler(dr *repo.DupeRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		a, errResult := requireID(req, "person_a")
		if errResult != nil {
			return errResult, nil
		}
		b, errResult := requireID(req, "person_b")
		if errResult != nil {
			return errResult, nil
		}
		var err error
		verb := "dismissed"
		if req.GetBool("undo", false) {
			err = dr.Undismiss(ctx, a, b)
			verb = "undismissed"
		} else {
			err = dr.Dismiss(ctx, a, b)
		}
		if err != nil {
			return mcpError(err)
		}
		return gomcp.NewToolResultText(fmt.Sprintf("People #%d and #%d %s", a, b, verb)), nil
	}
}
