package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/jdanielnd/crm-cli/internal/tree"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// Nested tags in the Tags module (patterns/nested-tags). A tag named work/clients/acme is a child of
// work/clients. The Tags view has two layouts, flat (the default) and tree, picked in the ctrl+o view
// options and kept per device in config.toml ([tags] layout). The tree layout is the tree view module
// (treeview.go) with the tag nodes: a parent with no objects of its own shows as a virtual parent.
//
// Verbs on a tree node follow the universal ones: rename of a parent renames its children (a path prefix),
// d deletes the whole subtree with a typed count, m merges the children along, and the palette has
// Nest under... and Unnest for the selected tags.

const (
	tagCollection = "tag"
	layoutFlat    = "flat"
	layoutTree    = "tree"
)

// loadTagPrefs reads the per-device Tags settings from config.toml.
func (m *Model) loadTagPrefs() {
	if v := modules.Value(m.env.DataDir, "tags", "layout"); v == layoutTree || v == layoutFlat {
		m.tagLayout = v
	}
	m.tagChildren = modules.Value(m.env.DataDir, "tags", "page") == "children"
}

// saveTagPref keeps one Tags setting for this device. Without a data directory it lasts for the session.
func (m *Model) saveTagPref(key, value string) {
	if m.env.DataDir == "" {
		return
	}
	if err := modules.SetValue(m.env.DataDir, "tags", key, value); err != nil {
		m.setStatus(err.Error(), true)
	}
}

func (m *Model) setTagLayout(v string) {
	m.tagLayout = v
	m.saveTagPref("layout", v)
}

func (m *Model) setTagChildren(on bool) {
	m.tagChildren = on
	v := ""
	if on {
		v = "children"
	}
	m.saveTagPref("page", v)
}

// tagTreeOn says the Tags list shows the tree. The possible case duplicates filter is a flat list of
// groups and ignores the layout.
func (m *Model) tagTreeOn() bool {
	return m.tagLayout == layoutTree && m.tagFilter != "duplicates"
}

// whenTagTree is true on the Tags list while it shows the tree (not on a tag page, not in the bar).
func whenTagTree(m *Model) bool {
	return m.view == viewTags && m.focus == focusList && !m.onTagPage() && m.barKind == "" && m.tagTreeOn()
}

// tagCountMeta is the right hand text of a tag node: its own objects, and with children the objects
// including the children in brackets.
func tagCountMeta(n tree.Node) string {
	switch {
	case n.Virtual:
		return fmt.Sprintf("(%d)", n.Total)
	case n.Children > 0:
		return fmt.Sprintf("%d (%d)", n.Own, n.Total)
	}
	return strconv.Itoa(n.Own)
}

// tagTreeRows builds the rows of the tree layout from the tags the filter and the search kept.
func (m *Model) tagTreeRows(list []repo.TagStat) ([]row, error) {
	nodes, err := m.tagr.TreeOf(m.ctx, list)
	if err != nil {
		return nil, err
	}
	rows := m.treeRowsOf(tagCollection, nodes, strings.TrimSpace(m.tagQuery) != "", func(n tree.Node) row {
		r := row{tag: n.Path, title: cleanCell(n.Name), meta: tagCountMeta(n)}
		s, real := n.Data.(repo.TagStat)
		if !real {
			s, real = m.tagStats[n.Path] // a tag the filter left out, shown for the tags below it
		}
		if n.Virtual && !real {
			r.tagID = treeRowID(n.Path)
			r.sub = "group"
			return r
		}
		r.tagID, r.sub = s.ID, tagBreakdown(s)
		return r
	})
	return rows, nil
}

// tagNodeOf finds the node of the last tree build.
func (m *Model) tagNodeOf(path string) (tree.Node, bool) {
	for _, n := range m.treeAll[tagCollection] {
		if n.Path == path {
			return n, true
		}
	}
	return tree.Node{}, false
}

// virtualTagForm is the detail pane of a virtual parent: a path that no tag is named after.
func (m *Model) virtualTagForm(path string) []formRow {
	m.formTitle = "#" + path
	name := specRow(repo.Ref{Kind: tagKind, ID: treeRowID(path)}, tagNameSpec(), path)
	name.head = "Parent"
	rows := []formRow{name}
	if n, ok := m.tagNodeOf(path); ok {
		rows = append(rows, infoRow("objects", fmt.Sprintf("%d with the tags below", n.Total)),
			infoRow("children", strconv.Itoa(n.Children)))
	}
	rows = append(rows, infoRow("note", "no object carries this name itself. It exists because of the tags below it"))
	return rows
}

// --- actions ---

func tagTreeActions() []action {
	acts := treeActions("tag-tree", tagCollection, whenTagTree)
	acts = append(acts,
		action{id: "tag-nest", title: "Tags: nest under...", group: "Tags", write: true,
			desc: "put the selected tags (the highlighted one with nothing selected) under a parent: pick a tag or type a new path, and the tags become parent/name with their children. A name the parent holds offers a merge. One undo step. Same as crm tag nest",
			when: whenTagList, run: func(m *Model) tea.Cmd { return m.nestVerb() }},
		action{id: "tag-unnest", title: "Tags: unnest", group: "Tags", write: true,
			desc: "move the selected tags up one level, a/b/c becomes a/c, children follow. One undo step. Same as crm tag unnest",
			when: whenTagList, run: func(m *Model) tea.Cmd { return m.unnestVerb() }},
		action{id: "tag-layout", title: "Tags: tree or flat layout", group: "Tags",
			desc: "switch the Tags list between a flat list and a tree where work/clients is under work. Remembered on this device (also in ctrl+o)",
			when: func(m *Model) bool { return m.view == viewTags && !m.onTagPage() && m.barKind == "" },
			run: func(m *Model) tea.Cmd {
				next := layoutTree
				if m.tagLayout == layoutTree {
					next = layoutFlat
				}
				m.setTagLayout(next)
				m.cursor, m.offset = 0, 0
				m.report(m.rebuild(false))
				m.setStatus("tags layout: "+next, false)
				return nil
			}},
		action{id: "tag-children", title: "Tags: include children on tag pages", group: "Tags",
			desc: "a tag page lists the objects of the tags below it too (work lists work/clients/acme objects). Toggles, remembered on this device (also in ctrl+o)",
			when: func(m *Model) bool { return m.view == viewTags && m.barKind == "" },
			run: func(m *Model) tea.Cmd {
				m.setTagChildren(!m.tagChildren)
				m.report(m.rebuild(true))
				m.setStatus("tag page includes children: "+onOff(m.tagChildren), false)
				return nil
			}},
	)
	return acts
}

// tagRoots are the tags a tree verb acts on: the selected ones, else the highlighted one, without a tag
// whose parent is selected too (that one moves along).
func (m *Model) tagRoots() []string {
	var roots []string
	names := m.bulkTagNames()
	for _, n := range names {
		under := false
		for _, o := range names {
			under = under || tree.IsUnder(n, o)
		}
		if !under {
			roots = append(roots, n)
		}
	}
	return roots
}

// nestVerb is Nest under...: pick or create the parent, then move the selected tags below it.
func (m *Model) nestVerb() tea.Cmd {
	tags := m.tagRoots()
	if len(tags) == 0 {
		return nil
	}
	stats, err := m.tagr.Stats(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	nodes, err := m.tagr.TreeOf(m.ctx, stats)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	items := []overlayItem{{id: "tag:", label: "(top level)", desc: "no parent", value: ""}}
	for _, n := range nodes {
		inside := false
		for _, t := range tags {
			inside = inside || tree.InSubtree(n.Path, t)
		}
		if inside {
			continue // a tag cannot go under itself or its own children
		}
		desc := ""
		if n.Virtual {
			desc = "parent"
		}
		items = append(items, overlayItem{id: "tag:" + n.Path, label: "#" + n.Path, desc: desc, value: n.Path})
	}
	title := "Nest #" + tags[0] + " under"
	if len(tags) > 1 {
		title = fmt.Sprintf("Nest %d tags under", len(tags))
	}
	m.openPicker(title, items, func(m *Model, it overlayItem) { m.runNest(tags, it.value) })
	m.ov.create = func(q string) (overlayItem, bool) {
		q = strings.TrimSpace(strings.TrimPrefix(q, "#"))
		if q == "" || !validate.TagName(q).OK() {
			return overlayItem{}, false
		}
		for _, t := range tags {
			if tree.InSubtree(q, t) {
				return overlayItem{}, false
			}
		}
		return overlayItem{id: "create", label: fmt.Sprintf("Create #%s", q), desc: "a new parent, the tags move under it", value: q}, true
	}
	m.ov.refilter()
	return nil
}

// runNest nests the tags, or when a name is taken asks once whether to merge into it.
func (m *Model) runNest(tags []string, parent string) {
	m.runTreeChange("nest", func(merge bool) (*repo.TagTreeResult, error) { return m.tagr.Nest(m.ctx, tags, parent, merge) },
		func() (*repo.TagTreeResult, error) { return m.tagr.NestDryRun(m.ctx, tags, parent, true) })
}

// unnestVerb is Unnest: the selected tags move up one level.
func (m *Model) unnestVerb() tea.Cmd {
	tags := m.tagRoots()
	if len(tags) == 0 {
		return nil
	}
	m.runTreeChange("unnest", func(merge bool) (*repo.TagTreeResult, error) { return m.tagr.Unnest(m.ctx, tags, merge) },
		func() (*repo.TagTreeResult, error) { return m.tagr.UnnestDryRun(m.ctx, tags, true) })
	return nil
}

// runTreeChange runs a tree change as one undo step. A name that is taken asks "merge into it?" with the
// counts of the dry run, and the yes runs the change with merging.
func (m *Model) runTreeChange(what string, run func(merge bool) (*repo.TagTreeResult, error), dry func() (*repo.TagTreeResult, error)) {
	res, err := run(false)
	var exists *repo.TagExistsError
	if errors.As(err, &exists) {
		preview, derr := dry()
		if derr != nil {
			m.setStatus(derr.Error(), true)
			return
		}
		m.mode = modeConfirm
		m.confirm = &confirmState{
			prompt: fmt.Sprintf("tag #%s exists. %s and merge into it (%d renamed, %d merged, %d records move)? y/n", exists.Existing, what, preview.Renamed, preview.Merged, preview.Records),
			yes: func() {
				res, err := run(true)
				m.finishTreeChange(what, res, err)
			},
		}
		return
	}
	m.finishTreeChange(what, res, err)
}

// finishTreeChange records the undo step of a tree change and reports it.
func (m *Model) finishTreeChange(what string, res *repo.TagTreeResult, err error) {
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.record(res.Label(),
		func() error { return m.tagr.UndoTree(m.ctx, res) },
		func() error { return m.tagr.RedoTree(m.ctx, res) })
	m.marked = map[int64]bool{}
	m.afterWrite()
	msg := fmt.Sprintf("%s: %d tags renamed, %d merged (ctrl+z undoes)", what, res.Renamed, res.Merged)
	m.setStatus(msg, false)
	m.pendingCmd = m.showToast(strings.Replace(msg, " (ctrl+z undoes)", " · ctrl+z undoes", 1), false)
}
