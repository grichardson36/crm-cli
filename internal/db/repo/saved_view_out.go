package repo

import (
	"context"

	"github.com/jdanielnd/crm-cli/internal/query"
)

// Row is the map of one saved view as the CLI prints it and MCP returns it: the same keys in both doors.
func (r *SavedViewRepo) Row(ctx context.Context, v *SavedView, withQuery bool) map[string]any {
	m := map[string]any{
		"id": v.ID, "uuid": v.UUID, "name": v.Name, "kind": v.Kind, "tabs": v.ShowInTabs, "example": v.Example,
		"position": v.Position, "filter": r.Describe(ctx, v), "order": query.DescribeOrder(v.Query), "deleted": v.Deleted,
		"created_at": v.CreatedAt, "updated_at": v.UpdatedAt,
	}
	if withQuery {
		m["query"] = v.Query
	}
	return m
}

// Rows is Row for a list.
func (r *SavedViewRepo) Rows(ctx context.Context, list []SavedView) []map[string]any {
	out := make([]map[string]any, len(list))
	for i := range list {
		out[i] = r.Row(ctx, &list[i], false)
	}
	return out
}

// RunRows turns the rows of a view run into maps: id, name, detail, meta and group (when the view is
// grouped), then one entry per optional column of the view.
func RunRows(q query.Query, rows []query.Row) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		m := map[string]any{"id": r.ID, "name": r.Label, "detail": r.Sub, "meta": r.Meta}
		if q.GroupBy != "" {
			m["group"] = r.GroupLabel
		}
		for j, c := range q.Columns {
			if j < len(r.Extra) {
				m[c] = r.Extra[j]
			}
		}
		out[i] = m
	}
	return out
}
