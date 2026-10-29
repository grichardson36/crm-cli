---
type: feature-area
---
# Tags module

The Tags view: tags as a module of their own, with the universal verbs and no tag-specific key. Branch feat/tags-module (owner request 2026-10-30), not merged. Inspired by Obsidian's Tag Wrangler (rename and merge from a list of tags). The older tag rows (add, apply, delete, rename, merge) are in [[features-tags-groups-and-views]].

**Follows patterns:** [[universal-verbs]], [[bulk-actions-always]], [[grouped-lists-with-dividers]], [[one-abstraction-many-contexts]], [[related-segment]], [[field-types-drive-editors]], [[hash-means-tag]], [[modules-are-user-arranged]]

**See also:** [[index]], [[current-tui-keys]], [[nested-tags]], [[tree-view-module]]

## Behavior

- Module `tags` ("Tags", labels on anything, rename, merge, clean up) in the default order after Deals. Number keys of the modules after it move by one (Birthdays 8, Merge 9, Archived 0, Sync has none and opens with `v`).
- The list shows the tags in use: name, a dim breakdown by kind ("1 person, 1 org") and the number of objects. `ctrl+o` offers Sort (name, number of objects) and Filter (tags in use, unused tags, possible case duplicates, all tags). `/` finds a tag by name, `esc` clears it. Counts leave out archived objects. A tag with only archived objects is not unused.
- `enter` opens the tag page: the objects that carry the tag, grouped by kind under dim headings in the kind colors of the related segment. `enter` on an object opens it in its own view, `esc` goes back.
- `tab` moves into the detail pane. `enter` or `e` on the name edits it in place and renames the tag on every object. A name another tag has (in any case) asks once "merge into it?" and merges. One undo step either way.
- `d` deletes: a tag in use is untagged everywhere (existing behavior), an unused name is removed. `m` merges the `space`-selected tags into the highlighted one, with nothing selected it asks which tag to merge into. `l` gives the tag to an object of any kind (on a tag page it links another object to the page's tag), `u` takes it off (on a page: the highlighted or selected objects). `a g` adds a tag. `space`, `ctrl+a` and `shift+up`/`shift+down` select. Several tags or objects in one destructive verb ask for the count typed back, and the whole verb is one undo step.
- Case duplicates are tags that differ only by case, spaces, hyphens or underscores (`internal/db/repo/tag_manage.go`, `DupKey`). A slash is not a separator because `parent/child` is a nested tag ([[nested-tags]]) and the levels stay apart. Palette: "Tags: merge case duplicates" merges each group into its most used spelling (ties go to the oldest tag), "Tags: delete unused tags" removes the names no object carries. Both show the count, ask for it typed back and undo as one step.
- Nested tags (branch feat/nested-tags, owner decisions 2026-10-29): a tag named `work/clients/acme` is a child of `work/clients`. `ctrl+o` has Layout (flat list or tree, per device) and Tag page (own objects or include children, per device). In the tree `up`/`down` move, `right` opens a node or goes to its first child, `left` closes it or goes to the parent, `ctrl+up`/`ctrl+down` jump between siblings, `enter`/`e` open the tag page. Counts show `own (with children)`, a parent no tag is named after is a virtual parent. Rename of a parent renames its children and a taken name asks to merge. `d` on a parent shows the tags and taggings and asks for the tag count typed back, `m` merges the children along. Palette: Tags: nest under..., Tags: unnest, Tags: tree or flat layout, Tags: include children on tag pages, Tree: expand all, Tree: collapse all. A parent in use is not an unused tag and case duplicates compare whole paths, parents first. CLI: `crm tag tree`, `crm tag nest`, `crm tag unnest`, `rename --merge`, `delete --only-this`. MCP: `crm_tag_tree`, `crm_tag_nest`, `crm_tag_unnest`, `crm_tag_delete`.
- A tag row in the command bar is this view with one row (`enterBarContext`), so `d`, `l`, `u`, `m` and `e` there are the same actions, not copies.
- CLI: `crm tag unused` and `crm tag duplicates` list (JSON or table), `--apply` deletes or merges. MCP: `crm_tag_unused`, `crm_tag_duplicates` (`apply`).

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| TM01 | Tags view with name, count, breakdown by kind, sort and filter (web) | Obsidian Tag Wrangler | DONE | BRANCH feat/tags-module. Module `tags`, `ctrl+o` Sort and Filter, `/` search | - | - | fast, simple | P2 |
| TM02 | Tag page: objects grouped by kind, open, link, unlink (Us) | Us | DONE | BRANCH feat/tags-module. `enter` opens the page, grouped list with kind colored headings | - | - | connected | P2 |
| TM03 | Rename in place, rename into an existing tag merges (web) | Obsidian Tag Wrangler | DONE | BRANCH feat/tags-module. Name field of the detail pane, one confirm, one undo step | `enter` opens the page and `e` is the same action, so the rename starts from the detail pane (`tab` then `enter`). Should `e` on the list rename at once? | - | tactile, safe | P2 |
| TM04 | Bulk verbs on selected tags: delete, merge into the highlighted, link, unlink (Us) | Us | DONE | BRANCH feat/tags-module. Typed count for several, one undo step | - | - | fast, safe | P2 |
| TM05 | Case duplicates and unused tags: filters, palette clean-up, CLI and MCP (Us) | Us | DONE | BRANCH feat/tags-module. `crm tag duplicates`, `crm tag unused`, `crm_tag_duplicates`, `crm_tag_unused` | - | - | smart, safe | P2 |
| TM06 | Nested tags by name path, `hierarchy: path` field property, virtual parents, counts with children (web) | Obsidian Tag Wrangler | DONE | BRANCH feat/nested-tags. `model.FieldSpec.Hierarchy`, `internal/tree`, `internal/validate` path codes, `repo/tag_tree.go`. No migration | - | - | smart | P3 |
| TM07 | Tree view module: expand and collapse, own and total counts, search, selection and every verb, arrows only, jump unit sibling group (Us) | Obsidian, k9s | DONE | BRANCH feat/nested-tags. `internal/tui/treeview.go`, Tags `ctrl+o` Layout, kept per device in `[tags]` of config.toml | The tree shows names in order, Sort by count applies to the flat list | - | tactile, fast | P3 |
| TM08 | Tree verbs: rename and delete a subtree, merge with children, nest and unnest (bulk), tag page include children (Us) | Obsidian Bulk Tag Manager | DONE | BRANCH feat/nested-tags. One undo step each, typed count for a subtree delete | Unnest moves up one level, not to the top | - | safe, connected | P3 |
| TM09 | Nested tags on the command line and MCP: tree, nest, unnest, delete with children (Us) | Us | DONE | BRANCH feat/nested-tags. `crm tag tree`, `nest`, `unnest`, `crm_tag_tree`, `crm_tag_nest`, `crm_tag_unnest`, `crm_tag_delete` | - | - | smart | P3 |
