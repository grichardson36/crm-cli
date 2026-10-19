package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Tasks and interactions point at people, organizations and deals through activity_target. These
// helpers are shared by TaskRepo and InteractionRepo.

// Targets are the live linked objects of an activity, by target kind.
type Targets struct {
	People, Orgs, Deals []int64
}

// addTargets links an activity to objects of one kind. Ones already linked are skipped.
func addTargets(ctx context.Context, q DBTX, sourceKind string, sourceID int64, targetKind string, ids []int64) error {
	for _, id := range ids {
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM activity_target WHERE source_kind = ? AND source_id = ?
			AND target_kind = ? AND target_id = ? AND deleted_at IS NULL`, sourceKind, sourceID, targetKind, id).Scan(&n); err != nil {
			return fmt.Errorf("find %s link: %w", targetKind, err)
		}
		if n > 0 {
			continue
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES (?, ?, ?, ?, ?)`,
			uuid.New().String(), sourceKind, sourceID, targetKind, id); err != nil {
			return fmt.Errorf("link %s %d to %s %d: %w", sourceKind, sourceID, targetKind, id, err)
		}
	}
	return nil
}

var targetTable = map[string]string{"person": "people", "organization": "organizations", "deal": "deals"}

// loadTargets reads the live targets of the given activities, hiding archived targets.
func loadTargets(ctx context.Context, q DBTX, sourceKind string, sourceIDs []int64) (map[int64]*Targets, error) {
	out := map[int64]*Targets{}
	if len(sourceIDs) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(sourceIDs)), ",")
	args := []any{sourceKind}
	for _, id := range sourceIDs {
		args = append(args, id)
	}
	rows, err := q.QueryContext(ctx, `SELECT a.source_id, a.target_kind, a.target_id FROM activity_target a
		WHERE a.source_kind = ? AND a.deleted_at IS NULL AND a.source_id IN (`+ph+`)
		AND CASE a.target_kind
		      WHEN 'person' THEN (SELECT archived FROM people WHERE id = a.target_id)
		      WHEN 'organization' THEN (SELECT archived FROM organizations WHERE id = a.target_id)
		      ELSE (SELECT archived FROM deals WHERE id = a.target_id) END = 0
		ORDER BY a.target_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("load %s targets: %w", sourceKind, err)
	}
	defer rows.Close()
	for rows.Next() {
		var src, tid int64
		var kind string
		if err := rows.Scan(&src, &kind, &tid); err != nil {
			return nil, err
		}
		t := out[src]
		if t == nil {
			t = &Targets{}
			out[src] = t
		}
		switch kind {
		case "person":
			t.People = append(t.People, tid)
		case "organization":
			t.Orgs = append(t.Orgs, tid)
		default:
			t.Deals = append(t.Deals, tid)
		}
	}
	return out, rows.Err()
}
