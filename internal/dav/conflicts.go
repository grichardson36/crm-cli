package dav

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// UndoConflict restores the value that lost a conflict, through the person repo so updated_at,
// field versions and the search index stay correct, and marks the conflict undone. The change
// reaches the phone on its next sync. It fails with ErrConflict when the field has changed
// since the conflict was resolved, because undoing would then overwrite newer work.
func UndoConflict(ctx context.Context, db *sql.DB, id int64) (*model.SyncConflict, error) {
	davRepo := repo.NewDavRepo(db)
	people := repo.NewPersonRepo(db)
	c, err := davRepo.Conflict(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Undone {
		return nil, fmt.Errorf("sync conflict %d is already undone: %w", id, model.ErrConflict)
	}
	if c.Winner == model.ConflictWinnerLink || c.Field == fieldLink {
		return nil, fmt.Errorf("sync conflict %d records a link, not a changed value, so there is nothing to undo: %w", id, model.ErrValidation)
	}
	p, err := people.FindByID(ctx, c.PersonID)
	if err != nil {
		return nil, fmt.Errorf("person %d is archived or gone, restore it first: %w", c.PersonID, model.ErrConflict)
	}
	if err := people.LoadDetails(ctx, p); err != nil {
		return nil, err
	}

	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.TrimSpace(*p)
	}
	winnerVal, loserVal := str(c.DeviceValue), str(c.CRMValue)
	if c.Winner == model.ConflictWinnerCRM {
		winnerVal, loserVal = loserVal, winnerVal
	}

	var current string
	if kind, ok := strings.CutPrefix(c.Field, handlePrefix); ok {
		for _, h := range p.Handles {
			if h.Kind == kind {
				current = strings.TrimSpace(h.Value)
			}
		}
		if current != winnerVal && handleKey(current) != handleKey(winnerVal) {
			return nil, fmt.Errorf("%s changed since the conflict (now %q): %w", c.Field, current, model.ErrConflict)
		}
		if loserVal == "" {
			if err := people.RemoveHandle(ctx, p.ID, kind); err != nil {
				return nil, err
			}
		} else if err := people.SetHandle(ctx, p.ID, kind, loserVal); err != nil {
			return nil, err
		}
	} else {
		raw := rawScalars(p)
		cur, known := raw[c.Field]
		if !known {
			return nil, fmt.Errorf("unknown field %q: %w", c.Field, model.ErrValidation)
		}
		if cur != winnerVal {
			return nil, fmt.Errorf("%s changed since the conflict (now %q): %w", c.Field, cur, model.ErrConflict)
		}
		in := model.UpdatePersonInput{}
		switch c.Field {
		case fieldFirst:
			if loserVal == "" {
				return nil, fmt.Errorf("first name cannot be empty: %w", model.ErrValidation)
			}
			in.FirstName = &loserVal
		case fieldLast:
			in.LastName = &loserVal
		case fieldCompany:
			in.Company = &loserVal
		case fieldTitle:
			in.Title = &loserVal
		case fieldNotes:
			in.Notes = &loserVal
		case fieldLocation:
			in.Location = &loserVal
		case fieldBirthday:
			in.Birthday = &loserVal
		}
		if _, err := people.Update(ctx, p.ID, in); err != nil {
			return nil, err
		}
		if c.Field == fieldCompany && loserVal != "" {
			if _, err := repo.NewOrgAutoRepo(db).ApplyCompany(ctx, p.ID, loserVal, repo.ModeRepoint); err != nil {
				return nil, err
			}
		}
	}
	if err := davRepo.MarkConflictUndone(ctx, id); err != nil {
		return nil, err
	}
	return davRepo.Conflict(ctx, id)
}
