package repo

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Tag management for the Tags module (patterns/features-tags-module): counts per object kind, the
// objects that carry a tag, tags that differ only by case or separators, and tags no object carries.
// The TUI, the CLI (crm tag unused, crm tag duplicates) and MCP all call these functions.

// TagStat is a tag with how many objects carry it. Count and ByKind count live taggings of objects that
// are not archived. Archived counts the live taggings on archived objects: a tag with neither is unused.
type TagStat struct {
	ID       int64          `json:"id"`
	Name     string         `json:"name"`
	Count    int            `json:"count"`
	ByKind   map[string]int `json:"by_kind"`
	Archived int            `json:"archived"`
}

// Unused says that no object carries the tag, archived ones included.
func (t TagStat) Unused() bool { return t.Count == 0 && t.Archived == 0 }

// Stats lists every tag, unused ones too, by name (case-insensitive), with its counts.
func (r *TagRepo) Stats(ctx context.Context) ([]TagStat, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name FROM tags")
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	var out []TagStat
	at := map[int64]int{}
	for rows.Next() {
		var t TagStat
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		t.ByKind = map[string]int{}
		at[t.ID] = len(out)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, kind := range model.TaggableTypes {
		table := entityTables[kind]
		q := fmt.Sprintf(`SELECT g.tag_id, e.archived, COUNT(*) FROM taggings g JOIN %s e ON e.id = g.entity_id
			WHERE g.entity_type = ? AND g.deleted_at IS NULL GROUP BY g.tag_id, e.archived`, table)
		rs, err := r.db.QueryContext(ctx, q, kind)
		if err != nil {
			return nil, fmt.Errorf("count %s taggings: %w", kind, err)
		}
		for rs.Next() {
			var tagID int64
			var archived bool
			var n int
			if err := rs.Scan(&tagID, &archived, &n); err != nil {
				rs.Close()
				return nil, err
			}
			i, ok := at[tagID]
			if !ok {
				continue
			}
			if archived {
				out[i].Archived += n
			} else {
				out[i].Count += n
				out[i].ByKind[kind] += n
			}
		}
		if err := rs.Err(); err != nil {
			rs.Close()
			return nil, err
		}
		rs.Close()
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Unused lists the tags no object carries.
func (r *TagRepo) Unused(ctx context.Context) ([]TagStat, error) {
	all, err := r.Stats(ctx)
	if err != nil {
		return nil, err
	}
	var out []TagStat
	for _, t := range all {
		if t.Unused() {
			out = append(out, t)
		}
	}
	return out, nil
}

// TaggedObject is an object that carries a tag, with the words a list shows for it.
type TaggedObject struct {
	Kind   string `json:"kind"`
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

var taggedSQL = map[string]string{
	"person":       `SELECT e.id, trim(e.first_name || ' ' || COALESCE(e.last_name, '')), COALESCE(e.company, COALESCE(e.title, '')) FROM people e`,
	"organization": `SELECT e.id, e.name, COALESCE(e.industry, '') FROM organizations e`,
	"deal":         `SELECT e.id, e.title, e.stage FROM deals e`,
	"task":         `SELECT e.id, e.title, COALESCE(e.due_at, '') FROM tasks e`,
	"interaction":  `SELECT e.id, COALESCE(NULLIF(e.subject, ''), substr(COALESCE(e.content, ''), 1, 80), e.type), e.type FROM interactions e`,
}

// TaggedObjects lists the objects that carry the tag, grouped by kind in the order of model.TaggableTypes
// and by name inside a kind. Archived objects are left out.
func (r *TagRepo) TaggedObjects(ctx context.Context, tagName string) ([]TaggedObject, error) {
	tagName = NormalizeTagName(tagName)
	id, _, err := r.idByName(ctx, r.db, tagName)
	if err != nil {
		return nil, err
	}
	var out []TaggedObject
	for _, kind := range model.TaggableTypes {
		q := taggedSQL[kind] + ` JOIN taggings g ON g.entity_type = ? AND g.entity_id = e.id
			WHERE g.tag_id = ? AND g.deleted_at IS NULL AND e.archived = 0`
		rs, err := r.db.QueryContext(ctx, q, kind, id)
		if err != nil {
			return nil, fmt.Errorf("list tagged %s: %w", kind, err)
		}
		var part []TaggedObject
		for rs.Next() {
			o := TaggedObject{Kind: kind}
			if err := rs.Scan(&o.ID, &o.Name, &o.Detail); err != nil {
				rs.Close()
				return nil, err
			}
			part = append(part, o)
		}
		if err := rs.Err(); err != nil {
			rs.Close()
			return nil, err
		}
		rs.Close()
		sort.SliceStable(part, func(i, j int) bool {
			a, b := strings.ToLower(part[i].Name), strings.ToLower(part[j].Name)
			if a != b {
				return a < b
			}
			return part[i].ID < part[j].ID
		})
		out = append(out, part...)
	}
	return out, nil
}

// DupKey is what tags that differ only by case or separators share: lower case, with spaces, hyphens and
// underscores removed. A slash is not a separator here, it is how a nested tag is written (parent/child).
func DupKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(NormalizeTagName(name)) {
		switch r {
		case ' ', '\t', '-', '_':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TagDupGroup is a set of tags that differ only by case or separators. Keep is the most used spelling
// (ties go to the oldest tag), the one the others merge into.
type TagDupGroup struct {
	Key  string    `json:"key"`
	Keep string    `json:"keep"`
	Tags []TagStat `json:"tags"` // Keep first, then the others by use
}

// Duplicates finds the groups of tags that differ only by case or separators, by key.
func (r *TagRepo) Duplicates(ctx context.Context) ([]TagDupGroup, error) {
	all, err := r.Stats(ctx)
	if err != nil {
		return nil, err
	}
	by := map[string][]TagStat{}
	var keys []string
	for _, t := range all {
		k := DupKey(t.Name)
		if k == "" {
			continue
		}
		if _, seen := by[k]; !seen {
			keys = append(keys, k)
		}
		by[k] = append(by[k], t)
	}
	sort.Strings(keys)
	var out []TagDupGroup
	for _, k := range keys {
		g := by[k]
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(i, j int) bool {
			if g[i].Count+g[i].Archived != g[j].Count+g[j].Archived {
				return g[i].Count+g[i].Archived > g[j].Count+g[j].Archived
			}
			return g[i].ID < g[j].ID
		})
		out = append(out, TagDupGroup{Key: k, Keep: g[0].Name, Tags: g})
	}
	return out, nil
}

// TagDupMergeResult is what merging case duplicates did, or with DryRun what it would do. Merges holds
// one result per tag merged away, in the order they ran.
type TagDupMergeResult struct {
	DryRun bool              `json:"dry_run"`
	Groups int               `json:"groups"`
	Merged int               `json:"merged"` // tags merged away
	Total  int               `json:"total"`  // records that now carry a tag because of the merges
	Merges []*TagMergeResult `json:"merges"`
}

// Label is a short description for messages and the undo history.
func (r TagDupMergeResult) Label() string {
	return fmt.Sprintf("merge %d duplicate tags", r.Merged)
}

// MergeDuplicates merges every tag of a duplicate group into the most used spelling of its group. With
// only, just the groups that contain one of those names. DryRun writes nothing.
func (r *TagRepo) MergeDuplicates(ctx context.Context, dryRun bool, only ...string) (*TagDupMergeResult, error) {
	groups, err := r.Duplicates(ctx)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range only {
		want[NormalizeTagName(n)] = true
	}
	res := &TagDupMergeResult{DryRun: dryRun}
	for _, g := range groups {
		if len(want) > 0 {
			hit := false
			for _, t := range g.Tags {
				hit = hit || want[t.Name]
			}
			if !hit {
				continue
			}
		}
		res.Groups++
		for _, t := range g.Tags[1:] {
			m, err := r.merge(ctx, t.Name, g.Keep, dryRun)
			if err != nil {
				return res, err
			}
			res.Merges = append(res.Merges, m)
			res.Merged++
			res.Total += m.Total
		}
	}
	return res, nil
}

// UndoDuplicates undoes the merges of a MergeDuplicates result, newest first.
func (r *TagRepo) UndoDuplicates(ctx context.Context, res *TagDupMergeResult) error {
	for i := len(res.Merges) - 1; i >= 0; i-- {
		if err := r.UndoMerge(ctx, res.Merges[i]); err != nil {
			return err
		}
	}
	return nil
}

// RedoDuplicates repeats the merges of a MergeDuplicates result.
func (r *TagRepo) RedoDuplicates(ctx context.Context, res *TagDupMergeResult) error {
	for _, m := range res.Merges {
		if err := r.RedoMerge(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

type deadTagging struct {
	id                              int64
	uuid, entityType                string
	entityID                        int64
	createdAt, updatedAt, deletedAt string
}

type unusedRow struct {
	id                    int64
	name, uuid, createdAt string
	taggings              []deadTagging
}

// TagUnusedResult is what deleting unused tags did, or with DryRun what it would do.
type TagUnusedResult struct {
	DryRun bool     `json:"dry_run"`
	Count  int      `json:"count"`
	Tags   []string `json:"tags"`
	rows   []unusedRow
}

// Label is a short description for messages and the undo history.
func (r TagUnusedResult) Label() string {
	if len(r.Tags) == 1 {
		return "delete unused tag #" + r.Tags[0]
	}
	return fmt.Sprintf("delete %d unused tags", r.Count)
}

// DeleteUnused removes the tags no object carries. With names, only those (a name that is not an unused
// tag is not found). The tag rows go with a tombstone, and so do the soft deleted taggings they still own.
// The result keeps what was removed so one undo puts it all back.
func (r *TagRepo) DeleteUnused(ctx context.Context, dryRun bool, names ...string) (*TagUnusedResult, error) {
	unused, err := r.Unused(ctx)
	if err != nil {
		return nil, err
	}
	pick := unused
	if len(names) > 0 {
		have := map[string]TagStat{}
		for _, t := range unused {
			have[t.Name] = t
		}
		pick = nil
		for _, n := range names {
			n = NormalizeTagName(n)
			t, ok := have[n]
			if !ok {
				return nil, fmt.Errorf("unused tag %q: %w", n, model.ErrNotFound)
			}
			pick = append(pick, t)
		}
	}
	res := &TagUnusedResult{DryRun: dryRun}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	device, _ := DeviceID(ctx, tx)
	for _, t := range pick {
		row := unusedRow{id: t.ID, name: t.Name}
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(uuid, ''), COALESCE(created_at, datetime('now')) FROM tags WHERE id = ?", t.ID).Scan(&row.uuid, &row.createdAt); err != nil {
			return nil, err
		}
		rs, err := tx.QueryContext(ctx, "SELECT id, uuid, entity_type, entity_id, created_at, updated_at, COALESCE(deleted_at, '') FROM taggings WHERE tag_id = ?", t.ID)
		if err != nil {
			return nil, err
		}
		for rs.Next() {
			var d deadTagging
			if err := rs.Scan(&d.id, &d.uuid, &d.entityType, &d.entityID, &d.createdAt, &d.updatedAt, &d.deletedAt); err != nil {
				rs.Close()
				return nil, err
			}
			row.taggings = append(row.taggings, d)
		}
		rs.Close()
		res.rows = append(res.rows, row)
		res.Tags = append(res.Tags, t.Name)
		if dryRun {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO tombstones (table_name, uuid, device_id) SELECT 'taggings', uuid, ? FROM taggings WHERE tag_id = ?", device, t.ID); err != nil {
			return nil, fmt.Errorf("tombstone taggings: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM taggings WHERE tag_id = ?", t.ID); err != nil {
			return nil, fmt.Errorf("drop taggings: %w", err)
		}
		if row.uuid != "" {
			if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO tombstones (table_name, uuid, device_id) VALUES ('tags', ?, ?)", row.uuid, device); err != nil {
				return nil, fmt.Errorf("tombstone tag: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM tags WHERE id = ?", t.ID); err != nil {
			return nil, fmt.Errorf("remove tag: %w", err)
		}
	}
	res.Count = len(res.rows)
	if dryRun {
		return res, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}

// UndoDeleteUnused brings the removed tags back with their ids and uuids, and the soft deleted taggings
// they owned. A tag whose name was taken since is a conflict.
func (r *TagRepo) UndoDeleteUnused(ctx context.Context, res *TagUnusedResult) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, row := range res.rows {
		var taken int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tags WHERE name = ? OR id = ?", row.name, row.id).Scan(&taken); err != nil {
			return err
		}
		if taken > 0 {
			return fmt.Errorf("tag %q exists again, cannot undo: %w", row.name, model.ErrConflict)
		}
		u := row.uuid
		if u == "" {
			u = uuid.New().String()
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO tags (id, name, uuid, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'))", row.id, row.name, u, row.createdAt); err != nil {
			return fmt.Errorf("restore tag: %w", err)
		}
		for _, d := range row.taggings {
			var del any
			if d.deletedAt != "" {
				del = d.deletedAt
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO taggings (id, uuid, tag_id, entity_type, entity_id, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
				d.id, d.uuid, row.id, d.entityType, d.entityID, d.createdAt, d.updatedAt, del); err != nil {
				return fmt.Errorf("restore tagging: %w", err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM tombstones WHERE table_name = 'taggings' AND uuid = ?", d.uuid); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM tombstones WHERE table_name = 'tags' AND uuid = ?", u); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RedoDeleteUnused removes the tags again.
func (r *TagRepo) RedoDeleteUnused(ctx context.Context, res *TagUnusedResult) error {
	_, err := r.DeleteUnused(ctx, false, res.Tags...)
	return err
}
