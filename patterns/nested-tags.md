---
type: pattern
scale: object
status: decided by owner 2026-10-29 (Q68, Q72, Q73), built on branch feat/nested-tags
---
# Nested tags

**Therefore.** A tag named `work/clients/acme` is a child of `work/clients`, which is a child of `work`. The hierarchy comes from the name alone. There is no parent column and no migration. The schema flags the name field with `hierarchy: path` (`model.Fields["tag"]`, `FieldSpec.Hierarchy`), so any other collection with path-like names gets the same behavior by declaring the same property ([[tree-view-module]]). A parent that no tag is named after is a virtual parent: it shows in the tree so its children have a place, and it has no objects of its own.

**Because.** Obsidian's Tag Wrangler and Bulk Tag Manager show this is the shape people already write (`#work/clients`). A path in the name needs no second source of truth, so a rename, an import, a device sync or an agent's `crm tag apply` cannot leave a parent pointer behind. It follows [[field-types-drive-editors]]: a declared field property drives the editor, the check and the view.

## Rules

1. **Path syntax.** `/` separates levels. A segment is any text without `/`. Spaces around a segment are trimmed.
2. **Validation** is in `internal/validate` ([[one-validation-framework]]), checked at every door (CLI, MCP, TUI field editor, `#parent/child` tokens, the pipe line `related` segment, imports):

| Code | Rule |
|---|---|
| E_PATH_EDGE_SLASH | no leading or trailing `/` |
| E_PATH_EMPTY_SEGMENT | no empty segment (`a//b`) |
| E_PATH_TOO_DEEP | at most 8 levels (`model.MaxPathDepth`) |
| E_PATH_TOO_LONG | at most 200 characters (`model.MaxPathLen`) |
| E_PATH_CYCLE | a rename, nest or merge may not move a tag under itself |

3. **Counts.** Each node has its own objects and the objects including the children, an object counted once. Counts leave out archived objects.
4. **Rename** of a parent renames every descendant (a path prefix rewrite). A name that exists, in any case, asks the existing "merge into it?" and merges, children with the same names merging too. One undo step.
5. **Delete** of a parent untags the whole subtree: the dialog shows the count and asks for it typed back, one undo step. A virtual parent deletes the tags below it. A single tag with no children keeps the y/n question.
6. **Merge** (`m`): the children of the source become children of the target and children with the same name merge.
7. **Nest** is the palette action "Tags: nest under..." for the selected tags (bulk). The parent is picked or created with the pick-or-create typeahead, "(top level)" is a choice. It is a path rename. **Unnest** moves the selected tags up one level. A tag under another selected tag follows it. A taken name asks to merge. One undo step for the whole selection. CLI: `crm tag nest <tags...> --under <parent>`, `crm tag unnest <tags...>`.
8. **Tag page.** A parent's page lists its own objects. The view option "Tag page: include children" lists the objects of the tags below it as well, each once, and a row that came in through a child names that tag.
9. **Orphan and duplicate tools respect the hierarchy.** A tag with no objects of its own but a tag in use below it is a parent in use, not unused (`TagStat.Parent`). Case duplicates compare whole paths, a virtual parent counts (`Work` and `work`), merging a duplicate parent merges its children, and groups run parents first.
10. **Tag application.** `#parent/child` in short syntax is one tag. `crm tag apply` and the pipe line accept paths. Applying a path never creates rows for the parents.
11. **CLI and MCP.** `crm tag tree` (indented text, `-f json` for a flat list in tree order with `path`, `depth`, `virtual`, `own`, `total`, `children`), `crm tag nest`, `crm tag unnest`, `crm tag delete` (children go too, `--only-this` keeps them), `crm tag rename --merge`. MCP: `crm_tag_tree`, `crm_tag_nest`, `crm_tag_unnest`, `crm_tag_delete`, and `crm_tag_rename` and `crm_tag_merge` move the children. All call the same repo functions (`internal/db/repo/tag_tree.go`).

## How a collection declares a hierarchy

See [[tree-view-module]].

**Larger patterns.** [[field-types-drive-editors]], [[universal-verbs]], [[bulk-actions-always]], [[one-abstraction-many-contexts]]

**See also.** [[features-tags-module]], [[tree-view-module]], [[hash-means-tag]]

**Inspired by.** Obsidian Tag Wrangler and Bulk Tag Manager ([[inspirations]])
