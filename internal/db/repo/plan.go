package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// PlanRepo applies planner output to tasks and keeps the journal that makes it undoable.
type PlanRepo struct {
	db *sql.DB
}

// NewPlanRepo creates a new PlanRepo.
func NewPlanRepo(db *sql.DB) *PlanRepo { return &PlanRepo{db: db} }

// Chunk is one placed block of a task, RFC 3339 UTC.
type Chunk struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// PlanEntry is the new placement of one task. A nil Start unplaces it. Blocks has more than one
// element for a split task; the first block is the start.
type PlanEntry struct {
	TaskID int64
	Start  *string
	Blocks []Chunk
	Lock   bool
}

// JournalEntry is what a task had before a plan was applied.
type JournalEntry struct {
	TaskID     int64   `json:"task_id"`
	PrevStart  *string `json:"prev_start"`
	PrevLocked bool    `json:"prev_locked"`
	PrevChunks []Chunk `json:"prev_chunks"`
}

// PlanJournal is one applied plan.
type PlanJournal struct {
	ID        int64          `json:"id"`
	UUID      string         `json:"uuid"`
	Note      string         `json:"note"`
	Entries   []JournalEntry `json:"entries"`
	Undone    bool           `json:"undone"`
	CreatedAt string         `json:"created_at"`
}

func readChunks(ctx context.Context, q DBTX, taskID int64) ([]Chunk, error) {
	rows, err := q.QueryContext(ctx, `SELECT start_utc, end_utc FROM task_chunks WHERE task_id = ? ORDER BY start_utc`, taskID)
	if err != nil {
		return nil, fmt.Errorf("read chunks: %w", err)
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.Start, &c.End); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Chunks returns the placed blocks of every split task.
func (r *PlanRepo) Chunks(ctx context.Context) (map[int64][]Chunk, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.task_id, c.start_utc, c.end_utc FROM task_chunks c JOIN tasks t ON t.id = c.task_id WHERE t.archived = 0 ORDER BY c.task_id, c.start_utc`)
	if err != nil {
		return nil, fmt.Errorf("read chunks: %w", err)
	}
	defer rows.Close()
	out := map[int64][]Chunk{}
	for rows.Next() {
		var id int64
		var c Chunk
		if err := rows.Scan(&id, &c.Start, &c.End); err != nil {
			return nil, err
		}
		out[id] = append(out[id], c)
	}
	return out, rows.Err()
}

func writePlacement(ctx context.Context, tx DBTX, taskID int64, start *string, locked bool, chunks []Chunk) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE tasks SET scheduled_start = ?, scheduled_locked = ?, updated_at = datetime('now') WHERE id = ? AND archived = 0`, start, locked, taskID)
	if err != nil {
		return fmt.Errorf("place task %d: %w", taskID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("task %d: %w", taskID, model.ErrNotFound)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_chunks WHERE task_id = ?`, taskID); err != nil {
		return fmt.Errorf("clear chunks: %w", err)
	}
	for _, c := range chunks {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_chunks (task_id, start_utc, end_utc) VALUES (?, ?, ?)`, taskID, c.Start, c.End); err != nil {
			return fmt.Errorf("write chunk: %w", err)
		}
	}
	return nil
}

// Apply writes the placements in one transaction and journals what they replaced. Only
// scheduled_start, scheduled_locked and the chunks of the named tasks change.
func (r *PlanRepo) Apply(ctx context.Context, entries []PlanEntry, note string) (*PlanJournal, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("nothing to apply: %w", model.ErrValidation)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var journal []JournalEntry
	for _, e := range entries {
		if e.Start != nil {
			if _, err := tz.ParseInstant(*e.Start); err != nil {
				return nil, fmt.Errorf("task %d: bad start: %w", e.TaskID, model.ErrValidation)
			}
		}
		var prev JournalEntry
		prev.TaskID = e.TaskID
		var ps sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT scheduled_start, scheduled_locked FROM tasks WHERE id = ? AND archived = 0`, e.TaskID).Scan(&ps, &prev.PrevLocked)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("task %d: %w", e.TaskID, model.ErrNotFound)
		}
		if err != nil {
			return nil, fmt.Errorf("read task %d: %w", e.TaskID, err)
		}
		if ps.Valid {
			s := ps.String
			prev.PrevStart = &s
		}
		if prev.PrevChunks, err = readChunks(ctx, tx, e.TaskID); err != nil {
			return nil, err
		}
		journal = append(journal, prev)
		chunks := e.Blocks
		if len(chunks) < 2 {
			chunks = nil
		}
		if err := writePlacement(ctx, tx, e.TaskID, e.Start, e.Lock, chunks); err != nil {
			return nil, err
		}
	}
	js, err := json.Marshal(journal)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	res, err := tx.ExecContext(ctx, `INSERT INTO plan_journal (uuid, note, entries_json) VALUES (?, ?, ?)`, id, note, string(js))
	if err != nil {
		return nil, fmt.Errorf("journal plan: %w", err)
	}
	jid, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit plan: %w", err)
	}
	return r.find(ctx, jid)
}

func (r *PlanRepo) find(ctx context.Context, id int64) (*PlanJournal, error) {
	var j PlanJournal
	var js string
	err := r.db.QueryRowContext(ctx, `SELECT id, uuid, coalesce(note, ''), entries_json, undone, created_at FROM plan_journal WHERE id = ?`, id).Scan(&j.ID, &j.UUID, &j.Note, &js, &j.Undone, &j.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("plan %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	if err := json.Unmarshal([]byte(js), &j.Entries); err != nil {
		return nil, fmt.Errorf("read plan entries: %w", err)
	}
	return &j, nil
}

// History lists applied plans, newest first.
func (r *PlanRepo) History(ctx context.Context, limit int) ([]*PlanJournal, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM plan_journal ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []*PlanJournal
	for _, id := range ids {
		j, err := r.find(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

// Undo restores what the most recent applied plan replaced and marks it undone. It returns
// ErrNotFound when there is nothing to undo.
func (r *PlanRepo) Undo(ctx context.Context) (*PlanJournal, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM plan_journal WHERE undone = 0 ORDER BY id DESC LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no applied plan to undo: %w", model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find plan: %w", err)
	}
	j, err := r.find(ctx, id)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, e := range j.Entries {
		// A task deleted since is skipped.
		if err := writePlacement(ctx, tx, e.TaskID, e.PrevStart, e.PrevLocked, e.PrevChunks); err != nil && !errors.Is(err, model.ErrNotFound) {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE plan_journal SET undone = 1, updated_at = datetime('now') WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("mark plan undone: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit undo: %w", err)
	}
	j.Undone = true
	return j, nil
}
