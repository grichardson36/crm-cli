package repo

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// DupeRepo finds duplicate candidates and stores dismissals.
type DupeRepo struct {
	db *sql.DB
}

// NewDupeRepo creates a new DupeRepo.
func NewDupeRepo(db *sql.DB) *DupeRepo {
	return &DupeRepo{db: db}
}

// Find loads all active people and returns duplicate groups at or above min.
// Dismissed pairs are hidden.
func (r *DupeRepo) Find(ctx context.Context, min dedupe.Level) ([]dedupe.Group, error) {
	people, err := r.loadPeople(ctx)
	if err != nil {
		return nil, err
	}
	dismissed, err := r.dismissed(ctx)
	if err != nil {
		return nil, err
	}
	tbl, err := NewAliasRepo(r.db).Table(ctx)
	if err != nil {
		return nil, err
	}
	return dedupe.Find(people, dedupe.Options{Min: min, Dismissed: dismissed, Aliases: tbl}), nil
}

func (r *DupeRepo) loadPeople(ctx context.Context) ([]dedupe.Person, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, first_name, last_name, email, phone FROM people WHERE archived = 0`)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	defer rows.Close()
	var people []dedupe.Person
	index := map[int64]int{}
	for rows.Next() {
		var p dedupe.Person
		var last, email, phone sql.NullString
		if err := rows.Scan(&p.ID, &p.FirstName, &last, &email, &phone); err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		p.LastName = last.String
		if email.Valid {
			p.Emails = append(p.Emails, email.String)
		}
		if phone.Valid {
			p.Phones = append(p.Phones, phone.String)
		}
		index[p.ID] = len(people)
		people = append(people, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := r.each(ctx, `SELECT person_id, email FROM person_emails`, func(id int64, v string) {
		if i, ok := index[id]; ok {
			people[i].Emails = append(people[i].Emails, v)
		}
	}); err != nil {
		return nil, err
	}
	if err := r.each(ctx, `SELECT person_id, phone_number FROM person_phones`, func(id int64, v string) {
		if i, ok := index[id]; ok {
			people[i].Phones = append(people[i].Phones, v)
		}
	}); err != nil {
		return nil, err
	}
	hrows, err := r.db.QueryContext(ctx, `SELECT person_id, kind, value FROM person_handles`)
	if err != nil {
		return nil, fmt.Errorf("list handles: %w", err)
	}
	defer hrows.Close()
	for hrows.Next() {
		var id int64
		var kind, value string
		if err := hrows.Scan(&id, &kind, &value); err != nil {
			return nil, err
		}
		if i, ok := index[id]; ok {
			if people[i].Handles == nil {
				people[i].Handles = map[string]string{}
			}
			people[i].Handles[kind] = value
		}
	}
	return people, hrows.Err()
}

func (r *DupeRepo) each(ctx context.Context, query string, fn func(id int64, v string)) error {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			return err
		}
		fn(id, v)
	}
	return rows.Err()
}

func (r *DupeRepo) dismissed(ctx context.Context) (map[[2]int64]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT person_a, person_b FROM dupe_dismissals WHERE archived = 0`)
	if err != nil {
		return nil, fmt.Errorf("list dismissals: %w", err)
	}
	defer rows.Close()
	out := map[[2]int64]bool{}
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[[2]int64{a, b}] = true
	}
	return out, rows.Err()
}

func orderedPair(a, b int64) (int64, int64, error) {
	if a == b {
		return 0, 0, fmt.Errorf("a person cannot be dismissed against itself: %w", model.ErrValidation)
	}
	if a > b {
		a, b = b, a
	}
	return a, b, nil
}

func (r *DupeRepo) mustExist(ctx context.Context, id int64) error {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM people WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("person %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// Dismiss marks two people as not duplicates. Dismissing a pair twice is a no-op.
func (r *DupeRepo) Dismiss(ctx context.Context, idA, idB int64) error {
	a, b, err := orderedPair(idA, idB)
	if err != nil {
		return err
	}
	for _, id := range []int64{a, b} {
		if err := r.mustExist(ctx, id); err != nil {
			return err
		}
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO dupe_dismissals (person_a, person_b, uuid) VALUES (?, ?, ?)
		 ON CONFLICT(person_a, person_b) DO UPDATE SET archived = 0,
		   dismissed_at = datetime('now'), updated_at = datetime('now')`,
		a, b, uuid.New().String())
	if err != nil {
		return fmt.Errorf("dismiss: %w", err)
	}
	return nil
}

// Undismiss makes a dismissed pair eligible for duplicate detection again (soft delete).
func (r *DupeRepo) Undismiss(ctx context.Context, idA, idB int64) error {
	a, b, err := orderedPair(idA, idB)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE dupe_dismissals SET archived = 1, updated_at = datetime('now')
		 WHERE person_a = ? AND person_b = ? AND archived = 0`, a, b)
	if err != nil {
		return fmt.Errorf("undismiss: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no dismissal for people %d and %d: %w", a, b, model.ErrNotFound)
	}
	return nil
}
