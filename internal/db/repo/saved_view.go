package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// SavedView is a named query over one object kind (patterns/saved-views). The query is the neutral
// declarative one of internal/query, stored as JSON.
type SavedView struct {
	ID         int64       `json:"id"`
	UUID       string      `json:"uuid"`
	Name       string      `json:"name"`
	Kind       string      `json:"kind"`
	Query      query.Query `json:"query"`
	Position   int         `json:"position"`
	ShowInTabs bool        `json:"show_in_tabs"`
	Example    bool        `json:"example,omitempty"`
	CreatedAt  string      `json:"created_at"`
	UpdatedAt  string      `json:"updated_at"`
	Deleted    bool        `json:"deleted,omitempty"`
}

// SavedViewInput creates a saved view.
type SavedViewInput struct {
	Name       string
	Kind       string
	Query      query.Query
	ShowInTabs bool
}

// SavedViewPatch changes a saved view. A nil field is left alone.
type SavedViewPatch struct {
	Name       *string
	Kind       *string
	Query      *query.Query
	ShowInTabs *bool
}

// SavedViewRepo stores saved views in the saved_view table. Delete is soft and Restore brings a view
// back, like every other object (patterns/soft-delete-restorable).
type SavedViewRepo struct{ db *sql.DB }

// NewSavedViewRepo creates a SavedViewRepo.
func NewSavedViewRepo(db *sql.DB) *SavedViewRepo { return &SavedViewRepo{db: db} }

const savedViewCols = "id, uuid, name, kind, query, position, show_in_tabs, example, created_at, updated_at, deleted_at IS NOT NULL"

func scanSavedView(s rowScanner) (*SavedView, error) {
	var v SavedView
	var q string
	var tabs, ex int
	if err := s.Scan(&v.ID, &v.UUID, &v.Name, &v.Kind, &q, &v.Position, &tabs, &ex, &v.CreatedAt, &v.UpdatedAt, &v.Deleted); err != nil {
		return nil, err
	}
	v.ShowInTabs, v.Example = tabs == 1, ex == 1
	parsed, err := query.Parse(q)
	if err != nil {
		return nil, fmt.Errorf("saved view %q: %w", v.Name, err)
	}
	v.Query = parsed
	return &v, nil
}

// Env is the environment a saved view runs in: now, and the live deal stages in order.
func (r *SavedViewRepo) Env(ctx context.Context, now time.Time) query.Env {
	vr := NewVocabRepo(r.db)
	return query.Env{Now: now, Choices: func(vocab string) []string {
		names, err := vr.Names(ctx, vocab)
		if err != nil {
			return nil
		}
		return names
	}}
}

func (r *SavedViewRepo) nameTaken(ctx context.Context, name string, exceptID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM saved_view WHERE name = ? COLLATE NOCASE AND deleted_at IS NULL AND id <> ?", name, exceptID).Scan(&n)
	return n > 0, err
}

// check validates the name, the kind and the query, and settles the relation values to uuids.
func (r *SavedViewRepo) check(ctx context.Context, name, kind string, q *query.Query, now time.Time) (string, error) {
	n, err := query.ValidName(name)
	if err != nil {
		return "", err
	}
	if err := r.Canonicalize(ctx, kind, q); err != nil {
		return "", err
	}
	if err := query.Validate(kind, *q, r.Env(ctx, now)); err != nil {
		return "", err
	}
	return n, nil
}

// Create stores a new saved view at the end of the list.
func (r *SavedViewRepo) Create(ctx context.Context, in SavedViewInput) (*SavedView, error) {
	q := in.Query.Clone()
	name, err := r.check(ctx, in.Name, in.Kind, &q, time.Now())
	if err != nil {
		return nil, err
	}
	if taken, err := r.nameTaken(ctx, name, 0); err != nil {
		return nil, fmt.Errorf("create saved view: %w", err)
	} else if taken {
		return nil, fmt.Errorf("a saved view named %q already exists: %w", name, model.ErrConflict)
	}
	tabs := 0
	if in.ShowInTabs {
		tabs = 1
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO saved_view (uuid, name, kind, query, position, show_in_tabs)
		VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(position) FROM saved_view WHERE deleted_at IS NULL), 0) + 1, ?)`,
		uuid.New().String(), name, in.Kind, q.JSON(), tabs)
	if err != nil {
		return nil, fmt.Errorf("create saved view: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.FindByID(ctx, id)
}

// FindByID finds a view by id, deleted or not.
func (r *SavedViewRepo) FindByID(ctx context.Context, id int64) (*SavedView, error) {
	v, err := scanSavedView(r.db.QueryRowContext(ctx, "SELECT "+savedViewCols+" FROM saved_view WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("saved view %d: %w", id, model.ErrNotFound)
	}
	return v, err
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Find finds a view by id, uuid or name. A live view wins over a deleted one of the same name.
func (r *SavedViewRepo) Find(ctx context.Context, ref string) (*SavedView, error) {
	ref = strings.TrimSpace(ref)
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return r.FindByID(ctx, id)
	}
	var row *sql.Row
	if uuidRe.MatchString(ref) {
		row = r.db.QueryRowContext(ctx, "SELECT "+savedViewCols+" FROM saved_view WHERE uuid = ? COLLATE NOCASE", ref)
	} else {
		row = r.db.QueryRowContext(ctx, "SELECT "+savedViewCols+" FROM saved_view WHERE name = ? COLLATE NOCASE ORDER BY deleted_at IS NOT NULL, id DESC LIMIT 1", ref)
	}
	v, err := scanSavedView(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("saved view %q: %w", ref, model.ErrNotFound)
	}
	return v, err
}

// List lists the live views in order. With deleted set it lists the deleted ones instead.
func (r *SavedViewRepo) List(ctx context.Context, deleted bool) ([]SavedView, error) {
	cond := "deleted_at IS NULL"
	if deleted {
		cond = "deleted_at IS NOT NULL"
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+savedViewCols+" FROM saved_view WHERE "+cond+" ORDER BY position, id")
	if err != nil {
		return nil, fmt.Errorf("list saved views: %w", err)
	}
	defer rows.Close()
	var out []SavedView
	for rows.Next() {
		v, err := scanSavedView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// Update changes a live saved view.
func (r *SavedViewRepo) Update(ctx context.Context, id int64, p SavedViewPatch) (*SavedView, error) {
	cur, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.Deleted {
		return nil, fmt.Errorf("saved view %q is deleted, restore it first: %w", cur.Name, model.ErrConflict)
	}
	name, kind, q, tabs := cur.Name, cur.Kind, cur.Query.Clone(), cur.ShowInTabs
	if p.Name != nil {
		name = *p.Name
	}
	if p.Kind != nil {
		kind = *p.Kind
	}
	if p.Query != nil {
		q = p.Query.Clone()
	}
	if p.ShowInTabs != nil {
		tabs = *p.ShowInTabs
	}
	name, err = r.check(ctx, name, kind, &q, time.Now())
	if err != nil {
		return nil, err
	}
	if taken, err := r.nameTaken(ctx, name, id); err != nil {
		return nil, fmt.Errorf("update saved view: %w", err)
	} else if taken {
		return nil, fmt.Errorf("a saved view named %q already exists: %w", name, model.ErrConflict)
	}
	t := 0
	if tabs {
		t = 1
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE saved_view SET name = ?, kind = ?, query = ?, show_in_tabs = ?, updated_at = datetime('now') WHERE id = ?`,
		name, kind, q.JSON(), t, id); err != nil {
		return nil, fmt.Errorf("update saved view: %w", err)
	}
	return r.FindByID(ctx, id)
}

// SetTabs shows or hides the view in the tabs and number keys.
func (r *SavedViewRepo) SetTabs(ctx context.Context, id int64, on bool) (*SavedView, error) {
	return r.Update(ctx, id, SavedViewPatch{ShowInTabs: &on})
}

// Delete soft deletes a live view.
func (r *SavedViewRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE saved_view SET deleted_at = datetime('now'), updated_at = datetime('now') WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("delete saved view: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := r.FindByID(ctx, id); err != nil {
			return err
		}
		return fmt.Errorf("saved view %d is already deleted: %w", id, model.ErrConflict)
	}
	return nil
}

// Restore brings a deleted view back. It is refused when a live view has taken the name.
func (r *SavedViewRepo) Restore(ctx context.Context, id int64) (*SavedView, error) {
	v, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !v.Deleted {
		return nil, fmt.Errorf("saved view %q is not deleted: %w", v.Name, model.ErrConflict)
	}
	if taken, err := r.nameTaken(ctx, v.Name, id); err != nil {
		return nil, err
	} else if taken {
		return nil, fmt.Errorf("a saved view named %q exists now, rename one first: %w", v.Name, model.ErrConflict)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE saved_view SET deleted_at = NULL, updated_at = datetime('now') WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("restore saved view: %w", err)
	}
	return r.FindByID(ctx, id)
}

// Move shifts a live view one place up (-1) or down (1) among the live views. It reports whether it moved.
func (r *SavedViewRepo) Move(ctx context.Context, id int64, delta int) (bool, error) {
	list, err := r.List(ctx, false)
	if err != nil {
		return false, err
	}
	idx := -1
	for i, v := range list {
		if v.ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return false, fmt.Errorf("saved view %d: %w", id, model.ErrNotFound)
	}
	to := idx + delta
	if to < 0 || to >= len(list) {
		return false, nil
	}
	list[idx], list[to] = list[to], list[idx]
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	for i, v := range list {
		if _, err := tx.ExecContext(ctx, `UPDATE saved_view SET position = ?, updated_at = CASE WHEN position <> ? THEN datetime('now') ELSE updated_at END WHERE id = ?`, i+1, i+1, v.ID); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// Run runs the saved view as it is stored.
func (r *SavedViewRepo) Run(ctx context.Context, v *SavedView, now time.Time) ([]query.Row, error) {
	return r.RunQuery(ctx, v.Kind, v.Query, now)
}

// RunQuery runs any query of a kind, for a session that tweaked the saved one without saving.
func (r *SavedViewRepo) RunQuery(ctx context.Context, kind string, q query.Query, now time.Time) ([]query.Row, error) {
	return query.Run(ctx, r.db, kind, q, r.Env(ctx, now))
}

// farLookup says how a relation value typed as a name or an id finds its row.
var farLookup = map[string]struct{ label, noun string }{
	"people":        {"TRIM(first_name || ' ' || COALESCE(last_name, ''))", "person"},
	"organizations": {"name", "organization"},
	"deals":         {"title", "deal"},
}

// Canonicalize rewrites the values of relation conditions to uuids, the one form that is the same on
// every device. A value that is already a uuid stays, a number is an id, any other text is a name that
// must match exactly one live row. Tag values stay names. The query is changed in place.
func (r *SavedViewRepo) Canonicalize(ctx context.Context, kind string, q *query.Query) error {
	for i := range q.Filter {
		c := &q.Filter[i]
		f, ok := query.FieldOf(kind, c.Field)
		if !ok || f.Class != query.ClassRelation || query.NeedsNoValue(c.Op) {
			continue
		}
		lk, ok := farLookup[f.FarTable()]
		if !ok {
			continue
		}
		for j, v := range c.Values {
			v = strings.TrimSpace(v)
			if uuidRe.MatchString(v) {
				c.Values[j] = strings.ToLower(v)
				continue
			}
			var rows *sql.Rows
			var err error
			if id, perr := strconv.ParseInt(v, 10, 64); perr == nil {
				rows, err = r.db.QueryContext(ctx, "SELECT uuid FROM "+f.FarTable()+" WHERE id = ? AND archived = 0", id)
			} else {
				rows, err = r.db.QueryContext(ctx, "SELECT uuid FROM "+f.FarTable()+" WHERE "+lk.label+" = ? COLLATE NOCASE AND archived = 0", v)
			}
			if err != nil {
				return fmt.Errorf("find %s %q: %w", lk.noun, v, err)
			}
			var found []string
			for rows.Next() {
				var u string
				if err := rows.Scan(&u); err != nil {
					rows.Close()
					return err
				}
				found = append(found, u)
			}
			rows.Close()
			switch len(found) {
			case 1:
				c.Values[j] = found[0]
			case 0:
				return validate.Fail(validate.Issue{Code: validate.CodeQueryValue, Field: c.Field, Expected: "an existing " + lk.noun + " (a name, an id or a uuid)", Received: v, Fix: "check the name or create the " + lk.noun + " first"})
			default:
				return validate.Fail(validate.Issue{Code: validate.CodeQueryValue, Field: c.Field, Expected: "one " + lk.noun + ", " + strconv.Itoa(len(found)) + " have that name", Received: v, Fix: "use the id or the uuid"})
			}
		}
	}
	return nil
}

// RelationLabels maps the uuids of a relation field's far table to the names to show. A uuid with no
// live row is left out.
func (r *SavedViewRepo) RelationLabels(ctx context.Context, farTable string, uuids []string) map[string]string {
	out := map[string]string{}
	lk, ok := farLookup[farTable]
	if !ok || len(uuids) == 0 {
		return out
	}
	args := make([]any, len(uuids))
	for i, u := range uuids {
		args[i] = u
	}
	rows, err := r.db.QueryContext(ctx, "SELECT uuid, "+lk.label+" FROM "+farTable+" WHERE uuid IN ("+strings.TrimSuffix(strings.Repeat("?,", len(uuids)), ",")+")", args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var u, l string
		if rows.Scan(&u, &l) == nil {
			out[u] = l
		}
	}
	return out
}

// RelationCandidates lists live rows of a far table for the typeahead of a relation value: uuid and name.
func (r *SavedViewRepo) RelationCandidates(ctx context.Context, farTable string) ([][2]string, error) {
	lk, ok := farLookup[farTable]
	if !ok {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, "SELECT uuid, "+lk.label+" FROM "+farTable+" WHERE archived = 0 ORDER BY "+lk.label+" COLLATE NOCASE LIMIT 2000")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var p [2]string
		if err := rows.Scan(&p[0], &p[1]); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
