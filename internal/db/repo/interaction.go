package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// InteractionRepo handles interaction database operations.
type InteractionRepo struct {
	db *sql.DB
}

// NewInteractionRepo creates a new InteractionRepo.
func NewInteractionRepo(db *sql.DB) *InteractionRepo {
	return &InteractionRepo{db: db}
}

var interactionColumns = "id, uuid, type, subject, content, direction, occurred_at, created_at, updated_at"

func scanInteraction(row interface{ Scan(...any) error }) (*model.Interaction, error) {
	var i model.Interaction
	err := row.Scan(
		&i.ID, &i.UUID, &i.Type, &i.Subject, &i.Content,
		&i.Direction, &i.OccurredAt, &i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// Create inserts a new interaction and links it to people.
func (r *InteractionRepo) Create(ctx context.Context, input model.CreateInteractionInput) (*model.Interaction, error) {
	if err := validate.InteractionCreate(&input).Err(); err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	typeName, err := resolveActiveType(ctx, tx, input.Type)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	occurredAt := "datetime('now')"
	args := []any{id, typeName, input.Subject, input.Content, input.Direction}

	var query string
	if input.OccurredAt != nil {
		query = `INSERT INTO interactions (uuid, type, subject, content, direction, occurred_at)
				 VALUES (?, ?, ?, ?, ?, ?)`
		args = append(args, *input.OccurredAt)
	} else {
		query = fmt.Sprintf(`INSERT INTO interactions (uuid, type, subject, content, direction, occurred_at)
				 VALUES (?, ?, ?, ?, ?, %s)`, occurredAt)
	}

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insert interaction: %w", err)
	}

	rowID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	// Link to people, organizations and deals
	for _, g := range []struct {
		kind string
		ids  []int64
	}{{"person", input.PersonIDs}, {"organization", input.OrgIDs}, {"deal", input.DealIDs}} {
		if err := addTargets(ctx, tx, "interaction", rowID, g.kind, g.ids); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return r.FindByID(ctx, rowID)
}

// Update edits an active interaction. The type must be an active note type.
// Subject, Content and Direction accept "" to clear. FTS follows through the update trigger.
func (r *InteractionRepo) Update(ctx context.Context, id int64, input model.UpdateInteractionInput) (*model.Interaction, error) {
	if err := validate.InteractionUpdate(&input).Err(); err != nil {
		return nil, err
	}
	var sets []string
	var args []any

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if input.Type != nil {
		typeName, err := resolveActiveType(ctx, tx, *input.Type)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "type = ?")
		args = append(args, typeName)
	}
	if input.Subject != nil {
		sets = append(sets, "subject = ?")
		args = append(args, nullIfEmpty(*input.Subject))
	}
	if input.Content != nil {
		sets = append(sets, "content = ?")
		args = append(args, nullIfEmpty(*input.Content))
	}
	if input.Direction != nil {
		sets = append(sets, "direction = ?")
		args = append(args, nullIfEmpty(*input.Direction))
	}
	if input.OccurredAt != nil {
		if strings.TrimSpace(*input.OccurredAt) == "" {
			return nil, fmt.Errorf("occurred_at cannot be empty: %w", model.ErrValidation)
		}
		sets = append(sets, "occurred_at = ?")
		args = append(args, *input.OccurredAt)
	}
	if len(sets) == 0 {
		return r.FindByID(ctx, id)
	}

	sets = append(sets, "updated_at = datetime('now')")
	args = append(args, id)
	res, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE interactions SET %s WHERE id = ? AND archived = 0", strings.Join(sets, ", ")), args...)
	if err != nil {
		return nil, fmt.Errorf("update interaction %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("interaction %d: %w", id, model.ErrNotFound)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}
	return r.FindByID(ctx, id)
}

// Archive soft-deletes an interaction. It stops counting toward keep-in-touch and drops out of search.
func (r *InteractionRepo) Archive(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE interactions SET archived = 1, updated_at = datetime('now') WHERE id = ? AND archived = 0", id)
	if err != nil {
		return fmt.Errorf("archive interaction %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("interaction %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// Restore brings an archived interaction back.
func (r *InteractionRepo) Restore(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE interactions SET archived = 0, updated_at = datetime('now') WHERE id = ? AND archived = 1", id)
	if err != nil {
		return fmt.Errorf("restore interaction %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("archived interaction %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// FindByID returns an interaction by ID with its person IDs.
func (r *InteractionRepo) FindByID(ctx context.Context, id int64) (*model.Interaction, error) {
	row := r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s FROM interactions WHERE id = ? AND archived = 0", interactionColumns), id)
	i, err := scanInteraction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("interaction %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find interaction %d: %w", id, err)
	}

	if err := r.loadTargets(ctx, i); err != nil {
		return nil, err
	}

	return i, nil
}

// FindAll lists interactions with optional filters.
func (r *InteractionRepo) FindAll(ctx context.Context, filters model.InteractionFilters) ([]*model.Interaction, error) {
	query := fmt.Sprintf("SELECT %s FROM interactions WHERE archived = 0", interactionColumns)
	var args []any

	for _, f := range []struct {
		kind string
		id   *int64
	}{{"person", filters.PersonID}, {"organization", filters.OrgID}, {"deal", filters.DealID}} {
		if f.id != nil {
			query += " AND id IN (SELECT source_id FROM activity_target WHERE source_kind = 'interaction' AND target_kind = '" + f.kind + "' AND target_id = ? AND deleted_at IS NULL)"
			args = append(args, *f.id)
		}
	}

	if filters.Type != nil {
		query += " AND type = ?"
		args = append(args, *filters.Type)
	}

	query += " ORDER BY occurred_at DESC"

	if filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list interactions: %w", err)
	}
	defer rows.Close()

	// Collect interactions first, then close rows before querying person IDs.
	// This avoids deadlock with MaxOpenConns(1) since getPersonIDs needs
	// its own connection.
	var interactions []*model.Interaction
	for rows.Next() {
		i, err := scanInteraction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan interaction: %w", err)
		}
		interactions = append(interactions, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interactions: %w", err)
	}
	rows.Close()

	// Batch-fetch the linked people, organizations and deals
	if err := r.loadTargets(ctx, interactions...); err != nil {
		return nil, err
	}

	return interactions, nil
}

// Search finds interactions matching a full-text query.
func (r *InteractionRepo) Search(ctx context.Context, query string, limit int) ([]*model.Interaction, error) {
	if limit <= 0 {
		limit = 20
	}

	ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"` + "*"
	sqlStr := fmt.Sprintf(
		`SELECT %s FROM interactions WHERE archived = 0 AND id IN (SELECT rowid FROM interactions_fts WHERE interactions_fts MATCH ?) ORDER BY occurred_at DESC LIMIT ?`,
		interactionColumns,
	)

	rows, err := r.db.QueryContext(ctx, sqlStr, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search interactions: %w", err)
	}
	defer rows.Close()

	var interactions []*model.Interaction
	for rows.Next() {
		i, err := scanInteraction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan interaction: %w", err)
		}
		interactions = append(interactions, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interactions: %w", err)
	}
	rows.Close()

	if err := r.loadTargets(ctx, interactions...); err != nil {
		return nil, err
	}

	return interactions, nil
}

// loadTargets batch-loads the linked people, organizations and deals of interactions.
func (r *InteractionRepo) loadTargets(ctx context.Context, interactions ...*model.Interaction) error {
	ids := make([]int64, len(interactions))
	for i, inter := range interactions {
		ids[i] = inter.ID
	}
	m, err := loadTargets(ctx, r.db, "interaction", ids)
	if err != nil {
		return err
	}
	for _, inter := range interactions {
		inter.PersonIDs, inter.OrgIDs, inter.DealIDs = nil, nil, nil
		if x := m[inter.ID]; x != nil {
			inter.PersonIDs, inter.OrgIDs, inter.DealIDs = x.People, x.Orgs, x.Deals
		}
	}
	return nil
}
