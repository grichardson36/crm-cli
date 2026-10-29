// Package tree builds the tree of a collection whose rows have a hierarchy path field
// (model.FieldSpec.Hierarchy == model.HierarchyPath, patterns/tree-view-module). A row named
// work/clients/acme is a child of work/clients, which is a child of work. A parent that no row names
// is a virtual parent: it is in the tree so its children have a place, and it has no row of its own.
// The package has no database and no screen. The repositories, the CLI, MCP and the terminal UI all
// build and read the same nodes.
package tree

import (
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Split returns the segments of a path.
func Split(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, model.PathSep)
}

// Join builds a path from segments, skipping empty ones.
func Join(segs ...string) string {
	var out []string
	for _, s := range segs {
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, model.PathSep)
}

// Parent is the path of the parent, "" for a top level path.
func Parent(path string) string {
	i := strings.LastIndex(path, model.PathSep)
	if i < 0 {
		return ""
	}
	return path[:i]
}

// Name is the last segment.
func Name(path string) string {
	return path[strings.LastIndex(path, model.PathSep)+1:]
}

// Depth is the number of segments: 1 for a top level path, 0 for "".
func Depth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, model.PathSep) + 1
}

// IsUnder says whether path is a strict descendant of ancestor.
func IsUnder(path, ancestor string) bool {
	return ancestor != "" && strings.HasPrefix(path, ancestor+model.PathSep)
}

// InSubtree says whether path is root or one of its descendants.
func InSubtree(path, root string) bool { return path == root || IsUnder(path, root) }

// Rebase moves a path from the subtree of from into the place to: from itself becomes to, from/x
// becomes to/x. A path outside the subtree comes back unchanged.
func Rebase(path, from, to string) string {
	switch {
	case path == from:
		return to
	case IsUnder(path, from):
		return to + path[len(from):]
	}
	return path
}

// Item is one row of the collection: its path, an id the caller understands, how many things it has
// of its own and, optionally, the identities of those things. With Refs the count that includes the
// children counts a thing once even when several rows of the subtree have it.
type Item struct {
	Path string
	ID   int64
	Own  int
	Refs []string
	// Data is whatever the caller wants back with the node.
	Data any
}

// Node is one place of the tree.
type Node struct {
	Path  string
	Name  string
	Depth int // 1 for a top level node
	// Virtual says no row has this path: it exists only because a descendant has one.
	Virtual bool
	ID      int64
	// Own counts the things of this row. Total counts them with the whole subtree.
	Own, Total int
	// Children is the number of direct children, Descendants of all nodes below.
	Children, Descendants int
	Data                  any
}

type acc struct {
	node Node
	refs map[string]bool
	kids []string
}

// Build returns the nodes in tree order: a parent, then its subtree, siblings sorted by name without
// regard to case. Virtual parents are added. Rows with the same path are one node (their counts add).
// When the items carry Refs, Total counts the distinct refs of the subtree, else it adds the Own counts.
func Build(items []Item) []Node {
	all := map[string]*acc{}
	get := func(path string) *acc {
		a, ok := all[path]
		if !ok {
			a = &acc{node: Node{Path: path, Name: Name(path), Depth: Depth(path), Virtual: true}, refs: map[string]bool{}}
			all[path] = a
		}
		return a
	}
	useRefs := false
	for _, it := range items {
		if it.Path == "" {
			continue
		}
		a := get(it.Path)
		if a.node.Virtual {
			a.node.Virtual, a.node.ID, a.node.Data = false, it.ID, it.Data
		}
		a.node.Own += it.Own
		for _, r := range it.Refs {
			a.refs[r] = true
			useRefs = true
		}
		for p := Parent(it.Path); p != ""; p = Parent(p) {
			get(p)
		}
	}
	var roots []string
	for p := range all {
		if par := Parent(p); par == "" {
			roots = append(roots, p)
		} else {
			all[par].kids = append(all[par].kids, p)
		}
	}
	less := func(a, b string) bool {
		la, lb := strings.ToLower(Name(a)), strings.ToLower(Name(b))
		if la != lb {
			return la < lb
		}
		return a < b
	}
	sort.Slice(roots, func(i, j int) bool { return less(roots[i], roots[j]) })
	out := make([]Node, 0, len(all))
	// walk appends the subtree of path. It returns the distinct refs, the summed own counts and the
	// number of descendants.
	var walk func(path string) (map[string]bool, int, int)
	walk = func(path string) (map[string]bool, int, int) {
		a := all[path]
		sort.Slice(a.kids, func(i, j int) bool { return less(a.kids[i], a.kids[j]) })
		at := len(out)
		out = append(out, Node{})
		refs := map[string]bool{}
		for r := range a.refs {
			refs[r] = true
		}
		sum, desc := a.node.Own, 0
		for _, k := range a.kids {
			kr, ks, kd := walk(k)
			for r := range kr {
				refs[r] = true
			}
			sum += ks
			desc += 1 + kd
		}
		n := a.node
		n.Children, n.Descendants = len(a.kids), desc
		n.Total = sum
		if useRefs {
			n.Total = len(refs)
		}
		out[at] = n
		return refs, sum, desc
	}
	for _, r := range roots {
		walk(r)
	}
	return out
}
