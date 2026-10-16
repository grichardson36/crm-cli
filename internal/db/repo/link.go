package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Ref names one object: its kind (person, organization, deal, task) and id.
type Ref struct {
	Kind string
	ID   int64
}

func (r Ref) String() string { return fmt.Sprintf("%s %d", r.Kind, r.ID) }

// LinkResult says what Link did. Replaced is set when the holder pointed at another object before
// (a person has one organization), so an undo can point it back.
type LinkResult struct {
	Changed  bool
	Replaced *Ref
}

// LinkRepo is the one place that makes, lists and removes links between objects
// (model.Relations). Every door calls it with the same names.
type LinkRepo struct {
	db     *sql.DB
	people *PersonRepo
}

// NewLinkRepo creates a LinkRepo.
func NewLinkRepo(db *sql.DB) *LinkRepo {
	return &LinkRepo{db: db, people: NewPersonRepo(db)}
}

// tableOf maps an object kind to its table. Names come from this fixed map, never from input.
var tableOf = map[string]string{"person": "people", "organization": "organizations", "deal": "deals", "task": "tasks"}

// holderColumn is the column of the holder row that stores a single-valued link.
func holderColumn(rel model.Relation) string {
	switch rel.Holder + ">" + rel.Target {
	case "person>organization", "deal>organization":
		return "org_id"
	case "deal>person":
		return "person_id"
	case "task>deal":
		return "deal_id"
	}
	return ""
}

func (r *LinkRepo) check(ctx context.Context, x Ref) error {
	table, ok := tableOf[x.Kind]
	if !ok {
		return fmt.Errorf("unknown object %q (use one of: person, organization, deal, task): %w", x.Kind, model.ErrValidation)
	}
	var archived int
	err := r.db.QueryRowContext(ctx, "SELECT archived FROM "+table+" WHERE id = ?", x.ID).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && archived != 0) {
		return fmt.Errorf("%s %d: %w", x.Kind, x.ID, model.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("find %s %d: %w", x.Kind, x.ID, err)
	}
	return nil
}

// orient returns the relation and the two refs as holder and target.
func (r *LinkRepo) orient(ctx context.Context, a, b Ref) (model.Relation, Ref, Ref, error) {
	if err := r.check(ctx, a); err != nil {
		return model.Relation{}, a, b, err
	}
	if err := r.check(ctx, b); err != nil {
		return model.Relation{}, a, b, err
	}
	rel, ok := model.RelationBetween(a.Kind, b.Kind)
	if !ok || a.Kind == b.Kind {
		return rel, a, b, fmt.Errorf("a %s cannot link to a %s: %w", a.Kind, b.Kind, model.ErrValidation)
	}
	if rel.Holder == a.Kind {
		return rel, a, b, nil
	}
	return rel, b, a, nil
}

// Link connects a and b. Linking what is already linked changes nothing. A single-valued side
// (person to organization, deal to organization, deal to person, task to deal) is replaced, and
// the old target comes back in LinkResult.Replaced.
func (r *LinkRepo) Link(ctx context.Context, a, b Ref) (LinkResult, error) {
	rel, h, t, err := r.orient(ctx, a, b)
	if err != nil {
		return LinkResult{}, err
	}
	if rel.Many {
		res, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO task_people (task_id, person_id) VALUES (?, ?)`, h.ID, t.ID)
		if err != nil {
			return LinkResult{}, fmt.Errorf("link %s to %s: %w", h, t, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return LinkResult{}, nil
		}
		return LinkResult{Changed: true}, r.touch(ctx, h)
	}
	cur, err := r.single(ctx, rel, h)
	if err != nil {
		return LinkResult{}, err
	}
	if cur != nil && *cur == t.ID {
		return LinkResult{}, nil
	}
	if err := r.setSingle(ctx, rel, h, &t.ID); err != nil {
		return LinkResult{}, err
	}
	out := LinkResult{Changed: true}
	if cur != nil {
		out.Replaced = &Ref{Kind: t.Kind, ID: *cur}
	}
	return out, nil
}

// Unlink removes the link between a and b. It is an error when they are not linked.
func (r *LinkRepo) Unlink(ctx context.Context, a, b Ref) error {
	rel, h, t, err := r.orient(ctx, a, b)
	if err != nil {
		return err
	}
	if rel.Many {
		res, err := r.db.ExecContext(ctx, `DELETE FROM task_people WHERE task_id = ? AND person_id = ?`, h.ID, t.ID)
		if err != nil {
			return fmt.Errorf("unlink %s from %s: %w", h, t, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%s is not linked to %s: %w", h, t, model.ErrNotFound)
		}
		return r.touch(ctx, h)
	}
	cur, err := r.single(ctx, rel, h)
	if err != nil {
		return err
	}
	if cur == nil || *cur != t.ID {
		return fmt.Errorf("%s is not linked to %s: %w", h, t, model.ErrNotFound)
	}
	return r.setSingle(ctx, rel, h, nil)
}

// Linked lists the ids of the otherKind objects linked to a, live ones only.
func (r *LinkRepo) Linked(ctx context.Context, a Ref, otherKind string) ([]int64, error) {
	if err := r.check(ctx, a); err != nil {
		return nil, err
	}
	rel, ok := model.RelationBetween(a.Kind, otherKind)
	if !ok || a.Kind == otherKind {
		return nil, fmt.Errorf("a %s cannot link to a %s: %w", a.Kind, otherKind, model.ErrValidation)
	}
	var q string
	switch {
	case rel.Many && rel.Holder == a.Kind:
		q = `SELECT p.id FROM task_people tp JOIN people p ON p.id = tp.person_id WHERE tp.task_id = ? AND p.archived = 0 ORDER BY p.id`
	case rel.Many:
		q = `SELECT t.id FROM task_people tp JOIN tasks t ON t.id = tp.task_id WHERE tp.person_id = ? AND t.archived = 0 ORDER BY t.id`
	case rel.Holder == a.Kind:
		q = fmt.Sprintf(`SELECT x.id FROM %s h JOIN %s x ON x.id = h.%s WHERE h.id = ? AND x.archived = 0`, tableOf[rel.Holder], tableOf[rel.Target], holderColumn(rel))
	default:
		q = fmt.Sprintf(`SELECT id FROM %s WHERE %s = ? AND archived = 0 ORDER BY id`, tableOf[rel.Holder], holderColumn(rel))
	}
	rows, err := r.db.QueryContext(ctx, q, a.ID)
	if err != nil {
		return nil, fmt.Errorf("list %ss linked to %s: %w", otherKind, a, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *LinkRepo) single(ctx context.Context, rel model.Relation, h Ref) (*int64, error) {
	var v sql.NullInt64
	q := fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", holderColumn(rel), tableOf[rel.Holder])
	if err := r.db.QueryRowContext(ctx, q, h.ID).Scan(&v); err != nil {
		return nil, fmt.Errorf("find %s: %w", h, err)
	}
	if !v.Valid {
		return nil, nil
	}
	return &v.Int64, nil
}

func (r *LinkRepo) setSingle(ctx context.Context, rel model.Relation, h Ref, to *int64) error {
	var before *model.Person
	if h.Kind == "person" {
		before, _ = r.people.FindByID(ctx, h.ID)
	}
	q := fmt.Sprintf("UPDATE %s SET %s = ?, updated_at = datetime('now') WHERE id = ?", tableOf[rel.Holder], holderColumn(rel))
	if _, err := r.db.ExecContext(ctx, q, to, h.ID); err != nil {
		return fmt.Errorf("update %s: %w", h, err)
	}
	if before != nil {
		if after, err := r.people.FindByID(ctx, h.ID); err == nil {
			return recordPersonFieldChanges(ctx, r.db, h.ID, before, after)
		}
	}
	return nil
}

func (r *LinkRepo) touch(ctx context.Context, h Ref) error {
	_, err := r.db.ExecContext(ctx, "UPDATE "+tableOf[h.Kind]+" SET updated_at = datetime('now') WHERE id = ?", h.ID)
	return err
}
