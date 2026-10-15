package repo

import (
	"context"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Reopen marks a completed task as not done. It returns ErrNotFound for a missing task and does
// nothing to a task that is already open.
func (r *TaskRepo) Reopen(ctx context.Context, id int64) (*model.Task, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET completed = 0, completed_at = NULL, updated_at = datetime('now') WHERE id = ? AND archived = 0 AND completed = 1`, id)
	if err != nil {
		return nil, fmt.Errorf("reopen task %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if err := recordFieldChanges(ctx, r.db, "task", id, map[string]string{"completed": "b:true"}, map[string]string{"completed": "b:false"}); err != nil {
			return nil, err
		}
	}
	return r.FindByID(ctx, id)
}
