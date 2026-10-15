package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates an MCP server with all CRM tools registered.
func NewServer(db *sql.DB, version string, opts ...Option) *server.MCPServer {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	s := server.NewMCPServer(
		"crm",
		version,
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(false, false),
	)

	pr := repo.NewPersonRepo(db)
	or := repo.NewOrgRepo(db)
	ir := repo.NewInteractionRepo(db)
	dr := repo.NewDealRepo(db)
	tr := repo.NewTaskRepo(db)
	tagr := repo.NewTagRepo(db)

	// Person tools
	s.AddTool(
		gomcp.NewTool("crm_person_search",
			gomcp.WithDescription("Search people by name, email, tag, or org"),
			gomcp.WithString("query", gomcp.Required(), gomcp.Description("Search query")),
			gomcp.WithNumber("limit", gomcp.Description("Max results (default 20)")),
		),
		personSearchHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_get",
			gomcp.WithDescription("Get full details for a person by ID"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
		),
		personGetHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_create",
			gomcp.WithDescription("Create a new person"),
			gomcp.WithString("first_name", gomcp.Required(), gomcp.Description("First name")),
			gomcp.WithString("last_name", gomcp.Description("Last name")),
			gomcp.WithString("email", gomcp.Description("Email address")),
			gomcp.WithString("phone", gomcp.Description("Phone number")),
			gomcp.WithString("title", gomcp.Description("Job title")),
			gomcp.WithString("company", gomcp.Description("Company name")),
			gomcp.WithString("location", gomcp.Description("Location")),
			gomcp.WithString("notes", gomcp.Description("Notes")),
		),
		personCreateHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_update",
			gomcp.WithDescription("Update person fields"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
			gomcp.WithString("first_name", gomcp.Description("First name")),
			gomcp.WithString("last_name", gomcp.Description("Last name")),
			gomcp.WithString("email", gomcp.Description("Email address")),
			gomcp.WithString("phone", gomcp.Description("Phone number")),
			gomcp.WithString("title", gomcp.Description("Job title")),
			gomcp.WithString("company", gomcp.Description("Company name")),
			gomcp.WithString("location", gomcp.Description("Location")),
			gomcp.WithString("notes", gomcp.Description("Notes")),
			gomcp.WithString("summary", gomcp.Description("AI-maintained summary/dossier")),
			gomcp.WithString("birthday", gomcp.Description("YYYY-MM-DD, or --MM-DD when the year is unknown. Empty string clears it")),
			gomcp.WithString("keep_in_touch", gomcp.Description("Keep-in-touch cadence: "+strings.Join(model.KeepInTouchCadences, ", ")+". Empty string clears it")),
			gomcp.WithString("snoozed_until", gomcp.Description("Hide from the due list until this date (YYYY-MM-DD). Empty string clears it")),
			gomcp.WithString("last_contacted_at", gomcp.Description("When you last reached out (YYYY-MM-DD or YYYY-MM-DD HH:MM:SS, UTC)")),
			gomcp.WithBoolean("is_favorite", gomcp.Description("Mark as favorite")),
			gomcp.WithNumber("priority_tier", gomcp.Description("Priority tier number. 0 clears it")),
		),
		personUpdateHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_add_contact_method",
			gomcp.WithDescription("Add an email, phone number, or social handle to a person. A person can have several emails and phones. The first one becomes primary"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
			gomcp.WithString("type", gomcp.Required(), gomcp.Enum("email", "phone", "handle"), gomcp.Description("What to add")),
			gomcp.WithString("value", gomcp.Required(), gomcp.Description("The email address, phone number, or handle URL/username")),
			gomcp.WithString("kind", gomcp.Description("For type=handle: "+strings.Join(model.HandleKinds, ", "))),
			gomcp.WithString("label", gomcp.Description("Optional label for an email or phone, e.g. Work")),
			gomcp.WithBoolean("primary", gomcp.Description("Make this the primary email or phone")),
		),
		contactMethodAddHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_remove_contact_method",
			gomcp.WithDescription("Remove an email, phone number, or social handle from a person"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
			gomcp.WithString("type", gomcp.Required(), gomcp.Enum("email", "phone", "handle"), gomcp.Description("What to remove")),
			gomcp.WithString("value", gomcp.Required(), gomcp.Description("The email or phone to remove, or for type=handle the handle kind")),
		),
		contactMethodRemoveHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_due",
			gomcp.WithDescription("List people you are due to reach out to, based on each person's keep-in-touch cadence. Most overdue first"),
			gomcp.WithNumber("within_days", gomcp.Description("Also include people due in the next N days (default 0)")),
			gomcp.WithNumber("limit", gomcp.Description("Max results (default 50)")),
		),
		dueHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_birthdays",
			gomcp.WithDescription("List upcoming birthdays, soonest first"),
			gomcp.WithNumber("days", gomcp.Description("Look ahead this many days (default 30)")),
		),
		birthdaysHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_mark_contacted",
			gomcp.WithDescription("Record that you just reached out to a person. Resets their keep-in-touch clock and clears any snooze"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
			gomcp.WithString("at", gomcp.Description("When (YYYY-MM-DD or YYYY-MM-DD HH:MM:SS, UTC). Default now")),
		),
		markContactedHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_delete",
			gomcp.WithDescription("Deprecated alias of crm_person_archive"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
		),
		personDeleteHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_archive",
			gomcp.WithDescription("Archive a person (reversible). Archived people are hidden from search, due, birthdays and dupes"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
		),
		personDeleteHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_restore",
			gomcp.WithDescription("Restore an archived person"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
		),
		personRestoreHandler(pr),
	)

	s.AddTool(
		gomcp.NewTool("crm_person_purge",
			gomcp.WithDescription("Permanently delete an already archived person and their dependent data. Cannot be undone"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Person ID")),
			gomcp.WithBoolean("confirm", gomcp.Required(), gomcp.Description("Must be true")),
		),
		personPurgeHandler(pr),
	)

	// Organization tools
	s.AddTool(
		gomcp.NewTool("crm_org_search",
			gomcp.WithDescription("Search organizations"),
			gomcp.WithString("query", gomcp.Required(), gomcp.Description("Search query")),
			gomcp.WithNumber("limit", gomcp.Description("Max results (default 20)")),
		),
		orgSearchHandler(or),
	)

	s.AddTool(
		gomcp.NewTool("crm_org_get",
			gomcp.WithDescription("Get organization details with people"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Organization ID")),
		),
		orgGetHandler(or),
	)

	// Interaction tools
	s.AddTool(
		gomcp.NewTool("crm_interaction_log",
			gomcp.WithDescription("Log an interaction of any defined type (see crm_note_types)"),
			gomcp.WithString("type", gomcp.Required(), gomcp.Description("Interaction type, case-insensitive. Defaults: call, email, meeting, note, message. Call crm_note_types for the full list")),
			gomcp.WithString("subject", gomcp.Description("Subject/title")),
			gomcp.WithString("content", gomcp.Description("Content/body")),
			gomcp.WithString("direction", gomcp.Description("Direction: inbound or outbound")),
			gomcp.WithString("occurred_at", gomcp.Description("When it occurred (ISO 8601)")),
			gomcp.WithArray("person_ids", gomcp.Description("Array of person IDs to link this interaction to")),
		),
		interactionLogHandler(ir),
	)

	s.AddTool(
		gomcp.NewTool("crm_note_types",
			gomcp.WithDescription("List interaction types usable with crm_interaction_log, and whether each counts as contact for keep-in-touch"),
			gomcp.WithBoolean("include_archived", gomcp.Description("Also list archived types")),
		),
		noteTypesHandler(repo.NewNoteTypeRepo(db)),
	)

	s.AddTool(
		gomcp.NewTool("crm_interaction_list",
			gomcp.WithDescription("List interactions, optionally filtered by person"),
			gomcp.WithNumber("person_id", gomcp.Description("Filter by person ID")),
			gomcp.WithString("type", gomcp.Description("Filter by type")),
			gomcp.WithNumber("limit", gomcp.Description("Max results (default 20)")),
		),
		interactionListHandler(ir),
	)

	// Search
	s.AddTool(
		gomcp.NewTool("crm_search",
			gomcp.WithDescription("Cross-entity full-text search across people, organizations, interactions, and deals"),
			gomcp.WithString("query", gomcp.Required(), gomcp.Description("Search query")),
			gomcp.WithString("type", gomcp.Description("Filter by entity type: person, organization, interaction, deal")),
			gomcp.WithNumber("limit", gomcp.Description("Max results per type (default 20)")),
		),
		searchHandler(pr, or, ir, dr),
	)

	// Context
	s.AddTool(
		gomcp.NewTool("crm_context",
			gomcp.WithDescription("Full context briefing for a person: profile, org, interactions, deals, tasks, relationships, tags"),
			gomcp.WithNumber("person_id", gomcp.Required(), gomcp.Description("Person ID")),
		),
		contextHandler(pr, or, ir, dr, tr, repo.NewRelationshipRepo(db), tagr),
	)

	// Deal tools
	s.AddTool(
		gomcp.NewTool("crm_deal_create",
			gomcp.WithDescription("Create a new deal"),
			gomcp.WithString("title", gomcp.Required(), gomcp.Description("Deal title")),
			gomcp.WithNumber("value", gomcp.Description("Deal value")),
			gomcp.WithString("stage", gomcp.Description("Stage: lead, prospect, proposal, negotiation, won, lost (default: lead)")),
			gomcp.WithNumber("person_id", gomcp.Description("Associated person ID")),
			gomcp.WithNumber("org_id", gomcp.Description("Associated organization ID")),
			gomcp.WithString("notes", gomcp.Description("Notes")),
		),
		dealCreateHandler(dr),
	)

	s.AddTool(
		gomcp.NewTool("crm_deal_update",
			gomcp.WithDescription("Update a deal (stage, value, etc.)"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Deal ID")),
			gomcp.WithString("title", gomcp.Description("Deal title")),
			gomcp.WithNumber("value", gomcp.Description("Deal value")),
			gomcp.WithString("stage", gomcp.Description("Stage: lead, prospect, proposal, negotiation, won, lost")),
			gomcp.WithString("notes", gomcp.Description("Notes")),
			gomcp.WithString("closed_at", gomcp.Description("Date the deal was closed")),
		),
		dealUpdateHandler(dr),
	)

	// Task tools
	s.AddTool(
		gomcp.NewTool("crm_task_create",
			gomcp.WithDescription("Create a follow-up task"),
			gomcp.WithString("title", gomcp.Required(), gomcp.Description("Task title")),
			gomcp.WithString("description", gomcp.Description("Description")),
			gomcp.WithNumber("person_id", gomcp.Description("Associated person ID")),
			gomcp.WithNumber("deal_id", gomcp.Description("Associated deal ID")),
			gomcp.WithString("due", gomcp.Description("Due date (ISO 8601)")),
			gomcp.WithString("priority", gomcp.Description("Priority: low, medium, high (default: medium)")),
			gomcp.WithArray("person_ids", gomcp.Description("Link the task to several people"), gomcp.WithNumberItems()),
			gomcp.WithString("recurrence", gomcp.Description("Repeat when completed, e.g. \"7 days\", \"1 mon\", weekly. Needs a due date")),
			gomcp.WithString("duration", gomcp.Description("How long it takes: 30m, 2h")),
			gomcp.WithString("earliest_start", gomcp.Description("Do not schedule before this date or time")),
			gomcp.WithString("deadline", gomcp.Description("The scheduler plans against this date or time")),
			gomcp.WithBoolean("hard_deadline", gomcp.Description("The deadline may use hours outside working hours")),
			gomcp.WithString("min_chunk", gomcp.Description("Smallest block the task may be split into")),
			gomcp.WithBoolean("auto_schedule", gomcp.Description("Let the planner place it (needs a duration)")),
			gomcp.WithString("schedule_name", gomcp.Description("Working hours rule, default work")),
			gomcp.WithString("scheduled_start", gomcp.Description("Place it at this date and time by hand; this locks it")),
			gomcp.WithBoolean("locked", gomcp.Description("Keep the planner away from the placed time")),
		),
		taskCreateHandler(tr),
	)

	s.AddTool(
		gomcp.NewTool("crm_task_complete",
			gomcp.WithDescription("Mark a task done. A recurring task creates its next occurrence and returns next_task_id"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Task ID")),
		),
		taskCompleteHandler(tr),
	)

	s.AddTool(
		gomcp.NewTool("crm_task_list",
			gomcp.WithDescription("List open tasks"),
			gomcp.WithNumber("person_id", gomcp.Description("Filter by person ID")),
			gomcp.WithBoolean("overdue", gomcp.Description("Show only overdue tasks")),
			gomcp.WithBoolean("include_completed", gomcp.Description("Include completed tasks")),
			gomcp.WithNumber("limit", gomcp.Description("Max results")),
		),
		taskListHandler(tr),
	)

	// Tag tools
	s.AddTool(
		gomcp.NewTool("crm_tag_apply",
			gomcp.WithDescription("Apply a tag to an entity"),
			gomcp.WithString("entity_type", gomcp.Required(), gomcp.Description("Entity type: person, organization, deal, interaction")),
			gomcp.WithNumber("entity_id", gomcp.Required(), gomcp.Description("Entity ID")),
			gomcp.WithString("tag", gomcp.Required(), gomcp.Description("Tag name")),
		),
		tagApplyHandler(tagr),
	)

	// Relationships
	rr := repo.NewRelationshipRepo(db)
	s.AddTool(
		gomcp.NewTool("crm_person_relate",
			gomcp.WithDescription("Create a relationship between two people"),
			gomcp.WithNumber("person_id", gomcp.Required(), gomcp.Description("Person ID")),
			gomcp.WithNumber("related_person_id", gomcp.Required(), gomcp.Description("Related person ID")),
			gomcp.WithString("type", gomcp.Required(), gomcp.Description("Relationship type: colleague, friend, manager, mentor, referred-by")),
			gomcp.WithString("notes", gomcp.Description("Optional notes about the relationship")),
		),
		relateHandler(rr),
	)

	// Stats
	s.AddTool(
		gomcp.NewTool("crm_stats",
			gomcp.WithDescription("CRM summary statistics"),
		),
		statsHandler(db),
	)

	registerDedupeTools(s, db)
	registerLogEditTools(s, ir)
	registerSyncTools(s, db)
	registerDavTools(s, db)
	registerEventTools(s, db)
	registerScheduleTools(s, db, tr)
	registerHelpTools(s, o)

	return s
}

// Helper to convert any value to JSON text result
func jsonResult(v any) (*gomcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return gomcp.NewToolResultError(fmt.Sprintf("marshal error: %s", err)), nil
	}
	return gomcp.NewToolResultText(string(b)), nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Safe type assertion helpers for MCP arguments.

func argString(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func argFloat(args map[string]any, key string) (float64, bool) {
	v, ok := args[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func argInt64(args map[string]any, key string) (int64, bool) {
	f, ok := argFloat(args, key)
	if !ok {
		return 0, false
	}
	if f != math.Trunc(f) || f > math.MaxInt64 || f < math.MinInt64 {
		return 0, false
	}
	return int64(f), true
}

func requireID(req gomcp.CallToolRequest, key string) (int64, *gomcp.CallToolResult) {
	id := int64(req.GetInt(key, 0))
	if id <= 0 {
		return 0, gomcp.NewToolResultError(fmt.Sprintf("%s must be a positive integer", key))
	}
	return id, nil
}

func mcpError(err error) (*gomcp.CallToolResult, error) {
	switch {
	case errors.Is(err, model.ErrNotFound):
		return gomcp.NewToolResultError("not found"), nil
	case errors.Is(err, model.ErrConflict):
		return gomcp.NewToolResultError("conflict: " + err.Error()), nil
	case errors.Is(err, model.ErrValidation):
		return gomcp.NewToolResultError("validation error: " + err.Error()), nil
	default:
		return gomcp.NewToolResultError("internal error"), nil
	}
}

// --- Person handlers ---

func personSearchHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		query := req.GetString("query", "")
		limit := req.GetInt("limit", 20)
		people, err := pr.Search(ctx, query, limit)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(people)
	}
}

func personGetHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		person, err := pr.FindByID(ctx, id)
		if err != nil {
			return mcpError(err)
		}
		if err := pr.LoadDetails(ctx, person); err != nil {
			return mcpError(err)
		}
		return jsonResult(person)
	}
}

func personCreateHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		input := model.CreatePersonInput{
			FirstName: req.GetString("first_name", ""),
			LastName:  strPtr(req.GetString("last_name", "")),
			Email:     strPtr(req.GetString("email", "")),
			Phone:     strPtr(req.GetString("phone", "")),
			Title:     strPtr(req.GetString("title", "")),
			Company:   strPtr(req.GetString("company", "")),
			Location:  strPtr(req.GetString("location", "")),
			Notes:     strPtr(req.GetString("notes", "")),
		}
		person, err := pr.Create(ctx, input)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(person)
	}
}

func personUpdateHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		args := req.GetArguments()
		input := model.UpdatePersonInput{}

		if s, ok := argString(args, "first_name"); ok {
			input.FirstName = &s
		}
		if s, ok := argString(args, "last_name"); ok {
			input.LastName = &s
		}
		if s, ok := argString(args, "email"); ok {
			input.Email = &s
		}
		if s, ok := argString(args, "phone"); ok {
			input.Phone = &s
		}
		if s, ok := argString(args, "title"); ok {
			input.Title = &s
		}
		if s, ok := argString(args, "company"); ok {
			input.Company = &s
		}
		if s, ok := argString(args, "location"); ok {
			input.Location = &s
		}
		if s, ok := argString(args, "notes"); ok {
			input.Notes = &s
		}
		if s, ok := argString(args, "summary"); ok {
			input.Summary = &s
		}
		if s, ok := argString(args, "birthday"); ok {
			input.Birthday = &s
		}
		if s, ok := argString(args, "keep_in_touch"); ok {
			input.KeepInTouch = &s
		}
		if s, ok := argString(args, "snoozed_until"); ok {
			input.SnoozedUntil = &s
		}
		if s, ok := argString(args, "last_contacted_at"); ok {
			input.LastContactedAt = &s
		}
		if b, ok := args["is_favorite"].(bool); ok {
			input.IsFavorite = &b
		}
		if f, ok := argFloat(args, "priority_tier"); ok {
			tier := int(f)
			input.PriorityTier = &tier
		}

		person, err := pr.Update(ctx, id, input)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(person)
	}
}

func personDeleteHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		err := pr.Archive(ctx, id)
		if err != nil {
			return mcpError(err)
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Person #%d archived", id)), nil
	}
}

func personRestoreHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		if err := pr.Restore(ctx, id); err != nil {
			return mcpError(err)
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Person #%d restored", id)), nil
	}
}

func personPurgeHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		if !req.GetBool("confirm", false) {
			return gomcp.NewToolResultError("validation error: purge is permanent, pass confirm: true"), nil
		}
		if err := pr.Purge(ctx, id); err != nil {
			return mcpError(err)
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Person #%d permanently deleted", id)), nil
	}
}

func noteTypesHandler(nr *repo.NoteTypeRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		types, err := nr.List(ctx, req.GetBool("include_archived", false))
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(types)
	}
}

// --- Organization handlers ---

func orgSearchHandler(or *repo.OrgRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		query := req.GetString("query", "")
		limit := req.GetInt("limit", 20)
		orgs, err := or.Search(ctx, query, limit)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(orgs)
	}
}

func orgGetHandler(or *repo.OrgRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		org, err := or.FindByID(ctx, id)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(org)
	}
}

// --- Interaction handlers ---

func interactionLogHandler(ir *repo.InteractionRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		args := req.GetArguments()
		input := model.CreateInteractionInput{
			Type:    req.GetString("type", "note"),
			Subject: strPtr(req.GetString("subject", "")),
			Content: strPtr(req.GetString("content", "")),
		}

		if dir := req.GetString("direction", ""); dir != "" {
			input.Direction = &dir
		}
		if at := req.GetString("occurred_at", ""); at != "" {
			input.OccurredAt = &at
		}

		// Handle person_ids as an array of numbers
		if ids, ok := args["person_ids"]; ok {
			if v, ok := ids.([]any); ok {
				for _, item := range v {
					switch n := item.(type) {
					case float64:
						input.PersonIDs = append(input.PersonIDs, int64(n))
					case int:
						input.PersonIDs = append(input.PersonIDs, int64(n))
					case int64:
						input.PersonIDs = append(input.PersonIDs, n)
					}
				}
			}
		}

		interaction, err := ir.Create(ctx, input)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(interaction)
	}
}

func interactionListHandler(ir *repo.InteractionRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		filters := model.InteractionFilters{
			Limit: req.GetInt("limit", 20),
		}
		if pid := int64(req.GetInt("person_id", 0)); pid > 0 {
			filters.PersonID = &pid
		}
		if t := req.GetString("type", ""); t != "" {
			filters.Type = &t
		}

		interactions, err := ir.FindAll(ctx, filters)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(interactions)
	}
}

// --- Search handler ---

func searchHandler(pr *repo.PersonRepo, or *repo.OrgRepo, ir *repo.InteractionRepo, dr *repo.DealRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		query := req.GetString("query", "")
		entityType := req.GetString("type", "")
		limit := req.GetInt("limit", 20)

		if entityType != "" && !model.ValidEntityType(entityType) {
			return gomcp.NewToolResultError(fmt.Sprintf("invalid entity type: %s", entityType)), nil
		}

		results := map[string]any{}

		if entityType == "" || entityType == "person" {
			people, err := pr.Search(ctx, query, limit)
			if err != nil {
				return mcpError(err)
			}
			results["people"] = people
		}
		if entityType == "" || entityType == "organization" {
			orgs, err := or.Search(ctx, query, limit)
			if err != nil {
				return mcpError(err)
			}
			results["organizations"] = orgs
		}
		if entityType == "" || entityType == "interaction" {
			interactions, err := ir.Search(ctx, query, limit)
			if err != nil {
				return mcpError(err)
			}
			results["interactions"] = interactions
		}
		if entityType == "" || entityType == "deal" {
			deals, err := dr.Search(ctx, query, limit)
			if err != nil {
				return mcpError(err)
			}
			results["deals"] = deals
		}

		return jsonResult(results)
	}
}

// --- Context handler ---

func contextHandler(pr *repo.PersonRepo, or *repo.OrgRepo, ir *repo.InteractionRepo, dr *repo.DealRepo, tr *repo.TaskRepo, rr *repo.RelationshipRepo, tagr *repo.TagRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "person_id")
		if errResult != nil {
			return errResult, nil
		}
		person, err := pr.FindByID(ctx, id)
		if err != nil {
			return mcpError(err)
		}
		if err := pr.LoadDetails(ctx, person); err != nil {
			return mcpError(err)
		}

		result := map[string]any{"person": person}

		if person.OrgID != nil {
			org, err := or.FindByID(ctx, *person.OrgID)
			if err != nil {
				return mcpError(err)
			}
			result["organization"] = org
		}

		interactions, err := ir.FindAll(ctx, model.InteractionFilters{PersonID: &person.ID, Limit: 10})
		if err != nil {
			return mcpError(err)
		}
		if len(interactions) > 0 {
			result["recent_interactions"] = interactions
		}

		deals, err := dr.FindAll(ctx, model.DealFilters{PersonID: &person.ID, Limit: 50})
		if err != nil {
			return mcpError(err)
		}
		if len(deals) > 0 {
			result["deals"] = deals
		}

		tasks, err := tr.FindAll(ctx, model.TaskFilters{PersonID: &person.ID, Limit: 50})
		if err != nil {
			return mcpError(err)
		}
		if len(tasks) > 0 {
			result["tasks"] = tasks
		}

		rels, err := rr.FindForPerson(ctx, person.ID)
		if err != nil {
			return mcpError(err)
		}
		if len(rels) > 0 {
			result["relationships"] = rels
		}

		tags, err := tagr.GetForEntity(ctx, "person", person.ID)
		if err != nil {
			return mcpError(err)
		}
		if len(tags) > 0 {
			result["tags"] = tags
		}

		return jsonResult(result)
	}
}

// --- Deal handlers ---

func dealCreateHandler(dr *repo.DealRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		args := req.GetArguments()
		input := model.CreateDealInput{
			Title: req.GetString("title", ""),
			Stage: req.GetString("stage", "lead"),
			Notes: strPtr(req.GetString("notes", "")),
		}

		if f, ok := argFloat(args, "value"); ok {
			input.Value = &f
		}
		if pid, ok := argInt64(args, "person_id"); ok {
			input.PersonID = &pid
		}
		if oid, ok := argInt64(args, "org_id"); ok {
			input.OrgID = &oid
		}

		deal, err := dr.Create(ctx, input)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(deal)
	}
}

func dealUpdateHandler(dr *repo.DealRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		args := req.GetArguments()
		input := model.UpdateDealInput{}

		if s, ok := argString(args, "title"); ok {
			input.Title = &s
		}
		if f, ok := argFloat(args, "value"); ok {
			input.Value = &f
		}
		if s, ok := argString(args, "stage"); ok {
			input.Stage = &s
		}
		if s, ok := argString(args, "notes"); ok {
			input.Notes = &s
		}
		if s, ok := argString(args, "closed_at"); ok {
			input.ClosedAt = &s
		}

		deal, err := dr.Update(ctx, id, input)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(deal)
	}
}

// --- Task handlers ---

func taskCreateHandler(tr *repo.TaskRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		args := req.GetArguments()
		input := model.CreateTaskInput{
			Title:       req.GetString("title", ""),
			Description: strPtr(req.GetString("description", "")),
			Priority:    req.GetString("priority", "medium"),
			DueAt:       strPtr(req.GetString("due", "")),
		}

		if pid, ok := argInt64(args, "person_id"); ok {
			input.PersonID = &pid
		}
		if did, ok := argInt64(args, "deal_id"); ok {
			input.DealID = &did
		}
		if raw, ok := args["person_ids"].([]any); ok {
			for _, v := range raw {
				if f, ok := v.(float64); ok && f == math.Trunc(f) && f > 0 {
					input.PersonIDs = append(input.PersonIDs, int64(f))
				}
			}
		}
		if rec, ok := argString(args, "recurrence"); ok && rec != "" {
			input.Recurrence = &rec
		}

		sched, err := scheduleFromArgs(args)
		if err != nil {
			return validationError(err)
		}
		input.Schedule = sched
		task, err := tr.Create(ctx, input)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(task)
	}
}

func taskListHandler(tr *repo.TaskRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		filters := model.TaskFilters{
			Limit:            req.GetInt("limit", 50),
			Overdue:          req.GetBool("overdue", false),
			IncludeCompleted: req.GetBool("include_completed", false),
		}

		if pid := int64(req.GetInt("person_id", 0)); pid > 0 {
			filters.PersonID = &pid
		}

		tasks, err := tr.FindAll(ctx, filters)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(tasks)
	}
}

func taskCompleteHandler(tr *repo.TaskRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		task, err := tr.Complete(ctx, id)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(task)
	}
}

// --- Contact method, reminder, and birthday handlers ---

func contactMethodAddHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		value := req.GetString("value", "")
		label := strPtr(req.GetString("label", ""))
		primary := req.GetBool("primary", false)

		var err error
		switch kind := req.GetString("type", ""); kind {
		case "email":
			err = pr.AddEmail(ctx, id, value, label, primary)
		case "phone":
			err = pr.AddPhone(ctx, id, value, label, primary)
		case "handle":
			err = pr.SetHandle(ctx, id, req.GetString("kind", ""), value)
		default:
			return gomcp.NewToolResultError("type must be email, phone, or handle"), nil
		}
		if err != nil {
			return mcpError(err)
		}
		return personWithDetails(ctx, pr, id)
	}
}

func contactMethodRemoveHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		value := req.GetString("value", "")

		var err error
		switch kind := req.GetString("type", ""); kind {
		case "email":
			err = pr.RemoveEmail(ctx, id, value)
		case "phone":
			err = pr.RemovePhone(ctx, id, value)
		case "handle":
			err = pr.RemoveHandle(ctx, id, value)
		default:
			return gomcp.NewToolResultError("type must be email, phone, or handle"), nil
		}
		if err != nil {
			return mcpError(err)
		}
		return personWithDetails(ctx, pr, id)
	}
}

func personWithDetails(ctx context.Context, pr *repo.PersonRepo, id int64) (*gomcp.CallToolResult, error) {
	person, err := pr.FindByID(ctx, id)
	if err != nil {
		return mcpError(err)
	}
	if err := pr.LoadDetails(ctx, person); err != nil {
		return mcpError(err)
	}
	return jsonResult(person)
}

func dueHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		due, err := pr.Due(ctx, req.GetInt("within_days", 0), req.GetInt("limit", 50))
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(due)
	}
}

func birthdaysHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		list, err := pr.Birthdays(ctx, req.GetInt("days", 30), time.Now())
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(list)
	}
}

func markContactedHandler(pr *repo.PersonRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		id, errResult := requireID(req, "id")
		if errResult != nil {
			return errResult, nil
		}
		when := req.GetString("at", "")
		if when == "" {
			when = time.Now().UTC().Format("2006-01-02 15:04:05")
		}
		empty := ""
		if _, err := pr.Update(ctx, id, model.UpdatePersonInput{LastContactedAt: &when, SnoozedUntil: &empty}); err != nil {
			return mcpError(err)
		}
		return personWithDetails(ctx, pr, id)
	}
}

// --- Tag handler ---

func tagApplyHandler(tagr *repo.TagRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		entityType := req.GetString("entity_type", "")
		entityID, errResult := requireID(req, "entity_id")
		if errResult != nil {
			return errResult, nil
		}
		tagName := req.GetString("tag", "")

		err := tagr.Apply(ctx, entityType, entityID, tagName)
		if err != nil {
			return mcpError(err)
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Tagged %s #%d with %q", entityType, entityID, tagName)), nil
	}
}

// --- Relationship handler ---

func relateHandler(rr *repo.RelationshipRepo) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		personID, errResult := requireID(req, "person_id")
		if errResult != nil {
			return errResult, nil
		}
		relatedID, errResult := requireID(req, "related_person_id")
		if errResult != nil {
			return errResult, nil
		}
		relType := req.GetString("type", "")
		notes := req.GetString("notes", "")

		var notesPtr *string
		if notes != "" {
			notesPtr = &notes
		}

		rel, err := rr.Create(ctx, personID, relatedID, relType, notesPtr)
		if err != nil {
			return mcpError(err)
		}
		return jsonResult(rel)
	}
}

// --- Stats handler ---

func statsHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		stats := map[string]any{}

		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM people WHERE archived = 0").Scan(&n); err != nil {
			return mcpError(fmt.Errorf("count people: %w", err))
		}
		stats["contacts"] = n

		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM organizations WHERE archived = 0").Scan(&n); err != nil {
			return mcpError(fmt.Errorf("count organizations: %w", err))
		}
		stats["organizations"] = n

		var dealCount int
		var dealValue float64
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*), COALESCE(SUM(value), 0) FROM deals WHERE archived = 0 AND stage NOT IN ('won', 'lost')").
			Scan(&dealCount, &dealValue); err != nil {
			return mcpError(fmt.Errorf("count deals: %w", err))
		}
		stats["open_deals"] = dealCount
		stats["deal_value"] = dealValue

		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM tasks WHERE archived = 0 AND completed = 0").Scan(&n); err != nil {
			return mcpError(fmt.Errorf("count open tasks: %w", err))
		}
		stats["open_tasks"] = n

		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM tasks WHERE archived = 0 AND completed = 0 AND due_at IS NOT NULL AND due_at < datetime('now')").Scan(&n); err != nil {
			return mcpError(fmt.Errorf("count overdue tasks: %w", err))
		}
		stats["overdue_tasks"] = n

		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM interactions WHERE archived = 0 AND occurred_at >= datetime('now', '-7 days')").Scan(&n); err != nil {
			return mcpError(fmt.Errorf("count interactions: %w", err))
		}
		stats["interactions_7days"] = n

		return jsonResult(stats)
	}
}
