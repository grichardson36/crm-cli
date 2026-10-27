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

// OrgRepo handles organization database operations.
type OrgRepo struct {
	db *sql.DB
}

// NewOrgRepo creates a new OrgRepo.
func NewOrgRepo(db *sql.DB) *OrgRepo {
	return &OrgRepo{db: db}
}

var orgColumns = "id, uuid, name, domain, industry, notes, summary, website, size, city, region, country, linkedin_url, description, logo_path, created_at, updated_at"

func scanOrg(row interface{ Scan(...any) error }) (*model.Organization, error) {
	var o model.Organization
	err := row.Scan(
		&o.ID, &o.UUID, &o.Name, &o.Domain, &o.Industry,
		&o.Notes, &o.Summary, &o.Website, &o.Size, &o.City, &o.Region, &o.Country, &o.LinkedinURL, &o.Description, &o.LogoPath,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// DB returns the database the repository works on.
func (r *OrgRepo) DB() *sql.DB { return r.db }

// Create inserts a new organization.
//
// Unless input.AllowDuplicate is set it refuses when a live organization has the same normalized
// name (case, punctuation, and/&, a leading "the" and trailing Inc, LLC and the like ignored, the
// same as NormalizeCompany) or the same normalized domain, and returns a *DuplicateOrgError.
func (r *OrgRepo) Create(ctx context.Context, input model.CreateOrgInput) (*model.Organization, error) {
	if err := validate.OrgCreate(&input).Err(); err != nil {
		return nil, err
	}
	if !input.AllowDuplicate {
		dup, reason, err := r.FindDuplicate(ctx, input.Name, input.Domain)
		if err != nil {
			return nil, err
		}
		if dup != nil {
			return nil, &DuplicateOrgError{Existing: dup, Reason: reason}
		}
	}
	id := uuid.New().String()
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO organizations (uuid, name, domain, industry, notes, website, size, city, region, country, linkedin_url, description, logo_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.Domain, input.Industry, input.Notes,
		input.Website, input.Size, input.City, input.Region, input.Country, input.LinkedinURL, input.Description, input.LogoPath,
	)
	if err != nil {
		return nil, fmt.Errorf("insert organization: %w", err)
	}

	rowID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	return r.FindByID(ctx, rowID)
}

// FindByID returns an organization by ID.
func (r *OrgRepo) FindByID(ctx context.Context, id int64) (*model.Organization, error) {
	row := r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s FROM organizations WHERE id = ? AND archived = 0", orgColumns), id)
	o, err := scanOrg(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("organization %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find organization %d: %w", id, err)
	}
	return o, nil
}

// FindAll lists organizations with optional filters.
func (r *OrgRepo) FindAll(ctx context.Context, limit int) ([]*model.Organization, error) {
	query := fmt.Sprintf("SELECT %s FROM organizations WHERE archived = 0 ORDER BY updated_at DESC", orgColumns)
	var args []any
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*model.Organization
	for rows.Next() {
		o, err := scanOrg(rows)
		if err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		orgs = append(orgs, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organizations: %w", err)
	}
	return orgs, nil
}

// Update modifies an organization's fields.
func (r *OrgRepo) Update(ctx context.Context, id int64, input model.UpdateOrgInput) (*model.Organization, error) {
	if err := validate.OrgUpdate(&input).Err(); err != nil {
		return nil, err
	}
	var setClauses []string
	var args []any

	if input.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *input.Name)
	}
	if input.Domain != nil {
		setClauses = append(setClauses, "domain = ?")
		args = append(args, *input.Domain)
	}
	if input.Industry != nil {
		setClauses = append(setClauses, "industry = ?")
		args = append(args, *input.Industry)
	}
	if input.Notes != nil {
		setClauses = append(setClauses, "notes = ?")
		args = append(args, *input.Notes)
	}
	if input.Summary != nil {
		setClauses = append(setClauses, "summary = ?")
		args = append(args, *input.Summary)
	}
	for _, f := range []struct {
		col string
		val *string
	}{
		{"website", input.Website}, {"size", input.Size}, {"city", input.City}, {"region", input.Region}, {"country", input.Country},
		{"linkedin_url", input.LinkedinURL}, {"description", input.Description}, {"logo_path", input.LogoPath},
	} {
		if f.val != nil {
			setClauses = append(setClauses, f.col+" = ?")
			args = append(args, *f.val)
		}
	}

	if len(setClauses) == 0 {
		return r.FindByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE organizations SET %s WHERE id = ? AND archived = 0", strings.Join(setClauses, ", "))
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update organization %d: %w", id, err)
	}
	if input.Name != nil {
		if err := r.touchMembers(ctx, id); err != nil {
			return nil, err
		}
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("organization %d: %w", id, model.ErrNotFound)
	}

	return r.FindByID(ctx, id)
}

// Archive soft-deletes an organization.
func (r *OrgRepo) Archive(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE organizations SET archived = 1, updated_at = datetime('now') WHERE id = ? AND archived = 0", id)
	if err != nil {
		return fmt.Errorf("archive organization %d: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("organization %d: %w", id, model.ErrNotFound)
	}

	return r.touchMembers(ctx, id)
}

// Search performs full-text search on organizations.
func (r *OrgRepo) Search(ctx context.Context, query string, limit int) ([]*model.Organization, error) {
	if limit <= 0 {
		limit = 20
	}

	ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"` + "*"
	sqlStr := fmt.Sprintf(
		`SELECT %s FROM organizations WHERE archived = 0 AND id IN (SELECT rowid FROM organizations_fts WHERE organizations_fts MATCH ?) ORDER BY updated_at DESC LIMIT ?`,
		orgColumns,
	)

	rows, err := r.db.QueryContext(ctx, sqlStr, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*model.Organization
	for rows.Next() {
		o, err := scanOrg(rows)
		if err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		orgs = append(orgs, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	return orgs, nil
}

// FindPeople returns the people with a live membership in an organization, current or past.
func (r *OrgRepo) FindPeople(ctx context.Context, orgID int64) ([]*model.Person, error) {
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT %s FROM people WHERE archived = 0
			AND id IN (SELECT person_id FROM membership WHERE org_id = ? AND deleted_at IS NULL) ORDER BY first_name`, personColumns), orgID)
	if err != nil {
		return nil, fmt.Errorf("find people for org %d: %w", orgID, err)
	}
	defer rows.Close()

	var people []*model.Person
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		people = append(people, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate people: %w", err)
	}
	return people, nil
}

// Restore brings an archived organization back.
func (r *OrgRepo) Restore(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE organizations SET archived = 0, updated_at = datetime('now') WHERE id = ? AND archived = 1", id)
	if err != nil {
		return fmt.Errorf("restore organization %d: %w", id, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("archived organization %d: %w", id, model.ErrNotFound)
	}
	return r.touchMembers(ctx, id)
}

// Overwrite sets every editable field to the values of o, including empty ones. Undo and redo
// use it so a field that was empty goes back to empty.
func (r *OrgRepo) Overwrite(ctx context.Context, id int64, o *model.Organization) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE organizations SET name = ?, domain = ?, industry = ?, notes = ?, website = ?, size = ?, city = ?, region = ?,
		 country = ?, linkedin_url = ?, description = ?, logo_path = ?, updated_at = datetime('now')
		 WHERE id = ? AND archived = 0`,
		o.Name, o.Domain, o.Industry, o.Notes, o.Website, o.Size, o.City, o.Region, o.Country, o.LinkedinURL, o.Description, o.LogoPath, id)
	if err != nil {
		return fmt.Errorf("overwrite organization %d: %w", id, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("organization %d: %w", id, model.ErrNotFound)
	}
	return r.touchMembers(ctx, id)
}

// touchMembers bumps updated_at of the people whose primary organization this is. The phone shows
// the organization name as the vCard ORG, so a rename must reach their contacts.
func (r *OrgRepo) touchMembers(ctx context.Context, orgID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE people SET updated_at = datetime('now') WHERE id IN
		(SELECT person_id FROM membership WHERE org_id = ? AND is_primary = 1 AND deleted_at IS NULL)`, orgID)
	if err != nil {
		return fmt.Errorf("touch members of organization %d: %w", orgID, err)
	}
	return nil
}

// DuplicateOrgError says a new organization would duplicate a live one. It is a conflict (exit code 4).
type DuplicateOrgError struct {
	Existing *model.Organization
	// Reason is "name" or "domain".
	Reason string
}

func (e *DuplicateOrgError) Error() string {
	return fmt.Sprintf("organization %q already exists with the same %s (id %d): use it, or pass --allow-duplicate (allow_duplicate in MCP) to create another", e.Existing.Name, e.Reason, e.Existing.ID)
}

// Unwrap makes the error a model.ErrConflict.
func (e *DuplicateOrgError) Unwrap() error { return model.ErrConflict }

// FindDuplicate returns a live organization whose normalized name or normalized domain equals the
// given ones, with the reason ("name" or "domain"). The name wins when both match different
// organizations. It returns nil when there is none.
func (r *OrgRepo) FindDuplicate(ctx context.Context, name string, domain *string) (*model.Organization, string, error) {
	key := NormalizeCompany(name)
	dom := ""
	if domain != nil {
		dom = normalizeDomain(*domain)
	}
	if key == "" && dom == "" {
		return nil, "", nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM organizations WHERE archived = 0 ORDER BY id", orgColumns))
	if err != nil {
		return nil, "", fmt.Errorf("check duplicate organization: %w", err)
	}
	defer rows.Close()
	var byDomain *model.Organization
	for rows.Next() {
		o, err := scanOrg(rows)
		if err != nil {
			return nil, "", err
		}
		if key != "" && NormalizeCompany(o.Name) == key {
			return o, "name", nil
		}
		if dom != "" && byDomain == nil && o.Domain != nil && normalizeDomain(*o.Domain) == dom {
			byDomain = o
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if byDomain != nil {
		return byDomain, "domain", nil
	}
	return nil, "", nil
}
