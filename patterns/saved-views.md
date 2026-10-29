---
type: pattern
scale: product
status: built, owner request 2026-10-29 (branch feat/saved-views)
---

# Saved views

**Problem.** A user who often looks at the same slice of data (open deals above a value, people tagged client who are due, tasks due this week grouped by day) rebuilds it each time with the view options. The built-in views are fixed and the only way to add one was code.

**Therefore.** A saved view is a module the user creates. It has a name, an object kind (person, organization, deal, task, note, tag), a filter, a sort, a group-by, optional columns and an "include in tabs" state. It appears in the Views screen next to the built-in views ([[views-screen]]), is shown or hidden and ordered like them, and takes a tab and a number key while it is shown. It renders with the list of its kind with the filter, sort and group applied, so marks, bulk verbs, the detail pane and `ctrl+o` work as in any list ([[bulk-actions-always]], [[one-abstraction-many-contexts]]). Grouping uses the grouped-list component ([[grouped-lists-with-dividers]]).

**Covered.**

- **Kinds:** person, organization, deal, task, note, tag. Operators by field type are in [[saved-view-query]]. The query is neutral data, never SQL.
- **Storage:** table `saved_view` (migration file 018, the version of a database is the position of the file) with uuid, name (unique among live rows, ignoring case), kind, query as JSON, position, `show_in_tabs`, `example`, `created_at`, `updated_at`, `deleted_at`. Delete is soft and Restore brings it back ([[soft-delete-restorable]]). Relation values in the query are uuids, so a view means the same on every device.
- **Object registry:** `model.SavedViewKind` (`saved_view`) in `model.Fields`, `repo.ArchiveObject`, `repo.RestoreObject` and `repo.LookupRecord`, and `savedViewDef` in the TUI registry, so Edit view, Delete view and Restore view are generated like for every object ([[one-verb-set]], [[universal-verbs]]). Add is the palette action "Save current view as...". Link does not apply.
- **CLI:** `crm views list|show|run|save|edit|rename|delete|restore|move|export|import|fields`. **MCP:** `crm_view_*`. Export and import use a small TOML file with the kind, filter, sort and group, relation values written as names and nothing personal.
- **TUI:** the Views screen lists saved views after the built-in ones (shown, Hidden, Deleted views). `enter` opens, `x` shows or hides, `shift+up` and `shift+down` move, `e` edits, `d` asks y or n and deletes, `r` restores. The editor (Edit view) is one row per field and one row per condition, each cell edited in place with the shared field editors, relation and tag values with the typeahead, `a` adds a condition, `d` removes it, `ctrl+s` saves. `ctrl+o` in a saved view shows its filter, sort and group and changes them for the session only. "Save current view as..." captures kind, search, sort and, from a saved view, its session options. No new bare key.
- **Examples:** Overdue follow-up (people not contacted in 90 days), Open deals by stage (grouped by stage in stage order), Tasks due this week (grouped by day). Fixed uuids, `example = 1`, off by default.
- **Validation:** one place, `internal/validate` (E_VIEW_KIND_UNKNOWN, E_VIEW_NAME_INVALID, E_QUERY_FIELD_UNKNOWN, E_QUERY_OPERATOR_INVALID, E_QUERY_VALUE_INVALID, E_QUERY_SORT_INVALID, E_QUERY_GROUP_INVALID, E_QUERY_TOO_LARGE), raised by `internal/query`.

**How device sync picks it up.** The table is in the replica registry (`internal/replica/replicasql/registry.go`, one line `{Name: "saved_view"}`). The query is one JSON field with the default last-writer-wins policy, a delete is the soft `deleted_at`. The examples have fixed uuids, so every device already agrees on them. The arrangement of the built-in modules (which are shown, in what order) stays per device in `config.toml`. Whether a saved view takes a tab is part of the synced row, and saved views always come after the built-in tabs.

**Limits.**

- Saved views cannot be placed between built-in tabs. They follow them, in their own order.
- The number keys run out: the default arrangement already uses 1 to 0, so a shown saved view has a tab and the palette entry "View: name" but no key until a built-in module is hidden.
- The bar search of People still jumps to the plain People list. In a saved view the bar text filters the rows of the view only when set before opening it.
- A saved view cannot nest another, and a view of a removed kind cannot exist: the kind is a closed list.
- OR across conditions is only inside one condition (any-of). There is no OR group.
- The Follow-up, Birthdays and Home lists cannot be saved from the TUI (they are not a list of one kind with a plain query). Use People with a `last_contacted` or `birthday` condition.

**Larger patterns.** [[modules-are-user-arranged]], [[views-screen]]

**Smaller patterns.** [[saved-view-query]], [[grouped-lists-with-dividers]], [[bulk-actions-always]]

**Sources.** Owner request 2026-10-29. [[research-view-module-standard]]
