package repo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/tree"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// Nested tags (patterns/nested-tags). The name of a tag is a path: work/clients/acme is a child of
// work/clients, which is a child of work. The hierarchy comes from the name alone (the schema flags
// the name field with hierarchy: path, model.Fields["tag"]), so there is no column, no migration and
// nothing to keep in sync. A parent that no tag is named after is virtual: it shows in the tree and
// has no row. Every operation here is a rename of a path prefix, a merge of subtrees or a delete of a
// subtree, and each returns a result that undoes and redoes the whole step.

type tagRow struct {
	ID   int64
	Name string
}

// subtreeTags lists the tags named root or under it, shallow first. An empty root lists every tag.
func (r *TagRepo) subtreeTags(ctx context.Context, q DBTX, root string) ([]tagRow, error) {
	query, args := "SELECT id, name FROM tags", []any{}
	if root != "" {
		query += " WHERE name = ? OR substr(name, 1, ?) = ?"
		args = append(args, root, utf8.RuneCountInString(root)+1, root+model.PathSep)
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list subtree of %q: %w", root, err)
	}
	defer rows.Close()
	var out []tagRow
	for rows.Next() {
		var t tagRow
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		di, dj := tree.Depth(out[i].Name), tree.Depth(out[j].Name)
		if di != dj {
			return di < dj
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// cleanTagName normalizes and checks the name of a tag (internal/validate, the one framework).
func cleanTagName(raw string) (string, error) {
	res := validate.TagName(NormalizeTagName(raw))
	if err := res.Err(); err != nil {
		return "", err
	}
	return res.Value, nil
}

// --- the tree ---

// Tree lists the tags as tree nodes in tree order (parents before children, siblings by name). Counts
// leave out archived objects. Own counts the objects of the tag itself and Total those of the tag and
// everything below it, an object counted once. With unused the tags no object carries are in it too.
func (r *TagRepo) Tree(ctx context.Context, unused bool) ([]tree.Node, error) {
	stats, err := r.Stats(ctx)
	if err != nil {
		return nil, err
	}
	if !unused {
		var keep []TagStat
		for _, s := range stats {
			if !s.Unused() {
				keep = append(keep, s)
			}
		}
		stats = keep
	}
	return r.TreeOf(ctx, stats)
}

// TreeOf builds the tree of the given tags (for a screen that filters the list first). Node.Data is the
// TagStat of a node that is a tag, nil for a virtual parent.
func (r *TagRepo) TreeOf(ctx context.Context, stats []TagStat) ([]tree.Node, error) {
	refs, err := r.liveRefs(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]tree.Item, 0, len(stats))
	for _, s := range stats {
		items = append(items, tree.Item{Path: s.Name, ID: s.ID, Own: s.Count, Refs: refs[s.ID], Data: s})
	}
	return tree.Build(items), nil
}

// liveRefs lists the objects of each tag as "kind:id", archived objects left out.
func (r *TagRepo) liveRefs(ctx context.Context) (map[int64][]string, error) {
	out := map[int64][]string{}
	for _, kind := range model.TaggableTypes {
		q := fmt.Sprintf(`SELECT g.tag_id, g.entity_id FROM taggings g JOIN %s e ON e.id = g.entity_id
			WHERE g.entity_type = ? AND g.deleted_at IS NULL AND e.archived = 0`, entityTables[kind])
		rs, err := r.db.QueryContext(ctx, q, kind)
		if err != nil {
			return nil, fmt.Errorf("list %s taggings: %w", kind, err)
		}
		for rs.Next() {
			var tagID, id int64
			if err := rs.Scan(&tagID, &id); err != nil {
				rs.Close()
				return nil, err
			}
			out[tagID] = append(out[tagID], fmt.Sprintf("%s:%d", kind, id))
		}
		if err := rs.Err(); err != nil {
			rs.Close()
			return nil, err
		}
		rs.Close()
	}
	return out, nil
}

// SubtreeStat says what a subtree holds, for the question before a delete.
type SubtreeStat struct {
	Path string `json:"path"`
	// Tags is the number of tag names in the subtree (the root counts when it is a tag of its own),
	// Taggings the live taggings on them (archived objects included) and Objects the distinct
	// objects that are not archived.
	Tags     int `json:"tags"`
	Taggings int `json:"taggings"`
	Objects  int `json:"objects"`
}

// SubtreeStat counts the subtree of a path. A path that is no tag and has no tag under it is not found.
func (r *TagRepo) SubtreeStat(ctx context.Context, path string) (SubtreeStat, error) {
	path = NormalizeTagName(path)
	st := SubtreeStat{Path: path}
	stats, err := r.Stats(ctx)
	if err != nil {
		return st, err
	}
	var in []TagStat
	for _, s := range stats {
		if tree.InSubtree(s.Name, path) {
			in = append(in, s)
			st.Tags++
			st.Taggings += s.Count + s.Archived
		}
	}
	if len(in) == 0 {
		return st, fmt.Errorf("tag %q: %w", path, model.ErrNotFound)
	}
	nodes, err := r.TreeOf(ctx, in)
	if err != nil {
		return st, err
	}
	for _, n := range nodes {
		if n.Path == path {
			st.Objects = n.Total
		}
	}
	return st, nil
}

// IsVirtual says the path is no tag but has tags under it.
func (r *TagRepo) IsVirtual(ctx context.Context, path string) (bool, error) {
	path = NormalizeTagName(path)
	set, err := r.subtreeTags(ctx, r.db, path)
	if err != nil {
		return false, err
	}
	return len(set) > 0 && set[0].Name != path, nil
}

// --- rename, merge, nest, unnest ---

// TagTreeOp is one step of a tree change: one tag renamed, or one tag merged into another.
type TagTreeOp struct {
	Kind string `json:"kind"` // rename or merge
	From string `json:"from"`
	To   string `json:"to"`
	// Records is how many records moved onto another tag by a merge.
	Records int `json:"records,omitempty"`
	merge   *TagMergeResult
}

// TagTreeResult is what a tree change did, or with DryRun what it would do. One undo step puts it all back.
type TagTreeResult struct {
	Action  string      `json:"action"` // rename, merge, nest or unnest
	DryRun  bool        `json:"dry_run"`
	Ops     []TagTreeOp `json:"ops"`
	Renamed int         `json:"renamed"` // tags that got a new name
	Merged  int         `json:"merged"`  // tags merged into a tag that existed
	Records int         `json:"records"` // records that moved onto another tag by the merges
	// Duplicates counts the taggings a merge target already had, which fold into the existing ones.
	Duplicates int `json:"duplicates"`
	moves      []tagMove
}

type tagMove struct{ from, to string }

// Label is a short description for messages and the undo history.
func (r TagTreeResult) Label() string {
	if len(r.moves) == 1 {
		mv := r.moves[0]
		switch r.Action {
		case "merge":
			return fmt.Sprintf("merge tag #%s into #%s", mv.from, mv.to)
		case "rename":
			return fmt.Sprintf("rename tag #%s to #%s", mv.from, mv.to)
		}
		return fmt.Sprintf("%s tag #%s to #%s", r.Action, mv.from, mv.to)
	}
	return fmt.Sprintf("%s %d tags", r.Action, len(r.moves))
}

type planned struct {
	from, to  string
	mergeInto string // a tag that has the new name: merge into it instead of renaming
}

// plan lists what moving the subtree of from to the place to takes, shallow first. A new name that a
// tag outside the subtree already has (in any case), or that an earlier step of the same plan takes, is
// a collision. With allowMerge a collision becomes a merge into that tag, without it the first one is a
// *TagExistsError.
func (r *TagRepo) plan(ctx context.Context, from, to string, allowMerge bool) ([]planned, error) {
	if to != "" {
		if err := validate.PathMove("tag", from, to).Err(); err != nil {
			return nil, err
		}
	}
	set, err := r.subtreeTags(ctx, r.db, from)
	if err != nil {
		return nil, err
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("tag %q: %w", from, model.ErrNotFound)
	}
	inSet := map[int64]bool{}
	for _, t := range set {
		inSet[t.ID] = true
	}
	all, err := r.subtreeTags(ctx, r.db, "")
	if err != nil {
		return nil, err
	}
	taken := map[string]string{} // lower case name -> name, of the tags outside the subtree
	for _, t := range all {
		if !inSet[t.ID] {
			taken[strings.ToLower(t.Name)] = t.Name
		}
	}
	var out []planned
	for _, t := range set {
		clean, err := cleanTagName(tree.Rebase(t.Name, from, to))
		if err != nil {
			return nil, err
		}
		p := planned{from: t.Name, to: clean}
		if other, hit := taken[strings.ToLower(clean)]; hit {
			if !allowMerge {
				return nil, &TagExistsError{Name: clean, Existing: other, From: t.Name}
			}
			p.mergeInto = other
		} else {
			if clean == t.Name {
				continue // nothing changes for this one
			}
			taken[strings.ToLower(clean)] = clean
		}
		out = append(out, p)
	}
	return out, nil
}

// RenameTree gives a tag a new name and moves everything under it along: renaming work to job turns
// work/clients into job/clients. A name that another tag has, in any case, is refused with a
// *TagExistsError (the caller offers a merge). With merge it is merged into that tag instead, children
// with the same names merging too. The result undoes and redoes the whole step.
func (r *TagRepo) RenameTree(ctx context.Context, oldName, newName string, merge bool) (*TagTreeResult, error) {
	return r.moveTrees(ctx, "rename", []tagMove{{NormalizeTagName(oldName), NormalizeTagName(newName)}}, merge, false)
}

// RenameTreeDryRun reports what RenameTree would do and writes nothing.
func (r *TagRepo) RenameTreeDryRun(ctx context.Context, oldName, newName string, merge bool) (*TagTreeResult, error) {
	return r.moveTrees(ctx, "rename", []tagMove{{NormalizeTagName(oldName), NormalizeTagName(newName)}}, merge, true)
}

// MergeTree merges the source tag into the target and its children follow: the children of the source
// become children of the target, and a child with the same name as one of the target's is merged into it.
// The target may be a virtual parent (a path other tags are under).
func (r *TagRepo) MergeTree(ctx context.Context, source, target string) (*TagTreeResult, error) {
	return r.mergeTree(ctx, source, target, false)
}

// MergeTreeDryRun reports what MergeTree would do and writes nothing.
func (r *TagRepo) MergeTreeDryRun(ctx context.Context, source, target string) (*TagTreeResult, error) {
	return r.mergeTree(ctx, source, target, true)
}

func (r *TagRepo) mergeTree(ctx context.Context, source, target string, dryRun bool) (*TagTreeResult, error) {
	source, target = NormalizeTagName(source), NormalizeTagName(target)
	if source == "" || target == "" {
		return nil, fmt.Errorf("merge needs a source and a target tag: %w", model.ErrValidation)
	}
	if source == target {
		return nil, fmt.Errorf("a tag cannot be merged into itself: %w", model.ErrValidation)
	}
	set, err := r.subtreeTags(ctx, r.db, target)
	if err != nil {
		return nil, err
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("tag %q: %w", target, model.ErrNotFound)
	}
	return r.moveTrees(ctx, "merge", []tagMove{{source, target}}, true, dryRun)
}

// Nest puts tags under a parent: nesting work under area makes area/work, and its children follow.
// The parent is a path. It needs no tag of its own, the tree shows it as a virtual parent. An empty
// parent moves the tags to the top level. A tag that is under another tag of the list follows that
// one. A name the parent already holds is refused with a *TagExistsError, or with merge merged.
func (r *TagRepo) Nest(ctx context.Context, tags []string, parent string, merge bool) (*TagTreeResult, error) {
	moves, err := nestMoves(tags, parent)
	if err != nil {
		return nil, err
	}
	return r.moveTrees(ctx, "nest", moves, merge, false)
}

// NestDryRun reports what Nest would do and writes nothing.
func (r *TagRepo) NestDryRun(ctx context.Context, tags []string, parent string, merge bool) (*TagTreeResult, error) {
	moves, err := nestMoves(tags, parent)
	if err != nil {
		return nil, err
	}
	return r.moveTrees(ctx, "nest", moves, merge, true)
}

// distinctTags normalizes the names, drops repeats and drops a tag that is under another tag of the
// list, because that one moves it along.
func distinctTags(tags []string) []string {
	var names []string
	seen := map[string]bool{}
	for _, t := range tags {
		t = NormalizeTagName(t)
		if t != "" && !seen[t] {
			seen[t] = true
			names = append(names, t)
		}
	}
	var out []string
	for _, t := range names {
		follows := false
		for _, o := range names {
			follows = follows || tree.IsUnder(t, o)
		}
		if !follows {
			out = append(out, t)
		}
	}
	return out
}

func nestMoves(tags []string, parent string) ([]tagMove, error) {
	parent = NormalizeTagName(parent)
	if parent != "" {
		if err := validate.TagName(parent).Err(); err != nil {
			return nil, err
		}
	}
	names := distinctTags(tags)
	if len(names) == 0 {
		return nil, fmt.Errorf("nest needs at least one tag: %w", model.ErrValidation)
	}
	var moves []tagMove
	for _, t := range names {
		if to := tree.Join(parent, tree.Name(t)); to != t {
			moves = append(moves, tagMove{t, to})
		}
	}
	if len(moves) == 0 {
		return nil, fmt.Errorf("the tags are already under #%s: %w", parent, model.ErrValidation)
	}
	return moves, nil
}

// Unnest moves tags up one level: a/b/c becomes a/c. A tag at the top level stays. Children follow.
func (r *TagRepo) Unnest(ctx context.Context, tags []string, merge bool) (*TagTreeResult, error) {
	moves, err := unnestMoves(tags)
	if err != nil {
		return nil, err
	}
	return r.moveTrees(ctx, "unnest", moves, merge, false)
}

// UnnestDryRun reports what Unnest would do and writes nothing.
func (r *TagRepo) UnnestDryRun(ctx context.Context, tags []string, merge bool) (*TagTreeResult, error) {
	moves, err := unnestMoves(tags)
	if err != nil {
		return nil, err
	}
	return r.moveTrees(ctx, "unnest", moves, merge, true)
}

func unnestMoves(tags []string) ([]tagMove, error) {
	var moves []tagMove
	for _, t := range distinctTags(tags) {
		if tree.Parent(t) != "" {
			moves = append(moves, tagMove{t, tree.Join(tree.Parent(tree.Parent(t)), tree.Name(t))})
		}
	}
	if len(moves) == 0 {
		return nil, fmt.Errorf("the tags are already at the top level: %w", model.ErrValidation)
	}
	return moves, nil
}

// moveTrees runs the moves one after the other. A move that fails undoes the ones before it.
func (r *TagRepo) moveTrees(ctx context.Context, action string, moves []tagMove, merge, dryRun bool) (*TagTreeResult, error) {
	res := &TagTreeResult{Action: action, DryRun: dryRun, moves: moves}
	for _, mv := range moves {
		if mv.from == "" || mv.to == "" {
			_ = r.undoOps(ctx, res)
			return nil, fmt.Errorf("a tag needs a name: %w", model.ErrValidation)
		}
		steps, err := r.plan(ctx, mv.from, mv.to, merge)
		if err != nil {
			_ = r.undoOps(ctx, res)
			return nil, err
		}
		for _, p := range steps {
			op := TagTreeOp{Kind: "rename", From: p.from, To: p.to}
			if p.mergeInto != "" {
				op = TagTreeOp{Kind: "merge", From: p.from, To: p.mergeInto}
				run := r.Merge
				if dryRun {
					run = r.MergeDryRun
				}
				m, err := run(ctx, p.from, p.mergeInto)
				switch {
				case err != nil && dryRun && errors.Is(err, model.ErrNotFound):
					// the target is a name an earlier step of this plan would create
				case err != nil:
					_ = r.undoOps(ctx, res)
					return nil, err
				default:
					op.merge, op.Records = m, m.Total
					res.Records += m.Total
					for _, n := range m.Duplicates {
						res.Duplicates += n
					}
				}
				res.Merged++
			} else {
				if !dryRun {
					if err := r.Rename(ctx, p.from, p.to); err != nil {
						_ = r.undoOps(ctx, res)
						return nil, err
					}
				}
				res.Renamed++
			}
			res.Ops = append(res.Ops, op)
		}
	}
	return res, nil
}

// undoOps reverses the steps of a result, newest first.
func (r *TagRepo) undoOps(ctx context.Context, res *TagTreeResult) error {
	if res.DryRun {
		return nil
	}
	for i := len(res.Ops) - 1; i >= 0; i-- {
		op := res.Ops[i]
		var err error
		if op.Kind == "merge" {
			err = r.UndoMerge(ctx, op.merge)
		} else {
			err = r.Rename(ctx, op.To, op.From)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// UndoTree puts everything a tree change did back.
func (r *TagRepo) UndoTree(ctx context.Context, res *TagTreeResult) error { return r.undoOps(ctx, res) }

// RedoTree repeats a tree change that UndoTree reversed.
func (r *TagRepo) RedoTree(ctx context.Context, res *TagTreeResult) error {
	again, err := r.moveTrees(ctx, res.Action, res.moves, true, false)
	if err != nil {
		return err
	}
	res.Ops, res.Renamed, res.Merged, res.Records, res.Duplicates = again.Ops, again.Renamed, again.Merged, again.Records, again.Duplicates
	return nil
}

// --- delete a subtree ---

// DeleteTagTree untags every object that carries the tag or a tag below it, in one transaction. The
// result undoes and redoes the whole step. A subtree no object carries is not found.
func (r *TagRepo) DeleteTagTree(ctx context.Context, path string) (*TagDeleteResult, error) {
	path = NormalizeTagName(path)
	set, err := r.subtreeTags(ctx, r.db, path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(set))
	for i, t := range set {
		names[i] = t.Name
	}
	return r.deleteTags(ctx, path, names)
}

// TaggedObjectsWithChildren lists the objects that carry the tag or a tag below it, an object once. Tag
// is the first of the tags (by name) the object carries in the subtree. A virtual parent has no objects
// of its own, so its page is the objects of its children.
func (r *TagRepo) TaggedObjectsWithChildren(ctx context.Context, tagName string) ([]TaggedObject, error) {
	return r.taggedObjects(ctx, NormalizeTagName(tagName), true)
}
