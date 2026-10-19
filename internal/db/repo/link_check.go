package repo

import (
	"context"
	"fmt"
)

// LinkProblems counts links that point at nothing. activity_target and merge_alias cannot carry
// foreign keys (their columns are polymorphic), so triggers prevent orphans and crm doctor checks.
type LinkProblems struct {
	// ActivityOrphans are activity_target rows whose task, interaction or target is gone.
	ActivityOrphans int `json:"activity_orphans"`
	// AliasOrphans are merge_alias rows whose survivor is gone.
	AliasOrphans int `json:"alias_orphans"`
	// ForeignKeys are rows foreign_key_check reports.
	ForeignKeys int `json:"foreign_key_violations"`
}

// Total is the number of problems.
func (p LinkProblems) Total() int { return p.ActivityOrphans + p.AliasOrphans + p.ForeignKeys }

// CheckLinks looks for orphan links. It only reads.
func CheckLinks(ctx context.Context, q DBTX) (LinkProblems, error) {
	var p LinkProblems
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM activity_target a WHERE
		NOT EXISTS (SELECT 1 FROM tasks WHERE a.source_kind = 'task' AND id = a.source_id)
		AND NOT EXISTS (SELECT 1 FROM interactions WHERE a.source_kind = 'interaction' AND id = a.source_id)
		OR NOT EXISTS (SELECT 1 FROM people WHERE a.target_kind = 'person' AND id = a.target_id)
		AND NOT EXISTS (SELECT 1 FROM organizations WHERE a.target_kind = 'organization' AND id = a.target_id)
		AND NOT EXISTS (SELECT 1 FROM deals WHERE a.target_kind = 'deal' AND id = a.target_id)`).Scan(&p.ActivityOrphans)
	if err != nil {
		return p, fmt.Errorf("check activity links: %w", err)
	}
	err = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM merge_alias m WHERE
		NOT EXISTS (SELECT 1 FROM people WHERE m.kind = 'person' AND id = m.survivor_id)
		AND NOT EXISTS (SELECT 1 FROM organizations WHERE m.kind = 'organization' AND id = m.survivor_id)`).Scan(&p.AliasOrphans)
	if err != nil {
		return p, fmt.Errorf("check merge aliases: %w", err)
	}
	rows, err := q.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return p, fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		p.ForeignKeys++
	}
	return p, rows.Err()
}
