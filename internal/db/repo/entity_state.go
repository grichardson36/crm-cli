package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// EntityState says whether a row exists and whether it is archived. entity is one of the
// entities that can carry external ids (person, task, ...).
func EntityState(ctx context.Context, q DBTX, entity string, id int64) (exists, archived bool, err error) {
	table, ok := externalEntityTables[entity]
	if !ok {
		return false, false, fmt.Errorf("unknown entity %q: %w", entity, model.ErrValidation)
	}
	var a int
	// table comes from the fixed map above, never from user input.
	err = q.QueryRowContext(ctx, "SELECT archived FROM "+table+" WHERE id = ?", id).Scan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("find %s %d: %w", entity, id, err)
	}
	return true, a != 0, nil
}

// FindByUUID returns a live task by uuid.
func (r *TaskRepo) FindByUUID(ctx context.Context, id string) (*model.Task, error) {
	t, err := scanTask(r.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE uuid = ? AND archived = 0`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("task %s: %w", id, model.ErrNotFound)
		}
		return nil, fmt.Errorf("find task %s: %w", id, err)
	}
	if err := r.loadPeople(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}
