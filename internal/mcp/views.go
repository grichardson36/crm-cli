package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/jdanielnd/crm-cli/internal/validate"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const viewQueryHelp = "The filter is a list of conditions, all must hold. Each is written FIELD OPERATOR VALUE|VALUE (a | means any of), for example " +
	`"last_contacted before 90 days ago", "tag has client|lead", "value between 1000|5000", "company is_not Acme", "birthday in_next 30". ` +
	"Operators by field type: text is, is_not, contains, empty, not_empty. date before, after, within_last N, in_next N, empty, not_empty (a date is 2026-12-01, 90 days ago, in 30 days, today or next friday). " +
	"choice is, is_not, in. number =, <, >, between. relation linked_to, not_linked_to, has_any (a name, id or uuid). tag has, does_not_have. " +
	"Call crm_view_fields for the fields of a kind. No SQL. Names of fields and operators that do not exist fail with E_QUERY_* codes."

// queryFromArgs builds a query from the shared tool arguments. base is the query to change on an edit.
func queryFromArgs(req gomcp.CallToolRequest, base query.Query) (query.Query, bool, error) {
	args := req.GetArguments()
	q := base.Clone()
	changed := false
	if raw, ok := args["query"]; ok && raw != nil {
		switch v := raw.(type) {
		case string:
			parsed, err := query.Parse(v)
			if err != nil {
				return q, false, model.NewExitError(model.ErrValidation, "%v", err)
			}
			return parsed, true, nil
		default:
			b, _ := json.Marshal(v)
			parsed, err := query.Parse(string(b))
			if err != nil {
				return q, false, model.NewExitError(model.ErrValidation, "%v", err)
			}
			return parsed, true, nil
		}
	}
	if _, ok := args["where"]; ok {
		q.Filter = nil
		for _, w := range req.GetStringSlice("where", nil) {
			c, err := query.ParseWhere(w)
			if err != nil {
				return q, false, err
			}
			q.Filter = append(q.Filter, c)
		}
		changed = true
	}
	if _, ok := args["sort"]; ok {
		q.Sort = nil
		for _, s := range req.GetStringSlice("sort", nil) {
			parsed, err := query.ParseSort(s)
			if err != nil {
				return q, false, err
			}
			q.Sort = append(q.Sort, parsed)
		}
		changed = true
	}
	if _, ok := args["group_by"]; ok {
		q.GroupBy = strings.TrimSpace(req.GetString("group_by", ""))
		if q.GroupBy == "" {
			q.Bucket = ""
		}
		changed = true
	}
	if _, ok := args["bucket"]; ok {
		q.Bucket = req.GetString("bucket", "")
		changed = true
	}
	if _, ok := args["columns"]; ok {
		q.Columns = req.GetStringSlice("columns", nil)
		changed = true
	}
	if _, ok := args["search"]; ok {
		q.Search = strings.TrimSpace(req.GetString("search", ""))
		changed = true
	}
	return q, changed, nil
}

func queryOptions() []gomcp.ToolOption {
	return []gomcp.ToolOption{
		gomcp.WithArray("where", gomcp.Items(map[string]any{"type": "string"}), gomcp.Description("Conditions, one string each. "+viewQueryHelp)),
		gomcp.WithArray("sort", gomcp.Items(map[string]any{"type": "string"}), gomcp.Description("Up to two sort keys, FIELD or FIELD:desc")),
		gomcp.WithString("group_by", gomcp.Description("Field to group the rows by")),
		gomcp.WithString("bucket", gomcp.Description("Date group size: day, week or month")),
		gomcp.WithArray("columns", gomcp.Items(map[string]any{"type": "string"}), gomcp.Description("Extra columns to return when the view runs")),
		gomcp.WithString("search", gomcp.Description("Words every row must contain")),
		gomcp.WithString("query", gomcp.Description("The whole query as a JSON string, instead of the fields above")),
	}
}

// registerViewTools adds the saved view tools (patterns/saved-views): list, show, run, save, edit,
// rename, delete, restore, move, export, import and fields. They call the same repository as crm views.
func registerViewTools(s *server.MCPServer, db *sql.DB) {
	r := repo.NewSavedViewRepo(db)
	find := func(ctx context.Context, req gomcp.CallToolRequest) (*repo.SavedView, error) {
		ref := strings.TrimSpace(req.GetString("view", ""))
		if ref == "" {
			return nil, model.NewExitError(model.ErrValidation, "view is required: an id, uuid or name")
		}
		return r.Find(ctx, ref)
	}

	s.AddTool(
		gomcp.NewTool("crm_view_list",
			gomcp.WithDescription("List the saved views in tab order: id, name, kind, whether it is in the tabs, whether it is an example, the filter in words. Three example views ship off by default"),
			gomcp.WithBoolean("deleted", gomcp.Description("List the deleted views instead")),
			gomcp.WithString("kind", gomcp.Description("Only views of this kind: person, organization, deal, task, note, tag")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			list, err := r.List(ctx, req.GetBool("deleted", false))
			if err != nil {
				return mcpError(err)
			}
			if k := query.NormalizeKind(req.GetString("kind", "")); k != "" {
				var keep []repo.SavedView
				for _, v := range list {
					if v.Kind == k {
						keep = append(keep, v)
					}
				}
				list = keep
			}
			return jsonResult(r.Rows(ctx, list))
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_show",
			gomcp.WithDescription("One saved view with its neutral query (filter, sort, group_by, bucket, columns, search)"),
			gomcp.WithString("view", gomcp.Required(), gomcp.Description("Saved view id, uuid or name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := find(ctx, req)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(r.Row(ctx, v, true))
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_run",
			gomcp.WithDescription("Run a saved view and return the matching rows in its order: id, name, detail, meta, group (when grouped) and the view's extra columns. The kind of the view says which object the ids are"),
			gomcp.WithString("view", gomcp.Required(), gomcp.Description("Saved view id, uuid or name")),
			gomcp.WithNumber("limit", gomcp.Description("Most rows to return")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := find(ctx, req)
			if err != nil {
				return mcpError(err)
			}
			if v.Deleted {
				return mcpError(model.NewExitError(model.ErrConflict, "saved view %q is deleted, restore it first", v.Name))
			}
			rows, err := r.Run(ctx, v, time.Now())
			if err != nil {
				return mcpError(err)
			}
			if n := req.GetInt("limit", 0); n > 0 && len(rows) > n {
				rows = rows[:n]
			}
			return jsonResult(map[string]any{"view": v.Name, "kind": v.Kind, "count": len(rows), "rows": repo.RunRows(v.Query, rows)})
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_save",
			append([]gomcp.ToolOption{
				gomcp.WithDescription("Create a saved view: a name, an object kind and a neutral query. It shows up in the terminal UI as a view like any other. " + viewQueryHelp),
				gomcp.WithString("name", gomcp.Required(), gomcp.Description("Name, unique among live views, at most 80 characters")),
				gomcp.WithString("kind", gomcp.Required(), gomcp.Description("person, organization, deal, task, note or tag")),
				gomcp.WithBoolean("tabs", gomcp.Description("Show the view as a tab with a number key")),
			}, queryOptions()...)...,
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			q, _, err := queryFromArgs(req, query.Query{})
			if err != nil {
				return mcpError(err)
			}
			v, err := r.Create(ctx, repo.SavedViewInput{Name: req.GetString("name", ""), Kind: req.GetString("kind", ""), Query: q, ShowInTabs: req.GetBool("tabs", false)})
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(r.Row(ctx, v, true))
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_edit",
			append([]gomcp.ToolOption{
				gomcp.WithDescription("Change a saved view. A field you leave out is unchanged. where, sort, columns replace the old list. " + viewQueryHelp),
				gomcp.WithString("view", gomcp.Required(), gomcp.Description("Saved view id, uuid or name")),
				gomcp.WithString("name", gomcp.Description("New name")),
				gomcp.WithString("kind", gomcp.Description("New kind (the query is checked against it)")),
				gomcp.WithBoolean("tabs", gomcp.Description("Show or hide the view in the tabs")),
			}, queryOptions()...)...,
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := find(ctx, req)
			if err != nil {
				return mcpError(err)
			}
			q, changed, err := queryFromArgs(req, v.Query)
			if err != nil {
				return mcpError(err)
			}
			args := req.GetArguments()
			p := repo.SavedViewPatch{}
			if changed {
				p.Query = &q
			}
			if _, ok := args["name"]; ok {
				n := req.GetString("name", "")
				p.Name = &n
			}
			if _, ok := args["kind"]; ok {
				k := req.GetString("kind", "")
				p.Kind = &k
			}
			if _, ok := args["tabs"]; ok {
				b := req.GetBool("tabs", false)
				p.ShowInTabs = &b
			}
			if p.Query == nil && p.Name == nil && p.Kind == nil && p.ShowInTabs == nil {
				return mcpError(validate.Fail(validate.Issue{Code: validate.CodeQueryValue, Field: "edit", Expected: "at least one thing to change", Fix: "give name, kind, tabs, where, sort, group_by, columns or search"}))
			}
			nv, err := r.Update(ctx, v.ID, p)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(r.Row(ctx, nv, true))
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_rename",
			gomcp.WithDescription("Rename a saved view"),
			gomcp.WithString("view", gomcp.Required(), gomcp.Description("Saved view id, uuid or name")),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("New name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := find(ctx, req)
			if err != nil {
				return mcpError(err)
			}
			n := req.GetString("name", "")
			nv, err := r.Update(ctx, v.ID, repo.SavedViewPatch{Name: &n})
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(r.Row(ctx, nv, false))
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_delete",
			gomcp.WithDescription("Delete a saved view. It is a soft delete: crm_view_restore brings it back"),
			gomcp.WithString("view", gomcp.Required(), gomcp.Description("Saved view id, uuid or name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := find(ctx, req)
			if err != nil {
				return mcpError(err)
			}
			if err := r.Delete(ctx, v.ID); err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"id": v.ID, "name": v.Name, "deleted": true})
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_restore",
			gomcp.WithDescription("Bring a deleted saved view back. Refused when a live view has taken the name"),
			gomcp.WithString("view", gomcp.Required(), gomcp.Description("Saved view id, uuid or name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := find(ctx, req)
			if err != nil {
				return mcpError(err)
			}
			nv, err := r.Restore(ctx, v.ID)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(r.Row(ctx, nv, false))
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_move",
			gomcp.WithDescription("Move a saved view one place up or down in the order of the tabs"),
			gomcp.WithString("view", gomcp.Required(), gomcp.Description("Saved view id, uuid or name")),
			gomcp.WithString("direction", gomcp.Required(), gomcp.Description("up or down")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := find(ctx, req)
			if err != nil {
				return mcpError(err)
			}
			delta := 0
			switch strings.ToLower(req.GetString("direction", "")) {
			case "up":
				delta = -1
			case "down":
				delta = 1
			default:
				return mcpError(model.NewExitError(model.ErrValidation, "direction must be up or down"))
			}
			if _, err := r.Move(ctx, v.ID, delta); err != nil {
				return mcpError(err)
			}
			list, err := r.List(ctx, false)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(r.Rows(ctx, list))
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_export",
			gomcp.WithDescription("A saved view as the text of a small TOML file to share: kind, filter, sort, group, nothing personal (relation values are names). crm_view_import reads it"),
			gomcp.WithString("view", gomcp.Required(), gomcp.Description("Saved view id, uuid or name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := find(ctx, req)
			if err != nil {
				return mcpError(err)
			}
			text, err := r.ExportFile(ctx, v).Encode()
			if err != nil {
				return mcpError(err)
			}
			return gomcp.NewToolResultText(text), nil
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_import",
			gomcp.WithDescription("Create a saved view from the text of a view TOML file (see crm_view_export). Unknown keys, fields or operators are refused"),
			gomcp.WithString("toml", gomcp.Required(), gomcp.Description("The file contents")),
			gomcp.WithString("name", gomcp.Description("Save under this name instead of the name in the file")),
			gomcp.WithBoolean("tabs", gomcp.Description("Show the view as a tab")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			v, err := r.ImportFile(ctx, []byte(req.GetString("toml", "")), req.GetString("name", ""), req.GetBool("tabs", false))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(r.Row(ctx, v, true))
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_view_fields",
			gomcp.WithDescription("The fields a saved view of a kind can filter, sort and group by, with the operators of each and its options"),
			gomcp.WithString("kind", gomcp.Required(), gomcp.Description("person, organization, deal, task, note or tag")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			infos, ok := query.FieldInfos(req.GetString("kind", ""))
			if !ok {
				return mcpError(validate.Fail(validate.Issue{Code: validate.CodeViewKind, Field: "kind", Expected: "one of " + strings.Join(query.Kinds, ", "), Received: req.GetString("kind", ""), Fix: "pick a kind"}))
			}
			return jsonResult(infos)
		},
	)
}
