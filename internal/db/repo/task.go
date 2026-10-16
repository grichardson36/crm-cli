package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// TaskRepo handles task database operations.
type TaskRepo struct {
	db *sql.DB
}

// NewTaskRepo creates a new TaskRepo.
func NewTaskRepo(db *sql.DB) *TaskRepo {
	return &TaskRepo{db: db}
}

const taskColumns = `id, uuid, title, description, person_id, deal_id, due_at, priority,
	completed, completed_at, archived, created_at, updated_at, recurrence`

func scanTask(row interface{ Scan(...any) error }) (*model.Task, error) {
	var t model.Task
	err := row.Scan(
		&t.ID, &t.UUID, &t.Title, &t.Description, &t.PersonID, &t.DealID,
		&t.DueAt, &t.Priority, &t.Completed, &t.CompletedAt,
		&t.Archived, &t.CreatedAt, &t.UpdatedAt, &t.Recurrence,
	)
	return &t, err
}

// loadPeople fills t.PersonIDs from task_people.
func (r *TaskRepo) loadPeople(ctx context.Context, t *model.Task) error {
	rows, err := r.db.QueryContext(ctx, `SELECT person_id FROM task_people WHERE task_id = ? ORDER BY person_id`, t.ID)
	if err != nil {
		return fmt.Errorf("load task people: %w", err)
	}
	defer rows.Close()
	t.PersonIDs = nil
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		t.PersonIDs = append(t.PersonIDs, id)
	}
	return rows.Err()
}

// linkPeople links a task to people, ignoring ones already linked.
func (r *TaskRepo) linkPeople(ctx context.Context, taskID int64, ids []int64) error {
	for _, pid := range ids {
		if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO task_people (task_id, person_id) VALUES (?, ?)`, taskID, pid); err != nil {
			return fmt.Errorf("link person %d to task %d: %w", pid, taskID, err)
		}
	}
	return nil
}

// Create inserts a new task.
func (r *TaskRepo) Create(ctx context.Context, input model.CreateTaskInput) (*model.Task, error) {
	if !model.ValidPriority(input.Priority) {
		return nil, fmt.Errorf("invalid priority %q: %w", input.Priority, model.ErrValidation)
	}

	if input.Recurrence != nil {
		if _, _, _, err := model.ParseRecurrence(*input.Recurrence); err != nil {
			return nil, err
		}
		if input.DueAt == nil {
			return nil, fmt.Errorf("a recurring task needs a due date: %w", model.ErrValidation)
		}
	}

	cols := []string{"uuid", "title", "description", "person_id", "deal_id", "due_at", "priority", "recurrence"}
	id := uuid.New().String()
	vals := []any{id, input.Title, input.Description, input.PersonID, input.DealID, input.DueAt, input.Priority, input.Recurrence}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO tasks ("+strings.Join(cols, ", ")+") VALUES ("+ph+")", vals...)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	taskID, _ := result.LastInsertId()
	people := input.PersonIDs
	if input.PersonID != nil {
		people = append(people, *input.PersonID)
	}
	if err := r.linkPeople(ctx, taskID, people); err != nil {
		return nil, err
	}
	if len(input.Tags) > 0 {
		tr := NewTagRepo(r.db)
		for _, name := range input.Tags {
			if err := tr.Apply(ctx, "task", taskID, name); err != nil {
				return nil, err
			}
		}
	}
	return r.FindByID(ctx, taskID)
}

// FindByID returns a task by ID.
func (r *TaskRepo) FindByID(ctx context.Context, id int64) (*model.Task, error) {
	t, err := scanTask(r.db.QueryRowContext(ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE id = ? AND archived = 0`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("task %d: %w", id, model.ErrNotFound)
		}
		return nil, fmt.Errorf("find task %d: %w", id, err)
	}
	if err := r.loadPeople(ctx, t); err != nil {
		return nil, err
	}
	if err := r.loadTags(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// loadTags fills the tag names of a task.
func (r *TaskRepo) loadTags(ctx context.Context, t *model.Task) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT tg.name FROM tags tg JOIN taggings x ON x.tag_id = tg.id
		 WHERE x.entity_type = 'task' AND x.entity_id = ? ORDER BY tg.name`, t.ID)
	if err != nil {
		return fmt.Errorf("load task tags: %w", err)
	}
	defer rows.Close()
	t.Tags = nil
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan task tag: %w", err)
		}
		t.Tags = append(t.Tags, name)
	}
	return rows.Err()
}

// FindAll returns tasks with optional filters.
func (r *TaskRepo) FindAll(ctx context.Context, filters model.TaskFilters) ([]*model.Task, error) {
	query := `SELECT ` + taskColumns + ` FROM tasks WHERE archived = 0`
	var args []any

	if filters.PersonID != nil {
		query += " AND (person_id = ? OR id IN (SELECT task_id FROM task_people WHERE person_id = ?))"
		args = append(args, *filters.PersonID, *filters.PersonID)
	}
	if filters.DealID != nil {
		query += " AND deal_id = ?"
		args = append(args, *filters.DealID)
	}
	if filters.Tag != nil {
		query += " AND id IN (SELECT x.entity_id FROM taggings x JOIN tags tg ON tg.id = x.tag_id WHERE x.entity_type = 'task' AND tg.name = ?)"
		args = append(args, *filters.Tag)
	}
	if filters.Overdue {
		query += " AND completed = 0 AND due_at IS NOT NULL AND due_at < datetime('now')"
	}
	if !filters.IncludeCompleted {
		query += " AND completed = 0"
	}

	query += " ORDER BY CASE WHEN due_at IS NULL THEN 1 ELSE 0 END, due_at ASC, created_at DESC"

	if filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for _, t := range tasks {
		if err := r.loadPeople(ctx, t); err != nil {
			return nil, err
		}
		if err := r.loadTags(ctx, t); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

// Complete marks a task as completed.
func (r *TaskRepo) Complete(ctx context.Context, id int64) (*model.Task, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET completed = 1, completed_at = datetime('now'), updated_at = datetime('now')
		 WHERE id = ? AND archived = 0 AND completed = 0`, id)
	if err != nil {
		return nil, fmt.Errorf("complete task %d: %w", id, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Check if the task exists but is already completed
		var completed int
		err := r.db.QueryRowContext(ctx, "SELECT completed FROM tasks WHERE id = ? AND archived = 0", id).Scan(&completed)
		if err != nil {
			return nil, fmt.Errorf("task %d: %w", id, model.ErrNotFound)
		}
		if completed == 1 {
			return nil, fmt.Errorf("task %d is already completed: %w", id, model.ErrConflict)
		}
		return nil, fmt.Errorf("task %d: %w", id, model.ErrNotFound)
	}
	done, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := recordFieldChanges(ctx, r.db, "task", id, map[string]string{"completed": "b:false"}, map[string]string{"completed": "b:true"}); err != nil {
		return nil, err
	}
	if done.Recurrence != nil && done.DueAt != nil {
		nextDue, err := model.NextDue(*done.DueAt, *done.Recurrence, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		next, err := r.Create(ctx, model.CreateTaskInput{
			Title:       done.Title,
			Description: done.Description,
			PersonID:    done.PersonID,
			DealID:      done.DealID,
			DueAt:       &nextDue,
			Priority:    done.Priority,
			PersonIDs:   done.PersonIDs,
			Recurrence:  done.Recurrence,
		})
		if err != nil {
			return nil, err
		}
		done.NextTaskID = &next.ID
	}
	return done, nil
}

// Update modifies a task.
func (r *TaskRepo) Update(ctx context.Context, id int64, input model.UpdateTaskInput) (*model.Task, error) {
	var setClauses []string
	var args []any

	if input.Title != nil {
		setClauses = append(setClauses, "title = ?")
		args = append(args, *input.Title)
	}
	if input.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *input.Description)
	}
	if input.PersonID != nil {
		setClauses = append(setClauses, "person_id = ?")
		args = append(args, *input.PersonID)
	}
	if input.DealID != nil {
		setClauses = append(setClauses, "deal_id = ?")
		args = append(args, *input.DealID)
	}
	if input.DueAt != nil {
		setClauses = append(setClauses, "due_at = ?")
		args = append(args, *input.DueAt)
	}
	if input.Priority != nil {
		if !model.ValidPriority(*input.Priority) {
			return nil, fmt.Errorf("invalid priority %q: %w", *input.Priority, model.ErrValidation)
		}
		setClauses = append(setClauses, "priority = ?")
		args = append(args, *input.Priority)
	}

	if input.Recurrence != nil {
		if *input.Recurrence == "" {
			setClauses = append(setClauses, "recurrence = NULL")
		} else {
			if _, _, _, err := model.ParseRecurrence(*input.Recurrence); err != nil {
				return nil, err
			}
			setClauses = append(setClauses, "recurrence = ?")
			args = append(args, *input.Recurrence)
		}
	}

	if len(setClauses) == 0 {
		return r.FindByID(ctx, id)
	}

	before, _ := r.FindByID(ctx, id)
	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = ? AND archived = 0", strings.Join(setClauses, ", "))
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update task %d: %w", id, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("task %d: %w", id, model.ErrNotFound)
	}
	if input.PersonID != nil {
		if err := r.linkPeople(ctx, id, []int64{*input.PersonID}); err != nil {
			return nil, err
		}
	}

	after, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if before != nil {
		if err := recordFieldChanges(ctx, r.db, "task", id, taskFieldValues(before), taskFieldValues(after)); err != nil {
			return nil, err
		}
	}
	return after, nil
}

// Restore brings an archived task back.
func (r *TaskRepo) Restore(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE tasks SET archived = 0, updated_at = datetime('now') WHERE id = ? AND archived = 1", id)
	if err != nil {
		return fmt.Errorf("restore task %d: %w", id, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("archived task %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// Reopen marks a completed task as open again. It does not remove a next occurrence that
// Complete created: archive that task yourself.
func (r *TaskRepo) Reopen(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET completed = 0, completed_at = NULL, updated_at = datetime('now')
		 WHERE id = ? AND archived = 0 AND completed = 1`, id)
	if err != nil {
		return fmt.Errorf("reopen task %d: %w", id, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("completed task %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// Archive soft-deletes a task.
func (r *TaskRepo) Archive(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE tasks SET archived = 1, updated_at = datetime('now') WHERE id = ? AND archived = 0", id)
	if err != nil {
		return fmt.Errorf("archive task %d: %w", id, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task %d: %w", id, model.ErrNotFound)
	}
	return nil
}
