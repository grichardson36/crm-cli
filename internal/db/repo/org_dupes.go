package repo

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// "Not a duplicate" for organization merge candidates (owner decision 2026-10-27). A dismissed pair
// is stored as a row in org_dupe_dismissals (uuid, timestamps, deleted_at) so it can sync later, it
// no longer shows up in FindOrgCandidates, and it is reversible: UndismissOrgPair soft deletes the
// row and dismissing the pair again revives it.

func orderedOrgPair(a, b int64) (int64, int64, error) {
	if a == b {
		return 0, 0, fmt.Errorf("an organization cannot be dismissed against itself: %w", model.ErrValidation)
	}
	if a > b {
		a, b = b, a
	}
	return a, b, nil
}

func (r *OrgRepo) mustExistOrg(ctx context.Context, id int64) error {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("organization %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// DismissOrgPair marks two organizations as not duplicates. It says whether anything changed:
// dismissing a pair that is already dismissed is a no-op.
func (r *OrgRepo) DismissOrgPair(ctx context.Context, idA, idB int64) (bool, error) {
	a, b, err := orderedOrgPair(idA, idB)
	if err != nil {
		return false, err
	}
	for _, id := range []int64{a, b} {
		if err := r.mustExistOrg(ctx, id); err != nil {
			return false, err
		}
	}
	var deleted sql.NullString
	err = r.db.QueryRowContext(ctx, `SELECT deleted_at FROM org_dupe_dismissals WHERE org_a = ? AND org_b = ?`, a, b).Scan(&deleted)
	switch {
	case err == nil && !deleted.Valid:
		return false, nil
	case err == nil:
		_, err = r.db.ExecContext(ctx, `UPDATE org_dupe_dismissals SET deleted_at = NULL, dismissed_at = datetime('now'), updated_at = datetime('now')
			WHERE org_a = ? AND org_b = ?`, a, b)
	case err == sql.ErrNoRows:
		_, err = r.db.ExecContext(ctx, `INSERT INTO org_dupe_dismissals (uuid, org_a, org_b) VALUES (?, ?, ?)`, uuid.New().String(), a, b)
	}
	if err != nil {
		return false, fmt.Errorf("dismiss: %w", err)
	}
	return true, nil
}

// UndismissOrgPair makes a dismissed pair a merge candidate again (soft delete).
func (r *OrgRepo) UndismissOrgPair(ctx context.Context, idA, idB int64) error {
	a, b, err := orderedOrgPair(idA, idB)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE org_dupe_dismissals SET deleted_at = datetime('now'), updated_at = datetime('now')
		WHERE org_a = ? AND org_b = ? AND deleted_at IS NULL`, a, b)
	if err != nil {
		return fmt.Errorf("undismiss: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no dismissal for organizations %d and %d: %w", a, b, model.ErrNotFound)
	}
	return nil
}

func (r *OrgRepo) dismissedOrgPairs(ctx context.Context) (map[[2]int64]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT org_a, org_b FROM org_dupe_dismissals WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list dismissals: %w", err)
	}
	defer rows.Close()
	out := map[[2]int64]bool{}
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[[2]int64{a, b}] = true
	}
	return out, rows.Err()
}

// DismissedOrgCandidates lists the pairs that were dismissed and whose organizations are both live,
// with the reason they matched, so they can be shown and un-dismissed.
func (r *OrgRepo) DismissedOrgCandidates(ctx context.Context) ([]OrgCandidate, error) {
	all, err := r.findOrgCandidates(ctx, 0, false)
	if err != nil {
		return nil, err
	}
	dismissed, err := r.dismissedOrgPairs(ctx)
	if err != nil {
		return nil, err
	}
	out := []OrgCandidate{}
	for _, c := range all {
		if dismissed[[2]int64{min(c.AID, c.BID), max(c.AID, c.BID)}] {
			out = append(out, c)
		}
	}
	return out, nil
}
