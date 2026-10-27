package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/jdanielnd/crm-cli/internal/model"
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

// PurgeArchivedOptions selects what a bulk purge removes. Only people can be purged, the other
// archived kinds have no purge. Kind limits the selection ("" or "person"), IDs limits it to those
// archived people (empty means every archived person). Apply false only reports.
type PurgeArchivedOptions struct {
	Kind  string
	IDs   []int64
	Apply bool
}

// PurgeSkip is a requested id that was not purged and why.
type PurgeSkip struct {
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}

// PurgeArchivedResult is the plan (Apply false) or the outcome (Apply true) of a bulk purge.
type PurgeArchivedResult struct {
	Applied bool        `json:"applied"`
	Count   int         `json:"count"`
	People  []PurgeItem `json:"people"`
	Skipped []PurgeSkip `json:"skipped,omitempty"`
	Purged  int         `json:"purged"`
	Errors  []PurgeSkip `json:"errors,omitempty"`
}

// PurgeItem is one person in a bulk purge.
type PurgeItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PurgeArchived permanently deletes archived people, each one exactly as PersonRepo.Purge does
// (same dependents, same tombstones, one transaction per person). Without Apply nothing is written
// and the result lists who would go. A person that fails stays and is listed in Errors, the rest
// still purge.
func (r *PersonRepo) PurgeArchived(ctx context.Context, opts PurgeArchivedOptions) (PurgeArchivedResult, error) {
	res := PurgeArchivedResult{Applied: opts.Apply}
	if opts.Kind != "" && opts.Kind != "person" {
		return res, fmt.Errorf("only archived people can be purged, not %s: %w", opts.Kind, model.ErrValidation)
	}
	items, err := ListArchived(ctx, r.db, "person")
	if err != nil {
		return res, err
	}
	byID := map[int64]string{}
	var all []int64
	for _, it := range items {
		byID[it.ID] = it.Name
		all = append(all, it.ID)
	}
	want := all
	if len(opts.IDs) > 0 {
		want = nil
		seen := map[int64]bool{}
		for _, id := range opts.IDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			if _, ok := byID[id]; !ok {
				res.Skipped = append(res.Skipped, PurgeSkip{ID: id, Reason: "not an archived person"})
				continue
			}
			want = append(want, id)
		}
	}
	for _, id := range want {
		res.People = append(res.People, PurgeItem{ID: id, Name: byID[id]})
	}
	res.Count = len(res.People)
	if !opts.Apply {
		return res, nil
	}
	for _, p := range res.People {
		if err := r.Purge(ctx, p.ID); err != nil {
			res.Errors = append(res.Errors, PurgeSkip{ID: p.ID, Reason: err.Error()})
			continue
		}
		res.Purged++
	}
	return res, nil
}
