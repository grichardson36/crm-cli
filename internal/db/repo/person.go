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

// PersonRepo handles person database operations.
type PersonRepo struct {
	db *sql.DB
}

// NewPersonRepo creates a new PersonRepo.
func NewPersonRepo(db *sql.DB) *PersonRepo {
	return &PersonRepo{db: db}
}

// DB returns the database the repository works on.
func (r *PersonRepo) DB() *sql.DB { return r.db }

var personColumns = "id, uuid, first_name, last_name, email, phone, title, company, location, notes, summary, created_at, updated_at, birthday_month, birthday_day, birthday_year, keep_in_touch, keep_in_touch_snoozed_until, last_contacted_at, is_favorite, priority_tier"

func scanPerson(row interface{ Scan(...any) error }) (*model.Person, error) {
	var p model.Person
	err := row.Scan(
		&p.ID, &p.UUID, &p.FirstName, &p.LastName,
		&p.Email, &p.Phone, &p.Title, &p.Company,
		&p.Location, &p.Notes, &p.Summary,
		&p.CreatedAt, &p.UpdatedAt,
		&p.BirthdayMonth, &p.BirthdayDay, &p.BirthdayYear,
		&p.KeepInTouch, &p.KeepInTouchSnoozedUntil, &p.LastContactedAt,
		&p.IsFavorite, &p.PriorityTier,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Create inserts a new person after checking for duplicates.
func (r *PersonRepo) Create(ctx context.Context, input model.CreatePersonInput) (*model.Person, error) {
	// Check for duplicate: same first_name + last_name, or same email
	if err := r.checkDuplicate(ctx, input); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO people (uuid, first_name, last_name, email, phone, title, company, location, notes)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.FirstName, input.LastName, input.Email, input.Phone,
		input.Title, input.Company, input.Location, input.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("insert person: %w", err)
	}

	rowID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	if err := r.syncPrimary(ctx, rowID, input.Email, input.Phone); err != nil {
		return nil, err
	}
	if err := r.setPrimaryOrg(ctx, rowID, input.OrgID); err != nil {
		return nil, err
	}

	return r.FindByID(ctx, rowID)
}

// setPrimaryOrg makes the organization the person's primary one, adding the membership if the
// person has none there. A nil or zero id changes nothing.
func (r *PersonRepo) setPrimaryOrg(ctx context.Context, personID int64, orgID *int64) error {
	if orgID == nil || *orgID == 0 {
		return nil
	}
	yes := true
	_, err := NewLinkRepo(r.db).Link(ctx, Ref{"person", personID}, Ref{"organization", *orgID}, model.LinkAttrs{Primary: &yes})
	return err
}

func (r *PersonRepo) checkDuplicate(ctx context.Context, input model.CreatePersonInput) error {
	// Check by email if provided
	if input.Email != nil && *input.Email != "" {
		var count int
		err := r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM people p WHERE p.archived = 0
			 AND (p.email = ? OR p.id IN (SELECT person_id FROM person_emails WHERE email = ?))`, *input.Email, *input.Email).Scan(&count)
		if err != nil {
			return fmt.Errorf("check duplicate email: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("a person with email %q already exists: %w", *input.Email, model.ErrConflict)
		}
	}

	// Check by full name if last name is provided
	if input.LastName != nil && *input.LastName != "" {
		var count int
		err := r.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM people WHERE first_name = ? AND last_name = ? AND archived = 0",
			input.FirstName, *input.LastName).Scan(&count)
		if err != nil {
			return fmt.Errorf("check duplicate name: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("a person named %q %q already exists: %w", input.FirstName, *input.LastName, model.ErrConflict)
		}
	}

	return nil
}

// FindByID returns a person by ID.
func (r *PersonRepo) FindByID(ctx context.Context, id int64) (*model.Person, error) {
	row := r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s FROM people WHERE id = ? AND archived = 0", personColumns), id)
	p, err := scanPerson(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("person %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find person %d: %w", id, err)
	}
	return p, nil
}

// FindAll lists people with optional filters.
func (r *PersonRepo) FindAll(ctx context.Context, filters model.PersonFilters) ([]*model.Person, error) {
	archived := 0
	if filters.Archived {
		archived = 1
	}
	query := fmt.Sprintf("SELECT %s FROM people WHERE archived = %d", personColumns, archived)
	var args []any

	if filters.Tag != nil {
		query = fmt.Sprintf(
			"SELECT p.%s FROM people p JOIN taggings tg ON tg.entity_type = 'person' AND tg.entity_id = p.id JOIN tags t ON t.id = tg.tag_id WHERE tg.deleted_at IS NULL AND p.archived = %d AND t.name = ?",
			strings.ReplaceAll(personColumns, ", ", ", p."), archived,
		)
		args = append(args, *filters.Tag)
	}

	if filters.OrgID != nil {
		col := "id"
		if filters.Tag != nil {
			col = "p.id"
		}
		query += " AND " + col + " IN (SELECT person_id FROM membership WHERE org_id = ? AND deleted_at IS NULL)"
		args = append(args, *filters.OrgID)
	}

	// The tag join also has an updated_at column, so qualify it.
	if filters.Tag != nil {
		query += " ORDER BY p.updated_at DESC"
	} else {
		query += " ORDER BY updated_at DESC"
	}

	if filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
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

// Update modifies a person's fields.
func (r *PersonRepo) Update(ctx context.Context, id int64, input model.UpdatePersonInput) (*model.Person, error) {
	var setClauses []string
	var args []any

	if input.FirstName != nil {
		setClauses = append(setClauses, "first_name = ?")
		args = append(args, *input.FirstName)
	}
	if input.LastName != nil {
		setClauses = append(setClauses, "last_name = ?")
		args = append(args, *input.LastName)
	}
	if input.Email != nil {
		setClauses = append(setClauses, "email = ?")
		args = append(args, *input.Email)
	}
	if input.Phone != nil {
		setClauses = append(setClauses, "phone = ?")
		args = append(args, *input.Phone)
	}
	if input.Title != nil {
		setClauses = append(setClauses, "title = ?")
		args = append(args, *input.Title)
	}
	if input.Company != nil {
		setClauses = append(setClauses, "company = ?")
		args = append(args, *input.Company)
	}
	if input.Location != nil {
		setClauses = append(setClauses, "location = ?")
		args = append(args, *input.Location)
	}
	if input.Notes != nil {
		setClauses = append(setClauses, "notes = ?")
		args = append(args, *input.Notes)
	}
	if input.Summary != nil {
		setClauses = append(setClauses, "summary = ?")
		args = append(args, *input.Summary)
	}

	if input.Birthday != nil {
		if *input.Birthday == "" {
			setClauses = append(setClauses, "birthday_month = NULL", "birthday_day = NULL", "birthday_year = NULL")
		} else {
			m, d, y, err := model.ParseBirthday(*input.Birthday)
			if err != nil {
				return nil, err
			}
			setClauses = append(setClauses, "birthday_month = ?", "birthday_day = ?", "birthday_year = ?")
			args = append(args, m, d, y)
		}
	}
	if input.KeepInTouch != nil {
		if *input.KeepInTouch == "" || *input.KeepInTouch == "unset" {
			setClauses = append(setClauses, "keep_in_touch = NULL", "keep_in_touch_set_at = NULL")
		} else if !model.ValidCadence(*input.KeepInTouch) {
			return nil, fmt.Errorf("invalid keep-in-touch cadence %q (must be one of: %s): %w", *input.KeepInTouch, strings.Join(model.KeepInTouchCadences, ", "), model.ErrValidation)
		} else {
			// Restart the first-due clock only when the cadence actually changes.
			setClauses = append(setClauses,
				"keep_in_touch_set_at = CASE WHEN keep_in_touch IS ? THEN keep_in_touch_set_at ELSE datetime('now') END",
				"keep_in_touch = ?")
			args = append(args, *input.KeepInTouch, *input.KeepInTouch)
		}
	}
	if input.SnoozedUntil != nil {
		setClauses = append(setClauses, "keep_in_touch_snoozed_until = ?")
		args = append(args, nullIfEmpty(*input.SnoozedUntil))
	}
	if input.LastContactedAt != nil {
		setClauses = append(setClauses, "last_contacted_at = ?")
		args = append(args, nullIfEmpty(*input.LastContactedAt))
	}
	if input.IsFavorite != nil {
		setClauses = append(setClauses, "is_favorite = ?")
		args = append(args, *input.IsFavorite)
	}
	if input.PriorityTier != nil {
		setClauses = append(setClauses, "priority_tier = ?")
		if *input.PriorityTier == 0 {
			args = append(args, nil)
		} else {
			args = append(args, *input.PriorityTier)
		}
	}

	if len(setClauses) == 0 {
		if err := r.setPrimaryOrg(ctx, id, input.OrgID); err != nil {
			return nil, err
		}
		return r.FindByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	// One transaction covers the update and its per-field change records.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	selectQuery := fmt.Sprintf("SELECT %s FROM people WHERE id = ? AND archived = 0", personColumns)
	before, err := scanPerson(tx.QueryRowContext(ctx, selectQuery, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("person %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find person %d: %w", id, err)
	}

	query := fmt.Sprintf("UPDATE people SET %s WHERE id = ? AND archived = 0", strings.Join(setClauses, ", "))
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("update person %d: %w", id, err)
	}

	after, err := scanPerson(tx.QueryRowContext(ctx, selectQuery, id))
	if err != nil {
		return nil, fmt.Errorf("reload person %d: %w", id, err)
	}
	if err := recordPersonFieldChanges(ctx, tx, id, before, after); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	if err := r.syncPrimary(ctx, id, input.Email, input.Phone); err != nil {
		return nil, err
	}
	if err := r.setPrimaryOrg(ctx, id, input.OrgID); err != nil {
		return nil, err
	}

	return r.FindByID(ctx, id)
}

// Archive soft-deletes a person.
func (r *PersonRepo) Archive(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE people SET archived = 1, updated_at = datetime('now') WHERE id = ? AND archived = 0", id)
	if err != nil {
		return fmt.Errorf("archive person %d: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("person %d: %w", id, model.ErrNotFound)
	}

	return nil
}

// Search performs full-text search on people.
func (r *PersonRepo) Search(ctx context.Context, query string, limit int) ([]*model.Person, error) {
	if limit <= 0 {
		limit = 20
	}

	sql := fmt.Sprintf(
		`SELECT %s FROM people WHERE archived = 0 AND (id IN (SELECT rowid FROM people_fts WHERE people_fts MATCH ?)
		 OR id IN (SELECT person_id FROM person_emails WHERE email LIKE ?)
		 OR id IN (SELECT person_id FROM person_phones WHERE phone_number LIKE ?)) ORDER BY updated_at DESC LIMIT ?`,
		personColumns,
	)

	// Quote the query to handle special FTS5 characters like @
	ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"` + "*"
	like := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx, sql, ftsQuery, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("search people: %w", err)
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
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	return people, nil
}
