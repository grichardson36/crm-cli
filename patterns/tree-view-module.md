---
type: pattern
scale: module
status: decided by owner 2026-10-29 (Q68, Q72, Q73), built on branch feat/nested-tags
---
# Tree view module

**Therefore.** One reusable tree in the TUI renders any collection whose rows declare a hierarchy path field. It is not tag specific. The Tags module is its first user ([[nested-tags]]). It is the "tree / outline" row of [[research-view-module-standard]]: rows with a parent, here a parent derived from the path.

**Because.** [[one-abstraction-many-contexts]]: a second collection with path-like names (folders of notes, organization units, deal categories) should get expand and collapse, counts, search, selection and every verb by declaring a field, with no new UI code.

## How a collection declares a hierarchy

1. In `model.Fields` give the name field `Hierarchy: model.HierarchyPath`. `model.HierarchyOf(kind)` finds it. `internal/validate.Path(kind, raw)` then checks every value with the path codes of [[nested-tags]] (empty segment, edge slash, depth, length), because `validate.Field` applies the rule to any field with the property.
2. The repository supplies `tree.Item{Path, ID, Own, Refs}` rows. `tree.Build` (`internal/tree`) returns the nodes in tree order with virtual parents, `Own`, `Total` (distinct refs of the subtree, or the sum without refs), `Children`, `Descendants`. The package has no database and no screen, so the CLI, MCP and TUI share it.
3. The view builds rows with `m.treeRowsOf(collection, nodes, forceOpen, mk)` (`internal/tui/treeview.go`). `mk` fills the content of a row. The module adds depth, the ▾ ▸ · mark and the open state, and leaves out the children of collapsed nodes.
4. The view registers `treeActions(prefix, collection, when)`: expand, collapse, expand all, collapse all. Selection, bulk verbs, search and the detail pane are the ones of the shared list. A node that stands for no row (a virtual parent) gets a stable negative id (`treeRowID`) so it can be selected.
5. Verbs that change a path use prefix rewrites (`tree.Rebase`) so a parent carries its subtree. The Tags verbs are the reference.

## Navigation (arrows only, no new bare key)

| Key | Behavior |
|---|---|
| `up`, `down` | move through the visible rows |
| `right` | expand a collapsed node, else go to its first child. On a node without children `right` is the focus-detail key of every list |
| `left` | collapse an open node, else go to the parent |
| `enter`, `e` | activate: open the node (the tag page for a tag), the same rule as lists |
| `home`, `end` | first and last row |
| `ctrl+up`, `ctrl+down` | the jump unit is the sibling group (`unitSibling` in `navunits.go`): the previous or next row at the same level, and when there is none the node one level out. At the ends the first or last row |

The module registers `right` and `left` in the view scope of the collection, so they win over focus-detail only while the tree layout is on ([[keymap-layers-and-scopes]], [[navigation-units-and-homes]]). `tab` still moves into the detail pane.

## Rows

- Title: the last segment, indented two spaces per level, after the mark. A search shows the matches and their ancestors with every node open, `esc` restores the open state.
- Meta: the node's own count, and with children `own (total)`. A virtual parent shows `(total)`.
- Sort by count applies to the flat layout, the tree keeps names in order.

## Layout choice and memory

The Tags module has Layout (flat list or tree) and Tag page (own objects or include children) in the `ctrl+o` overlay. Both are remembered per device in `config.toml` (`[tags] layout`, `page`), read by `modules.Value` and written by `modules.SetValue`. The default is the flat list. The possible case duplicates filter always uses its own grouped list.

**Larger patterns.** [[one-abstraction-many-contexts]], [[grouped-lists-with-dividers]], [[bulk-actions-always]], [[universal-verbs]]

**See also.** [[nested-tags]], [[features-tags-module]], [[current-tui-keys]]
