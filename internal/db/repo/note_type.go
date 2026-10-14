package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

var noteTypeNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// NoteTypeRepo handles note type database operations.
type NoteTypeRepo struct {
	db *sql.DB
}

// NewNoteTypeRepo creates a new NoteTypeRepo.
func NewNoteTypeRepo(db *sql.DB) *NoteTypeRepo {
	return &NoteTypeRepo{db: db}
}

const noteTypeColumns = "id, uuid, name, counts_as_contact, archived, created_at, updated_at"

func scanNoteType(row interface{ Scan(...any) error }) (*model.NoteType, error) {
	var n model.NoteType
	if err := row.Scan(&n.ID, &n.UUID, &n.Name, &n.CountsAsContact, &n.Archived, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}
	return &n, nil
}

// List returns note types ordered by name. Archived types are included only when all is true.
func (r *NoteTypeRepo) List(ctx context.Context, all bool) ([]*model.NoteType, error) {
	query := "SELECT " + noteTypeColumns + " FROM note_types"
	if !all {
		query += " WHERE archived = 0"
	}
	query += " ORDER BY name"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list note types: %w", err)
	}
	defer rows.Close()
	var out []*model.NoteType
	for rows.Next() {
		n, err := scanNoteType(rows)
		if err != nil {
			return nil, fmt.Errorf("scan note type: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// FindByName returns a note type by name (case-insensitive), archived or not.
func (r *NoteTypeRepo) FindByName(ctx context.Context, name string) (*model.NoteType, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+noteTypeColumns+" FROM note_types WHERE name = ?", strings.TrimSpace(name))
	n, err := scanNoteType(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("note type %q: %w", name, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find note type %q: %w", name, err)
	}
	return n, nil
}

// Add creates a note type. If an archived type with the same name exists it is
// restored and its counts_as_contact value updated.
func (r *NoteTypeRepo) Add(ctx context.Context, name string, countsAsContact bool) (*model.NoteType, error) {
	name = strings.TrimSpace(name)
	if !noteTypeNameRE.MatchString(name) {
		return nil, fmt.Errorf("invalid note type name %q (letters, digits, - and _ only): %w", name, model.ErrValidation)
	}
	existing, err := r.FindByName(ctx, name)
	switch {
	case err == nil:
		if !existing.Archived {
			return nil, fmt.Errorf("note type %q already exists: %w", existing.Name, model.ErrConflict)
		}
		if _, err := r.db.ExecContext(ctx,
			"UPDATE note_types SET archived = 0, counts_as_contact = ?, updated_at = datetime('now') WHERE id = ?",
			countsAsContact, existing.ID); err != nil {
			return nil, fmt.Errorf("restore note type: %w", err)
		}
		return r.FindByName(ctx, name)
	case !errors.Is(err, model.ErrNotFound):
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx,
		"INSERT INTO note_types (uuid, name, counts_as_contact) VALUES (?, ?, ?)",
		uuid.New().String(), name, countsAsContact); err != nil {
		return nil, fmt.Errorf("insert note type: %w", err)
	}
	return r.FindByName(ctx, name)
}

// Archive hides a note type from new interactions. Existing interactions keep it.
func (r *NoteTypeRepo) Archive(ctx context.Context, name string) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE note_types SET archived = 1, updated_at = datetime('now') WHERE name = ? AND archived = 0",
		strings.TrimSpace(name))
	if err != nil {
		return fmt.Errorf("archive note type: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("note type %q: %w", name, model.ErrNotFound)
	}
	return nil
}

// resolveActiveType returns the canonical name of an active note type.
func resolveActiveType(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, name string) (string, error) {
	var canonical string
	err := q.QueryRowContext(ctx,
		"SELECT name FROM note_types WHERE name = ? AND archived = 0", strings.TrimSpace(name)).Scan(&canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("invalid interaction type %q (see 'crm log types'): %w", name, model.ErrValidation)
	}
	if err != nil {
		return "", fmt.Errorf("resolve note type %q: %w", name, err)
	}
	return canonical, nil
}
