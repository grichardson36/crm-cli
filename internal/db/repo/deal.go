package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// DealRepo handles deal database operations.
type DealRepo struct {
	db *sql.DB
}

// NewDealRepo creates a new DealRepo.
func NewDealRepo(db *sql.DB) *DealRepo {
	return &DealRepo{db: db}
}

const dealColumns = `id, uuid, title, value, stage, notes, closed_at, archived, created_at, updated_at`

func scanDeal(row interface{ Scan(...any) error }) (*model.Deal, error) {
	var d model.Deal
	err := row.Scan(
		&d.ID, &d.UUID, &d.Title, &d.Value, &d.Stage,
		&d.Notes, &d.ClosedAt,
		&d.Archived, &d.CreatedAt, &d.UpdatedAt,
	)
	return &d, err
}

// loadLinks fills the linked people and organizations of deals. Archived people and organizations
// are hidden. OrgID is the primary organization.
func (r *DealRepo) loadLinks(ctx context.Context, deals ...*model.Deal) error {
	for _, d := range deals {
		d.PersonIDs, d.OrgIDs, d.OrgID = nil, nil, nil
		rows, err := r.db.QueryContext(ctx, `SELECT p.id FROM deal_person dp JOIN people p ON p.id = dp.person_id
			WHERE dp.deal_id = ? AND dp.deleted_at IS NULL AND p.archived = 0 ORDER BY p.id`, d.ID)
		if err != nil {
			return fmt.Errorf("load deal people: %w", err)
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			d.PersonIDs = append(d.PersonIDs, id)
		}
		rows.Close()
		rows, err = r.db.QueryContext(ctx, `SELECT o.id, dorg.is_primary FROM deal_org dorg JOIN organizations o ON o.id = dorg.org_id
			WHERE dorg.deal_id = ? AND dorg.deleted_at IS NULL AND o.archived = 0 ORDER BY dorg.is_primary DESC, o.id`, d.ID)
		if err != nil {
			return fmt.Errorf("load deal organizations: %w", err)
		}
		for rows.Next() {
			var id int64
			var primary int
			if err := rows.Scan(&id, &primary); err != nil {
				rows.Close()
				return err
			}
			d.OrgIDs = append(d.OrgIDs, id)
			if primary == 1 {
				oid := id
				d.OrgID = &oid
			}
		}
		rows.Close()
	}
	return nil
}

// linkInput links the person and organization of a create or update call. The organization
// becomes the deal's primary one.
func (r *DealRepo) linkInput(ctx context.Context, dealID int64, personID, orgID *int64) error {
	lr := NewLinkRepo(r.db)
	if personID != nil {
		if _, err := lr.Link(ctx, Ref{"deal", dealID}, Ref{"person", *personID}, model.LinkAttrs{}); err != nil {
			return err
		}
	}
	if orgID != nil {
		yes := true
		if _, err := lr.Link(ctx, Ref{"deal", dealID}, Ref{"organization", *orgID}, model.LinkAttrs{Primary: &yes}); err != nil {
			return err
		}
	}
	return nil
}

func closedStage(stage string) bool { return stage == "won" || stage == "lost" }

// Create inserts a new deal.
func (r *DealRepo) Create(ctx context.Context, input model.CreateDealInput) (*model.Deal, error) {
	if !model.ValidDealStage(input.Stage) {
		return nil, fmt.Errorf("invalid deal stage %q: %w", input.Stage, model.ErrValidation)
	}

	id := uuid.New().String()
	// closed_at is set when a deal starts out won or lost (patterns/one-verb-set).
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO deals (uuid, title, value, stage, notes, closed_at)
		 VALUES (?, ?, ?, ?, ?, CASE WHEN ? IN ('won', 'lost') THEN datetime('now') END)`,
		id, input.Title, input.Value, input.Stage, input.Notes, input.Stage)
	if err != nil {
		return nil, fmt.Errorf("create deal: %w", err)
	}

	dealID, _ := result.LastInsertId()
	if err := r.linkInput(ctx, dealID, input.PersonID, input.OrgID); err != nil {
		return nil, err
	}
	return r.FindByID(ctx, dealID)
}

// FindByID returns a deal by ID.
func (r *DealRepo) FindByID(ctx context.Context, id int64) (*model.Deal, error) {
	d, err := scanDeal(r.db.QueryRowContext(ctx,
		`SELECT `+dealColumns+` FROM deals WHERE id = ? AND archived = 0`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("deal %d: %w", id, model.ErrNotFound)
		}
		return nil, fmt.Errorf("find deal %d: %w", id, err)
	}
	if err := r.loadLinks(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// FindAll returns deals with optional filters.
func (r *DealRepo) FindAll(ctx context.Context, filters model.DealFilters) ([]*model.Deal, error) {
	query := `SELECT ` + dealColumns + ` FROM deals WHERE archived = 0`
	var args []any

	if filters.Stage != nil {
		query += " AND stage = ?"
		args = append(args, *filters.Stage)
	}
	if filters.PersonID != nil {
		query += " AND id IN (SELECT deal_id FROM deal_person WHERE person_id = ? AND deleted_at IS NULL)"
		args = append(args, *filters.PersonID)
	}
	if filters.OrgID != nil {
		query += " AND id IN (SELECT deal_id FROM deal_org WHERE org_id = ? AND deleted_at IS NULL)"
		args = append(args, *filters.OrgID)
	}
	if filters.ExcludeClosed {
		query += " AND stage NOT IN ('won', 'lost')"
	}

	query += " ORDER BY created_at DESC"

	if filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list deals: %w", err)
	}
	defer rows.Close()

	var deals []*model.Deal
	for rows.Next() {
		d, err := scanDeal(rows)
		if err != nil {
			return nil, fmt.Errorf("scan deal: %w", err)
		}
		deals = append(deals, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := r.loadLinks(ctx, deals...); err != nil {
		return nil, err
	}
	return deals, nil
}

// Update modifies a deal.
func (r *DealRepo) Update(ctx context.Context, id int64, input model.UpdateDealInput) (*model.Deal, error) {
	var setClauses []string
	var args []any

	if input.Title != nil {
		setClauses = append(setClauses, "title = ?")
		args = append(args, *input.Title)
	}
	if input.Value != nil {
		setClauses = append(setClauses, "value = ?")
		args = append(args, *input.Value)
	}
	if input.Stage != nil {
		if !model.ValidDealStage(*input.Stage) {
			return nil, fmt.Errorf("invalid deal stage %q: %w", *input.Stage, model.ErrValidation)
		}
		setClauses = append(setClauses, "stage = ?")
		args = append(args, *input.Stage)
		// closed_at follows the stage (Salesforce): re-stamped to now on every move into won or
		// lost, including won to lost, and cleared when the deal reopens.
		if input.ClosedAt == nil {
			if closedStage(*input.Stage) {
				setClauses = append(setClauses, "closed_at = CASE WHEN stage <> ? THEN datetime('now') ELSE closed_at END")
				args = append(args, *input.Stage)
			} else {
				setClauses = append(setClauses, "closed_at = NULL")
			}
		}
	}
	if input.Notes != nil {
		setClauses = append(setClauses, "notes = ?")
		args = append(args, *input.Notes)
	}
	if input.ClosedAt != nil {
		setClauses = append(setClauses, "closed_at = ?")
		args = append(args, nullIfEmpty(*input.ClosedAt))
	}

	if len(setClauses) == 0 {
		if err := r.linkInput(ctx, id, input.PersonID, input.OrgID); err != nil {
			return nil, err
		}
		return r.FindByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE deals SET %s WHERE id = ? AND archived = 0", strings.Join(setClauses, ", "))
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update deal %d: %w", id, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("deal %d: %w", id, model.ErrNotFound)
	}
	if err := r.linkInput(ctx, id, input.PersonID, input.OrgID); err != nil {
		return nil, err
	}

	return r.FindByID(ctx, id)
}

// Archive soft-deletes a deal.
func (r *DealRepo) Archive(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE deals SET archived = 1, updated_at = datetime('now') WHERE id = ? AND archived = 0", id)
	if err != nil {
		return fmt.Errorf("archive deal %d: %w", id, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("deal %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// Search finds deals matching a full-text query.
func (r *DealRepo) Search(ctx context.Context, query string, limit int) ([]*model.Deal, error) {
	if limit <= 0 {
		limit = 20
	}

	ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"` + "*"
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+dealColumns+`
		 FROM deals WHERE archived = 0 AND id IN (SELECT rowid FROM deals_fts WHERE deals_fts MATCH ?)
		 ORDER BY updated_at DESC LIMIT ?`,
		ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search deals: %w", err)
	}
	defer rows.Close()

	var deals []*model.Deal
	for rows.Next() {
		d, err := scanDeal(rows)
		if err != nil {
			return nil, fmt.Errorf("scan deal: %w", err)
		}
		deals = append(deals, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := r.loadLinks(ctx, deals...); err != nil {
		return nil, err
	}
	return deals, nil
}

// Pipeline returns a summary of deals grouped by stage.
func (r *DealRepo) Pipeline(ctx context.Context) ([]*model.PipelineStage, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT stage, COUNT(*) as count, COALESCE(SUM(value), 0) as total_value
		 FROM deals WHERE archived = 0
		 GROUP BY stage
		 ORDER BY CASE stage
		   WHEN 'lead' THEN 1
		   WHEN 'prospect' THEN 2
		   WHEN 'proposal' THEN 3
		   WHEN 'negotiation' THEN 4
		   WHEN 'won' THEN 5
		   WHEN 'lost' THEN 6
		 END`)
	if err != nil {
		return nil, fmt.Errorf("pipeline: %w", err)
	}
	defer rows.Close()

	var stages []*model.PipelineStage
	for rows.Next() {
		var s model.PipelineStage
		if err := rows.Scan(&s.Stage, &s.Count, &s.TotalValue); err != nil {
			return nil, fmt.Errorf("scan pipeline stage: %w", err)
		}
		stages = append(stages, &s)
	}
	return stages, rows.Err()
}

// Restore brings an archived deal back.
func (r *DealRepo) Restore(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE deals SET archived = 0, updated_at = datetime('now') WHERE id = ? AND archived = 1", id)
	if err != nil {
		return fmt.Errorf("restore deal %d: %w", id, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("archived deal %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// Overwrite sets every editable field to the values of d, including empty ones. Undo and redo
// use it so a field that was empty goes back to empty.
func (r *DealRepo) Overwrite(ctx context.Context, id int64, d *model.Deal) error {
	if !model.ValidDealStage(d.Stage) {
		return fmt.Errorf("invalid deal stage %q: %w", d.Stage, model.ErrValidation)
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE deals SET title = ?, value = ?, stage = ?, notes = ?, closed_at = ?,
		        updated_at = datetime('now')
		 WHERE id = ? AND archived = 0`,
		d.Title, d.Value, d.Stage, d.Notes, d.ClosedAt, id)
	if err != nil {
		return fmt.Errorf("overwrite deal %d: %w", id, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("deal %d: %w", id, model.ErrNotFound)
	}
	return nil
}
