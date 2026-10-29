package tui

import (
	"hash/crc32"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/tree"
)

// The tree view module (patterns/tree-view-module). A reusable tree for any collection whose rows
// declare a hierarchy path field (model.FieldSpec.Hierarchy == model.HierarchyPath): the module turns the
// tree nodes (internal/tree) into the rows of the shared list, so selection, bulk verbs, search and the
// detail pane work as in every other list. Nothing in this file knows about tags. A collection supplies
// its nodes and a function that fills the content of a row (treeRowsOf), and registers its navigation
// with the actions below.
//
// Navigation uses arrows only, no new bare key:
//
//	up, down            move through the visible rows
//	right               expand a collapsed node, else move to its first child
//	left                collapse an open node, else move to its parent
//	enter, e            activate the node (the collection decides: the tag page for a tag)
//	home, end           first and last row
//	ctrl+up, ctrl+down  jump to the previous or next sibling group: the previous or next row at the same
//	                    level, or when there is none the node one level out (navunits.go, unitSibling)
//
// A node that has children shows ▾ when open and ▸ when collapsed. Every node starts open. The open and
// collapsed state is kept per collection for the session and survives edits, because it is keyed by path.

// treeClosedSet is the set of collapsed paths of a collection.
func (m *Model) treeClosedSet(collection string) map[string]bool {
	if m.treeClosed == nil {
		m.treeClosed = map[string]map[string]bool{}
	}
	if m.treeClosed[collection] == nil {
		m.treeClosed[collection] = map[string]bool{}
	}
	return m.treeClosed[collection]
}

// treeRowID is a stable id for a node that has no row of its own (a virtual parent), so it can be
// selected and found again after a rebuild. It is negative, so it never meets a database id.
func treeRowID(path string) int64 { return -(int64(crc32.ChecksumIEEE([]byte(path))) + 1) }

const (
	treeOpenMark   = "▾"
	treeClosedMark = "▸"
	treeLeafMark   = "·"
)

// treeRowsOf turns nodes (in tree order) into rows. Children of a collapsed node are left out, unless
// forceOpen is set (while searching, every match must be visible). mk fills the content of a row.
func (m *Model) treeRowsOf(collection string, nodes []tree.Node, forceOpen bool, mk func(n tree.Node) row) []row {
	closed := m.treeClosedSet(collection)
	if m.treeAll == nil {
		m.treeAll = map[string][]tree.Node{}
	}
	m.treeAll[collection] = nodes
	var rows []row
	skip := 0 // depth of the collapsed node whose subtree is being skipped
	for _, n := range nodes {
		if skip > 0 && n.Depth > skip {
			continue
		}
		skip = 0
		r := mk(n)
		r.node, r.depth, r.virtual, r.kids = true, n.Depth, n.Virtual, n.Children
		switch {
		case n.Children == 0:
			r.mark = treeLeafMark
		case closed[n.Path] && !forceOpen:
			r.mark, skip = treeClosedMark, n.Depth
		default:
			r.mark, r.open = treeOpenMark, true
		}
		rows = append(rows, r)
	}
	return rows
}

// treeCollapseAll collapses every node that has children. treeExpandAll opens them all.
func (m *Model) treeCollapseAll(collection string) {
	closed := m.treeClosedSet(collection)
	for _, n := range m.treeAll[collection] {
		if n.Children > 0 {
			closed[n.Path] = true
		}
	}
}

func (m *Model) treeExpandAll(collection string) {
	m.treeClosed[collection] = map[string]bool{}
}

// cursorNode is the tree row under the cursor.
func (m *Model) cursorNode() (row, bool) {
	r, ok := m.selected()
	if !ok || !r.node || r.skip {
		return row{}, false
	}
	return r, true
}

// treeExpand is right: open a collapsed node, else move to the first child.
func (m *Model) treeExpand(collection string) {
	r, ok := m.cursorNode()
	if !ok || r.kids == 0 {
		return
	}
	if !r.open {
		delete(m.treeClosedSet(collection), r.tag)
		m.report(m.rebuild(true))
		m.refreshDetail(false)
		return
	}
	m.move(1)
}

// treeCollapse is left: close an open node, else move to the parent.
func (m *Model) treeCollapse(collection string) {
	r, ok := m.cursorNode()
	if !ok {
		return
	}
	if r.kids > 0 && r.open {
		m.treeClosedSet(collection)[r.tag] = true
		m.report(m.rebuild(true))
		m.refreshDetail(false)
		return
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.rows[i].node && m.rows[i].depth == r.depth-1 {
			m.cursor = i
			m.clamp()
			m.refreshDetail(false)
			return
		}
	}
}

// treeParentExists says left has somewhere to go from the row: a parent row above it.
func (m *Model) treeParentExists(r row) bool { return r.depth > 1 }

// treeJump is ctrl+up and ctrl+down, the jump of a tree: the next row at the same level (skipping the
// subtree of the cursor row), or when the cursor row is the last of its siblings the next row one level out.
// Backwards it is the previous row at the same level, else the parent. At the ends the first or last row.
func (m *Model) treeJump(dir int) {
	r, ok := m.cursorNode()
	if !ok {
		m.move(dir * max(1, m.bodyHeight()-1))
		return
	}
	target := -1
	if dir > 0 {
		for i := m.cursor + 1; i < len(m.rows); i++ {
			if m.rows[i].node && m.rows[i].depth <= r.depth {
				target = i
				break
			}
		}
		if target < 0 {
			target = m.lastSelectable()
		}
	} else {
		for i := m.cursor - 1; i >= 0; i-- {
			if m.rows[i].node && m.rows[i].depth <= r.depth {
				target = i
				break
			}
		}
		if target < 0 {
			target = 0
		}
	}
	m.cursor = target
	m.clamp()
	m.refreshDetail(false)
}

// treeActions are the navigation actions of the tree view module. when says in which view and layout
// the tree is on, collection names the open and closed state.
func treeActions(prefix, collection string, when func(m *Model) bool) []action {
	hasKids := func(m *Model) bool {
		r, ok := m.cursorNode()
		return when(m) && ok && r.kids > 0
	}
	canLeft := func(m *Model) bool {
		r, ok := m.cursorNode()
		return when(m) && ok && ((r.kids > 0 && r.open) || m.treeParentExists(r))
	}
	return []action{
		{id: prefix + "-expand", title: "Tree: expand or go to first child", group: "Move", hint: "right", keys: []string{"right"},
			desc: "in a tree view: open the highlighted node, or when it is open move to its first child. A node without children does nothing here and right moves into the detail pane as in a list",
			when: hasKids, run: func(m *Model) tea.Cmd { m.treeExpand(collection); return nil }},
		{id: prefix + "-collapse", title: "Tree: collapse or go to parent", group: "Move", hint: "left", keys: []string{"left"},
			desc: "in a tree view: close the highlighted node, or when it is closed (or has no children) move to its parent",
			when: canLeft, run: func(m *Model) tea.Cmd { m.treeCollapse(collection); return nil }},
		{id: prefix + "-expand-all", title: "Tree: expand all", group: "Views",
			desc: "open every node of the tree", when: when,
			run: func(m *Model) tea.Cmd { m.treeExpandAll(collection); m.report(m.rebuild(true)); return nil }},
		{id: prefix + "-collapse-all", title: "Tree: collapse all", group: "Views",
			desc: "close every node of the tree, so only the top level shows", when: when,
			run: func(m *Model) tea.Cmd { m.treeCollapseAll(collection); m.report(m.rebuild(false)); return nil }},
	}
}
