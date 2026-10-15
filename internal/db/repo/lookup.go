package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Lookup is a short description of one record, used to preview a change.
type Lookup struct {
	Entity   string `json:"entity"`
	ID       int64  `json:"id"`
	Label    string `json:"label"`
	Archived bool   `json:"archived"`
}

// lookupSQL maps an entity to a query returning its label and archived flag.
var lookupSQL = map[string]string{
	"person":       "SELECT trim(first_name || ' ' || coalesce(last_name, '')), archived FROM people WHERE id = ?",
	"organization": "SELECT name, archived FROM organizations WHERE id = ?",
	"deal":         "SELECT title, archived FROM deals WHERE id = ?",
	"task":         "SELECT title, archived FROM tasks WHERE id = ?",
	"interaction":  "SELECT coalesce(subject, type), archived FROM interactions WHERE id = ?",
	"event":        "SELECT title, archived FROM events WHERE id = ?",
}

// LookupRecord finds a record by entity and ID, archived or not. It returns ErrNotFound when
// the record does not exist and ErrValidation for an unknown entity.
func LookupRecord(ctx context.Context, db *sql.DB, entity string, id int64) (*Lookup, error) {
	q, ok := lookupSQL[entity]
	if !ok {
		return nil, fmt.Errorf("unknown entity %q: %w", entity, model.ErrValidation)
	}
	l := &Lookup{Entity: entity, ID: id}
	var archived int
	err := db.QueryRowContext(ctx, q, id).Scan(&l.Label, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s %d: %w", entity, id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup %s %d: %w", entity, id, err)
	}
	l.Archived = archived != 0
	return l, nil
}
