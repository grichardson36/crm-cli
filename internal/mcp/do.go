package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/quick"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerDoTools adds crm_do, the one-line command that the CLI (crm do) and the TUI command bar share.
func registerDoTools(s *server.MCPServer, db *sql.DB) {
	s.AddTool(
		gomcp.NewTool("crm_do",
			gomcp.WithDescription("Run one line of text. Verbs: n/note, t/task, log <type>, set, tag, snooze. "+
				"Examples: \"n Sam called about the deal #deal remind fri\", \"t Sam send contract by mon !high\", "+
				"\"set Sam: company=Acme title=CTO\", \"tag Sam vip -old\", \"snooze Sam 2w\". "+
				"A name after the verb picks the person, or pass person_id. Short syntax: #tag @Name !high ~30m by fri every 2 weeks remind fri, and plain dates (fri 3pm) in tasks and logs. "+
				"Use dry_run to see what would happen first. A name that matches no one or several people returns problems with candidates and changes nothing."),
			gomcp.WithString("text", gomcp.Required(), gomcp.Description("The line, starting with a verb")),
			gomcp.WithNumber("person_id", gomcp.Description("The person the line is about, so it needs no name")),
			gomcp.WithBoolean("dry_run", gomcp.Description("Only report what would happen")),
			gomcp.WithBoolean("no_syntax", gomcp.Description("Treat #, @, !, ~, by, remind, every and dates as plain text")),
			gomcp.WithBoolean("keep_unresolved", gomcp.Description("Keep names that match nobody or several people as plain text instead of returning them as problems")),
			gomcp.WithBoolean("link_new", gomcp.Description("Create a contact for each @name that matches nobody, and link it. Never done unless true")),
		),
		doHandler(db),
	)
}

func doHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		text := req.GetString("text", "")
		if text == "" {
			return gomcp.NewToolResultError("text is required"), nil
		}
		personID := int64(req.GetInt("person_id", 0))
		if personID < 0 {
			return gomcp.NewToolResultError("person_id must be a positive integer"), nil
		}
		if personID > 0 {
			if _, err := repo.LookupRecord(ctx, db, "person", personID); err != nil {
				return mcpError(err)
			}
		}
		env, err := quick.LoadEnv(ctx, db, time.Now(), personID)
		if err != nil {
			return mcpError(err)
		}
		env.NoSyntax = req.GetBool("no_syntax", false)
		plan := quick.Parse(text, env)
		dry := req.GetBool("dry_run", false)
		if len(plan.Names) > 0 && (req.GetBool("keep_unresolved", false) || req.GetBool("link_new", false)) {
			var created []string
			var err error
			plan, created, err = quick.SettleNames(ctx, quick.NewRepos(db), text, env, plan, req.GetBool("keep_unresolved", false), req.GetBool("link_new", false), dry)
			if err != nil {
				return mcpError(err)
			}
			for _, name := range created {
				if dry {
					plan.Warnings = append(plan.Warnings, "would create a contact for "+name)
				} else {
					plan.Warnings = append(plan.Warnings, "created a contact for "+name)
				}
			}
		}
		if plan.Verb == "" {
			return gomcp.NewToolResultError("validation error: the text must start with a verb: n, note, t, task, log, set, tag or snooze"), nil
		}
		if len(plan.Problems) > 0 {
			return doError(quick.OutputOf(plan, nil))
		}
		if !plan.Ready() {
			return doError(quick.OutputOf(plan, nil))
		}
		if dry {
			return jsonResult(quick.OutputOf(plan, nil))
		}
		res, err := quick.Apply(ctx, quick.NewRepos(db), plan)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(quick.OutputOf(plan, res))
	}
}

// doError returns the plan as an error result, so the agent sees the problems and candidates.
func doError(o quick.Output) (*gomcp.CallToolResult, error) {
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return gomcp.NewToolResultError("internal error"), nil
	}
	return gomcp.NewToolResultError(string(b)), nil
}

