package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// ArchivedItem is one soft-deleted object of any kind, for the Archived view.
type ArchivedItem struct {
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Detail    string `json:"detail,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// ListArchived lists the soft-deleted people, organizations, deals and tasks, newest first. A kind
// ("person", "organization", "deal", "task") limits the list, "" lists them all.
func ListArchived(ctx context.Context, db *sql.DB, kind string) ([]ArchivedItem, error) {
	parts := []struct{ kind, sql string }{
		{"person", `SELECT 'person', id, trim(first_name || ' ' || COALESCE(last_name, '')), COALESCE(NULLIF(company, ''), title, ''), updated_at FROM people WHERE archived = 1`},
		{"organization", `SELECT 'organization', id, name, COALESCE(industry, ''), updated_at FROM organizations WHERE archived = 1`},
		{"deal", `SELECT 'deal', id, title, stage, updated_at FROM deals WHERE archived = 1`},
		{"task", `SELECT 'task', id, title, COALESCE(due_at, ''), updated_at FROM tasks WHERE archived = 1`},
	}
	var out []ArchivedItem
	for _, p := range parts {
		if kind != "" && kind != p.kind {
			continue
		}
		rows, err := db.QueryContext(ctx, p.sql+" ORDER BY updated_at DESC, id DESC")
		if err != nil {
			return nil, fmt.Errorf("list archived %ss: %w", p.kind, err)
		}
		for rows.Next() {
			var it ArchivedItem
			var detail sql.NullString
			if err := rows.Scan(&it.Kind, &it.ID, &it.Name, &detail, &it.UpdatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			it.Detail = detail.String
			out = append(out, it)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	// Newest first across kinds.
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}
