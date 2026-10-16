package repo

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// ArchiveObject is the one soft delete for every object kind (person, organization, deal, task).
func ArchiveObject(ctx context.Context, db *sql.DB, kind string, id int64) error {
	switch kind {
	case "person":
		return NewPersonRepo(db).Archive(ctx, id)
	case "organization":
		return NewOrgRepo(db).Archive(ctx, id)
	case "deal":
		return NewDealRepo(db).Archive(ctx, id)
	case "task":
		return NewTaskRepo(db).Archive(ctx, id)
	}
	return fmt.Errorf("unknown object %q: %w", kind, model.ErrValidation)
}

// RestoreObject is the one restore for every object kind. It brings back a soft-deleted row.
func RestoreObject(ctx context.Context, db *sql.DB, kind string, id int64) error {
	switch kind {
	case "person":
		return NewPersonRepo(db).Restore(ctx, id)
	case "organization":
		return NewOrgRepo(db).Restore(ctx, id)
	case "deal":
		return NewDealRepo(db).Restore(ctx, id)
	case "task":
		return NewTaskRepo(db).Restore(ctx, id)
	}
	return fmt.Errorf("unknown object %q: %w", kind, model.ErrValidation)
}
